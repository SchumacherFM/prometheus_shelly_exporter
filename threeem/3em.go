package threeem

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/samber/lo"

	"github.com/corestoreio/pkg/util/byteconv"
	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
)

type Collector struct {
	opts                Options
	powerDesc           *prometheus.Desc
	pfDesc              *prometheus.Desc
	currentDesc         *prometheus.Desc
	voltageDesc         *prometheus.Desc
	totalDesc           *prometheus.Desc
	totalReturnedDesc   *prometheus.Desc
	energyDesc          *prometheus.Desc
	energyReturnedDesc  *prometheus.Desc
	upDesc              *prometheus.Desc
	topicValueCollector map[string]float64 // topic => value
}

type Options struct {
	Timeout time.Duration
	Log     *zap.Logger
	TestCB  func()
}

func NewCollector(ctx context.Context, messageChan <-chan mqtt.Message, opts Options) *Collector {
	// Note, that energy and returned_energy do not survive power cycle or
	// reboot -- this is how the value is implemented on other Shellies. Shelly
	// 3EM features a persisted version which is not affected by power cycling
	// or lack of connectivity. To get the persisted counters use total and
	// total_returned.
	c := &Collector{
		opts:               opts,
		powerDesc:          prometheus.NewDesc("shelly3em_power", "instantaneous active power in Watts", []string{"device", "phase"}, nil),
		pfDesc:             prometheus.NewDesc("shelly3em_pf", "power factor (dimensionless)", []string{"device", "phase"}, nil),
		currentDesc:        prometheus.NewDesc("shelly3em_current", "current in Amps", []string{"device", "phase"}, nil),
		voltageDesc:        prometheus.NewDesc("shelly3em_voltage", "grid voltage in Volts", []string{"device", "phase"}, nil),
		totalDesc:          prometheus.NewDesc("shelly3em_total", "total energy in Wh (accumulated in device's non-volatile memory)", []string{"device", "phase"}, nil),
		totalReturnedDesc:  prometheus.NewDesc("shelly3em_total_returned", "total energy returned to the grid in Wh (accumulated in device's non-volatile memory)", []string{"device", "phase"}, nil),
		energyDesc:         prometheus.NewDesc("shelly3em_energy", "energy counter in Watt-minute since last report", []string{"device", "phase"}, nil),
		energyReturnedDesc: prometheus.NewDesc("shelly3em_energy_returned", "energy returned to the grid in Watt-minute since last report", []string{"device", "phase"}, nil),
		upDesc:             prometheus.NewDesc("shelly3em_up", "Whether scrape was successful", []string{"last_error"}, nil),
	}
	_ = c.swapTopicValueCollector()

	go func() {
		for {
			select {
			case msg, ok := <-messageChan:
				if !ok {
					if opts.TestCB != nil {
						opts.TestCB()
					}
					return
				}
				if false == strings.Contains(msg.Topic(), "/emeter/") {
					continue
				}

				f64, _, err := byteconv.ParseFloat(msg.Payload())
				if err != nil {
					opts.Log.Error("failed to parse payload", zap.Error(err), zap.String("topic", msg.Topic()))
					continue
				}

				c.topicValueCollector[msg.Topic()] = f64

			case <-ctx.Done():
				return
			}
		}
	}()

	return c
}

func (c *Collector) swapTopicValueCollector() []lo.Entry[string, float64] {
	lm := c.topicValueCollector
	c.topicValueCollector = make(map[string]float64, 24)
	entries := lo.Entries(lm)
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Key < entries[j].Key
	})

	return entries
}

func (c *Collector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.powerDesc
	ch <- c.pfDesc
	ch <- c.currentDesc
	ch <- c.voltageDesc
	ch <- c.totalDesc
	ch <- c.totalReturnedDesc
	ch <- c.energyDesc
	ch <- c.energyReturnedDesc
	ch <- c.upDesc
}

func (c *Collector) Collect(ch chan<- prometheus.Metric) {
	if err := c.collect(ch); err == nil {
		ch <- prometheus.MustNewConstMetric(c.upDesc, prometheus.GaugeValue, 1, "")
	} else {
		c.opts.Log.Error("Scrape failed", zap.Error(err))
		ch <- prometheus.MustNewConstMetric(c.upDesc, prometheus.GaugeValue, 0, err.Error())
	}
}

func getMsgInfo(topic string) (deviceID, phaseID, lastPath string) {
	topicPaths := strings.Split(topic, "/")
	// shellies/shellyem3-washtumbler/emeter/0/voltage
	deviceID, _ = strings.CutPrefix(topicPaths[1], "shellyem3-")

	return deviceID, topicPaths[len(topicPaths)-2], topicPaths[len(topicPaths)-1]
}

func (c *Collector) collect(ch chan<- prometheus.Metric) error {
	tv := c.swapTopicValueCollector()

	for _, kv := range tv {

		deviceID, phaseID, lastPath := getMsgInfo(kv.Key)

		switch lastPath {
		case "energy":
			ch <- prometheus.MustNewConstMetric(c.energyDesc, prometheus.GaugeValue, kv.Value, deviceID, phaseID)
		case "returned_energy":
			ch <- prometheus.MustNewConstMetric(c.energyReturnedDesc, prometheus.GaugeValue, kv.Value, deviceID, phaseID)
		case "total":
			ch <- prometheus.MustNewConstMetric(c.totalDesc, prometheus.GaugeValue, kv.Value, deviceID, phaseID)
		case "total_returned":
			ch <- prometheus.MustNewConstMetric(c.totalReturnedDesc, prometheus.GaugeValue, kv.Value, deviceID, phaseID)
		case "power":
			ch <- prometheus.MustNewConstMetric(c.powerDesc, prometheus.GaugeValue, kv.Value, deviceID, phaseID)
		case "voltage":
			ch <- prometheus.MustNewConstMetric(c.voltageDesc, prometheus.GaugeValue, kv.Value, deviceID, phaseID)
		case "current":
			ch <- prometheus.MustNewConstMetric(c.currentDesc, prometheus.GaugeValue, kv.Value, deviceID, phaseID)
		case "pf":
			ch <- prometheus.MustNewConstMetric(c.pfDesc, prometheus.GaugeValue, kv.Value, deviceID, phaseID)
		default:
			c.opts.Log.Warn("unhandled topic", zap.String("topic", kv.Key), zap.Float64("value", kv.Value))
		}
	}
	return nil
}

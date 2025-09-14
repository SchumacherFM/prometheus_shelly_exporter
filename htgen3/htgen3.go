package htgen3

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/tidwall/gjson"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
)

type Event struct {
	Src    string `json:"src"`
	Dst    string `json:"dst"`
	Method string `json:"method"`
	Params Params `json:"params"`
}
type (
	Ble   struct{}
	Cloud struct {
		Connected bool `json:"connected"`
	}
)

type Battery struct {
	V       float64 `json:"V"`
	Percent int     `json:"percent"`
}
type External struct {
	Present bool `json:"present"`
}
type Devicepower0 struct {
	ID       int      `json:"id"`
	Battery  Battery  `json:"battery"`
	External External `json:"external"`
}
type (
	HtUI      struct{}
	Humidity0 struct {
		ID int     `json:"id"`
		Rh float64 `json:"rh"`
	}
)

type Mqtt struct {
	Connected bool `json:"connected"`
}
type (
	AvailableUpdates struct{}
	WakeupReason     struct {
		Boot  string `json:"boot"`
		Cause string `json:"cause"`
	}
)

type Sys struct {
	Mac              string           `json:"mac"`
	RestartRequired  bool             `json:"restart_required"`
	Time             any              `json:"time"`
	Unixtime         any              `json:"unixtime"`
	LastSyncTs       any              `json:"last_sync_ts"`
	Uptime           int              `json:"uptime"`
	RAMSize          int              `json:"ram_size"`
	RAMFree          int              `json:"ram_free"`
	RAMMinFree       int              `json:"ram_min_free"`
	FsSize           int              `json:"fs_size"`
	FsFree           int              `json:"fs_free"`
	CfgRev           int              `json:"cfg_rev"`
	KvsRev           int              `json:"kvs_rev"`
	WebhookRev       int              `json:"webhook_rev"`
	AvailableUpdates AvailableUpdates `json:"available_updates"`
	WakeupReason     WakeupReason     `json:"wakeup_reason"`
	WakeupPeriod     int              `json:"wakeup_period"`
	ResetReason      int              `json:"reset_reason"`
	UtcOffset        int              `json:"utc_offset"`
}
type Temperature0 struct {
	ID int     `json:"id"`
	TC float64 `json:"tC"`
	TF float64 `json:"tF"`
}
type Wifi struct {
	StaIP  string `json:"sta_ip"`
	Status string `json:"status"`
	Ssid   string `json:"ssid"`
	Rssi   int    `json:"rssi"`
	StaIP6 any    `json:"sta_ip6"`
}
type Ws struct {
	Connected bool `json:"connected"`
}
type Params struct {
	Ts           float64      `json:"ts"`
	Ble          Ble          `json:"ble"`
	Cloud        Cloud        `json:"cloud"`
	Devicepower0 Devicepower0 `json:"devicepower:0"`
	HtUI         HtUI         `json:"ht_ui"`
	Humidity0    Humidity0    `json:"humidity:0"`
	Mqtt         Mqtt         `json:"mqtt"`
	Sys          Sys          `json:"sys"`
	Temperature0 Temperature0 `json:"temperature:0"`
	Wifi         Wifi         `json:"wifi"`
	Ws           Ws           `json:"ws"`
}

type lastMSG struct {
	time  time.Time
	topic string
	msg   mqtt.Message
}

type Collector struct {
	opts    Options
	tmpDesc *prometheus.Desc
	humDesc *prometheus.Desc
	batDesc *prometheus.Desc
	upDesc  *prometheus.Desc
	lastMSG lastMSG
}

type Options struct {
	Timeout time.Duration
	Log     *zap.Logger
}

func NewCollector(ctx context.Context, messageChan <-chan mqtt.Message, opts Options) *Collector {
	c := &Collector{
		opts:    opts,
		tmpDesc: prometheus.NewDesc("shellyhtgen3_temperature", "Sensor temperature", []string{"device", "unit"}, nil),
		humDesc: prometheus.NewDesc("shellyhtgen3_humidity", "Sensor humidity", []string{"device", "unit"}, nil),
		batDesc: prometheus.NewDesc("shellyhtgen3_battery", "Sensor battery", []string{"device", "unit"}, nil),
		upDesc:  prometheus.NewDesc("shellyhtgen3_up", "Whether scrape was successful", []string{"status"}, nil),
		lastMSG: lastMSG{
			time:  time.Now(),
			topic: nullTopic,
			msg:   nullMessage{},
		},
	}

	go func() {
		for {
			select {
			case msg, ok := <-messageChan:
				if !ok {
					return
				}

				if r := gjson.GetBytes(msg.Payload(), "method"); r.String() == "NotifyFullStatus" {
					opts.Log.Debug("message from mqtt",
						zap.String("topic", msg.Topic()),
						zap.Int("length", len(msg.Payload())))

					// race condition but who cares, just old data gets overwritten
					c.lastMSG = lastMSG{
						time:  time.Now(),
						topic: msg.Topic(),
						msg:   msg,
					}
				}

			case <-ctx.Done():
				return
			}
		}
	}()

	return c
}

func (c *Collector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.upDesc
	ch <- c.tmpDesc
	ch <- c.humDesc
	ch <- c.batDesc
}

func (c *Collector) Collect(ch chan<- prometheus.Metric) {
	ctx, cancel := context.WithTimeout(context.Background(), c.opts.Timeout)
	defer cancel()

	if err := c.collect(ctx, ch); err == nil {
		ch <- prometheus.MustNewConstMetric(c.upDesc, prometheus.GaugeValue, 1, "")
	} else {
		c.opts.Log.Error("Scrape failed", zap.Error(err))
		ch <- prometheus.MustNewConstMetric(c.upDesc, prometheus.GaugeValue, 0, err.Error())
	}
}

func (c *Collector) collect(ctx context.Context, ch chan<- prometheus.Metric) error {
	switch {
	case strings.HasSuffix(c.lastMSG.msg.Topic(), "/rpc"):
		var info Event
		if err := json.Unmarshal(c.lastMSG.msg.Payload(), &info); err != nil {
			return fmt.Errorf("collect: json unmarshal failed: %w for data: %q", err, c.lastMSG.msg.Payload())
		}
		if info.Method != "NotifyFullStatus" {
			// not interested in NotifyStatus
			return nil
		}

		devID := info.Src // MAC address if the device
		ch <- prometheus.MustNewConstMetric(c.tmpDesc, prometheus.GaugeValue, info.Params.Temperature0.TC, devID, "c")
		ch <- prometheus.MustNewConstMetric(c.tmpDesc, prometheus.GaugeValue, info.Params.Temperature0.TF, devID, "f")
		ch <- prometheus.MustNewConstMetric(c.humDesc, prometheus.GaugeValue, info.Params.Humidity0.Rh, devID, "%")
		ch <- prometheus.MustNewConstMetric(c.batDesc, prometheus.GaugeValue, info.Params.Devicepower0.Battery.V, devID, "V")
		ch <- prometheus.MustNewConstMetric(c.batDesc, prometheus.GaugeValue, float64(info.Params.Devicepower0.Battery.Percent), devID, "%")

	default:
		c.opts.Log.Warn("unhandled topic", zap.String("topic", c.lastMSG.msg.Topic()))
	}

	return nil
}

const nullTopic = "shellies/null/rpc"

type nullMessage struct{}

func (nullMessage) Duplicate() bool {
	return false
}

func (nullMessage) Qos() byte {
	return 0
}

func (nullMessage) Retained() bool {
	return false
}

func (nullMessage) Topic() string {
	return nullTopic
}

func (nullMessage) MessageID() uint16 {
	return 0
}

func (nullMessage) Payload() []byte {
	return []byte(`{}`)
}

func (nullMessage) Ack() {
}

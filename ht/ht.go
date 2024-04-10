package ht

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
)

// Info represents the minimal data. If you want more, let me know.
type Info struct {
	Mac     string `json:"mac"`
	IsValid bool   `json:"is_valid"`
	Tmp     struct {
		Value   float64 `json:"value"`
		Units   string  `json:"units"`
		TC      float64 `json:"tC"`
		TF      float64 `json:"tF"`
		IsValid bool    `json:"is_valid"`
	} `json:"tmp"`
	Hum struct {
		Value   float64 `json:"value"`
		IsValid bool    `json:"is_valid"`
	} `json:"hum"`
	Bat struct {
		Value   int     `json:"value"`
		Voltage float64 `json:"voltage"`
	} `json:"bat"`
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
		tmpDesc: prometheus.NewDesc("shellyht_temperature", "Sensor temperature", []string{"device", "unit"}, nil),
		humDesc: prometheus.NewDesc("shellyht_humidity", "Sensor humidity", []string{"device", "unit"}, nil),
		batDesc: prometheus.NewDesc("shellyht_battery", "Sensor battery", []string{"device", "unit"}, nil),
		upDesc:  prometheus.NewDesc("shellyht_up", "Whether scrape was successful", []string{"status"}, nil),
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
				if false == strings.HasSuffix(msg.Topic(), "/info") {
					continue
				}

				c.lastMSG = lastMSG{
					time:  time.Now(),
					topic: msg.Topic(),
					msg:   msg,
				}
				opts.Log.Info("message from mqtt",
					zap.String("topic", msg.Topic()),
					zap.Int("length", len(msg.Payload())))

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
	case strings.HasSuffix(c.lastMSG.msg.Topic(), "/info"):
		var info Info
		if err := json.Unmarshal(c.lastMSG.msg.Payload(), &info); err != nil {
			return fmt.Errorf("collect: json unmarshal failed: %w for data: %q", err, c.lastMSG.msg.Payload())
		}
		devID := info.Mac // MAC address if the device
		ch <- prometheus.MustNewConstMetric(c.tmpDesc, prometheus.GaugeValue, info.Tmp.TC, devID, "c")
		ch <- prometheus.MustNewConstMetric(c.tmpDesc, prometheus.GaugeValue, info.Tmp.TF, devID, "f")
		ch <- prometheus.MustNewConstMetric(c.humDesc, prometheus.GaugeValue, info.Hum.Value, devID, "%")
		ch <- prometheus.MustNewConstMetric(c.batDesc, prometheus.GaugeValue, info.Bat.Voltage, devID, "V")
		ch <- prometheus.MustNewConstMetric(c.batDesc, prometheus.GaugeValue, float64(info.Bat.Value), devID, "%")

	default:
		c.opts.Log.Warn("unhandled topic", zap.String("topic", c.lastMSG.msg.Topic()))
	}

	return nil
}

const nullTopic = "shellies/null/info"

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

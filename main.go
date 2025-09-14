package main

import (
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/SchumacherFM/prometheus_shelly_exporter/ht"
	"github.com/SchumacherFM/prometheus_shelly_exporter/htgen3"
	"github.com/SchumacherFM/prometheus_shelly_exporter/threeem"
	"github.com/eclipse/paho.mqtt.golang"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/collectors/version"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/exporter-toolkit/web"
	slogzap "github.com/samber/slog-zap/v2"
	"github.com/urfave/cli/v2"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func main() {
	app := &cli.App{
		Commands: []*cli.Command{
			{
				Name:   "debug",
				Usage:  "inspect topics and their data",
				Flags:  []cli.Flag{},
				Action: actionDebug,
			},
			{
				Name:  "prom",
				Usage: "forwards the mqtt data towards prometheus",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:    "http-listen-address",
						Value:   ":80",
						EnvVars: []string{"HTTP_LISTEN_ADDRESS"},
					},
					&cli.StringFlag{
						Name:  "http-path-metrics",
						Value: "/metrics",
					},
					&cli.BoolFlag{
						Name:  "enable-exporter-metrics",
						Value: false,
						Usage: "if true sends metrics about the go runtime",
					},
				},
				Action: actionProm,
			},
		},
		Usage: "Converts received data from MQTT towards prometheus format",
		Flags: []cli.Flag{
			&cli.StringSliceFlag{
				Name:     "mqtt-url",
				Required: true,
				Usage:    "mqtt://hostname:port",
				EnvVars:  []string{"MQTT_HOSTS"},
			},
			&cli.StringFlag{
				Name:    "mqtt-user",
				Value:   "",
				Usage:   "mqtt username",
				EnvVars: []string{"MQTT_USERNAME"},
			},
			&cli.StringFlag{
				Name:    "mqtt-pass",
				Value:   "",
				Usage:   "mqtt password",
				EnvVars: []string{"MQTT_PASSWORD"},
			},
			&cli.StringSliceFlag{
				Name:    "topic",
				Aliases: []string{"t"},
				Value:   cli.NewStringSlice("shellies/+/info"), // "shellies/+/online"
				Usage:   "MQTT Topics http://www.steves-internet-guide.com/understanding-mqtt-topics/",
			},
			&cli.BoolFlag{
				Name:  "verbose",
				Value: false,
			},
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}

func newMQTTClient(c *cli.Context) (mqtt.Client, func(), error) {
	o := mqtt.NewClientOptions()
	for _, mqttUrl := range c.StringSlice("mqtt-url") {
		u, err := url.Parse(mqttUrl)
		if err != nil {
			return nil, nil, fmt.Errorf("failed tp parse URL: %q with: %w", mqttUrl, err)
		}
		o.Servers = append(o.Servers, u)
	}
	o.Username = c.String("mqtt-user")
	o.Password = c.String("mqtt-pass")
	o.AutoReconnect = true

	mqc := mqtt.NewClient(o)

	tk := mqc.Connect()
	<-tk.Done()
	if err := tk.Error(); err != nil {
		return nil, nil, fmt.Errorf("failed tp connect: %w", err)
	}

	return mqc, func() {
		mqc.Disconnect(100)
	}, nil
}

func actionDebug(c *cli.Context) error {
	mqc, cancel, err := newMQTTClient(c)
	if err != nil {
		return err
	}
	defer cancel()

	for _, topic := range c.StringSlice("topic") {
		tk := mqc.Subscribe(topic, 0, func(client mqtt.Client, message mqtt.Message) {
			t := time.Now().Format("2006-01-02T15:04:05.999")
			fmt.Printf("%s::: message topic:: %s=%s\n", t, message.Topic(), string(message.Payload()))
			message.Ack()
		})
		<-tk.Done()
		if err := tk.Error(); err != nil {
			return err
		} else {
			fmt.Println("subscribed to:", topic)
		}

	}

	fmt.Println("blocking and waiting for messages")
	<-c.Done() // wait until someone kills us

	return nil
}

func actionProm(c *cli.Context) error {
	mqc, cancel, err := newMQTTClient(c)
	if err != nil {
		return err
	}
	defer cancel()

	zapencCfg := zap.NewProductionEncoderConfig()
	zapencCfg.EncodeTime = zapcore.RFC3339NanoTimeEncoder

	zapLvl := zap.InfoLevel
	if c.Bool("verbose") {
		zapLvl = zap.DebugLevel
	}
	zaplog := zap.New(zapcore.NewCore(
		zapcore.NewJSONEncoder(zapencCfg),
		zapcore.AddSync(os.Stdout),
		zapLvl,
	))
	defer zaplog.Sync()

	messageChanHT := make(chan mqtt.Message)
	messageChanHTGen3 := make(chan mqtt.Message)
	messageChan3EM := make(chan mqtt.Message)
	go subscribe(c, mqc, zaplog, messageChanHT, messageChanHTGen3, messageChan3EM)
	defer mqc.Unsubscribe(c.StringSlice("topic")...)
	defer func() {
		close(messageChanHT)
		close(messageChanHTGen3)
		close(messageChan3EM)
	}()

	reg := prometheus.NewPedanticRegistry()
	reg.MustRegister(ht.NewCollector(c.Context, messageChanHT, ht.Options{
		Timeout: 60 * time.Second,
		Log:     zaplog,
	}))
	reg.MustRegister(htgen3.NewCollector(c.Context, messageChanHTGen3, htgen3.Options{
		Timeout: 60 * time.Second,
		Log:     zaplog,
	}))
	reg.MustRegister(threeem.NewCollector(c.Context, messageChan3EM, threeem.Options{
		Timeout: 60 * time.Second,
		Log:     zaplog,
	}))

	if c.Bool("enable-exporter-metrics") {
		reg.MustRegister(
			collectors.NewBuildInfoCollector(),
			collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
			collectors.NewGoCollector(),
			version.NewCollector("shelly_exporter"),
		)
	}

	http.Handle(c.String("http-path-metrics"), promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html>
			<head><title>Shelly Exporter</title></head>
			<body>
			<h1>Shelly Exporter</h1>
			<p><a href="` + c.String("http-path-metrics") + `">Metrics</a></p>
			</body>
			</html>`))
	})

	server := &http.Server{}

	zaplog.Info("http config",
		zap.String("path", c.String("http-path-metrics")),
		zap.String("listen_address", c.String("http-listen-address")),
	)

	slogLogWrap := slog.New(slogzap.Option{Level: slog.LevelDebug, Logger: zaplog}.NewZapHandler())

	var empty string
	if err := web.ListenAndServe(server, &web.FlagConfig{
		WebListenAddresses: &[]string{c.String("http-listen-address")},
		WebSystemdSocket:   nil,
		WebConfigFile:      &empty,
	}, slogLogWrap); err != nil {
		zaplog.Fatal("ListenAndServe failed", zap.Error(err))
		return err
	}

	return nil
}

func subscribe(c *cli.Context, mqc mqtt.Client, log *zap.Logger, messageChans ...chan<- mqtt.Message) {
	log.Info("subscribing to", zap.Strings("topics", c.StringSlice("topic")))
	for _, topic := range c.StringSlice("topic") {
		tk := mqc.Subscribe(topic, 0, func(client mqtt.Client, message mqtt.Message) {
			for _, messageChan := range messageChans {
				messageChan <- message
			}
			message.Ack()
		})
		<-tk.Done()
		if err := tk.Error(); err != nil {
			log.Error("subscribe failed", zap.Error(err), zap.String("topic", topic))
		} else {
			log.Info("subscribed to", zap.String("topic", topic))
		}
	}
}

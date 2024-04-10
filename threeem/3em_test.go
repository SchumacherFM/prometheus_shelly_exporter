package threeem

import (
	"bufio"
	"context"
	"os"
	"strings"
	"testing"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

var _ prometheus.Collector = (*Collector)(nil)

func TestCollector_collect(t *testing.T) {
	ctx := context.Background()
	msgChan := make(chan mqtt.Message)

	log, _ := zap.NewDevelopment(zap.Development())
	msgGoRoutineDone := make(chan struct{})
	c := NewCollector(ctx, msgChan, Options{
		Log: log,
		TestCB: func() {
			close(msgGoRoutineDone)
		},
	})

	fp, err := os.Open("testdata/3em.txt")
	require.NoError(t, err)
	defer fp.Close()

	s := bufio.NewScanner(fp)
	s.Split(bufio.ScanLines)
	for s.Scan() {
		lineParts := strings.Split(s.Text(), "+01:00:")
		topicValue := strings.Split(lineParts[1], ":")
		msgChan <- mockMsg{
			topic:   topicValue[0],
			payload: topicValue[1],
		}
	}
	require.NoError(t, s.Err())
	close(msgChan)
	<-msgGoRoutineDone

	err = testutil.CollectAndCompare(c, strings.NewReader(`
# HELP shelly3em_current current in Amps
# TYPE shelly3em_current gauge
shelly3em_current{device="washtumbler",phase="0"} 0.01
shelly3em_current{device="washtumbler",phase="1"} 0.04
shelly3em_current{device="washtumbler",phase="2"} 0.01
# HELP shelly3em_energy energy counter in Watt-minute since last report
# TYPE shelly3em_energy gauge
shelly3em_energy{device="washtumbler",phase="0"} 1
shelly3em_energy{device="washtumbler",phase="1"} 1
shelly3em_energy{device="washtumbler",phase="2"} 1
# HELP shelly3em_energy_returned energy returned to the grid in Watt-minute since last report
# TYPE shelly3em_energy_returned gauge
shelly3em_energy_returned{device="washtumbler",phase="0"} 0
shelly3em_energy_returned{device="washtumbler",phase="1"} 0
shelly3em_energy_returned{device="washtumbler",phase="2"} 0
# HELP shelly3em_pf power factor (dimensionless)
# TYPE shelly3em_pf gauge
shelly3em_pf{device="washtumbler",phase="0"} 0.12
shelly3em_pf{device="washtumbler",phase="1"} 0.04
shelly3em_pf{device="washtumbler",phase="2"} 0.1
# HELP shelly3em_power instantaneous active power in Watts
# TYPE shelly3em_power gauge
shelly3em_power{device="washtumbler",phase="0"} 0
shelly3em_power{device="washtumbler",phase="1"} 0
shelly3em_power{device="washtumbler",phase="2"} 0
# HELP shelly3em_total total energy in Wh (accumulated in device's non-volatile memory)
# TYPE shelly3em_total gauge
shelly3em_total{device="washtumbler",phase="0"} 610.1
shelly3em_total{device="washtumbler",phase="1"} 1110.3
shelly3em_total{device="washtumbler",phase="2"} 17.9
# HELP shelly3em_total_returned total energy returned to the grid in Wh (accumulated in device's non-volatile memory)
# TYPE shelly3em_total_returned gauge
shelly3em_total_returned{device="washtumbler",phase="0"} 0
shelly3em_total_returned{device="washtumbler",phase="1"} 0
shelly3em_total_returned{device="washtumbler",phase="2"} 0.6
# HELP shelly3em_up Whether scrape was successful
# TYPE shelly3em_up gauge
shelly3em_up{last_error=""} 1
# HELP shelly3em_voltage grid voltage in Volts
# TYPE shelly3em_voltage gauge
shelly3em_voltage{device="washtumbler",phase="0"} 231.39
shelly3em_voltage{device="washtumbler",phase="1"} 230.79
shelly3em_voltage{device="washtumbler",phase="2"} 231.15
`),
		"shelly3em_power",
		"shelly3em_pf",
		"shelly3em_current",
		"shelly3em_voltage",
		"shelly3em_total",
		"shelly3em_total_returned",
		"shelly3em_energy",
		"shelly3em_energy_returned",
		"shelly3em_up",
	)
	require.NoError(t, err)
}

type mockMsg struct {
	topic   string
	payload string
}

func (mockMsg) Duplicate() bool {
	// TODO implement me
	panic("implement me")
}

func (mockMsg) Qos() byte {
	// TODO implement me
	panic("implement me")
}

func (mockMsg) Retained() bool {
	// TODO implement me
	panic("implement me")
}

func (m mockMsg) Topic() string {
	return m.topic
}

func (mockMsg) MessageID() uint16 {
	// TODO implement me
	panic("implement me")
}

func (m mockMsg) Payload() []byte {
	return []byte(m.payload)
}

func (mockMsg) Ack() {
}

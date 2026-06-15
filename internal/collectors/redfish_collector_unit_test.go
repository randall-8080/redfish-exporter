package collectors

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"

	"github.com/e-breuninger/redfish-exporter/internal/config"
)

// stubCollector is a prometheus.Collector that emits a single constant metric.
// If releaseC is set, Collect blocks until the channel is closed.
type stubCollector struct {
	desc     *prometheus.Desc
	releaseC chan struct{}
}

func newStubCollector(name string) *stubCollector {
	return &stubCollector{
		desc: prometheus.NewDesc(name, name, nil, nil),
	}
}

func (s *stubCollector) Describe(ch chan<- *prometheus.Desc) { ch <- s.desc }

func (s *stubCollector) Collect(ch chan<- prometheus.Metric) {
	if s.releaseC != nil {
		<-s.releaseC
	}
	ch <- prometheus.MustNewConstMetric(s.desc, prometheus.GaugeValue, 1.0)
}

// makeTestRedfishCollector builds a RedfishCollector with pre-built sub-collectors,
// bypassing the gofish client dependency for unit testing.
func makeTestRedfishCollector(ctx context.Context, target string, subs []namedCollector) *RedfishCollector {
	return &RedfishCollector{
		target: target,
		ctx:    ctx,
		subs:   subs,
		durationDesc: prometheus.NewDesc(
			"redfish_exporter_collector_duration_seconds",
			"Collector scrape duration",
			[]string{"target", "collector"},
			nil,
		),
	}
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestNewRedfishCollector_SubCollectorSelection(t *testing.T) {
	tests := []struct {
		name     string
		cfg      config.CollectorsConfig
		wantSubs int
	}{
		{"no collectors enabled", config.CollectorsConfig{}, 0},
		{"thermal only", config.CollectorsConfig{Thermal: true}, 1},
		{"power only", config.CollectorsConfig{Power: true}, 1},
		{"systems only", config.CollectorsConfig{Systems: true}, 1},
		{"memory only", config.CollectorsConfig{Memory: true}, 1},
		{"all enabled", config.CollectorsConfig{Thermal: true, Power: true, Systems: true, Memory: true}, 4},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// nil client is safe here — sub-collectors only store it, never call it during construction.
			c := NewRedfishCollector(context.Background(), "http://bmc.example.com", nil, discardLogger(), tt.cfg)
			assert.Len(t, c.subs, tt.wantSubs)
		})
	}
}

func TestRedfishCollector_Describe_SendsSubAndDurationDescriptors(t *testing.T) {
	subs := []namedCollector{
		{"thermal", newStubCollector("test_describe_thermal")},
		{"power", newStubCollector("test_describe_power")},
	}
	c := makeTestRedfishCollector(context.Background(), "target", subs)

	ch := make(chan *prometheus.Desc, 10)
	c.Describe(ch)
	close(ch)

	var descs []*prometheus.Desc
	for d := range ch {
		descs = append(descs, d)
	}
	// 1 desc per sub-collector + 1 duration desc
	assert.Len(t, descs, 3)
}

func TestRedfishCollector_Collect_EmitsDurationPerSubCollector(t *testing.T) {
	subs := []namedCollector{
		{"thermal", newStubCollector("test_collect_duration_thermal")},
		{"power", newStubCollector("test_collect_duration_power")},
	}
	c := makeTestRedfishCollector(context.Background(), "test-target", subs)

	// 1 stub metric per sub-collector (2) + 1 duration metric per sub-collector (2) = 4
	assert.Equal(t, 4, testutil.CollectAndCount(c))
}

func TestRedfishCollector_Collect_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	releaseC := make(chan struct{})
	subs := []namedCollector{
		{"blocking", &stubCollector{
			desc:     prometheus.NewDesc("test_cancel_blocking_stub", "blocking stub", nil, nil),
			releaseC: releaseC,
		}},
	}
	c := makeTestRedfishCollector(ctx, "target", subs)

	done := make(chan struct{})
	go func() {
		defer close(done)
		ch := make(chan prometheus.Metric, 100)
		c.Collect(ch)
	}()

	cancel()

	select {
	case <-done:
		// Collect returned after context cancellation — correct
	case <-time.After(time.Second):
		t.Fatal("Collect did not return within 1s after context cancellation")
	}

	close(releaseC) // let the blocked sub-collector goroutine exit cleanly
}

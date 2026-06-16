package collectors

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stmcginnis/gofish"

	"github.com/e-breuninger/redfish-exporter/internal/config"
)

type namedCollector struct {
	name string
	c    prometheus.Collector
}

type RedfishCollector struct {
	target string
	done   <-chan struct{} // closed when the probe request context is done
	subs   []namedCollector

	durationDesc *prometheus.Desc
}

func NewRedfishCollector(ctx context.Context, target string, client *gofish.APIClient, logger *slog.Logger, cc config.CollectorsConfig) *RedfishCollector {
	var subs []namedCollector
	if cc.Thermal {
		subs = append(subs, namedCollector{"thermal", NewThermalCollector(target, client, logger)})
	}
	if cc.Power {
		subs = append(subs, namedCollector{"power", NewPowerCollector(target, client, logger)})
	}
	if cc.Systems {
		subs = append(subs, namedCollector{"systems", NewSystemsCollector(target, client, logger)})
	}
	if cc.Memory {
		subs = append(subs, namedCollector{"memory", NewMemoryCollector(target, client, logger)})
	}

	return &RedfishCollector{
		target: target,
		done:   ctx.Done(),
		subs:   subs,
		durationDesc: prometheus.NewDesc(
			"redfish_exporter_collector_duration_seconds",
			"Collector scrape duration",
			[]string{"target", "collector"},
			nil,
		),
	}
}

// Describe sends the super-set of all possible descriptors of metrics
// collected by this Collector.
func (c *RedfishCollector) Describe(ch chan<- *prometheus.Desc) {
	// Send the descriptors of each sub-collector.
	for _, sc := range c.subs {
		sc.c.Describe(ch)
	}
	ch <- c.durationDesc
}

// Collect is called by the Prometheus registry when collecting metrics.
func (c *RedfishCollector) Collect(ch chan<- prometheus.Metric) {
	// Use an intermediate channel so we can stop forwarding to ch when the
	// request context is canceled without racing with prometheus closing ch.
	inner := make(chan prometheus.Metric)

	go func() {
		defer close(inner)
		var wg sync.WaitGroup
		wg.Add(len(c.subs))

		// Collect metrics from each sub-collector in parallel.
		for _, sc := range c.subs {
			go func() {
				defer wg.Done()
				start := time.Now()
				sc.c.Collect(inner)
				// Record the duration of the scrape.
				inner <- prometheus.MustNewConstMetric(
					c.durationDesc,
					prometheus.GaugeValue,
					time.Since(start).Seconds(),
					c.target,
					sc.name,
				)
			}()
		}
		wg.Wait()
	}()

	// Forward metrics to ch until all sub-collectors finish or the request is canceled.
	for {
		select {
		case m, ok := <-inner:
			if !ok {
				return
			}
			ch <- m
		case <-c.done:
			// Drain inner in the background so sub-collector goroutines can exit cleanly.
			go func() {
				for range inner { //nolint:revive // intentional drain: unblocks sub-collector goroutines on cancel
				}
			}()
			return
		}
	}
}

package collectors

import (
	"log/slog"
	"sync"
	"time"

	"github.com/e-breuninger/redfish-exporter/internal/config"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stmcginnis/gofish"
)

type namedCollector struct {
	name string
	c    prometheus.Collector
}

type RedfishCollector struct {
	target string
	subs   []namedCollector

	durationDesc *prometheus.Desc
}

func NewRedfishCollector(target string, client *gofish.APIClient, logger *slog.Logger, cc config.CollectorsConfig) *RedfishCollector {
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
	var wg sync.WaitGroup
	wg.Add(len(c.subs))

	// Collect metrics from each sub-collector in parallel.
	for _, sc := range c.subs {
		go func() {
			defer wg.Done()
			start := time.Now()
			sc.c.Collect(ch)
			// Record the duration of the scrape.
			ch <- prometheus.MustNewConstMetric(
				c.durationDesc,
				prometheus.GaugeValue,
				time.Since(start).Seconds(),
				c.target,
				sc.name,
			)
		}()
	}
	wg.Wait()
}

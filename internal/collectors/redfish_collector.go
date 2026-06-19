package collectors

import (
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stmcginnis/gofish"

	"github.com/e-breuninger/redfish-exporter/internal/config"
)

// subCollector is the internal interface all per-subsystem collectors must implement.
// collect returns a non-nil error when the top-level Redfish resource fetch fails,
// allowing RedfishCollector to emit redfish_collector_success=0.
type subCollector interface {
	Describe(ch chan<- *prometheus.Desc)
	collect(ch chan<- prometheus.Metric) error
}

type namedCollector struct {
	name string
	c    subCollector
}

type RedfishCollector struct {
	target string
	subs   []namedCollector
	logger *slog.Logger

	durationDesc *prometheus.Desc
	successDesc  *prometheus.Desc
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
	if cc.Network {
		subs = append(subs, namedCollector{"network", NewNetworkCollector(target, client, logger)})
	}
	if cc.Managers {
		subs = append(subs, namedCollector{"managers", NewManagersCollector(target, client, logger)})
	}

	return &RedfishCollector{
		target: target,
		subs:   subs,
		logger: logger,
		durationDesc: prometheus.NewDesc(
			"redfish_exporter_collector_duration_seconds",
			"Collector scrape duration",
			[]string{"target", "collector"},
			nil,
		),
		successDesc: prometheus.NewDesc(
			"redfish_collector_success",
			"Whether the collector scraped without error (1=success, 0=failure)",
			[]string{"target", "collector"},
			nil,
		),
	}
}

// Describe sends the super-set of all possible descriptors of metrics
// collected by this Collector.
func (c *RedfishCollector) Describe(ch chan<- *prometheus.Desc) {
	for _, sc := range c.subs {
		sc.c.Describe(ch)
	}
	ch <- c.durationDesc
	ch <- c.successDesc
}

// Collect fans out to all sub-collectors in parallel and waits for all to
// finish before returning. Writing directly to ch is safe because the registry
// keeps ch open until Collect returns.
func (c *RedfishCollector) Collect(ch chan<- prometheus.Metric) {
	var wg sync.WaitGroup
	wg.Add(len(c.subs))
	for _, sc := range c.subs {
		go func() {
			defer wg.Done()
			start := time.Now()
			success := c.runCollector(sc, ch)
			ch <- prometheus.MustNewConstMetric(c.successDesc, prometheus.GaugeValue, success, c.target, sc.name)
			ch <- prometheus.MustNewConstMetric(c.durationDesc, prometheus.GaugeValue, time.Since(start).Seconds(), c.target, sc.name)
		}()
	}
	wg.Wait()
}

// runCollector invokes sc.collect with panic recovery.
// Returns 1.0 on success, 0.0 on error or panic.
func (c *RedfishCollector) runCollector(sc namedCollector, ch chan<- prometheus.Metric) (success float64) {
	success = 1.0
	c.logger.Debug("starting collector", "collector", sc.name, "target", c.target)
	defer func() {
		if r := recover(); r != nil {
			c.logger.Error("collector panicked", "collector", sc.name, "panic", fmt.Sprintf("%v", r), "target", c.target)
			success = 0.0
		}
		c.logger.Debug("collector finished", "collector", sc.name, "success", success == 1.0, "target", c.target)
	}()
	if err := sc.c.collect(ch); err != nil {
		c.logger.Debug("collector returned error", "collector", sc.name, "error", err, "target", c.target)
		success = 0.0
	}
	return success
}

package collectors

import (
	"fmt"
	"log/slog"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stmcginnis/gofish"
	"github.com/stmcginnis/gofish/schemas"
)

type MemoryCollector struct {
	target string
	client *gofish.APIClient
	logger *slog.Logger

	// descriptors
	memoryCapacityDesc *prometheus.Desc
	memoryStatusDesc   *prometheus.Desc
}

func NewMemoryCollector(target string, client *gofish.APIClient, logger *slog.Logger) *MemoryCollector {
	memoryBaseLabels := []string{"system_id", "memory_id", "memory_location"}

	return &MemoryCollector{
		target: target,
		client: client,
		logger: logger,

		memoryCapacityDesc: prometheus.NewDesc(
			prometheus.BuildFQName("redfish", "memory", "capacity_mib"),
			"Memory module capacity in MiB",
			memoryBaseLabels,
			prometheus.Labels{"target": target},
		),
		memoryStatusDesc: prometheus.NewDesc(
			prometheus.BuildFQName("redfish", "memory", "health"),
			"Memory module health (1=OK, 0=not OK)",
			append(memoryBaseLabels, "health"),
			prometheus.Labels{"target": target},
		),
	}
}

// Describe sends the super-set of all possible descriptors of metrics collected by this Collector.
func (c *MemoryCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.memoryCapacityDesc
	ch <- c.memoryStatusDesc
}

// Collect satisfies prometheus.Collector; use redfish_collector_success to detect failures.
// Collect satisfies prometheus.Collector; use redfish_collector_success to detect failures.
// When run standalone it creates its own scrape context.
func (c *MemoryCollector) Collect(ch chan<- prometheus.Metric) {
	_ = c.collect(newScrapeContext(c.client, c.logger, false), ch)
}

func (c *MemoryCollector) collect(_ *scrapeContext, ch chan<- prometheus.Metric) error {
	systems, err := c.client.Service.Systems()
	if err != nil {
		c.logger.Error("failed to get systems", "error", err, "target", c.target)
		return fmt.Errorf("get systems: %w", err)
	}

	for _, system := range systems {
		memory, err := system.Memory()
		if err != nil {
			c.logger.Error("failed to get memory for system", "error", err, "system_id", system.ID, "target", c.target)
			continue
		}

		for _, mem := range memory {
			if mem.Status.State == schemas.AbsentState || mem.Status.State == schemas.DisabledState {
				c.logger.Debug("skipping memory module", "memory_id", mem.ID, "state", mem.Status.State, "system", system.ID, "target", c.target)
				continue
			}

			// Deprecated, but the most widely populated location field.
			memLocation := mem.DeviceLocator //nolint:staticcheck // SA1019
			if memLocation == "" {
				memLocation = mem.ID
			}

			// Export capacity metric
			if mem.CapacityMiB != nil {
				ch <- prometheus.MustNewConstMetric(
					c.memoryCapacityDesc,
					prometheus.GaugeValue,
					float64(*mem.CapacityMiB),
					system.ID, mem.ID, memLocation,
				)
			} else {
				c.logger.Debug("skipping memory capacity: not reported", "memory_id", mem.ID, "system", system.ID, "target", c.target)
			}

			// Export health status metric
			if mem.Status.State == schemas.EnabledState {
				ch <- prometheus.MustNewConstMetric(
					c.memoryStatusDesc,
					prometheus.GaugeValue,
					healthToFloat(mem.Status.Health),
					system.ID, mem.ID, memLocation, string(mem.Status.Health),
				)
			}
		}
	}
	return nil
}

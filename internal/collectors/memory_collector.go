package collectors

import (
	"log/slog"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stmcginnis/gofish"
	"github.com/stmcginnis/gofish/common"
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

// Collect is called by the Prometheus registry when collecting metrics.
func (c *MemoryCollector) Collect(ch chan<- prometheus.Metric) {
	systems, err := c.client.Service.Systems()
	if err != nil {
		c.logger.Error("failed to get systems", "error", err, "target", c.target)
		return
	}

	for _, system := range systems {
		memory, err := system.Memory()
		if err != nil {
			c.logger.Error("failed to get memory for system", "error", err, "system_id", system.ID, "target", c.target)
			continue
		}

		for _, mem := range memory {
			// Skip absent or disabled memory modules
			if mem.Status.State == common.AbsentState || mem.Status.State == common.DisabledState {
				continue
			}

			// Determine memory location (socket/slot info)
			memLocation := mem.DeviceLocator
			if memLocation == "" {
				memLocation = mem.ID
			}

			// Export capacity metric
			ch <- prometheus.MustNewConstMetric(
				c.memoryCapacityDesc,
				prometheus.GaugeValue,
				float64(mem.CapacityMiB),
				system.ID, mem.ID, memLocation,
			)

			// Export health status metric
			if mem.Status.State == common.EnabledState {
				ch <- prometheus.MustNewConstMetric(
					c.memoryStatusDesc,
					prometheus.GaugeValue,
					healthToFloat(mem.Status.Health),
					system.ID, mem.ID, memLocation, string(mem.Status.Health),
				)
			}
		}
	}
}

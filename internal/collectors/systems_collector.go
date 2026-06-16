package collectors

import (
	"log/slog"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stmcginnis/gofish"
)

type SystemsCollector struct {
	target string
	client *gofish.APIClient
	logger *slog.Logger

	// descriptors
	systemInfoDesc            *prometheus.Desc
	systemHealthDesc          *prometheus.Desc
	systemHealthRollupDesc    *prometheus.Desc
	processorCountDesc        *prometheus.Desc
	processorHealthDesc       *prometheus.Desc
	processorHealthRollupDesc *prometheus.Desc
	memoryGiBDesc             *prometheus.Desc
	memoryHealthDesc          *prometheus.Desc
	memoryHealthRollupDesc    *prometheus.Desc
}

// NewSystemsCollector creates a new SystemsCollector instance.
func NewSystemsCollector(target string, client *gofish.APIClient, logger *slog.Logger) *SystemsCollector {
	systemBaseLabels := []string{"system_id"}

	return &SystemsCollector{
		target: target,
		client: client,
		logger: logger,
		systemInfoDesc: prometheus.NewDesc(
			prometheus.BuildFQName("redfish", "system", "info"),
			"System identity info metric (always 1)",
			append(systemBaseLabels, "serial_number", "manufacturer", "model"),
			prometheus.Labels{"target": target},
		),
		systemHealthDesc: prometheus.NewDesc(
			prometheus.BuildFQName("redfish", "system", "health"),
			"Health of the system resource itself (1=OK, 0=not OK)",
			append(systemBaseLabels, "health"),
			prometheus.Labels{"target": target},
		),
		systemHealthRollupDesc: prometheus.NewDesc(
			prometheus.BuildFQName("redfish", "system", "health_rollup"),
			"Overall combined health of the system including sub-components (1=OK, 0=not OK)",
			append(systemBaseLabels, "health"),
			prometheus.Labels{"target": target},
		),
		processorCountDesc: prometheus.NewDesc(
			prometheus.BuildFQName("redfish", "system", "processor_count"),
			"Number of physical processors in the system",
			systemBaseLabels,
			prometheus.Labels{"target": target},
		),
		processorHealthDesc: prometheus.NewDesc(
			prometheus.BuildFQName("redfish", "system", "processor_health"),
			"Health of the processor summary (1=OK, 0=not OK)",
			append(systemBaseLabels, "health"),
			prometheus.Labels{"target": target},
		),
		processorHealthRollupDesc: prometheus.NewDesc(
			prometheus.BuildFQName("redfish", "system", "processor_health_rollup"),
			"Rollup health of all processors including sub-components (1=OK, 0=not OK)",
			append(systemBaseLabels, "health"),
			prometheus.Labels{"target": target},
		),
		memoryGiBDesc: prometheus.NewDesc(
			prometheus.BuildFQName("redfish", "system", "memory_gib"),
			"Total system memory in gibibytes",
			systemBaseLabels,
			prometheus.Labels{"target": target},
		),
		memoryHealthDesc: prometheus.NewDesc(
			prometheus.BuildFQName("redfish", "system", "memory_health"),
			"Health of the memory summary (1=OK, 0=not OK)",
			append(systemBaseLabels, "health"),
			prometheus.Labels{"target": target},
		),
		memoryHealthRollupDesc: prometheus.NewDesc(
			prometheus.BuildFQName("redfish", "system", "memory_health_rollup"),
			"Rollup health of all memory including sub-components (1=OK, 0=not OK)",
			append(systemBaseLabels, "health"),
			prometheus.Labels{"target": target},
		),
	}
}

// Describe sends the super-set of all possible descriptors of metrics
// collected by this Collector.
func (c *SystemsCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.systemInfoDesc
	ch <- c.systemHealthDesc
	ch <- c.systemHealthRollupDesc
	ch <- c.processorCountDesc
	ch <- c.processorHealthDesc
	ch <- c.processorHealthRollupDesc
	ch <- c.memoryGiBDesc
	ch <- c.memoryHealthDesc
	ch <- c.memoryHealthRollupDesc
}

// Collect is called by the Prometheus registry when collecting metrics.
func (c *SystemsCollector) Collect(ch chan<- prometheus.Metric) {
	systems, err := c.client.Service.Systems()
	if err != nil {
		c.logger.Error("Failed to get Systems collection", "error", err)
		return
	}
	for _, system := range systems {
		ch <- prometheus.MustNewConstMetric(
			c.systemInfoDesc,
			prometheus.GaugeValue,
			1,
			system.ID, system.SerialNumber, system.Manufacturer, system.Model,
		)

		if system.Status.Health != "" {
			ch <- prometheus.MustNewConstMetric(
				c.systemHealthDesc,
				prometheus.GaugeValue,
				healthToFloat(system.Status.Health),
				system.ID, string(system.Status.Health),
			)
		}
		if system.Status.HealthRollup != "" {
			ch <- prometheus.MustNewConstMetric(
				c.systemHealthRollupDesc,
				prometheus.GaugeValue,
				healthToFloat(system.Status.HealthRollup),
				system.ID, string(system.Status.HealthRollup),
			)
		}

		ps := system.ProcessorSummary
		if ps.Count > 0 {
			ch <- prometheus.MustNewConstMetric(
				c.processorCountDesc,
				prometheus.GaugeValue,
				float64(ps.Count),
				system.ID,
			)
		}
		if ps.Status.Health != "" {
			ch <- prometheus.MustNewConstMetric(
				c.processorHealthDesc,
				prometheus.GaugeValue,
				healthToFloat(ps.Status.Health),
				system.ID, string(ps.Status.Health),
			)
		}
		if ps.Status.HealthRollup != "" {
			ch <- prometheus.MustNewConstMetric(
				c.processorHealthRollupDesc,
				prometheus.GaugeValue,
				healthToFloat(ps.Status.HealthRollup),
				system.ID, string(ps.Status.HealthRollup),
			)
		}

		ms := system.MemorySummary
		if ms.TotalSystemMemoryGiB > 0 {
			ch <- prometheus.MustNewConstMetric(
				c.memoryGiBDesc,
				prometheus.GaugeValue,
				float64(ms.TotalSystemMemoryGiB),
				system.ID,
			)
		}
		if ms.Status.Health != "" {
			ch <- prometheus.MustNewConstMetric(
				c.memoryHealthDesc,
				prometheus.GaugeValue,
				healthToFloat(ms.Status.Health),
				system.ID, string(ms.Status.Health),
			)
		}
		if ms.Status.HealthRollup != "" {
			ch <- prometheus.MustNewConstMetric(
				c.memoryHealthRollupDesc,
				prometheus.GaugeValue,
				healthToFloat(ms.Status.HealthRollup),
				system.ID, string(ms.Status.HealthRollup),
			)
		}
	}
}

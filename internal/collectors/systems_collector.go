package collectors

import (
	"log/slog"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stmcginnis/gofish"
	"github.com/stmcginnis/gofish/common"
)

type SystemsCollector struct {
	target string
	client *gofish.APIClient
	logger *slog.Logger

	// descriptors
	systemMetadataDesc           *prometheus.Desc
	systemStatusHealthRollupDesc *prometheus.Desc
}

// NewSystemsCollector creates a new SystemsCollector instance.
// It uses the System resource from Redfish to collect system-related metrics.
func NewSystemsCollector(target string, client *gofish.APIClient, logger *slog.Logger) *SystemsCollector {

	systemBaseLabels := []string{"system_id"}

	return &SystemsCollector{
		target: target,
		client: client,
		logger: logger,
		systemMetadataDesc: prometheus.NewDesc(
			prometheus.BuildFQName("redfish", "system", "metadata"),
			"Metadata information about the system (always 1)",
			append(systemBaseLabels, "serial_number", "manufacturer", "model"),
			prometheus.Labels{"target": target},
		),
		systemStatusHealthRollupDesc: prometheus.NewDesc(
			prometheus.BuildFQName("redfish", "system", "status_health_rollup"),
			"Overall combined health status of the system (1=OK, 0=Critical)",
			systemBaseLabels,
			prometheus.Labels{"target": target},
		),
	}
}

// Describe sends the super-set of all possible descriptors of metrics
// collected by this Collector.
func (c *SystemsCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.systemMetadataDesc
	ch <- c.systemStatusHealthRollupDesc
}

// Collect is called by the Prometheus registry when collecting metrics.
func (c *SystemsCollector) Collect(ch chan<- prometheus.Metric) {
	systems, err := c.client.Service.Systems()
	if err != nil {
		c.logger.Error("Failed to get Systems collection", "error", err)
		return
	}
	for _, system := range systems {

		// Set metadata metric
		ch <- prometheus.MustNewConstMetric(
			c.systemMetadataDesc,
			prometheus.GaugeValue,
			1,
			system.ID,
			system.SerialNumber,
			system.Manufacturer,
			system.Model,
		)

		// Set the overall health rollup metric
		healthValue := float64(0)
		if system.Status.HealthRollup == common.OKHealth {
			healthValue = 1
		}

		ch <- prometheus.MustNewConstMetric(
			c.systemStatusHealthRollupDesc,
			prometheus.GaugeValue,
			healthValue,
			system.ID,
		)
	}
}

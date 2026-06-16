package collectors

import (
	"log/slog"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stmcginnis/gofish"
	"github.com/stmcginnis/gofish/common"
)

type ThermalCollector struct {
	target string
	client *gofish.APIClient
	logger *slog.Logger

	// descriptors
	tempDesc       *prometheus.Desc
	tempStatusDesc *prometheus.Desc
	fanDesc        *prometheus.Desc
	fanStatusDesc  *prometheus.Desc
}

func NewThermalCollector(target string, client *gofish.APIClient, logger *slog.Logger) *ThermalCollector {

	thermalBaseLabels := []string{"chassis_id"}

	return &ThermalCollector{
		target: target,
		client: client,
		logger: logger,

		tempDesc: prometheus.NewDesc(
			prometheus.BuildFQName("redfish", "temperature", "celsius"),
			"Redfish thermal temperature readings in Celsius",
			append(thermalBaseLabels, "sensor", "physical_context"), prometheus.Labels{"target": target},
		),
		tempStatusDesc: prometheus.NewDesc(
			prometheus.BuildFQName("redfish", "temperature", "health"),
			"Redfish thermal temperature health (1=OK, 0=not OK)",
			append(thermalBaseLabels, "sensor", "physical_context", "health"), prometheus.Labels{"target": target},
		),
		fanDesc: prometheus.NewDesc(
			prometheus.BuildFQName("redfish", "fan", "rpm"),
			"Redfish thermal fan speed in RPM",
			append(thermalBaseLabels, "fan", "physical_context", "unit"), prometheus.Labels{"target": target},
		),
		fanStatusDesc: prometheus.NewDesc(
			prometheus.BuildFQName("redfish", "fan", "health"),
			"Redfish thermal fan health (1=OK, 0=not OK)",
			append(thermalBaseLabels, "fan", "physical_context", "health"), prometheus.Labels{"target": target},
		),
	}
}

// Describe sends the super-set of all possible descriptors of metrics collected by this Collector.
func (c *ThermalCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.tempDesc
	ch <- c.tempStatusDesc
	ch <- c.fanDesc
	ch <- c.fanStatusDesc
}

// Collect is called by the Prometheus registry when collecting metrics.
func (c *ThermalCollector) Collect(ch chan<- prometheus.Metric) {

	chassies, err := c.client.Service.Chassis()
	if err != nil || len(chassies) == 0 {
		c.logger.Error("failed to get chassis", "error", err, "target", c.target)
		return
	}
	for _, chassis := range chassies {
		thermal, err := chassis.Thermal()
		if err != nil {
			c.logger.Error("failed to get thermal data", "error", err, "chassis", chassis.ID, "target", c.target)
			continue
		}
		if thermal == nil {
			continue
		}
		// Export temperature readings and status
		for _, t := range thermal.Temperatures {
			// Skip disabled or absent sensors
			if t.Status.State == common.DisabledState || t.Status.State == common.AbsentState {
				continue
			}
			if t.ReadingCelsius == nil {
				continue
			}
			physicalContext := ""
			if t.PhysicalContext != nil {
				physicalContext = string(*t.PhysicalContext)
			}
			ch <- prometheus.MustNewConstMetric(
				c.tempDesc,
				prometheus.GaugeValue,
				float64(*t.ReadingCelsius),
				chassis.ID, t.Name, physicalContext,
			)

			// Export temperature status as a one-hot metric (1=OK, 0=not OK)
			if t.Status.State == common.EnabledState {
				ch <- prometheus.MustNewConstMetric(
					c.tempStatusDesc,
					prometheus.GaugeValue,
					healthToFloat(t.Status.Health),
					chassis.ID, t.Name, physicalContext, string(t.Status.Health),
				)
			}
		}

		// Export fan status metrics
		for _, f := range thermal.Fans {
			if f.Status.State == common.AbsentState {
				// Skip absent fans
				continue
			}
			// Some fans may not have a name (looking at you, HPE) or OEM names
			// we skip those to avoid metric label issues and zero values
			if f.Name == "" {
				continue
			}
			physicalContext := ""
			if f.PhysicalContext != nil {
				physicalContext = string(*f.PhysicalContext)
			}
			// RPM reading is optional; emit it only when available
			if f.Reading != nil {
				ch <- prometheus.MustNewConstMetric(
					c.fanDesc,
					prometheus.GaugeValue,
					float64(*f.Reading),
					chassis.ID, f.Name, physicalContext, string(f.ReadingUnits),
				)
			}
			// Health is independent of reading availability — a fan can be Critical with no RPM data
			if f.Status.State == common.EnabledState {
				ch <- prometheus.MustNewConstMetric(
					c.fanStatusDesc,
					prometheus.GaugeValue,
					healthToFloat(f.Status.Health),
					chassis.ID, f.Name, physicalContext, string(f.Status.Health),
				)
			}
		}
	}
}

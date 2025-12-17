package collectors

import (
	"log/slog"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stmcginnis/gofish"
	"github.com/stmcginnis/gofish/common"
)

// Todo: Add chassis label to baseLabels
// Todo: move target to constant labels?
// Todo: Handle ThermalSubsystem vs. legacy Thermal resource - some systems may have both
type ThermalCollector struct {
	target string
	client *gofish.APIClient
	logger *slog.Logger

	// descriptors
	ThermalSubsystemStatusDesc *prometheus.Desc

	tempDesc       *prometheus.Desc
	tempStatusDesc *prometheus.Desc
	fanDesc        *prometheus.Desc
	fanStatusDesc  *prometheus.Desc
}

func NewThermalCollector(target string, client *gofish.APIClient, logger *slog.Logger, baseLabels []string) *ThermalCollector {

	return &ThermalCollector{
		target: target,
		client: client,
		logger: logger,

		ThermalSubsystemStatusDesc: prometheus.NewDesc(
			"redfish_thermal_subsystem_status",
			"The overall status of the thermal subsystem",
			append(baseLabels, "chassis", "health"), nil, // todo: move target to constant labels?
		),
		tempDesc: prometheus.NewDesc(
			"redfish_temperature_celsius",
			"Redfish thermal temperature readings in Celsius",
			append(baseLabels, "sensor", "physical_context"), nil,
		),
		tempStatusDesc: prometheus.NewDesc(
			"redfish_temperature_status",
			"Redfish thermal temperature status health report (1=OK, 0=not OK)",
			append(baseLabels, "sensor", "physical_context", "health"), nil,
		),
		fanDesc: prometheus.NewDesc(
			"redfish_fan_rpm",
			"Redfish thermal fan speed in RPM",
			append(baseLabels, "fan", "physical_context"), nil,
		),
		fanStatusDesc: prometheus.NewDesc(
			"redfish_fan_status",
			"Redfish thermal fan status health report (1=OK, 0=not OK)",
			append(baseLabels, "fan", "physical_context", "health"), nil,
		),
	}
}

// Describe sends the super-set of all possible descriptors of metrics collected by this Collector.
func (c *ThermalCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.ThermalSubsystemStatusDesc
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
	// Read thermal data from all chassis
	// Some systems may have multiple chassis (e.g., blade enclosures)
	for _, chassis := range chassies {

		// Check for thermal subsystem presence
		thewrmalSubsystem, err := chassis.ThermalSubsystem()
		if err != nil || thewrmalSubsystem == nil {
			c.logger.Debug("thermal subsystem not present", "chassis", chassis.ID, "target", c.target)
		} else {
			ch <- prometheus.MustNewConstMetric(
				c.ThermalSubsystemStatusDesc,
				prometheus.GaugeValue,
				1.0, // thermal subsystem present
				[]string{c.target, chassis.ID, string(thewrmalSubsystem.Status.Health)}...,
			)
		}

		thermal, err := chassis.Thermal()
		if err != nil {
			c.logger.Debug("failed to get thermal data", "error", err, "chassis", chassis.ID, "target", c.target)
			continue
		}
		c.logger.Debug("got thermal data", "temp_count", len(thermal.Temperatures), "fan_count", len(thermal.Fans), "chassis", chassis.ID, "target", c.target)

		// Export temperature readings and status
		for _, t := range thermal.Temperatures {
			ch <- prometheus.MustNewConstMetric(
				c.tempDesc,
				prometheus.GaugeValue,
				float64(t.ReadingCelsius),
				[]string{c.target, t.Name, string(t.PhysicalContext)}...,
			)

			// Export temperature status as a one-hot metric (1=OK, 0=not OK)
			if t.Status.State == common.EnabledState {
				c.logger.Debug("exporting temperature status", "sensor", t.Name, "health", t.Status.Health, "target", c.target)
				statusValue := 0.0 // all non -OK statuses
				if t.Status.Health == common.OKHealth {
					statusValue = 1.0
				}
				ch <- prometheus.MustNewConstMetric(
					c.tempStatusDesc,
					prometheus.GaugeValue,
					statusValue,
					[]string{c.target, t.Name, string(t.PhysicalContext), string(t.Status.Health)}...,
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
			// we skip those to avoid metric label issues and zero values on rpm
			if f.Name == "" {
				continue
			}
			ch <- prometheus.MustNewConstMetric(
				c.fanDesc,
				prometheus.GaugeValue,
				float64(f.Reading),
				[]string{c.target, f.Name, string(f.PhysicalContext)}...,
			)
			if f.Status.State == common.EnabledState {
				c.logger.Debug("exporting fan status", "fan", f.Name, "health", f.Status.Health, "target", c.target)
				statusValue := 0.0 // all non -OK statuses
				if f.Status.Health == common.OKHealth {
					statusValue = 1.0
				}
				ch <- prometheus.MustNewConstMetric(
					c.fanStatusDesc,
					prometheus.GaugeValue,
					statusValue,
					[]string{c.target, f.Name, string(f.PhysicalContext), string(f.Status.Health)}...,
				)
			}
		}

	}
}

package collectors

import (
	"fmt"
	"log/slog"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stmcginnis/gofish"
	"github.com/stmcginnis/gofish/redfish"
)

type PowerCollector struct {
	target string
	client *gofish.APIClient
	logger *slog.Logger

	// descriptors
	powerStateDesc            *prometheus.Desc
	chassisHealthDesc         *prometheus.Desc
	chassisHealthRollupDesc   *prometheus.Desc
	powerCapacityDesc         *prometheus.Desc
	powerConsumedDesc         *prometheus.Desc
	powerAverageConsumedDesc  *prometheus.Desc
	powerMaxConsumedDesc      *prometheus.Desc
	powerMinConsumedDesc      *prometheus.Desc
	powerControlHealthDesc    *prometheus.Desc
	powerRedundancyHealthDesc *prometheus.Desc
	powerSuppliesHealthDesc   *prometheus.Desc
}

// NewPowerCollector creates a new PowerCollector instance.
// It uses the Power resource from Redfish to collect power-related metrics.
// Note: PowerSubsystem is not implemented yet.
func NewPowerCollector(target string, client *gofish.APIClient, logger *slog.Logger) *PowerCollector {

	powerBaseLabels := []string{"chassis_id"}

	return &PowerCollector{
		target: target,
		client: client,
		logger: logger,
		powerStateDesc: prometheus.NewDesc(
			prometheus.BuildFQName("redfish", "power", "state"),
			"Power state of the overall chassis (1=On, 0=Off)",
			powerBaseLabels,
			prometheus.Labels{"target": target},
		),
		chassisHealthDesc: prometheus.NewDesc(
			prometheus.BuildFQName("redfish", "chassis", "health"),
			"Health of the chassis (1=OK, 0=not OK)",
			append(powerBaseLabels, "health"),
			prometheus.Labels{"target": target},
		),
		chassisHealthRollupDesc: prometheus.NewDesc(
			prometheus.BuildFQName("redfish", "chassis", "health_rollup"),
			"Rollup health of the chassis including all subordinate resources (1=OK, 0=not OK)",
			append(powerBaseLabels, "health"),
			prometheus.Labels{"target": target},
		),
		// PowerCapacityWatts
		powerCapacityDesc: prometheus.NewDesc(
			prometheus.BuildFQName("redfish", "power", "capacity_watts"),
			"Power capacity overall of the chassis in Watts",
			append(powerBaseLabels, "name", "member_id"),
			prometheus.Labels{"target": target},
		),
		powerConsumedDesc: prometheus.NewDesc(
			prometheus.BuildFQName("redfish", "power", "consumed_watts"),
			"Power consumed currently of the chassis in Watts",
			append(powerBaseLabels, "name", "member_id"),
			prometheus.Labels{"target": target},
		),
		// PowerAverageConsumedWatts
		powerAverageConsumedDesc: prometheus.NewDesc(
			prometheus.BuildFQName("redfish", "power", "average_consumed_watts"),
			"Power average consumed of the chassis in Watts",
			append(powerBaseLabels, "name", "member_id"),
			prometheus.Labels{"target": target},
		),
		// PowerMaxConsumedWatts
		powerMaxConsumedDesc: prometheus.NewDesc(
			prometheus.BuildFQName("redfish", "power", "max_consumed_watts"),
			"Power max consumed of the chassis in Watts",
			append(powerBaseLabels, "name", "member_id"),
			prometheus.Labels{"target": target},
		),
		// PowerMinConsumedWatts
		powerMinConsumedDesc: prometheus.NewDesc(
			prometheus.BuildFQName("redfish", "power", "min_consumed_watts"),
			"Power min consumed of the chassis in Watts",
			append(powerBaseLabels, "name", "member_id"),
			prometheus.Labels{"target": target},
		),
		powerControlHealthDesc: prometheus.NewDesc(
			prometheus.BuildFQName("redfish", "power", "control_health"),
			"Health of a power control domain (1=OK, 0=not OK)",
			append(powerBaseLabels, "name", "member_id", "health"),
			prometheus.Labels{"target": target},
		),
		powerRedundancyHealthDesc: prometheus.NewDesc(
			prometheus.BuildFQName("redfish", "power", "redundancy_health"),
			"Health status of the power redundancy subsystem (1=OK, 0=not OK)",
			append(powerBaseLabels, "name", "member_id", "health"),
			prometheus.Labels{"target": target},
		),
		// PowerSuppliesHealth
		powerSuppliesHealthDesc: prometheus.NewDesc(
			prometheus.BuildFQName("redfish", "power", "supply_health"),
			"Health status of a power supply (1=OK, 0=not OK)",
			append(powerBaseLabels, "name", "member_id", "health"),
			prometheus.Labels{"target": target},
		),
	}
}

// Describe sends the super-set of all possible descriptors of metrics collected by this Collector.
func (c *PowerCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.powerStateDesc
	ch <- c.chassisHealthDesc
	ch <- c.chassisHealthRollupDesc
	ch <- c.powerCapacityDesc
	ch <- c.powerConsumedDesc
	ch <- c.powerAverageConsumedDesc
	ch <- c.powerMaxConsumedDesc
	ch <- c.powerMinConsumedDesc
	ch <- c.powerControlHealthDesc
	ch <- c.powerRedundancyHealthDesc
	ch <- c.powerSuppliesHealthDesc
}

// Collect satisfies prometheus.Collector; use redfish_collector_success to detect failures.
func (c *PowerCollector) Collect(ch chan<- prometheus.Metric) { _ = c.collect(ch) }

func (c *PowerCollector) collect(ch chan<- prometheus.Metric) error {
	chassies, err := c.client.Service.Chassis()
	if err != nil {
		c.logger.Error("failed to get chassis", "error", err, "target", c.target)
		return fmt.Errorf("get chassis: %w", err)
	}
	for _, chassis := range chassies {
		powerState := 0.0
		if chassis.PowerState == redfish.OnPowerState {
			powerState = 1.0
		}
		ch <- prometheus.MustNewConstMetric(
			c.powerStateDesc,
			prometheus.GaugeValue,
			powerState,
			chassis.ID,
		)
		if chassis.Status.Health != "" {
			ch <- prometheus.MustNewConstMetric(
				c.chassisHealthDesc,
				prometheus.GaugeValue,
				healthToFloat(chassis.Status.Health),
				chassis.ID, string(chassis.Status.Health),
			)
		}
		if chassis.Status.HealthRollup != "" {
			ch <- prometheus.MustNewConstMetric(
				c.chassisHealthRollupDesc,
				prometheus.GaugeValue,
				healthToFloat(chassis.Status.HealthRollup),
				chassis.ID, string(chassis.Status.HealthRollup),
			)
		}

		power, err := chassis.Power()
		if err != nil || power == nil {
			c.logger.Error("failed to get power subsystem", "chassis_id", chassis.ID, "error", err, "target", c.target)
			continue
		}

		for _, pc := range power.PowerControl {
			if pc.Status.Health != "" {
				ch <- prometheus.MustNewConstMetric(
					c.powerControlHealthDesc,
					prometheus.GaugeValue,
					healthToFloat(pc.Status.Health),
					chassis.ID, pc.Name, pc.MemberID, string(pc.Status.Health),
				)
			}
			if pc.PowerCapacityWatts != nil {
				ch <- prometheus.MustNewConstMetric(
					c.powerCapacityDesc,
					prometheus.GaugeValue,
					float64(*pc.PowerCapacityWatts),
					chassis.ID, pc.Name, pc.MemberID,
				)
			}
			if pc.PowerConsumedWatts != nil {
				ch <- prometheus.MustNewConstMetric(
					c.powerConsumedDesc,
					prometheus.GaugeValue,
					float64(*pc.PowerConsumedWatts),
					chassis.ID, pc.Name, pc.MemberID,
				)
			}
			if pc.PowerMetrics != nil {
				if pc.PowerMetrics.AverageConsumedWatts != nil {
					ch <- prometheus.MustNewConstMetric(
						c.powerAverageConsumedDesc,
						prometheus.GaugeValue,
						float64(*pc.PowerMetrics.AverageConsumedWatts),
						chassis.ID, pc.Name, pc.MemberID,
					)
				}
				if pc.PowerMetrics.MaxConsumedWatts != nil {
					ch <- prometheus.MustNewConstMetric(
						c.powerMaxConsumedDesc,
						prometheus.GaugeValue,
						float64(*pc.PowerMetrics.MaxConsumedWatts),
						chassis.ID, pc.Name, pc.MemberID,
					)
				}
				if pc.PowerMetrics.MinConsumedWatts != nil {
					ch <- prometheus.MustNewConstMetric(
						c.powerMinConsumedDesc,
						prometheus.GaugeValue,
						float64(*pc.PowerMetrics.MinConsumedWatts),
						chassis.ID, pc.Name, pc.MemberID,
					)
				}
			}
		}

		for _, redundancy := range power.Redundancy {
			if redundancy.Name == "" {
				c.logger.Debug("skipping unnamed redundancy entry", "chassis", chassis.ID, "target", c.target)
				continue
			}
			ch <- prometheus.MustNewConstMetric(
				c.powerRedundancyHealthDesc,
				prometheus.GaugeValue,
				healthToFloat(redundancy.Status.Health),
				chassis.ID, redundancy.Name, redundancy.MemberID, string(redundancy.Status.Health),
			)
		}

		for _, ps := range power.PowerSupplies {
			if ps.Name == "" {
				c.logger.Debug("skipping unnamed power supply", "chassis", chassis.ID, "target", c.target)
				continue
			}
			ch <- prometheus.MustNewConstMetric(
				c.powerSuppliesHealthDesc,
				prometheus.GaugeValue,
				healthToFloat(ps.Status.Health),
				chassis.ID, ps.Name, ps.MemberID, string(ps.Status.Health),
			)
		}

	}
	return nil
}

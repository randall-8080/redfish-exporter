package collectors

import (
	"log/slog"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stmcginnis/gofish"
	"github.com/stmcginnis/gofish/common"
	"github.com/stmcginnis/gofish/redfish"
)

type PowerCollector struct {
	target string
	client *gofish.APIClient
	logger *slog.Logger

	// descriptors
	powerStateDesc            *prometheus.Desc
	powerCapacityDesc         *prometheus.Desc
	powerConsumedDesc         *prometheus.Desc
	powerAverageConsumedDesc  *prometheus.Desc
	powerMaxConsumedDesc      *prometheus.Desc
	powerMinConsumedDesc      *prometheus.Desc
	PowerredundancyHealthDesc *prometheus.Desc
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
		// PowerCapacityWatts
		powerCapacityDesc: prometheus.NewDesc(
			prometheus.BuildFQName("redfish", "power", "capacity_watts"),
			"Power capacity overall of the chassis in Watts",
			append(powerBaseLabels, "name"),
			prometheus.Labels{"target": target},
		),
		// PowerAllocatedWatts
		powerConsumedDesc: prometheus.NewDesc(
			prometheus.BuildFQName("redfish", "power", "consumed_watts"),
			"Power consumed currently of the chassis in Watts",
			append(powerBaseLabels, "name"),
			prometheus.Labels{"target": target},
		),
		// PowerAverageConsumedWatts
		powerAverageConsumedDesc: prometheus.NewDesc(
			prometheus.BuildFQName("redfish", "power_metrics", "average_consumed_watts"),
			"Power average consumed of the chassis in Watts",
			append(powerBaseLabels, "name"),
			prometheus.Labels{"target": target},
		),
		// PowerMaxConsumedWatts
		powerMaxConsumedDesc: prometheus.NewDesc(
			prometheus.BuildFQName("redfish", "power_metrics", "max_consumed_watts"),
			"Power max consumed of the chassis in Watts",
			append(powerBaseLabels, "name"),
			prometheus.Labels{"target": target},
		),
		// PowerMinConsumedWatts
		powerMinConsumedDesc: prometheus.NewDesc(
			prometheus.BuildFQName("redfish", "power_metrics", "min_consumed_watts"),
			"Power min consumed of the chassis in Watts",
			append(powerBaseLabels, "name"),
			prometheus.Labels{"target": target},
		),
		// PowerredundancyHealth
		PowerredundancyHealthDesc: prometheus.NewDesc(
			prometheus.BuildFQName("redfish", "power_redundancy", "health"),
			"Health status of the power redundancy subsystem",
			append(powerBaseLabels, "name"),
			prometheus.Labels{"target": target},
		),
		// PowerSuppliesHealth
		powerSuppliesHealthDesc: prometheus.NewDesc(
			prometheus.BuildFQName("redfish", "power_supplies", "health"),
			"Health status of the power suppliy device",
			append(powerBaseLabels, "name"),
			prometheus.Labels{"target": target},
		),
	}
}

// Describe sends the super-set of all possible descriptors of metrics collected by this Collector.
func (c *PowerCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.powerStateDesc
	ch <- c.powerCapacityDesc
	ch <- c.powerConsumedDesc
	ch <- c.powerAverageConsumedDesc
	ch <- c.powerMaxConsumedDesc
	ch <- c.powerMinConsumedDesc
	ch <- c.PowerredundancyHealthDesc
	ch <- c.powerSuppliesHealthDesc
}

// Collect is called by the Prometheus registry when collecting metrics.
func (c *PowerCollector) Collect(ch chan<- prometheus.Metric) {

	chassies, err := c.client.Service.Chassis()
	if err != nil || len(chassies) == 0 {
		c.logger.Error("failed to get chassis", "error", err, "target", c.target)
		return
	}
	for _, chassis := range chassies {
		// Set the overall power capacity metric
		ch <- prometheus.MustNewConstMetric(
			c.powerStateDesc,
			prometheus.GaugeValue,
			func() float64 {
				if chassis.PowerState == redfish.OnPowerState {
					return 1.0
				}
				return 0.0
			}(),
			chassis.ID,
		)

		power, err := chassis.Power()
		if err != nil || power == nil {
			c.logger.Error("failed to get power subsystem", "chassis_id", chassis.ID, "error", err, "target", c.target)
			continue
		}

		for _, pc := range power.PowerControl {
			// Set the power capacity metric
			ch <- prometheus.MustNewConstMetric(
				c.powerCapacityDesc,
				prometheus.GaugeValue,
				float64(pc.PowerCapacityWatts),
				chassis.ID, pc.Name,
			)
			// Set the power consumed metric
			ch <- prometheus.MustNewConstMetric(
				c.powerConsumedDesc,
				prometheus.GaugeValue,
				float64(pc.PowerConsumedWatts),
				chassis.ID, pc.Name,
			)
			// Set the power average consumed metric
			ch <- prometheus.MustNewConstMetric(
				c.powerAverageConsumedDesc,
				prometheus.GaugeValue,
				float64(pc.PowerMetrics.AverageConsumedWatts),
				chassis.ID, pc.Name,
			)
			// Set the power max consumed metric
			ch <- prometheus.MustNewConstMetric(
				c.powerMaxConsumedDesc,
				prometheus.GaugeValue,
				float64(pc.PowerMetrics.MaxConsumedWatts),
				chassis.ID, pc.Name,
			)
			// Set the power min consumed metric
			ch <- prometheus.MustNewConstMetric(
				c.powerMinConsumedDesc,
				prometheus.GaugeValue,
				float64(pc.PowerMetrics.MinConsumedWatts),
				chassis.ID, pc.Name,
			)
			// Set the power redundancy health metric
			for _, redundancy := range power.Redundancy {

				// Todo: Is there a better way to convert Health to a numeric value?
				healthValue := 0.0
				switch redundancy.Status.Health {
				case common.OKHealth:
					healthValue = 1.0
				case common.WarningHealth:
					healthValue = 0.5
				case common.CriticalHealth:
					healthValue = 0.0
				}
				ch <- prometheus.MustNewConstMetric(
					c.PowerredundancyHealthDesc,
					prometheus.GaugeValue,
					healthValue,
					chassis.ID, redundancy.Name,
				)
			}

			// Set the power supplies health metric
			for _, ps := range power.PowerSupplies {
				// Todo: Is there a better way to convert Health to a numeric value?
				healthValue := 0.0
				switch ps.Status.Health {
				case common.OKHealth:
					healthValue = 1.0
				case common.WarningHealth:
					healthValue = 0.5
				case common.CriticalHealth:
					healthValue = 0.0
				}
				ch <- prometheus.MustNewConstMetric(
					c.powerSuppliesHealthDesc,
					prometheus.GaugeValue,
					healthValue,
					chassis.ID, ps.Name,
				)
			}
		}

	}

}

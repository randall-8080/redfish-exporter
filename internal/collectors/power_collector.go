package collectors

import (
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
	powerStateDesc *prometheus.Desc
}

func NewPowerCollector(target string, client *gofish.APIClient, logger *slog.Logger) *PowerCollector {

	powerBaseLabels := []string{"chassis_id"}

	return &PowerCollector{
		target: target,
		client: client,
		logger: logger,
		powerStateDesc: prometheus.NewDesc(
			prometheus.BuildFQName("redfish", "power", "state"),
			"Power state of the system (1=On, 0=Off)",
			powerBaseLabels,
			prometheus.Labels{"target": target},
		),
	}
}

// Describe sends the super-set of all possible descriptors of metrics collected by this Collector.
func (c *PowerCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.powerStateDesc
}

// Collect is called by the Prometheus registry when collecting metrics.
func (c *PowerCollector) Collect(ch chan<- prometheus.Metric) {

	chassies, err := c.client.Service.Chassis()
	if err != nil || len(chassies) == 0 {
		c.logger.Error("failed to get chassis", "error", err, "target", c.target)
		return
	}
	for _, chassis := range chassies {
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
	}

}

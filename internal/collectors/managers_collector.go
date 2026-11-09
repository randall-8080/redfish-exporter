package collectors

import (
	"fmt"
	"log/slog"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stmcginnis/gofish"
)

type ManagersCollector struct {
	target string
	client *gofish.APIClient
	logger *slog.Logger

	managerInfoDesc   *prometheus.Desc
	managerHealthDesc *prometheus.Desc
}

func NewManagersCollector(target string, client *gofish.APIClient, logger *slog.Logger) *ManagersCollector {
	managerBaseLabels := []string{"manager_id"}

	return &ManagersCollector{
		target: target,
		client: client,
		logger: logger,
		managerInfoDesc: prometheus.NewDesc(
			prometheus.BuildFQName("redfish", "manager", "info"),
			"Manager (BMC) identity and firmware info (always 1)",
			append(managerBaseLabels, "manager_type", "model", "firmware_version", "manufacturer"),
			prometheus.Labels{"target": target},
		),
		managerHealthDesc: prometheus.NewDesc(
			prometheus.BuildFQName("redfish", "manager", "health"),
			"Manager (BMC) health (1=OK, 0=not OK)",
			append(managerBaseLabels, "health"),
			prometheus.Labels{"target": target},
		),
	}
}

func (c *ManagersCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.managerInfoDesc
	ch <- c.managerHealthDesc
}

// Collect satisfies prometheus.Collector; use redfish_collector_success to detect failures.
// When run standalone it creates its own scrape context.
func (c *ManagersCollector) Collect(ch chan<- prometheus.Metric) {
	_ = c.collect(newScrapeContext(c.client, c.logger, false), ch)
}

func (c *ManagersCollector) collect(_ *scrapeContext, ch chan<- prometheus.Metric) error {
	managers, err := c.client.Service.Managers()
	if err != nil {
		c.logger.Error("failed to get managers", "error", err, "target", c.target)
		return fmt.Errorf("get managers: %w", err)
	}
	for _, manager := range managers {
		ch <- prometheus.MustNewConstMetric(
			c.managerInfoDesc,
			prometheus.GaugeValue,
			1,
			manager.ID, string(manager.ManagerType), manager.Model, manager.FirmwareVersion, manager.Manufacturer,
		)
		if manager.Status.Health != "" {
			ch <- prometheus.MustNewConstMetric(
				c.managerHealthDesc,
				prometheus.GaugeValue,
				healthToFloat(manager.Status.Health),
				manager.ID, string(manager.Status.Health),
			)
		} else {
			c.logger.Debug("skipping manager health: no health status", "manager", manager.ID, "target", c.target)
		}
	}
	return nil
}

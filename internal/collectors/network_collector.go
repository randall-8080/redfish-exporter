package collectors

import (
	"log/slog"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stmcginnis/gofish"
	"github.com/stmcginnis/gofish/common"
	"github.com/stmcginnis/gofish/redfish"
)

type NetworkCollector struct {
	target string
	client *gofish.APIClient
	logger *slog.Logger

	// descriptors — EthernetInterface (per System NIC)
	ifLinkUpDesc *prometheus.Desc
	ifSpeedDesc  *prometheus.Desc
	ifHealthDesc *prometheus.Desc

	// descriptors — NetworkAdapter (per Chassis physical adapter)
	adapterHealthDesc *prometheus.Desc
}

func NewNetworkCollector(target string, client *gofish.APIClient, logger *slog.Logger) *NetworkCollector {
	ifLabels := []string{"system_id", "interface_id", "mac_address"}
	adapterLabels := []string{"chassis_id", "adapter_id", "manufacturer", "model"}

	return &NetworkCollector{
		target: target,
		client: client,
		logger: logger,

		ifLinkUpDesc: prometheus.NewDesc(
			prometheus.BuildFQName("redfish", "network_interface", "link_up"),
			"Whether the network interface link is up (1=LinkUp, 0=other)",
			ifLabels,
			prometheus.Labels{"target": target},
		),
		ifSpeedDesc: prometheus.NewDesc(
			prometheus.BuildFQName("redfish", "network_interface", "speed_mbps"),
			"Current link speed of the network interface in Mbps",
			ifLabels,
			prometheus.Labels{"target": target},
		),
		ifHealthDesc: prometheus.NewDesc(
			prometheus.BuildFQName("redfish", "network_interface", "health"),
			"Health of the network interface (1=OK, 0=not OK)",
			append(ifLabels, "health"),
			prometheus.Labels{"target": target},
		),
		adapterHealthDesc: prometheus.NewDesc(
			prometheus.BuildFQName("redfish", "network_adapter", "health"),
			"Health of a physical network adapter (1=OK, 0=not OK)",
			append(adapterLabels, "health"),
			prometheus.Labels{"target": target},
		),
	}
}

func (c *NetworkCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.ifLinkUpDesc
	ch <- c.ifSpeedDesc
	ch <- c.ifHealthDesc
	ch <- c.adapterHealthDesc
}

func (c *NetworkCollector) Collect(ch chan<- prometheus.Metric) {
	c.collectEthernetInterfaces(ch)
	c.collectNetworkAdapters(ch)
}

func (c *NetworkCollector) collectEthernetInterfaces(ch chan<- prometheus.Metric) {
	systems, err := c.client.Service.Systems()
	if err != nil {
		c.logger.Error("failed to get systems", "error", err, "target", c.target)
		return
	}
	for _, system := range systems {
		nics, err := system.EthernetInterfaces()
		if err != nil {
			c.logger.Error("failed to get ethernet interfaces", "error", err, "system", system.ID, "target", c.target)
			continue
		}
		for _, nic := range nics {
			if nic.Status.State == common.AbsentState {
				continue
			}
			linkUp := 0.0
			if nic.LinkStatus == redfish.LinkUpLinkStatus {
				linkUp = 1.0
			}
			ch <- prometheus.MustNewConstMetric(
				c.ifLinkUpDesc,
				prometheus.GaugeValue,
				linkUp,
				system.ID, nic.ID, nic.MACAddress,
			)
			if nic.SpeedMbps > 0 {
				ch <- prometheus.MustNewConstMetric(
					c.ifSpeedDesc,
					prometheus.GaugeValue,
					float64(nic.SpeedMbps),
					system.ID, nic.ID, nic.MACAddress,
				)
			}
			if nic.Status.State == common.EnabledState && nic.Status.Health != "" {
				ch <- prometheus.MustNewConstMetric(
					c.ifHealthDesc,
					prometheus.GaugeValue,
					healthToFloat(nic.Status.Health),
					system.ID, nic.ID, nic.MACAddress, string(nic.Status.Health),
				)
			}
		}
	}
}

func (c *NetworkCollector) collectNetworkAdapters(ch chan<- prometheus.Metric) {
	chassis, err := c.client.Service.Chassis()
	if err != nil {
		c.logger.Error("failed to get chassis", "error", err, "target", c.target)
		return
	}
	for _, chassis := range chassis {
		adapters, err := chassis.NetworkAdapters()
		if err != nil {
			c.logger.Error("failed to get network adapters", "error", err, "chassis", chassis.ID, "target", c.target)
			continue
		}
		for _, adapter := range adapters {
			if adapter.Status.Health == "" {
				continue
			}
			ch <- prometheus.MustNewConstMetric(
				c.adapterHealthDesc,
				prometheus.GaugeValue,
				healthToFloat(adapter.Status.Health),
				chassis.ID, adapter.ID, adapter.Manufacturer, adapter.Model, string(adapter.Status.Health),
			)
		}
	}
}

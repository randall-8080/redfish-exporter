//go:build integration

package collectors

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/e-breuninger/redfish-exporter/internal/client"
	"github.com/e-breuninger/redfish-exporter/internal/config"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

func startMockupContainer(t *testing.T) (target string) {
	t.Helper()
	ctx := context.Background()
	req := testcontainers.ContainerRequest{
		Image:        "dmtf/redfish-mockup-server:1.2.9",
		ExposedPorts: []string{"8000/tcp"},
		WaitingFor:   wait.ForHTTP("/redfish/v1/").WithPort("8000/tcp").WithStartupTimeout(30 * time.Second),
	}
	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	require.NoError(t, err)
	t.Cleanup(func() { container.Terminate(ctx) })

	host, err := container.Host(ctx)
	require.NoError(t, err)
	port, err := container.MappedPort(ctx, "8000")
	require.NoError(t, err)
	return "http://" + host + ":" + port.Port()
}

func TestRedfishCollectorIntegration(t *testing.T) {
	target := startMockupContainer(t)

	cc := config.CollectorsConfig{
		Thermal: true,
		Power:   true,
		Systems: true,
		Memory:  true,
		Network: true,
	}
	redfishClient, err := client.NewClientFromModuleConfig(config.Module{Collectors: cc}, target)
	require.NoError(t, err, "failed to create Redfish client")

	collector := NewRedfishCollector(target, redfishClient, slog.Default(), cc)

	registry := prometheus.NewRegistry()
	require.NoError(t, registry.Register(collector))

	mfs, err := registry.Gather()
	require.NoError(t, err)

	got := make(map[string]bool, len(mfs))
	for _, mf := range mfs {
		got[mf.GetName()] = true
	}

	t.Run("duration metric emitted for every collector", func(t *testing.T) {
		assert.Equal(t, 5, testutil.CollectAndCount(collector, "redfish_exporter_collector_duration_seconds"))
	})

	t.Run("power", func(t *testing.T) {
		for _, name := range []string{
			"redfish_chassis_health",
			"redfish_power_state",
			"redfish_power_consumed_watts",
			"redfish_power_capacity_watts",
			"redfish_power_average_consumed_watts",
			"redfish_power_max_consumed_watts",
			"redfish_power_min_consumed_watts",
			"redfish_power_control_health",
			"redfish_power_supply_health",
		} {
			assert.True(t, got[name], "missing metric %s", name)
		}
	})

	t.Run("thermal", func(t *testing.T) {
		for _, name := range []string{
			"redfish_temperature_celsius",
			"redfish_temperature_health",
			"redfish_fan_rpm",
			"redfish_fan_health",
		} {
			assert.True(t, got[name], "missing metric %s", name)
		}
	})

	t.Run("systems", func(t *testing.T) {
		for _, name := range []string{
			"redfish_system_info",
			"redfish_system_health",
			"redfish_system_health_rollup",
			"redfish_system_processor_count",
			"redfish_system_processor_health",
			"redfish_system_processor_health_rollup",
			"redfish_system_memory_gib",
			"redfish_system_memory_health",
			"redfish_system_memory_health_rollup",
		} {
			assert.True(t, got[name], "missing metric %s", name)
		}
	})

	t.Run("memory", func(t *testing.T) {
		for _, name := range []string{
			"redfish_memory_capacity_mib",
			"redfish_memory_health",
		} {
			assert.True(t, got[name], "missing metric %s", name)
		}
	})

	t.Run("network", func(t *testing.T) {
		for _, name := range []string{
			"redfish_network_interface_link_up",
			"redfish_network_interface_speed_mbps",
			"redfish_network_interface_health",
		} {
			assert.True(t, got[name], "missing metric %s", name)
		}
	})
}

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

func TestRedfishCollectorIntegration(t *testing.T) {

	ctx := context.Background()

	// Start redfish mockup server container
	req := testcontainers.ContainerRequest{
		Image:        "dmtf/redfish-mockup-server:latest",
		ExposedPorts: []string{"8000/tcp"},
		WaitingFor:   wait.ForHTTP("/redfish/v1/").WithPort("8000/tcp").WithStartupTimeout(30 * time.Second),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	require.NoError(t, err)
	defer container.Terminate(ctx)

	// Get container host and port and combine to target URL
	host, err := container.Host(ctx)
	require.NoError(t, err, "failed to get container host")

	port, err := container.MappedPort(ctx, "8000")
	require.NoError(t, err, "failed to get mapped port")

	target := "http://" + host + ":" + port.Port()

	// Configure collector with all sub-collectors enabled
	config := &config.Config{
		Modules: map[string]config.Module{
			"default": {
				Collectors: config.CollectorsConfig{
					Thermal: true,
					Power:   true,
					Systems: true,
				},
			},
		},
	}

	client, err := client.NewClientFromModuleConfig(config.Modules["default"], target)
	require.NoError(t, err, "failed to create Redfish client")

	// Create collector
	collector := NewRedfishCollector(target, client, slog.Default(), config.Modules["default"].Collectors)

	// Create registry and register collector
	registry := prometheus.NewRegistry()
	err = registry.Register(collector)
	require.NoError(t, err)

	// Collect metrics
	metrics, err := registry.Gather()
	require.NoError(t, err)

	// Verify we got metrics
	assert.NotEmpty(t, metrics, "expected to collect metrics")

	// Verify metrics contain expected families
	metricFamilies := make(map[string]bool)
	for _, mf := range metrics {
		metricFamilies[mf.GetName()] = true
	}

	// Check for some expected metric names
	expectedMetrics := []string{
		"redfish_exporter_collector_duration_seconds",
	}

	for _, expected := range expectedMetrics {
		assert.True(t, metricFamilies[expected], "expected metric %s not found", expected)
	}

	// Verify collector count
	count := testutil.CollectAndCount(collector)
	assert.Greater(t, count, 0, "expected to collect at least one metric")
}

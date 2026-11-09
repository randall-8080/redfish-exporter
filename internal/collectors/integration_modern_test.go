//go:build integration

package collectors

import (
	"context"
	"embed"
	"io/fs"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/e-breuninger/redfish-exporter/internal/client"
	"github.com/e-breuninger/redfish-exporter/internal/config"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// The mockups below are synthetic fixtures, not captures of live hardware:
// they follow the published DMTF schemas (DSP8010) and the resource shapes
// the Supermicro Redfish User Guide documents for H13/H14 systems. They
// contain no credentials, serial numbers, or real inventory. Live-hardware
// compatibility remains unverified.

//go:embed testdata/mockups
var mockupsFS embed.FS

// startSyntheticMockupContainer runs the DMTF mockup server with one of the
// synthetic mockups copied into the container.
func startSyntheticMockupContainer(t *testing.T, name string) string {
	t.Helper()
	root := "testdata/mockups/" + name
	var files []testcontainers.ContainerFile
	err := fs.WalkDir(mockupsFS, root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		f, err := mockupsFS.Open(path)
		if err != nil {
			return err
		}
		t.Cleanup(func() { f.Close() })
		files = append(files, testcontainers.ContainerFile{
			Reader:            f,
			ContainerFilePath: "/mockup/" + rel,
		})
		return nil
	})
	require.NoError(t, err)
	require.NotEmpty(t, files, "mockup %s has no files", name)

	ctx := context.Background()
	req := testcontainers.ContainerRequest{
		Image:        "dmtf/redfish-mockup-server:1.2.9",
		ExposedPorts: []string{"8000/tcp"},
		WaitingFor:   wait.ForHTTP("/redfish/v1/").WithPort("8000/tcp").WithStartupTimeout(30 * time.Second),
		Files:        files,
		Cmd:          []string{"-H", "0.0.0.0", "-p", "8000", "-D", "/mockup"},
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

func collectFromTarget(t *testing.T, target string) map[string]*dto.MetricFamily {
	t.Helper()
	cc := config.CollectorsConfig{
		Thermal:  true,
		Power:    true,
		Systems:  true,
		Memory:   true,
		Network:  true,
		Managers: true,
	}
	redfishClient, err := client.NewClientFromModuleConfig(config.Module{Collectors: cc}, target)
	require.NoError(t, err)
	t.Cleanup(func() { redfishClient.Logout() })
	collector := NewRedfishCollector(target, redfishClient, slog.Default(), cc)
	return gatherFamilies(t, collector)
}

func TestIntegration_ModernOnlyChassis(t *testing.T) {
	target := startSyntheticMockupContainer(t, "modern")
	families := collectFromTarget(t, target)

	// Thermal from sensors, fan resources, and environment metrics.
	v, ok := gaugeValue(t, families, "redfish_temperature_celsius", map[string]string{
		"target": target, "chassis_id": "1", "sensor": "CPU1 Temp", "physical_context": "CPU",
	})
	require.True(t, ok, "missing sensor temperature")
	assert.Equal(t, 45.5, v)
	v, ok = gaugeValue(t, families, "redfish_temperature_threshold_celsius", map[string]string{
		"target": target, "chassis_id": "1", "sensor": "CPU1 Temp", "physical_context": "CPU", "level": "upper_critical",
	})
	require.True(t, ok, "missing sensor threshold")
	assert.Equal(t, 85.0, v)
	v, ok = gaugeValue(t, families, "redfish_fan_rpm", map[string]string{
		"target": target, "chassis_id": "1", "fan": "Fan 1", "physical_context": "SystemBoard", "unit": "RPM",
	})
	require.True(t, ok, "missing fan rpm")
	assert.Equal(t, 2400.0, v)
	v, ok = gaugeValue(t, families, "redfish_fan_speed_percent", map[string]string{
		"target": target, "chassis_id": "1", "fan": "Fan 1", "physical_context": "SystemBoard",
	})
	require.True(t, ok, "missing fan speed percent")
	assert.Equal(t, 60.0, v)
	v, ok = gaugeValue(t, families, "redfish_humidity_percent", map[string]string{
		"target": target, "chassis_id": "1", "sensor": "Humidity 1", "physical_context": "SystemBoard",
	})
	require.True(t, ok, "missing humidity")
	assert.Equal(t, 41.0, v)
	v, ok = gaugeValue(t, families, "redfish_thermal_subsystem_health", map[string]string{
		"target": target, "chassis_id": "1", "health": "OK",
	})
	require.True(t, ok, "missing thermal subsystem health")
	assert.Equal(t, 1.0, v)

	// Power from the subsystem, PSU metrics, environment metrics, and
	// unconsumed sensors.
	v, ok = gaugeValue(t, families, "redfish_power_capacity_watts", map[string]string{
		"target": target, "chassis_id": "1", "name": "PowerSubsystem", "member_id": "",
	})
	require.True(t, ok, "missing subsystem capacity")
	assert.Equal(t, 1600.0, v)
	v, ok = gaugeValue(t, families, "redfish_power_consumed_watts", map[string]string{
		"target": target, "chassis_id": "1", "name": "EnvironmentMetrics", "member_id": "",
	})
	require.True(t, ok, "missing environment metrics consumption")
	assert.Equal(t, 250.0, v)
	v, ok = gaugeValue(t, families, "redfish_power_supply_input_watts", map[string]string{
		"target": target, "chassis_id": "1", "name": "PSU 1", "member_id": "PSU1",
	})
	require.True(t, ok, "missing PSU input watts")
	assert.Equal(t, 300.0, v)
	v, ok = gaugeValue(t, families, "redfish_power_supply_temperature_celsius", map[string]string{
		"target": target, "chassis_id": "1", "name": "PSU 1", "member_id": "PSU1",
	})
	require.True(t, ok, "missing PSU temperature")
	assert.Equal(t, 40.0, v)

	// Sensors claimed by excerpts are not re-emitted: the total power sensor
	// backs the consumption excerpt and the PSU input sensor backs the PSU
	// metrics, so the generic power family stays empty.
	assert.Nil(t, families["redfish_sensor_power_watts"], "claimed power sensors must be suppressed")
	// The PSU temperature sensor is suppressed from the temperature family
	// because the power collector reports it for the supply.
	_, ok = gaugeValue(t, families, "redfish_temperature_celsius", map[string]string{
		"target": target, "chassis_id": "1", "sensor": "PSU1 Temp", "physical_context": "PowerSupply",
	})
	assert.False(t, ok, "PSU temperature sensor must be suppressed from the temperature family")

	// Unclaimed voltage and current sensors are still emitted.
	_, ok = gaugeValue(t, families, "redfish_sensor_voltage_volts", map[string]string{
		"target": target, "chassis_id": "1", "sensor": "System Volt", "physical_context": "SystemBoard",
	})
	assert.True(t, ok, "unclaimed voltage sensor must be emitted")
	_, ok = gaugeValue(t, families, "redfish_sensor_current_amps", map[string]string{
		"target": target, "chassis_id": "1", "sensor": "System Amps", "physical_context": "SystemBoard",
	})
	assert.True(t, ok, "unclaimed current sensor must be emitted")

	// No legacy resources exist: legacy-only metrics stay absent.
	assert.Nil(t, families["redfish_power_control_health"])
	assert.Nil(t, families["redfish_power_average_consumed_watts"])

	for _, m := range families["redfish_collector_success"].GetMetric() {
		assert.Equal(t, 1.0, m.GetGauge().GetValue(), "collector %v must succeed", m.GetLabel())
	}
}

func TestIntegration_HybridChassis(t *testing.T) {
	target := startSyntheticMockupContainer(t, "hybrid")
	families := collectFromTarget(t, target)

	// Modern sources win for equivalent readings; the legacy
	// representations of the same sensors are suppressed.
	v, ok := gaugeValue(t, families, "redfish_temperature_celsius", map[string]string{
		"target": target, "chassis_id": "1", "sensor": "CPU1Temp", "physical_context": "CPU",
	})
	require.True(t, ok, "missing sensor-sourced temperature")
	assert.Equal(t, 45.5, v)
	_, ok = gaugeValue(t, families, "redfish_temperature_celsius", map[string]string{
		"target": target, "chassis_id": "1", "sensor": "CPU1 Temp", "physical_context": "CPU",
	})
	assert.False(t, ok, "legacy temperature must be suppressed when sensors win")
	// The second legacy temperature has no sensor equivalent and is lost to
	// the ladder by design: the sensor family is emitted as a whole.

	// Fan RPM and health from sensors; the legacy percent fan is exported
	// as percent, never as RPM.
	v, ok = gaugeValue(t, families, "redfish_fan_rpm", map[string]string{
		"target": target, "chassis_id": "1", "fan": "Fan1", "physical_context": "SystemBoard", "unit": "RPM",
	})
	require.True(t, ok, "missing sensor-sourced fan rpm")
	assert.Equal(t, 2400.0, v)
	v, ok = gaugeValue(t, families, "redfish_fan_speed_percent", map[string]string{
		"target": target, "chassis_id": "1", "fan": "Fan 1", "physical_context": "SystemBoard",
	})
	require.True(t, ok, "missing fan resource percent")
	assert.Equal(t, 60.0, v)
	var rpmFans []string
	for _, m := range families["redfish_fan_rpm"].GetMetric() {
		for _, lp := range m.GetLabel() {
			if lp.GetName() == "fan" {
				rpmFans = append(rpmFans, lp.GetValue())
			}
		}
	}
	assert.NotContains(t, rpmFans, "Fan2", "legacy percent fan must never surface as RPM")

	// Legacy-only families are still collected from the Power resource.
	v, ok = gaugeValue(t, families, "redfish_power_average_consumed_watts", map[string]string{
		"target": target, "chassis_id": "1", "name": "System Power Control", "member_id": "0",
	})
	require.True(t, ok, "legacy average consumption must be collected")
	assert.Equal(t, 200.0, v)
	v, ok = gaugeValue(t, families, "redfish_power_control_health", map[string]string{
		"target": target, "chassis_id": "1", "name": "System Power Control", "member_id": "0", "health": "OK",
	})
	require.True(t, ok, "legacy control health must be collected")
	assert.Equal(t, 1.0, v)

	// Capacity and consumption come from the modern sources; the legacy zone
	// values do not add duplicate series.
	assert.Equal(t, 1, len(families["redfish_power_capacity_watts"].GetMetric()))
	assert.Equal(t, 1, len(families["redfish_power_consumed_watts"].GetMetric()))

	// PSU health and readings come from the modern supply; the legacy
	// supply readings do not add duplicate series.
	_, ok = gaugeValue(t, families, "redfish_power_supply_health", map[string]string{
		"target": target, "chassis_id": "1", "name": "PSU 1", "member_id": "PSU1", "health": "OK",
	})
	assert.True(t, ok, "missing modern PSU health")
	assert.Equal(t, 1, len(families["redfish_power_supply_input_watts"].GetMetric()))

	// The PSU temperature sensor is reported for the supply, not as a
	// chassis temperature.
	_, ok = gaugeValue(t, families, "redfish_temperature_celsius", map[string]string{
		"target": target, "chassis_id": "1", "sensor": "PSU1 Temp", "physical_context": "PowerSupply",
	})
	assert.False(t, ok, "PSU temperature sensor must be suppressed from the temperature family")

	// The memory module without a reported capacity is skipped instead of
	// exported as zero.
	v, ok = gaugeValue(t, families, "redfish_memory_capacity_mib", map[string]string{
		"target": target, "system_id": "1", "memory_id": "DIMM1", "memory_location": "DIMM_A1",
	})
	require.True(t, ok, "DIMM with capacity must be exported")
	assert.Equal(t, 131072.0, v)
	_, ok = gaugeValue(t, families, "redfish_memory_capacity_mib", map[string]string{
		"target": target, "system_id": "1", "memory_id": "DIMM2", "memory_location": "DIMM_A2",
	})
	assert.False(t, ok, "DIMM without a reported capacity must be skipped")

	for _, m := range families["redfish_collector_success"].GetMetric() {
		assert.Equal(t, 1.0, m.GetGauge().GetValue(), "collector %v must succeed", m.GetLabel())
	}
}

func TestIntegration_BrokenSensorMemberDegrades(t *testing.T) {
	target := startSyntheticMockupContainer(t, "broken")
	families := collectFromTarget(t, target)

	// The usable sensor is still collected ...
	_, ok := gaugeValue(t, families, "redfish_temperature_celsius", map[string]string{
		"target": target, "chassis_id": "1", "sensor": "CPU1 Temp", "physical_context": "CPU",
	})
	assert.True(t, ok, "the healthy sensor must still be collected")

	// ... but the scrape is marked failed: a collection member that 404s is
	// a server problem, not a capability gap.
	require.NotNil(t, families["redfish_collector_success"])
	for _, m := range families["redfish_collector_success"].GetMetric() {
		collector := ""
		for _, lp := range m.GetLabel() {
			if lp.GetName() == "collector" {
				collector = lp.GetValue()
			}
		}
		if collector == "thermal" || collector == "power" {
			assert.Equal(t, 0.0, m.GetGauge().GetValue(), "collector %s must report the broken member", collector)
		}
	}
}

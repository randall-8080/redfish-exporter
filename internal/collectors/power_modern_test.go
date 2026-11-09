package collectors

import (
	"net/http"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/e-breuninger/redfish-exporter/internal/config"
)

// powerModernChassisItem is a chassis that links only modern power resources.
const powerModernChassisItem = `{
	"@odata.id": "/redfish/v1/Chassis/1",
	"Id": "1",
	"PowerState": "On",
	"Status": {"Health": "OK", "HealthRollup": "OK"},
	"Sensors": {"@odata.id": "/redfish/v1/Chassis/1/Sensors"},
	"PowerSubsystem": {"@odata.id": "/redfish/v1/Chassis/1/PowerSubsystem"},
	"EnvironmentMetrics": {"@odata.id": "/redfish/v1/Chassis/1/EnvironmentMetrics"}
}`

// powerModernRoutes builds the routes of a modern-only power chassis: a
// power subsystem with two supplies (one with metrics), environment metrics
// with a total-power excerpt, and sensors covering power, voltage, and
// current, some of which are claimed by the excerpts' DataSourceUri.
func powerModernRoutes() map[string]string {
	return map[string]string{
		"/redfish/v1/Chassis/1/PowerSubsystem": `{
			"@odata.id": "/redfish/v1/Chassis/1/PowerSubsystem",
			"Id": "PowerSubsystem",
			"CapacityWatts": 1600,
			"PowerSupplyRedundancy": [{"RedundancyType": "N+1", "Status": {"State": "Enabled", "Health": "OK"}}],
			"Status": {"State": "Enabled", "Health": "OK"},
			"PowerSupplies": {"@odata.id": "/redfish/v1/Chassis/1/PowerSubsystem/PowerSupplies"}
		}`,
		"/redfish/v1/Chassis/1/PowerSubsystem/PowerSupplies": `{
			"@odata.id": "/redfish/v1/Chassis/1/PowerSubsystem/PowerSupplies",
			"Members": [
				{"@odata.id": "/redfish/v1/Chassis/1/PowerSubsystem/PowerSupplies/PSU1"},
				{"@odata.id": "/redfish/v1/Chassis/1/PowerSubsystem/PowerSupplies/PSU2"}
			],
			"Members@odata.count": 2
		}`,
		"/redfish/v1/Chassis/1/PowerSubsystem/PowerSupplies/PSU1": `{
			"@odata.id": "/redfish/v1/Chassis/1/PowerSubsystem/PowerSupplies/PSU1",
			"Id": "PSU1",
			"Name": "PSU 1",
			"Status": {"State": "Enabled", "Health": "OK"},
			"Metrics": {"@odata.id": "/redfish/v1/Chassis/1/PowerSubsystem/PowerSupplies/PSU1/Metrics"}
		}`,
		"/redfish/v1/Chassis/1/PowerSubsystem/PowerSupplies/PSU1/Metrics": `{
			"@odata.id": "/redfish/v1/Chassis/1/PowerSubsystem/PowerSupplies/PSU1/Metrics",
			"InputPowerWatts": {"Reading": 300, "DataSourceUri": "/redfish/v1/Chassis/1/Sensors/PSU1InPower"},
			"OutputPowerWatts": {"Reading": 280, "DataSourceUri": "/redfish/v1/Chassis/1/Sensors/PSU1OutPower"},
			"InputVoltage": {"Reading": 230, "DataSourceUri": "/redfish/v1/Chassis/1/Sensors/PSU1Volt"},
			"InputCurrentAmps": {"Reading": 1.5, "DataSourceUri": "/redfish/v1/Chassis/1/Sensors/PSU1Amps"},
			"TemperatureCelsius": {"Reading": 40, "DataSourceUri": "/redfish/v1/Chassis/1/Sensors/PSU1Temp"}
		}`,
		// PSU2 has no Metrics link: health only, no readings.
		"/redfish/v1/Chassis/1/PowerSubsystem/PowerSupplies/PSU2": `{
			"@odata.id": "/redfish/v1/Chassis/1/PowerSubsystem/PowerSupplies/PSU2",
			"Id": "PSU2",
			"Name": "PSU 2",
			"Status": {"State": "Enabled", "Health": "Warning"}
		}`,
		"/redfish/v1/Chassis/1/EnvironmentMetrics": `{
			"@odata.id": "/redfish/v1/Chassis/1/EnvironmentMetrics",
			"PowerWatts": {"Reading": 250, "DataSourceUri": "/redfish/v1/Chassis/1/Sensors/TotalPower"}
		}`,
		"/redfish/v1/Chassis/1/Sensors": sensorsCollectionJSON(
			"PSU1InPower", "PSU1OutPower", "PSU1Volt", "PSU1Amps", "PSU1Temp", "TotalPower", "FanPower", "SysVolt", "SysAmps"),
		"/redfish/v1/Chassis/1/Sensors/PSU1InPower":  sensorJSON("PSU1InPower", "PSU1 In Power", "Power", ptr(300), "PowerSupply", "Enabled", "OK", ""),
		"/redfish/v1/Chassis/1/Sensors/PSU1OutPower": sensorJSON("PSU1OutPower", "PSU1 Out Power", "Power", ptr(280), "PowerSupply", "Enabled", "OK", ""),
		"/redfish/v1/Chassis/1/Sensors/PSU1Volt":     sensorJSON("PSU1Volt", "PSU1 Volt", "Voltage", ptr(230), "PowerSupply", "Enabled", "OK", ""),
		"/redfish/v1/Chassis/1/Sensors/PSU1Amps":     sensorJSON("PSU1Amps", "PSU1 Amps", "Current", ptr(1.5), "PowerSupply", "Enabled", "OK", ""),
		"/redfish/v1/Chassis/1/Sensors/PSU1Temp":     sensorJSON("PSU1Temp", "PSU1 Temp", "Temperature", ptr(40), "PowerSupply", "Enabled", "OK", ""),
		"/redfish/v1/Chassis/1/Sensors/TotalPower":   sensorJSON("TotalPower", "Total Power", "Power", ptr(250), "SystemBoard", "Enabled", "OK", ""),
		"/redfish/v1/Chassis/1/Sensors/FanPower": sensorJSON("FanPower", "Fan Power", "Power", ptr(60), "SystemBoard", "Enabled", "OK",
			`{"UpperCritical": {"Reading": 120}}`),
		"/redfish/v1/Chassis/1/Sensors/SysVolt": sensorJSON("SysVolt", "System Volt", "Voltage", ptr(12.1), "SystemBoard", "Enabled", "OK", ""),
		"/redfish/v1/Chassis/1/Sensors/SysAmps": sensorJSON("SysAmps", "System Amps", "Current", ptr(8.2), "SystemBoard", "Enabled", "OK", ""),
	}
}

func TestPowerCollector_ModernOnly(t *testing.T) {
	r := defaultResponses()
	r.chassisItem = powerModernChassisItem
	r.extraRoutes = powerModernRoutes()
	srv := newTestServer(t, r)
	defer srv.Close()
	c := NewPowerCollector(srv.URL, newTestGoFishClient(t, srv.URL), discardLogger())

	families := gatherFamilies(t, c)

	// Chassis-level metrics.
	v, ok := gaugeValue(t, families, "redfish_power_state", map[string]string{"target": srv.URL, "chassis_id": "1"})
	require.True(t, ok)
	assert.Equal(t, 1.0, v)

	// Subsystem health and capacity.
	v, ok = gaugeValue(t, families, "redfish_power_subsystem_health", map[string]string{"target": srv.URL, "chassis_id": "1", "health": "OK"})
	require.True(t, ok, "missing power subsystem health")
	assert.Equal(t, 1.0, v)
	v, ok = gaugeValue(t, families, "redfish_power_capacity_watts", map[string]string{
		"target": srv.URL, "chassis_id": "1", "name": "PowerSubsystem", "member_id": "",
	})
	require.True(t, ok, "missing subsystem capacity")
	assert.Equal(t, 1600.0, v)
	assert.Equal(t, 1, len(families["redfish_power_capacity_watts"].GetMetric()), "capacity must come from a single source")

	// Consumption from the environment metrics excerpt.
	v, ok = gaugeValue(t, families, "redfish_power_consumed_watts", map[string]string{
		"target": srv.URL, "chassis_id": "1", "name": "EnvironmentMetrics", "member_id": "",
	})
	require.True(t, ok, "missing environment metrics consumption")
	assert.Equal(t, 250.0, v)

	// Modern redundancy group.
	v, ok = gaugeValue(t, families, "redfish_power_redundancy_health", map[string]string{
		"target": srv.URL, "chassis_id": "1", "name": "N+1", "member_id": "", "health": "OK",
	})
	require.True(t, ok, "missing modern redundancy health")
	assert.Equal(t, 1.0, v)

	// Supply health from the modern supplies, keyed by Id.
	v, ok = gaugeValue(t, families, "redfish_power_supply_health", map[string]string{
		"target": srv.URL, "chassis_id": "1", "name": "PSU 1", "member_id": "PSU1", "health": "OK",
	})
	require.True(t, ok, "missing modern PSU 1 health")
	assert.Equal(t, 1.0, v)
	v, ok = gaugeValue(t, families, "redfish_power_supply_health", map[string]string{
		"target": srv.URL, "chassis_id": "1", "name": "PSU 2", "member_id": "PSU2", "health": "Warning",
	})
	require.True(t, ok, "missing modern PSU 2 health")
	assert.Equal(t, 0.0, v, "warning health must map to 0")

	// PSU 1 readings from its metrics resource.
	for name, want := range map[string]struct {
		metric string
		value  float64
	}{
		"input_watts":         {"redfish_power_supply_input_watts", 300},
		"output_watts":        {"redfish_power_supply_output_watts", 280},
		"input_volts":         {"redfish_power_supply_input_volts", 230},
		"input_amps":          {"redfish_power_supply_input_amps", 1.5},
		"temperature_celsius": {"redfish_power_supply_temperature_celsius", 40},
	} {
		v, ok := gaugeValue(t, families, want.metric, map[string]string{
			"target": srv.URL, "chassis_id": "1", "name": "PSU 1", "member_id": "PSU1",
		})
		require.True(t, ok, "missing PSU %s", name)
		assert.Equal(t, want.value, v)
	}
	// PSU 2 has no metrics resource: no readings.
	_, ok = gaugeValue(t, families, "redfish_power_supply_input_watts", map[string]string{
		"target": srv.URL, "chassis_id": "1", "name": "PSU 2", "member_id": "PSU2",
	})
	assert.False(t, ok, "PSU without metrics must not report readings")

	// Generic sensor families: only sensors not claimed by an emitted
	// excerpt's DataSourceUri.
	_, ok = gaugeValue(t, families, "redfish_sensor_power_watts", map[string]string{
		"target": srv.URL, "chassis_id": "1", "sensor": "Total Power", "physical_context": "SystemBoard",
	})
	assert.False(t, ok, "total power sensor is reported by the environment metrics excerpt")
	v, ok = gaugeValue(t, families, "redfish_sensor_power_watts", map[string]string{
		"target": srv.URL, "chassis_id": "1", "sensor": "Fan Power", "physical_context": "SystemBoard",
	})
	require.True(t, ok, "unclaimed power sensor must be emitted")
	assert.Equal(t, 60.0, v)
	v, ok = gaugeValue(t, families, "redfish_sensor_power_threshold_watts", map[string]string{
		"target": srv.URL, "chassis_id": "1", "sensor": "Fan Power", "physical_context": "SystemBoard", "level": "upper_critical",
	})
	require.True(t, ok, "power sensor threshold must be emitted")
	assert.Equal(t, 120.0, v)

	_, ok = gaugeValue(t, families, "redfish_sensor_voltage_volts", map[string]string{
		"target": srv.URL, "chassis_id": "1", "sensor": "PSU1 Volt", "physical_context": "PowerSupply",
	})
	assert.False(t, ok, "PSU voltage sensor is reported by the PSU metrics excerpt")
	v, ok = gaugeValue(t, families, "redfish_sensor_voltage_volts", map[string]string{
		"target": srv.URL, "chassis_id": "1", "sensor": "System Volt", "physical_context": "SystemBoard",
	})
	require.True(t, ok, "unclaimed voltage sensor must be emitted")
	assert.Equal(t, 12.1, v)

	_, ok = gaugeValue(t, families, "redfish_sensor_current_amps", map[string]string{
		"target": srv.URL, "chassis_id": "1", "sensor": "PSU1 Amps", "physical_context": "PowerSupply",
	})
	assert.False(t, ok, "PSU current sensor is reported by the PSU metrics excerpt")
	v, ok = gaugeValue(t, families, "redfish_sensor_current_amps", map[string]string{
		"target": srv.URL, "chassis_id": "1", "sensor": "System Amps", "physical_context": "SystemBoard",
	})
	require.True(t, ok, "unclaimed current sensor must be emitted")
	assert.Equal(t, 8.2, v)

	// No legacy metrics: the chassis links no Power resource.
	assert.Nil(t, families["redfish_power_control_health"])
	assert.Nil(t, families["redfish_power_average_consumed_watts"])
}

func TestPowerCollector_SubsystemAbsentFallsBackToLegacy(t *testing.T) {
	r := defaultResponses()
	// Wall-powered systems (e.g. Supermicro boards without redundant
	// supplies) report an absent power subsystem with no capacity.
	r.chassisItem = `{
		"@odata.id": "/redfish/v1/Chassis/1",
		"Id": "1",
		"PowerState": "On",
		"Status": {"Health": "OK", "HealthRollup": "OK"},
		"Power": {"@odata.id": "/redfish/v1/Chassis/1/Power"},
		"PowerSubsystem": {"@odata.id": "/redfish/v1/Chassis/1/PowerSubsystem"}
	}`
	r.extraRoutes = map[string]string{
		"/redfish/v1/Chassis/1/PowerSubsystem": `{
			"@odata.id": "/redfish/v1/Chassis/1/PowerSubsystem",
			"Id": "PowerSubsystem",
			"CapacityWatts": 0,
			"Status": {"State": "Absent"},
			"PowerSupplies": {"@odata.id": "/redfish/v1/Chassis/1/PowerSubsystem/PowerSupplies"}
		}`,
		"/redfish/v1/Chassis/1/PowerSubsystem/PowerSupplies": `{
			"@odata.id": "/redfish/v1/Chassis/1/PowerSubsystem/PowerSupplies",
			"Members": [],
			"Members@odata.count": 0
		}`,
	}
	srv := newTestServer(t, r)
	defer srv.Close()
	client := newTestGoFishClient(t, srv.URL)
	c := NewPowerCollector(srv.URL, client, discardLogger())

	// An absent subsystem is a capability gap, not a failure.
	ctx := newScrapeContext(client, discardLogger(), false)
	ch := make(chan prometheus.Metric, 32)
	err := c.collect(ctx, ch)
	close(ch)
	assert.NoError(t, err)

	families := gatherFamilies(t, c)
	// Capacity falls back to the legacy power control zone.
	v, ok := gaugeValue(t, families, "redfish_power_capacity_watts", map[string]string{
		"target": srv.URL, "chassis_id": "1", "name": "System Power Control", "member_id": "0",
	})
	require.True(t, ok, "legacy capacity must be the fallback")
	assert.Equal(t, 1600.0, v)
	// No subsystem health for an absent subsystem.
	assert.Nil(t, families["redfish_power_subsystem_health"])
}

func TestPowerCollector_LegacyPSUFieldsFallback(t *testing.T) {
	r := defaultResponses()
	r.power = `{
		"@odata.id": "/redfish/v1/Chassis/1/Power",
		"PowerControl": [],
		"Redundancy": [],
		"PowerSupplies": [{
			"MemberId": "0",
			"Name": "PSU1",
			"LineInputVoltage": 230,
			"PowerInputWatts": 300,
			"PowerOutputWatts": 280,
			"Status": {"Health": "OK", "State": "Enabled"}
		}]
	}`
	srv := newTestServer(t, r)
	defer srv.Close()
	c := NewPowerCollector(srv.URL, newTestGoFishClient(t, srv.URL), discardLogger())

	families := gatherFamilies(t, c)
	for metric, want := range map[string]float64{
		"redfish_power_supply_input_watts":  300,
		"redfish_power_supply_output_watts": 280,
		"redfish_power_supply_input_volts":  230,
	} {
		v, ok := gaugeValue(t, families, metric, map[string]string{
			"target": srv.URL, "chassis_id": "1", "name": "PSU1", "member_id": "0",
		})
		require.True(t, ok, "missing legacy PSU reading %s", metric)
		assert.Equal(t, want, v)
	}
	// No modern supplies: no input current (the legacy resource has none).
	assert.Nil(t, families["redfish_power_supply_input_amps"])
}

func TestPowerCollector_PSUMemberFailureDegrades(t *testing.T) {
	r := defaultResponses()
	r.chassisItem = powerModernChassisItem
	routes := powerModernRoutes()
	routes["/redfish/v1/Chassis/1/PowerSubsystem/PowerSupplies"] = `{
		"@odata.id": "/redfish/v1/Chassis/1/PowerSubsystem/PowerSupplies",
		"Members": [
			{"@odata.id": "/redfish/v1/Chassis/1/PowerSubsystem/PowerSupplies/PSU1"},
			{"@odata.id": "/redfish/v1/Chassis/1/PowerSubsystem/PowerSupplies/Broken"}
		],
		"Members@odata.count": 2
	}`
	r.extraRoutes = routes
	r.overrides = map[string]routeOverride{
		"/redfish/v1/Chassis/1/PowerSubsystem/PowerSupplies/Broken": {status: http.StatusInternalServerError},
	}
	srv := newTestServer(t, r)
	defer srv.Close()
	client := newTestGoFishClient(t, srv.URL)
	c := NewPowerCollector(srv.URL, client, discardLogger())

	ctx := newScrapeContext(client, discardLogger(), false)
	ch := make(chan prometheus.Metric, 64)
	err := c.collect(ctx, ch)
	close(ch)

	assert.Error(t, err, "a failing PSU member must degrade the scrape")
	families := gatherFamilies(t, c)
	_, ok := gaugeValue(t, families, "redfish_power_supply_health", map[string]string{
		"target": srv.URL, "chassis_id": "1", "name": "PSU 1", "member_id": "PSU1", "health": "OK",
	})
	assert.True(t, ok, "the healthy PSU must still be collected")
}

func TestPowerCollector_PowerSubsystemUnauthorizedDegrades(t *testing.T) {
	r := defaultResponses()
	r.chassisItem = powerModernChassisItem
	r.extraRoutes = powerModernRoutes()
	r.overrides = map[string]routeOverride{
		"/redfish/v1/Chassis/1/PowerSubsystem": {status: http.StatusUnauthorized},
	}
	srv := newTestServer(t, r)
	defer srv.Close()
	client := newTestGoFishClient(t, srv.URL)
	c := NewPowerCollector(srv.URL, client, discardLogger())

	ctx := newScrapeContext(client, discardLogger(), false)
	ch := make(chan prometheus.Metric, 64)
	err := c.collect(ctx, ch)
	close(ch)

	assert.Error(t, err, "an authorization failure must degrade the scrape")
	families := gatherFamilies(t, c)
	// Chassis-level metrics are still collected.
	_, ok := gaugeValue(t, families, "redfish_power_state", map[string]string{"target": srv.URL, "chassis_id": "1"})
	assert.True(t, ok)
}

func TestPowerCollector_PreservesZeroPSUReading(t *testing.T) {
	r := defaultResponses()
	r.chassisItem = powerModernChassisItem
	routes := powerModernRoutes()
	routes["/redfish/v1/Chassis/1/PowerSubsystem/PowerSupplies/PSU1/Metrics"] = `{
		"@odata.id": "/redfish/v1/Chassis/1/PowerSubsystem/PowerSupplies/PSU1/Metrics",
		"OutputPowerWatts": {"Reading": 0, "DataSourceUri": "/redfish/v1/Chassis/1/Sensors/PSU1OutPower"}
	}`
	r.extraRoutes = routes
	srv := newTestServer(t, r)
	defer srv.Close()
	c := NewPowerCollector(srv.URL, newTestGoFishClient(t, srv.URL), discardLogger())

	families := gatherFamilies(t, c)
	v, ok := gaugeValue(t, families, "redfish_power_supply_output_watts", map[string]string{
		"target": srv.URL, "chassis_id": "1", "name": "PSU 1", "member_id": "PSU1",
	})
	require.True(t, ok, "a real zero PSU reading must be preserved")
	assert.Equal(t, 0.0, v)
}

func TestPowerCollector_ThermalSuppressesPSUTemperatureSensor(t *testing.T) {
	r := defaultResponses()
	r.chassisItem = `{
		"@odata.id": "/redfish/v1/Chassis/1",
		"Id": "1",
		"PowerState": "On",
		"Status": {"Health": "OK", "HealthRollup": "OK"},
		"Sensors": {"@odata.id": "/redfish/v1/Chassis/1/Sensors"},
		"PowerSubsystem": {"@odata.id": "/redfish/v1/Chassis/1/PowerSubsystem"}
	}`
	routes := powerModernRoutes()
	delete(routes, "/redfish/v1/Chassis/1/EnvironmentMetrics")
	routes["/redfish/v1/Chassis/1/Sensors"] = sensorsCollectionJSON("PSU1Temp", "CPU1Temp")
	routes["/redfish/v1/Chassis/1/Sensors/CPU1Temp"] = sensorJSON("CPU1Temp", "CPU1 Temp", "Temperature", ptr(45), "CPU", "Enabled", "OK", "")
	r.extraRoutes = routes
	srv := newTestServer(t, r)
	defer srv.Close()

	// Run the full collector with thermal and power enabled so they share
	// one scrape context.
	cc := config.CollectorsConfig{Thermal: true, Power: true}
	collector := NewRedfishCollector(srv.URL, newTestGoFishClient(t, srv.URL), discardLogger(), cc)
	families := gatherFamilies(t, collector)

	// The PSU temperature is reported by the power collector ...
	v, ok := gaugeValue(t, families, "redfish_power_supply_temperature_celsius", map[string]string{
		"target": srv.URL, "chassis_id": "1", "name": "PSU 1", "member_id": "PSU1",
	})
	require.True(t, ok, "missing PSU temperature")
	assert.Equal(t, 40.0, v)
	// ... and suppressed from the chassis temperature family.
	_, ok = gaugeValue(t, families, "redfish_temperature_celsius", map[string]string{
		"target": srv.URL, "chassis_id": "1", "sensor": "PSU1 Temp", "physical_context": "PowerSupply",
	})
	assert.False(t, ok, "PSU temperature sensor must not be emitted as a chassis temperature")
	// The regular CPU temperature is unaffected.
	_, ok = gaugeValue(t, families, "redfish_temperature_celsius", map[string]string{
		"target": srv.URL, "chassis_id": "1", "sensor": "CPU1 Temp", "physical_context": "CPU",
	})
	assert.True(t, ok, "regular temperature sensors must still be emitted")

	// Both collectors succeeded.
	for _, m := range families["redfish_collector_success"].GetMetric() {
		assert.Equal(t, 1.0, m.GetGauge().GetValue(), "collector %v must succeed", m.GetLabel())
	}
}

func TestPowerCollector_SameTypeRedundancyGroupsDisambiguated(t *testing.T) {
	r := defaultResponses()
	r.chassisItem = powerModernChassisItem
	routes := powerModernRoutes()
	// Two groups without GroupName sharing the same type: identical label
	// sets would fail the whole Prometheus gather.
	routes["/redfish/v1/Chassis/1/PowerSubsystem"] = `{
		"@odata.id": "/redfish/v1/Chassis/1/PowerSubsystem",
		"Id": "PowerSubsystem",
		"CapacityWatts": 1600,
		"PowerSupplyRedundancy": [
			{"RedundancyType": "N+1", "Status": {"State": "Enabled", "Health": "OK"}},
			{"RedundancyType": "N+1", "Status": {"State": "Enabled", "Health": "Warning"}}
		],
		"Status": {"State": "Enabled", "Health": "OK"},
		"PowerSupplies": {"@odata.id": "/redfish/v1/Chassis/1/PowerSubsystem/PowerSupplies"}
	}`
	r.extraRoutes = routes
	srv := newTestServer(t, r)
	defer srv.Close()
	c := NewPowerCollector(srv.URL, newTestGoFishClient(t, srv.URL), discardLogger())

	// gatherFamilies fails on duplicate series; both groups must survive it.
	families := gatherFamilies(t, c)
	f := families["redfish_power_redundancy_health"]
	require.NotNil(t, f)
	assert.Equal(t, 2, len(f.GetMetric()), "both same-typed redundancy groups must be emitted")
	_, ok := gaugeValue(t, families, "redfish_power_redundancy_health", map[string]string{
		"target": srv.URL, "chassis_id": "1", "name": "N+1", "member_id": "", "health": "OK",
	})
	assert.True(t, ok, "first group keeps the plain type name")
	_, ok = gaugeValue(t, families, "redfish_power_redundancy_health", map[string]string{
		"target": srv.URL, "chassis_id": "1", "name": "N+1 (1)", "member_id": "", "health": "Warning",
	})
	assert.True(t, ok, "second group must be disambiguated")
}

func TestPowerCollector_SensorNotSuppressedByUnemittedExcerpt(t *testing.T) {
	r := defaultResponses()
	r.chassisItem = powerModernChassisItem
	routes := powerModernRoutes()
	// The excerpt links a voltage sensor but reports no reading: the sensor
	// must stay available in the generic voltage family.
	routes["/redfish/v1/Chassis/1/PowerSubsystem/PowerSupplies/PSU1/Metrics"] = `{
		"@odata.id": "/redfish/v1/Chassis/1/PowerSubsystem/PowerSupplies/PSU1/Metrics",
		"InputVoltage": {"DataSourceUri": "/redfish/v1/Chassis/1/Sensors/SysVolt"}
	}`
	r.extraRoutes = routes
	srv := newTestServer(t, r)
	defer srv.Close()
	c := NewPowerCollector(srv.URL, newTestGoFishClient(t, srv.URL), discardLogger())

	families := gatherFamilies(t, c)
	v, ok := gaugeValue(t, families, "redfish_sensor_voltage_volts", map[string]string{
		"target": srv.URL, "chassis_id": "1", "sensor": "System Volt", "physical_context": "SystemBoard",
	})
	require.True(t, ok, "a sensor whose excerpt carries no reading must not be suppressed")
	assert.Equal(t, 12.1, v)
}

func TestPowerCollector_LegacyVoltageFallbackAndDedup(t *testing.T) {
	r := defaultResponses()
	// A voltage sensor with no reading must not block the legacy Voltages
	// fallback, and same-named legacy entries must be disambiguated.
	r.power = `{
		"@odata.id": "/redfish/v1/Chassis/1/Power",
		"PowerControl": [],
		"Redundancy": [],
		"PowerSupplies": [],
		"Voltages": [
			{"MemberId": "0", "Name": "System Voltage", "ReadingVolts": 12.0, "PhysicalContext": "SystemBoard", "Status": {"State": "Enabled", "Health": "OK"}},
			{"MemberId": "1", "Name": "System Voltage", "ReadingVolts": 5.0, "PhysicalContext": "SystemBoard", "Status": {"State": "Enabled", "Health": "OK"}}
		]
	}`
	r.chassisItem = `{
		"@odata.id": "/redfish/v1/Chassis/1",
		"Id": "1",
		"PowerState": "On",
		"Power": {"@odata.id": "/redfish/v1/Chassis/1/Power"},
		"Sensors": {"@odata.id": "/redfish/v1/Chassis/1/Sensors"}
	}`
	r.extraRoutes = map[string]string{
		"/redfish/v1/Chassis/1/Sensors":          sensorsCollectionJSON("NullVolt"),
		"/redfish/v1/Chassis/1/Sensors/NullVolt": sensorJSON("NullVolt", "Null Volt", "Voltage", nil, "SystemBoard", "Enabled", "OK", ""),
	}
	srv := newTestServer(t, r)
	defer srv.Close()
	c := NewPowerCollector(srv.URL, newTestGoFishClient(t, srv.URL), discardLogger())

	families := gatherFamilies(t, c)
	f := families["redfish_sensor_voltage_volts"]
	require.NotNil(t, f, "legacy voltages must be emitted despite a sensor without a reading")
	assert.Equal(t, 2, len(f.GetMetric()), "same-named legacy voltages must both be emitted")
	v, ok := gaugeValue(t, families, "redfish_sensor_voltage_volts", map[string]string{
		"target": srv.URL, "chassis_id": "1", "sensor": "System Voltage", "physical_context": "SystemBoard",
	})
	require.True(t, ok)
	assert.Equal(t, 12.0, v)
	v, ok = gaugeValue(t, families, "redfish_sensor_voltage_volts", map[string]string{
		"target": srv.URL, "chassis_id": "1", "sensor": "System Voltage (1)", "physical_context": "SystemBoard",
	})
	require.True(t, ok, "second same-named voltage must be disambiguated")
	assert.Equal(t, 5.0, v)
}

func TestPowerCollector_ModernSuppliesWithoutMetricsFallBackToLegacyReadings(t *testing.T) {
	r := defaultResponses()
	r.chassisItem = powerModernChassisItem
	routes := powerModernRoutes()
	// The subsystem lists supplies but none has a metrics resource: the
	// legacy supply fields must provide the readings.
	routes["/redfish/v1/Chassis/1/PowerSubsystem/PowerSupplies/PSU1"] = `{
		"@odata.id": "/redfish/v1/Chassis/1/PowerSubsystem/PowerSupplies/PSU1",
		"Id": "PSU1",
		"Name": "PSU 1",
		"Status": {"State": "Enabled", "Health": "OK"}
	}`
	delete(routes, "/redfish/v1/Chassis/1/PowerSubsystem/PowerSupplies/PSU1/Metrics")
	delete(routes, "/redfish/v1/Chassis/1/EnvironmentMetrics")
	routes["/redfish/v1/Chassis/1"] = `{
		"@odata.id": "/redfish/v1/Chassis/1",
		"Id": "1",
		"PowerState": "On",
		"Status": {"Health": "OK", "HealthRollup": "OK"},
		"Sensors": {"@odata.id": "/redfish/v1/Chassis/1/Sensors"},
		"Power": {"@odata.id": "/redfish/v1/Chassis/1/Power"},
		"PowerSubsystem": {"@odata.id": "/redfish/v1/Chassis/1/PowerSubsystem"}
	}`
	r.power = `{
		"@odata.id": "/redfish/v1/Chassis/1/Power",
		"PowerControl": [],
		"Redundancy": [],
		"PowerSupplies": [{
			"MemberId": "0",
			"Name": "PSU1",
			"LineInputVoltage": 230,
			"PowerInputWatts": 300,
			"PowerOutputWatts": 280,
			"Status": {"Health": "OK", "State": "Enabled"}
		}]
	}`
	r.extraRoutes = routes
	srv := newTestServer(t, r)
	defer srv.Close()
	c := NewPowerCollector(srv.URL, newTestGoFishClient(t, srv.URL), discardLogger())

	families := gatherFamilies(t, c)
	for metric, want := range map[string]float64{
		"redfish_power_supply_input_watts":  300,
		"redfish_power_supply_output_watts": 280,
		"redfish_power_supply_input_volts":  230,
	} {
		v, ok := gaugeValue(t, families, metric, map[string]string{
			"target": srv.URL, "chassis_id": "1", "name": "PSU1", "member_id": "0",
		})
		require.True(t, ok, "missing legacy PSU reading %s despite no metrics resources", metric)
		assert.Equal(t, want, v)
	}
}

func TestPowerCollector_AbsentPSUTemperatureSensorNotSuppressed(t *testing.T) {
	r := defaultResponses()
	r.chassisItem = `{
		"@odata.id": "/redfish/v1/Chassis/1",
		"Id": "1",
		"PowerState": "On",
		"Status": {"Health": "OK", "HealthRollup": "OK"},
		"Sensors": {"@odata.id": "/redfish/v1/Chassis/1/Sensors"},
		"PowerSubsystem": {"@odata.id": "/redfish/v1/Chassis/1/PowerSubsystem"}
	}`
	routes := powerModernRoutes()
	delete(routes, "/redfish/v1/Chassis/1/EnvironmentMetrics")
	// PSU2 is an absent slot whose metrics resource still carries a
	// temperature: the power collector skips it, so the sensor must stay in
	// the temperature family.
	routes["/redfish/v1/Chassis/1/PowerSubsystem/PowerSupplies/PSU2"] = `{
		"@odata.id": "/redfish/v1/Chassis/1/PowerSubsystem/PowerSupplies/PSU2",
		"Id": "PSU2",
		"Name": "PSU 2",
		"Status": {"State": "Absent"},
		"Metrics": {"@odata.id": "/redfish/v1/Chassis/1/PowerSubsystem/PowerSupplies/PSU2/Metrics"}
	}`
	routes["/redfish/v1/Chassis/1/PowerSubsystem/PowerSupplies/PSU2/Metrics"] = `{
		"@odata.id": "/redfish/v1/Chassis/1/PowerSubsystem/PowerSupplies/PSU2/Metrics",
		"TemperatureCelsius": {"Reading": 40, "DataSourceUri": "/redfish/v1/Chassis/1/Sensors/PSU1Temp"}
	}`
	delete(routes, "/redfish/v1/Chassis/1/PowerSubsystem/PowerSupplies/PSU1/Metrics")
	routes["/redfish/v1/Chassis/1/PowerSubsystem/PowerSupplies/PSU1"] = `{
		"@odata.id": "/redfish/v1/Chassis/1/PowerSubsystem/PowerSupplies/PSU1",
		"Id": "PSU1",
		"Name": "PSU 1",
		"Status": {"State": "Enabled", "Health": "OK"}
	}`
	r.extraRoutes = routes
	srv := newTestServer(t, r)
	defer srv.Close()

	cc := config.CollectorsConfig{Thermal: true, Power: true}
	collector := NewRedfishCollector(srv.URL, newTestGoFishClient(t, srv.URL), discardLogger(), cc)
	families := gatherFamilies(t, collector)

	// The absent supply reports no PSU temperature ...
	assert.Nil(t, families["redfish_power_supply_temperature_celsius"], "absent supply must report no temperature")
	// ... so its sensor stays available as a chassis temperature.
	_, ok := gaugeValue(t, families, "redfish_temperature_celsius", map[string]string{
		"target": srv.URL, "chassis_id": "1", "sensor": "PSU1 Temp", "physical_context": "PowerSupply",
	})
	assert.True(t, ok, "temperature sensor of an absent supply must not be suppressed")
}

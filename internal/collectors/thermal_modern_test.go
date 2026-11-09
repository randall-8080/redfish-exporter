package collectors

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sensorJSON builds a Sensor resource body. reading is nil for a missing
// reading; thresholdsJSON is a raw Thresholds object or empty.
func sensorJSON(id, name, readingType string, reading *float64, context, state, health, thresholdsJSON string) string {
	readingStr := "null"
	if reading != nil {
		readingStr = strconv.FormatFloat(*reading, 'f', -1, 64)
	}
	thresholds := ""
	if thresholdsJSON != "" {
		thresholds = fmt.Sprintf(",\"Thresholds\": %s", thresholdsJSON)
	}
	return fmt.Sprintf(`{
		"@odata.id": "/redfish/v1/Chassis/1/Sensors/%s",
		"Id": "%s",
		"Name": "%s",
		"ReadingType": "%s",
		"Reading": %s,
		"PhysicalContext": "%s",
		"Status": {"State": "%s", "Health": "%s"}%s
	}`, id, id, name, readingType, readingStr, context, state, health, thresholds)
}

// sensorsCollectionJSON builds a sensor collection listing the given member IDs.
func sensorsCollectionJSON(ids ...string) string {
	links := make([]string, 0, len(ids))
	for _, id := range ids {
		links = append(links, fmt.Sprintf(`{"@odata.id": "/redfish/v1/Chassis/1/Sensors/%s"}`, id))
	}
	return fmt.Sprintf(`{
		"@odata.id": "/redfish/v1/Chassis/1/Sensors",
		"Members": [%s],
		"Members@odata.count": %d
	}`, strings.Join(links, ","), len(ids))
}

// gaugeValue returns the value of the named family's metric with exactly the
// given labels, and whether it exists.
func gaugeValue(t *testing.T, families map[string]*dto.MetricFamily, name string, labels map[string]string) (float64, bool) {
	t.Helper()
	m := findMetric(families[name], labels)
	if m == nil {
		return 0, false
	}
	return m.GetGauge().GetValue(), true
}

func ptr(v float64) *float64 { return &v }

// modernChassisItem is a chassis that links only modern thermal resources.
const modernChassisItem = `{
	"@odata.id": "/redfish/v1/Chassis/1",
	"Id": "1",
	"PowerState": "On",
	"Status": {"Health": "OK", "HealthRollup": "OK"},
	"Sensors": {"@odata.id": "/redfish/v1/Chassis/1/Sensors"},
	"ThermalSubsystem": {"@odata.id": "/redfish/v1/Chassis/1/ThermalSubsystem"},
	"EnvironmentMetrics": {"@odata.id": "/redfish/v1/Chassis/1/EnvironmentMetrics"}
}`

// modernThermalRoutes builds the routes of a modern-only chassis: sensors for
// temperature, fan rotation, fan percent, and humidity, a thermal subsystem
// with one fan and thermal metrics, and environment metrics.
func modernThermalRoutes() map[string]string {
	return map[string]string{
		"/redfish/v1/Chassis/1/Sensors": sensorsCollectionJSON("CPU1Temp", "Fan1", "Fan1Pct", "Humidity1"),
		"/redfish/v1/Chassis/1/Sensors/CPU1Temp": sensorJSON("CPU1Temp", "CPU1 Temp", "Temperature", ptr(45.5), "CPU", "Enabled", "OK",
			`{"UpperCritical": {"Reading": 85}, "LowerCritical": {"Reading": -5}, "UpperCaution": {"Reading": 75}}`),
		"/redfish/v1/Chassis/1/Sensors/Fan1": sensorJSON("Fan1", "Fan 1", "Rotational", ptr(2400), "SystemBoard", "Enabled", "OK",
			`{"LowerCritical": {"Reading": 500}}`),
		"/redfish/v1/Chassis/1/Sensors/Fan1Pct": sensorJSON("Fan1Pct", "Fan 1", "Percent", ptr(60), "SystemBoard", "Enabled", "OK", ""),
		"/redfish/v1/Chassis/1/Sensors/Humidity1": sensorJSON("Humidity1", "Humidity 1", "Humidity", ptr(41), "SystemBoard", "Enabled", "OK",
			`{"UpperCritical": {"Reading": 80}}`),
		"/redfish/v1/Chassis/1/ThermalSubsystem": `{
			"@odata.id": "/redfish/v1/Chassis/1/ThermalSubsystem",
			"Id": "ThermalSubsystem",
			"Status": {"State": "Enabled", "Health": "OK"},
			"Fans": {"@odata.id": "/redfish/v1/Chassis/1/ThermalSubsystem/Fans"},
			"ThermalMetrics": {"@odata.id": "/redfish/v1/Chassis/1/ThermalSubsystem/ThermalMetrics"}
		}`,
		"/redfish/v1/Chassis/1/ThermalSubsystem/Fans": `{
			"@odata.id": "/redfish/v1/Chassis/1/ThermalSubsystem/Fans",
			"Members": [{"@odata.id": "/redfish/v1/Chassis/1/ThermalSubsystem/Fans/FAN1"}],
			"Members@odata.count": 1
		}`,
		"/redfish/v1/Chassis/1/ThermalSubsystem/Fans/FAN1": `{
			"@odata.id": "/redfish/v1/Chassis/1/ThermalSubsystem/Fans/FAN1",
			"Id": "FAN1",
			"Name": "Fan 1",
			"PhysicalContext": "SystemBoard",
			"SpeedPercent": {"Reading": 60, "DataSourceUri": "/redfish/v1/Chassis/1/Sensors/Fan1Pct", "SpeedRPM": 2400},
			"Status": {"State": "Enabled", "Health": "OK"}
		}`,
		"/redfish/v1/Chassis/1/ThermalSubsystem/ThermalMetrics": `{
			"@odata.id": "/redfish/v1/Chassis/1/ThermalSubsystem/ThermalMetrics",
			"TemperatureReadingsCelsius": [],
			"TemperatureSummaryCelsius": {}
		}`,
		"/redfish/v1/Chassis/1/EnvironmentMetrics": `{
			"@odata.id": "/redfish/v1/Chassis/1/EnvironmentMetrics",
			"TemperatureCelsius": {"Reading": 25, "DataSourceUri": "/redfish/v1/Chassis/1/Sensors/InletTemp"},
			"HumidityPercent": {"Reading": 41, "DataSourceUri": "/redfish/v1/Chassis/1/Sensors/Humidity1"},
			"PowerWatts": {"Reading": 250, "DataSourceUri": "/redfish/v1/Chassis/1/Sensors/TotalPower"}
		}`,
	}
}

func TestThermalCollector_ModernOnly(t *testing.T) {
	r := defaultResponses()
	r.chassisItem = modernChassisItem
	r.extraRoutes = modernThermalRoutes()
	srv := newTestServer(t, r)
	defer srv.Close()
	c := NewThermalCollector(srv.URL, newTestGoFishClient(t, srv.URL), discardLogger())

	families := gatherFamilies(t, c)
	base := map[string]string{"target": srv.URL, "chassis_id": "1"}

	// Temperature from the sensor, with thresholds.
	v, ok := gaugeValue(t, families, "redfish_temperature_celsius", map[string]string{
		"target": srv.URL, "chassis_id": "1", "sensor": "CPU1 Temp", "physical_context": "CPU",
	})
	require.True(t, ok, "missing temperature reading from sensor")
	assert.Equal(t, 45.5, v)
	v, ok = gaugeValue(t, families, "redfish_temperature_health", map[string]string{
		"target": srv.URL, "chassis_id": "1", "sensor": "CPU1 Temp", "physical_context": "CPU", "health": "OK",
	})
	require.True(t, ok, "missing temperature health")
	assert.Equal(t, 1.0, v)
	for level, want := range map[string]float64{
		"upper_critical": 85, "lower_critical": -5, "upper_caution": 75,
	} {
		v, ok := gaugeValue(t, families, "redfish_temperature_threshold_celsius", map[string]string{
			"target": srv.URL, "chassis_id": "1", "sensor": "CPU1 Temp", "physical_context": "CPU", "level": level,
		})
		require.True(t, ok, "missing threshold %s", level)
		assert.Equal(t, want, v)
	}

	// Fan RPM from the rotational sensor with its threshold.
	v, ok = gaugeValue(t, families, "redfish_fan_rpm", map[string]string{
		"target": srv.URL, "chassis_id": "1", "fan": "Fan 1", "physical_context": "SystemBoard", "unit": "RPM",
	})
	require.True(t, ok, "missing fan rpm from rotational sensor")
	assert.Equal(t, 2400.0, v)
	v, ok = gaugeValue(t, families, "redfish_fan_threshold_rpm", map[string]string{
		"target": srv.URL, "chassis_id": "1", "fan": "Fan 1", "physical_context": "SystemBoard", "level": "lower_critical",
	})
	require.True(t, ok, "missing fan threshold")
	assert.Equal(t, 500.0, v)

	// Fan health comes from the fan resource, the canonical source. The
	// rotational sensor's own health must not add a second series.
	v, ok = gaugeValue(t, families, "redfish_fan_health", map[string]string{
		"target": srv.URL, "chassis_id": "1", "fan": "Fan 1", "physical_context": "SystemBoard", "health": "OK",
	})
	require.True(t, ok, "missing fan health from fan resource")
	assert.Equal(t, 1.0, v)
	assert.Equal(t, 1, len(families["redfish_fan_health"].GetMetric()), "fan health emitted more than once")

	// Fan speed percent from the fan resource excerpt.
	v, ok = gaugeValue(t, families, "redfish_fan_speed_percent", map[string]string{
		"target": srv.URL, "chassis_id": "1", "fan": "Fan 1", "physical_context": "SystemBoard",
	})
	require.True(t, ok, "missing fan speed percent")
	assert.Equal(t, 60.0, v)

	// Humidity from the humidity sensor with its threshold.
	v, ok = gaugeValue(t, families, "redfish_humidity_percent", map[string]string{
		"target": srv.URL, "chassis_id": "1", "sensor": "Humidity 1", "physical_context": "SystemBoard",
	})
	require.True(t, ok, "missing humidity reading")
	assert.Equal(t, 41.0, v)
	v, ok = gaugeValue(t, families, "redfish_humidity_threshold_percent", map[string]string{
		"target": srv.URL, "chassis_id": "1", "sensor": "Humidity 1", "physical_context": "SystemBoard", "level": "upper_critical",
	})
	require.True(t, ok, "missing humidity threshold")
	assert.Equal(t, 80.0, v)

	// Thermal subsystem health.
	v, ok = gaugeValue(t, families, "redfish_thermal_subsystem_health", map[string]string{
		"target": srv.URL, "chassis_id": "1", "health": "OK",
	})
	require.True(t, ok, "missing thermal subsystem health")
	assert.Equal(t, 1.0, v)

	// No legacy metrics: the chassis links no Thermal resource.
	for _, absent := range []string{"redfish_power_state"} {
		_, ok := families[absent]
		assert.False(t, ok, "%s must not be emitted by the thermal collector", absent)
	}
	_ = base
}

func TestThermalCollector_HybridSensorsWin(t *testing.T) {
	r := defaultResponses()
	// Legacy Thermal with a temperature and fan, plus modern sensors for the
	// same physical sensors under different names.
	r.chassisItem = `{
		"@odata.id": "/redfish/v1/Chassis/1",
		"Id": "1",
		"Thermal": {"@odata.id": "/redfish/v1/Chassis/1/Thermal"},
		"Sensors": {"@odata.id": "/redfish/v1/Chassis/1/Sensors"}
	}`
	r.extraRoutes = map[string]string{
		"/redfish/v1/Chassis/1/Sensors":          sensorsCollectionJSON("CPU1Temp", "Fan1"),
		"/redfish/v1/Chassis/1/Sensors/CPU1Temp": sensorJSON("CPU1Temp", "CPU1Temp", "Temperature", ptr(45.5), "CPU", "Enabled", "OK", ""),
		"/redfish/v1/Chassis/1/Sensors/Fan1":     sensorJSON("Fan1", "Fan1", "Rotational", ptr(2400), "SystemBoard", "Enabled", "OK", ""),
	}
	srv := newTestServer(t, r)
	defer srv.Close()
	c := NewThermalCollector(srv.URL, newTestGoFishClient(t, srv.URL), discardLogger())

	families := gatherFamilies(t, c)

	// Sensors are the modern source and win; the legacy representations of
	// the same physical sensors must not add duplicate series.
	_, ok := gaugeValue(t, families, "redfish_temperature_celsius", map[string]string{
		"target": srv.URL, "chassis_id": "1", "sensor": "CPU1Temp", "physical_context": "CPU",
	})
	assert.True(t, ok, "missing sensor-sourced temperature")
	_, ok = gaugeValue(t, families, "redfish_temperature_celsius", map[string]string{
		"target": srv.URL, "chassis_id": "1", "sensor": "CPU1 Temp", "physical_context": "CPU",
	})
	assert.False(t, ok, "legacy temperature must be suppressed when sensors win")
	assert.Equal(t, 1, len(families["redfish_temperature_celsius"].GetMetric()))

	_, ok = gaugeValue(t, families, "redfish_fan_rpm", map[string]string{
		"target": srv.URL, "chassis_id": "1", "fan": "Fan1", "physical_context": "SystemBoard", "unit": "RPM",
	})
	assert.True(t, ok, "missing sensor-sourced fan rpm")
	assert.Equal(t, 1, len(families["redfish_fan_rpm"].GetMetric()), "fan rpm emitted more than once")

	// No fan resources exist, so fan health falls through to the sensors.
	_, ok = gaugeValue(t, families, "redfish_fan_health", map[string]string{
		"target": srv.URL, "chassis_id": "1", "fan": "Fan1", "physical_context": "SystemBoard", "health": "OK",
	})
	assert.True(t, ok, "missing sensor-sourced fan health")
}

func TestThermalCollector_HybridLegacyEmptyFallsBackToSensors(t *testing.T) {
	r := defaultResponses()
	r.chassisItem = `{
		"@odata.id": "/redfish/v1/Chassis/1",
		"Id": "1",
		"Thermal": {"@odata.id": "/redfish/v1/Chassis/1/Thermal"},
		"Sensors": {"@odata.id": "/redfish/v1/Chassis/1/Sensors"}
	}`
	// An empty legacy response must not prevent sensor collection.
	r.thermal = `{
		"@odata.id": "/redfish/v1/Chassis/1/Thermal",
		"Temperatures": [],
		"Fans": []
	}`
	r.extraRoutes = map[string]string{
		"/redfish/v1/Chassis/1/Sensors":          sensorsCollectionJSON("CPU1Temp"),
		"/redfish/v1/Chassis/1/Sensors/CPU1Temp": sensorJSON("CPU1Temp", "CPU1Temp", "Temperature", ptr(45.5), "CPU", "Enabled", "OK", ""),
	}
	srv := newTestServer(t, r)
	defer srv.Close()
	c := NewThermalCollector(srv.URL, newTestGoFishClient(t, srv.URL), discardLogger())

	families := gatherFamilies(t, c)
	_, ok := gaugeValue(t, families, "redfish_temperature_celsius", map[string]string{
		"target": srv.URL, "chassis_id": "1", "sensor": "CPU1Temp", "physical_context": "CPU",
	})
	assert.True(t, ok, "empty legacy thermal must fall back to sensors")
}

func TestThermalCollector_LegacyThermalNotFoundUsesSensors(t *testing.T) {
	r := defaultResponses()
	r.chassisItem = `{
		"@odata.id": "/redfish/v1/Chassis/1",
		"Id": "1",
		"Thermal": {"@odata.id": "/redfish/v1/Chassis/1/Thermal"},
		"Sensors": {"@odata.id": "/redfish/v1/Chassis/1/Sensors"}
	}`
	r.extraRoutes = map[string]string{
		"/redfish/v1/Chassis/1/Sensors":          sensorsCollectionJSON("CPU1Temp"),
		"/redfish/v1/Chassis/1/Sensors/CPU1Temp": sensorJSON("CPU1Temp", "CPU1Temp", "Temperature", ptr(45.5), "CPU", "Enabled", "OK", ""),
	}
	r.overrides = map[string]routeOverride{
		"/redfish/v1/Chassis/1/Thermal": {status: http.StatusNotFound},
	}
	srv := newTestServer(t, r)
	defer srv.Close()
	client := newTestGoFishClient(t, srv.URL)
	c := NewThermalCollector(srv.URL, client, discardLogger())

	// A 404 on the deprecated resource is a capability gap, not a failure.
	ctx := newScrapeContext(client, discardLogger(), false)
	ch := make(chan prometheus.Metric, 16)
	err := c.collect(ctx, ch)
	close(ch)
	assert.NoError(t, err)

	families := gatherFamilies(t, c)
	_, ok := gaugeValue(t, families, "redfish_temperature_celsius", map[string]string{
		"target": srv.URL, "chassis_id": "1", "sensor": "CPU1Temp", "physical_context": "CPU",
	})
	assert.True(t, ok, "sensors must be collected when legacy thermal 404s")
}

func TestThermalCollector_PercentNeverExportedAsRPM(t *testing.T) {
	r := defaultResponses()
	// A legacy fan reporting percent units and a fan resource with only a
	// percent reading: neither may surface as fan_rpm.
	r.thermal = `{
		"@odata.id": "/redfish/v1/Chassis/1/Thermal",
		"Temperatures": [],
		"Fans": [{
			"MemberId": "0",
			"Name": "Fan1",
			"Reading": 60,
			"ReadingUnits": "Percent",
			"PhysicalContext": "SystemBoard",
			"Status": {"Health": "OK", "State": "Enabled"}
		}]
	}`
	r.chassisItem = `{
		"@odata.id": "/redfish/v1/Chassis/1",
		"Id": "1",
		"Thermal": {"@odata.id": "/redfish/v1/Chassis/1/Thermal"},
		"ThermalSubsystem": {"@odata.id": "/redfish/v1/Chassis/1/ThermalSubsystem"}
	}`
	r.extraRoutes = map[string]string{
		"/redfish/v1/Chassis/1/ThermalSubsystem": `{
			"@odata.id": "/redfish/v1/Chassis/1/ThermalSubsystem",
			"Id": "ThermalSubsystem",
			"Status": {"State": "Enabled", "Health": "OK"},
			"Fans": {"@odata.id": "/redfish/v1/Chassis/1/ThermalSubsystem/Fans"}
		}`,
		"/redfish/v1/Chassis/1/ThermalSubsystem/Fans": `{
			"@odata.id": "/redfish/v1/Chassis/1/ThermalSubsystem/Fans",
			"Members": [{"@odata.id": "/redfish/v1/Chassis/1/ThermalSubsystem/Fans/FAN1"}],
			"Members@odata.count": 1
		}`,
		"/redfish/v1/Chassis/1/ThermalSubsystem/Fans/FAN1": `{
			"@odata.id": "/redfish/v1/Chassis/1/ThermalSubsystem/Fans/FAN1",
			"Id": "FAN1",
			"Name": "Fan 1",
			"PhysicalContext": "SystemBoard",
			"SpeedPercent": {"Reading": 55},
			"Status": {"State": "Enabled", "Health": "OK"}
		}`,
	}
	srv := newTestServer(t, r)
	defer srv.Close()
	c := NewThermalCollector(srv.URL, newTestGoFishClient(t, srv.URL), discardLogger())

	families := gatherFamilies(t, c)
	assert.Nil(t, families["redfish_fan_rpm"], "no RPM source exists; fan_rpm must not be emitted")

	v, ok := gaugeValue(t, families, "redfish_fan_speed_percent", map[string]string{
		"target": srv.URL, "chassis_id": "1", "fan": "Fan 1", "physical_context": "SystemBoard",
	})
	require.True(t, ok, "fan resource percent must be emitted")
	assert.Equal(t, 55.0, v)

	// The legacy percent fan is suppressed by the fan resource percent
	// source; only one percent series exists.
	assert.Equal(t, 1, len(families["redfish_fan_speed_percent"].GetMetric()))
}

func TestThermalCollector_PreservesZeroAndNegativeReadings(t *testing.T) {
	r := defaultResponses()
	r.chassisItem = `{
		"@odata.id": "/redfish/v1/Chassis/1",
		"Id": "1",
		"Sensors": {"@odata.id": "/redfish/v1/Chassis/1/Sensors"}
	}`
	r.extraRoutes = map[string]string{
		"/redfish/v1/Chassis/1/Sensors":              sensorsCollectionJSON("ZeroTemp", "NegativeTemp", "NullTemp", "AbsentTemp"),
		"/redfish/v1/Chassis/1/Sensors/ZeroTemp":     sensorJSON("ZeroTemp", "Zero Temp", "Temperature", ptr(0), "CPU", "Enabled", "OK", ""),
		"/redfish/v1/Chassis/1/Sensors/NegativeTemp": sensorJSON("NegativeTemp", "Negative Temp", "Temperature", ptr(-5.5), "CPU", "Enabled", "OK", ""),
		"/redfish/v1/Chassis/1/Sensors/NullTemp":     sensorJSON("NullTemp", "Null Temp", "Temperature", nil, "CPU", "Enabled", "OK", ""),
		"/redfish/v1/Chassis/1/Sensors/AbsentTemp":   sensorJSON("AbsentTemp", "Absent Temp", "Temperature", ptr(30), "CPU", "Absent", "OK", ""),
	}
	srv := newTestServer(t, r)
	defer srv.Close()
	c := NewThermalCollector(srv.URL, newTestGoFishClient(t, srv.URL), discardLogger())

	families := gatherFamilies(t, c)
	v, ok := gaugeValue(t, families, "redfish_temperature_celsius", map[string]string{
		"target": srv.URL, "chassis_id": "1", "sensor": "Zero Temp", "physical_context": "CPU",
	})
	require.True(t, ok, "real zero reading must be preserved")
	assert.Equal(t, 0.0, v)

	v, ok = gaugeValue(t, families, "redfish_temperature_celsius", map[string]string{
		"target": srv.URL, "chassis_id": "1", "sensor": "Negative Temp", "physical_context": "CPU",
	})
	require.True(t, ok, "negative reading must be preserved")
	assert.Equal(t, -5.5, v)

	for _, sensor := range []string{"Null Temp", "Absent Temp"} {
		_, ok := gaugeValue(t, families, "redfish_temperature_celsius", map[string]string{
			"target": srv.URL, "chassis_id": "1", "sensor": sensor, "physical_context": "CPU",
		})
		assert.False(t, ok, "%s must be skipped", sensor)
	}
}

func TestThermalCollector_SameNameSensorsDisambiguated(t *testing.T) {
	r := defaultResponses()
	r.chassisItem = `{
		"@odata.id": "/redfish/v1/Chassis/1",
		"Id": "1",
		"Sensors": {"@odata.id": "/redfish/v1/Chassis/1/Sensors"}
	}`
	r.extraRoutes = map[string]string{
		"/redfish/v1/Chassis/1/Sensors":       sensorsCollectionJSON("TempA", "TempB"),
		"/redfish/v1/Chassis/1/Sensors/TempA": sensorJSON("TempA", "CPU Temp", "Temperature", ptr(40), "CPU", "Enabled", "OK", ""),
		"/redfish/v1/Chassis/1/Sensors/TempB": sensorJSON("TempB", "CPU Temp", "Temperature", ptr(41), "CPU", "Enabled", "OK", ""),
	}
	srv := newTestServer(t, r)
	defer srv.Close()
	c := NewThermalCollector(srv.URL, newTestGoFishClient(t, srv.URL), discardLogger())

	families := gatherFamilies(t, c)
	f := families["redfish_temperature_celsius"]
	require.NotNil(t, f)
	// Both distinct sensors are preserved; neither is silently merged.
	assert.Equal(t, 2, len(f.GetMetric()))
	v, ok := gaugeValue(t, families, "redfish_temperature_celsius", map[string]string{
		"target": srv.URL, "chassis_id": "1", "sensor": "CPU Temp", "physical_context": "CPU",
	})
	require.True(t, ok)
	assert.Equal(t, 40.0, v)
	v, ok = gaugeValue(t, families, "redfish_temperature_celsius", map[string]string{
		"target": srv.URL, "chassis_id": "1", "sensor": "CPU Temp (TempB)", "physical_context": "CPU",
	})
	require.True(t, ok, "second sensor with the same name must be disambiguated")
	assert.Equal(t, 41.0, v)
}

func TestThermalCollector_ThermalMetricsExcerptDedup(t *testing.T) {
	r := defaultResponses()
	r.chassisItem = `{
		"@odata.id": "/redfish/v1/Chassis/1",
		"Id": "1",
		"Sensors": {"@odata.id": "/redfish/v1/Chassis/1/Sensors"},
		"ThermalSubsystem": {"@odata.id": "/redfish/v1/Chassis/1/ThermalSubsystem"}
	}`
	// Only a humidity sensor: no temperature sensors, so the thermal metrics
	// excerpts are the temperature source.
	r.extraRoutes = map[string]string{
		"/redfish/v1/Chassis/1/Sensors":           sensorsCollectionJSON("Humidity1"),
		"/redfish/v1/Chassis/1/Sensors/Humidity1": sensorJSON("Humidity1", "Humidity 1", "Humidity", ptr(41), "SystemBoard", "Enabled", "OK", ""),
		"/redfish/v1/Chassis/1/ThermalSubsystem": `{
			"@odata.id": "/redfish/v1/Chassis/1/ThermalSubsystem",
			"Id": "ThermalSubsystem",
			"Status": {"State": "Enabled", "Health": "OK"},
			"ThermalMetrics": {"@odata.id": "/redfish/v1/Chassis/1/ThermalSubsystem/ThermalMetrics"}
		}`,
		// The intake summary repeats the first reading's sensor via
		// DataSourceUri and must be skipped; exhaust references a distinct
		// sensor and must be emitted.
		"/redfish/v1/Chassis/1/ThermalSubsystem/ThermalMetrics": `{
			"@odata.id": "/redfish/v1/Chassis/1/ThermalSubsystem/ThermalMetrics",
			"TemperatureReadingsCelsius": [
				{"DeviceName": "Inlet", "DataSourceUri": "/redfish/v1/Chassis/1/Sensors/InletTemp", "PhysicalContext": "Intake", "Reading": 22},
				{"DeviceName": "PCH", "DataSourceUri": "/redfish/v1/Chassis/1/Sensors/PCHTemp", "PhysicalContext": "SystemBoard", "Reading": 50}
			],
			"TemperatureSummaryCelsius": {
				"Intake": {"DataSourceUri": "/redfish/v1/Chassis/1/Sensors/InletTemp", "Reading": 22},
				"Exhaust": {"DataSourceUri": "/redfish/v1/Chassis/1/Sensors/ExhaustTemp", "Reading": 35}
			}
		}`,
	}
	srv := newTestServer(t, r)
	defer srv.Close()
	c := NewThermalCollector(srv.URL, newTestGoFishClient(t, srv.URL), discardLogger())

	families := gatherFamilies(t, c)
	f := families["redfish_temperature_celsius"]
	require.NotNil(t, f)
	// Inlet, PCH, and exhaust: three distinct readings, no duplicates.
	assert.Equal(t, 3, len(f.GetMetric()))
	v, ok := gaugeValue(t, families, "redfish_temperature_celsius", map[string]string{
		"target": srv.URL, "chassis_id": "1", "sensor": "Inlet", "physical_context": "Intake",
	})
	require.True(t, ok)
	assert.Equal(t, 22.0, v)
	v, ok = gaugeValue(t, families, "redfish_temperature_celsius", map[string]string{
		"target": srv.URL, "chassis_id": "1", "sensor": "Exhaust", "physical_context": "",
	})
	require.True(t, ok, "excerpt with an unknown DataSourceUri must be emitted")
	assert.Equal(t, 35.0, v)
}

func TestThermalCollector_EnvironmentHumidityResolvesSensorName(t *testing.T) {
	r := defaultResponses()
	r.chassisItem = `{
		"@odata.id": "/redfish/v1/Chassis/1",
		"Id": "1",
		"Sensors": {"@odata.id": "/redfish/v1/Chassis/1/Sensors"},
		"EnvironmentMetrics": {"@odata.id": "/redfish/v1/Chassis/1/EnvironmentMetrics"}
	}`
	// The humidity sensor exists but reports no reading; the environment
	// metrics excerpt carries the value and must reuse the sensor's name.
	r.extraRoutes = map[string]string{
		"/redfish/v1/Chassis/1/Sensors":           sensorsCollectionJSON("Humidity1"),
		"/redfish/v1/Chassis/1/Sensors/Humidity1": sensorJSON("Humidity1", "Humidity 1", "Humidity", nil, "SystemBoard", "Enabled", "OK", ""),
		"/redfish/v1/Chassis/1/EnvironmentMetrics": `{
			"@odata.id": "/redfish/v1/Chassis/1/EnvironmentMetrics",
			"HumidityPercent": {"Reading": 41, "DataSourceUri": "/redfish/v1/Chassis/1/Sensors/Humidity1"}
		}`,
	}
	srv := newTestServer(t, r)
	defer srv.Close()
	c := NewThermalCollector(srv.URL, newTestGoFishClient(t, srv.URL), discardLogger())

	families := gatherFamilies(t, c)
	v, ok := gaugeValue(t, families, "redfish_humidity_percent", map[string]string{
		"target": srv.URL, "chassis_id": "1", "sensor": "Humidity 1", "physical_context": "",
	})
	require.True(t, ok, "humidity excerpt must resolve the sensor name via DataSourceUri")
	assert.Equal(t, 41.0, v)
	assert.Equal(t, 1, len(families["redfish_humidity_percent"].GetMetric()))
}

func TestThermalCollector_SensorsFailureDegradesButLegacyCollected(t *testing.T) {
	r := defaultResponses()
	r.chassisItem = `{
		"@odata.id": "/redfish/v1/Chassis/1",
		"Id": "1",
		"Thermal": {"@odata.id": "/redfish/v1/Chassis/1/Thermal"},
		"Sensors": {"@odata.id": "/redfish/v1/Chassis/1/Sensors"}
	}`
	r.overrides = map[string]routeOverride{
		"/redfish/v1/Chassis/1/Sensors": {status: http.StatusInternalServerError},
	}
	srv := newTestServer(t, r)
	defer srv.Close()
	client := newTestGoFishClient(t, srv.URL)
	c := NewThermalCollector(srv.URL, client, discardLogger())

	ctx := newScrapeContext(client, discardLogger(), false)
	ch := make(chan prometheus.Metric, 16)
	err := c.collect(ctx, ch)
	close(ch)

	// The failure is surfaced (redfish_collector_success=0) ...
	assert.Error(t, err, "a failing sensors collection must degrade the scrape")
	// ... but the legacy thermal resource is still collected.
	count := 0
	for range ch {
		count++
	}
	assert.Equal(t, 4, count, "legacy temperature and fan metrics must still be emitted")
}

func TestThermalCollector_PaginatedSensorCollection(t *testing.T) {
	r := defaultResponses()
	r.chassisItem = `{
		"@odata.id": "/redfish/v1/Chassis/1",
		"Id": "1",
		"Sensors": {"@odata.id": "/redfish/v1/Chassis/1/Sensors"}
	}`
	r.extraRoutes = map[string]string{
		// Page one lists one member and points at page two via nextLink.
		"/redfish/v1/Chassis/1/Sensors": `{
			"@odata.id": "/redfish/v1/Chassis/1/Sensors",
			"Members": [{"@odata.id": "/redfish/v1/Chassis/1/Sensors/CPU1Temp"}],
			"Members@odata.count": 2,
			"Members@odata.nextLink": "/redfish/v1/Chassis/1/Sensors/page-2"
		}`,
		"/redfish/v1/Chassis/1/Sensors/page-2": `{
			"@odata.id": "/redfish/v1/Chassis/1/Sensors/page-2",
			"Members": [{"@odata.id": "/redfish/v1/Chassis/1/Sensors/CPU2Temp"}],
			"Members@odata.count": 1
		}`,
		"/redfish/v1/Chassis/1/Sensors/CPU1Temp": sensorJSON("CPU1Temp", "CPU1 Temp", "Temperature", ptr(45), "CPU", "Enabled", "OK", ""),
		"/redfish/v1/Chassis/1/Sensors/CPU2Temp": sensorJSON("CPU2Temp", "CPU2 Temp", "Temperature", ptr(46), "CPU", "Enabled", "OK", ""),
	}
	srv := newTestServer(t, r)
	defer srv.Close()
	c := NewThermalCollector(srv.URL, newTestGoFishClient(t, srv.URL), discardLogger())

	families := gatherFamilies(t, c)
	f := families["redfish_temperature_celsius"]
	require.NotNil(t, f)
	assert.Equal(t, 2, len(f.GetMetric()), "sensors from both collection pages must be collected")
}

func TestThermalCollector_MultiChassisMixedSources(t *testing.T) {
	r := defaultResponses()
	// Chassis 1 keeps the legacy default fixture; chassis 2 is modern-only.
	r.chassis = `{
		"@odata.id": "/redfish/v1/Chassis",
		"Members": [
			{"@odata.id": "/redfish/v1/Chassis/1"},
			{"@odata.id": "/redfish/v1/Chassis/2"}
		],
		"Members@odata.count": 2
	}`
	extra := map[string]string{
		"/redfish/v1/Chassis/2": `{
			"@odata.id": "/redfish/v1/Chassis/2",
			"Id": "2",
			"Sensors": {"@odata.id": "/redfish/v1/Chassis/2/Sensors"}
		}`,
		"/redfish/v1/Chassis/2/Sensors": `{
			"@odata.id": "/redfish/v1/Chassis/2/Sensors",
			"Members": [{"@odata.id": "/redfish/v1/Chassis/2/Sensors/CPU1Temp"}],
			"Members@odata.count": 1
		}`,
		"/redfish/v1/Chassis/2/Sensors/CPU1Temp": `{
			"@odata.id": "/redfish/v1/Chassis/2/Sensors/CPU1Temp",
			"Id": "CPU1Temp",
			"Name": "CPU1Temp",
			"ReadingType": "Temperature",
			"Reading": 50,
			"PhysicalContext": "CPU",
			"Status": {"State": "Enabled", "Health": "OK"}
		}`,
	}
	r.extraRoutes = extra
	srv := newTestServer(t, r)
	defer srv.Close()
	c := NewThermalCollector(srv.URL, newTestGoFishClient(t, srv.URL), discardLogger())

	families := gatherFamilies(t, c)
	// Each chassis selects its own source: legacy on 1, sensors on 2.
	_, ok := gaugeValue(t, families, "redfish_temperature_celsius", map[string]string{
		"target": srv.URL, "chassis_id": "1", "sensor": "CPU1 Temp", "physical_context": "CPU",
	})
	assert.True(t, ok, "legacy chassis must keep its legacy reading")
	_, ok = gaugeValue(t, families, "redfish_temperature_celsius", map[string]string{
		"target": srv.URL, "chassis_id": "2", "sensor": "CPU1Temp", "physical_context": "CPU",
	})
	assert.True(t, ok, "modern chassis must use its sensor reading")
}

func TestThermalCollector_RepeatedScrapesSeeFreshValues(t *testing.T) {
	var mu sync.Mutex
	reading := 45.0
	mux := http.NewServeMux()
	mux.HandleFunc("/redfish/v1/", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"@odata.id": "/redfish/v1/", "Id": "RootService", "Name": "Root Service", "Chassis": {"@odata.id": "/redfish/v1/Chassis"}}`)
	})
	mux.HandleFunc("/redfish/v1/Chassis", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"@odata.id": "/redfish/v1/Chassis", "Members": [{"@odata.id": "/redfish/v1/Chassis/1"}], "Members@odata.count": 1}`)
	})
	mux.HandleFunc("/redfish/v1/Chassis/1", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"@odata.id": "/redfish/v1/Chassis/1", "Id": "1", "Sensors": {"@odata.id": "/redfish/v1/Chassis/1/Sensors"}}`)
	})
	mux.HandleFunc("/redfish/v1/Chassis/1/Sensors", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"@odata.id": "/redfish/v1/Chassis/1/Sensors", "Members": [{"@odata.id": "/redfish/v1/Chassis/1/Sensors/CPU1Temp"}], "Members@odata.count": 1}`)
	})
	mux.HandleFunc("/redfish/v1/Chassis/1/Sensors/CPU1Temp", func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		body := fmt.Sprintf(`{"@odata.id": "/redfish/v1/Chassis/1/Sensors/CPU1Temp", "Id": "CPU1Temp", "Name": "CPU1 Temp", "ReadingType": "Temperature", "Reading": %s, "PhysicalContext": "CPU", "Status": {"State": "Enabled", "Health": "OK"}}`, strconv.FormatFloat(reading, 'f', -1, 64))
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, body)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	client := newTestGoFishClient(t, srv.URL)
	c := NewThermalCollector(srv.URL, client, discardLogger())

	first := gatherFamilies(t, c)
	v, ok := gaugeValue(t, first, "redfish_temperature_celsius", map[string]string{
		"target": srv.URL, "chassis_id": "1", "sensor": "CPU1 Temp", "physical_context": "CPU",
	})
	require.True(t, ok)
	assert.Equal(t, 45.0, v)

	mu.Lock()
	reading = 47.5
	mu.Unlock()

	second := gatherFamilies(t, c)
	v, ok = gaugeValue(t, second, "redfish_temperature_celsius", map[string]string{
		"target": srv.URL, "chassis_id": "1", "sensor": "CPU1 Temp", "physical_context": "CPU",
	})
	require.True(t, ok)
	assert.Equal(t, 47.5, v, "a new scrape must fetch fresh values, not cached ones")
}

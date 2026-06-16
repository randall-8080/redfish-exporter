package collectors

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
)

func TestThermalCollector_Collect_HappyPath(t *testing.T) {
	srv := newTestServer(t, defaultResponses())
	defer srv.Close()
	c := NewThermalCollector(srv.URL, newTestGoFishClient(t, srv.URL), discardLogger())

	// temp_celsius(1) + temp_status(1) + fan_rpm(1) + fan_status(1) = 4
	assert.Equal(t, 4, testutil.CollectAndCount(c))
}

func TestThermalCollector_Collect_SkipsAbsentTemperature(t *testing.T) {
	r := defaultResponses()
	r.thermal = `{
		"@odata.id": "/redfish/v1/Chassis/1/Thermal",
		"Temperatures": [{"MemberId": "0", "Name": "CPU Temp", "ReadingCelsius": 0.0, "Status": {"Health": "OK", "State": "Absent"}}],
		"Fans": []
	}`
	srv := newTestServer(t, r)
	defer srv.Close()
	c := NewThermalCollector(srv.URL, newTestGoFishClient(t, srv.URL), discardLogger())

	assert.Equal(t, 0, testutil.CollectAndCount(c))
}

func TestThermalCollector_Collect_SkipsDisabledTemperature(t *testing.T) {
	r := defaultResponses()
	r.thermal = `{
		"@odata.id": "/redfish/v1/Chassis/1/Thermal",
		"Temperatures": [{"MemberId": "0", "Name": "CPU Temp", "ReadingCelsius": 45.0, "Status": {"Health": "OK", "State": "Disabled"}}],
		"Fans": []
	}`
	srv := newTestServer(t, r)
	defer srv.Close()
	c := NewThermalCollector(srv.URL, newTestGoFishClient(t, srv.URL), discardLogger())

	assert.Equal(t, 0, testutil.CollectAndCount(c))
}

func TestThermalCollector_Collect_SkipsNilReadingCelsius(t *testing.T) {
	r := defaultResponses()
	r.thermal = `{
		"@odata.id": "/redfish/v1/Chassis/1/Thermal",
		"Temperatures": [{"MemberId": "0", "Name": "CPU Temp", "Status": {"Health": "OK", "State": "Enabled"}}],
		"Fans": []
	}`
	srv := newTestServer(t, r)
	defer srv.Close()
	c := NewThermalCollector(srv.URL, newTestGoFishClient(t, srv.URL), discardLogger())

	assert.Equal(t, 0, testutil.CollectAndCount(c))
}

func TestThermalCollector_Collect_SkipsAbsentFan(t *testing.T) {
	r := defaultResponses()
	r.thermal = `{
		"@odata.id": "/redfish/v1/Chassis/1/Thermal",
		"Temperatures": [],
		"Fans": [{"MemberId": "0", "Name": "Fan1", "Reading": 2400, "Status": {"Health": "OK", "State": "Absent"}}]
	}`
	srv := newTestServer(t, r)
	defer srv.Close()
	c := NewThermalCollector(srv.URL, newTestGoFishClient(t, srv.URL), discardLogger())

	assert.Equal(t, 0, testutil.CollectAndCount(c))
}

func TestThermalCollector_Collect_SkipsFanWithNoName(t *testing.T) {
	r := defaultResponses()
	r.thermal = `{
		"@odata.id": "/redfish/v1/Chassis/1/Thermal",
		"Temperatures": [],
		"Fans": [{"MemberId": "0", "Name": "", "Reading": 2400, "Status": {"Health": "OK", "State": "Enabled"}}]
	}`
	srv := newTestServer(t, r)
	defer srv.Close()
	c := NewThermalCollector(srv.URL, newTestGoFishClient(t, srv.URL), discardLogger())

	assert.Equal(t, 0, testutil.CollectAndCount(c))
}

func TestThermalCollector_Collect_FanNilReadingEmitsHealth(t *testing.T) {
	r := defaultResponses()
	r.thermal = `{
		"@odata.id": "/redfish/v1/Chassis/1/Thermal",
		"Temperatures": [],
		"Fans": [{"MemberId": "0", "Name": "Fan1", "Status": {"Health": "Critical", "State": "Enabled"}}]
	}`
	srv := newTestServer(t, r)
	defer srv.Close()
	c := NewThermalCollector(srv.URL, newTestGoFishClient(t, srv.URL), discardLogger())

	// nil Reading → no fan_rpm; Enabled+Critical → fan_health(0) still emitted
	assert.Equal(t, 1, testutil.CollectAndCount(c))
}

func TestThermalCollector_Collect_NilThermalResource(t *testing.T) {
	r := defaultResponses()
	// chassis item without a Thermal link → chassis.Thermal() returns (nil, nil)
	r.chassisItem = `{
		"@odata.id": "/redfish/v1/Chassis/1",
		"Id": "1",
		"PowerState": "On",
		"Power": {"@odata.id": "/redfish/v1/Chassis/1/Power"}
	}`
	srv := newTestServer(t, r)
	defer srv.Close()
	c := NewThermalCollector(srv.URL, newTestGoFishClient(t, srv.URL), discardLogger())

	// must not panic; nil guard skips the chassis silently
	assert.Equal(t, 0, testutil.CollectAndCount(c))
}

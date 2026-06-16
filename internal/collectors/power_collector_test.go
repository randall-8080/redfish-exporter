package collectors

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
)

func TestPowerCollector_Collect_HappyPath(t *testing.T) {
	srv := newTestServer(t, defaultResponses())
	defer srv.Close()
	c := NewPowerCollector(srv.URL, newTestGoFishClient(t, srv.URL), discardLogger())

	// powerState(1) + chassis_health(1) + chassis_health_rollup(1) + control_health(1) +
	// capacity(1) + consumed(1) + avg(1) + max(1) + min(1) + supply_health(1) = 10
	assert.Equal(t, 10, testutil.CollectAndCount(c))
}

func TestPowerCollector_Collect_SkipsNilOptionalFields(t *testing.T) {
	r := defaultResponses()
	r.power = `{
		"@odata.id": "/redfish/v1/Chassis/1/Power",
		"PowerControl": [{"MemberId": "0", "Name": "Sys"}],
		"Redundancy": [],
		"PowerSupplies": [{"MemberId": "0", "Name": "PSU1", "Status": {"Health": "OK", "State": "Enabled"}}]
	}`
	srv := newTestServer(t, r)
	defer srv.Close()
	c := NewPowerCollector(srv.URL, newTestGoFishClient(t, srv.URL), discardLogger())

	// powerState(1) + chassis_health(1) + chassis_health_rollup(1) + supply_health(1) = 4;
	// capacity/consumed/metrics omitted (nil); PowerControl has no Status → no control_health
	assert.Equal(t, 4, testutil.CollectAndCount(c))
}

func TestPowerCollector_Collect_PowerStateOff(t *testing.T) {
	r := defaultResponses()
	r.chassisItem = `{
		"@odata.id": "/redfish/v1/Chassis/1",
		"Id": "1",
		"PowerState": "Off",
		"Power":   {"@odata.id": "/redfish/v1/Chassis/1/Power"},
		"Thermal": {"@odata.id": "/redfish/v1/Chassis/1/Thermal"}
	}`
	srv := newTestServer(t, r)
	defer srv.Close()
	c := NewPowerCollector(srv.URL, newTestGoFishClient(t, srv.URL), discardLogger())

	// chassisItem override has no Status → no chassis health metrics;
	// default power has PowerControl with Status → control_health(1)
	// powerState(1) + control_health(1) + capacity(1) + consumed(1) + avg(1) + max(1) + min(1) + supply_health(1) = 8
	assert.Equal(t, 8, testutil.CollectAndCount(c))
}

func TestPowerCollector_Collect_SupplyWarningHealth(t *testing.T) {
	r := defaultResponses()
	r.power = `{
		"@odata.id": "/redfish/v1/Chassis/1/Power",
		"PowerControl": [],
		"Redundancy": [],
		"PowerSupplies": [{"MemberId": "0", "Name": "PSU1", "Status": {"Health": "Warning", "State": "Enabled"}}]
	}`
	srv := newTestServer(t, r)
	defer srv.Close()
	c := NewPowerCollector(srv.URL, newTestGoFishClient(t, srv.URL), discardLogger())

	// powerState(1) + chassis_health(1) + chassis_health_rollup(1) + supply_health(1, value=0.0 for Warning) = 4
	assert.Equal(t, 4, testutil.CollectAndCount(c))
}

func TestPowerCollector_Collect_SkipsUnnamedSupply(t *testing.T) {
	r := defaultResponses()
	r.power = `{
		"@odata.id": "/redfish/v1/Chassis/1/Power",
		"PowerControl": [],
		"Redundancy": [],
		"PowerSupplies": [
			{"MemberId": "0", "Name": "", "Status": {"Health": "OK", "State": "Enabled"}},
			{"MemberId": "1", "Name": "", "Status": {"Health": "OK", "State": "Enabled"}}
		]
	}`
	srv := newTestServer(t, r)
	defer srv.Close()
	c := NewPowerCollector(srv.URL, newTestGoFishClient(t, srv.URL), discardLogger())

	// both unnamed PSUs skipped; powerState(1) + chassis_health(1) + chassis_health_rollup(1) = 3
	assert.Equal(t, 3, testutil.CollectAndCount(c))
}

func TestPowerCollector_Collect_ChassisHealthCritical(t *testing.T) {
	r := defaultResponses()
	r.chassisItem = `{
		"@odata.id": "/redfish/v1/Chassis/1",
		"Id": "1",
		"PowerState": "On",
		"Status": {"Health": "Critical", "HealthRollup": "Critical"},
		"Power": {"@odata.id": "/redfish/v1/Chassis/1/Power"}
	}`
	r.power = `{
		"@odata.id": "/redfish/v1/Chassis/1/Power",
		"PowerControl": [], "Redundancy": [], "PowerSupplies": []
	}`
	srv := newTestServer(t, r)
	defer srv.Close()
	c := NewPowerCollector(srv.URL, newTestGoFishClient(t, srv.URL), discardLogger())

	// powerState(1) + chassis_health(0) + chassis_health_rollup(0) = 3; values are 0.0 for Critical
	assert.Equal(t, 3, testutil.CollectAndCount(c))
}

func TestPowerCollector_Collect_PowerControlHealthWarning(t *testing.T) {
	r := defaultResponses()
	r.power = `{
		"@odata.id": "/redfish/v1/Chassis/1/Power",
		"PowerControl": [{"MemberId": "0", "Name": "Sys", "Status": {"Health": "Warning", "State": "Enabled"}}],
		"Redundancy": [],
		"PowerSupplies": []
	}`
	srv := newTestServer(t, r)
	defer srv.Close()
	c := NewPowerCollector(srv.URL, newTestGoFishClient(t, srv.URL), discardLogger())

	// powerState(1) + chassis_health(1) + chassis_health_rollup(1) + control_health(1, value=0.0) = 4
	assert.Equal(t, 4, testutil.CollectAndCount(c))
}

func TestPowerCollector_Collect_SkipsUnnamedRedundancy(t *testing.T) {
	r := defaultResponses()
	r.power = `{
		"@odata.id": "/redfish/v1/Chassis/1/Power",
		"PowerControl": [],
		"Redundancy": [
			{"MemberId": "0", "Name": "", "Status": {"Health": "OK"}},
			{"MemberId": "1", "Name": "", "Status": {"Health": "OK"}}
		],
		"PowerSupplies": []
	}`
	srv := newTestServer(t, r)
	defer srv.Close()
	c := NewPowerCollector(srv.URL, newTestGoFishClient(t, srv.URL), discardLogger())

	// both unnamed redundancy groups skipped; powerState(1) + chassis_health(1) + chassis_health_rollup(1) = 3
	assert.Equal(t, 3, testutil.CollectAndCount(c))
}

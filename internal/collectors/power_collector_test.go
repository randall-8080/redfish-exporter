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

	// powerState(1) + capacity(1) + consumed(1) + avg(1) + max(1) + min(1) + supply_health(1) = 7
	assert.Equal(t, 7, testutil.CollectAndCount(c))
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

	// powerState(1) + supply_health(1) = 2; capacity/consumed/metrics omitted (nil)
	assert.Equal(t, 2, testutil.CollectAndCount(c))
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

	// same metric count as happy path; powerState emits 0.0 for Off state
	assert.Equal(t, 7, testutil.CollectAndCount(c))
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

	// powerState(1) + supply_health(1, value=0.5 for Warning) = 2
	assert.Equal(t, 2, testutil.CollectAndCount(c))
}

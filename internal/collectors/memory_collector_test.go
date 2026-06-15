package collectors

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
)

func TestMemoryCollector_Collect_HappyPath(t *testing.T) {
	srv := newTestServer(t, defaultResponses())
	defer srv.Close()
	c := NewMemoryCollector(srv.URL, newTestGoFishClient(t, srv.URL), discardLogger())

	// memory_capacity_mib(1) + memory_status(1) = 2
	assert.Equal(t, 2, testutil.CollectAndCount(c))
}

func TestMemoryCollector_Collect_SkipsAbsentModule(t *testing.T) {
	r := defaultResponses()
	r.memoryItem = `{
		"@odata.id": "/redfish/v1/Systems/1/Memory/1",
		"Id": "1",
		"CapacityMiB": 16384,
		"DeviceLocator": "DIMM_A1",
		"Status": {"Health": "OK", "State": "Absent"}
	}`
	srv := newTestServer(t, r)
	defer srv.Close()
	c := NewMemoryCollector(srv.URL, newTestGoFishClient(t, srv.URL), discardLogger())

	assert.Equal(t, 0, testutil.CollectAndCount(c))
}

func TestMemoryCollector_Collect_SkipsDisabledModule(t *testing.T) {
	r := defaultResponses()
	r.memoryItem = `{
		"@odata.id": "/redfish/v1/Systems/1/Memory/1",
		"Id": "1",
		"CapacityMiB": 16384,
		"DeviceLocator": "DIMM_A1",
		"Status": {"Health": "OK", "State": "Disabled"}
	}`
	srv := newTestServer(t, r)
	defer srv.Close()
	c := NewMemoryCollector(srv.URL, newTestGoFishClient(t, srv.URL), discardLogger())

	assert.Equal(t, 0, testutil.CollectAndCount(c))
}

func TestMemoryCollector_Collect_StandbySpareEmitsCapacityOnly(t *testing.T) {
	r := defaultResponses()
	r.memoryItem = `{
		"@odata.id": "/redfish/v1/Systems/1/Memory/1",
		"Id": "1",
		"CapacityMiB": 16384,
		"DeviceLocator": "DIMM_A1",
		"Status": {"Health": "OK", "State": "StandbySpare"}
	}`
	srv := newTestServer(t, r)
	defer srv.Close()
	c := NewMemoryCollector(srv.URL, newTestGoFishClient(t, srv.URL), discardLogger())

	// not absent/disabled → capacity emitted; not enabled → no status metric
	assert.Equal(t, 1, testutil.CollectAndCount(c))
}

func TestMemoryCollector_Collect_FallsBackToIDWhenLocatorMissing(t *testing.T) {
	r := defaultResponses()
	r.memoryItem = `{
		"@odata.id": "/redfish/v1/Systems/1/Memory/1",
		"Id": "1",
		"CapacityMiB": 16384,
		"Status": {"Health": "OK", "State": "Enabled"}
	}`
	srv := newTestServer(t, r)
	defer srv.Close()
	c := NewMemoryCollector(srv.URL, newTestGoFishClient(t, srv.URL), discardLogger())

	// DeviceLocator empty → falls back to ID; metrics still emitted normally
	assert.Equal(t, 2, testutil.CollectAndCount(c))
}

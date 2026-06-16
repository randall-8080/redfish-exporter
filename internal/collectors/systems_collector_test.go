package collectors

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
)

func TestSystemsCollector_Collect_HappyPath(t *testing.T) {
	srv := newTestServer(t, defaultResponses())
	defer srv.Close()
	c := NewSystemsCollector(srv.URL, newTestGoFishClient(t, srv.URL), discardLogger())

	// system_info(1) + system_health(1) + system_health_rollup(1)
	// + processor_count(1) + processor_health(1) + processor_health_rollup(1)
	// + memory_gib(1) + memory_health(1) + memory_health_rollup(1) = 9
	assert.Equal(t, 9, testutil.CollectAndCount(c))
}

func TestSystemsCollector_Collect_WarningHealthRollup(t *testing.T) {
	r := defaultResponses()
	r.systemItem = `{
		"@odata.id": "/redfish/v1/Systems/1",
		"Id": "1",
		"SerialNumber": "SN123",
		"Manufacturer": "Acme",
		"Model": "TestServer",
		"Status": {"HealthRollup": "Warning"},
		"Memory": {"@odata.id": "/redfish/v1/Systems/1/Memory"}
	}`
	srv := newTestServer(t, r)
	defer srv.Close()
	c := NewSystemsCollector(srv.URL, newTestGoFishClient(t, srv.URL), discardLogger())

	// no Health field → no system_health; system_info(1) + system_health_rollup(1, value=0.0) = 2
	assert.Equal(t, 2, testutil.CollectAndCount(c))
}

func TestSystemsCollector_Collect_HealthOnlyNoRollup(t *testing.T) {
	// Supermicro and some other BMCs populate Health but omit HealthRollup
	r := defaultResponses()
	r.systemItem = `{
		"@odata.id": "/redfish/v1/Systems/1",
		"Id": "1",
		"SerialNumber": "SN123",
		"Manufacturer": "Supermicro",
		"Model": "Super Server",
		"Status": {"State": "Enabled", "Health": "OK"},
		"Memory": {"@odata.id": "/redfish/v1/Systems/1/Memory"}
	}`
	srv := newTestServer(t, r)
	defer srv.Close()
	c := NewSystemsCollector(srv.URL, newTestGoFishClient(t, srv.URL), discardLogger())

	// system_info(1) + system_health(1) — no health_rollup emitted for absent field
	assert.Equal(t, 2, testutil.CollectAndCount(c))
}

func TestSystemsCollector_Collect_ProcessorAndMemorySummary(t *testing.T) {
	r := defaultResponses()
	r.systemItem = `{
		"@odata.id": "/redfish/v1/Systems/1",
		"Id": "1",
		"SerialNumber": "SN123",
		"Manufacturer": "Supermicro",
		"Model": "Super Server",
		"Status": {"State": "Enabled", "Health": "OK"},
		"ProcessorSummary": {
			"Count": 1,
			"Status": {"State": "Enabled", "Health": "OK", "HealthRollup": "OK"}
		},
		"MemorySummary": {
			"TotalSystemMemoryGiB": 512,
			"Status": {"State": "Enabled", "Health": "OK", "HealthRollup": "OK"}
		},
		"Memory": {"@odata.id": "/redfish/v1/Systems/1/Memory"}
	}`
	srv := newTestServer(t, r)
	defer srv.Close()
	c := NewSystemsCollector(srv.URL, newTestGoFishClient(t, srv.URL), discardLogger())

	// system_info(1) + system_health(1)
	// + processor_count(1) + processor_health(1) + processor_health_rollup(1)
	// + memory_gib(1) + memory_health(1) + memory_health_rollup(1) = 8
	assert.Equal(t, 8, testutil.CollectAndCount(c))
}

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

	// system_metadata(1) + system_status_health_rollup(1) = 2
	assert.Equal(t, 2, testutil.CollectAndCount(c))
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

	// same count; health_rollup emits 0.0 for non-OK states
	assert.Equal(t, 2, testutil.CollectAndCount(c))
}

package collectors

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
)

func TestManagersCollector_Collect_HappyPath(t *testing.T) {
	srv := newTestServer(t, defaultResponses())
	defer srv.Close()
	c := NewManagersCollector(srv.URL, newTestGoFishClient(t, srv.URL), discardLogger())

	// manager_info(1) + manager_health(1) = 2
	assert.Equal(t, 2, testutil.CollectAndCount(c))
}

func TestManagersCollector_Collect_SkipsHealthWhenAbsent(t *testing.T) {
	r := defaultResponses()
	r.managerItem = `{
		"@odata.id": "/redfish/v1/Managers/BMC",
		"Id": "BMC",
		"ManagerType": "BMC",
		"Model": "TestBMC",
		"FirmwareVersion": "1.0.0",
		"Status": {"State": "Enabled"}
	}`
	srv := newTestServer(t, r)
	defer srv.Close()
	c := NewManagersCollector(srv.URL, newTestGoFishClient(t, srv.URL), discardLogger())

	// only manager_info; no health field in response
	assert.Equal(t, 1, testutil.CollectAndCount(c))
}

func TestManagersCollector_Collect_UnhealthyManager(t *testing.T) {
	r := defaultResponses()
	r.managerItem = `{
		"@odata.id": "/redfish/v1/Managers/BMC",
		"Id": "BMC",
		"ManagerType": "BMC",
		"Model": "TestBMC",
		"FirmwareVersion": "1.0.0",
		"Status": {"Health": "Critical", "State": "Enabled"}
	}`
	srv := newTestServer(t, r)
	defer srv.Close()
	c := NewManagersCollector(srv.URL, newTestGoFishClient(t, srv.URL), discardLogger())

	// manager_info(1) + manager_health(1, value=0.0) = 2
	assert.Equal(t, 2, testutil.CollectAndCount(c))
}

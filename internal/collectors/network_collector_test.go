package collectors

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
)

func TestNetworkCollector_Collect_HappyPath(t *testing.T) {
	srv := newTestServer(t, defaultResponses())
	defer srv.Close()
	c := NewNetworkCollector(srv.URL, newTestGoFishClient(t, srv.URL), discardLogger())

	// link_up(1) + speed(1) + interface_health(1) + adapter_health(1) = 4
	assert.Equal(t, 4, testutil.CollectAndCount(c))
}

func TestNetworkCollector_Collect_LinkDown(t *testing.T) {
	r := defaultResponses()
	r.ethernetInterfaceItem = `{
		"@odata.id": "/redfish/v1/Systems/1/EthernetInterfaces/1",
		"Id": "1",
		"MACAddress": "AA:BB:CC:DD:EE:FF",
		"SpeedMbps": 0,
		"LinkStatus": "LinkDown",
		"Status": {"Health": "OK", "State": "Enabled"}
	}`
	srv := newTestServer(t, r)
	defer srv.Close()
	c := NewNetworkCollector(srv.URL, newTestGoFishClient(t, srv.URL), discardLogger())

	// link_up(0) emitted; speed skipped (0); interface_health(1); adapter_health(1) = 3
	assert.Equal(t, 3, testutil.CollectAndCount(c))
}

func TestNetworkCollector_Collect_NilSpeedSkipped(t *testing.T) {
	r := defaultResponses()
	r.ethernetInterfaceItem = `{
		"@odata.id": "/redfish/v1/Systems/1/EthernetInterfaces/1",
		"Id": "1",
		"MACAddress": "AA:BB:CC:DD:EE:FF",
		"LinkStatus": "LinkUp",
		"Status": {"Health": "OK", "State": "Enabled"}
	}`
	srv := newTestServer(t, r)
	defer srv.Close()
	c := NewNetworkCollector(srv.URL, newTestGoFishClient(t, srv.URL), discardLogger())

	// SpeedMbps absent → speed skipped; link_up(1) + interface_health(1) + adapter_health(1) = 3
	assert.Equal(t, 3, testutil.CollectAndCount(c))
}

func TestNetworkCollector_Collect_SkipsAbsentInterface(t *testing.T) {
	r := defaultResponses()
	r.ethernetInterfaceItem = `{
		"@odata.id": "/redfish/v1/Systems/1/EthernetInterfaces/1",
		"Id": "1",
		"MACAddress": "AA:BB:CC:DD:EE:FF",
		"Status": {"Health": "OK", "State": "Absent"}
	}`
	srv := newTestServer(t, r)
	defer srv.Close()
	c := NewNetworkCollector(srv.URL, newTestGoFishClient(t, srv.URL), discardLogger())

	// absent interface skipped entirely; only adapter_health(1) = 1
	assert.Equal(t, 1, testutil.CollectAndCount(c))
}

func TestNetworkCollector_Collect_AdapterUnhealthy(t *testing.T) {
	r := defaultResponses()
	r.networkAdapterItem = `{
		"@odata.id": "/redfish/v1/Chassis/1/NetworkAdapters/1",
		"Id": "1",
		"Manufacturer": "Intel",
		"Model": "X550",
		"Status": {"Health": "Critical", "State": "Enabled"}
	}`
	srv := newTestServer(t, r)
	defer srv.Close()
	c := NewNetworkCollector(srv.URL, newTestGoFishClient(t, srv.URL), discardLogger())

	// adapter_health emits 0.0; count unchanged
	assert.Equal(t, 4, testutil.CollectAndCount(c))
}

func TestNetworkCollector_Collect_SkipsAdapterWithNoHealth(t *testing.T) {
	r := defaultResponses()
	r.networkAdapterItem = `{
		"@odata.id": "/redfish/v1/Chassis/1/NetworkAdapters/1",
		"Id": "1",
		"Manufacturer": "Intel",
		"Model": "X550"
	}`
	srv := newTestServer(t, r)
	defer srv.Close()
	c := NewNetworkCollector(srv.URL, newTestGoFishClient(t, srv.URL), discardLogger())

	// no health field → adapter_health skipped; link_up(1) + speed(1) + interface_health(1) = 3
	assert.Equal(t, 3, testutil.CollectAndCount(c))
}

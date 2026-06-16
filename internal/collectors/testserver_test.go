package collectors

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stmcginnis/gofish"
	"github.com/stretchr/testify/require"
)

// redfishResponses holds the JSON bodies served by the test Redfish server.
type redfishResponses struct {
	serviceRoot           string
	chassis               string
	chassisItem           string
	power                 string
	thermal               string
	networkAdapters       string
	networkAdapterItem    string
	systems               string
	systemItem            string
	memory                string
	memoryItem            string
	ethernetInterfaces    string
	ethernetInterfaceItem string
}

func defaultResponses() redfishResponses {
	return redfishResponses{
		serviceRoot: `{
			"@odata.id": "/redfish/v1/",
			"Id": "RootService",
			"Name": "Root Service",
			"Chassis": {"@odata.id": "/redfish/v1/Chassis"},
			"Systems": {"@odata.id": "/redfish/v1/Systems"}
		}`,
		chassis: `{
			"@odata.id": "/redfish/v1/Chassis",
			"Members": [{"@odata.id": "/redfish/v1/Chassis/1"}],
			"Members@odata.count": 1
		}`,
		chassisItem: `{
			"@odata.id": "/redfish/v1/Chassis/1",
			"Id": "1",
			"PowerState": "On",
			"Status": {"Health": "OK", "HealthRollup": "OK"},
			"Power":           {"@odata.id": "/redfish/v1/Chassis/1/Power"},
			"Thermal":         {"@odata.id": "/redfish/v1/Chassis/1/Thermal"},
			"NetworkAdapters": {"@odata.id": "/redfish/v1/Chassis/1/NetworkAdapters"}
		}`,
		power: `{
			"@odata.id": "/redfish/v1/Chassis/1/Power",
			"PowerControl": [{
				"MemberId": "0",
				"Name": "System Power Control",
				"PowerCapacityWatts": 1600.0,
				"PowerConsumedWatts": 250.0,
				"PowerMetrics": {
					"AverageConsumedWatts": 200.0,
					"MaxConsumedWatts": 300.0,
					"MinConsumedWatts": 150.0
				},
				"Status": {"Health": "OK", "State": "Enabled"}
			}],
			"Redundancy": [],
			"PowerSupplies": [{
				"MemberId": "0",
				"Name": "PSU1",
				"Status": {"Health": "OK", "State": "Enabled"}
			}]
		}`,
		thermal: `{
			"@odata.id": "/redfish/v1/Chassis/1/Thermal",
			"Temperatures": [{
				"MemberId": "0",
				"Name": "CPU1 Temp",
				"ReadingCelsius": 45.0,
				"PhysicalContext": "CPU",
				"Status": {"Health": "OK", "State": "Enabled"}
			}],
			"Fans": [{
				"MemberId": "0",
				"Name": "Fan1",
				"Reading": 2400,
				"ReadingUnits": "RPM",
				"PhysicalContext": "SystemBoard",
				"Status": {"Health": "OK", "State": "Enabled"}
			}]
		}`,
		networkAdapters: `{
			"@odata.id": "/redfish/v1/Chassis/1/NetworkAdapters",
			"Members": [{"@odata.id": "/redfish/v1/Chassis/1/NetworkAdapters/1"}],
			"Members@odata.count": 1
		}`,
		networkAdapterItem: `{
			"@odata.id": "/redfish/v1/Chassis/1/NetworkAdapters/1",
			"Id": "1",
			"Manufacturer": "Intel",
			"Model": "X550",
			"Status": {"Health": "OK", "State": "Enabled"}
		}`,
		systems: `{
			"@odata.id": "/redfish/v1/Systems",
			"Members": [{"@odata.id": "/redfish/v1/Systems/1"}],
			"Members@odata.count": 1
		}`,
		systemItem: `{
			"@odata.id": "/redfish/v1/Systems/1",
			"Id": "1",
			"SerialNumber": "SN123",
			"Manufacturer": "Acme",
			"Model": "TestServer",
			"Status": {"Health": "OK", "HealthRollup": "OK"},
			"ProcessorSummary": {
				"Count": 2,
				"Status": {"Health": "OK", "HealthRollup": "OK"}
			},
			"MemorySummary": {
				"TotalSystemMemoryGiB": 128,
				"Status": {"Health": "OK", "HealthRollup": "OK"}
			},
			"EthernetInterfaces": {"@odata.id": "/redfish/v1/Systems/1/EthernetInterfaces"},
			"Memory": {"@odata.id": "/redfish/v1/Systems/1/Memory"}
		}`,
		memory: `{
			"@odata.id": "/redfish/v1/Systems/1/Memory",
			"Members": [{"@odata.id": "/redfish/v1/Systems/1/Memory/1"}],
			"Members@odata.count": 1
		}`,
		memoryItem: `{
			"@odata.id": "/redfish/v1/Systems/1/Memory/1",
			"Id": "1",
			"CapacityMiB": 16384,
			"DeviceLocator": "DIMM_A1",
			"Status": {"Health": "OK", "State": "Enabled"}
		}`,
		ethernetInterfaces: `{
			"@odata.id": "/redfish/v1/Systems/1/EthernetInterfaces",
			"Members": [{"@odata.id": "/redfish/v1/Systems/1/EthernetInterfaces/1"}],
			"Members@odata.count": 1
		}`,
		ethernetInterfaceItem: `{
			"@odata.id": "/redfish/v1/Systems/1/EthernetInterfaces/1",
			"Id": "1",
			"MACAddress": "AA:BB:CC:DD:EE:FF",
			"SpeedMbps": 10000,
			"LinkStatus": "LinkUp",
			"Status": {"Health": "OK", "State": "Enabled"}
		}`,
	}
}

// newTestServer starts an httptest.Server that serves the given Redfish JSON responses.
func newTestServer(t *testing.T, r redfishResponses) *httptest.Server {
	t.Helper()
	routes := map[string]string{
		"/redfish/v1/":                               r.serviceRoot,
		"/redfish/v1/Chassis":                        r.chassis,
		"/redfish/v1/Chassis/1":                      r.chassisItem,
		"/redfish/v1/Chassis/1/Power":                r.power,
		"/redfish/v1/Chassis/1/Thermal":              r.thermal,
		"/redfish/v1/Chassis/1/NetworkAdapters":      r.networkAdapters,
		"/redfish/v1/Chassis/1/NetworkAdapters/1":    r.networkAdapterItem,
		"/redfish/v1/Systems":                        r.systems,
		"/redfish/v1/Systems/1":                      r.systemItem,
		"/redfish/v1/Systems/1/Memory":               r.memory,
		"/redfish/v1/Systems/1/Memory/1":             r.memoryItem,
		"/redfish/v1/Systems/1/EthernetInterfaces":   r.ethernetInterfaces,
		"/redfish/v1/Systems/1/EthernetInterfaces/1": r.ethernetInterfaceItem,
	}
	mux := http.NewServeMux()
	for path, body := range routes {
		body := body
		mux.HandleFunc(path, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(body))
		})
	}
	return httptest.NewServer(mux)
}

// newTestGoFishClient connects a gofish client to the test server without authentication.
func newTestGoFishClient(t *testing.T, serverURL string) *gofish.APIClient {
	t.Helper()
	client, err := gofish.Connect(gofish.ClientConfig{Endpoint: serverURL})
	require.NoError(t, err)
	t.Cleanup(func() { client.Logout() })
	return client
}

package collectors

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
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
	managers              string
	managerItem           string

	// overrides replaces the handler for a path with a fixed status and body,
	// simulating missing (404), unauthorized (401), failing (5xx), or
	// malformed resources.
	overrides map[string]routeOverride

	// extraRoutes serves additional JSON resources with status 200, e.g.
	// modern Redfish resources or additional chassis.
	extraRoutes map[string]string
}

// routeOverride describes a canned non-default response for one path.
type routeOverride struct {
	status int
	body   string
}

func defaultResponses() redfishResponses {
	return redfishResponses{
		serviceRoot: `{
			"@odata.id": "/redfish/v1/",
			"Id": "RootService",
			"Name": "Root Service",
			"Chassis":  {"@odata.id": "/redfish/v1/Chassis"},
			"Systems":  {"@odata.id": "/redfish/v1/Systems"},
			"Managers": {"@odata.id": "/redfish/v1/Managers"}
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
		managers: `{
			"@odata.id": "/redfish/v1/Managers",
			"Members": [{"@odata.id": "/redfish/v1/Managers/BMC"}],
			"Members@odata.count": 1
		}`,
		managerItem: `{
			"@odata.id": "/redfish/v1/Managers/BMC",
			"Id": "BMC",
			"ManagerType": "BMC",
			"Model": "TestBMC",
			"FirmwareVersion": "1.0.0",
			"Manufacturer": "Acme",
			"Status": {"Health": "OK", "State": "Enabled"}
		}`,
	}
}

// newTestServer starts an httptest.Server that serves the given Redfish JSON responses.
func newTestServer(t *testing.T, r redfishResponses) *httptest.Server {
	t.Helper()
	handlers := map[string]http.HandlerFunc{
		"/redfish/v1/":                               jsonHandler(r.serviceRoot),
		"/redfish/v1/Chassis":                        jsonHandler(r.chassis),
		"/redfish/v1/Chassis/1":                      jsonHandler(r.chassisItem),
		"/redfish/v1/Chassis/1/Power":                jsonHandler(r.power),
		"/redfish/v1/Chassis/1/Thermal":              jsonHandler(r.thermal),
		"/redfish/v1/Chassis/1/NetworkAdapters":      jsonHandler(r.networkAdapters),
		"/redfish/v1/Chassis/1/NetworkAdapters/1":    jsonHandler(r.networkAdapterItem),
		"/redfish/v1/Systems":                        jsonHandler(r.systems),
		"/redfish/v1/Systems/1":                      jsonHandler(r.systemItem),
		"/redfish/v1/Systems/1/Memory":               jsonHandler(r.memory),
		"/redfish/v1/Systems/1/Memory/1":             jsonHandler(r.memoryItem),
		"/redfish/v1/Systems/1/EthernetInterfaces":   jsonHandler(r.ethernetInterfaces),
		"/redfish/v1/Systems/1/EthernetInterfaces/1": jsonHandler(r.ethernetInterfaceItem),
		"/redfish/v1/Managers":                       jsonHandler(r.managers),
		"/redfish/v1/Managers/BMC":                   jsonHandler(r.managerItem),
	}
	for path, body := range r.extraRoutes {
		handlers[path] = jsonHandler(body)
	}
	for path, o := range r.overrides {
		status, body := o.status, o.body
		handlers[path] = func(w http.ResponseWriter, _ *http.Request) {
			if body != "" {
				w.Header().Set("Content-Type", "application/json")
			}
			w.WriteHeader(status)
			if body != "" {
				_, _ = w.Write([]byte(body))
			}
		}
	}
	mux := http.NewServeMux()
	for path, handler := range handlers {
		mux.Handle(path, handler)
	}
	return httptest.NewServer(mux)
}

// jsonHandler returns a handler that serves body as JSON with status 200.
func jsonHandler(body string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}
}

// newTestGoFishClient connects a gofish client to the test server without authentication.
func newTestGoFishClient(t *testing.T, serverURL string) *gofish.APIClient {
	t.Helper()
	client, err := gofish.Connect(gofish.ClientConfig{Endpoint: serverURL})
	require.NoError(t, err)
	t.Cleanup(func() { client.Logout() })
	return client
}

// gatherFamilies collects c through a fresh pedantic registry and returns the
// metric families keyed by name. Duplicate series and inconsistent metrics
// make the registry Gather fail, so a successful call proves the output is
// free of duplicate samples.
func gatherFamilies(t *testing.T, c prometheus.Collector) map[string]*dto.MetricFamily {
	t.Helper()
	reg := prometheus.NewPedanticRegistry()
	require.NoError(t, reg.Register(c))
	families, err := reg.Gather()
	require.NoError(t, err)
	out := make(map[string]*dto.MetricFamily, len(families))
	for _, f := range families {
		out[f.GetName()] = f
	}
	return out
}

// findMetric returns the metric in the family whose labels match want
// exactly, or nil when no such metric exists.
func findMetric(f *dto.MetricFamily, want map[string]string) *dto.Metric {
	if f == nil {
		return nil
	}
	for _, m := range f.GetMetric() {
		if metricLabelsMatch(m, want) {
			return m
		}
	}
	return nil
}

// metricLabelsMatch reports whether the metric's labels match want exactly.
func metricLabelsMatch(m *dto.Metric, want map[string]string) bool {
	labels := make(map[string]string, len(m.GetLabel()))
	for _, lp := range m.GetLabel() {
		labels[lp.GetName()] = lp.GetValue()
	}
	if len(labels) != len(want) {
		return false
	}
	for k, v := range want {
		if labels[k] != v {
			return false
		}
	}
	return true
}

package collectors

import "github.com/stmcginnis/gofish/schemas"

// healthToFloat converts a Redfish Health value to a binary Prometheus gauge.
// OK=1.0, any other value (Warning, Critical, unknown)=0.0.
// The raw health string should be passed as a "health" label so severity
// remains queryable: metric{health="Critical"} == 0 vs metric{health="Warning"} == 0.
func healthToFloat(h schemas.Health) float64 {
	if h == schemas.OKHealth {
		return 1.0
	}
	return 0.0
}

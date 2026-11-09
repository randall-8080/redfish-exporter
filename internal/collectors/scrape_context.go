package collectors

import (
	"log/slog"
	"sync"

	"github.com/stmcginnis/gofish"
	"github.com/stmcginnis/gofish/schemas"
)

// scrapeContext caches Redfish resources for the duration of a single scrape
// so that parallel sub-collectors do not fetch the same resource twice. A
// context is created per Collect call and never reused, so cached data can
// never go stale.
type scrapeContext struct {
	client *gofish.APIClient
	logger *slog.Logger

	// powerEnabled records whether the power sub-collector runs in this
	// scrape. Thermal uses it to decide whether temperature sensors consumed
	// by power supply metrics are emitted by the power collector instead.
	powerEnabled bool

	mu             sync.Mutex
	chassis        []*schemas.Chassis
	chassisErr     error
	chassisFetched bool

	sensors    map[string]*sensorFetch
	envMetrics map[string]*envMetricsFetch
}

// sensorFetch is the cached result of a chassis Sensors collection fetch.
type sensorFetch struct {
	sensors []*schemas.Sensor
	err     error
}

// envMetricsFetch is the cached result of a chassis EnvironmentMetrics fetch.
type envMetricsFetch struct {
	metrics *schemas.EnvironmentMetrics
	err     error
}

func newScrapeContext(client *gofish.APIClient, logger *slog.Logger, powerEnabled bool) *scrapeContext {
	return &scrapeContext{
		client:       client,
		logger:       logger,
		powerEnabled: powerEnabled,
	}
}

// Chassis returns the service chassis list, fetching it at most once per
// scrape. When individual members fail, gofish returns the chassis it could
// fetch together with an error; both are passed through so callers can still
// collect from the usable chassis while surfacing the failure.
func (s *scrapeContext) Chassis() ([]*schemas.Chassis, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.chassisFetched {
		s.chassis, s.chassisErr = s.client.Service.Chassis()
		s.chassisFetched = true
	}
	return s.chassis, s.chassisErr
}

// Sensors returns the chassis sensor collection, fetching it at most once per
// scrape so the thermal and power collectors share one fetch. Partial
// results from a collection with failing members are passed through with
// their error.
func (s *scrapeContext) Sensors(chassis *schemas.Chassis) ([]*schemas.Sensor, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sensors == nil {
		s.sensors = make(map[string]*sensorFetch)
	}
	if f, ok := s.sensors[chassis.ODataID]; ok {
		return f.sensors, f.err
	}
	sensors, err := chassis.Sensors()
	s.sensors[chassis.ODataID] = &sensorFetch{sensors: sensors, err: err}
	return sensors, err
}

// EnvironmentMetrics returns the chassis EnvironmentMetrics resource,
// fetching it at most once per scrape so the thermal and power collectors
// share one fetch.
func (s *scrapeContext) EnvironmentMetrics(chassis *schemas.Chassis) (*schemas.EnvironmentMetrics, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.envMetrics == nil {
		s.envMetrics = make(map[string]*envMetricsFetch)
	}
	if f, ok := s.envMetrics[chassis.ODataID]; ok {
		return f.metrics, f.err
	}
	metrics, err := chassis.EnvironmentMetrics()
	s.envMetrics[chassis.ODataID] = &envMetricsFetch{metrics: metrics, err: err}
	return metrics, err
}

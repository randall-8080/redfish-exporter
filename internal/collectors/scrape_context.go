package collectors

import (
	"errors"
	"fmt"
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
	power      map[string]*powerSubsystemFetch
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

// powerSubsystemFetch is the cached result of fetching a chassis power
// subsystem tree: the subsystem, its power supplies, and each supply's
// metrics resource.
type powerSubsystemFetch struct {
	subsystem *schemas.PowerSubsystem
	psus      []*schemas.PowerSupplyUnit
	// metrics maps a power supply's ODataID to its metrics resource.
	metrics map[string]*schemas.PowerSupplyMetrics
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

// PowerSubsystem fetches the chassis power subsystem tree once per scrape:
// the subsystem, its power supplies, and each supply's metrics resource.
// Fetch failures are joined into the returned error while the fetched parts
// remain available; a subsystem the service does not implement (no link or
// 404) yields a nil subsystem without an error.
func (s *scrapeContext) PowerSubsystem(chassis *schemas.Chassis) (*powerSubsystemFetch, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.power == nil {
		s.power = make(map[string]*powerSubsystemFetch)
	}
	if f, ok := s.power[chassis.ODataID]; ok {
		return f, f.err
	}
	f := &powerSubsystemFetch{metrics: make(map[string]*schemas.PowerSupplyMetrics)}
	subsystem, err := chassis.PowerSubsystem()
	switch {
	case err == nil && subsystem != nil:
		f.subsystem = subsystem
		psus, err := subsystem.PowerSupplies()
		if err != nil {
			// gofish returns the supplies it could fetch; keep them.
			f.err = errors.Join(f.err, err)
		}
		f.psus = psus
		for _, psu := range psus {
			metrics, err := psu.Metrics()
			if err != nil && !isNotFound(err) {
				f.err = errors.Join(f.err, fmt.Errorf("power supply %s: %w", psu.ID, err))
			}
			if metrics != nil {
				f.metrics[psu.ODataID] = metrics
			}
		}
	case err == nil:
		// No PowerSubsystem link; legacy and sensor sources still apply.
	case isNotFound(err):
		s.logger.Debug("power subsystem not found", "chassis", chassis.ID)
	default:
		f.err = err
	}
	s.power[chassis.ODataID] = f
	return f, f.err
}

// PSUConsumedTemperatureURIs returns the DataSourceUri set of temperature
// sensors that power supply metrics report as
// redfish_power_supply_temperature_celsius. The thermal collector uses it to
// avoid emitting the same reading as a chassis temperature; it is only
// meaningful when the power collector runs in this scrape.
func (s *scrapeContext) PSUConsumedTemperatureURIs(chassis *schemas.Chassis) map[string]bool {
	f, _ := s.PowerSubsystem(chassis)
	uris := make(map[string]bool)
	// Mirror the power collector: absent supplies report nothing, so their
	// sensors stay available to the temperature family.
	for _, psu := range f.psus {
		if psu.Status.State == schemas.AbsentState {
			continue
		}
		m := f.metrics[psu.ODataID]
		if m == nil {
			continue
		}
		if m.TemperatureCelsius.Reading != nil && m.TemperatureCelsius.DataSourceURI != "" {
			uris[m.TemperatureCelsius.DataSourceURI] = true
		}
	}
	return uris
}

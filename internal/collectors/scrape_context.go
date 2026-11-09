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
//
// Each resource is cached behind a per-key entry whose fetch runs outside
// the context lock: callers for the same key share one fetch, while fetches
// for different keys proceed in parallel.
type scrapeContext struct {
	client *gofish.APIClient
	logger *slog.Logger

	// powerEnabled records whether the power sub-collector runs in this
	// scrape. Thermal uses it to decide whether temperature sensors consumed
	// by power supply metrics are emitted by the power collector instead.
	powerEnabled bool

	mu      sync.Mutex
	chassis *fetchEntry[[]*schemas.Chassis]
	sensors map[string]*fetchEntry[[]*schemas.Sensor]
	env     map[string]*fetchEntry[*schemas.EnvironmentMetrics]
	power   map[string]*fetchEntry[*powerSubsystemFetch]
}

// fetchEntry holds the result of a single fetch. The first caller executes
// do; concurrent callers block until the fetch completes and share its
// result.
type fetchEntry[T any] struct {
	once sync.Once
	val  T
	err  error
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

// entry returns the cache entry for key, creating it under the context
// lock. The fetch itself runs outside the lock.
func entryFor[T any](s *scrapeContext, m *map[string]*fetchEntry[T], key string) *fetchEntry[T] {
	s.mu.Lock()
	defer s.mu.Unlock()
	if *m == nil {
		*m = make(map[string]*fetchEntry[T])
	}
	e, ok := (*m)[key]
	if !ok {
		e = &fetchEntry[T]{}
		(*m)[key] = e
	}
	return e
}

// Chassis returns the service chassis list, fetching it at most once per
// scrape. When individual members fail, gofish returns the chassis it could
// fetch together with an error; both are passed through so callers can still
// collect from the usable chassis while surfacing the failure.
func (s *scrapeContext) Chassis() ([]*schemas.Chassis, error) {
	s.mu.Lock()
	e := s.chassis
	if e == nil {
		e = &fetchEntry[[]*schemas.Chassis]{}
		s.chassis = e
	}
	s.mu.Unlock()
	e.once.Do(func() { e.val, e.err = s.client.Service.Chassis() })
	return e.val, e.err
}

// Sensors returns the chassis sensor collection, fetching it at most once
// per scrape so the thermal and power collectors share one fetch. Partial
// results from a collection with failing members are passed through with
// their error.
func (s *scrapeContext) Sensors(chassis *schemas.Chassis) ([]*schemas.Sensor, error) {
	e := entryFor(s, &s.sensors, chassis.ODataID)
	e.once.Do(func() { e.val, e.err = chassis.Sensors() })
	return e.val, e.err
}

// EnvironmentMetrics returns the chassis EnvironmentMetrics resource,
// fetching it at most once per scrape so the thermal and power collectors
// share one fetch.
func (s *scrapeContext) EnvironmentMetrics(chassis *schemas.Chassis) (*schemas.EnvironmentMetrics, error) {
	e := entryFor(s, &s.env, chassis.ODataID)
	e.once.Do(func() { e.val, e.err = chassis.EnvironmentMetrics() })
	return e.val, e.err
}

// PowerSubsystem fetches the chassis power subsystem tree once per scrape:
// the subsystem, its power supplies, and each supply's metrics resource.
// Fetch failures are joined into the returned error while the fetched parts
// remain available; a subsystem the service does not implement (no link or
// 404) yields a nil subsystem without an error.
func (s *scrapeContext) PowerSubsystem(chassis *schemas.Chassis) (*powerSubsystemFetch, error) {
	e := entryFor(s, &s.power, chassis.ODataID)
	e.once.Do(func() {
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
		e.val, e.err = f, f.err
	})
	return e.val, e.err
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

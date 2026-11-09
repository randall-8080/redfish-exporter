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

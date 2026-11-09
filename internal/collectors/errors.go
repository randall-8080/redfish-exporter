package collectors

import (
	"errors"
	"net/http"

	"github.com/stmcginnis/gofish/schemas"
)

// isNotFound reports whether err is an HTTP 404 from the Redfish service.
// For an optionally linked resource a 404 means the service does not
// implement that resource — a capability gap rather than a failure.
func isNotFound(err error) bool {
	var rfErr *schemas.Error
	return errors.As(err, &rfErr) && rfErr.HTTPReturnedStatusCode == http.StatusNotFound
}

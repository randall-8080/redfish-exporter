package cmd

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/e-breuninger/redfish-exporter/internal/config"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// minimalConfig returns a Config with a default module and all collectors disabled.
func minimalConfig() *config.Config {
	return &config.Config{
		Modules: map[string]config.Module{
			"default": {},
		},
	}
}

// minimalRedfishServer returns an httptest.Server that serves a bare Redfish service root.
func minimalRedfishServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/redfish/v1/" {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"@odata.id":"/redfish/v1/","Id":"RootService","Name":"Root Service"}`)
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestProbeHandler_MissingTarget(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/probe", nil)
	w := httptest.NewRecorder()

	probeHandler(w, req, discardLogger(), minimalConfig())

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestProbeHandler_UnknownModule(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/probe?target=http://bmc.example.com&module=nonexistent", nil)
	w := httptest.NewRecorder()

	probeHandler(w, req, discardLogger(), minimalConfig())

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestProbeHandler_EmptyModuleDefaultsToDefault(t *testing.T) {
	// no module param → should use "default", not reject the request
	srv := minimalRedfishServer(t)

	req := httptest.NewRequest(http.MethodGet, "/probe?target="+srv.URL, nil)
	w := httptest.NewRecorder()

	probeHandler(w, req, discardLogger(), minimalConfig())

	// 200, not 400 "Unknown module"
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestProbeHandler_UnreachableTarget(t *testing.T) {
	// closed server → connection refused → 500
	closed := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {}))
	closed.Close()

	req := httptest.NewRequest(http.MethodGet, "/probe?target="+closed.URL, nil)
	w := httptest.NewRecorder()

	probeHandler(w, req, discardLogger(), minimalConfig())

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestProbeHandler_HappyPath(t *testing.T) {
	srv := minimalRedfishServer(t)

	req := httptest.NewRequest(http.MethodGet, "/probe?target="+srv.URL, nil)
	w := httptest.NewRecorder()

	probeHandler(w, req, discardLogger(), minimalConfig())

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestProbeHandler_NamedModule(t *testing.T) {
	srv := minimalRedfishServer(t)

	cfg := &config.Config{
		Modules: map[string]config.Module{
			"default": {},
			"custom":  {Collectors: config.CollectorsConfig{Thermal: true}},
		},
	}
	req := httptest.NewRequest(http.MethodGet, "/probe?target="+srv.URL+"&module=custom", nil)
	w := httptest.NewRecorder()

	probeHandler(w, req, discardLogger(), cfg)

	assert.Equal(t, http.StatusOK, w.Code)
}

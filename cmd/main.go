package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"

	"github.com/alecthomas/kingpin/v2"
	"github.com/e-breuninger/redfish-exporter/internal/client"
	"github.com/e-breuninger/redfish-exporter/internal/collectors"
	"github.com/e-breuninger/redfish-exporter/internal/config"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/common/promslog"
	"github.com/prometheus/common/promslog/flag"
	"github.com/prometheus/common/version"
	"github.com/prometheus/exporter-toolkit/web"
	"github.com/prometheus/exporter-toolkit/web/kingpinflag"
)

var (
	configFile   = kingpin.Flag("config.file", "Redfish exporter configuration file.").Default("config.yaml").ExistingFile()
	toolkitFlags = kingpinflag.AddFlags(kingpin.CommandLine, ":9042")
	metricsPath  = "/metrics"
	probePath    = "/probe"
)

func Run() {

	promslogConfig := &promslog.Config{}

	flag.AddFlags(kingpin.CommandLine, promslogConfig)
	kingpin.Version(version.Print("redfish-expoter"))
	kingpin.HelpFlag.Short('h')
	kingpin.Parse()
	logger := promslog.New(promslogConfig)

	logger.Info("Starting redfish-exporter", "version", version.Info())
	logger.Info("Build context", "build", version.BuildContext())

	logger.Info("Loading config file", "file", *configFile)
	config, err := config.Load(*configFile)

	if err != nil {
		logger.Error("Error loading config", "err", err)
		os.Exit(1)
	}
	configJSON, err := json.Marshal(config)
	if err != nil {
		logger.Error("Failed to marshal config to JSON", "err", err)
	}
	logger.Debug("Loaded config file", "config", string(configJSON))

	http.Handle(metricsPath, promhttp.Handler())
	http.HandleFunc("/probe", func(w http.ResponseWriter, req *http.Request) {
		probeHandler(w, req, logger, config)
	})

	landingConfig := web.LandingConfig{
		Name:        "Redfish Exporter",
		Description: "Prometheus Exporter for probing Redfish API into metrics",
		Version:     version.Info(),
		Links: []web.LandingLinks{
			{
				Address: metricsPath,
				Text:    "Metrics",
			},
			{
				Address: probePath,
				Text:    "Probe Redfish Endpoint",
			},
		},
	}
	landingPage, err := web.NewLandingPage(landingConfig)
	if err != nil {
		logger.Error("error creating landing page", "err", err)
		os.Exit(1)
	}
	http.Handle("/", landingPage)

	server := &http.Server{}
	if err := web.ListenAndServe(server, toolkitFlags, logger); err != nil {
		logger.Error("Failed to start the server", "err", err)
		os.Exit(1)
	}
}

func probeHandler(w http.ResponseWriter, r *http.Request, logger *slog.Logger, config *config.Config) {
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	r = r.WithContext(ctx)

	module := r.URL.Query().Get("module")
	if module == "" {
		module = "default"
	}
	if _, ok := config.Modules[module]; !ok {
		http.Error(w, fmt.Sprintf("Unknown module %q", module), http.StatusBadRequest)
		logger.Debug("Unknown module", "module", module)
		return
	}

	target := r.URL.Query().Get("target")
	if target == "" {
		http.Error(w, "Target parameter is missing", http.StatusBadRequest)
		return
	}
	logger.Debug("Probing target", "module", module, "target", target)

	client, err := client.NewClientFromModuleConfig(config.Modules[module], target)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to create Redfish client: %v", err), http.StatusInternalServerError)
		logger.Error("Failed to create Redfish client", "err", err, "module", module, "target", target)
		return
	}

	// Ensure the client logs out when the probeHandler function returns
	defer client.Logout()

	// Create registry for probe internal metrics
	registry := prometheus.NewPedanticRegistry()
	registry.MustRegister(collectors.NewRedfishCollector(target, client, logger, config.Modules[module].Collectors))

	// Serve metrics
	h := promhttp.HandlerFor(registry, promhttp.HandlerOpts{})
	h.ServeHTTP(w, r)
}

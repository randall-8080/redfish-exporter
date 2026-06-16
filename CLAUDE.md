# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

A Prometheus exporter for Redfish BMC/iDRAC/iLO endpoints. Follows the **blackbox exporter** pattern:
- No persistent state; scrape-on-demand via HTTP `/probe` handler
- Target passed as query param: `?target=https://bmc.example.com&module=default`
- One binary, `net/http` stdlib server, `github.com/prometheus/exporter-toolkit` for TLS/auth and landing page
- Module path: `github.com/e-breuninger/redfish-exporter`

## Commands

```bash
go build ./...                                    # build
go test ./...                                     # unit tests only (no Docker needed)
DOCKER_HOST="unix:///run/user/1000/podman/podman.sock" TESTCONTAINERS_RYUK_DISABLED=true \
  go test -tags=integration ./...                   # integration tests (Podman, rootless)
go test -tags=integration -run TestRedfishCollector ./internal/collectors/
go run main.go --config.file config.yaml          # run locally
```

Linting uses `.golangci.yml`; run `golangci-lint run` locally or via `pre-commit run golangci-lint`. There is no Makefile.

## Architecture

Request flow: `GET /probe?target=<url>&module=<name>` → `probeHandler` in `cmd/main.go` → creates a `gofish.APIClient` per-scrape → instantiates `RedfishCollector` → registers it in a fresh `prometheus.Registry` → serves the gathered metrics.

`/metrics` exposes only the exporter's own process metrics (standard Go runtime metrics via default registry).

### Key packages

- **`internal/config`** — loads `config.yaml`; validates that a `default` module exists. Config maps module names to `Module{Auth, Insecure, CollectorsConfig}`.
- **`internal/client`** — thin wrapper around `gofish.Connect()`. Returns `*gofish.APIClient`; caller owns `Logout()`.
- **`internal/collectors`** — `RedfishCollector` is the top-level `prometheus.Collector` registered per probe. It holds a list of sub-collectors (thermal, power, systems, memory). `Collect()` fans them out in parallel with `sync.WaitGroup` and records per-collector duration via `redfish_exporter_collector_duration_seconds{target,collector}`.

### Sub-collector pattern

Each sub-collector (`ThermalCollector`, `PowerCollector`, `SystemsCollector`, `MemoryCollector`) follows the same shape:

```go
type XCollector struct {
    target string
    client *gofish.APIClient
    logger *slog.Logger
    // one *prometheus.Desc field per metric
}

func NewXCollector(target string, client *gofish.APIClient, logger *slog.Logger) *XCollector { ... }
func (c *XCollector) Describe(ch chan<- *prometheus.Desc) { /* send all descs */ }
func (c *XCollector) Collect(ch chan<- prometheus.Metric) { /* fetch Redfish, emit metrics */ }
```

`target` is always set as a constant label (`prometheus.Labels{"target": target}`) on every `prometheus.Desc`, so it appears on every metric without being a variable label.

Enabled collectors are selected via `config.CollectorsConfig` boolean flags (`Thermal`, `Power`, `Systems`, `Memory`).

### Health encoding

Redfish `common.Health` values are encoded as binary floats: `OK=1.0`, anything else `0.0`. The raw Redfish health string is carried as a `health` variable label (e.g. `health="Warning"`), so alert rules can inspect the exact value without a three-valued metric. Disabled/absent items are skipped rather than emitted as zero.

## Config File Format

```yaml
modules:
  default:
    auth:
      username: admin
      password: secret
    collectors:
      thermal: true
      power: true
      systems: true
      memory: true
    insecure: false
```

A `default` module is required; the server rejects requests for unknown module names.

## Testing Requirements

Every new feature or collector must be covered by **both** a unit test and an integration test:

- **Unit tests** — table-driven, `_test.go` next to the source file, no external dependencies.
- **Integration tests** — in the same `_test.go` file, tagged with `//go:build integration`, using `testcontainers-go` to spin up `dmtf/redfish-mockup-server:latest`. Test against the real mockup server; do not mock the Redfish HTTP layer.

A collector added without both test types will not be accepted.

## Integration Tests

`internal/collectors/redfish_collector_test.go` uses `testcontainers-go` to spin up `dmtf/redfish-mockup-server:latest`. Tests run as part of `go test ./...` and **require Docker**. There is currently no build tag to skip them without Docker.

## hack/ Directory

`hack/` contains a manual-testing harness (`hack_test.go`) for running against a real BMC. It is **not** part of the regular test suite. Run it with:

```bash
REDFISH_ENDPOINT=https://bmc.example.com REDFISH_USERNAME=admin REDFISH_PASSWORD=secret \
  go test ./hack/ -v -run TestHpeIlo4
```

The test skips automatically if those env vars are unset. `hack/.gitignore` has `*` — any local credential files added here are not tracked.

## Key Dependencies

```
github.com/stmcginnis/gofish           — Redfish client library (pinned to untagged commit)
github.com/prometheus/client_golang    — Prometheus metrics
github.com/prometheus/exporter-toolkit — TLS, auth, landing page
github.com/alecthomas/kingpin/v2       — CLI flags
github.com/prometheus/common/promslog  — structured slog-backed logger
gopkg.in/yaml.v3                       — config parsing
github.com/testcontainers/testcontainers-go — integration tests (test only)
```

## Versioning

This project uses **semantic versioning** with **Conventional Commits**:

- `feat:` → minor bump; `fix:` → patch bump; `feat!:` / `BREAKING CHANGE:` footer → major bump
- Scopes are optional but encouraged: `feat(thermal):`, `fix(power):`, `chore(ci):`
- Releases are triggered by pushing a `v*` tag; GoReleaser handles changelog and container publishing

Always ask before creating a commit. Commit after every self-contained logical change rather than batching unrelated edits into one commit.

## DO NOT

- Do not add HTTP framework routers (gin, echo, chi) — use `net/http` ServeMux
- Do not use the global `log` package — use the `*slog.Logger` passed into each collector
- Do not store Redfish sessions globally — each probe creates and defers `Logout()` on its own client
- Do not mock the Redfish HTTP layer in integration tests — use the real mockup container

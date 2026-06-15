# Redfish Exporter

A Prometheus exporter for Redfish-based hardware monitoring.

## Description

This exporter collects hardware metrics from Redfish-compatible servers
and exposes them in Prometheus format for monitoring and alerting.
It works similarly to the Blackbox exporter, using modules to define different monitoring configurations.

## Usage

```bash
# Run the exporter
./redfish_exporter --config config.yaml

# Default port: 9042
# Metrics endpoint: http://localhost:9042/metrics
```

### Configuration

See `config.example.yaml` for a complete configuration example.

Basic structure:

```yaml
modules:
    default:
        timeout: 10s
        username: admin
        password: secret
```

### Prometheus Configuration

```yaml
scrape_configs:    
  - job_name: 'redfish_default'
    metrics_path: /probe
    params:
      module: [default]
    static_configs:
        - targets:
            - https://foo.bar
    relabel_configs:
      - source_labels: [__address__]
        target_label: __param_target
      - source_labels: [__param_target]
        target_label: instance
      - target_label: __address__
        replacement: redfish_exporter:9042  # Redfish Exporter address 
```

## Metrics

### Health encoding

Redfish reports component health as an enum: `OK`, `Warning`, or `Critical`. This exporter maps those values to floats in two patterns depending on the metric:

**Three-valued** — used where `Warning` is a distinct, actionable state (power redundancy, power supplies):

| Redfish Health | Value |
|---|---|
| OK | 1.0 |
| Warning | 0.5 |
| Critical | 0.0 |

**Binary** — used for sensors where any non-OK state should alert (temperatures, fans, memory, system rollup):

| Redfish Health | Value |
|---|---|
| OK | 1.0 |
| Warning / Critical | 0.0 |

Binary health metrics also carry a `health` label with the raw Redfish string (`OK`, `Warning`, `Critical`), so the actual severity is still queryable even though the value is 0 for both non-OK states:

```promql
redfish_temperature_status{health="Warning"} == 0
redfish_temperature_status{health="Critical"} == 0
```

### Metric reference

| Metric | Health encoding | Labels |
|---|---|---|
| `redfish_system_status_health_rollup` | binary | `target`, `system_id` |
| `redfish_temperature_status` | binary | `target`, `chassis_id`, `sensor`, `physical_context`, `health` |
| `redfish_fan_status` | binary | `target`, `chassis_id`, `fan`, `physical_context`, `health` |
| `redfish_memory_status` | binary | `target`, `system_id`, `memory_id`, `memory_location`, `health` |
| `redfish_power_redundancy_health` | three-valued | `target`, `chassis_id`, `name`, `member_id` |
| `redfish_power_supplies_health` | three-valued | `target`, `chassis_id`, `name`, `member_id` |

Disabled or absent components are skipped entirely — they produce no metric rather than a zero value.

## License

See LICENSE file for details.
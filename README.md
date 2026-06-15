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

All health metrics use binary encoding: `1=OK`, `0=not OK`. Every health metric carries a `health` label with the raw Redfish string (`OK`, `Warning`, `Critical`), so severity is still queryable:

| Redfish Health | Value |
|---|---|
| OK | 1.0 |
| Warning | 0.0 |
| Critical | 0.0 |

Simple alerting catches any unhealthy state:

```promql
redfish_temperature_health == 0
```

Severity filtering:

```promql
redfish_temperature_health{health="Critical"} == 0
redfish_temperature_health{health="Warning"} == 0
```

### Collectors

Each collector is enabled per-module in the config under `collectors:`.

**Power** (`collectors.power: true`) — reads `Chassis[*].Power`

Tracks chassis power state, per-control-zone capacity and consumption (current, average, min, max), redundancy group health, and individual PSU health.

| Metric | Description |
|---|---|
| `redfish_power_state` | Chassis power state (1=On, 0=Off) |
| `redfish_power_capacity_watts` | Max power capacity per control zone |
| `redfish_power_consumed_watts` | Current power consumption per control zone |
| `redfish_power_average_consumed_watts` | Average power consumption per control zone |
| `redfish_power_max_consumed_watts` | Peak power consumption per control zone |
| `redfish_power_min_consumed_watts` | Minimum power consumption per control zone |
| `redfish_power_redundancy_health` | Power redundancy group health (three-valued) |
| `redfish_power_supply_health` | Individual PSU health (three-valued) |

**Thermal** (`collectors.thermal: true`) — reads `Chassis[*].Thermal`

Tracks per-sensor temperature readings and health, and per-fan speed and health. Disabled sensors and absent or unnamed fans are skipped.

| Metric | Description |
|---|---|
| `redfish_temperature_celsius` | Temperature reading in Celsius |
| `redfish_temperature_health` | Temperature sensor health (binary) |
| `redfish_fan_rpm` | Fan speed in RPM |
| `redfish_fan_health` | Fan health (binary) |

**Systems** (`collectors.systems: true`) — reads `Systems[*]`

Emits a metadata info metric and overall health rollup per system.

| Metric | Description |
|---|---|
| `redfish_system_info` | System identity info (serial, manufacturer, model); always 1 |
| `redfish_system_health_rollup` | Overall system health rollup (binary) |

**Memory** (`collectors.memory: true`) — reads `Systems[*].Memory[*]`

Tracks capacity and health per DIMM slot. Absent and disabled modules are skipped entirely. Modules in `StandbySpare` state emit capacity but no status metric.

| Metric | Description |
|---|---|
| `redfish_memory_capacity_mib` | DIMM capacity in MiB |
| `redfish_memory_health` | DIMM health (binary) |

### Metric reference

| Metric | Health encoding | Labels |
|---|---|---|
| `redfish_system_info` | — | `target`, `system_id`, `serial_number`, `manufacturer`, `model` |
| `redfish_system_health_rollup` | binary | `target`, `system_id`, `health` |
| `redfish_temperature_health` | binary | `target`, `chassis_id`, `sensor`, `physical_context`, `health` |
| `redfish_fan_health` | binary | `target`, `chassis_id`, `fan`, `physical_context`, `health` |
| `redfish_memory_health` | binary | `target`, `system_id`, `memory_id`, `memory_location`, `health` |
| `redfish_power_redundancy_health` | binary | `target`, `chassis_id`, `name`, `member_id`, `health` |
| `redfish_power_supply_health` | binary | `target`, `chassis_id`, `name`, `member_id`, `health` |

Disabled or absent components are skipped entirely — they produce no metric rather than a zero value.

## License

See LICENSE file for details.

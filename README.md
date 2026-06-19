# Redfish Exporter

> **Experimental:** This project is under active development. Metrics, configuration, and behaviour may change without notice between versions.

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
    auth:
      username: admin
      password: secret
    collectors:
      thermal: true
      power: true
      systems: true
      memory: true
      network: true
      managers: true
    insecure: false
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

**Power** (`collectors.power: true`) — reads `Chassis[*]` and `Chassis[*].Power`

Tracks chassis-level health, power state, per-control-zone capacity and consumption (current, average, min, max), power control domain health, redundancy group health, and individual PSU health.

| Metric | Description |
|---|---|
| `redfish_power_state` | Chassis power state (1=On, 0=Off) |
| `redfish_chassis_health` | Chassis health |
| `redfish_chassis_health_rollup` | Chassis rollup health including all subordinate resources |
| `redfish_power_capacity_watts` | Max power capacity per control zone |
| `redfish_power_consumed_watts` | Current power consumption per control zone |
| `redfish_power_average_consumed_watts` | Average power consumption per control zone |
| `redfish_power_max_consumed_watts` | Peak power consumption per control zone |
| `redfish_power_min_consumed_watts` | Minimum power consumption per control zone |
| `redfish_power_control_health` | Power control domain health |
| `redfish_power_redundancy_health` | Power redundancy group health |
| `redfish_power_supply_health` | Individual PSU health |

**Thermal** (`collectors.thermal: true`) — reads `Chassis[*].Thermal`

Tracks per-sensor temperature readings and health, and per-fan speed and health. Disabled and absent sensors are skipped. Fans with no RPM reading still emit a health metric.

| Metric | Description |
|---|---|
| `redfish_temperature_celsius` | Temperature reading in Celsius |
| `redfish_temperature_health` | Temperature sensor health |
| `redfish_fan_rpm` | Fan speed in RPM |
| `redfish_fan_health` | Fan health |

**Systems** (`collectors.systems: true`) — reads `Systems[*]`

Emits system identity, overall health, and summary health and counts for processors and memory.

| Metric | Description |
|---|---|
| `redfish_system_info` | System identity info (serial, manufacturer, model); always 1 |
| `redfish_system_health` | System health |
| `redfish_system_health_rollup` | Overall system rollup health including sub-components |
| `redfish_system_processor_count` | Number of physical processors |
| `redfish_system_processor_health` | Processor summary health |
| `redfish_system_processor_health_rollup` | Processor rollup health |
| `redfish_system_memory_gib` | Total system memory in GiB |
| `redfish_system_memory_health` | Memory summary health |
| `redfish_system_memory_health_rollup` | Memory rollup health |

**Memory** (`collectors.memory: true`) — reads `Systems[*].Memory[*]`

Tracks capacity and health per DIMM slot. Absent and disabled modules are skipped entirely.

| Metric | Description |
|---|---|
| `redfish_memory_capacity_mib` | DIMM capacity in MiB |
| `redfish_memory_health` | DIMM health |

**Managers** (`collectors.managers: true`) — reads `Managers[*]`

Exposes BMC/management controller identity and health. Useful for tracking firmware versions and BMC availability.

| Metric | Description |
|---|---|
| `redfish_manager_info` | BMC identity (type, model, firmware version, manufacturer); always 1 |
| `redfish_manager_health` | BMC health |

**Network** (`collectors.network: true`) — reads `Systems[*].EthernetInterfaces[*]` and `Chassis[*].NetworkAdapters[*]`

Tracks per-NIC link state, speed, and health from the system's ethernet interfaces, plus health of physical network adapters on the chassis. Absent interfaces are skipped. Speed is only emitted when non-zero.

| Metric | Description |
|---|---|
| `redfish_network_interface_link_up` | Whether the NIC link is up (1=LinkUp, 0=other) |
| `redfish_network_interface_speed_mbps` | Current link speed in Mbps |
| `redfish_network_interface_health` | NIC health |
| `redfish_network_adapter_health` | Physical network adapter health |

### Metric reference

| Metric | Labels |
|---|---|
| `redfish_chassis_health` | `target`, `chassis_id`, `health` |
| `redfish_chassis_health_rollup` | `target`, `chassis_id`, `health` |
| `redfish_power_state` | `target`, `chassis_id` |
| `redfish_power_capacity_watts` | `target`, `chassis_id`, `name`, `member_id` |
| `redfish_power_consumed_watts` | `target`, `chassis_id`, `name`, `member_id` |
| `redfish_power_average_consumed_watts` | `target`, `chassis_id`, `name`, `member_id` |
| `redfish_power_max_consumed_watts` | `target`, `chassis_id`, `name`, `member_id` |
| `redfish_power_min_consumed_watts` | `target`, `chassis_id`, `name`, `member_id` |
| `redfish_power_control_health` | `target`, `chassis_id`, `name`, `member_id`, `health` |
| `redfish_power_redundancy_health` | `target`, `chassis_id`, `name`, `member_id`, `health` |
| `redfish_power_supply_health` | `target`, `chassis_id`, `name`, `member_id`, `health` |
| `redfish_temperature_celsius` | `target`, `chassis_id`, `sensor`, `physical_context` |
| `redfish_temperature_health` | `target`, `chassis_id`, `sensor`, `physical_context`, `health` |
| `redfish_fan_rpm` | `target`, `chassis_id`, `fan`, `physical_context`, `unit` |
| `redfish_fan_health` | `target`, `chassis_id`, `fan`, `physical_context`, `health` |
| `redfish_system_info` | `target`, `system_id`, `serial_number`, `manufacturer`, `model` |
| `redfish_system_health` | `target`, `system_id`, `health` |
| `redfish_system_health_rollup` | `target`, `system_id`, `health` |
| `redfish_system_processor_count` | `target`, `system_id` |
| `redfish_system_processor_health` | `target`, `system_id`, `health` |
| `redfish_system_processor_health_rollup` | `target`, `system_id`, `health` |
| `redfish_system_memory_gib` | `target`, `system_id` |
| `redfish_system_memory_health` | `target`, `system_id`, `health` |
| `redfish_system_memory_health_rollup` | `target`, `system_id`, `health` |
| `redfish_memory_capacity_mib` | `target`, `system_id`, `memory_id`, `memory_location` |
| `redfish_memory_health` | `target`, `system_id`, `memory_id`, `memory_location`, `health` |
| `redfish_manager_info` | `target`, `manager_id`, `manager_type`, `model`, `firmware_version`, `manufacturer` |
| `redfish_manager_health` | `target`, `manager_id`, `health` |
| `redfish_network_interface_link_up` | `target`, `system_id`, `interface_id`, `mac_address` |
| `redfish_network_interface_speed_mbps` | `target`, `system_id`, `interface_id`, `mac_address` |
| `redfish_network_interface_health` | `target`, `system_id`, `interface_id`, `mac_address`, `health` |
| `redfish_network_adapter_health` | `target`, `chassis_id`, `adapter_id`, `manufacturer`, `model`, `health` |

Disabled or absent components are skipped entirely — they produce no metric rather than a zero value. Optional fields (e.g. `HealthRollup`, `ProcessorSummary`) are also skipped when not provided by the BMC.

## License

MIT — see [LICENSE](LICENSE) for details.

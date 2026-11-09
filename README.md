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

### Source selection and fallback

Modern Redfish services expose power and thermal data through several
resources: chassis `Sensors`, `ThermalSubsystem` (with its `Fans` collection
and `ThermalMetrics`), `PowerSubsystem` (with its power supplies and their
metrics resources), and `EnvironmentMetrics`. Older services expose the
deprecated `Thermal` and `Power` resources; many expose both.

The exporter discovers capabilities from the links each chassis reports and
selects a source **per chassis and per metric family** — there is no global
legacy/modern switch. Modern sources are preferred; a deprecated resource is
used when the modern sources for a family are absent, return no readings, or
are not implemented (404). An empty legacy response therefore cannot hide
modern readings, and a chassis without modern resources keeps its legacy
output unchanged.

When the same measurement is represented in several places — a sensor
resource, an excerpt with a `DataSourceUri`, and a legacy resource — only one
representation is exported: the first source in the family's ladder that
yields a reading. `DataSourceUri` links are used for deduplication, never
name matching. Sensors claimed by an emitted excerpt (for example the input
voltage sensor referenced by a PSU metrics resource) are not re-exported in
the generic sensor families, and a PSU temperature sensor is reported for the
supply rather than as a chassis temperature when the power collector runs.
Distinct sensors that share a name are disambiguated with their stable ID
(`sensor="CPU Temp (TempB)"`) instead of being merged or dropped.

`EnvironmentMetrics` is license-restricted on some vendors (for example
Supermicro requires SFT-DCMS-SINGLE); an absent or 404 resource is treated as
a capability gap and does not affect the rest of the collection. The
integration test fixtures are synthetic mockups based on the published DMTF
schemas and Supermicro's documented resource shapes; live H13/H14 hardware
remains unverified.

### Missing data, errors, and compatibility

- Optional resources that a service does not link, or returns 404 for, are
  capability gaps: collection continues and `redfish_collector_success`
  stays 1.
- Any other fetch failure — authorization errors, timeouts, malformed
  responses, server errors, or a collection member that cannot be fetched —
  is logged and sets `redfish_collector_success` to 0 for the affected
  collector while everything else that could be collected is still emitted.
  A value of 1 means the scrape collected every resource the service
  advertised.
- Readings absent from a response are skipped, never exported as zero. Real
  zero and negative readings are preserved as-is.
- Fans reporting percent units are exported as `redfish_fan_speed_percent`
  and never as RPM; RPM is never inferred from percent.
- Source selection happens per metric family, not per reading: when a modern
  source yields any reading for a family, the whole family comes from that
  source. On chassis exposing both legacy and modern resources, series labels
  therefore change from legacy names (for example `sensor="CPU1 Temp"`) to
  sensor names (`sensor="CPU1Temp"`), and a legacy-only reading in a family
  whose modern source won is not emitted separately. Legacy-only services
  keep their existing series unchanged.

### Collectors

Each collector is enabled per-module in the config under `collectors:`.

**Power** (`collectors.power: true`) — reads `Chassis[*]` state, `Sensors`, `PowerSubsystem` (with power supply metrics), `EnvironmentMetrics`, and the deprecated `Power` resource

Tracks chassis-level health, power state, capacity and consumption per control zone, power subsystem health, redundancy group health, per-PSU health and input/output measurements, and generic power, voltage, and current sensor readings.

| Metric | Description |
|---|---|
| `redfish_power_state` | Chassis power state (1=On, 0=Off) |
| `redfish_chassis_health` | Chassis health |
| `redfish_chassis_health_rollup` | Chassis rollup health including all subordinate resources |
| `redfish_power_capacity_watts` | Max power capacity per control zone or subsystem |
| `redfish_power_consumed_watts` | Current power consumption per control zone or chassis |
| `redfish_power_average_consumed_watts` | Average power consumption per control zone |
| `redfish_power_max_consumed_watts` | Peak power consumption per control zone |
| `redfish_power_min_consumed_watts` | Minimum power consumption per control zone |
| `redfish_power_control_health` | Power control domain health |
| `redfish_power_redundancy_health` | Power redundancy group health |
| `redfish_power_supply_health` | Individual PSU health |
| `redfish_power_subsystem_health` | Power subsystem health |
| `redfish_power_supply_input_watts` | PSU input power in Watts |
| `redfish_power_supply_output_watts` | PSU output power in Watts |
| `redfish_power_supply_input_volts` | PSU input voltage in Volts |
| `redfish_power_supply_input_amps` | PSU input current in Amperes |
| `redfish_power_supply_temperature_celsius` | PSU temperature in Celsius |
| `redfish_sensor_power_watts` | Power sensor reading not reported through another metric |
| `redfish_sensor_voltage_volts` | Voltage sensor reading not reported through another metric |
| `redfish_sensor_current_amps` | Current sensor reading not reported through another metric |
| `redfish_sensor_power_threshold_watts` | Power sensor threshold in Watts |
| `redfish_sensor_voltage_threshold_volts` | Voltage sensor threshold in Volts |
| `redfish_sensor_current_threshold_amps` | Current sensor threshold in Amperes |

Capacity source order: `PowerSubsystem.CapacityWatts` → legacy power control zones. A subsystem reporting `State: Absent` (for example wall-powered Supermicro boards) is a capability gap, not a zero capacity. Consumption source order: `EnvironmentMetrics.PowerWatts` → legacy zones. PSU health: subsystem power supplies → legacy supplies. PSU readings: PSU metrics resource → legacy supply fields. Redundancy: subsystem redundancy groups → legacy redundancy. Power capacity, allocation, and PSU input/output are never conflated: capacity is not consumption, and PSU measurements are not chassis totals.

**Thermal** (`collectors.thermal: true`) — reads `Chassis[*]` `Sensors`, `ThermalSubsystem` (fans and thermal metrics), `EnvironmentMetrics`, and the deprecated `Thermal` resource

Tracks per-sensor temperature readings, health, and thresholds; per-fan speed (RPM and percent), health, and thresholds; and relative humidity.

| Metric | Description |
|---|---|
| `redfish_temperature_celsius` | Temperature reading in Celsius |
| `redfish_temperature_health` | Temperature sensor health |
| `redfish_temperature_threshold_celsius` | Temperature sensor threshold in Celsius |
| `redfish_fan_rpm` | Fan speed in RPM |
| `redfish_fan_health` | Fan health |
| `redfish_fan_threshold_rpm` | Fan sensor threshold in RPM |
| `redfish_fan_speed_percent` | Fan speed in percent (0-100) |
| `redfish_humidity_percent` | Relative humidity in percent |
| `redfish_humidity_threshold_percent` | Humidity sensor threshold in percent |
| `redfish_thermal_subsystem_health` | Thermal subsystem health |

Threshold metrics carry a `level` label: `lower_fatal`, `lower_critical`, `lower_caution`, `upper_caution`, `upper_critical`, `upper_fatal`. Temperature source order: `Sensors` → `ThermalMetrics` excerpts → `EnvironmentMetrics` → legacy `Thermal`. Fan RPM: `Sensors` → fan resource excerpts → legacy fans. Fan health: fan resources → `Sensors` → legacy fans. Fan speed percent: fan resources → `EnvironmentMetrics` → legacy percent-unit fans. Humidity: `Sensors` → `EnvironmentMetrics`.

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
| `redfish_power_subsystem_health` | `target`, `chassis_id`, `health` |
| `redfish_thermal_subsystem_health` | `target`, `chassis_id`, `health` |
| `redfish_power_capacity_watts` | `target`, `chassis_id`, `name`, `member_id` |
| `redfish_power_consumed_watts` | `target`, `chassis_id`, `name`, `member_id` |
| `redfish_power_average_consumed_watts` | `target`, `chassis_id`, `name`, `member_id` |
| `redfish_power_max_consumed_watts` | `target`, `chassis_id`, `name`, `member_id` |
| `redfish_power_min_consumed_watts` | `target`, `chassis_id`, `name`, `member_id` |
| `redfish_power_control_health` | `target`, `chassis_id`, `name`, `member_id`, `health` |
| `redfish_power_redundancy_health` | `target`, `chassis_id`, `name`, `member_id`, `health` |
| `redfish_power_supply_health` | `target`, `chassis_id`, `name`, `member_id`, `health` |
| `redfish_power_supply_input_watts` | `target`, `chassis_id`, `name`, `member_id` |
| `redfish_power_supply_output_watts` | `target`, `chassis_id`, `name`, `member_id` |
| `redfish_power_supply_input_volts` | `target`, `chassis_id`, `name`, `member_id` |
| `redfish_power_supply_input_amps` | `target`, `chassis_id`, `name`, `member_id` |
| `redfish_power_supply_temperature_celsius` | `target`, `chassis_id`, `name`, `member_id` |
| `redfish_sensor_power_watts` | `target`, `chassis_id`, `sensor`, `physical_context` |
| `redfish_sensor_voltage_volts` | `target`, `chassis_id`, `sensor`, `physical_context` |
| `redfish_sensor_current_amps` | `target`, `chassis_id`, `sensor`, `physical_context` |
| `redfish_sensor_power_threshold_watts` | `target`, `chassis_id`, `sensor`, `physical_context`, `level` |
| `redfish_sensor_voltage_threshold_volts` | `target`, `chassis_id`, `sensor`, `physical_context`, `level` |
| `redfish_sensor_current_threshold_amps` | `target`, `chassis_id`, `sensor`, `physical_context`, `level` |
| `redfish_temperature_celsius` | `target`, `chassis_id`, `sensor`, `physical_context` |
| `redfish_temperature_health` | `target`, `chassis_id`, `sensor`, `physical_context`, `health` |
| `redfish_temperature_threshold_celsius` | `target`, `chassis_id`, `sensor`, `physical_context`, `level` |
| `redfish_fan_rpm` | `target`, `chassis_id`, `fan`, `physical_context`, `unit` |
| `redfish_fan_health` | `target`, `chassis_id`, `fan`, `physical_context`, `health` |
| `redfish_fan_threshold_rpm` | `target`, `chassis_id`, `fan`, `physical_context`, `level` |
| `redfish_fan_speed_percent` | `target`, `chassis_id`, `fan`, `physical_context` |
| `redfish_humidity_percent` | `target`, `chassis_id`, `sensor`, `physical_context` |
| `redfish_humidity_threshold_percent` | `target`, `chassis_id`, `sensor`, `physical_context`, `level` |
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

### Known limitations

- Energy counters (`EnergykWh`), PSU capacity ratings, PSU output rails and fan speeds, secondary fan rotor speeds, and `PowerSubsystem.Allocation` are not exported.
- Bare percent-type sensors are not exported: a percent reading is only exported when a fan resource or `EnvironmentMetrics` identifies it as a fan speed.
- Legacy `Thermal` thresholds and legacy supply health-rollup values are not exported; thresholds come from `Sensor` resources.
- When a chassis has legacy `Power` zones but no `EnvironmentMetrics` (for example a hybrid Supermicro without the DCSM license), a total-power sensor may appear both as a legacy zone in `redfish_power_consumed_watts` and as a generic `redfish_sensor_power_watts` reading; the two representations cannot be linked without name matching.

## License

MIT — see [LICENSE](LICENSE) for details.

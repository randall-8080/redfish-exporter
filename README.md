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

## License

See LICENSE file for details.
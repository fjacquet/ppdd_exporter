# Configuration

The exporter reads a YAML file (default `config.yaml`). Passwords support `${ENV_VAR}`
interpolation or a `passwordFile` reference.

```yaml
server:
  host: "0.0.0.0"
  port: "9099"
  uri: "/metrics"
  logName: ""            # "" -> stdout
collection:
  interval: "5m"          # DD stats are slow-moving
  timeout: "60s"          # per-system collection timeout
systems:
  - name: dd-prod-01
    host: dd01.example.com  # :3009 implied
    username: ppdd-monitor  # a read-only/monitor DD user suffices
    password: "${DD01_PASSWORD}"
    insecureSkipVerify: true
```

| Key | Default | Notes |
|---|---|---|
| `server.port` | `9099` | metrics/health port |
| `collection.interval` | `5m` | poll cadence |
| `collection.timeout` | `60s` | per-system timeout |
| `systems[].port` | `3009` | DD REST API port |

Config reloads on **SIGHUP** or file change (restart to apply system/client changes).

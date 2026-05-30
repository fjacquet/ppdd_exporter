# Configuration

The exporter reads a YAML file (default `config.yaml`). Passwords support `${ENV_VAR}`
interpolation or a `passwordFile` reference. A `${VAR}` whose environment variable is
unset is a **load error** (fail fast), not a silent empty password.

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
    # insecureSkipVerify: true   # see warning below
```

| Key | Default | Notes |
|---|---|---|
| `server.port` | `9099` | metrics/health port |
| `collection.interval` | `5m` | poll cadence |
| `collection.timeout` | `60s` | per-system timeout |
| `systems[].port` | `3009` | DD REST API port |
| `systems[].insecureSkipVerify` | `false` | disables TLS verification — see below |

!!! warning "`insecureSkipVerify`"
    Setting this to `true` disables TLS certificate verification, exposing the
    connection to man-in-the-middle attacks. Leave it off in production; only enable
    it for dev/test against an appliance with a self-signed cert (the Compose demo's
    `config.demo.yaml` uses it to reach the bundled `mockdd`).

## Hot reload

Config reloads on **SIGHUP** or when the file changes (the watcher follows the parent
directory, so atomic "write-temp + rename" updates are picked up too). On a successful
reload the exporter **rebuilds and swaps** its DD clients and collection loop in place,
applying changes to `systems` and `collection` (interval/timeout) without a restart;
`/metrics` and `/health` keep serving the last snapshot across the swap. Changes to the
`server` section (host/port/uri) still require a restart and are logged as such. A reload
that fails to load/validate is logged and dropped — the running config stays in effect.

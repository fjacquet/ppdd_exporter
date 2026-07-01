# Configuration

The exporter reads a YAML file (default `config.yaml`). The `name`, `host`, `username`,
and `password` fields all support `${ENV_VAR}` interpolation; `password` additionally
accepts a `passwordFile` reference. A `${VAR}` whose environment variable is unset is a
**load error** (fail fast), not a silent misconfiguration.

```yaml
server:
  host: "0.0.0.0"
  port: "9441"
  uri: "/metrics"
  logName: ""            # "" -> stdout
collection:
  interval: "5m"          # DD stats are slow-moving
  timeout: "60s"          # per-system collection timeout
systems:
  - name: dd-prod-01
    host: "${PPDD1_HOSTNAME}"    # or a literal: dd01.example.com — :3009 implied
    username: "${PPDD1_USERNAME}"  # a read-only/monitor DD user suffices
    password: "${PPDD1_PASSWORD}"
    # insecureSkipVerify: true   # see warning below
```

!!! tip "Single-system env convenience vs multi-system `config.yaml`"
    `config.yaml` is the source of truth — one `systems` entry per appliance. Using
    `${ENV_VAR}` for `host`, `username`, and `password` is a convenience for
    single-appliance deployments where you prefer to keep all secrets in a `.env` file
    (gitignored) rather than in `config.yaml`. For multi-appliance setups use distinct
    variable names per system (e.g. `PPDD1_HOSTNAME`, `PPDD2_HOSTNAME`) or supply literal
    values directly in `config.yaml`.

| Key | Default | Notes |
|---|---|---|
| `server.port` | `9441` | metrics/health port |
| `collection.interval` | `5m` | poll cadence |
| `collection.timeout` | `60s` | per-system timeout |
| `systems[].port` | `3009` | DD REST API port |
| `systems[].insecureSkipVerify` | `false` | disables TLS verification — see below |

!!! warning "`insecureSkipVerify`"
    Setting this to `true` disables TLS certificate verification, exposing the
    connection to man-in-the-middle attacks. Leave it off in production; only enable
    it for dev/test against an appliance with a self-signed cert (the Compose demo's
    `config.demo.yaml` uses it to reach the bundled `mockdd`).

## Secrets

`${ENV_VAR}` references are interpolated in **name**, **host**, **username**, and
**password** at config-load time. A referenced variable that is not set causes an immediate error
(fail fast — a typo in a secret name shows up at startup, not as repeated auth
failures).

Passwords additionally support a file-based alternative:

1. `${ENV_VAR}` inside `password` — variable must be set.
2. `passwordFile` — read and trimmed when `password` resolves empty.

### Passwords with special characters

Any character is safe end to end — the password is sent in a JSON request body, so
nothing needs URL-encoding. The only place quoting matters is **parsing at load time**,
and it differs by where you put the password:

| Source | Rule |
|---|---|
| `.env`, single-quoted `'…'` | Fully literal — no `$` expansion, no `\` escapes, no `#` comment. Best default. Cannot contain a literal `'`. |
| `.env`, double-quoted `"…"` | Expands `$VAR`/`${VAR}` and processes `\` escapes. `$`, `\`, `"` are special — write `\$`, `\\`, `\"`. |
| `.env`, unquoted | `$VAR` expands; a ` #` (space-hash) starts a comment; a value **starting** with `'`/`"` is treated as quoted. |
| `config.yaml` inline | Only the exact `${NAME}` token is interpolated (`os.LookupEnv`), so a literal password containing `${NAME}` is treated as an env ref. Prefer referencing an env var. |
| `passwordFile` | Read **verbatim** (only surrounding whitespace trimmed) — no interpolation, no escaping. The bulletproof option. |

For quotes inside the password specifically: use double quotes to include a `'`, single
quotes to include a `"`. If the password has **both** `'` and `"` (or a `\`, or starts
with a quote), use `passwordFile` — it needs no escaping at all. When referencing an env
var from `config.yaml` (`password: "${PPDD1_PASSWORD}"`) the value is inserted verbatim
and never re-scanned, so the env var itself may contain `$`, `${…}`, or any character.

### .env loading

The exporter binary loads a `.env` file natively at startup — from the working
directory first, then next to the config file — so `cp .env.example .env` works
for bare-metal and systemd runs exactly like it does under docker compose.
Already-set environment variables **always take precedence** over `.env` values,
so secret injection (systemd `Environment=`, Kubernetes secrets, CI) can never be
shadowed by a stray file.

## Hot reload

Config reloads on **SIGHUP** or when the file changes (the watcher follows the parent
directory, so atomic "write-temp + rename" updates are picked up too). On a successful
reload the exporter **rebuilds and swaps** its DD clients and collection loop in place,
applying changes to `systems` and `collection` (interval/timeout) without a restart;
`/metrics` and `/health` keep serving the last snapshot across the swap. Changes to the
`server` section (host/port/uri) still require a restart and are logged as such. A reload
that fails to load/validate is logged and dropped — the running config stays in effect.

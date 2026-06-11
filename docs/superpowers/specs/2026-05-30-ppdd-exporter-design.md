# PPDD Exporter — Design Spec

**Date:** 2026-05-30
**Status:** Approved (design); pending implementation plan
**Author:** Fred Jacquet (with Claude Code)

## Summary

A Go Prometheus exporter for **Dell PowerProtect DD (Data Domain)** appliances. One
process monitors **many DD systems**, polling each on an interval, publishing an
immutable snapshot, and serving it at `/metrics`. It mirrors the operational shape of the
sibling [`pflex_exporter`](https://github.com/fjacquet/pflex_exporter) — snapshot model,
multi-system, hot config reload, `/health` — **minus the OTLP export path**, which is
deferred to a later milestone.

Coverage spans four DD metric domains: **Capacity & dedup, MTrees, Replication, and
Health & ops**.

### Decisions locked in brainstorming

| Decision | Choice | Rationale |
|---|---|---|
| Architecture | Prometheus-only, snapshot model | Keep pflex's clean background-loop/multi-system/hot-reload design; defer OTLP |
| Connection model | Direct to each DD appliance (`:3009/api/v1`) | The linked Dell API (4118 v8.3.0) is the per-appliance DD REST API, not DDMC |
| Metric scope | Capacity & dedup, MTrees, Replication, Health & ops | Full DD coverage |
| Internal structure | **A — modular per-domain collectors** | Isolates "docs-only" endpoint/field risk to one file each; trivial phasing; per-module tests |
| Fixture source | Dell docs only | Field mappings modeled from documented schema; **provisional until validated against a live DD** |

## Target API

- **API:** Dell PowerProtect DD Series Appliances REST API (developer.dell.com API **4118**, version **8.3.0**).
- **DD OS:** 7.2+ only (the `/api/...` path scheme). Pre-7.2 (`/rest/...`) is out of scope.
- **Base URL:** `https://<dd-host>:3009/api/v1/<resource>`.
- **Auth:** token-based. `POST /api/v1/auth` with credentials returns an
  `X-DD-AUTH-TOKEN` response header; that token is sent on subsequent requests until it is
  rejected (401), at which point the client re-authenticates. `DELETE /api/v1/auth` on
  shutdown to release the session.
- **System addressing:** resources live under `/dd-systems/{id}/...`; `0` (or the system's
  own id from `GET /api/v1/dd-systems`) addresses the local appliance.

> ⚠️ **Provisional endpoints.** Because fixtures are modeled from docs only, the exact
> paths and JSON field names below are best-effort and **must be validated against a real
> DD appliance**. The modular design (below) contains each correction to a single file.

## Architecture

### Snapshot model (central design choice)

A single background `Collector` loop polls every configured DD system in parallel on
`collection.interval`, builds an immutable `Snapshot`, and pointer-swaps it into a
`SnapshotStore` (RWMutex). Both the `/metrics` handler and `/health` read the latest
snapshot rather than fetching on scrape — decoupling DD API load from the number of
scrapers. A synchronous startup cycle populates the store before serving; `--once` runs a
single cycle and exits.

Graceful degradation: one system's (or one module's) failure never blocks the others.

### Per-system client (`internal/ddclient`)

One client per appliance, resty-based:

- **Auth:** `POST /api/v1/auth` → capture `X-DD-AUTH-TOKEN`; attach it to every request;
  on 401 re-login once and retry; `DELETE /api/v1/auth` on `Close()`.
- **Retry policy excludes 4xx** — never retry auth/permission failures (carried over from
  pflex; do not add a blanket retry-after-error condition).
- `insecureSkipVerify` configurable per system.
- A `Client` interface is defined so the collector can run against an `httptest` mock.

### Modular collectors (Approach A)

```go
type ResourceCollector interface {
    Name() string                                       // "capacity", "mtrees", ...
    Collect(ctx context.Context, c ddclient.Client) ([]Sample, error)
}
```

The per-system cycle iterates the registered collectors; each owns its endpoint path,
response struct, and `parse → []Sample` logic. A failing module degrades to an empty slice
plus a `ppdd_collector_up{collector="<name>"}` 0/1 signal — it never crashes the cycle.

| Module | Endpoint (provisional) | Key metrics |
|---|---|---|
| `capacity` | `…/dd-systems/{id}/file-system` (df / space-usage) | `ppdd_filesystem_total_bytes`, `_used_bytes`, `_available_bytes`; `ppdd_compression_global_factor`, `_local_factor`, `_total_factor`; cleaning/GC state + last-run |
| `mtrees` | `…/dd-systems/{id}/mtrees` | per-mtree `ppdd_mtree_logical_used_bytes`, `_physical_used_bytes`, quota soft/hard limit bytes, compression factor |
| `replication` | `…/dd-systems/{id}/replications` | per-context `ppdd_replication_state` (enum gauge), `_precomp_bytes_remaining`, sync lag seconds, throughput bytes/sec |
| `health` | `…/hardware/*`, `…/protocols/*`, `…/alerts`, `…/stats/system-stats` | disk/PSU/fan/NVRAM state gauges; active stream counts vs. model limit; CPU utilization; `ppdd_alerts_active{severity}` |

### HTTP surface (`main.go`)

- `GET /metrics` — Prometheus exposition from the latest snapshot (unchecked collector;
  `Describe` emits nothing so the metric-name set can be dynamic).
- `GET /health` — snapshot freshness + per-system last-scrape status.
- Wires the server, the collection loop, and hot config reload (SIGHUP + file watch).

## Metric & label conventions

- Namespace prefix **`ppdd_`**; **unit-explicit** names (`_bytes`, `_factor`, `_seconds`,
  `_bytes_per_second`) following pflex's Gen2 convention.
- Every metric carries a **`system`** label (the configured DD name). Per-module identity
  labels: `mtree`, `context`, `disk`, `enclosure`, `severity`, etc.
- States are exposed as **enum gauges** (e.g. `ppdd_replication_state{state="..."} 1`) and
  numeric where natural.
- **Label-key consistency (load-bearing):** a metric name must carry one label-key set
  across all series. Where a metric is emitted from more than one path, builders emit a
  union label set in a fixed canonical order with empty values for inapplicable keys. A
  test guards this.

## Configuration (`config.yaml`)

Same shape as pflex, with `systems:` replacing `clusters:`:

```yaml
server:
  host: "0.0.0.0"
  port: "9099"            # penciled to avoid colliding with pflex's 2112; easily changed
  uri: "/metrics"
  logName: "/var/log/ppdd_exporter/ppdd-exporter.log"   # "" -> stdout

collection:
  interval: "5m"           # DD stats are slow-moving; 5 min is ample
  timeout: "60s"           # per-system collection timeout

systems:
  - name: dd-prod-01
    host: dd01.example.com   # :3009 implied
    username: ppdd-monitor
    password: "${PPDD1_PASSWORD}"   # or passwordFile: /etc/ppdd_exporter/dd01.pass
    insecureSkipVerify: true
```

- `${ENV_VAR}` interpolation and `passwordFile` references (reuse pflex's
  `internal/utils/env` + `internal/models/safe_config`).
- Hot reload via SIGHUP + file watch (reuse pflex's `internal/config/watcher`).
- A `monitor`/read-only DD user is sufficient.

## Project layout & tooling

- **Go 1.26**, same dependency set as pflex: `go-resty/resty`,
  `prometheus/client_golang`, `spf13/cobra`, `sirupsen/logrus`, `fsnotify/fsnotify`,
  `gopkg.in/yaml.v2`. (No OTLP deps in this milestone.)
- **Layout:**
  - `main.go` — CLI (cobra), wiring, HTTP server, reload.
  - `internal/ddclient/` — client, auth, interface, mock.
  - `internal/ppdd/` — `ResourceCollector` interface, the four modules, snapshot,
    Prometheus collector, sample model.
  - `internal/config/`, `internal/models/`, `internal/logging/`, `internal/utils/` —
    carried over / adapted from pflex.
- **Tooling carried over:** `Makefile` (`make sure`, `make ci`), `golangci-lint`,
  `govulncheck`, CycloneDX SBOM, Semgrep-on-write hook (no inline suppression — restructure
  instead), Dockerfile with a non-root `USER`, MkDocs Material docs, GitHub Actions
  (`ci.yml`, `release.yml`, `docs.yml`). **Apache-2.0** license.

### Documentation & changelog (part of "done")

Documentation is maintained alongside the code, not deferred:

- **MkDocs Material site** (`mkdocs.yml` + `docs/`): home, getting-started
  (installation/configuration/quickstart), metrics reference, deployment — published to GitHub
  Pages via `docs.yml`.
- **`README.md`**, a project **`CLAUDE.md`** capturing the load-bearing conventions, and an
  **Apache-2.0 `LICENSE`**.
- **`CHANGELOG.md`** in [Keep a Changelog](https://keepachangelog.com/) + SemVer format:
  maintained from the first commit with an `[Unreleased]` section that gains an entry as each
  phase/feature lands; releases cut a dated version section (first tag `v0.1.0`).
- **Rule:** any change that adds or alters a user-visible feature updates the relevant docs and
  the `CHANGELOG.md` `[Unreleased]` section in the **same commit**.

## Testing

- `httptest` TLS **mock DD** serving `/api/v1/auth` and each module's endpoint from
  `testdata/*.json` fixtures, built from documented field shapes (flagged provisional).
- Per-module `parse → []Sample` unit tests against fixtures.
- Collector test asserts results via a Prometheus registry gather.
- Mixed-system label-key-consistency test (the load-bearing rule).
- Test handlers write fixtures through a `writeBytes(io.Writer, …)` helper to satisfy the
  Semgrep "write-to-ResponseWriter" rule.
- Auth flow test: login captures token; 401 triggers exactly one re-login; retry excludes
  4xx.

## Phased delivery

1. **Skeleton + auth + capacity** — `ddclient` (auth, interface, mock), snapshot store,
   `/metrics`, `/health`, config + reload, and the `capacity` module end-to-end. Proves the
   whole pipeline (the MVP).
2. **MTrees** — add the `mtrees` module + fixtures + tests.
3. **Replication** — add the `replication` module + fixtures + tests.
4. **Health & ops** — add the `health` module (hardware, protocols/streams, alerts,
   system perf) + fixtures + tests.
5. **Polish** — MkDocs site, CI/release workflows, Docker image, README, dashboards.

## Out of scope (this milestone)

- OTLP metric push (deferred; the snapshot model already supports adding it later).
- DDMC / PowerProtect Data Manager aggregation (direct-to-appliance only).
- Pre-7.2 DD OS (`/rest/...` path scheme) and legacy Basic auth.
- Write/management operations — the exporter is strictly read-only.

## Known risks

- **Docs-only field mappings** — endpoint paths and JSON field names are provisional until
  validated against a live DD. Mitigated by the modular design (one file per domain) and
  fixture-driven tests that make corrections cheap.
- **Stream/limit metrics** may require model-specific capability data; if a documented
  source isn't available, those land as a follow-up within the `health` module.

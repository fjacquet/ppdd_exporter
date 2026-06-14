# Changelog

All notable changes to this project are documented here. The format is based on
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project adheres to
[Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.7.1] - 2026-06-14

### Added
- **Grafana dashboard suite.** Replaced the single overview dashboard with five linked,
  auto-provisioned dashboards (all tagged `ppdd`, with a cross-navigation dropdown):
  - `ppdd-overview` — NOC at-a-glance: a fleet KPI row (systems, collectors down, failed
    disks, critical alerts, replication not-NORMAL, need-resync, GC running, worst used %)
    plus a per-system summary table with inline gauges and status coloring.
  - `ppdd-capacity` — utilization bargauge, used-vs-available and dedup trends, and a
    GC/cleaning state timeline (`ppdd_filesystem_cleaning_running`).
  - `ppdd-mtrees` — per-MTree logical usage, **quota utilization %** (guarded on a non-zero
    hard limit), and a detail table with soft/hard quotas, degraded, and retention-lock.
  - `ppdd-replication` — MTree context table/timeline (state, connected, enabled,
    need-resync) and file replication (network/logical bytes, active files, status).
  - `ppdd-health` — failed-disk detail, alerts by severity/class, CPU, throughput, and the
    collector-up matrix.
  - Surfaces the previously-unused metrics (GC, MTree quotas, replication connected/enabled,
    file-replication logical bytes / active files / status, per-disk failures). Shared
    crosshair tooltip, consistent threshold palette, and panel descriptions throughout.
    Every query was validated end-to-end against the `mockdd` Compose demo.

## [0.7.0] - 2026-06-14

### Changed
- Validated and corrected all DD API mappings against the PowerProtect DD 8.7.0 OpenAPI
  spec (`docs/swagger/13345-8.7.0.json`), now checked in as the source of truth
  (see [ADR-0009](docs/adr/0009-validate-against-8.7.0-openapi.md)):
  - Corrected endpoints: disks → `/api/v1/.../storage/disks`, performance →
    `/api/v3/.../stats/performance`, file-systems path (plural).
  - Fixed fields: alerts array key `alert_list`, mtree quota via `quota_config`,
    disk failed state `FAILED`, mtree compression from the top-level factor.
  - **Breaking:** removed `ppdd_compression_{global,local,total}_factor` (no source on
    8.7.0); replaced the `ppdd_replication_*` metrics with `ppdd_mtree_replication_*`
    (posture) and `ppdd_file_replication_*` (stats). Alert label values now use DD enum
    casing (e.g. `CRITICAL`). The `mockdd` demo and the Grafana overview dashboard were
    repointed to the corrected metric set.
- Upgraded dependencies (`golang.org/x/*`, `prometheus/common`+`procfs`, `pflag`,
  `protobuf`); `govulncheck` reports no advisories.

### Added
- **Windows release builds.** GoReleaser now cross-compiles `windows/amd64` and
  `windows/arm64` alongside linux/darwin; Windows artifacts ship as `.zip` (others
  remain `.tar.gz`).

## [0.6.0] - 2026-06-12

### Added
- **Native `.env` loading at startup (no-override semantics).** `ppdd_exporter` now
  calls `config.LoadDotEnv` before `config.Load`, trying `./.env` then the config
  file's directory. Already-set environment variables always win (godotenv
  no-override), so secret injection via systemd `Environment=`, Kubernetes secrets,
  or CI environment can never be shadowed by a stray `.env` file. Mirrors the
  `obs_exporter` implementation (ADR-0005 pattern).
- **`--trace` flag and `--once --debug` sample dump for live-appliance validation.**
  `--trace` logs every DD API response body (method, URL, status, payload) via a resty
  `OnAfterResponse` hook — request headers are never logged, so the `X-DD-AUTH-TOKEN`
  session token cannot leak, and the auth endpoint's response is skipped entirely.
  `--once --debug` now also prints every collected sample sorted in Prometheus
  exposition style, diffable against `docs/metrics.md`.

## [0.3.0] - 2026-06-05

### Security
- **CI now scans for vulnerabilities and insecure patterns.** The `make ci` gate adds
  `golangci-lint` (pinned `v2.12.2`) and `govulncheck`, and `ci.yml` gains a dedicated
  **Semgrep** job (`--config auto --error`) plus a CycloneDX SBOM artifact job. Known-CVE
  dependencies and flagged code now fail CI before merge.
- **All GitHub Actions are pinned to full commit SHAs** (with `# vX.Y.Z` comments) across
  `ci.yml`, `release.yml`, and `docs.yml`, hardening against mutable-tag repoint attacks.
  Top-level workflow `permissions` are now least-privilege (`contents: read`), with write
  scopes granted only to the jobs that need them.
- Pinned the Dockerfile build stage to `golang:1.26.4`.

### Changed
- **Release pipeline migrated to GoReleaser** (`.goreleaser.yaml`), replacing the
  hand-rolled build matrix and `softprops/action-gh-release`. It owns cross-compilation
  (`linux,darwin × amd64,arm64`), `tar.gz` archives (bundling `LICENSE`, `README.md`,
  `config.yaml`), `checksums.txt`, the CycloneDX SBOM (kept on **cyclonedx-gomod**, so its
  content matches `make sbom`), and the GitHub Release. Reproducible-build flags
  (`-trimpath`, `mod_timestamp`) were added. Releases now ship **both `tar.gz` archives
  and the raw binaries** for each `os/arch`. The multi-arch GHCR image (build-time SBOM + max-mode
  provenance) is retained, now SHA-pinned and tagged via `docker/metadata-action`.
  See [ADR-0008](docs/adr/0008-ci-supply-chain-hardening.md).

### Added
- `.github/dependabot.yml` to keep the SHA-pinned Actions, Go modules, and Docker base
  current (weekly).
- `make tools` / `make tools-sbom` (install dev/CI tooling), `make lint`, `make vuln`,
  `make sbom`, `make release`, and `make release-snapshot` (local GoReleaser dry-run).
- Hardened the DD client TLS config (`MinVersion: tls.VersionTLS12`) so the opt-in
  `insecureSkipVerify` path still negotiates a modern floor.
- **Homebrew cask** published to the `fjacquet/homebrew-tap` tap on each release
  (`brew install --cask fjacquet/tap/ppdd_exporter`; macOS only). Skipped automatically
  until the tap repo and `HOMEBREW_TAP_GITHUB_TOKEN` secret exist.
- [ADR 0008](docs/adr/0008-ci-supply-chain-hardening.md) documenting the scanning,
  SHA-pinning, and GoReleaser migration decisions.

## [0.2.0] - 2026-06-03

### Added
- Architecture Decision Records under `docs/adr/` (MADR format) documenting the snapshot model, modular collectors, provisional API mappings, auth/retry policy, hot-reload swap, and the label-key consistency invariant, plus a template and index wired into the docs nav.
- Metrics `ppdd_compression_factor`, `ppdd_mtree_degraded`, and `ppdd_mtree_retention_lock_enabled`.
- `docker-compose.server.yml` (+ `.env.example`) — a server stack that runs the exporter against a real DD appliance (no `mockdd`), with secrets from a gitignored `.env` and Grafana configured for remote access via `GF_SERVER_ROOT_URL`.

### Changed
- Realigned collectors to the documented PowerProtect DD 7.3 REST API: the `/rest` prefix with per-resource versions (`mtrees` v3.0, `stats` v2.0), list pagination (no more silent truncation past 20 items), active-only alerts with a `class` label, capacity sourced from `/system`, and MTree usage from per-MTree v2.0 stats. Auth now posts the documented flat body to `/rest/v1.0/auth`. Metrics without a source in the guide (compression split, GC cleaning, MTree `physical_used`, quotas) are retained as flagged-provisional best-effort fetches. See [ADR-0007](docs/adr/0007-dd-rest-api-realignment.md).

## [0.1.1] - 2026-05-31

### Added
- Releases now publish a Software Bill of Materials: a CycloneDX SBOM file (`*_sbom.cdx.json`, generated by Syft) is attached to each GitHub release, and the GHCR container image carries SBOM and max-mode provenance attestations.

### Changed
- Bumped all GitHub Actions in the CI/release/docs workflows to current major versions, moving off the deprecated Node 20 runtime.

## [0.1.0] - 2026-05-30

### Security
- The default `config.yaml` no longer enables `insecureSkipVerify` (it is commented out with a warning). Disabling TLS certificate verification exposes the connection to man-in-the-middle attacks and is now opt-in only; the Compose demo keeps it in `config.demo.yaml` where it is needed for `mockdd`.

### Fixed
- `/health` now correctly reports unhealthy systems: `OK` is set to `false` (and `Err` populated) when all collectors fail to produce any data for a system, instead of always reporting healthy.
- `passwordFile` contents are now trimmed of surrounding whitespace (e.g. trailing newline from `echo`) before being used as the system password, preventing silent authentication failures.
- Config watcher startup errors are now logged as warnings instead of being silently swallowed, making misconfiguration easier to diagnose.
- Config loading now **fails fast** when a `${VAR}` password reference points at an unset environment variable, instead of silently collapsing to an empty password that fails authentication at runtime.
- The DD client retry predicate no longer panics on transport/TLS errors: resty passes a `nil` response in that case, which the old `r.StatusCode() >= 500` check dereferenced. Such errors are now retried.
- The collection error message distinguishes "all N collectors failed" from "no domain samples collected", instead of mislabelling the zero-sample case as a total failure.
- `mockdd` fixture endpoints now reject non-GET methods (`405`) and the startup banner logs a reachable `https://localhost:3009` instead of the bind address.

### Changed
- Config hot reload now **applies** changes instead of only logging them: a successful reload rebuilds and swaps the DD clients and collection loop in place (picking up `systems` and `collection` interval/timeout changes), while `server` host/port/uri changes are flagged as restart-required. The shared snapshot keeps `/metrics` and `/health` serving across the swap.
- The config watcher now follows the parent directory instead of the file inode, so atomic "write-temp + rename" config updates (used by many editors and config managers) still trigger a reload.
- `make ci` now includes the `cli` build step, matching the documented CI gate (gofmt + vet + race tests + build).

### Added
- Scaffold: Go module, Makefile, CLI skeleton.
- Core snapshot pipeline, token-auth client, config + hot reload, `/metrics` + `/health`, capacity & dedup metrics.
- Added per-MTree usage, compression, and quota metrics.
- Added replication context metrics (state, sync lag, backlog, throughput).
- Added health & ops metrics (disk state, active alerts by severity, system CPU/throughput).
- MkDocs documentation site.
- Sample Grafana dashboard (`grafana/dashboards/ppdd-overview.json`) covering capacity/dedup, MTrees, replication, and health.
- Two end-to-end Docker Compose demo stacks at the repo root (`docker-compose.yml` built from source, `docker-compose.ghcr.yml` from GHCR) wiring mockdd → exporter → Prometheus → Grafana with auto-provisioned datasource and dashboard; Prometheus/Grafana pinned to `:latest` to match `pflex_exporter`.
- `mockdd` (`cmd/mockdd`): a self-contained fake DD appliance serving demo fixtures over TLS, so the stacks populate the dashboard without real hardware.

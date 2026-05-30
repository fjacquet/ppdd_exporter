# Changelog

All notable changes to this project are documented here. The format is based on
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project adheres to
[Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Fixed
- `/health` now correctly reports unhealthy systems: `OK` is set to `false` (and `Err` populated) when all collectors fail to produce any data for a system, instead of always reporting healthy.
- `passwordFile` contents are now trimmed of surrounding whitespace (e.g. trailing newline from `echo`) before being used as the system password, preventing silent authentication failures.
- Config watcher startup errors are now logged as warnings instead of being silently swallowed, making misconfiguration easier to diagnose.

### Added
- Scaffold: Go module, Makefile, CLI skeleton.
- Core snapshot pipeline, token-auth client, config + hot reload, `/metrics` + `/health`, capacity & dedup metrics.
- Added per-MTree usage, compression, and quota metrics.
- Added replication context metrics (state, sync lag, backlog, throughput).
- Added health & ops metrics (disk state, active alerts by severity, system CPU/throughput).
- MkDocs documentation site.
- Sample Grafana dashboard (`grafana/ppdd-overview.json`) covering capacity/dedup, MTrees, replication, and health.
- Two end-to-end Docker Compose demo stacks (exporter built from source, and pulled from GHCR) wiring mockdd → exporter → Prometheus → Grafana with auto-provisioned datasource and dashboard.
- `mockdd` (`cmd/mockdd`): a self-contained fake DD appliance serving demo fixtures over TLS, so the stacks populate the dashboard without real hardware.

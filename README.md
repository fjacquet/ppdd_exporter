# ppdd_exporter

A Go Prometheus exporter for **Dell PowerProtect DD (Data Domain)** appliances. One
process monitors many DD systems, polls each on an interval, and serves metrics at
`/metrics`. Modeled on `pflex_exporter` (Prometheus-only; OTLP deferred).

Full docs: built with MkDocs Material (see `mkdocs.yml`).

## Quick start

```bash
make cli
export PPDD1_PASSWORD='your-monitor-password'
./bin/ppdd_exporter --config config.yaml
# metrics: http://localhost:9099/metrics   health: http://localhost:9099/health
```

## Try it end-to-end (no appliance needed)

Two Compose stacks bring up **mockdd → exporter → Prometheus → Grafana** with a
pre-built dashboard:

```bash
make demo          # exporter built from source
make demo-ghcr     # …or the published image; make demo-down to stop
```

(Equivalently, from the repo root: `docker compose up --build`. Requires a running
Docker daemon; use `docker compose`, not the older `docker-compose`.)

Then open Grafana at <http://localhost:3000> (admin/admin) → **“PowerProtect DD — Overview”**.
See [docs/deployment/compose-demo.md](docs/deployment/compose-demo.md). The dashboard JSON is
[`grafana/dashboards/ppdd-overview.json`](grafana/dashboards/ppdd-overview.json).

## Metric domains

Capacity & dedup, MTrees, Replication, Health & ops. See [docs/metrics.md](docs/metrics.md).

> API field mappings are modeled from Dell docs (DD OS 8.3 REST API) and are being
> validated against live appliances. `ppdd_collector_up{collector="..."}` reports per-module
> health.

## License

Apache-2.0.

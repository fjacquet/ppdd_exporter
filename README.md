# ppdd_exporter

A Go Prometheus exporter for **Dell PowerProtect DD (Data Domain)** appliances. One
process monitors many DD systems, polls each on an interval, and serves metrics at
`/metrics`. Modeled on `pflex_exporter` (Prometheus-only; OTLP deferred).

Full docs: built with MkDocs Material (see `mkdocs.yml`).

## Quick start

```bash
make cli
export DD01_PASSWORD='your-monitor-password'
./bin/ppdd_exporter --config config.yaml
# metrics: http://localhost:9099/metrics   health: http://localhost:9099/health
```

## Try it end-to-end (no appliance needed)

Two Compose stacks bring up **mockdd → exporter → Prometheus → Grafana** with a
pre-built dashboard:

```bash
# exporter built from source:
docker compose -f deploy/compose/docker-compose.build.yml up --build
# …or the published image:
docker compose -f deploy/compose/docker-compose.ghcr.yml up
```

Then open Grafana at <http://localhost:3000> (admin/admin) → **“PowerProtect DD — Overview”**.
See [docs/deployment/compose-demo.md](docs/deployment/compose-demo.md). The dashboard JSON is
[`grafana/ppdd-overview.json`](grafana/ppdd-overview.json).

## Metric domains

Capacity & dedup, MTrees, Replication, Health & ops. See [docs/metrics.md](docs/metrics.md).

> API field mappings are modeled from Dell docs (DD OS 8.3 REST API) and are being
> validated against live appliances. `ppdd_collector_up{collector="..."}` reports per-module
> health.

## License

Apache-2.0.

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
# metrics: http://localhost:9441/metrics   health: http://localhost:9441/health
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

## Node Exporter Full (Grafana 1860)

This repo bundles the community [Node Exporter Full](https://grafana.com/grafana/dashboards/1860-node-exporter-full/)
dashboard (`node-exporter-full.json`, auto-provisioned). It visualizes **host OS** metrics
(CPU, memory, disk, network) exposed by [`prom/node-exporter`](https://hub.docker.com/r/prom/node-exporter) —
**not** this exporter's own metrics.

`node_exporter` is **not** part of this demo stack: it belongs on the hosts you actually want to
monitor, not bolted onto the exporter's compose. To use this dashboard, run `prom/node-exporter`
on those hosts and add a `node-exporter` scrape job to your Prometheus; the dashboard then
visualizes them.

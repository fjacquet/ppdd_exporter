# End-to-end demo (Compose)

Two Docker Compose stacks bring up the full observability path —
**mockdd → exporter → Prometheus → Grafana** — so you can see the dashboard populated
without a real Data Domain appliance.

`mockdd` is a tiny fake DD appliance (`cmd/mockdd`) that serves the same REST surface the
exporter calls, returning demo fixtures over self-signed TLS on `:3009`. It is a demo aid,
not a faithful emulator.

## Two stacks

| File | Exporter source |
|---|---|
| `deploy/compose/docker-compose.build.yml` | **Built from source** (the repo `Dockerfile`) |
| `deploy/compose/docker-compose.ghcr.yml` | **Pulled from GHCR** (`ghcr.io/fjacquet/ppdd_exporter`) |

Both are identical otherwise (the `mockdd` image is built locally in both — there is no
published mock image).

## Run it

From the repo root:

```bash
# Build the exporter from source:
docker compose -f deploy/compose/docker-compose.build.yml up --build

# …or run the published image instead:
docker compose -f deploy/compose/docker-compose.ghcr.yml up
# pin a tag: PPDD_IMAGE_TAG=v0.1.0 docker compose -f deploy/compose/docker-compose.ghcr.yml up
```

Then open:

- **Grafana** — <http://localhost:3000> (admin / admin) → dashboard **“PowerProtect DD — Overview”** (folder *PowerProtect DD*). The Prometheus datasource and dashboard are auto-provisioned.
- **Prometheus** — <http://localhost:9090>
- **Exporter** — <http://localhost:9099/metrics> and <http://localhost:9099/health>

Tear down with `docker compose -f <file> down`.

## What's wired

- `mockdd` serves the fixtures from `cmd/mockdd/fixtures/` (capacity, 4 MTrees, 2
  replication contexts incl. one *lagging*, a failed disk, alerts by severity, system perf).
- The exporter uses `deploy/compose/config.yaml` (a 30s interval for a snappy demo) pointed
  at the `mockdd` service.
- Prometheus scrapes `exporter:9099` (`deploy/compose/prometheus/prometheus.yml`).
- Grafana provisioning lives in `deploy/compose/grafana/provisioning/`; the dashboard JSON
  is the repo's canonical `grafana/ppdd-overview.json`.

## Pointing at a real appliance

Edit `deploy/compose/config.yaml`: set `host` to your DD, supply real credentials (use a
read-only/monitor user; `${ENV}` interpolation and `passwordFile` are supported), and remove
the `mockdd` service from the compose file.

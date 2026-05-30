# ppdd_exporter

A Go Prometheus exporter for **Dell PowerProtect DD (Data Domain)** appliances. One process
monitors many DD systems, polls each on an interval, publishes an immutable snapshot, and
serves metrics at `/metrics`. Modeled on `pflex_exporter` (Prometheus-only; OTLP deferred).

- **Snapshot model** — one background loop decouples DD API load from scrape count.
- **Modular collectors** — capacity & dedup, MTrees, replication, health & ops.
- **Per-module health** — `ppdd_collector_up{collector="..."}` reports each module's status.

!!! warning "Provisional API mappings"
    DD REST API field mappings are modeled from Dell docs (DD OS 8.3) and are being validated
    against live appliances.

See [Installation](getting-started/installation.md) and the [Metrics reference](metrics.md).

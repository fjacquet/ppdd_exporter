# Quick start

```bash
make cli
export PPDD1_PASSWORD='your-monitor-password'
./bin/ppdd_exporter --config config.yaml
# metrics: http://localhost:9099/metrics
# health:  http://localhost:9099/health
```

Useful flags:

- `--once` — run a single collection cycle, log the result, and exit (connectivity check).
- `--debug` — verbose logging, including per-collector failures. Combined with
  `--once`, it also prints **every collected sample** (sorted, exposition style)
  so you can diff a live appliance against the [metrics reference](../metrics.md).
- `--trace` — log every DD API response body (method, URL, status, payload; the
  auth token is never logged). Use it when a metric you expect is absent: the
  exporter never guesses values, so an unexpected payload shape shows up as a
  missing sample — the trace shows what the appliance actually returned.

Validating against a real appliance:

```bash
ppdd_exporter --config config.yaml --once --debug --trace 2>trace.log | sort > samples.txt
# samples.txt  → every metric collected (compare with docs/metrics.md)
# trace.log    → raw API payloads for anything missing or suspicious
```

Then point Prometheus at the target:

```yaml
scrape_configs:
  - job_name: ppdd
    scrape_interval: 5m      # match collection.interval; data only refreshes that often
    static_configs:
      - targets: ['localhost:9099']
```

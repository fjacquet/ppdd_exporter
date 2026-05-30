# Quick start

```bash
make cli
export DD01_PASSWORD='your-monitor-password'
./bin/ppdd_exporter --config config.yaml
# metrics: http://localhost:9099/metrics
# health:  http://localhost:9099/health
```

Run a single cycle (useful for validation): `./bin/ppdd_exporter --once --debug`.

Then point Prometheus at the target:

```yaml
scrape_configs:
  - job_name: ppdd
    scrape_interval: 5m      # match collection.interval; data only refreshes that often
    static_configs:
      - targets: ['localhost:9099']
```

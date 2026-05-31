# 0001. Decouple DD polling from scrapes with a snapshot model

- **Status:** accepted
- **Date:** 2026-05-30
- **Deciders:** Fred Jacquet

## Context and problem statement

The exporter monitors one or more Dell PowerProtect DD appliances over a token-authenticated
REST API. DD statistics are slow-moving (capacity, dedup, replication, health), while a
Prometheus deployment may scrape `/metrics` frequently and from multiple replicas. If each
scrape triggered live DD API calls, load on the appliance — and the number of auth sessions —
would scale with scrape count rather than with the data's actual rate of change.

## Considered options

- **Collect-on-scrape** — the `/metrics` handler queries every DD system synchronously per scrape.
- **Snapshot model** — a background loop polls on an interval and publishes an immutable snapshot that handlers read.

## Decision outcome

Chosen option: **the snapshot model**. A single background `Collector`
(`internal/ppdd/collector.go`) polls every configured system in parallel on
`collection.interval`, builds an immutable `Snapshot`, and pointer-swaps it into a
`SnapshotStore` guarded by an `RWMutex`. The `/metrics` handler is an *unchecked* Prometheus
collector (`prometheus.go`) that reads the latest snapshot; `/health` reads the same snapshot.
A synchronous startup cycle primes the store before serving, and `--once` runs a single cycle
and exits.

### Consequences

- Good — DD API load and session count are a function of `collection.interval`, independent of scrape frequency or scraper count.
- Good — `/metrics` and `/health` are fast and never block on the appliance; both share one consistent view.
- Good — graceful degradation: one system's failure never blocks others (the loop publishes whatever succeeded).
- Bad — metrics can be up to one `collection.interval` stale.
- Neutral — handlers must tolerate an empty/initial snapshot; the startup cycle exists to minimize that window.

## Related

- [0002. Modular per-domain ResourceCollectors](0002-modular-resource-collectors.md)
- [0006. One label-key set per metric name](0006-label-key-consistency-invariant.md)

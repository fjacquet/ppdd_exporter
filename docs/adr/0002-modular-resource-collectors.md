# 0002. Modular per-domain ResourceCollectors

- **Status:** accepted
- **Date:** 2026-05-30
- **Deciders:** Fred Jacquet

## Context and problem statement

The exporter covers four DD metric domains (capacity & dedup, MTrees, replication, health &
ops), and the API field mappings are modeled from documentation only (see
[ADR-0003](0003-provisional-api-mappings.md)), so individual endpoints and JSON fields are
expected to need correction. We need a structure that isolates that risk, allows domains to be
added or fixed independently, and keeps a single failing domain from taking down the whole
collection cycle.

## Considered options

- **Monolithic collector** — one function fetches all endpoints and assembles every metric.
- **Modular collectors** — each domain is a self-contained unit behind a small interface.

## Decision outcome

Chosen option: **modular collectors**. Each domain implements `ResourceCollector`
(`internal/ppdd/resource.go`) and owns its endpoint path, response struct, and `parse → []Sample`
logic. Collectors are registered in `Registry()`, and the per-system cycle iterates them. A
module failure degrades to an empty sample slice plus a `ppdd_collector_up{collector="<name>"}`
`0`/`1` signal — it never crashes the cycle.

```go
type ResourceCollector interface {
    Name() string
    Collect(ctx context.Context, c ddclient.Client) ([]Sample, error)
}
```

### Consequences

- Good — each API correction is contained to one struct plus one `testdata/<name>.json` fixture in one module.
- Good — domains can be phased in and tested in isolation (per-module table tests).
- Good — partial failures are observable in-band via `ppdd_collector_up`.
- Bad — a small amount of per-module boilerplate (struct + parse + registration + fixture).
- Neutral — adding a domain is a fixed checklist (see CLAUDE.md → "Adding a metric domain").

## Related

- [0001. Snapshot model](0001-prometheus-snapshot-model.md)
- [0003. Provisional API mappings](0003-provisional-api-mappings.md)

# 0006. One label-key set per metric name

- **Status:** accepted
- **Date:** 2026-05-30
- **Deciders:** Fred Jacquet

## Context and problem statement

The `/metrics` handler is an *unchecked* Prometheus collector (its `Describe` emits nothing) so
that the set of metric names can vary per snapshot (see [ADR-0001](0001-prometheus-snapshot-model.md)).
The trade-off: `client_golang` does **not** enforce a consistent variable-label-key set per
metric name for an unchecked collector. If two samples share the same metric name (`fqName`) but
carry different label keys, a checked registry would fail `Gather`/scrape with an "inconsistent
label names" error. We must guarantee consistency ourselves.

## Considered options

- **Rely on discipline** — trust that every collector emits a fixed label set per metric name.
- **Enforce the invariant** — guard it both statically (tests) and at runtime (collector).

## Decision outcome

Chosen option: **enforce the invariant**. The rule is: *a metric name carries exactly one
label-key set across all of its series.*

- **Statically:** `internal/ppdd/labels_test.go` runs every collector against fixtures and fails the build if any metric name appears with two different label-key sets.
- **At runtime:** `PromCollector.Collect` records the first label-key set seen for each metric name within a scrape and drops any later sample whose keys disagree, so a stray inconsistency can never break the whole scrape.

### Consequences

- Good — exported series shape is stable; scrapes cannot fail due to per-sample label-key drift.
- Good — the invariant is caught at build time when adding/altering a collector, not in production.
- Bad — a genuinely divergent sample is silently dropped at runtime (defensive; the static test is the real gate).
- Neutral — collectors should set labels in a fixed order per metric name; `WithSystem` prepends the `system` label consistently.

## Related

- [0001. Snapshot model](0001-prometheus-snapshot-model.md)
- [0002. Modular per-domain ResourceCollectors](0002-modular-resource-collectors.md)

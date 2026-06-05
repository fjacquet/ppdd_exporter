# Architecture decision records

This directory records the significant architectural decisions for `ppdd_exporter` — the
*why* behind the design, in the form of dated [MADR](https://adr.github.io/madr/)-style records.
Decisions are immutable once accepted: rather than editing a past record, add a new one that
supersedes it.

To add a decision, copy [`0000-template.md`](0000-template.md) to the next number and link it
here.

| ADR | Decision | Status |
|---|---|---|
| [0001](0001-prometheus-snapshot-model.md) | Decouple DD polling from scrapes with a snapshot model | accepted |
| [0002](0002-modular-resource-collectors.md) | Modular per-domain `ResourceCollector`s | accepted |
| [0003](0003-provisional-api-mappings.md) | Treat DD API mappings as provisional until validated | accepted |
| [0004](0004-token-auth-retry-policy.md) | Token auth with a retry policy that excludes 4xx | accepted |
| [0005](0005-config-hot-reload-rebuild-and-swap.md) | Hot reload rebuilds and swaps runtime components | accepted |
| [0006](0006-label-key-consistency-invariant.md) | One label-key set per metric name | accepted |
| [0007](0007-dd-rest-api-realignment.md) | Realign collectors to the documented DD 7.3 REST API | accepted |
| [0008](0008-ci-supply-chain-hardening.md) | CI/CD supply-chain hardening: scanning, SHA-pinned Actions, GoReleaser | accepted |

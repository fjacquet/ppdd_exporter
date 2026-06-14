# 0009. Validate DD API mappings against the checked-in 8.7.0 OpenAPI spec

- **Status:** accepted
- **Date:** 2026-06-14
- **Deciders:** Fred Jacquet

## Context and problem statement

[ADR-0003](0003-provisional-api-mappings.md) shipped the DD API mappings as **docs-only and
provisional**, and [ADR-0007](0007-dd-rest-api-realignment.md) realigned them to the prose
**DD 7.3 REST API guide** while leaving four endpoints (`replications`, `hardware/disks`,
`stats/system-stats`, `file-system`) explicitly provisional and **all field values
unvalidated**. Dell's **PowerProtect DD 8.7.0 OpenAPI 3.1 spec** (`13345-8.7.0.json`, 211
paths, 751 component schemas) then became available — a far stronger source than prose: exact
paths, JSON keys, types, and enum values. Still no live appliance was available.

Validating against it confirmed **all four provisional endpoints were wrong** and surfaced
field-level errors in two of the three "documented" modules (alerts wrapper key, mtree quota
nesting, disk state enum).

## Considered options

- **Stay on the 7.3 guide; wait for a live appliance** — no churn now, but ships known-wrong endpoints until hardware appears.
- **Validate shape against the checked-in 8.7.0 OpenAPI spec now** — correct every module to the spec by hand (keeping the modular one-struct-one-fixture convention); defer runtime-value confirmation to a live box.
- **Generate the client/types from the spec** (openapi codegen) — exhaustive, but discards the lean hand-tuned structs and the per-module correction model.

## Decision outcome

Chosen option: **validate shape against the checked-in 8.7.0 OpenAPI spec and hand-correct**.
The spec is committed at `docs/swagger/13345-8.7.0.json` as the source of truth; each module's
path/struct/fixture was corrected one at a time (per [ADR-0002](0002-modular-resource-collectors.md)),
not code-generated. The spec validates **shape only** — runtime **values** still require a live
appliance, so that part of ADR-0003/0007 stands.

Concretely: file-systems (plural), disks → `/api/v1/.../storage/disks`, performance →
`/api/v3/.../stats/performance`; alerts `alert_list`, mtree `quota_config`, disk `FAILED`,
mtree top-level `compression_factor`; and the bogus single `replication` collector split into
**`mtree_replication`** (posture) and **`file_replication`** (stats) to match the real API.

### Consequences

- Good — all four previously-provisional endpoints are fixed, and the spec is checked in, so a future DD version bump re-validates by diffing a new OpenAPI file.
- Good — replication is now modeled as the two distinct domains the API actually exposes.
- Good — the `mockdd` demo, `docs/metrics.md`, and the Grafana dashboard were updated in lockstep (a path/shape change that skips any of these silently breaks the Compose demo; `make ci` does not catch it).
- **Bad (breaking)** — removed `ppdd_compression_{global,local,total}_factor` (no source on 8.7.0); `ppdd_replication_*` replaced by `ppdd_mtree_replication_*` + `ppdd_file_replication_*`; alert label values now use DD enum casing (e.g. `CRITICAL`). This reverses ADR-0007's "no metric removed" stance; dashboards and queries must be updated.
- Neutral — the spec uses **two prefixes** (`/rest/v*` legacy, `/api/v*` newer) and some resources exist only under `/api/`; `endpoints.go` now mixes both.
- Neutral — **values are still unconfirmed against a live appliance** (carried over from ADR-0003/0007); shape is validated, data is not.
- Neutral — hand-correction was chosen over codegen to preserve the lean structs; revisit codegen only if the surface grows materially.

## Related

- Supersedes [0003. Treat DD API mappings as provisional until validated](0003-provisional-api-mappings.md) — shape is now spec-validated; only value confirmation remains open.
- Amends [0007. Realign collectors to the documented DD 7.3 REST API contract](0007-dd-rest-api-realignment.md) — closes its provisional-endpoints checklist and reverses its "no metric removed" consequence.
- [0002. Modular per-domain ResourceCollectors](0002-modular-resource-collectors.md)
- [0006. One label-key set per metric name](0006-label-key-consistency-invariant.md)

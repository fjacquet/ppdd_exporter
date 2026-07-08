# Design: Fix Grafana Table Joins & Variable Refresh (live DD3410 validation)

**Date:** 2026-07-08
**Status:** Approved (design); implementation plan pending
**Scope:** Grafana dashboard JSON only — no collector, mockdd, or metric changes.

## Origin

Live validation against a real appliance (`diab-DD3410`, DD OS 8.7.0) via the Compose
server stack (`docker-compose.server.yml`). The MTree dashboard showed "No data" and the
"MTree detail" table showed one row per metric. Investigation **validated the whole data
pipeline as correct** — exporter, API mappings, collectors, mockdd fixtures, Prometheus
scrape, and datasource all work; live MTree data (`ppdd_mtree_logical_used_bytes{mtree="/data/col1/alfred"}=40 GB`)
reaches Prometheus fresh. The two defects are entirely in the Grafana view layer.

## Findings (evidence)

| # | Symptom | Root cause | Evidence |
|---|---|---|---|
| 1 | MTree panels showed "No data" | `$system` template var `refresh=2` ("on time-range change") never re-queried on a plain reload; the tab cached an empty variable from before MTree data existed | Hard-refresh (Cmd+Shift+R) made all panels populate; overview worked because its var reads `ppdd_filesystem_total_bytes`, present since day one |
| 2 | Detail tables show one row **per metric** instead of one joined row per entity | Multi-query table panels use the Grafana `merge` transform, which stacks the N per-column query results instead of joining them on the entity label | Confirmed after a clean `docker compose restart grafana` + hard-refresh (dashboards are bind-mounted from the current `fc412a9` checkout, so this is the current repo behavior, not a cache artifact) |

Confirmed **non-issues:** exporter is correct (the stale GHCR image only lacks the cosmetic
`ppdd_exporter_build_info` metric — needs a release, out of scope here); "Quota utilization %"
being empty is correct (this appliance has no MTree quotas); per-MTree Dedup `0.0` is a
separate low-priority mapping question, deferred.

### Table-join bug is systemic

A survey of every `table` panel found the `merge` bug in **4 of 5** multi-query tables:

| Dashboard | Panel | Queries | Join key |
|---|---|---|---|
| overview | Systems at a glance | 8 | `system` |
| mtrees | MTree detail | 7 | `mtree` |
| replication | MTree replication contexts | 4 | `destination` (per-context; verify live) |
| replication | File replication contexts | 3 | `context` |
| health | Failed disk detail | 1 | — (single query; already correct, no change) |

Row explosion is `queries × entities` (e.g. 7 × 2 MTrees = 14 rows), each carrying only
its own column.

## Decisions

1. **Table joins.** In each of the 4 broken panels, replace the `merge` transformation with
   **`joinByField`** (`mode: outer`, `byField: <join key>`). Extend each panel's existing
   `organize` transform to hide the duplicate `Time` / `instance` / `job` / repeated-entity
   columns that the join produces, and keep the `Value #<refId> → friendly name` renames and
   column ordering. Result: one row per entity with every column populated.

2. **Variable refresh.** Set the `system` template variable `refresh` from `2` → `1`
   ("on dashboard load") across **all 5** dashboards, so a newly-appearing metric populates on
   open without a hard-refresh. (Chosen over MTree-only for consistency — every dashboard
   shares this variable pattern and is subject to the same staleness.)

3. **Quota panel polish.** The "Quota utilization %" bargauge divides by `(quota_hard > 0)`,
   yielding "No data" when no quota is configured. Render a friendly zero/empty state instead
   (e.g. `... or on() vector(0)` or a panel "No value" mapping), so an unconfigured quota reads
   as `0%`/"no quota" rather than a broken-looking "No data".

4. **Docs/CHANGELOG.** Add a `CHANGELOG.md` entry (Fixed). Metric names are unchanged, so no
   `metrics.md` edit; Grafana-only, so no `cmd/mockdd` or collector change.

## Files touched

- `grafana/dashboards/ppdd-overview.json` — join (Systems at a glance) + var refresh
- `grafana/dashboards/ppdd-mtrees.json` — join (MTree detail) + var refresh + quota panel
- `grafana/dashboards/ppdd-replication.json` — join (both replication tables) + var refresh
- `grafana/dashboards/ppdd-capacity.json` — var refresh
- `grafana/dashboards/ppdd-health.json` — var refresh (Failed disk detail table unchanged)
- `CHANGELOG.md` — Fixed entry

## Risks & verification

- **Grafana transforms are finicky.** `joinByField` can leave numbered duplicate columns
  (`system 2`, `Time 1`…) that the `organize` step must hide; the exact exclude list is
  panel-specific and must be confirmed by rendering, not assumed.
- **MTree-replication join key** (`destination` vs `source` vs a composite) must be confirmed
  against live/mock data — `joinByField` takes a single field, so the chosen key must uniquely
  identify a context row. If no single label is unique, add a preceding `label_replace`/concat
  step to synthesize one.
- **Verification loop (per panel):** edit repo JSON → `git pull` on the epg host (clean; only
  `config.yaml` is locally modified, no dashboard edits) → `docker compose -f docker-compose.server.yml restart grafana`
  → confirm (a) one row per entity with all columns filled, and (b) opening the dashboard fresh
  (no hard-refresh) populates. mockdd fixtures already contain MTree + replication data, so the
  laptop demo (`docker-compose.yml`) can exercise the join without live hardware.

## Out of scope

- Exporter `build_info` on the server (needs a new GHCR release).
- Per-MTree `compression_factor = 0` mapping investigation.

# Design: Validate & Correct API Mappings Against DD 8.7.0 OpenAPI

**Date:** 2026-06-13
**Status:** Approved (design); implementation plan pending
**Source of truth:** PowerProtect DD 8.7.0 OpenAPI 3.1 spec (`13345-8.7.0.json`, 211 paths, 751 schemas)

## Problem

The exporter's endpoint paths and JSON field mappings were modeled from the DD 7.3
PDF guide, with four endpoints explicitly marked `PROVISIONAL`. Validation against the
authoritative 8.7.0 OpenAPI spec confirms **all four provisional endpoints are wrong**
and surfaces field-level errors in two of the three "documented" modules. This design
corrects every mapping to match the 8.7.0 spec.

The spec is treated as **authoritative for shape** (paths, keys, types, enums). It does
not carry live values, so a comment is retained only where a runtime *value* assumption
remains unconfirmed on a live appliance. The `PROVISIONAL` markings are otherwise removed
and replaced with "validated against 8.7.0 OpenAPI".

## Validation Findings (evidence)

The spec uses **two path prefixes** — `/rest/v*` (legacy) and `/api/v*` (newer). Some
resources only exist under `/api/`; the code previously assumed `/rest/` everywhere.

| # | Module / endpoint | Verdict | Correction |
|---|---|---|---|
| 1 | capacity `/rest/v1.0/system` | ✅ path+fields OK | none (`physical_capacity.{total,used,available}`, `compression_factor`) |
| 2 | capacity-extra `/rest/v1.0/.../file-system` | ❌ path + all fields wrong | path → `/file-systems`; clean state → `fs_clean_status == "running"`; **drop compression-split metrics** |
| 3 | mtrees `/rest/v3.0/.../mtrees` | ⚠️ path OK, quota fields wrong | quota → `quota_config.{soft_limit,hard_limit}` |
| 4 | mtree-stats `/rest/v2.0/.../mtrees/{id}/stats/capacity` | ✅ OK | use top-level `compression_factor` instead of tier-weighting |
| 5 | disks `/rest/v1.0/.../hardware/disks` | ❌ path + fields wrong | path → `/api/v1/.../storage/disks`; key `diskInfo`; field `status`; failed = `status == "FAILED"` |
| 6 | sys-stats `/rest/v1.0/.../stats/system-stats` | ❌ path + shape wrong | path → `/api/v3/.../stats/performance`; array `statsPerformance[]` (latest by `collectionEpoch`); cpu = `averageCpuUtilization`; r/w = `throughput.{read,write}` |
| 7 | alerts `/rest/v1.0/.../alerts` | ⚠️ path OK, key wrong | array key `alert_list` (not `alert`); `is_active=true` query valid |
| 8 | replication `/rest/v1.0/.../replications` | ❌ endpoint does not exist | split into two collectors (below) |

Pagination envelope (`paging`: `current_page/page_entries/total_entries/page_size`)
matches the existing `pagingInfo` struct exactly — **no change**.

## Decisions

1. **Replication** → split into two collectors (the old endpoint conflated posture and
   perf, and none of its perf fields exist on either real endpoint).
2. **MTree N+1** → keep the per-mtree `stats/capacity` call; fix in place using the
   top-level `compression_factor`.
3. **Spec authority** → authoritative for 8.7.0; drop `PROVISIONAL` on validated fields.
4. **Compression split** (`ppdd_compression_{global,local,total}_factor`) → **dropped**.
   `/system` already provides `ppdd_compression_factor`; the split has no source on
   `/file-systems` and adds an endpoint for marginal value.

## Changes

### A. `internal/ppdd/endpoints.go`
| Const | New value |
|---|---|
| `pathSystem` | `/rest/v1.0/system` (unchanged) |
| `pathAlerts` | `/rest/v1.0/dd-systems/0/alerts` (unchanged) |
| `pathMTrees` | `/rest/v3.0/dd-systems/0/mtrees` (unchanged) |
| `pathFileSystem` | `/rest/v1.0/dd-systems/0/file-systems` |
| `pathDisks` | `/api/v1/dd-systems/0/storage/disks` |
| `pathPerformance` | `/api/v3/dd-systems/0/stats/performance` |
| `pathMTreeReplication` | `/api/v1/dd-systems/0/mtree-replications` |
| `pathFileReplication` | `/rest/v1.0/dd-systems/0/stats/file-replications` |
| `pathReplication` | **removed** |
| `mtreeStatsPath()` | unchanged |

### B. Existing-module struct/logic fixes
- **capacity.go** — `fileSystemResp`: drop `*_compression_factor` + `cleaning.status`;
  read `fs_clean_status string` (running ⇒ `ppdd_filesystem_cleaning_running=1`). Remove
  the three `ppdd_compression_*_factor` samples.
- **mtrees.go** — replace `QuotaSoftLimit`/`QuotaHardLimit` with nested
  `quota_config.{soft_limit,hard_limit}`; in `mtreeUsage`, take `statsCapacityInfo`
  top-level `compression_factor` (keep tier sum only for logical used / post-comp bytes).
- **health.go disks** — array key `diskInfo`; struct field `Status` (`json:"status"`);
  failed when `status == "FAILED"`.
- **health.go system perf** — decode `statsPerformance[]`, pick max `collectionEpoch`;
  `ppdd_system_cpu_percent = averageCpuUtilization`,
  `ppdd_system_read_bytes_per_second = throughput.read`,
  `ppdd_system_write_bytes_per_second = throughput.write`.
- **health.go alerts** — array key `alert_list`.

### C. Replication collectors (new)
- `internal/ppdd/mtree_replication.go` — `MTreeReplication` reads
  `pathMTreeReplication` → `contexts[]`:
  - `ppdd_mtree_replication_state{state,source,destination} = 1`
  - `ppdd_mtree_replication_need_resync{source,destination}` (bool→0/1)
  - `ppdd_mtree_replication_connected{source,destination}` (bool→0/1)
  - labels: `source=sourceHost`, `destination=destinationHost`.
- `internal/ppdd/file_replication.go` — `FileReplication` reads
  `pathFileReplication` → `context[]`:
  - `ppdd_file_replication_network_bytes{context}`
  - `ppdd_file_replication_logical_replicated_bytes{context}`
  - `ppdd_file_replication_active_files{context}`
  - `ppdd_file_replication_status{context,status} = 1`
  - label `context = id`.
- Delete `internal/ppdd/replication.go` (+ its test); update `Registry()` in
  `resource.go` to `{Capacity, MTrees, MTreeReplication, FileReplication, Health}`.

### D. Fixtures & mock (memory: shape changes must update mockdd or the demo breaks)
- `internal/ppdd/testdata/`: reshape to real schemas —
  `file-system.json` → `file-systems.json`; `disks.json` (key `diskInfo`, `status`);
  `system-stats.json` → `performance.json` (`statsPerformance[]`); `alerts.json`
  (`alert_list`); `mtrees.json` (`quota_config`); replace `replications.json` with
  `mtree-replications.json` + `file-replications.json`.
- `cmd/mockdd/main.go`: update the `routes` map keys to the corrected paths (including
  the new `/api/...` paths) and point them at the renamed fixtures.

### E. Docs / tests / changelog
- `docs/metrics.md`: remove `ppdd_compression_{global,local,total}_factor`; document the
  new `ppdd_mtree_replication_*` and `ppdd_file_replication_*` metrics; correct any names.
- Per-module tests updated to the new fixtures; `labels_test.go` covers new metrics
  (label-key consistency per metric name).
- `CHANGELOG.md`: one entry — "Validate and correct API mappings against DD 8.7.0 OpenAPI".
- Comments: "7.3 guide / PROVISIONAL" → "validated against 8.7.0 OpenAPI"; retain a note
  only where a runtime value (not shape) is still unconfirmed on a live appliance.

## Out of scope
- Adding net-new metric domains beyond the replication split.
- Live-appliance value confirmation (shape is validated; values are not).
- N+1 elimination for mtree usage (explicitly deferred).

## Testing
`make ci` (gofmt + vet + lint + race tests + govulncheck + build) is the gate. Each
module's table test runs against its corrected fixture; the mockdd Compose demo must
serve every corrected path. No live appliance is required for this pass.

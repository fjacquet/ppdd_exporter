# DD REST API Realignment (Theme A — Correctness)

**Date:** 2026-06-03
**Status:** Approved design — ready for implementation planning
**Source of truth:** *PowerProtect DD Series Appliance 7.3 — REST API Guide* (36 pp.), referred to below as "the guide."
**Scope:** Correctness realignment of the collector layer against the documented API contract. No live appliance is available; the guide is authoritative, and every unvalidated mapping stays explicitly provisional and centrally correctable.

---

## 1. Problem

The exporter's API mappings were modeled from older/partial Dell docs. Reading the full 7.3 guide against the current code (`internal/ppdd/*.go`, `internal/ddclient/*.go`) surfaced five correctness gaps:

1. **Endpoint versions are wrong.** Code uses a uniform `/api/v1/...`. The guide shows the `/rest/` prefix (documented as supported on all DD OS versions) with **per-resource** versions: `mtrees` is **v3.0**, `stats/capacity` is **v2.0**, `system`/`alerts`/`auth` are **v1.0**.
2. **No pagination.** List endpoints page with **default size 20, max 200**. Collectors read only the first array, so any system with >20 MTrees / alerts / replication contexts **silently drops data**.
3. **Alerts are historical, not active.** Without `?is_active=true`, `/alerts` returns historical alerts; `ppdd_alerts_active` is mislabeled.
4. **Capacity shape is invented.** `/file-system` with `physical_*_bytes` fields does not appear in the guide; documented current-state capacity lives in `GET /rest/v1.0/system`.
5. **MTree usage conflated with list.** The v3.0 `/mtrees` list returns metadata only; per-MTree usage comes from a separate `GET /rest/v2.0/.../mtrees/{id}/stats/capacity`.

**Constraint (project convention):** API mappings are *provisional until confirmed against a live DD*. This design strengthens mappings to the documented contract but must keep undocumented mappings honestly flagged and correctable in one place.

---

## 2. Strategy

**Approach: central path registry + generic pagination helper + PDF-derived fixtures.**

- A single `internal/ppdd/endpoints.go` holds every path and per-resource version — the one place a future live-DD correction edits.
- One `paginate()` helper fixes the data-loss bug for all list collectors at once, while each collector continues to decode its own named array (provisional risk stays localized, matching the existing module convention).
- Fixtures are copied **verbatim from the guide's sample responses** so tests encode the documented contract.
- **No metric is removed.** Documented metrics are *added* and capacity sources are *retargeted*; the four undocumented metrics are *kept as flagged-provisional*. Result: zero dashboard breakage.

Rejected alternatives: operator-configurable endpoint map in `config.yaml` (YAGNI — adds config surface for a one-time decision); minimal in-place edits (duplicates pagination 3×, no central correction point).

---

## 3. Documented vs. provisional split

| Endpoint | Guide status | Action |
|---|---|---|
| `GET /rest/v1.0/system` | Documented (pp.11–12) | New capacity source (+ single `compression_factor`) |
| `GET /rest/v1.0/dd-systems/0/alerts?is_active=true` | Documented (pp.19,29) | Active alerts + `class` label |
| `GET /rest/v3.0/dd-systems/0/mtrees` | Documented (pp.32–33) | MTree metadata + health |
| `GET /rest/v2.0/dd-systems/0/mtrees/{id}/stats/capacity` | Documented (pp.33–36) | Per-MTree usage |
| `POST/DELETE /rest/v1.0/auth` | Documented (pp.5,10,13) | Flat `{username,password}` body |
| `/dd-systems/0/replications` | **Not in guide** | Prefix-only → `/rest/v1.0/`; fields stay provisional |
| `/dd-systems/0/hardware/disks` | **Not in guide** | Prefix-only → `/rest/v1.0/`; fields stay provisional |
| `/dd-systems/0/stats/system-stats` | **Not in guide** | Prefix-only → `/rest/v1.0/`; fields stay provisional |
| `/dd-systems/0/file-system` | **Not in guide** | Retained best-effort for provisional compression-split + `cleaning_running` only |

---

## 4. Components

### 4.1 `endpoints.go` (new)

Single source of truth for paths. Comment block explains the `/rest/` prefix choice and that version tokens are **not** uniform.

```go
const (
    pathAuth        = "/rest/v1.0/auth"                        // POST login / DELETE logout
    pathSystem      = "/rest/v1.0/system"                      // capacity, compression_factor
    pathAlerts      = "/rest/v1.0/dd-systems/0/alerts"         // + ?is_active=true
    pathMTrees      = "/rest/v3.0/dd-systems/0/mtrees"         // v3.0 metadata list
    pathReplication = "/rest/v1.0/dd-systems/0/replications"   // PROVISIONAL: not in guide
    pathDisks       = "/rest/v1.0/dd-systems/0/hardware/disks" // PROVISIONAL: not in guide
    pathSystemStats = "/rest/v1.0/dd-systems/0/stats/system-stats" // PROVISIONAL: not in guide
    pathFileSystem  = "/rest/v1.0/dd-systems/0/file-system"    // PROVISIONAL: cleaning + compression split
)

func mtreeStatsPath(id string) string // "/rest/v2.0/dd-systems/0/mtrees/{id}/stats/capacity"
```

### 4.2 `paginate()` helper (new, `ppdd` package)

```go
type pagingInfo struct {
    CurrentPage, PageEntries, TotalEntries, PageSize int // JSON: current_page, page_entries, total_entries, page_size
}

const pageSize = 200 // documented max
const maxPages = 100 // safety cap → log warning and stop

// paginate GETs basePath across all pages, handing each page's raw JSON to onPage,
// which decodes its own named array and returns the page's paging_info.
func paginate(ctx context.Context, c ddclient.Client, basePath, extraQuery string,
    onPage func(page json.RawMessage) (pagingInfo, error)) error
```

- Builds `basePath?page=N&size=200[&extraQuery]`.
- **Stop** when `(current_page+1)*page_size >= total_entries`, when a short page returns, or at `maxPages` (logs a warning so silent truncation never recurs).
- Used by `mtrees`, `alerts`, `replication`. Single-object endpoints (`system`) use plain `Get`.

### 4.3 `ddclient` changes

- **`auth.go`** — POST/DELETE `pathAuth`; request body becomes **flat** `{"username","password"}` (was `{"auth_info":{...}}`). Prominent comment: *highest-risk change; first revert point if logins fail.*
- **`mock.go`** — `Get` falls back to a path-without-query lookup when no exact match is registered, so collectors using `paginate` resolve against clean registered paths. Live `SystemClient` unaffected.

### 4.4 Collector reworks

**capacity** — two calls:
- `GET pathSystem` (primary; error fails the collector): `physical_capacity.{total,used,available}` → existing `ppdd_filesystem_{total,used,available}_bytes`; `compression_factor` → **new** `ppdd_compression_factor`.
- `GET pathFileSystem` (best-effort; provisional): `ppdd_compression_{global,local,total}_factor` + `ppdd_filesystem_cleaning_running`. Failure drops only these samples.

**mtrees** — paginated list + per-MTree stats:
- v3.0 list → `name`, `is_degraded`, `mtree_rl_detail.rl_status`, plus provisional quota fields if present.
- New: `ppdd_mtree_degraded{mtree}` (`is_degraded=="degraded"`), `ppdd_mtree_retention_lock_enabled{mtree}` (`rl_status` not in {`never-enabled`,`disabled`}).
- Per-MTree `mtreeStatsPath(id)` (best-effort, sequential): latest `collection_epoch` → `tier_capacity_usage[].logical_capacity.used` → `ppdd_mtree_logical_used_bytes`; `tier_data_written[].compression_factor` → `ppdd_mtree_compression_factor`.
- Provisional kept: `ppdd_mtree_physical_used_bytes` ← `post_comp_written` (semantic note: *written*, not *used*); `ppdd_mtree_quota_{soft,hard}_limit_bytes` ← list fields (flagged).
- N+1 request tradeoff documented; bounded concurrency deferred.

**replication / health(disks,system-stats)** — prefix moved to `/rest/v1.0/`; alerts moved into the documented path. Fields unchanged and flagged provisional. Replication and alerts list fetches go through `paginate`.

**health(alerts)** — `paginate(pathAlerts, "is_active=true", …)`; decode `severity` + `class` → `ppdd_alerts_active{severity, class}`.

---

## 5. Data flow (unchanged contract)

Collectors emit label-less `Sample`s; the loop stamps `system` and `ppdd_collector_up{collector}`. The snapshot store and `/metrics` unchecked collector are untouched. `labels_test.go` continues to guard one label-key set per metric name — the new keys (`class` on alerts; `mtree` on the new health metrics) are internally consistent across their series.

## 6. Error handling

- Primary fetch failure → collector returns error → `ppdd_collector_up{collector}=0` (existing graceful degradation).
- Best-effort sub-fetches (`/file-system`, per-MTree stats) drop only their own samples; the collector stays up if its documented primary succeeded.
- `paginate` returns the first hard error; partial pages already harvested are discarded (a paging failure mid-list is a collector failure, not silent truncation).

## 7. Testing

- Fixtures (verbatim from guide) in `internal/ppdd/testdata/`: new `system.json` (pp.11–12), rewritten `mtrees.json` (v3.0, pp.32–33), new `mtree-stats.json` (pp.34–36), updated `alerts.json` (with `class`); `file-system.json` retained for provisional extras.
- Per-collector tests updated to documented shapes.
- **Pagination test**: register `…?page=0&size=200` and `…?page=1&size=200` for one list collector; assert items past page 0 are collected.
- `ddclient.Mock` query-fallback test.
- `system_test.go` fake DD server: handlers move to `/rest/v1.0/auth` (assert flat body) and `/rest/v1.0/system`.
- `labels_test.go` stays green.
- Gate: `make ci` (gofmt + vet + race + build) and `uvx … mkdocs build --strict`.

## 8. Docs & changelog (same change, per CLAUDE.md)

- `docs/metrics.md`: add `ppdd_compression_factor`, `ppdd_mtree_degraded`, `ppdd_mtree_retention_lock_enabled`; mark provisional metrics explicitly.
- New ADR in `docs/adr/`: records the `/rest` prefix + per-resource version decision, the documented-vs-provisional split, and a **"validate against live DD" checklist** enumerating every revert point (auth body/path first).
- `CHANGELOG.md` entry.

## 9. Out of scope (future themes)

- Theme B (new domains): services status, system info/licenses, DDBoost status — `/system` already decoded here makes uptime/license metrics a trivial follow-up.
- Theme C (hygiene): `include_fields`/`exclude_fields` partial responses, server-side filters beyond `is_active`, bounded per-MTree concurrency.

## 10. Success criteria

1. All collector paths resolve through `endpoints.go`; no `/api/v1/` literals remain.
2. List collectors collect beyond 20 items (pagination test proves it).
3. `ppdd_alerts_active` reflects active alerts and carries `{severity, class}`.
4. Capacity metrics source from `/system`; `ppdd_compression_factor` exported.
5. New MTree health metrics exported; documented usage sourced from per-MTree stats.
6. No existing metric name removed; provisional ones flagged in code + docs.
7. `make ci` and `mkdocs build --strict` pass; ADR checklist present.

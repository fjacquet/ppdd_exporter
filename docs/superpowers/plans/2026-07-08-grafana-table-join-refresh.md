# Grafana Table-Join & Variable-Refresh Fix — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the Grafana detail tables render one row per entity (not one row per metric), and make the `$system` variable populate on dashboard load so panels never show stale "No data".

**Architecture:** Grafana-only change to `grafana/dashboards/ppdd-*.json`. Each multi-query table's `merge` transform (which stacks per-column query results) is replaced with `joinByField` (outer join on the row's entity field); each table query is normalized to `<agg> by (<display-keys>)(...)` so `instance`/`job` are dropped and the joined columns are predictable. A new `go test` package guards the invariants (valid JSON, `$system` refresh=on-load, no `merge` in multi-query tables) so `make ci` catches regressions — which today it does not for dashboards.

**Tech Stack:** Grafana 11 dashboard JSON; PromQL; Go 1.26 `testing` (stdlib only).

## Global Constraints

- **Grafana-only:** no changes to Go collectors, `cmd/mockdd`, fixtures, or metric names/labels.
- **Metric names unchanged** → no `docs/metrics.md` edit.
- **Datasource:** every panel/variable uses datasource uid `prometheus` (do not change).
- **Join keys (verbatim):** overview→`system`, mtrees→`mtree`, file-replication→`context`, mtree-replication→`destination`.
- **Test command:** `make test` runs `go test -race -coverprofile=cover.out -covermode=atomic ./...`. CI gate: `make ci` = `lint test build vuln`.
- **Verification split:** overview + mtrees verify against the **live** stack on the `epg` host (`docker-compose.server.yml`); replication tables verify against the **mockdd laptop demo** (`docker-compose.yml`) because the live appliance currently has zero replication contexts. mockdd fixtures already contain MTree + replication data.
- **Live-edit loop:** the running Grafana bind-mounts `grafana/dashboards` from the checkout. After pushing, on the host: `git pull` (clean — only `config.yaml` is locally modified) → `docker compose -f <file> restart grafana` → hard-refresh browser.

---

## File Structure

| File | Responsibility | Action |
|---|---|---|
| `internal/dashboards/dashboards_test.go` | Invariant guard: valid JSON, `$system` refresh=1, multi-query tables use `joinByField` not `merge` | Create |
| `grafana/dashboards/ppdd-overview.json` | "Systems at a glance" join + var refresh | Modify |
| `grafana/dashboards/ppdd-mtrees.json` | "MTree detail" join + var refresh + quota panel | Modify |
| `grafana/dashboards/ppdd-replication.json` | Both replication table joins + var refresh | Modify |
| `grafana/dashboards/ppdd-capacity.json` | var refresh | Modify |
| `grafana/dashboards/ppdd-health.json` | var refresh (its single-query table is untouched) | Modify |
| `CHANGELOG.md` | "Fixed" entry | Modify |

---

## Task 1: Guard test + `$system` refresh → on-load (all 5 dashboards)

**Files:**
- Create: `internal/dashboards/dashboards_test.go`
- Modify: `grafana/dashboards/ppdd-overview.json`, `ppdd-mtrees.json`, `ppdd-replication.json`, `ppdd-capacity.json`, `ppdd-health.json`

**Interfaces:**
- Produces: a `go test` package `dashboards` with `TestDashboardsValidJSON`, `TestSystemVarRefreshOnLoad` (Task 2 adds a third test to the same file).

- [ ] **Step 1: Write the failing tests**

Create `internal/dashboards/dashboards_test.go`:

```go
package dashboards

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// dashboardFiles returns every ppdd-*.json under grafana/dashboards, resolved
// from the repo root (located by walking up to go.mod) so the test is
// independent of the working directory `go test` runs in.
func dashboardFiles(t *testing.T) []string {
	t.Helper()
	_, self, _, _ := runtime.Caller(0)
	root := filepath.Dir(self)
	for {
		if _, err := os.Stat(filepath.Join(root, "go.mod")); err == nil {
			break
		}
		parent := filepath.Dir(root)
		if parent == root {
			t.Fatal("could not locate repo root (go.mod)")
		}
		root = parent
	}
	files, err := filepath.Glob(filepath.Join(root, "grafana", "dashboards", "ppdd-*.json"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no dashboards matched (err=%v)", err)
	}
	return files
}

type ddTransform struct {
	ID string `json:"id"`
}
type ddPanel struct {
	Title           string            `json:"title"`
	Type            string            `json:"type"`
	Targets         []json.RawMessage `json:"targets"`
	Transformations []ddTransform     `json:"transformations"`
	Panels          []ddPanel         `json:"panels"`
}
type ddVar struct {
	Name    string `json:"name"`
	Refresh int    `json:"refresh"`
}
type ddDashboard struct {
	Templating struct {
		List []ddVar `json:"list"`
	} `json:"templating"`
	Panels []ddPanel `json:"panels"`
}

func load(t *testing.T, path string) ddDashboard {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var d ddDashboard
	if err := json.Unmarshal(b, &d); err != nil {
		t.Fatalf("%s is not valid JSON: %v", filepath.Base(path), err)
	}
	return d
}

func eachPanel(panels []ddPanel, fn func(ddPanel)) {
	for _, p := range panels {
		fn(p)
		eachPanel(p.Panels, fn)
	}
}

func TestDashboardsValidJSON(t *testing.T) {
	for _, f := range dashboardFiles(t) {
		load(t, f)
	}
}

func TestSystemVarRefreshOnLoad(t *testing.T) {
	for _, f := range dashboardFiles(t) {
		d := load(t, f)
		for _, v := range d.Templating.List {
			if v.Name == "system" && v.Refresh != 1 {
				t.Errorf("%s: $system var refresh=%d, want 1 (on dashboard load)", filepath.Base(f), v.Refresh)
			}
		}
	}
}

// TestMultiQueryTablesJoinNotMerge is added in Task 2.
```

- [ ] **Step 2: Run tests to verify `TestSystemVarRefreshOnLoad` fails**

Run: `go test ./internal/dashboards/ -run TestSystemVarRefreshOnLoad -v`
Expected: FAIL — every dashboard reports `$system var refresh=2, want 1`.

- [ ] **Step 3: Flip the refresh value in all 5 dashboards**

The `$system` template variable is the only key whose value is the integer `2` (the
dashboard-level auto-refresh is a string like `"1m"`). Confirm one hit per file, then replace:

```bash
grep -rn '"refresh": 2' grafana/dashboards/ppdd-*.json    # expect exactly 1 per file
```

For each of the 5 files, change that one line from `"refresh": 2` to `"refresh": 1`
(edit in place; do not reformat the rest of the file).

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/dashboards/ -run 'TestDashboardsValidJSON|TestSystemVarRefreshOnLoad' -v`
Expected: PASS (both).

- [ ] **Step 5: Commit**

```bash
git add internal/dashboards/dashboards_test.go grafana/dashboards/ppdd-*.json
git commit -m "fix(grafana): \$system variable refreshes on load; add dashboard guard test"
```

---

## Task 2: Replace `merge` with `joinByField` in all 4 multi-query tables

Each table currently runs N per-column queries and stitches them with `merge`, which stacks
them into N×entities rows. Fix = (a) normalize each query to `<agg> by (<display-keys>)(...)`
so only the display-key labels + value survive, and (b) swap `merge` → `joinByField` (outer)
on the row's entity field. The `organize` transform keeps its `indexByName`/`renameByName` and
gains `excludeByName` entries for the duplicate `Time N` / secondary-key columns the join
appends. Exact duplicate names are confirmed by rendering (verify steps below).

**Files:**
- Modify: `internal/dashboards/dashboards_test.go` (add third test)
- Modify: `ppdd-overview.json`, `ppdd-mtrees.json`, `ppdd-replication.json`

**Interfaces:**
- Consumes: `dashboardFiles`, `ddPanel`, `eachPanel` from Task 1.

- [ ] **Step 1: Add the failing join test**

Append to `internal/dashboards/dashboards_test.go`:

```go
func TestMultiQueryTablesJoinNotMerge(t *testing.T) {
	for _, f := range dashboardFiles(t) {
		d := load(t, f)
		eachPanel(d.Panels, func(p ddPanel) {
			if p.Type != "table" || len(p.Targets) < 2 {
				return
			}
			var hasJoin bool
			for _, tr := range p.Transformations {
				if tr.ID == "merge" {
					t.Errorf("%s: table %q uses 'merge' (row-explodes); use 'joinByField'", filepath.Base(f), p.Title)
				}
				if tr.ID == "joinByField" {
					hasJoin = true
				}
			}
			if !hasJoin {
				t.Errorf("%s: multi-query table %q has no 'joinByField' transform", filepath.Base(f), p.Title)
			}
		})
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./internal/dashboards/ -run TestMultiQueryTablesJoinNotMerge -v`
Expected: FAIL — 4 panels report `uses 'merge'` (Systems at a glance, MTree detail, MTree replication contexts, File replication contexts).

- [ ] **Step 3: Fix `ppdd-overview.json` → "Systems at a glance" (join on `system`)**

Set the 8 target `expr` values (wrap the non-aggregated ones in `by (system)`):

| refId | new expr |
|---|---|
| Used | `max by (system) (100 * ppdd_filesystem_used_bytes{system=~"$system"} / ppdd_filesystem_total_bytes{system=~"$system"})` |
| UsedBytes | `sum by (system) (ppdd_filesystem_used_bytes{system=~"$system"})` |
| Total | `sum by (system) (ppdd_filesystem_total_bytes{system=~"$system"})` |
| Dedup | `max by (system) (ppdd_compression_factor{system=~"$system"})` |
| Alerts | `sum by (system) (ppdd_alerts_active{system=~"$system"})` *(unchanged)* |
| Disks | `sum by (system) (ppdd_disk_failed{system=~"$system"})` *(unchanged)* |
| ReplBad | `count by (system) (ppdd_mtree_replication_state{system=~"$system", state!="NORMAL"} == 1)` *(unchanged)* |
| CPU | `max by (system) (ppdd_system_cpu_percent{system=~"$system"})` |

Replace the panel's `transformations` array with (join key `system` is unique per row, so the
only duplicated column is `Time`; 8 frames → `Time` + `Time 1`…`Time 7`):

```json
"transformations": [
  { "id": "joinByField", "options": { "byField": "system", "mode": "outer" } },
  { "id": "organize", "options": {
    "excludeByName": { "Time": true, "Time 1": true, "Time 2": true, "Time 3": true, "Time 4": true, "Time 5": true, "Time 6": true, "Time 7": true, "__name__": true, "job": true, "instance": true },
    "indexByName": { "system": 0, "Value #Used": 1, "Value #UsedBytes": 2, "Value #Total": 3, "Value #Dedup": 4, "Value #CPU": 5, "Value #Alerts": 6, "Value #Disks": 7, "Value #ReplBad": 8 },
    "renameByName": { "system": "System", "Value #Used": "Used %", "Value #UsedBytes": "Used", "Value #Total": "Total", "Value #Dedup": "Dedup x", "Value #CPU": "CPU %", "Value #Alerts": "Alerts", "Value #Disks": "Disks failed", "Value #ReplBad": "Repl not normal" }
  } }
]
```

- [ ] **Step 4: Fix `ppdd-mtrees.json` → "MTree detail" (join on `mtree`)**

Set the 7 target `expr` values to `<agg> by (system, mtree)(...)`:

| refId | new expr |
|---|---|
| Logical | `sum by (system, mtree) (ppdd_mtree_logical_used_bytes{system=~"$system"})` |
| Physical | `sum by (system, mtree) (ppdd_mtree_physical_used_bytes{system=~"$system"})` |
| Compression | `max by (system, mtree) (ppdd_mtree_compression_factor{system=~"$system"})` |
| QuotaSoft | `sum by (system, mtree) (ppdd_mtree_quota_soft_limit_bytes{system=~"$system"})` |
| QuotaHard | `sum by (system, mtree) (ppdd_mtree_quota_hard_limit_bytes{system=~"$system"})` |
| Degraded | `max by (system, mtree) (ppdd_mtree_degraded{system=~"$system"})` |
| RetentionLock | `max by (system, mtree) (ppdd_mtree_retention_lock_enabled{system=~"$system"})` |

Replace the panel's `transformations` array with (join key `mtree`; 7 frames each carry
`system`, so exclude `system 1`…`system 6` and `Time`…`Time 6`, keep the first `system`):

```json
"transformations": [
  { "id": "joinByField", "options": { "byField": "mtree", "mode": "outer" } },
  { "id": "organize", "options": {
    "excludeByName": { "Time": true, "Time 1": true, "Time 2": true, "Time 3": true, "Time 4": true, "Time 5": true, "Time 6": true, "system 1": true, "system 2": true, "system 3": true, "system 4": true, "system 5": true, "system 6": true, "__name__": true, "job": true, "instance": true },
    "indexByName": { "system": 0, "mtree": 1, "Value #Logical": 2, "Value #Physical": 3, "Value #Compression": 4, "Value #QuotaSoft": 5, "Value #QuotaHard": 6, "Value #Degraded": 7, "Value #RetentionLock": 8 },
    "renameByName": { "system": "System", "mtree": "MTree", "Value #Logical": "Logical used", "Value #Physical": "Physical used", "Value #Compression": "Dedup x", "Value #QuotaSoft": "Quota soft", "Value #QuotaHard": "Quota hard", "Value #Degraded": "Degraded", "Value #RetentionLock": "Retention lock" }
  } }
]
```

- [ ] **Step 5: Fix `ppdd-replication.json` → both tables**

"File replication contexts" (join on `context`), set 3 exprs to `by (system, context)`:

| refId | new expr |
|---|---|
| Network | `sum by (system, context) (ppdd_file_replication_network_bytes{system=~"$system"})` |
| Logical | `sum by (system, context) (ppdd_file_replication_logical_replicated_bytes{system=~"$system"})` |
| Files | `sum by (system, context) (ppdd_file_replication_active_files{system=~"$system"})` |

```json
"transformations": [
  { "id": "joinByField", "options": { "byField": "context", "mode": "outer" } },
  { "id": "organize", "options": {
    "excludeByName": { "Time": true, "Time 1": true, "Time 2": true, "system 1": true, "system 2": true, "__name__": true, "job": true, "instance": true },
    "indexByName": { "system": 0, "context": 1, "Value #Network": 2, "Value #Logical": 3, "Value #Files": 4 },
    "renameByName": { "system": "System", "context": "Context", "Value #Network": "Network bytes", "Value #Logical": "Logical replicated", "Value #Files": "Active files" }
  } }
]
```

"MTree replication contexts" (join on `destination`). Normalize the 3 numeric queries; keep the
`State` query as-is (it uses `label_replace` to surface the `current_state` string and cannot be
aggregated without losing that label):

| refId | new expr |
|---|---|
| State | `label_replace(ppdd_mtree_replication_state{system=~"$system"} == 1, "current_state", "$1", "state", "(.*)")` *(unchanged)* |
| Connected | `max by (system, source, destination) (ppdd_mtree_replication_connected{system=~"$system"})` |
| Enabled | `max by (system, source, destination) (ppdd_mtree_replication_enabled{system=~"$system"})` |
| Resync | `max by (system, source, destination) (ppdd_mtree_replication_need_resync{system=~"$system"})` |

```json
"transformations": [
  { "id": "joinByField", "options": { "byField": "destination", "mode": "outer" } },
  { "id": "organize", "options": {
    "excludeByName": { "Time": true, "Time 1": true, "Time 2": true, "Time 3": true, "system 1": true, "system 2": true, "system 3": true, "source 1": true, "source 2": true, "source 3": true, "__name__": true, "job": true, "instance": true, "state": true, "Value #State": true },
    "indexByName": { "system": 0, "source": 1, "destination": 2, "current_state": 3, "Value #Connected": 4, "Value #Enabled": 5, "Value #Resync": 6 },
    "renameByName": { "system": "System", "source": "Source", "destination": "Destination", "current_state": "State", "Value #Connected": "Connected", "Value #Enabled": "Enabled", "Value #Resync": "Need resync" }
  } }
]
```

- [ ] **Step 6: Run tests + JSON validity**

Run: `go test ./internal/dashboards/ -v`
Expected: PASS (all three tests, including `TestMultiQueryTablesJoinNotMerge`).

- [ ] **Step 7: Verify overview + mtrees render on the live stack (epg)**

On the `epg` host:
```bash
cd ~/ppdd_exporter && git pull --ff-only
docker compose -f docker-compose.server.yml restart grafana
```
Hard-refresh Grafana. Expected:
- **Systems at a glance:** exactly **1 row** (`diab-DD3410`) with Used %, Used, Total, Dedup x, CPU %, Alerts, Disks failed all populated on the same line.
- **MTree detail:** exactly **2 rows** (`/data/col1/alfred`, `/data/col1/backup`), each with Logical/Physical/Dedup/Quota columns on one line.

If any stray column shows (e.g. an un-excluded `Time 7` or `source`), note its exact header and add `"<name>": true` to that panel's `excludeByName`, then re-verify.

- [ ] **Step 8: Verify replication tables on the mockdd demo**

On a machine with the repo (laptop is fine — no appliance needed):
```bash
docker compose up -d --build      # docker-compose.yml → mockdd
```
Open `http://localhost:3000` → PowerProtect DD — Replication. Expected: "MTree replication
contexts" and "File replication contexts" each show **one row per context** (mockdd fixtures
`mtree-replications.json` / `file-replications.json`), all columns on one line, State column
populated. Adjust `excludeByName` for any stray column and re-verify. Tear down: `docker compose down`.

- [ ] **Step 9: Commit**

```bash
git add internal/dashboards/dashboards_test.go grafana/dashboards/ppdd-overview.json grafana/dashboards/ppdd-mtrees.json grafana/dashboards/ppdd-replication.json
git commit -m "fix(grafana): join detail tables by entity (joinByField) instead of merge"
```

---

## Task 3: "Quota utilization %" empty-state polish

The bargauge query filters to `(quota_hard > 0)`, so an appliance with no MTree quotas returns
zero series → "No data". Add an `or` fallback that emits `0` for every MTree, so unconfigured
quotas read as `0%` bars instead of a broken-looking panel.

**Files:**
- Modify: `grafana/dashboards/ppdd-mtrees.json` ("Quota utilization %" panel)

- [ ] **Step 1: Change the query expr**

Set the "Quota utilization %" panel's single target `expr` to:

```
100 * ppdd_mtree_logical_used_bytes{system=~"$system"} / (ppdd_mtree_quota_hard_limit_bytes{system=~"$system"} > 0) or (ppdd_mtree_logical_used_bytes{system=~"$system"} >= 0) * 0
```

(The first term yields real % for quota'd MTrees; `or` supplies `0` for MTrees absent from it — i.e. those with no hard quota — preserving the `mtree` label so each shows a 0% bar.)

- [ ] **Step 2: Validate JSON**

Run: `go test ./internal/dashboards/ -run TestDashboardsValidJSON -v`
Expected: PASS.

- [ ] **Step 3: Verify on the live stack (epg)**

`git pull --ff-only && docker compose -f docker-compose.server.yml restart grafana` on epg,
hard-refresh. Expected: "Quota utilization %" now shows `/data/col1/alfred` and `/data/col1/backup`
at **0%** (not "No data").

- [ ] **Step 4: Commit**

```bash
git add grafana/dashboards/ppdd-mtrees.json
git commit -m "fix(grafana): quota utilization shows 0% instead of No data when unquota'd"
```

---

## Task 4: CHANGELOG + full CI gate + final verification

**Files:**
- Modify: `CHANGELOG.md`

- [ ] **Step 1: Add a CHANGELOG entry**

Under the top `## [Unreleased]` (or a new one if absent), add to a `### Fixed` list:

```markdown
### Fixed
- Grafana detail tables (Systems at a glance, MTree detail, replication contexts) now render
  one row per entity instead of one row per metric (joinByField replaces the merge transform).
- The `$system` dashboard variable now refreshes on load, so panels no longer show stale
  "No data" until a hard browser refresh.
- "Quota utilization %" shows 0% for MTrees with no hard quota instead of "No data".
```

- [ ] **Step 2: Run the full CI gate**

Run: `make ci`
Expected: PASS — gofmt/vet/lint clean, all tests pass (including `internal/dashboards`), build succeeds, govulncheck clean.

- [ ] **Step 3: Final live sanity check (epg)**

Confirm on the live stack that all four dashboards (overview, mtrees, capacity, health) open
fresh — with **no hard refresh** — and populate immediately (validates the refresh=1 fix), and
that each detail table shows one row per entity.

- [ ] **Step 4: Commit**

```bash
git add CHANGELOG.md
git commit -m "docs: changelog for grafana table-join + refresh fixes"
```

- [ ] **Step 5: Open PR**

```bash
git push -u origin fix/grafana-table-join-refresh
gh pr create --fill
```

---

## Self-Review

**Spec coverage:**
- Decision 1 (table joins, all 4 panels) → Task 2 (overview, mtrees, both replication tables). ✓
- Decision 2 (`refresh`→on-load, all 5) → Task 1. ✓
- Decision 3 (quota polish) → Task 3. ✓
- Decision 4 (CHANGELOG; no metrics.md/mockdd/collector change) → Task 4; constraints enforce no other changes. ✓
- Spec risk "transforms are finicky / confirm duplicate columns live" → Task 2 Steps 7–8 render-and-adjust. ✓
- Spec risk "mtree-replication join key confirm live" → Task 2 Step 8 (mockdd). ✓
- Spec verification loop (push→pull→restart grafana; mockdd exercises joins) → encoded in Global Constraints + Task 2/3 verify steps. ✓

**Placeholder scan:** every expr, transform block, and test is spelled out. The "adjust excludeByName if a stray column appears" steps are bounded corrections to concrete starting values (the deterministic `Time N`/`system N`/`source N` duplicates), verified by rendering — not open-ended TODOs.

**Type consistency:** `dashboardFiles`, `load`, `eachPanel`, `ddPanel`, `ddVar`, `ddDashboard` are defined in Task 1 and reused unchanged in Task 2. refIds in the expr tables match the `Value #<refId>` keys in each panel's `organize` `indexByName`/`renameByName`. Join keys match the Global Constraints line.

## Known limitation (documented, not a defect)

`joinByField` joins on a single field, so the per-entity tables key on `mtree` / `destination` /
`context`. For a multi-appliance fleet where two systems share an identical MTree/context name,
those rows would merge. Acceptable now (single appliance) and for typical per-system viewing; a
composite join key is a future refinement if fleet rollups surface it.

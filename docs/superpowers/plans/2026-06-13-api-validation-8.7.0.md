# DD 8.7.0 API Mapping Correction — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Correct every exporter endpoint path and JSON field mapping to match the authoritative PowerProtect DD 8.7.0 OpenAPI spec (`docs/swagger/13345-8.7.0.json`), and split the bogus replication collector into MTree-replication and file-replication collectors.

**Architecture:** Each metric domain is a `ResourceCollector` (`internal/ppdd/`) owning its endpoint path, JSON struct, and `parse → []Sample`. Corrections are localized: one struct + one `testdata/*.json` fixture per module, plus a matching update to `cmd/mockdd` (the demo appliance) so the Compose demo keeps working. Tests are table-driven against fixtures using `ddclient.Mock`.

**Tech Stack:** Go, `encoding/json`, logrus, Prometheus client; `make ci` (gofmt + vet + golangci-lint + race tests + govulncheck + build) is the gate.

---

## Background facts (validated against `docs/swagger/13345-8.7.0.json`)

Real enum values used in fixtures below:
- `mtreeIsdegraded`: `not-degraded`, `degraded`
- `retentionLockStatus_2_0`: `never-enabled`, `enabled`, `disabled`, `status-unknown`
- `DiskStatusEnum`: `...`, `FAILED`, `...` (failed disk uses `FAILED`)
- `alertSeverity`: `DEBUG|INFO|NOTICE|WARNING|ERROR|CRITICAL|ALERT|EMERGENCY`
- `alertClass`: `Cifs|Storage|Cluster|Network|Security|Filesystem|Environment|Replication|HardwareFailure|SystemMaintenance|Syslog|Firmware|ha|Cloud|capacity|dataAvailability|infrastructure`
- `cleanInfo` (`fs_clean_status`): `clean|running|inactive|waiting`
- `MtreeReplicationState`: `CONNECTING|UNINITIALIZED|INITIALIZING|NORMAL|RESYNCING|RECOVERING`
- `MtreeReplicationMode`: `SOURCE|TARGET`
- `fileReplStatus` (`repl_status`): `completed|error|warning|unknown`
- `fileReplDirection` (`direction`): `inbound|outbound`

The list `paging` envelope (`current_page/page_entries/total_entries/page_size`) is unchanged — `pagingInfo` and `paginate()` are correct as-is.

## File structure (created / modified)

| File | Responsibility | Action |
|---|---|---|
| `internal/ppdd/endpoints.go` | path constants | modify (rename/repoint values; add 2, remove 1) |
| `internal/ppdd/capacity.go` | capacity + clean state | modify (drop compression split; `fs_clean_status`) |
| `internal/ppdd/capacity_test.go` | capacity test | modify |
| `internal/ppdd/testdata/file-systems.json` | capacity-extra fixture | create (replaces `file-system.json`) |
| `internal/ppdd/mtrees.go` | mtree metadata + usage | modify (`quota_config`; top-level compression) |
| `internal/ppdd/testdata/mtrees.json` | mtree list fixture | modify (`quota_config`) |
| `internal/ppdd/health.go` | disks + alerts + perf | modify (3 fixes) |
| `internal/ppdd/health_test.go` | health test | modify |
| `internal/ppdd/testdata/disks.json` | disks fixture | modify (`diskInfo`/`status`) |
| `internal/ppdd/testdata/performance.json` | perf fixture | create (replaces `system-stats.json`) |
| `internal/ppdd/testdata/alerts.json` | alerts fixture | modify (`alert_list`) |
| `internal/ppdd/mtree_replication.go` | MTree replication posture | create |
| `internal/ppdd/mtree_replication_test.go` | test | create |
| `internal/ppdd/testdata/mtree-replications.json` | fixture | create |
| `internal/ppdd/file_replication.go` | file replication stats | create |
| `internal/ppdd/file_replication_test.go` | test | create |
| `internal/ppdd/testdata/file-replications.json` | fixture | create |
| `internal/ppdd/replication.go` | old bogus collector | delete |
| `internal/ppdd/replication_test.go` | old test | delete |
| `internal/ppdd/testdata/replications.json` | old fixture | delete |
| `internal/ppdd/resource.go` | Registry | modify (swap collectors) |
| `internal/ppdd/labels_test.go` | full-mock helper | modify (`buildFullMock`) |
| `cmd/mockdd/main.go` | demo routes | modify |
| `cmd/mockdd/fixtures/*.json` | demo fixtures | modify/rename to match |
| `docs/metrics.md` | metric reference | modify |
| `grafana/dashboards/ppdd-overview.json` | demo dashboard panels | modify |
| `CHANGELOG.md` | changelog | modify |

---

## Task 1: Capacity — `/file-systems` + drop compression split

**Files:**
- Modify: `internal/ppdd/endpoints.go` (value of `pathFileSystem`)
- Modify: `internal/ppdd/capacity.go`
- Create: `internal/ppdd/testdata/file-systems.json`
- Delete: `internal/ppdd/testdata/file-system.json`
- Modify: `internal/ppdd/capacity_test.go`
- Modify: `internal/ppdd/labels_test.go` (`buildFullMock` fixture path)

- [ ] **Step 1: Create the new fixture**

Create `internal/ppdd/testdata/file-systems.json`:

```json
{
  "hostname": "dd01",
  "fs_clean_status": "running",
  "fs_uptime_secs": 256674
}
```

- [ ] **Step 2: Update the capacity test (failing)**

Replace the body of `TestCapacityCollect` in `internal/ppdd/capacity_test.go` so it loads the renamed fixture and drops the compression-split assertions:

```go
func TestCapacityCollect(t *testing.T) {
	m := ddclient.NewMock("dd01")
	for path, file := range map[string]string{
		pathSystem:     "testdata/system.json",
		pathFileSystem: "testdata/file-systems.json",
	} {
		b, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		m.SetJSON(path, string(b))
	}

	got, err := Capacity{}.Collect(context.Background(), m)
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	want := map[string]float64{
		"ppdd_filesystem_total_bytes":      100000000000,
		"ppdd_filesystem_used_bytes":       40000000000,
		"ppdd_filesystem_available_bytes":  60000000000,
		"ppdd_compression_factor":          9.9,
		"ppdd_filesystem_cleaning_running": 1,
	}
	seen := map[string]float64{}
	for _, s := range got {
		seen[s.Name] = s.Value
	}
	for name, v := range want {
		if seen[name] != v {
			t.Errorf("%s = %v, want %v", name, seen[name], v)
		}
	}
	for _, gone := range []string{"ppdd_compression_global_factor", "ppdd_compression_local_factor", "ppdd_compression_total_factor"} {
		if _, ok := seen[gone]; ok {
			t.Errorf("metric %s should no longer be emitted", gone)
		}
	}
}
```

Also in `TestCapacityProvisionalIsBestEffort`, change `"testdata/system.json"` references as-is (it only loads `pathSystem`, no change needed) — leave that test unchanged.

- [ ] **Step 3: Run the test to verify it fails**

Run: `go test ./internal/ppdd/ -run TestCapacity -v`
Expected: FAIL (old fixture file missing / compression-split metrics still emitted).

- [ ] **Step 4: Repoint the endpoint constant**

In `internal/ppdd/endpoints.go`, change the `pathFileSystem` line to:

```go
	pathFileSystem  = "/rest/v1.0/dd-systems/0/file-systems" // validated 8.7.0: filesysInfo (clean state)
```

- [ ] **Step 5: Rewrite the capacity provisional struct + logic**

In `internal/ppdd/capacity.go`, replace `fileSystemResp` and `capacityProvisional`:

```go
// fileSystemResp is the validated 8.7.0 /file-systems shape (schema filesysInfo).
// Only the GC clean state is consumed; fetched best-effort.
type fileSystemResp struct {
	CleanStatus string `json:"fs_clean_status"` // enum: clean|running|inactive|waiting
}
```

```go
// capacityProvisional fetches the /file-systems clean state (best-effort: a failure
// drops only this sample, never the whole collector).
func capacityProvisional(ctx context.Context, c ddclient.Client) []Sample {
	var fs fileSystemResp
	if err := c.Get(ctx, pathFileSystem, &fs); err != nil {
		return nil
	}
	cleaning := 0.0
	if fs.CleanStatus == "running" {
		cleaning = 1
	}
	return []Sample{
		{Name: "ppdd_filesystem_cleaning_running", Value: cleaning},
	}
}
```

Leave `Collect` and `systemResp` unchanged (they are correct).

- [ ] **Step 6: Update `buildFullMock` to the renamed fixture**

In `internal/ppdd/labels_test.go`, in `buildFullMock`, change the `pathFileSystem` map entry from `"testdata/file-system.json"` to `"testdata/file-systems.json"`. (Leave the `pathReplication` entry alone for now — Task 7 replaces it.)

- [ ] **Step 7: Delete the old fixture**

Run: `git rm internal/ppdd/testdata/file-system.json`

- [ ] **Step 8: Run the package tests to verify they pass**

Run: `go test ./internal/ppdd/ -run 'TestCapacity|TestLabelKey' -v`
Expected: PASS.

- [ ] **Step 9: Commit**

```bash
git add internal/ppdd/endpoints.go internal/ppdd/capacity.go internal/ppdd/capacity_test.go internal/ppdd/labels_test.go internal/ppdd/testdata/file-systems.json
git commit -m "fix(capacity): correct /file-systems path and drop unsourced compression split (8.7.0)

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 2: MTrees — `quota_config` + top-level compression_factor

**Files:**
- Modify: `internal/ppdd/mtrees.go`
- Modify: `internal/ppdd/testdata/mtrees.json`
- Modify: `internal/ppdd/mtrees_test.go` (assertion name unchanged; value stays)

- [ ] **Step 1: Update the mtree list fixture**

Replace `internal/ppdd/testdata/mtrees.json` with the validated quota shape:

```json
{
  "mtree": [
    {
      "id": "%2Fdata%2Fcol1%2Fbackup1",
      "name": "/data/col1/backup1",
      "is_degraded": "degraded",
      "mtree_rl_detail": { "rl_status": "enabled" },
      "quota_config": { "soft_limit": 10000000000, "hard_limit": 12000000000 }
    }
  ],
  "paging_info": { "current_page": 0, "page_entries": 1, "total_entries": 1, "page_size": 200 }
}
```

- [ ] **Step 2: Update the per-mtree stats fixture to include top-level compression_factor**

Replace `internal/ppdd/testdata/mtree-stats.json` (add top-level `compression_factor` to the latest epoch entry; keep tier arrays for logical-used/post-comp):

```json
{
  "stats_capacity": [
    {
      "collection_epoch": 1589223600,
      "compression_factor": 1.0,
      "tier_capacity_usage": [ { "tier": "active", "logical_capacity": { "used": 1 } } ],
      "tier_data_written": [ { "tier": "active", "compression_factor": 1.0, "post_comp_written": 1 } ]
    },
    {
      "collection_epoch": 1589310000,
      "compression_factor": 6.25,
      "tier_capacity_usage": [ { "tier": "active", "logical_capacity": { "used": 5000000000 } } ],
      "tier_data_written": [ { "tier": "active", "compression_factor": 6.25, "post_comp_written": 800000000 } ]
    }
  ]
}
```

- [ ] **Step 3: Run the test to verify it fails**

Run: `go test ./internal/ppdd/ -run TestMTrees -v`
Expected: FAIL — `ppdd_mtree_quota_soft_limit_bytes` is now 0 (struct still reads the old top-level keys).

- [ ] **Step 4: Fix the mtree struct (quota nesting)**

In `internal/ppdd/mtrees.go`, replace the two PROVISIONAL quota fields in `mtreeListItem` with the nested `quota_config`:

```go
	QuotaConfig struct {
		SoftLimit float64 `json:"soft_limit"`
		HardLimit float64 `json:"hard_limit"`
	} `json:"quota_config"` // validated 8.7.0: schema quotaConfig
```

(Remove the old `QuotaSoftLimit` / `QuotaHardLimit` fields.)

- [ ] **Step 5: Update the quota sample emission**

In `MTrees.Collect`, change the two quota samples to read from `mt.QuotaConfig`:

```go
			Sample{Name: "ppdd_mtree_quota_soft_limit_bytes", Labels: lbl, Value: mt.QuotaConfig.SoftLimit},
			Sample{Name: "ppdd_mtree_quota_hard_limit_bytes", Labels: lbl, Value: mt.QuotaConfig.HardLimit},
```

- [ ] **Step 6: Use top-level compression_factor in usage**

In `mtreeStatsResp`, add the top-level field to the `StatsCapacity` element struct:

```go
	StatsCapacity []struct {
		CollectionEpoch   int64   `json:"collection_epoch"`
		CompressionFactor float64 `json:"compression_factor"` // validated 8.7.0: top-level
		TierCapacityUsage []struct {
			LogicalCapacity struct {
				Used float64 `json:"used"`
			} `json:"logical_capacity"`
		} `json:"tier_capacity_usage"`
		TierDataWritten []struct {
			CompressionFactor float64 `json:"compression_factor"`
			PostCompWritten   float64 `json:"post_comp_written"`
		} `json:"tier_data_written"`
	} `json:"stats_capacity"`
```

In `mtreeUsage`, replace the tier-weighted compression block. Keep summing `postComp` for `ppdd_mtree_physical_used_bytes`, but take compression directly:

```go
	var logicalUsed, postComp float64
	for _, t := range latest.TierCapacityUsage {
		logicalUsed += t.LogicalCapacity.Used
	}
	for _, t := range latest.TierDataWritten {
		postComp += t.PostCompWritten
	}
	comp := latest.CompressionFactor
	lbl := []Label{{Key: "mtree", Value: mt.Name}}
	return []Sample{
		{Name: "ppdd_mtree_logical_used_bytes", Labels: lbl, Value: logicalUsed},
		{Name: "ppdd_mtree_compression_factor", Labels: lbl, Value: comp},
		{Name: "ppdd_mtree_physical_used_bytes", Labels: lbl, Value: postComp},
	}
```

- [ ] **Step 7: Run the test to verify it passes**

Run: `go test ./internal/ppdd/ -run TestMTrees -v`
Expected: PASS (`ppdd_mtree_compression_factor` = 6.25 from top-level; quota soft = 10000000000).

- [ ] **Step 8: Commit**

```bash
git add internal/ppdd/mtrees.go internal/ppdd/testdata/mtrees.json internal/ppdd/testdata/mtree-stats.json
git commit -m "fix(mtrees): read quota_config and top-level compression_factor (8.7.0)

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 3: Health/disks — `/api/v1/.../storage/disks`

**Files:**
- Modify: `internal/ppdd/endpoints.go` (`pathDisks` value)
- Modify: `internal/ppdd/health.go` (`healthDisks`)
- Modify: `internal/ppdd/testdata/disks.json`
- Modify: `internal/ppdd/health_test.go` (disk id assertion unchanged: `1b`)

- [ ] **Step 1: Update the disks fixture**

Replace `internal/ppdd/testdata/disks.json`:

```json
{ "diskInfo": [
  { "id": "1a", "status": "IN_USE" },
  { "id": "1b", "status": "FAILED" }
] }
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/ppdd/ -run TestHealthCollect -v`
Expected: FAIL — `disk 1b failed = 0, want 1` (struct reads `disk`/`state`, fixture has `diskInfo`/`status`).

- [ ] **Step 3: Repoint the endpoint constant**

In `internal/ppdd/endpoints.go`, change `pathDisks`:

```go
	pathDisks       = "/api/v1/dd-systems/0/storage/disks" // validated 8.7.0: schema DiskInfos
```

- [ ] **Step 4: Fix `healthDisks`**

In `internal/ppdd/health.go`, replace `healthDisks`:

```go
func healthDisks(ctx context.Context, c ddclient.Client) []Sample {
	var r struct {
		DiskInfo []struct {
			ID     string `json:"id"`
			Status string `json:"status"` // enum DiskStatusEnum; FAILED == failed
		} `json:"diskInfo"`
	}
	if err := c.Get(ctx, pathDisks, &r); err != nil {
		return nil
	}
	var out []Sample
	for _, d := range r.DiskInfo {
		failed := 0.0
		if d.Status == "FAILED" {
			failed = 1
		}
		out = append(out, Sample{Name: "ppdd_disk_failed", Labels: []Label{{Key: "disk", Value: d.ID}}, Value: failed})
	}
	return out
}
```

- [ ] **Step 5: Run the test to verify it passes**

Run: `go test ./internal/ppdd/ -run TestHealthCollect -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/ppdd/endpoints.go internal/ppdd/health.go internal/ppdd/testdata/disks.json
git commit -m "fix(health): correct disks endpoint to /api/v1 storage/disks (8.7.0)

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 4: Health/perf — `/api/v3/.../stats/performance`

**Files:**
- Modify: `internal/ppdd/endpoints.go` (rename `pathSystemStats` → `pathPerformance`)
- Modify: `internal/ppdd/health.go` (`healthSystemStats`)
- Create: `internal/ppdd/testdata/performance.json`
- Delete: `internal/ppdd/testdata/system-stats.json`
- Modify: `internal/ppdd/health_test.go` (`loadHealthMock`)

- [ ] **Step 1: Create the performance fixture**

Create `internal/ppdd/testdata/performance.json` (array; latest epoch wins):

```json
{
  "statsPerformance": [
    { "collectionEpoch": 1589223600, "averageCpuUtilization": 10, "throughput": { "read": 1, "write": 1 } },
    { "collectionEpoch": 1589310000, "averageCpuUtilization": 38, "throughput": { "read": 200000000, "write": 150000000 } }
  ],
  "paging_info": { "current_page": 0, "page_entries": 2, "total_entries": 2, "page_size": 200 }
}
```

- [ ] **Step 2: Update the health test for the new value + fixture name**

In `internal/ppdd/health_test.go`, in `loadHealthMock` change the map entry key and file:

```go
		pathPerformance: "testdata/performance.json",
```

(Remove the `pathSystemStats: "testdata/system-stats.json"` entry.)

In `TestHealthCollect`, change the cpu expectation from `37.5` to `38`:

```go
	if seen["cpu"] != 38 {
		t.Errorf("cpu = %v, want 38", seen["cpu"])
	}
```

- [ ] **Step 3: Run the test to verify it fails**

Run: `go test ./internal/ppdd/ -run TestHealthCollect -v`
Expected: FAIL (compile error: `pathPerformance` undefined).

- [ ] **Step 4: Rename the endpoint constant**

In `internal/ppdd/endpoints.go`, replace the `pathSystemStats` line:

```go
	pathPerformance = "/api/v3/dd-systems/0/stats/performance" // validated 8.7.0: schema SystemPerformance
```

- [ ] **Step 5: Rewrite `healthSystemStats`**

In `internal/ppdd/health.go`, replace `healthSystemStats`:

```go
func healthSystemStats(ctx context.Context, c ddclient.Client) []Sample {
	var r struct {
		StatsPerformance []struct {
			CollectionEpoch       int64   `json:"collectionEpoch"`
			AverageCPUUtilization float64 `json:"averageCpuUtilization"`
			Throughput            struct {
				Read  float64 `json:"read"`
				Write float64 `json:"write"`
			} `json:"throughput"`
		} `json:"statsPerformance"`
	}
	if err := c.Get(ctx, pathPerformance, &r); err != nil || len(r.StatsPerformance) == 0 {
		return nil
	}
	latest := r.StatsPerformance[0]
	for _, s := range r.StatsPerformance[1:] {
		if s.CollectionEpoch > latest.CollectionEpoch {
			latest = s
		}
	}
	return []Sample{
		{Name: "ppdd_system_cpu_percent", Value: latest.AverageCPUUtilization},
		{Name: "ppdd_system_read_bytes_per_second", Value: latest.Throughput.Read},
		{Name: "ppdd_system_write_bytes_per_second", Value: latest.Throughput.Write},
	}
}
```

- [ ] **Step 6: Delete the old fixture**

Run: `git rm internal/ppdd/testdata/system-stats.json`

- [ ] **Step 7: Run the test to verify it passes**

Run: `go test ./internal/ppdd/ -run TestHealthCollect -v`
Expected: PASS (cpu = 38).

- [ ] **Step 8: Commit**

```bash
git add internal/ppdd/endpoints.go internal/ppdd/health.go internal/ppdd/health_test.go internal/ppdd/testdata/performance.json
git commit -m "fix(health): correct system perf to /api/v3 stats/performance array (8.7.0)

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 5: Health/alerts — `alert_list` key + real enum casing

**Files:**
- Modify: `internal/ppdd/health.go` (`healthAlerts` array key)
- Modify: `internal/ppdd/testdata/alerts.json`
- Modify: `internal/ppdd/health_test.go` (severity assertion casing)
- Modify: `internal/ppdd/labels_test.go` (`TestAlertsPaginationCollectsAllPages` inline JSON key)

- [ ] **Step 1: Update the alerts fixture (key + real enum values)**

Replace `internal/ppdd/testdata/alerts.json`:

```json
{
  "alert_list": [
    { "id": "a1", "severity": "WARNING",  "class": "capacity" },
    { "id": "a2", "severity": "CRITICAL", "class": "HardwareFailure" },
    { "id": "a3", "severity": "CRITICAL", "class": "HardwareFailure" }
  ],
  "paging_info": { "current_page": 0, "page_entries": 3, "total_entries": 3, "page_size": 200 }
}
```

- [ ] **Step 2: Update the health test severity assertion**

In `internal/ppdd/health_test.go`, in `TestHealthCollect` change the alert case to the uppercase enum value:

```go
		case s.Name == "ppdd_alerts_active" && s.LabelValue("severity") == "CRITICAL":
			seen["crit"] = s.Value
```

(The `want 2` expectation is unchanged.)

- [ ] **Step 3: Update the pagination test inline JSON key**

In `internal/ppdd/labels_test.go`, in `TestAlertsPaginationCollectsAllPages`, change both `"alert":` keys in the two inline JSON strings to `"alert_list":`. The two `m.SetJSON(...)` payloads become:

```go
	m.SetJSON("/rest/v1.0/dd-systems/0/alerts?page=0&size=200&is_active=true",
		`{"paging_info":{"current_page":0,"page_entries":2,"total_entries":3,"page_size":2},"alert_list":[{"severity":"WARNING","class":"capacity"},{"severity":"WARNING","class":"capacity"}]}`)
	m.SetJSON("/rest/v1.0/dd-systems/0/alerts?page=1&size=200&is_active=true",
		`{"paging_info":{"current_page":1,"page_entries":1,"total_entries":3,"page_size":2},"alert_list":[{"severity":"WARNING","class":"capacity"}]}`)
```

- [ ] **Step 4: Run the tests to verify they fail**

Run: `go test ./internal/ppdd/ -run 'TestHealthCollect|TestAlertsPagination' -v`
Expected: FAIL — alert counts are 0 (struct still decodes `alert`).

- [ ] **Step 5: Fix the alerts array key**

In `internal/ppdd/health.go`, in `healthAlerts`, change the decode struct's JSON tag from `alert` to `alert_list`:

```go
		var r struct {
			Alert []struct {
				Severity string `json:"severity"`
				Class    string `json:"class"`
			} `json:"alert_list"`
			PagingInfo pagingInfo `json:"paging_info"`
		}
```

- [ ] **Step 6: Run the tests to verify they pass**

Run: `go test ./internal/ppdd/ -run 'TestHealthCollect|TestAlertsPagination' -v`
Expected: PASS (critical = 2; pagination total = 3).

- [ ] **Step 7: Commit**

```bash
git add internal/ppdd/health.go internal/ppdd/health_test.go internal/ppdd/labels_test.go internal/ppdd/testdata/alerts.json
git commit -m "fix(health): alerts array key is alert_list; use real 8.7.0 enum casing

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 6: New collector — MTree replication posture

**Files:**
- Modify: `internal/ppdd/endpoints.go` (add `pathMTreeReplication`)
- Create: `internal/ppdd/mtree_replication.go`
- Create: `internal/ppdd/testdata/mtree-replications.json`
- Create: `internal/ppdd/mtree_replication_test.go`

This task adds the collector and its test but does NOT register it yet (Registry swap happens in Task 7 so the old collector stays runnable until then). An unregistered exported type compiles fine in Go.

- [ ] **Step 1: Add the endpoint constant**

In `internal/ppdd/endpoints.go`, add:

```go
	pathMTreeReplication = "/api/v1/dd-systems/0/mtree-replications" // validated 8.7.0: schema MtreeReplicationInfos
```

- [ ] **Step 2: Create the fixture**

Create `internal/ppdd/testdata/mtree-replications.json`:

```json
{
  "contexts": [
    {
      "id": "1",
      "state": "NORMAL",
      "mode": "SOURCE",
      "sourceHost": "dd01",
      "destinationHost": "dd02",
      "enabled": true,
      "needResync": false,
      "connected": true
    }
  ],
  "paging_info": { "current_page": 0, "page_entries": 1, "total_entries": 1, "page_size": 200 }
}
```

- [ ] **Step 3: Write the failing test**

Create `internal/ppdd/mtree_replication_test.go`:

```go
package ppdd

import (
	"context"
	"os"
	"testing"

	"github.com/fjacquet/ppdd_exporter/internal/ddclient"
)

func TestMTreeReplicationCollect(t *testing.T) {
	body, err := os.ReadFile("testdata/mtree-replications.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	m := ddclient.NewMock("dd01")
	m.SetJSON(pathMTreeReplication, string(body))

	got, err := MTreeReplication{}.Collect(context.Background(), m)
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	var stateOK, connOK bool
	for _, s := range got {
		if s.Name == "ppdd_mtree_replication_state" &&
			s.LabelValue("state") == "NORMAL" &&
			s.LabelValue("source") == "dd01" &&
			s.LabelValue("destination") == "dd02" && s.Value == 1 {
			stateOK = true
		}
		if s.Name == "ppdd_mtree_replication_connected" && s.Value == 1 {
			connOK = true
		}
	}
	if !stateOK || !connOK {
		t.Fatalf("stateOK=%v connOK=%v; got %d samples", stateOK, connOK, len(got))
	}
}
```

- [ ] **Step 4: Run the test to verify it fails**

Run: `go test ./internal/ppdd/ -run TestMTreeReplication -v`
Expected: FAIL (compile error: `MTreeReplication` undefined).

- [ ] **Step 5: Write the collector**

Create `internal/ppdd/mtree_replication.go`:

```go
package ppdd

import (
	"context"
	"encoding/json"

	"github.com/fjacquet/ppdd_exporter/internal/ddclient"
)

// MTreeReplication collects per-context MTree replication posture (state, connection,
// resync need). Validated against the 8.7.0 OpenAPI (schema MtreeReplicationInfos).
// Throughput/lag are not on this endpoint; file-replication stats carry those.
type MTreeReplication struct{}

func (MTreeReplication) Name() string { return "mtree_replication" }

func boolGauge(b bool) float64 {
	if b {
		return 1
	}
	return 0
}

func (MTreeReplication) Collect(ctx context.Context, c ddclient.Client) ([]Sample, error) {
	var out []Sample
	err := paginate(ctx, c, pathMTreeReplication, "", func(page json.RawMessage) (pagingInfo, error) {
		var r struct {
			Contexts []struct {
				State           string `json:"state"`
				SourceHost      string `json:"sourceHost"`
				DestinationHost string `json:"destinationHost"`
				Enabled         bool   `json:"enabled"`
				NeedResync      bool   `json:"needResync"`
				Connected       bool   `json:"connected"`
			} `json:"contexts"`
			PagingInfo pagingInfo `json:"paging_info"`
		}
		if err := json.Unmarshal(page, &r); err != nil {
			return pagingInfo{}, err
		}
		for _, ctxn := range r.Contexts {
			id := []Label{{Key: "source", Value: ctxn.SourceHost}, {Key: "destination", Value: ctxn.DestinationHost}}
			stateLbl := append([]Label{{Key: "state", Value: ctxn.State}}, id...)
			out = append(out,
				Sample{Name: "ppdd_mtree_replication_state", Labels: stateLbl, Value: 1},
				Sample{Name: "ppdd_mtree_replication_connected", Labels: id, Value: boolGauge(ctxn.Connected)},
				Sample{Name: "ppdd_mtree_replication_need_resync", Labels: id, Value: boolGauge(ctxn.NeedResync)},
			)
		}
		return r.PagingInfo, nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
```

- [ ] **Step 6: Run the test to verify it passes**

Run: `go test ./internal/ppdd/ -run TestMTreeReplication -v`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/ppdd/endpoints.go internal/ppdd/mtree_replication.go internal/ppdd/mtree_replication_test.go internal/ppdd/testdata/mtree-replications.json
git commit -m "feat(replication): add MTree replication posture collector (8.7.0)

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 7: New collector — file replication stats; retire old collector

**Files:**
- Modify: `internal/ppdd/endpoints.go` (add `pathFileReplication`, remove `pathReplication`)
- Create: `internal/ppdd/file_replication.go`
- Create: `internal/ppdd/testdata/file-replications.json`
- Create: `internal/ppdd/file_replication_test.go`
- Delete: `internal/ppdd/replication.go`, `internal/ppdd/replication_test.go`, `internal/ppdd/testdata/replications.json`
- Modify: `internal/ppdd/resource.go` (Registry)
- Modify: `internal/ppdd/labels_test.go` (`buildFullMock`)

- [ ] **Step 1: Add the endpoint constant and remove the old one**

In `internal/ppdd/endpoints.go`, add:

```go
	pathFileReplication = "/rest/v1.0/dd-systems/0/stats/file-replications" // validated 8.7.0: schema fileReplicationList
```

and delete the `pathReplication = ...` line.

- [ ] **Step 2: Create the fixture**

Create `internal/ppdd/testdata/file-replications.json`:

```json
{
  "context": [
    {
      "id": "ctx1",
      "active_files": 3,
      "logical_replicated": 5000000,
      "network_bytes": 1048576,
      "repl_status": "completed"
    }
  ],
  "paging_info": { "current_page": 0, "page_entries": 1, "total_entries": 1, "page_size": 200 }
}
```

- [ ] **Step 3: Write the failing test**

Create `internal/ppdd/file_replication_test.go`:

```go
package ppdd

import (
	"context"
	"os"
	"testing"

	"github.com/fjacquet/ppdd_exporter/internal/ddclient"
)

func TestFileReplicationCollect(t *testing.T) {
	body, err := os.ReadFile("testdata/file-replications.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	m := ddclient.NewMock("dd01")
	m.SetJSON(pathFileReplication, string(body))

	got, err := FileReplication{}.Collect(context.Background(), m)
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	seen := map[string]float64{}
	var statusOK bool
	for _, s := range got {
		if s.LabelValue("context") == "ctx1" {
			seen[s.Name] = s.Value
		}
		if s.Name == "ppdd_file_replication_status" && s.LabelValue("status") == "completed" && s.Value == 1 {
			statusOK = true
		}
	}
	if seen["ppdd_file_replication_network_bytes"] != 1048576 {
		t.Errorf("network_bytes = %v, want 1048576", seen["ppdd_file_replication_network_bytes"])
	}
	if seen["ppdd_file_replication_active_files"] != 3 {
		t.Errorf("active_files = %v, want 3", seen["ppdd_file_replication_active_files"])
	}
	if !statusOK {
		t.Errorf("expected status sample for completed=1")
	}
}
```

- [ ] **Step 4: Run the test to verify it fails**

Run: `go test ./internal/ppdd/ -run TestFileReplication -v`
Expected: FAIL (compile error: `FileReplication` undefined).

- [ ] **Step 5: Write the collector**

Create `internal/ppdd/file_replication.go`:

```go
package ppdd

import (
	"context"
	"encoding/json"

	"github.com/fjacquet/ppdd_exporter/internal/ddclient"
)

// FileReplication collects per-context file-replication stats. Validated against the
// 8.7.0 OpenAPI (schema fileReplicationList). Label `context` is the opaque context id.
type FileReplication struct{}

func (FileReplication) Name() string { return "file_replication" }

func (FileReplication) Collect(ctx context.Context, c ddclient.Client) ([]Sample, error) {
	var out []Sample
	err := paginate(ctx, c, pathFileReplication, "", func(page json.RawMessage) (pagingInfo, error) {
		var r struct {
			Context []struct {
				ID                string  `json:"id"`
				ActiveFiles       float64 `json:"active_files"`
				LogicalReplicated float64 `json:"logical_replicated"`
				NetworkBytes      float64 `json:"network_bytes"`
				ReplStatus        string  `json:"repl_status"`
			} `json:"context"`
			PagingInfo pagingInfo `json:"paging_info"`
		}
		if err := json.Unmarshal(page, &r); err != nil {
			return pagingInfo{}, err
		}
		for _, fc := range r.Context {
			lbl := []Label{{Key: "context", Value: fc.ID}}
			statusLbl := append([]Label{{Key: "status", Value: fc.ReplStatus}}, lbl...)
			out = append(out,
				Sample{Name: "ppdd_file_replication_network_bytes", Labels: lbl, Value: fc.NetworkBytes},
				Sample{Name: "ppdd_file_replication_logical_replicated_bytes", Labels: lbl, Value: fc.LogicalReplicated},
				Sample{Name: "ppdd_file_replication_active_files", Labels: lbl, Value: fc.ActiveFiles},
				Sample{Name: "ppdd_file_replication_status", Labels: statusLbl, Value: 1},
			)
		}
		return r.PagingInfo, nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
```

- [ ] **Step 6: Run the test to verify it passes**

Run: `go test ./internal/ppdd/ -run TestFileReplication -v`
Expected: PASS.

- [ ] **Step 7: Delete the old replication collector, test, and fixture**

```bash
git rm internal/ppdd/replication.go internal/ppdd/replication_test.go internal/ppdd/testdata/replications.json
```

- [ ] **Step 8: Swap the Registry**

In `internal/ppdd/resource.go`, change `Registry()` to:

```go
	return []ResourceCollector{
		Capacity{},
		MTrees{},
		MTreeReplication{},
		FileReplication{},
		Health{},
	}
```

- [ ] **Step 9: Update `buildFullMock`**

In `internal/ppdd/labels_test.go`, in `buildFullMock`, replace the `pathReplication` map entry (and rename the file-system fixture) so the map reads:

```go
	for path, file := range map[string]string{
		pathSystem:           "testdata/system.json",
		pathFileSystem:       "testdata/file-systems.json",
		pathMTrees:           "testdata/mtrees.json",
		pathMTreeReplication: "testdata/mtree-replications.json",
		pathFileReplication:  "testdata/file-replications.json",
		mtreeStatsPath("%2Fdata%2Fcol1%2Fbackup1"): "testdata/mtree-stats.json",
	} {
```

- [ ] **Step 10: Run the full package test suite**

Run: `go test ./internal/ppdd/ -v`
Expected: PASS (all collectors; label-key consistency holds).

- [ ] **Step 11: Commit**

```bash
git add internal/ppdd/endpoints.go internal/ppdd/file_replication.go internal/ppdd/file_replication_test.go internal/ppdd/testdata/file-replications.json internal/ppdd/resource.go internal/ppdd/labels_test.go
git commit -m "feat(replication): add file-replication stats collector; retire bogus /replications

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 8: Update the mock appliance (`cmd/mockdd`)

The memory note records: endpoint/shape changes must also update `cmd/mockdd` + fixtures or the Compose demo breaks (and `make ci` won't catch it). This task aligns the demo with the corrected paths and shapes.

**Files:**
- Modify: `cmd/mockdd/main.go` (`routes` map)
- Modify/rename: `cmd/mockdd/fixtures/*.json`

- [ ] **Step 1: Update the routes map**

In `cmd/mockdd/main.go`, replace the `routes` map with the corrected paths:

```go
var routes = map[string]string{
	"/rest/v1.0/system":                            "fixtures/system.json",
	"/rest/v1.0/dd-systems/0/file-systems":         "fixtures/file-systems.json",
	"/rest/v3.0/dd-systems/0/mtrees":               "fixtures/mtrees.json",
	"/api/v1/dd-systems/0/mtree-replications":      "fixtures/mtree-replications.json",
	"/rest/v1.0/dd-systems/0/stats/file-replications": "fixtures/file-replications.json",
	"/api/v1/dd-systems/0/storage/disks":           "fixtures/disks.json",
	"/rest/v1.0/dd-systems/0/alerts":               "fixtures/alerts.json",
	"/api/v3/dd-systems/0/stats/performance":       "fixtures/performance.json",
}
```

- [ ] **Step 2: Sync the demo fixtures to the corrected shapes**

Copy the corrected exporter fixtures into the demo and remove the stale ones:

```bash
cp internal/ppdd/testdata/file-systems.json       cmd/mockdd/fixtures/file-systems.json
cp internal/ppdd/testdata/disks.json              cmd/mockdd/fixtures/disks.json
cp internal/ppdd/testdata/performance.json        cmd/mockdd/fixtures/performance.json
cp internal/ppdd/testdata/alerts.json             cmd/mockdd/fixtures/alerts.json
cp internal/ppdd/testdata/mtrees.json             cmd/mockdd/fixtures/mtrees.json
cp internal/ppdd/testdata/mtree-stats.json        cmd/mockdd/fixtures/mtree-stats.json
cp internal/ppdd/testdata/mtree-replications.json cmd/mockdd/fixtures/mtree-replications.json
cp internal/ppdd/testdata/file-replications.json  cmd/mockdd/fixtures/file-replications.json
git rm cmd/mockdd/fixtures/file-system.json cmd/mockdd/fixtures/replications.json cmd/mockdd/fixtures/system-stats.json
```

- [ ] **Step 3: Build the mock to verify it compiles and embeds**

Run: `go build ./cmd/mockdd/`
Expected: builds with no error (the `//go:embed fixtures/*.json` picks up the renamed files).

- [ ] **Step 4: Smoke-test the mock end-to-end (optional but recommended)**

Run the mock and confirm one corrected path serves JSON:

```bash
go run ./cmd/mockdd/ &
sleep 1
TOKEN=$(curl -sk -X POST https://localhost:3009/rest/v1.0/auth -D - -o /dev/null | grep -i x-dd-auth-token | awk '{print $2}' | tr -d '\r')
curl -sk -H "X-DD-AUTH-TOKEN: $TOKEN" https://localhost:3009/api/v1/dd-systems/0/storage/disks
kill %1
```

Expected: the `disks.json` body with `diskInfo`/`status` is returned.

- [ ] **Step 5: Commit**

```bash
git add cmd/mockdd/
git commit -m "chore(mockdd): align demo routes and fixtures with 8.7.0 paths/shapes

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 9: Docs, changelog, and comment cleanup

**Files:**
- Modify: `docs/metrics.md`
- Modify: `CHANGELOG.md`
- Modify: comments in `internal/ppdd/endpoints.go`, `mtrees.go`, `paginate.go` (PROVISIONAL/7.3-guide wording)

- [ ] **Step 1: Update `docs/metrics.md`**

Replace the capacity provisional block (lines ~9-11) so it reads:

```markdown
- `ppdd_filesystem_cleaning_running` (1 while GC runs; from `/file-systems` `fs_clean_status`)
```

Replace the mtrees provisional block (lines ~16-17) so it reads:

```markdown
- `ppdd_mtree_physical_used_bytes` (post-comp written), `ppdd_mtree_quota_soft_limit_bytes` / `ppdd_mtree_quota_hard_limit_bytes` (from `quota_config`)
```

Replace the entire `## replication` section with two sections:

```markdown
## mtree_replication (labels: source, destination; +state on the state metric)
- `ppdd_mtree_replication_state{state}` (1 for the current state; `state` ∈ CONNECTING|UNINITIALIZED|INITIALIZING|NORMAL|RESYNCING|RECOVERING)
- `ppdd_mtree_replication_connected` (1 if connected)
- `ppdd_mtree_replication_need_resync` (1 if a resync is required)

## file_replication (labels: context; +status on the status metric)
- `ppdd_file_replication_network_bytes`
- `ppdd_file_replication_logical_replicated_bytes`
- `ppdd_file_replication_active_files`
- `ppdd_file_replication_status{status}` (1 for the current status; `status` ∈ completed|error|warning|unknown)
```

In the `## health` section, note the alert label casing:

```markdown
- `ppdd_alerts_active{severity, class}` (active alerts only, `is_active=true`; `severity` is the DD enum casing, e.g. `CRITICAL`)
```

- [ ] **Step 2: Add a CHANGELOG entry**

In `CHANGELOG.md`, under the top/unreleased section, add:

```markdown
### Changed
- Validated and corrected all DD API mappings against the PowerProtect DD 8.7.0 OpenAPI
  spec (`docs/swagger/13345-8.7.0.json`):
  - Corrected endpoints: disks → `/api/v1/.../storage/disks`, performance →
    `/api/v3/.../stats/performance`, file-systems path (plural).
  - Fixed fields: alerts array key `alert_list`, mtree quota via `quota_config`,
    disk failed state `FAILED`, mtree compression from the top-level factor.
  - **Breaking:** removed `ppdd_compression_{global,local,total}_factor` (no source on
    8.7.0); replaced the `ppdd_replication_*` metrics with `ppdd_mtree_replication_*`
    (posture) and `ppdd_file_replication_*` (stats). Alert label values now use DD enum
    casing (e.g. `CRITICAL`).
```

- [ ] **Step 3: Clean up provisional comments**

In `internal/ppdd/endpoints.go`, rewrite the file's doc comment block so it no longer claims paths are "modeled from Dell docs only" for the corrected ones. Replace the header comment with:

```go
// DD REST API paths, validated against the PowerProtect DD 8.7.0 OpenAPI spec
// (docs/swagger/13345-8.7.0.json). The /api/ vs /rest/ prefix and the per-resource
// version token are both as the spec defines them. A future version bump re-validates
// against a new spec and edits THIS FILE ONLY.
```

In `internal/ppdd/mtrees.go`, change the `mtreeListItem` comment from "Quota fields are PROVISIONAL..." to:

```go
// mtreeListItem is the validated 8.7.0 v3.0 mtree metadata (schema mtreeInfoDetail_3_0).
```

In `internal/ppdd/paginate.go`, the `pagingInfo` comment "(guide pp.17,20)" → "(schema `paging`, validated 8.7.0)".

- [ ] **Step 4: Build the docs to verify strict mode passes**

Run: `uvx --with mkdocs-material --with pymdown-extensions mkdocs build --strict`
Expected: build succeeds with no warnings.

- [ ] **Step 5: Commit**

```bash
git add docs/metrics.md CHANGELOG.md internal/ppdd/endpoints.go internal/ppdd/mtrees.go internal/ppdd/paginate.go
git commit -m "docs: document 8.7.0 metric corrections; drop PROVISIONAL wording

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 9b: Grafana dashboard — repoint dropped/renamed metrics

The demo dashboard `grafana/dashboards/ppdd-overview.json` references three dropped
compression-split metrics and the three old `ppdd_replication_*` metrics. Panels that
query removed metrics render empty, so they must be repointed to the corrected metric set.

**Files:**
- Modify: `grafana/dashboards/ppdd-overview.json`

Affected panels and the required expr changes (PromQL `expr` strings inside `targets`):

| Panel | Old expr | New expr |
|---|---|---|
| `Total Dedup / Compression Factor` (stat) | `ppdd_compression_total_factor{system=~"$system"}` | `ppdd_compression_factor{system=~"$system"}` |
| `Compression Factors` (timeseries) | 4 targets: `ppdd_compression_global_factor`, `ppdd_compression_local_factor`, `ppdd_compression_total_factor`, `ppdd_compression_factor` | single target: `ppdd_compression_factor{system=~"$system"}` (remove the three dropped series) |
| `Replication Sync Lag` (bargauge) | `ppdd_replication_sync_lag_seconds{system=~"$system"}` | retitle to `File Replication Network Bytes`; expr `ppdd_file_replication_network_bytes{system=~"$system"}` |
| `Replication Backlog (pre-comp remaining)` (stat) | `ppdd_replication_precomp_bytes_remaining{system=~"$system"}` | retitle to `Contexts Needing Resync`; expr `sum(ppdd_mtree_replication_need_resync{system=~"$system"}) or vector(0)` |
| `Contexts Not Normal` (stat) | `count(ppdd_replication_state{system=~"$system", state!="normal"} == 1) or vector(0)` | `count(ppdd_mtree_replication_state{system=~"$system", state!="NORMAL"} == 1) or vector(0)` (metric renamed; state enum is uppercase `NORMAL`) |

- [ ] **Step 1: Edit the dashboard JSON**

Open `grafana/dashboards/ppdd-overview.json` and apply the expr (and where noted, panel `title`) changes from the table above. For the `Compression Factors` timeseries, reduce its `targets` array to a single entry querying `ppdd_compression_factor{system=~"$system"}` (keep that target's `refId`, drop the others). Where a panel is retitled, update its `title` field and, if the bytes unit differs, set the field `unit` appropriately (`decbytes` for the network-bytes bargauge; the resync-count stat has no unit / `short`). Do not change panel `id`s, grid positions, or datasource references.

- [ ] **Step 2: Verify the JSON is valid and references only current metrics**

Run:
```bash
python3 -c "import json; json.load(open('grafana/dashboards/ppdd-overview.json')); print('valid json')"
grep -oE 'ppdd_[a-z_]+' grafana/dashboards/ppdd-overview.json | sort -u
```
Expected: `valid json`, and the metric list contains NONE of: `ppdd_compression_global_factor`, `ppdd_compression_local_factor`, `ppdd_compression_total_factor`, `ppdd_replication_state`, `ppdd_replication_sync_lag_seconds`, `ppdd_replication_precomp_bytes_remaining`. It SHOULD now contain `ppdd_file_replication_network_bytes`, `ppdd_mtree_replication_need_resync`, `ppdd_mtree_replication_state`.

- [ ] **Step 3: Commit**

```bash
git add grafana/dashboards/ppdd-overview.json
git commit -m "fix(grafana): repoint dashboard to corrected 8.7.0 metric set

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 10: Full CI gate

**Files:** none (verification only)

- [ ] **Step 1: Run the full CI gate**

Run: `make ci`
Expected: gofmt clean, vet clean, lint clean, race tests PASS, govulncheck clean, build OK.

- [ ] **Step 2: If lint flags the shared `boolGauge` helper as duplicated or unused**

`boolGauge` is defined once in `mtree_replication.go` and used there. If a future reviewer wants it shared, that's out of scope. No action unless `make ci` fails; if it does, address the specific lint message inline.

- [ ] **Step 3: Final commit (only if Step 2 required changes)**

```bash
git add -A
git commit -m "chore: satisfy CI gate after 8.7.0 mapping corrections

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Self-review notes (already applied)

- **Spec coverage:** every finding in the design (capacity/file-systems, mtree quota+compression, disks, performance, alerts, replication split, mockdd, docs, changelog, comment cleanup) maps to Tasks 1-9; Task 9b repoints the Grafana dashboard to the corrected metric set; Task 10 is the gate.
- **Type consistency:** `MTreeReplication`/`FileReplication` names match between collector, test, and `Registry()`; `pathPerformance`, `pathMTreeReplication`, `pathFileReplication` names are consistent across endpoints.go and all referencing files; fixture filenames (`file-systems.json`, `performance.json`, `mtree-replications.json`, `file-replications.json`) match every loader.
- **Compile order:** unused package-level constants are legal in Go, so adding `pathMTreeReplication` (Task 6) before its Registry use (Task 7) compiles; the old `Replication`/`pathReplication` survive until Task 7 deletes them together with their references.

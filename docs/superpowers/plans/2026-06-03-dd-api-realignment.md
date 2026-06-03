# DD REST API Realignment Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Realign the collector layer to the documented PowerProtect DD 7.3 REST API contract — per-resource endpoint versions, pagination, active alerts, and capacity/MTree rework — without removing any existing metric.

**Architecture:** A single `endpoints.go` centralizes all paths/versions. A `paginate()` helper fixes silent data-loss across list collectors while each collector still decodes its own JSON (provisional risk localized). Documented metrics are added and capacity sources retargeted; undocumented metrics are kept as flagged-provisional best-effort fetches. Fixtures are copied verbatim from the guide's sample responses.

**Tech Stack:** Go 1.26, resty v2 (HTTP), logrus, prometheus/client_golang, standard `testing`. Test gate: `make ci` (gofmt + vet + race + build).

**Spec:** `docs/superpowers/specs/2026-06-03-dd-api-realignment-design.md`

---

## File Structure

**Create:**
- `internal/ppdd/endpoints.go` — all DD resource paths + per-resource versions (single correction point)
- `internal/ppdd/paginate.go` — generic page-following helper + `pagingInfo` envelope
- `internal/ppdd/paginate_test.go` — multi-page helper test
- `internal/ppdd/testdata/system.json` — `/system` sample (capacity + compression)
- `internal/ppdd/testdata/mtree-stats.json` — per-MTree v2.0 stats sample
- `docs/adr/0007-dd-rest-api-realignment.md` — decision record + live-DD validation checklist

**Modify:**
- `internal/ddclient/auth.go` — flat body, `/rest/v1.0/auth`
- `internal/ddclient/system_test.go` — fake server paths + flat-body assertion
- `internal/ddclient/mock.go` — query-stripped path fallback
- `internal/ppdd/capacity.go` + `capacity_test.go` + `testdata/file-system.json`
- `internal/ppdd/mtrees.go` + `mtrees_test.go` + `testdata/mtrees.json`
- `internal/ppdd/replication.go` + `replication_test.go` + `testdata/replications.json`
- `internal/ppdd/health.go` + `health_test.go` + `testdata/alerts.json`
- `internal/ppdd/labels_test.go` — registered mock paths
- `docs/metrics.md`, `CHANGELOG.md`

---

## Task 1: Endpoint registry

**Files:**
- Create: `internal/ppdd/endpoints.go`
- Test: `internal/ppdd/endpoints_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/ppdd/endpoints_test.go
package ppdd

import "testing"

func TestMTreeStatsPath(t *testing.T) {
	got := mtreeStatsPath("%2Fdata%2Fcol1%2Fbackup1")
	want := "/rest/v2.0/dd-systems/0/mtrees/%2Fdata%2Fcol1%2Fbackup1/stats/capacity"
	if got != want {
		t.Fatalf("mtreeStatsPath = %q, want %q", got, want)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/ppdd/ -run TestMTreeStatsPath -v`
Expected: FAIL — `undefined: mtreeStatsPath`

- [ ] **Step 3: Write the registry**

```go
// internal/ppdd/endpoints.go
package ppdd

import "fmt"

// DD REST API paths. The /rest/ prefix is documented as supported across DD OS
// versions; the version token is PER-RESOURCE (NOT a uniform v1) per the
// PowerProtect DD 7.3 REST API guide. A future correction against a live
// appliance edits THIS FILE ONLY.
const (
	pathSystem      = "/rest/v1.0/system"                          // capacity + compression_factor (documented)
	pathAlerts      = "/rest/v1.0/dd-systems/0/alerts"             // documented; query is_active=true
	pathMTrees      = "/rest/v3.0/dd-systems/0/mtrees"             // documented; v3.0 metadata list
	pathReplication = "/rest/v1.0/dd-systems/0/replications"       // PROVISIONAL: not in guide
	pathDisks       = "/rest/v1.0/dd-systems/0/hardware/disks"     // PROVISIONAL: not in guide
	pathSystemStats = "/rest/v1.0/dd-systems/0/stats/system-stats" // PROVISIONAL: not in guide
	pathFileSystem  = "/rest/v1.0/dd-systems/0/file-system"        // PROVISIONAL: cleaning + compression split
)

// mtreeStatsPath returns the per-MTree capacity stats path (v2.0, documented).
// id is the URL-encoded MTree object ID from the v3.0 mtree list.
func mtreeStatsPath(id string) string {
	return fmt.Sprintf("/rest/v2.0/dd-systems/0/mtrees/%s/stats/capacity", id)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/ppdd/ -run TestMTreeStatsPath -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/ppdd/endpoints.go internal/ppdd/endpoints_test.go
git commit -m "feat(ppdd): add central DD endpoint registry

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

## Task 2: Auth alignment (flat body, /rest/v1.0/auth)

**Files:**
- Modify: `internal/ddclient/auth.go`
- Modify: `internal/ddclient/system_test.go:25-50` (fake server handlers + Get path)

- [ ] **Step 1: Update the fake DD server and assertion in the test (failing)**

In `internal/ddclient/system_test.go`, replace the `newFakeDD` mux registrations and the body assertion. Change the auth handler path to `/rest/v1.0/auth`, assert a flat body, and serve `/rest/v1.0/system`:

```go
func newFakeDD(t *testing.T, logins *int32) *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/rest/v1.0/auth", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			atomic.AddInt32(logins, 1)
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if _, ok := body["username"]; !ok { // flat shape, NOT {auth_info:{...}}
				t.Errorf("auth body missing flat username field: %v", body)
			}
			w.Header().Set("X-DD-AUTH-TOKEN", authToken)
			w.WriteHeader(http.StatusCreated)
		case http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		}
	})
	mux.HandleFunc("/rest/v1.0/system", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-DD-AUTH-TOKEN") != authToken {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		writeBytes(w, []byte(`{"physical_used":99}`))
	})
	return httptest.NewTLSServer(mux)
}
```

Add `"encoding/json"` to the test imports. Then update the two `c.Get(...)` calls in `TestSystemClientAuthAndGet` from `/api/v1/dd-systems/0/file-system` to `/rest/v1.0/system`.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/ddclient/ -run TestSystemClientAuthAndGet -v`
Expected: FAIL — auth still posts to `/api/v1/auth` (404 → no token) and/or wrapped body assertion fails.

- [ ] **Step 3: Rewrite auth.go**

```go
// internal/ddclient/auth.go
package ddclient

import (
	"context"
	"fmt"
)

// authPath is the documented DD login/logout endpoint (7.3 REST API guide).
const authPath = "/rest/v1.0/auth"

// authRequest is the DD login body. Per the 7.3 guide this is a FLAT
// {username,password} object posted to /rest/v1.0/auth.
//
// HIGHEST-RISK MAPPING: if logins fail against a live appliance, revert HERE
// first. The prior guess was {"auth_info":{...}} posted to /api/v1/auth.
type authRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// ensureToken logs in if no token is cached, capturing X-DD-AUTH-TOKEN.
func (c *SystemClient) ensureToken(ctx context.Context) error {
	if c.currentToken() != "" {
		return nil
	}
	body := authRequest{Username: c.cfg.Username, Password: c.cfg.Password}

	resp, err := c.rc.R().SetContext(ctx).SetBody(body).Post(authPath)
	if err != nil {
		return fmt.Errorf("auth POST: %w", err)
	}
	if resp.StatusCode() >= 300 {
		return fmt.Errorf("auth POST: status %d", resp.StatusCode())
	}
	tok := resp.Header().Get("X-DD-AUTH-TOKEN")
	if tok == "" {
		return fmt.Errorf("auth POST: no X-DD-AUTH-TOKEN in response")
	}
	c.mu.Lock()
	c.token = tok
	c.mu.Unlock()
	return nil
}

// Close logs out (best effort) and is safe to call with no active session.
func (c *SystemClient) Close() error {
	if c.currentToken() == "" {
		return nil
	}
	_, _ = c.rc.R().SetHeader("X-DD-AUTH-TOKEN", c.currentToken()).Delete(authPath)
	c.clearToken()
	return nil
}
```

- [ ] **Step 4: Check the 401-relogin test still points at a served path**

Open `internal/ddclient/system_test.go` and read `TestSystemClientReloginOn401`. If its mux registers an auth handler at `/api/v1/auth` or a data path under `/api/v1/...`, update those literals to `/rest/v1.0/auth` and a `/rest/v1.0/system` data path so the test exercises the new endpoints. (The SystemClient itself is path-agnostic for `Get`; only auth is hardcoded.)

- [ ] **Step 5: Run the package tests**

Run: `go test ./internal/ddclient/ -v`
Expected: PASS (all tests, including relogin)

- [ ] **Step 6: Commit**

```bash
git add internal/ddclient/auth.go internal/ddclient/system_test.go
git commit -m "fix(ddclient): align auth to documented flat body at /rest/v1.0/auth

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

## Task 3: Mock query-stripped fallback

**Files:**
- Modify: `internal/ddclient/mock.go`
- Test: `internal/ddclient/mock_test.go` (add one test)

- [ ] **Step 1: Write the failing test**

Append to `internal/ddclient/mock_test.go`:

```go
func TestMockClientFallsBackToPathWithoutQuery(t *testing.T) {
	m := NewMock("dd01")
	m.SetJSON("/rest/v1.0/dd-systems/0/alerts", `{"alert":[{"severity":"warning"}]}`)

	var out struct {
		Alert []struct {
			Severity string `json:"severity"`
		} `json:"alert"`
	}
	// Caller appends ?page=&size=; the clean path is what was registered.
	if err := m.Get(context.Background(), "/rest/v1.0/dd-systems/0/alerts?page=0&size=200", &out); err != nil {
		t.Fatalf("Get with query should fall back to clean path: %v", err)
	}
	if len(out.Alert) != 1 || out.Alert[0].Severity != "warning" {
		t.Fatalf("unexpected decode: %+v", out)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/ddclient/ -run TestMockClientFallsBackToPathWithoutQuery -v`
Expected: FAIL — `mock: no response registered for /rest/...alerts?page=0&size=200`

- [ ] **Step 3: Add the fallback to mock.go**

Replace the `Get` method in `internal/ddclient/mock.go` and add the `strings` import:

```go
import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

func (m *Mock) Get(_ context.Context, path string, out any) error {
	body, ok := m.paths[path]
	if !ok {
		// Fall back to a query-stripped match so collectors that append
		// ?page=&size= resolve against a cleanly registered path.
		if i := strings.IndexByte(path, '?'); i >= 0 {
			body, ok = m.paths[path[:i]]
		}
	}
	if !ok {
		return fmt.Errorf("mock: no response registered for %s", path)
	}
	return json.Unmarshal([]byte(body), out)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/ddclient/ -v`
Expected: PASS (existing exact-match tests still pass; new fallback test passes)

- [ ] **Step 5: Commit**

```bash
git add internal/ddclient/mock.go internal/ddclient/mock_test.go
git commit -m "test(ddclient): mock falls back to path without query string

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

## Task 4: Pagination helper

**Files:**
- Create: `internal/ppdd/paginate.go`
- Test: `internal/ppdd/paginate_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/ppdd/paginate_test.go
package ppdd

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/fjacquet/ppdd_exporter/internal/ddclient"
)

func TestPaginateFollowsAllPages(t *testing.T) {
	m := ddclient.NewMock("dd01")
	// Server reports page_size 2, total 3 → two pages.
	m.SetJSON("/things?page=0&size=200",
		`{"paging_info":{"current_page":0,"page_entries":2,"total_entries":3,"page_size":2},"thing":[{"v":1},{"v":2}]}`)
	m.SetJSON("/things?page=1&size=200",
		`{"paging_info":{"current_page":1,"page_entries":1,"total_entries":3,"page_size":2},"thing":[{"v":3}]}`)

	var vals []int
	err := paginate(context.Background(), m, "/things", "", func(page json.RawMessage) (pagingInfo, error) {
		var r struct {
			Thing      []struct{ V int `json:"v"` } `json:"thing"`
			PagingInfo pagingInfo                    `json:"paging_info"`
		}
		if err := json.Unmarshal(page, &r); err != nil {
			return pagingInfo{}, err
		}
		for _, t := range r.Thing {
			vals = append(vals, t.V)
		}
		return r.PagingInfo, nil
	})
	if err != nil {
		t.Fatalf("paginate: %v", err)
	}
	if len(vals) != 3 {
		t.Fatalf("collected %v, want 3 items across 2 pages", vals)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/ppdd/ -run TestPaginateFollowsAllPages -v`
Expected: FAIL — `undefined: paginate` / `undefined: pagingInfo`

- [ ] **Step 3: Write paginate.go**

```go
// internal/ppdd/paginate.go
package ppdd

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/fjacquet/ppdd_exporter/internal/ddclient"
	log "github.com/sirupsen/logrus"
)

const (
	pageSize = 200 // documented max page size
	maxPages = 100 // safety cap to bound a runaway list
)

// pagingInfo is the DD list-response envelope (guide pp.17,20).
type pagingInfo struct {
	CurrentPage  int `json:"current_page"`
	PageEntries  int `json:"page_entries"`
	TotalEntries int `json:"total_entries"`
	PageSize     int `json:"page_size"`
}

// paginate GETs basePath across all pages, handing each page's raw JSON to onPage.
// onPage decodes its own named array and returns that page's paging_info so the
// loop knows when to stop. extraQuery carries collector params (no leading &).
func paginate(ctx context.Context, c ddclient.Client, basePath, extraQuery string,
	onPage func(page json.RawMessage) (pagingInfo, error)) error {
	for p := 0; p < maxPages; p++ {
		path := fmt.Sprintf("%s?page=%d&size=%d", basePath, p, pageSize)
		if extraQuery != "" {
			path += "&" + extraQuery
		}
		var raw json.RawMessage
		if err := c.Get(ctx, path, &raw); err != nil {
			return err
		}
		pi, err := onPage(raw)
		if err != nil {
			return err
		}
		// Stop when the server reports no usable paging, or all entries covered.
		if pi.PageSize <= 0 || pi.PageEntries == 0 ||
			(pi.CurrentPage+1)*pi.PageSize >= pi.TotalEntries {
			return nil
		}
	}
	log.WithField("path", basePath).Warn("pagination hit maxPages cap; list may be truncated")
	return nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/ppdd/ -run TestPaginateFollowsAllPages -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/ppdd/paginate.go internal/ppdd/paginate_test.go
git commit -m "feat(ppdd): add paginate helper for DD list endpoints

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

## Task 5: Active alerts with class label

**Files:**
- Modify: `internal/ppdd/health.go` (replace `healthAlerts`, repoint `healthDisks`/`healthSystemStats`)
- Modify: `internal/ppdd/testdata/alerts.json`
- Modify: `internal/ppdd/health_test.go` (`loadHealthMock` paths)

- [ ] **Step 1: Update the alerts fixture**

Replace `internal/ppdd/testdata/alerts.json` with active-shape data carrying `class`:

```json
{
  "alert": [
    { "id": "a1", "severity": "warning",  "class": "capacity" },
    { "id": "a2", "severity": "critical", "class": "hardware" },
    { "id": "a3", "severity": "critical", "class": "hardware" }
  ],
  "paging_info": { "current_page": 0, "page_entries": 3, "total_entries": 3, "page_size": 200 }
}
```

- [ ] **Step 2: Update loadHealthMock to documented paths (test edit)**

In `internal/ppdd/health_test.go`, change the registered paths in `loadHealthMock` to the registry constants (same package, so the consts are visible):

```go
func loadHealthMock(t *testing.T) *ddclient.Mock {
	t.Helper()
	m := ddclient.NewMock("dd01")
	for path, file := range map[string]string{
		pathDisks:       "testdata/disks.json",
		pathAlerts:      "testdata/alerts.json",
		pathSystemStats: "testdata/system-stats.json",
	} {
		b, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		m.SetJSON(path, string(b))
	}
	return m
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/ppdd/ -run TestHealthCollect -v`
Expected: FAIL — `healthAlerts` still GETs `/api/v1/...` and ignores paging; critical count assertion may still pass by luck on disks/cpu but alerts path now unregistered → 0 criticals. Confirm FAIL on `critical alerts = 0, want 2`.

- [ ] **Step 4: Rewrite the three health sub-fetches**

In `internal/ppdd/health.go`, add the `encoding/json` import, repoint disks/system-stats to the registry constants, and replace `healthAlerts`:

```go
import (
	"context"
	"encoding/json"

	"github.com/fjacquet/ppdd_exporter/internal/ddclient"
)

func healthDisks(ctx context.Context, c ddclient.Client) []Sample {
	var r struct {
		Disk []struct {
			ID    string `json:"id"`
			State string `json:"state"`
		} `json:"disk"`
	}
	if err := c.Get(ctx, pathDisks, &r); err != nil {
		return nil
	}
	var out []Sample
	for _, d := range r.Disk {
		failed := 0.0
		if d.State == "failed" {
			failed = 1
		}
		out = append(out, Sample{Name: "ppdd_disk_failed", Labels: []Label{{Key: "disk", Value: d.ID}}, Value: failed})
	}
	return out
}

func healthAlerts(ctx context.Context, c ddclient.Client) []Sample {
	type alertKey struct{ severity, class string }
	counts := map[alertKey]float64{}
	err := paginate(ctx, c, pathAlerts, "is_active=true", func(page json.RawMessage) (pagingInfo, error) {
		var r struct {
			Alert []struct {
				Severity string `json:"severity"`
				Class    string `json:"class"`
			} `json:"alert"`
			PagingInfo pagingInfo `json:"paging_info"`
		}
		if err := json.Unmarshal(page, &r); err != nil {
			return pagingInfo{}, err
		}
		for _, a := range r.Alert {
			counts[alertKey{a.Severity, a.Class}]++
		}
		return r.PagingInfo, nil
	})
	if err != nil {
		return nil
	}
	var out []Sample
	for k, n := range counts {
		out = append(out, Sample{
			Name:   "ppdd_alerts_active",
			Labels: []Label{{Key: "severity", Value: k.severity}, {Key: "class", Value: k.class}},
			Value:  n,
		})
	}
	return out
}

func healthSystemStats(ctx context.Context, c ddclient.Client) []Sample {
	var r struct {
		CPUAvgPercent    float64 `json:"cpu_avg_percent"`
		ReadBytesPerSec  float64 `json:"read_bytes_per_second"`
		WriteBytesPerSec float64 `json:"write_bytes_per_second"`
	}
	if err := c.Get(ctx, pathSystemStats, &r); err != nil {
		return nil
	}
	return []Sample{
		{Name: "ppdd_system_cpu_percent", Value: r.CPUAvgPercent},
		{Name: "ppdd_system_read_bytes_per_second", Value: r.ReadBytesPerSec},
		{Name: "ppdd_system_write_bytes_per_second", Value: r.WriteBytesPerSec},
	}
}
```

The two `critical`/`hardware` alerts now sum into one `{severity:critical, class:hardware}` sample with value 2, so the existing `TestHealthCollect` assertion (`crit == 2`, matched by `s.LabelValue("severity") == "critical"`) still holds.

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/ppdd/ -run 'TestHealth' -v`
Expected: PASS — both `TestHealthCollect` and `TestHealthDegradesPerEndpoint`

- [ ] **Step 6: Commit**

```bash
git add internal/ppdd/health.go internal/ppdd/health_test.go internal/ppdd/testdata/alerts.json
git commit -m "feat(ppdd): collect active alerts with class label; paginate alerts

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

## Task 6: Capacity rework (/system + provisional /file-system)

**Files:**
- Modify: `internal/ppdd/capacity.go`
- Create: `internal/ppdd/testdata/system.json`
- Modify: `internal/ppdd/testdata/file-system.json` (keep only provisional fields)
- Modify: `internal/ppdd/capacity_test.go`

- [ ] **Step 1: Create the documented /system fixture**

`internal/ppdd/testdata/system.json` (subset of guide pp.11–12 we consume):

```json
{
  "physical_capacity": { "available": 60000000000, "total": 100000000000, "used": 40000000000 },
  "logical_capacity": { "available": 130530410496, "total": 130530410496, "used": 0 },
  "compression_factor": 9.9,
  "uptime_secs": 256674,
  "version": "Data Domain OS 7.3.0.0",
  "model": "DD VE Version 5.0",
  "serialno": "AUDV3ZU7Z6SB7R"
}
```

- [ ] **Step 2: Trim the provisional file-system fixture**

`internal/ppdd/testdata/file-system.json` keeps only the provisional fields:

```json
{
  "global_compression_factor": 5.5,
  "local_compression_factor": 1.8,
  "total_compression_factor": 9.9,
  "cleaning": { "status": "running" }
}
```

- [ ] **Step 3: Update the capacity test (failing)**

Replace `internal/ppdd/capacity_test.go`:

```go
package ppdd

import (
	"context"
	"os"
	"testing"

	"github.com/fjacquet/ppdd_exporter/internal/ddclient"
)

func TestCapacityCollect(t *testing.T) {
	m := ddclient.NewMock("dd01")
	for path, file := range map[string]string{
		pathSystem:     "testdata/system.json",
		pathFileSystem: "testdata/file-system.json",
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
		"ppdd_compression_global_factor":   5.5,
		"ppdd_compression_local_factor":    1.8,
		"ppdd_compression_total_factor":    9.9,
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
}

func TestCapacityProvisionalIsBestEffort(t *testing.T) {
	m := ddclient.NewMock("dd01")
	b, err := os.ReadFile("testdata/system.json")
	if err != nil {
		t.Fatal(err)
	}
	m.SetJSON(pathSystem, string(b)) // no file-system registered

	got, err := Capacity{}.Collect(context.Background(), m)
	if err != nil {
		t.Fatalf("Collect must not fail when provisional /file-system is absent: %v", err)
	}
	var hasDocumented bool
	for _, s := range got {
		if s.Name == "ppdd_filesystem_total_bytes" && s.Value == 100000000000 {
			hasDocumented = true
		}
	}
	if !hasDocumented {
		t.Fatal("documented capacity samples missing when only /system is available")
	}
}
```

- [ ] **Step 4: Run test to verify it fails**

Run: `go test ./internal/ppdd/ -run TestCapacity -v`
Expected: FAIL — `Capacity.Collect` still GETs `/api/v1/.../file-system` and has no `ppdd_compression_factor`.

- [ ] **Step 5: Rewrite capacity.go**

```go
// internal/ppdd/capacity.go
package ppdd

import (
	"context"

	"github.com/fjacquet/ppdd_exporter/internal/ddclient"
)

// systemResp is the documented shape of GET /rest/v1.0/system (guide pp.11-12).
type systemResp struct {
	PhysicalCapacity struct {
		Total     float64 `json:"total"`
		Used      float64 `json:"used"`
		Available float64 `json:"available"`
	} `json:"physical_capacity"`
	CompressionFactor float64 `json:"compression_factor"`
}

// fileSystemResp is PROVISIONAL (not in the 7.3 guide). Retained only for the
// compression split and GC cleaning state, fetched best-effort.
type fileSystemResp struct {
	GlobalCompression float64 `json:"global_compression_factor"`
	LocalCompression  float64 `json:"local_compression_factor"`
	TotalCompression  float64 `json:"total_compression_factor"`
	Cleaning          struct {
		Status string `json:"status"`
	} `json:"cleaning"`
}

// Capacity collects filesystem capacity and compression. Documented values come
// from /system; the compression split and GC state are provisional best-effort.
type Capacity struct{}

func (Capacity) Name() string { return "capacity" }

func (Capacity) Collect(ctx context.Context, c ddclient.Client) ([]Sample, error) {
	var sys systemResp
	if err := c.Get(ctx, pathSystem, &sys); err != nil {
		return nil, err
	}
	out := []Sample{
		{Name: "ppdd_filesystem_total_bytes", Value: sys.PhysicalCapacity.Total},
		{Name: "ppdd_filesystem_used_bytes", Value: sys.PhysicalCapacity.Used},
		{Name: "ppdd_filesystem_available_bytes", Value: sys.PhysicalCapacity.Available},
		{Name: "ppdd_compression_factor", Value: sys.CompressionFactor},
	}
	return append(out, capacityProvisional(ctx, c)...), nil
}

// capacityProvisional fetches the undocumented /file-system extras (best-effort:
// a failure drops only these samples, never the whole collector).
func capacityProvisional(ctx context.Context, c ddclient.Client) []Sample {
	var fs fileSystemResp
	if err := c.Get(ctx, pathFileSystem, &fs); err != nil {
		return nil
	}
	cleaning := 0.0
	if fs.Cleaning.Status == "running" {
		cleaning = 1
	}
	return []Sample{
		{Name: "ppdd_compression_global_factor", Value: fs.GlobalCompression},
		{Name: "ppdd_compression_local_factor", Value: fs.LocalCompression},
		{Name: "ppdd_compression_total_factor", Value: fs.TotalCompression},
		{Name: "ppdd_filesystem_cleaning_running", Value: cleaning},
	}
}
```

- [ ] **Step 6: Run test to verify it passes**

Run: `go test ./internal/ppdd/ -run TestCapacity -v`
Expected: PASS (both capacity tests)

- [ ] **Step 7: Commit**

```bash
git add internal/ppdd/capacity.go internal/ppdd/capacity_test.go internal/ppdd/testdata/system.json internal/ppdd/testdata/file-system.json
git commit -m "feat(ppdd): source capacity from /system; add ppdd_compression_factor

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

## Task 7: MTree rework (v3.0 list + per-MTree v2.0 stats)

**Files:**
- Modify: `internal/ppdd/mtrees.go`
- Modify: `internal/ppdd/testdata/mtrees.json`
- Create: `internal/ppdd/testdata/mtree-stats.json`
- Modify: `internal/ppdd/mtrees_test.go`

- [ ] **Step 1: Rewrite the mtrees fixture (v3.0 metadata)**

`internal/ppdd/testdata/mtrees.json`:

```json
{
  "mtree": [
    {
      "id": "%2Fdata%2Fcol1%2Fbackup1",
      "name": "/data/col1/backup1",
      "is_degraded": "degraded",
      "mtree_rl_detail": { "rl_status": "enabled" },
      "quota_soft_limit_bytes": 10000000000,
      "quota_hard_limit_bytes": 12000000000
    }
  ],
  "paging_info": { "current_page": 0, "page_entries": 1, "total_entries": 1, "page_size": 200 }
}
```

- [ ] **Step 2: Create the per-MTree stats fixture**

`internal/ppdd/testdata/mtree-stats.json` (guide pp.34–36, two epochs; latest wins):

```json
{
  "stats_capacity": [
    {
      "collection_epoch": 1589223600,
      "tier_capacity_usage": [ { "tier": "active", "logical_capacity": { "used": 1 } } ],
      "tier_data_written": [ { "tier": "active", "compression_factor": 1.0, "post_comp_written": 1 } ]
    },
    {
      "collection_epoch": 1589310000,
      "tier_capacity_usage": [ { "tier": "active", "logical_capacity": { "used": 5000000000 } } ],
      "tier_data_written": [ { "tier": "active", "compression_factor": 6.25, "post_comp_written": 800000000 } ]
    }
  ]
}
```

- [ ] **Step 3: Update the mtrees test (failing)**

Replace `internal/ppdd/mtrees_test.go`:

```go
package ppdd

import (
	"context"
	"os"
	"testing"

	"github.com/fjacquet/ppdd_exporter/internal/ddclient"
)

func TestMTreesCollect(t *testing.T) {
	m := ddclient.NewMock("dd01")
	list, err := os.ReadFile("testdata/mtrees.json")
	if err != nil {
		t.Fatalf("read list fixture: %v", err)
	}
	m.SetJSON(pathMTrees, string(list))
	stats, err := os.ReadFile("testdata/mtree-stats.json")
	if err != nil {
		t.Fatalf("read stats fixture: %v", err)
	}
	m.SetJSON(mtreeStatsPath("%2Fdata%2Fcol1%2Fbackup1"), string(stats))

	got, err := MTrees{}.Collect(context.Background(), m)
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	seen := map[string]float64{}
	for _, s := range got {
		if s.LabelValue("mtree") == "/data/col1/backup1" {
			seen[s.Name] = s.Value
		}
	}
	checks := map[string]float64{
		"ppdd_mtree_logical_used_bytes":     5000000000, // latest epoch
		"ppdd_mtree_compression_factor":     6.25,
		"ppdd_mtree_physical_used_bytes":    800000000, // provisional: post_comp_written
		"ppdd_mtree_degraded":               1,
		"ppdd_mtree_retention_lock_enabled": 1,
		"ppdd_mtree_quota_soft_limit_bytes": 10000000000,
	}
	for name, want := range checks {
		if seen[name] != want {
			t.Errorf("%s = %v, want %v", name, seen[name], want)
		}
	}
}

func TestMTreesUsageIsBestEffort(t *testing.T) {
	m := ddclient.NewMock("dd01")
	list, err := os.ReadFile("testdata/mtrees.json")
	if err != nil {
		t.Fatal(err)
	}
	m.SetJSON(pathMTrees, string(list)) // no per-MTree stats registered

	got, err := MTrees{}.Collect(context.Background(), m)
	if err != nil {
		t.Fatalf("Collect must not fail when per-MTree stats are absent: %v", err)
	}
	var hasMeta bool
	for _, s := range got {
		if s.Name == "ppdd_mtree_degraded" && s.LabelValue("mtree") == "/data/col1/backup1" {
			hasMeta = true
		}
	}
	if !hasMeta {
		t.Fatal("metadata samples should survive when usage stats are unavailable")
	}
}
```

- [ ] **Step 4: Run test to verify it fails**

Run: `go test ./internal/ppdd/ -run TestMTrees -v`
Expected: FAIL — old `MTrees.Collect` reads usage from the list and lacks degraded/retention metrics.

- [ ] **Step 5: Rewrite mtrees.go**

```go
// internal/ppdd/mtrees.go
package ppdd

import (
	"context"
	"encoding/json"

	"github.com/fjacquet/ppdd_exporter/internal/ddclient"
)

// mtreeListItem is the documented v3.0 mtree metadata. Quota fields are
// PROVISIONAL (not shown in the guide's mtree sample) and kept best-effort.
type mtreeListItem struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	IsDegraded    string `json:"is_degraded"`
	MTreeRLDetail struct {
		RLStatus string `json:"rl_status"`
	} `json:"mtree_rl_detail"`
	QuotaSoftLimit float64 `json:"quota_soft_limit_bytes"` // PROVISIONAL
	QuotaHardLimit float64 `json:"quota_hard_limit_bytes"` // PROVISIONAL
}

// mtreeStatsResp is the documented v2.0 per-MTree capacity stats (guide pp.34-36).
type mtreeStatsResp struct {
	StatsCapacity []struct {
		CollectionEpoch   int64 `json:"collection_epoch"`
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
}

// MTrees collects per-MTree metadata/health (v3.0 list) and usage (v2.0 stats).
type MTrees struct{}

func (MTrees) Name() string { return "mtrees" }

func (MTrees) Collect(ctx context.Context, c ddclient.Client) ([]Sample, error) {
	var items []mtreeListItem
	err := paginate(ctx, c, pathMTrees, "", func(page json.RawMessage) (pagingInfo, error) {
		var r struct {
			MTree      []mtreeListItem `json:"mtree"`
			PagingInfo pagingInfo      `json:"paging_info"`
		}
		if err := json.Unmarshal(page, &r); err != nil {
			return pagingInfo{}, err
		}
		items = append(items, r.MTree...)
		return r.PagingInfo, nil
	})
	if err != nil {
		return nil, err
	}

	var out []Sample
	for _, mt := range items {
		lbl := []Label{{Key: "mtree", Value: mt.Name}}
		degraded := 0.0
		if mt.IsDegraded == "degraded" {
			degraded = 1
		}
		rl := 0.0
		switch mt.MTreeRLDetail.RLStatus {
		case "", "never-enabled", "disabled":
			// retention lock not active
		default:
			rl = 1
		}
		out = append(out,
			Sample{Name: "ppdd_mtree_degraded", Labels: lbl, Value: degraded},
			Sample{Name: "ppdd_mtree_retention_lock_enabled", Labels: lbl, Value: rl},
			Sample{Name: "ppdd_mtree_quota_soft_limit_bytes", Labels: lbl, Value: mt.QuotaSoftLimit},
			Sample{Name: "ppdd_mtree_quota_hard_limit_bytes", Labels: lbl, Value: mt.QuotaHardLimit},
		)
		out = append(out, mtreeUsage(ctx, c, mt)...)
	}
	return out, nil
}

// mtreeUsage fetches documented per-MTree usage from the latest collection_epoch
// (best-effort: a failure drops only this MTree's usage samples). N+1 requests —
// one per MTree; bounded concurrency is a future optimization.
func mtreeUsage(ctx context.Context, c ddclient.Client, mt mtreeListItem) []Sample {
	var r mtreeStatsResp
	if err := c.Get(ctx, mtreeStatsPath(mt.ID), &r); err != nil || len(r.StatsCapacity) == 0 {
		return nil
	}
	latest := r.StatsCapacity[0]
	for _, s := range r.StatsCapacity[1:] {
		if s.CollectionEpoch > latest.CollectionEpoch {
			latest = s
		}
	}
	var logicalUsed, comp, postComp float64
	for _, t := range latest.TierCapacityUsage {
		logicalUsed += t.LogicalCapacity.Used
	}
	if len(latest.TierDataWritten) > 0 {
		comp = latest.TierDataWritten[0].CompressionFactor
		for _, t := range latest.TierDataWritten {
			postComp += t.PostCompWritten
		}
	}
	lbl := []Label{{Key: "mtree", Value: mt.Name}}
	return []Sample{
		{Name: "ppdd_mtree_logical_used_bytes", Labels: lbl, Value: logicalUsed},
		{Name: "ppdd_mtree_compression_factor", Labels: lbl, Value: comp},
		{Name: "ppdd_mtree_physical_used_bytes", Labels: lbl, Value: postComp}, // PROVISIONAL: post_comp_written
	}
}
```

- [ ] **Step 6: Run test to verify it passes**

Run: `go test ./internal/ppdd/ -run TestMTrees -v`
Expected: PASS (both mtree tests)

- [ ] **Step 7: Commit**

```bash
git add internal/ppdd/mtrees.go internal/ppdd/mtrees_test.go internal/ppdd/testdata/mtrees.json internal/ppdd/testdata/mtree-stats.json
git commit -m "feat(ppdd): rework MTrees to v3.0 list + per-MTree v2.0 stats

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

## Task 8: Replication path move + pagination

**Files:**
- Modify: `internal/ppdd/replication.go`
- Modify: `internal/ppdd/testdata/replications.json`
- Modify: `internal/ppdd/replication_test.go`

- [ ] **Step 1: Add paging_info to the replications fixture**

`internal/ppdd/testdata/replications.json`:

```json
{
  "replication": [
    {
      "source": "dd01:/data/col1/backup1",
      "destination": "dd02:/data/col1/backup1",
      "state": "normal",
      "sync_lag_seconds": 120,
      "precomp_bytes_remaining": 2048,
      "throughput_bytes_per_second": 1048576
    }
  ],
  "paging_info": { "current_page": 0, "page_entries": 1, "total_entries": 1, "page_size": 200 }
}
```

- [ ] **Step 2: Update the replication test (failing)**

In `internal/ppdd/replication_test.go`, change the registered path to the registry constant:

```go
	m := ddclient.NewMock("dd01")
	m.SetJSON(pathReplication, string(body))
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/ppdd/ -run TestReplicationCollect -v`
Expected: FAIL — collector still GETs `/api/v1/.../replications` (unregistered) → no samples.

- [ ] **Step 4: Rewrite replication.go to paginate via the registry path**

```go
// internal/ppdd/replication.go
package ppdd

import (
	"context"
	"encoding/json"

	"github.com/fjacquet/ppdd_exporter/internal/ddclient"
)

// Replication collects per-context DR posture. PROVISIONAL: this endpoint and its
// fields are not in the 7.3 guide; only the prefix/version and pagination are aligned.
type Replication struct{}

func (Replication) Name() string { return "replication" }

func (Replication) Collect(ctx context.Context, c ddclient.Client) ([]Sample, error) {
	var out []Sample
	err := paginate(ctx, c, pathReplication, "", func(page json.RawMessage) (pagingInfo, error) {
		var r struct {
			Replication []struct {
				Source            string  `json:"source"`
				Destination       string  `json:"destination"`
				State             string  `json:"state"`
				SyncLagSeconds    float64 `json:"sync_lag_seconds"`
				PrecompRemaining  float64 `json:"precomp_bytes_remaining"`
				ThroughputBytesPS float64 `json:"throughput_bytes_per_second"`
			} `json:"replication"`
			PagingInfo pagingInfo `json:"paging_info"`
		}
		if err := json.Unmarshal(page, &r); err != nil {
			return pagingInfo{}, err
		}
		for _, ctxn := range r.Replication {
			id := []Label{{Key: "source", Value: ctxn.Source}, {Key: "destination", Value: ctxn.Destination}}
			stateLbl := append([]Label{{Key: "state", Value: ctxn.State}}, id...)
			out = append(out,
				Sample{Name: "ppdd_replication_state", Labels: stateLbl, Value: 1},
				Sample{Name: "ppdd_replication_sync_lag_seconds", Labels: id, Value: ctxn.SyncLagSeconds},
				Sample{Name: "ppdd_replication_precomp_bytes_remaining", Labels: id, Value: ctxn.PrecompRemaining},
				Sample{Name: "ppdd_replication_throughput_bytes_per_second", Labels: id, Value: ctxn.ThroughputBytesPS},
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

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/ppdd/ -run TestReplicationCollect -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/ppdd/replication.go internal/ppdd/replication_test.go internal/ppdd/testdata/replications.json
git commit -m "refactor(ppdd): move replication to /rest prefix and paginate

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

## Task 9: Update cross-collector label test + add a true multi-page collector test

**Files:**
- Modify: `internal/ppdd/labels_test.go`

- [ ] **Step 1: Repoint buildFullMock to registry paths and add the per-MTree stats path**

Replace `buildFullMock` in `internal/ppdd/labels_test.go`:

```go
func buildFullMock(t *testing.T) *ddclient.Mock {
	t.Helper()
	m := loadHealthMock(t)
	for path, file := range map[string]string{
		pathSystem:     "testdata/system.json",
		pathFileSystem: "testdata/file-system.json",
		pathMTrees:     "testdata/mtrees.json",
		pathReplication: "testdata/replications.json",
		mtreeStatsPath("%2Fdata%2Fcol1%2Fbackup1"): "testdata/mtree-stats.json",
	} {
		b, err := readFixture(file)
		if err != nil {
			t.Fatal(err)
		}
		m.SetJSON(path, b)
	}
	return m
}
```

- [ ] **Step 2: Add a multi-page alerts test proving items past page 0 are collected**

Append to `internal/ppdd/labels_test.go` (or a new `health_pagination_test.go`):

```go
func TestAlertsPaginationCollectsAllPages(t *testing.T) {
	m := ddclient.NewMock("dd01")
	// page_size 2, total 3 → two pages. is_active=true is appended by the collector.
	m.SetJSON("/rest/v1.0/dd-systems/0/alerts?page=0&size=200&is_active=true",
		`{"paging_info":{"current_page":0,"page_entries":2,"total_entries":3,"page_size":2},"alert":[{"severity":"warning","class":"capacity"},{"severity":"warning","class":"capacity"}]}`)
	m.SetJSON("/rest/v1.0/dd-systems/0/alerts?page=1&size=200&is_active=true",
		`{"paging_info":{"current_page":1,"page_entries":1,"total_entries":3,"page_size":2},"alert":[{"severity":"warning","class":"capacity"}]}`)

	got := healthAlerts(context.Background(), m)
	var total float64
	for _, s := range got {
		if s.Name == "ppdd_alerts_active" {
			total += s.Value
		}
	}
	if total != 3 {
		t.Fatalf("active alerts across pages = %v, want 3 (pagination dropped data)", total)
	}
}
```

- [ ] **Step 3: Run the full package test**

Run: `go test ./internal/ppdd/ -v`
Expected: PASS — including `TestLabelKeyConsistencyAcrossCollectors` and the new pagination test.

- [ ] **Step 4: Commit**

```bash
git add internal/ppdd/labels_test.go
git commit -m "test(ppdd): registry paths in full mock; multi-page alerts test

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

## Task 10: Docs, ADR, and changelog

**Files:**
- Modify: `docs/metrics.md`
- Create: `docs/adr/0007-dd-rest-api-realignment.md`
- Modify: `CHANGELOG.md`

- [ ] **Step 1: Update the metrics reference**

In `docs/metrics.md`, update the `capacity` and `mtrees` sections and mark provisional metrics:

```markdown
## capacity
- `ppdd_filesystem_total_bytes` / `ppdd_filesystem_used_bytes` / `ppdd_filesystem_available_bytes` (from `/system`)
- `ppdd_compression_factor` (from `/system`)
- _Provisional (no source in the 7.3 API guide; best-effort from `/file-system`):_
  `ppdd_compression_global_factor` / `ppdd_compression_local_factor` / `ppdd_compression_total_factor`,
  `ppdd_filesystem_cleaning_running`

## mtrees (labels: mtree)
- `ppdd_mtree_logical_used_bytes` / `ppdd_mtree_compression_factor` (per-MTree v2.0 stats)
- `ppdd_mtree_degraded` (1 if degraded) / `ppdd_mtree_retention_lock_enabled` (1 if retention lock active)
- _Provisional:_ `ppdd_mtree_physical_used_bytes` (mapped to `post_comp_written`),
  `ppdd_mtree_quota_soft_limit_bytes` / `ppdd_mtree_quota_hard_limit_bytes`
```

Update the `health` section's alerts line:

```markdown
- `ppdd_alerts_active{severity, class}` (active alerts only; `is_active=true`)
```

- [ ] **Step 2: Write the ADR with the live-DD validation checklist**

Create `docs/adr/0007-dd-rest-api-realignment.md`:

```markdown
# 7. DD REST API realignment to the documented 7.3 contract

Date: 2026-06-03

## Status
Accepted

## Context
Collector endpoints were modeled from partial docs as a uniform `/api/v1/...`. The
full PowerProtect DD 7.3 REST API guide shows the `/rest/` prefix with PER-RESOURCE
versions (mtrees v3.0, stats/capacity v2.0, system/alerts/auth v1.0), list pagination
(default 20, max 200), and an `is_active` alerts filter. No live appliance is
available, so the guide is treated as authoritative while undocumented mappings stay
provisional.

## Decision
- Centralize all paths in `internal/ppdd/endpoints.go` (single correction point).
- Use the `/rest/` prefix with documented per-resource versions.
- Add a `paginate()` helper for all list endpoints.
- Source capacity from `/system`; fetch active alerts with `is_active=true` + `class`.
- Rework MTrees to v3.0 metadata + per-MTree v2.0 stats.
- Keep undocumented metrics (compression split, GC cleaning, MTree physical_used,
  quotas) as flagged best-effort fetches — no metric removed.
- Align auth to the documented flat `{username,password}` body at `/rest/v1.0/auth`.

## Consequences
- Pagination fixes silent data loss past 20 items.
- MTree usage is N+1 requests (one stats call per MTree), best-effort.
- The auth body change is the highest-risk mapping.

## Validate against a live DD (revert points, in priority order)
1. **Auth** — `internal/ddclient/auth.go`: flat body + `/rest/v1.0/auth`. If logins
   fail, revert here first (prior shape: `{"auth_info":{...}}` at `/api/v1/auth`).
2. **Prefix/versions** — `internal/ppdd/endpoints.go`: confirm `/rest` vs `/api` and
   v1.0/v2.0/v3.0 tokens.
3. **Capacity** — confirm `/system` field names (`physical_capacity.{total,used,available}`,
   `compression_factor`).
4. **MTrees** — confirm v3.0 list fields (`id`, `is_degraded`, `mtree_rl_detail.rl_status`)
   and v2.0 stats (`stats_capacity[].tier_capacity_usage[].logical_capacity.used`).
5. **Alerts** — confirm `is_active` filter and `class` field.
6. **Provisional endpoints** — `replications`, `hardware/disks`, `stats/system-stats`,
   `file-system`: confirm or correct paths and field names.
```

- [ ] **Step 3: Add a changelog entry**

Add under the unreleased/top section of `CHANGELOG.md`:

```markdown
### Changed
- Realigned collectors to the documented PowerProtect DD 7.3 REST API: `/rest` prefix
  with per-resource versions (mtrees v3.0, stats v2.0), list pagination (no more silent
  truncation past 20 items), active-only alerts with a `class` label, capacity sourced
  from `/system`, and MTree usage from per-MTree v2.0 stats. Auth now posts the
  documented flat body to `/rest/v1.0/auth`.

### Added
- `ppdd_compression_factor`, `ppdd_mtree_degraded`, `ppdd_mtree_retention_lock_enabled`.

### Notes
- Metrics without a source in the 7.3 guide (compression split, GC cleaning, MTree
  physical_used, quotas) are retained as flagged-provisional best-effort fetches.
```

- [ ] **Step 4: Build docs strictly**

Run: `uvx --with mkdocs-material --with pymdown-extensions mkdocs build --strict`
Expected: build succeeds with no warnings.

- [ ] **Step 5: Commit**

```bash
git add docs/metrics.md docs/adr/0007-dd-rest-api-realignment.md CHANGELOG.md
git commit -m "docs: document API realignment, ADR, and changelog

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

## Task 11: Full CI gate

**Files:** none (verification only)

- [ ] **Step 1: Run the full CI gate**

Run: `make ci`
Expected: `gofmt` clean, `go vet` clean, `go test -race -cover ./...` all PASS, `go build` produces `bin/ppdd_exporter`.

- [ ] **Step 2: Grep for stale `/api/v1/` literals**

Run: `grep -rn "/api/v1/" internal/ || echo "none remaining"`
Expected: `none remaining` (all paths now flow through `endpoints.go`/`authPath`).

- [ ] **Step 3: If anything fails, fix and re-run**

Address failures in the offending task's files, re-run `make ci`, and amend/commit as appropriate.

---

## Self-Review

**Spec coverage:**
- §3 documented/provisional split → endpoints.go (T1), per-collector tasks (T5–T8), ADR table (T10). ✓
- §4.1 registry → T1 ✓ · §4.2 paginate → T4 ✓ · §4.3 auth+mock → T2, T3 ✓
- §4.4 capacity → T6 · mtrees → T7 · replication/disks/system-stats → T8 · alerts → T5 ✓
- §5 label consistency → T9 (`labels_test`) ✓ · §6 error handling (best-effort) → T6/T7 best-effort tests ✓
- §7 testing (fixtures, pagination test, mock fallback, system_test) → T2,T3,T4,T9 + fixtures across T5–T8 ✓
- §8 docs/ADR/changelog → T10 ✓ · §10 success criteria → T11 grep + `make ci` ✓

**Placeholder scan:** No TBD/TODO; every code step shows full code and exact commands. ✓

**Type consistency:** `pagingInfo` fields (`CurrentPage/PageEntries/TotalEntries/PageSize`) consistent across T4/T5/T7/T8. `mtreeListItem`/`mtreeStatsResp` defined in T7 and used only there. `pathSystem/pathAlerts/pathMTrees/pathReplication/pathDisks/pathSystemStats/pathFileSystem` + `mtreeStatsPath` defined in T1, used consistently. `authPath` in T2. `systemResp`/`fileSystemResp` in T6. ✓

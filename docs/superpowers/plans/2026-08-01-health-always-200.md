# /health always-200 + /livez /readyz (ppdd_exporter) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `/livez`/`/readyz` (always-200, no state) and make `/health`
always answer 200, matching obs_exporter's ADR-0013/ADR-0014 pattern.

**Architecture:** New `staticOKHandler` registered at `/livez`/`/readyz`.
`healthHandler` (`main.go:234-256`) loses its `StatusServiceUnavailable`
branch. JSON body shape (`built_at`, `systems: [{system, ok, last_scrape, err}]`)
unchanged.

**Tech Stack:** Go, `net/http`, `net/http/httptest`.

## Global Constraints

- Repo: `/Users/fjacquet/Projects/ppdd_exporter`.
- Spec: `/Users/fjacquet/Projects/obs_exporter/docs/superpowers/specs/2026-08-01-family-health-endpoint-design.md` (bucket B).
- `/health`'s path and JSON shape do not change — only the status code. Not a breaking change.
- `/livez`/`/readyz` are net-new — `### Added` in CHANGELOG.
- Next ADR number: 0010. ADR index table has 3 columns (`ADR | Decision | Status`) — match that.
- No `main_test.go` exists in this repo's root package today — this plan creates it.

---

### Task 1: `/livez` `/readyz` + drop `/health`'s 503

**Files:**
- Modify: `main.go:121-123` (add two `mux.HandleFunc` lines after the existing `/health` registration block)
- Modify: `main.go:234-256` (function `healthHandler`, remove 503 branch)
- Create: `main.go` — add `staticOKHandler` function after `healthHandler`'s closing brace
- Create: `main_test.go`

**Interfaces:**
- Consumes: `ppdd.SnapshotStore` (`internal/ppdd/snapshot.go:24`) — `Load() *Snapshot`, `NewSnapshotStore() *SnapshotStore`. `ppdd.Snapshot` (`internal/ppdd/snapshot.go:18-21`): `BuiltAt time.Time`, `Systems []*SystemSnapshot`. `ppdd.SystemSnapshot` (`internal/ppdd/snapshot.go:9-15`): `System string`, `LastScrape time.Time`, `OK bool`, `Err string`, `Samples []Sample`.
- Produces: `staticOKHandler(w http.ResponseWriter, _ *http.Request)`.

- [ ] **Step 1: Write failing tests**

Create `main_test.go`:

```go
package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/fjacquet/ppdd_exporter/internal/ppdd"
)

func TestLivezReturnsOK(t *testing.T) {
	rec := httptest.NewRecorder()
	staticOKHandler(rec, httptest.NewRequest(http.MethodGet, "/livez", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestReadyzReturnsOK(t *testing.T) {
	rec := httptest.NewRecorder()
	staticOKHandler(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestHealthReturns200WhenSystemUnhealthy(t *testing.T) {
	store := ppdd.NewSnapshotStore()
	store.Store(&ppdd.Snapshot{
		BuiltAt: time.Now(),
		Systems: []*ppdd.SystemSnapshot{
			{System: "dd01", OK: false, Err: "login POST: status 401"},
		},
	})

	rec := httptest.NewRecorder()
	healthHandler(rec, store)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var body struct {
		Systems []struct {
			System string `json:"system"`
			OK     bool   `json:"ok"`
			Err    string `json:"err"`
		} `json:"systems"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if len(body.Systems) != 1 || body.Systems[0].OK {
		t.Fatalf("systems = %+v, want one system with ok=false", body.Systems)
	}
}

func TestHealthReturns200WhenNoSystems(t *testing.T) {
	store := ppdd.NewSnapshotStore()

	rec := httptest.NewRecorder()
	healthHandler(rec, store)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test . -run 'TestLivezReturnsOK|TestReadyzReturnsOK|TestHealthReturns200' -v`
Expected: `TestLivezReturnsOK`/`TestReadyzReturnsOK` FAIL with `undefined: staticOKHandler`. `TestHealthReturns200*` FAIL with `status = 503, want 200`.

- [ ] **Step 3: Add `staticOKHandler` and register `/livez` `/readyz`**

In `main.go`, change lines 121-123 from:

```go
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		healthHandler(w, store)
	})
```

to:

```go
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		healthHandler(w, store)
	})
	mux.HandleFunc("/livez", staticOKHandler)
	mux.HandleFunc("/readyz", staticOKHandler)
```

After `healthHandler`'s closing brace (currently line 256), add:

```go

// staticOKHandler always answers 200 — no collection state, nothing that
// can make it fail. /livez and /readyz both use it: a probe wired here can
// never be the reason a healthy process gets restarted or pulled from
// rotation.
func staticOKHandler(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
}
```

- [ ] **Step 4: Drop the 503 branch in `healthHandler`**

Change the end of `healthHandler` (`main.go:234-256`) from:

```go
	healthy := len(snap.Systems) > 0
	for _, s := range snap.Systems {
		out.Systems = append(out.Systems, sysHealth{s.System, s.OK, s.LastScrape.Format(time.RFC3339), s.Err})
		if !s.OK {
			healthy = false
		}
	}
	w.Header().Set("Content-Type", "application/json")
	if !healthy {
		w.WriteHeader(http.StatusServiceUnavailable)
	}
	_ = json.NewEncoder(w).Encode(out)
```

to:

```go
	for _, s := range snap.Systems {
		out.Systems = append(out.Systems, sysHealth{s.System, s.OK, s.LastScrape.Format(time.RFC3339), s.Err})
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test . -run 'TestLivezReturnsOK|TestReadyzReturnsOK|TestHealthReturns200' -v`
Expected: all PASS.

- [ ] **Step 6: Run full test suite**

Run: `go test ./...`
Expected: all PASS.

- [ ] **Step 7: Commit**

```bash
git add main.go main_test.go
git commit -m "feat: add /livez /readyz, /health always answers 200

Matches obs_exporter's ADR-0013/ADR-0014 pattern: a system being
unreachable is data the exporter reports, not a failure of the
exporter itself. /livez and /readyz are trivial always-200 probe
endpoints with no state read; /health's JSON body (built_at,
systems[]) is unchanged, only its status code stops varying."
```

---

### Task 2: Chart, ADR, docs, CHANGELOG

**Files:**
- Modify: `charts/ppdd-exporter/values.yaml:50-56`
- Create: `docs/adr/0010-health-always-200-and-static-probes.md`
- Modify: `docs/adr/index.md` (append row after 0009, 3-column format `| ADR | Decision | Status |`)
- Modify: `CHANGELOG.md` (under existing `## [Unreleased]`)
- Modify: any deployment/troubleshooting docs mentioning `/health` and probe wiring — grep first (see Step 1)

**Interfaces:**
- Consumes: nothing (docs-only task).
- Produces: nothing.

- [ ] **Step 1: Find every doc mentioning `/health` as a probe target**

Run: `grep -rn '/health\|livenessProbe\|readinessProbe' docs/ README.md 2>/dev/null`

Update every hit describing `/health` as what the chart's probes use, or as
ever returning non-200: probes now use `/livez`/`/readyz` (always 200, no
system state); `/health` always answers 200 too, JSON body's `ok`/`err` per
system is the status signal. Use obs_exporter's `docs/deployment/kubernetes.md`
and `docs/operate/troubleshooting.md` (`~/Projects/obs_exporter/`) as the
structural template if this repo lacks comparable depth — swap "cluster" for
"system"/ppdd vocabulary.

- [ ] **Step 2: Update the chart**

In `charts/ppdd-exporter/values.yaml:50-56`, change:

```yaml
livenessProbe:
  httpGet:
    path: /health
readinessProbe:
  httpGet:
    path: /health
```

to:

```yaml
livenessProbe:
  httpGet:
    path: /livez
readinessProbe:
  httpGet:
    path: /readyz
```

(Only the `path:` values change — keep every other key in that block as-is.)

- [ ] **Step 3: Write ADR-0010**

Create `docs/adr/0010-health-always-200-and-static-probes.md`:

```markdown
# `/livez` `/readyz`, and `/health` always answering 200

## Status

Accepted (2026-08-01)

## Context

Same argument as obs_exporter's ADR-0013 and ADR-0014, applied here in one
pass: an exporter is a probe. "System unreachable" is data it reports, not a
failure of the exporter process. Coupling that fact to an HTTP status code
on any endpoint — the chart's `livenessProbe`/`readinessProbe`, or the
informational `/health` — risks something downstream (kubelet, a dashboard,
a script) treating a healthy, correctly-reporting exporter as down.

`charts/ppdd-exporter/values.yaml` wired both `livenessProbe` and
`readinessProbe` to `/health`, which answered 503 while any configured
system was unreachable. As a *liveness* check this is always wrong: no
restart makes an unreachable system reachable. As a *readiness* check it
pulls the exporter from the scrape pool exactly when the down-system metric
is the fact worth scraping.

## Decision

Two new endpoints, `/livez` and `/readyz`, both `staticOKHandler` — always
`200 OK`, no `SnapshotStore` read, nothing that can make either fail once
the process is running. The chart's default probes now point at them.
`/health`'s `healthHandler` no longer writes `http.StatusServiceUnavailable`
— it always answers 200, with the same JSON body (`built_at`,
`systems: [{system, ok, last_scrape, err}]`) as before. The per-system
`ok`/`err` fields are the only status channel now; nothing that parses the
body loses information.

## Consequences

- Anything gating on `/health`'s HTTP status code now sees 200
  unconditionally and must read `ok`/`err` per system instead.
- Chart default probe wiring changes; a fresh `helm install` or an upgrade
  without pinned probe overrides gets the fix automatically.
- Alert on a per-system `_up` metric (or `/health`'s body), never on any
  probe's HTTP status.
```

- [ ] **Step 4: Add the ADR to the index**

In `docs/adr/index.md`, after the `0009` row, add (3-column format):

```markdown
| [0010](0010-health-always-200-and-static-probes.md) | `/livez`/`/readyz` static probes; `/health` always answers 200 | accepted |
```

- [ ] **Step 5: CHANGELOG entry**

In `CHANGELOG.md`, under the existing `## [Unreleased]` heading, add:

```markdown
### Added

- `/livez` and `/readyz`: probe endpoints that always answer 200, with no
  dependency on system reachability or the collection cycle. See ADR-0010.

### Changed

- `/health` always answers 200, never 503. The JSON body's per-system
  `ok`/`err` fields are unchanged and remain the way to tell whether a
  system is degraded — read the body, not the status code. See ADR-0010.
  Not a breaking change: the path and JSON shape are unchanged.
- The chart's default `livenessProbe`/`readinessProbe` now point at
  `/livez`/`/readyz` instead of `/health`.
```

- [ ] **Step 6: Lint chart + build docs**

Run: `helm lint charts/ppdd-exporter` (or the exact CI invocation from `.github/workflows/` if different)
Expected: exits 0.

Run: `mkdocs build --strict` (if `mkdocs.yml` present)
Expected: exits 0.

- [ ] **Step 7: Commit**

```bash
git add charts/ppdd-exporter/values.yaml docs/adr/0010-health-always-200-and-static-probes.md \
  docs/adr/index.md CHANGELOG.md
git commit -m "docs+chart: record ADR-0010, repoint chart probes to /livez /readyz"
```

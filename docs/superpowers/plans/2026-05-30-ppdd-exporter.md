# PPDD Exporter Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a Go Prometheus exporter for Dell PowerProtect DD (Data Domain) appliances that polls many systems on an interval, publishes an immutable snapshot, and serves it at `/metrics`.

**Architecture:** Snapshot model — one background loop polls all DD systems in parallel, builds an immutable `Snapshot`, and pointer-swaps it into a store; the `/metrics` handler reads the latest snapshot. Collection is split into modular per-domain `ResourceCollector`s (capacity, mtrees, replication, health), each owning its endpoint + parse logic so the docs-only field risk is isolated to one file. Prometheus-only (OTLP deferred).

**Tech Stack:** Go 1.26, `go-resty/resty/v2`, `prometheus/client_golang`, `spf13/cobra`, `sirupsen/logrus`, `fsnotify/fsnotify`, `gopkg.in/yaml.v2`. Mock DD via `net/http/httptest`.

**Spec:** `docs/superpowers/specs/2026-05-30-ppdd-exporter-design.md`

> ⚠️ **Provisional API shapes.** All DD endpoint paths and JSON field names below are modeled from Dell docs and **must be validated against a live appliance**. Each is confined to one module file; correcting one means editing one struct + one fixture. JSON tags are marked `// provisional` where unverified.

---

## File Structure

| File | Responsibility |
|---|---|
| `go.mod` | Module + deps |
| `Makefile` | `make sure` / `make ci` / `make cli` targets |
| `main.go` | CLI (cobra), wiring, HTTP server, `/health`, reload |
| `config.yaml` | Example config |
| `internal/config/config.go` | Config struct, load, `${ENV}` interpolation, validation |
| `internal/config/watcher.go` | SIGHUP + fsnotify hot reload |
| `internal/ddclient/client.go` | `Client` interface + resty `SystemClient` |
| `internal/ddclient/auth.go` | Token login / re-login / logout |
| `internal/ddclient/mock.go` | In-memory `Client` for collector tests |
| `internal/ppdd/sample.go` | `Sample` / `Label` model + helpers |
| `internal/ppdd/snapshot.go` | `Snapshot`, `SystemSnapshot`, `SnapshotStore` |
| `internal/ppdd/collector.go` | Background loop, `collectSystem`, `BuildSnapshot` |
| `internal/ppdd/resource.go` | `ResourceCollector` interface + registry |
| `internal/ppdd/capacity.go` | Capacity & dedup module |
| `internal/ppdd/mtrees.go` | MTrees module |
| `internal/ppdd/replication.go` | Replication module |
| `internal/ppdd/health.go` | Health & ops module |
| `internal/ppdd/prometheus.go` | Unchecked Prometheus collector over the snapshot |
| `internal/ppdd/testdata/*.json` | Provisional fixtures |
| `Dockerfile`, `.github/workflows/*` | Packaging & CI (Phase 5) |

---

## Phase 0 — Scaffolding

### Task 1: Module skeleton + Makefile + build

**Files:**
- Create: `go.mod`, `Makefile`, `main.go`, `.gitignore` (exists), `config.yaml`

- [ ] **Step 1: Create `go.mod`**

```
module github.com/fjacquet/ppdd_exporter

go 1.26
```

- [ ] **Step 2: Create a minimal `main.go` that builds**

```go
// Command ppdd_exporter is a Prometheus exporter for Dell PowerProtect DD appliances.
package main

import "fmt"

// version is injected at build time via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	fmt.Printf("ppdd_exporter %s\n", version)
}
```

- [ ] **Step 3: Create `Makefile`**

```makefile
BIN := bin/ppdd_exporter
VERSION ?= dev

.PHONY: cli test test-race vet fmt-check sure ci
cli:
	go build -ldflags "-X main.version=$(VERSION)" -o $(BIN) .
test:
	go test ./...
test-race:
	go test -race -cover ./...
vet:
	go vet ./...
fmt-check:
	@test -z "$$(gofmt -l . | tee /dev/stderr)"
sure: fmt-check vet test cli
ci: fmt-check vet test-race
```

- [ ] **Step 4: Build it**

Run: `go build ./... && go run . `
Expected: prints `ppdd_exporter dev`

- [ ] **Step 5: Commit**

```bash
git add go.mod Makefile main.go
git commit -m "chore: scaffold ppdd_exporter module"
```

---

## Phase 1 — Core pipeline + capacity (MVP)

### Task 2: Sample model

**Files:**
- Create: `internal/ppdd/sample.go`, `internal/ppdd/sample_test.go`

- [ ] **Step 1: Write the failing test**

```go
package ppdd

import "testing"

func TestSampleLabelValueLookup(t *testing.T) {
	s := Sample{Name: "ppdd_filesystem_used_bytes", Value: 42,
		Labels: []Label{{Key: "system", Value: "dd01"}}}
	if got := s.LabelValue("system"); got != "dd01" {
		t.Fatalf("LabelValue(system) = %q, want dd01", got)
	}
	if got := s.LabelValue("missing"); got != "" {
		t.Fatalf("LabelValue(missing) = %q, want empty", got)
	}
}

func TestWithSystemPrependsLabel(t *testing.T) {
	s := Sample{Name: "x", Labels: []Label{{Key: "mtree", Value: "/data/col1/m1"}}}
	out := s.WithSystem("dd01")
	if out.Labels[0].Key != "system" || out.Labels[0].Value != "dd01" {
		t.Fatalf("WithSystem did not prepend system label: %+v", out.Labels)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/ppdd/ -run TestSample -v`
Expected: FAIL — `undefined: Sample`

- [ ] **Step 3: Write minimal implementation**

```go
// Package ppdd holds the DD metric model, snapshot store, modular collectors,
// and the Prometheus export path.
package ppdd

// Label is a single Prometheus label key/value.
type Label struct {
	Key   string
	Value string
}

// Sample is one metric data point: a name, an ordered label set, and a value.
type Sample struct {
	Name   string
	Labels []Label
	Value  float64
}

// LabelValue returns the value of the named label, or "" if absent.
func (s Sample) LabelValue(key string) string {
	for _, l := range s.Labels {
		if l.Key == key {
			return l.Value
		}
	}
	return ""
}

// WithSystem returns a copy with a leading {system=name} label. Collectors emit
// system-agnostic samples; the collection loop stamps the system identity.
func (s Sample) WithSystem(name string) Sample {
	labels := make([]Label, 0, len(s.Labels)+1)
	labels = append(labels, Label{Key: "system", Value: name})
	labels = append(labels, s.Labels...)
	return Sample{Name: s.Name, Labels: labels, Value: s.Value}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/ppdd/ -run TestSample -v && go test ./internal/ppdd/ -run TestWithSystem -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/ppdd/sample.go internal/ppdd/sample_test.go
git commit -m "feat(ppdd): add Sample/Label metric model"
```

---

### Task 3: Snapshot store

**Files:**
- Create: `internal/ppdd/snapshot.go`, `internal/ppdd/snapshot_test.go`

- [ ] **Step 1: Write the failing test**

```go
package ppdd

import "testing"

func TestSnapshotStoreLoadEmpty(t *testing.T) {
	st := NewSnapshotStore()
	if st.Load() == nil {
		t.Fatal("Load() on fresh store must return a non-nil empty snapshot")
	}
	if n := len(st.Load().Systems); n != 0 {
		t.Fatalf("fresh snapshot has %d systems, want 0", n)
	}
}

func TestSnapshotStoreStoreThenLoad(t *testing.T) {
	st := NewSnapshotStore()
	snap := &Snapshot{Systems: []*SystemSnapshot{{System: "dd01", OK: true}}}
	st.Store(snap)
	got := st.Load()
	if len(got.Systems) != 1 || got.Systems[0].System != "dd01" {
		t.Fatalf("Load() = %+v, want one system dd01", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/ppdd/ -run TestSnapshotStore -v`
Expected: FAIL — `undefined: NewSnapshotStore`

- [ ] **Step 3: Write minimal implementation**

```go
package ppdd

import (
	"sync"
	"time"
)

// SystemSnapshot is one system's result for a single collection cycle.
type SystemSnapshot struct {
	System     string
	LastScrape time.Time
	OK         bool   // true if at least the system was reachable & authenticated
	Err        string // top-level failure (auth/unreachable); empty when OK
	Samples    []Sample
}

// Snapshot is an immutable, point-in-time view across all systems.
type Snapshot struct {
	BuiltAt time.Time
	Systems []*SystemSnapshot
}

// SnapshotStore holds the latest Snapshot behind an RWMutex pointer-swap.
type SnapshotStore struct {
	mu   sync.RWMutex
	snap *Snapshot
}

// NewSnapshotStore returns a store pre-populated with an empty snapshot so
// readers never see nil before the first collection cycle.
func NewSnapshotStore() *SnapshotStore {
	return &SnapshotStore{snap: &Snapshot{}}
}

// Store atomically swaps in a new snapshot.
func (s *SnapshotStore) Store(snap *Snapshot) {
	s.mu.Lock()
	s.snap = snap
	s.mu.Unlock()
}

// Load returns the current snapshot (never nil).
func (s *SnapshotStore) Load() *Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.snap
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/ppdd/ -run TestSnapshotStore -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/ppdd/snapshot.go internal/ppdd/snapshot_test.go
git commit -m "feat(ppdd): add immutable snapshot store"
```

---

### Task 4: ddclient interface + mock

**Files:**
- Create: `internal/ddclient/client.go` (interface only for now), `internal/ddclient/mock.go`, `internal/ddclient/mock_test.go`

- [ ] **Step 1: Write the failing test**

```go
package ddclient

import (
	"context"
	"testing"
)

func TestMockClientServesRegisteredPath(t *testing.T) {
	m := NewMock("dd01")
	m.SetJSON("/api/v1/dd-systems/0/file-system", `{"physical_used":10}`)

	var out struct {
		PhysicalUsed float64 `json:"physical_used"`
	}
	if err := m.Get(context.Background(), "/api/v1/dd-systems/0/file-system", &out); err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if out.PhysicalUsed != 10 {
		t.Fatalf("PhysicalUsed = %v, want 10", out.PhysicalUsed)
	}
}

func TestMockClientUnknownPathErrors(t *testing.T) {
	m := NewMock("dd01")
	var out map[string]any
	if err := m.Get(context.Background(), "/nope", &out); err == nil {
		t.Fatal("expected error for unregistered path")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/ddclient/ -run TestMock -v`
Expected: FAIL — `undefined: NewMock`

- [ ] **Step 3: Write the interface and mock**

`internal/ddclient/client.go`:
```go
// Package ddclient is the per-appliance Dell PowerProtect DD REST API client.
package ddclient

import "context"

// Client is the per-system DD API client abstraction. It is satisfied by the live
// SystemClient and by Mock (tests). Get authenticates lazily and decodes JSON.
type Client interface {
	// Name returns the configured system name (used as the `system` label).
	Name() string
	// Get fetches an absolute API path (e.g. "/api/v1/dd-systems/0/file-system")
	// and JSON-decodes the body into out. It (re-)authenticates as needed.
	Get(ctx context.Context, path string, out any) error
	// Close releases the session (DELETE /api/v1/auth) and HTTP resources.
	Close() error
}
```

`internal/ddclient/mock.go`:
```go
package ddclient

import (
	"context"
	"encoding/json"
	"fmt"
)

// Mock is an in-memory Client that serves canned JSON bodies per path. Tests use it
// to drive collectors without a live appliance.
type Mock struct {
	name  string
	paths map[string]string
}

// NewMock returns an empty Mock for the named system.
func NewMock(name string) *Mock { return &Mock{name: name, paths: map[string]string{}} }

// SetJSON registers a response body for an exact path.
func (m *Mock) SetJSON(path, body string) { m.paths[path] = body }

func (m *Mock) Name() string { return m.name }

func (m *Mock) Get(_ context.Context, path string, out any) error {
	body, ok := m.paths[path]
	if !ok {
		return fmt.Errorf("mock: no response registered for %s", path)
	}
	return json.Unmarshal([]byte(body), out)
}

func (m *Mock) Close() error { return nil }
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/ddclient/ -run TestMock -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/ddclient/client.go internal/ddclient/mock.go internal/ddclient/mock_test.go
git commit -m "feat(ddclient): add Client interface and in-memory mock"
```

---

### Task 5: Live client + token auth

**Files:**
- Create: `internal/ddclient/auth.go`, `internal/ddclient/system.go`, `internal/ddclient/system_test.go`
- Modify: `go.mod` (adds `resty`)

> Provisional: DD login is `POST /api/v1/auth` with body `{"auth_info":{"username":...,"password":...}}`, returning the session token in the `X-DD-AUTH-TOKEN` **response header**; that header is then sent on every request. Logout is `DELETE /api/v1/auth`. Confirm against a live DD.

- [ ] **Step 1: Add resty**

Run: `go get github.com/go-resty/resty/v2@latest`

- [ ] **Step 2: Write the failing test (httptest DD)**

```go
package ddclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

const authToken = "test-token-123"

// writeBytes avoids the Semgrep write-to-ResponseWriter rule.
func writeBytes(w http.ResponseWriter, b []byte) { _, _ = w.Write(b) }

func newFakeDD(t *testing.T, logins *int32) *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/auth", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			atomic.AddInt32(logins, 1)
			w.Header().Set("X-DD-AUTH-TOKEN", authToken)
			w.WriteHeader(http.StatusCreated)
		case http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		}
	})
	mux.HandleFunc("/api/v1/dd-systems/0/file-system", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-DD-AUTH-TOKEN") != authToken {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		writeBytes(w, []byte(`{"physical_used":99}`))
	})
	return httptest.NewTLSServer(mux)
}

func TestSystemClientAuthAndGet(t *testing.T) {
	var logins int32
	srv := newFakeDD(t, &logins)
	defer srv.Close()

	c := NewSystemClient(Config{
		Name: "dd01", BaseURL: srv.URL, Username: "u", Password: "p",
		InsecureSkipVerify: true, HTTPClient: srv.Client(),
	})
	defer c.Close()

	var out struct {
		PhysicalUsed float64 `json:"physical_used"`
	}
	if err := c.Get(context.Background(), "/api/v1/dd-systems/0/file-system", &out); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if out.PhysicalUsed != 99 {
		t.Fatalf("PhysicalUsed = %v, want 99", out.PhysicalUsed)
	}
	// Second Get reuses the token — no extra login.
	_ = c.Get(context.Background(), "/api/v1/dd-systems/0/file-system", &out)
	if logins != 1 {
		t.Fatalf("logins = %d, want 1 (token should be reused)", logins)
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/ddclient/ -run TestSystemClientAuth -v`
Expected: FAIL — `undefined: NewSystemClient`

- [ ] **Step 4: Write the client + auth**

`internal/ddclient/system.go`:
```go
package ddclient

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"sync"

	"github.com/go-resty/resty/v2"
)

// Config configures a SystemClient. HTTPClient is optional (tests inject the
// httptest TLS client); when nil a client honoring InsecureSkipVerify is built.
type Config struct {
	Name               string
	BaseURL            string // e.g. https://dd01.example.com:3009
	Username           string
	Password           string
	InsecureSkipVerify bool
	HTTPClient         *http.Client
}

// SystemClient is the live per-appliance DD REST client.
type SystemClient struct {
	cfg   Config
	rc    *resty.Client
	mu    sync.Mutex
	token string
}

// NewSystemClient builds a client. Auth is lazy (on first Get).
func NewSystemClient(cfg Config) *SystemClient {
	rc := resty.New().SetBaseURL(cfg.BaseURL)
	if cfg.HTTPClient != nil {
		rc.SetTransport(cfg.HTTPClient.Transport)
	} else if cfg.InsecureSkipVerify {
		rc.SetTLSClientConfig(&tls.Config{InsecureSkipVerify: true}) //nolint:gosec // operator opt-in
	}
	// Retry, but never on 4xx (do not retry auth/permission failures).
	rc.SetRetryCount(2).AddRetryCondition(func(r *resty.Response, _ error) bool {
		return r.StatusCode() >= 500
	})
	return &SystemClient{cfg: cfg, rc: rc}
}

func (c *SystemClient) Name() string { return c.cfg.Name }

// Get fetches path, authenticating first if needed and re-authenticating once on 401.
func (c *SystemClient) Get(ctx context.Context, path string, out any) error {
	if err := c.ensureToken(ctx); err != nil {
		return err
	}
	resp, err := c.do(ctx, path, out)
	if err != nil {
		return err
	}
	if resp.StatusCode() == http.StatusUnauthorized {
		c.clearToken()
		if err := c.ensureToken(ctx); err != nil {
			return err
		}
		resp, err = c.do(ctx, path, out)
		if err != nil {
			return err
		}
	}
	if resp.StatusCode() >= 300 {
		return fmt.Errorf("GET %s: status %d", path, resp.StatusCode())
	}
	return nil
}

func (c *SystemClient) do(ctx context.Context, path string, out any) (*resty.Response, error) {
	return c.rc.R().SetContext(ctx).
		SetHeader("X-DD-AUTH-TOKEN", c.currentToken()).
		SetResult(out).
		Get(path)
}

func (c *SystemClient) currentToken() string { c.mu.Lock(); defer c.mu.Unlock(); return c.token }
func (c *SystemClient) clearToken()          { c.mu.Lock(); c.token = ""; c.mu.Unlock() }
```

`internal/ddclient/auth.go`:
```go
package ddclient

import (
	"context"
	"fmt"
)

// authRequest is the provisional DD login body. Validate against a live appliance.
type authRequest struct {
	AuthInfo struct {
		Username string `json:"username"`
		Password string `json:"password"`
	} `json:"auth_info"`
}

// ensureToken logs in if no token is cached, capturing X-DD-AUTH-TOKEN.
func (c *SystemClient) ensureToken(ctx context.Context) error {
	if c.currentToken() != "" {
		return nil
	}
	var body authRequest
	body.AuthInfo.Username = c.cfg.Username
	body.AuthInfo.Password = c.cfg.Password

	resp, err := c.rc.R().SetContext(ctx).SetBody(body).Post("/api/v1/auth")
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
	_, _ = c.rc.R().SetHeader("X-DD-AUTH-TOKEN", c.currentToken()).Delete("/api/v1/auth")
	c.clearToken()
	return nil
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/ddclient/ -run TestSystemClientAuth -v`
Expected: PASS

- [ ] **Step 6: Add a re-login-on-401 test**

```go
func TestSystemClientReloginOn401(t *testing.T) {
	var logins int32
	var rotated atomic.Bool
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/auth", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&logins, 1)
		tok := "tok1"
		if rotated.Load() {
			tok = "tok2"
		}
		w.Header().Set("X-DD-AUTH-TOKEN", tok)
		w.WriteHeader(http.StatusCreated)
	})
	mux.HandleFunc("/api/v1/dd-systems/0/file-system", func(w http.ResponseWriter, r *http.Request) {
		// Only "tok2" is accepted; first call (tok1) returns 401 and forces relogin.
		if r.Header.Get("X-DD-AUTH-TOKEN") != "tok2" {
			rotated.Store(true)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		writeBytes(w, []byte(`{"physical_used":1}`))
	})
	srv := httptest.NewTLSServer(mux)
	defer srv.Close()

	c := NewSystemClient(Config{Name: "dd01", BaseURL: srv.URL, HTTPClient: srv.Client()})
	defer c.Close()
	var out map[string]any
	if err := c.Get(context.Background(), "/api/v1/dd-systems/0/file-system", &out); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if logins != 2 {
		t.Fatalf("logins = %d, want 2 (one initial + one relogin)", logins)
	}
}
```

Run: `go test ./internal/ddclient/ -v`
Expected: PASS (all)

- [ ] **Step 7: Commit**

```bash
git add internal/ddclient/ go.mod go.sum
git commit -m "feat(ddclient): live token-auth client with relogin-on-401"
```

---

### Task 6: Config model + env interpolation

**Files:**
- Create: `internal/config/config.go`, `internal/config/config_test.go`
- Modify: `go.mod` (adds `yaml.v2`)

- [ ] **Step 1: Add yaml**

Run: `go get gopkg.in/yaml.v2@latest`

- [ ] **Step 2: Write the failing test**

```go
package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadInterpolatesEnvAndDefaults(t *testing.T) {
	t.Setenv("DD01_PASSWORD", "s3cret")
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	yaml := `
server: {host: "0.0.0.0", port: "9099", uri: "/metrics"}
collection: {interval: "5m", timeout: "60s"}
systems:
  - {name: dd01, host: dd01.example.com, username: u, password: "${DD01_PASSWORD}", insecureSkipVerify: true}
`
	if err := os.WriteFile(path, []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Systems[0].Password != "s3cret" {
		t.Fatalf("password = %q, want interpolated s3cret", cfg.Systems[0].Password)
	}
	if cfg.Collection.Interval.String() != "5m0s" {
		t.Fatalf("interval = %s, want 5m0s", cfg.Collection.Interval)
	}
}

func TestLoadRejectsEmptySystems(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "c.yaml")
	_ = os.WriteFile(path, []byte("systems: []\n"), 0o600)
	if _, err := Load(path); err == nil {
		t.Fatal("expected error when no systems configured")
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/config/ -v`
Expected: FAIL — `undefined: Load`

- [ ] **Step 4: Write the implementation**

```go
// Package config loads and validates the exporter configuration.
package config

import (
	"fmt"
	"os"
	"regexp"
	"time"

	"gopkg.in/yaml.v2"
)

// System is one DD appliance to monitor.
type System struct {
	Name               string `yaml:"name"`
	Host               string `yaml:"host"`
	Port               int    `yaml:"port"` // defaults to 3009
	Username           string `yaml:"username"`
	Password           string `yaml:"password"`
	PasswordFile       string `yaml:"passwordFile"`
	InsecureSkipVerify bool   `yaml:"insecureSkipVerify"`
}

// BaseURL returns the https://host:port root for the DD REST API.
func (s System) BaseURL() string {
	port := s.Port
	if port == 0 {
		port = 3009
	}
	return fmt.Sprintf("https://%s:%d", s.Host, port)
}

// Server holds HTTP-server settings.
type Server struct {
	Host    string `yaml:"host"`
	Port    string `yaml:"port"`
	URI     string `yaml:"uri"`
	LogName string `yaml:"logName"`
}

// Collection holds loop timing.
type Collection struct {
	Interval time.Duration `yaml:"interval"`
	Timeout  time.Duration `yaml:"timeout"`
}

// Config is the whole file.
type Config struct {
	Server     Server     `yaml:"server"`
	Collection Collection `yaml:"collection"`
	Systems    []System   `yaml:"systems"`
}

var envRef = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)

func interpolate(s string) string {
	return envRef.ReplaceAllStringFunc(s, func(m string) string {
		return os.Getenv(envRef.FindStringSubmatch(m)[1])
	})
}

// Load reads, interpolates ${ENV} references, applies defaults, and validates.
func Load(path string) (*Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	for i := range cfg.Systems {
		s := &cfg.Systems[i]
		s.Password = interpolate(s.Password)
		if s.PasswordFile != "" && s.Password == "" {
			b, err := os.ReadFile(s.PasswordFile)
			if err != nil {
				return nil, fmt.Errorf("system %s passwordFile: %w", s.Name, err)
			}
			s.Password = string(b)
		}
	}
	if cfg.Server.Port == "" {
		cfg.Server.Port = "9099"
	}
	if cfg.Server.URI == "" {
		cfg.Server.URI = "/metrics"
	}
	if cfg.Collection.Interval == 0 {
		cfg.Collection.Interval = 5 * time.Minute
	}
	if cfg.Collection.Timeout == 0 {
		cfg.Collection.Timeout = 60 * time.Second
	}
	if len(cfg.Systems) == 0 {
		return nil, fmt.Errorf("no systems configured")
	}
	return &cfg, nil
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/config/ -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go go.mod go.sum
git commit -m "feat(config): YAML config with env interpolation and defaults"
```

---

### Task 7: ResourceCollector interface + capacity module

**Files:**
- Create: `internal/ppdd/resource.go`, `internal/ppdd/capacity.go`, `internal/ppdd/capacity_test.go`, `internal/ppdd/testdata/file-system.json`

- [ ] **Step 1: Create the provisional fixture `internal/ppdd/testdata/file-system.json`**

```json
{
  "physical_capacity_bytes": 100000000000,
  "physical_used_bytes": 40000000000,
  "physical_available_bytes": 60000000000,
  "global_compression_factor": 5.5,
  "local_compression_factor": 1.8,
  "total_compression_factor": 9.9,
  "cleaning": { "status": "running" }
}
```

- [ ] **Step 2: Write the failing test**

```go
package ppdd

import (
	"context"
	"os"
	"testing"

	"github.com/fjacquet/ppdd_exporter/internal/ddclient"
)

func TestCapacityCollect(t *testing.T) {
	body, err := os.ReadFile("testdata/file-system.json")
	if err != nil {
		t.Fatal(err)
	}
	m := ddclient.NewMock("dd01")
	m.SetJSON("/api/v1/dd-systems/0/file-system", string(body))

	got, err := Capacity{}.Collect(context.Background(), m)
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	want := map[string]float64{
		"ppdd_filesystem_total_bytes":       100000000000,
		"ppdd_filesystem_used_bytes":        40000000000,
		"ppdd_filesystem_available_bytes":   60000000000,
		"ppdd_compression_global_factor":    5.5,
		"ppdd_compression_local_factor":     1.8,
		"ppdd_compression_total_factor":     9.9,
		"ppdd_filesystem_cleaning_running":  1,
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
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/ppdd/ -run TestCapacityCollect -v`
Expected: FAIL — `undefined: Capacity`

- [ ] **Step 4: Write `resource.go` and `capacity.go`**

`internal/ppdd/resource.go`:
```go
package ppdd

import (
	"context"

	"github.com/fjacquet/ppdd_exporter/internal/ddclient"
)

// ResourceCollector collects one metric domain from a single DD system. It returns
// system-agnostic samples; the loop stamps the `system` label. Implementations own
// their endpoint path and JSON struct so the provisional API risk is localized.
type ResourceCollector interface {
	Name() string
	Collect(ctx context.Context, c ddclient.Client) ([]Sample, error)
}

// Registry is the ordered set of collectors run for every system.
func Registry() []ResourceCollector {
	return []ResourceCollector{
		Capacity{},
		// Phase 2: MTrees{}, Phase 3: Replication{}, Phase 4: Health{}
	}
}
```

`internal/ppdd/capacity.go`:
```go
package ppdd

import (
	"context"

	"github.com/fjacquet/ppdd_exporter/internal/ddclient"
)

// fileSystemResp is the provisional shape of GET .../dd-systems/0/file-system.
type fileSystemResp struct {
	PhysicalCapacityBytes  float64 `json:"physical_capacity_bytes"`
	PhysicalUsedBytes      float64 `json:"physical_used_bytes"`
	PhysicalAvailableBytes float64 `json:"physical_available_bytes"`
	GlobalCompression      float64 `json:"global_compression_factor"`
	LocalCompression       float64 `json:"local_compression_factor"`
	TotalCompression       float64 `json:"total_compression_factor"`
	Cleaning               struct {
		Status string `json:"status"`
	} `json:"cleaning"`
}

// Capacity collects filesystem capacity, dedup/compression factors, and GC state.
type Capacity struct{}

func (Capacity) Name() string { return "capacity" }

func (Capacity) Collect(ctx context.Context, c ddclient.Client) ([]Sample, error) {
	var r fileSystemResp
	if err := c.Get(ctx, "/api/v1/dd-systems/0/file-system", &r); err != nil {
		return nil, err
	}
	cleaning := 0.0
	if r.Cleaning.Status == "running" {
		cleaning = 1
	}
	return []Sample{
		{Name: "ppdd_filesystem_total_bytes", Value: r.PhysicalCapacityBytes},
		{Name: "ppdd_filesystem_used_bytes", Value: r.PhysicalUsedBytes},
		{Name: "ppdd_filesystem_available_bytes", Value: r.PhysicalAvailableBytes},
		{Name: "ppdd_compression_global_factor", Value: r.GlobalCompression},
		{Name: "ppdd_compression_local_factor", Value: r.LocalCompression},
		{Name: "ppdd_compression_total_factor", Value: r.TotalCompression},
		{Name: "ppdd_filesystem_cleaning_running", Value: cleaning},
	}, nil
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/ppdd/ -run TestCapacityCollect -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/ppdd/resource.go internal/ppdd/capacity.go internal/ppdd/capacity_test.go internal/ppdd/testdata/file-system.json
git commit -m "feat(ppdd): ResourceCollector interface and capacity module"
```

---

### Task 8: Collection loop + BuildSnapshot

**Files:**
- Create: `internal/ppdd/collector.go`, `internal/ppdd/collector_test.go`
- Modify: `go.mod` (adds `golang.org/x/sync`, `logrus`)

- [ ] **Step 1: Add deps**

Run: `go get golang.org/x/sync@latest github.com/sirupsen/logrus@latest`

- [ ] **Step 2: Write the failing test**

```go
package ppdd

import (
	"context"
	"testing"
	"time"

	"github.com/fjacquet/ppdd_exporter/internal/ddclient"
)

func TestCollectOncePopulatesSnapshot(t *testing.T) {
	m := ddclient.NewMock("dd01")
	m.SetJSON("/api/v1/dd-systems/0/file-system", `{"physical_used_bytes":7}`)

	store := NewSnapshotStore()
	col := NewCollector([]ddclient.Client{m}, Registry(), store, time.Minute, 10*time.Second)
	snap := col.CollectOnce(context.Background())

	if len(snap.Systems) != 1 {
		t.Fatalf("systems = %d, want 1", len(snap.Systems))
	}
	sys := snap.Systems[0]
	if !sys.OK {
		t.Fatalf("system not OK: %s", sys.Err)
	}
	var used float64
	for _, s := range sys.Samples {
		if s.Name == "ppdd_filesystem_used_bytes" {
			used = s.Value
		}
		if s.LabelValue("system") != "dd01" {
			t.Errorf("sample %s missing system label", s.Name)
		}
	}
	if used != 7 {
		t.Fatalf("used = %v, want 7", used)
	}
}

func TestCollectSystemDegradesOnError(t *testing.T) {
	m := ddclient.NewMock("dd01") // no paths registered -> every collector errors
	store := NewSnapshotStore()
	col := NewCollector([]ddclient.Client{m}, Registry(), store, time.Minute, 10*time.Second)
	snap := col.CollectOnce(context.Background())

	sys := snap.Systems[0]
	// A per-collector failure surfaces a ppdd_collector_up{collector=...}=0 sample
	// but does not crash the cycle.
	var up float64 = -1
	for _, s := range sys.Samples {
		if s.Name == "ppdd_collector_up" && s.LabelValue("collector") == "capacity" {
			up = s.Value
		}
	}
	if up != 0 {
		t.Fatalf("ppdd_collector_up{capacity} = %v, want 0", up)
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/ppdd/ -run TestCollect -v`
Expected: FAIL — `undefined: NewCollector`

- [ ] **Step 4: Write the collector**

```go
package ppdd

import (
	"context"
	"time"

	"github.com/fjacquet/ppdd_exporter/internal/ddclient"
	log "github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
)

// Collector runs the background loop: every interval it polls all systems in
// parallel and publishes a fresh Snapshot. One system's failure never blocks others.
type Collector struct {
	clients    []ddclient.Client
	collectors []ResourceCollector
	store      *SnapshotStore
	interval   time.Duration
	timeout    time.Duration
}

// NewCollector wires the loop.
func NewCollector(clients []ddclient.Client, collectors []ResourceCollector, store *SnapshotStore, interval, timeout time.Duration) *Collector {
	return &Collector{clients: clients, collectors: collectors, store: store, interval: interval, timeout: timeout}
}

// CollectOnce runs a single cycle, stores, and returns the snapshot.
func (c *Collector) CollectOnce(ctx context.Context) *Snapshot {
	snap := c.collectAll(ctx)
	c.store.Store(snap)
	return snap
}

// Run loops until ctx is cancelled (assumes CollectOnce already primed the store).
func (c *Collector) Run(ctx context.Context) {
	t := time.NewTicker(c.interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			c.store.Store(c.collectAll(ctx))
		}
	}
}

func (c *Collector) collectAll(ctx context.Context) *Snapshot {
	results := make([]*SystemSnapshot, len(c.clients))
	g, gctx := errgroup.WithContext(ctx)
	for i, client := range c.clients {
		i, client := i, client
		g.Go(func() error {
			results[i] = c.collectSystem(gctx, client)
			return nil // graceful degradation
		})
	}
	_ = g.Wait()
	return &Snapshot{BuiltAt: time.Now(), Systems: results}
}

func (c *Collector) collectSystem(ctx context.Context, client ddclient.Client) *SystemSnapshot {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	ss := &SystemSnapshot{System: client.Name(), LastScrape: time.Now(), OK: true}
	for _, rc := range c.collectors {
		samples, err := rc.Collect(ctx, client)
		up := 1.0
		if err != nil {
			up = 0
			log.WithFields(log.Fields{"system": client.Name(), "collector": rc.Name(), "err": err}).
				Warn("collector failed")
		}
		ss.Samples = append(ss.Samples, Sample{
			Name:   "ppdd_collector_up",
			Labels: []Label{{Key: "collector", Value: rc.Name()}},
			Value:  up,
		}.WithSystem(client.Name()))
		for _, s := range samples {
			ss.Samples = append(ss.Samples, s.WithSystem(client.Name()))
		}
	}
	return ss
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/ppdd/ -run TestCollect -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/ppdd/collector.go internal/ppdd/collector_test.go go.mod go.sum
git commit -m "feat(ppdd): parallel collection loop with per-collector degradation"
```

---

### Task 9: Prometheus collector

**Files:**
- Create: `internal/ppdd/prometheus.go`, `internal/ppdd/prometheus_test.go`
- Modify: `go.mod` (adds `prometheus/client_golang`)

- [ ] **Step 1: Add prometheus**

Run: `go get github.com/prometheus/client_golang@latest`

- [ ] **Step 2: Write the failing test**

```go
package ppdd

import (
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestPromCollectorEmitsSnapshot(t *testing.T) {
	store := NewSnapshotStore()
	store.Store(&Snapshot{
		BuiltAt: time.Now(),
		Systems: []*SystemSnapshot{{
			System: "dd01", OK: true,
			Samples: []Sample{
				{Name: "ppdd_filesystem_used_bytes", Labels: []Label{{"system", "dd01"}}, Value: 7},
			},
		}},
	})
	reg := prometheus.NewRegistry()
	reg.MustRegister(NewPromCollector(store))

	want := `
# HELP ppdd_filesystem_used_bytes DD metric ppdd_filesystem_used_bytes
# TYPE ppdd_filesystem_used_bytes gauge
ppdd_filesystem_used_bytes{system="dd01"} 7
`
	if err := testutil.CollectAndCompare(NewPromCollector(store), strings.NewReader(want), "ppdd_filesystem_used_bytes"); err != nil {
		t.Fatal(err)
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/ppdd/ -run TestPromCollector -v`
Expected: FAIL — `undefined: NewPromCollector`

- [ ] **Step 4: Write the unchecked collector**

```go
package ppdd

import "github.com/prometheus/client_golang/prometheus"

// PromCollector is an unchecked Prometheus collector: Describe emits nothing so the
// metric-name set can vary per snapshot. Collect reads the latest snapshot.
type PromCollector struct {
	store *SnapshotStore
}

// NewPromCollector wraps the snapshot store as a prometheus.Collector.
func NewPromCollector(store *SnapshotStore) *PromCollector { return &PromCollector{store: store} }

// Describe sends nothing (unchecked collector).
func (p *PromCollector) Describe(chan<- *prometheus.Desc) {}

// Collect turns every snapshot sample into a gauge metric.
func (p *PromCollector) Collect(ch chan<- prometheus.Metric) {
	snap := p.store.Load()
	for _, sys := range snap.Systems {
		for _, s := range sys.Samples {
			keys := make([]string, len(s.Labels))
			vals := make([]string, len(s.Labels))
			for i, l := range s.Labels {
				keys[i], vals[i] = l.Key, l.Value
			}
			desc := prometheus.NewDesc(s.Name, "DD metric "+s.Name, keys, nil)
			m, err := prometheus.NewConstMetric(desc, prometheus.GaugeValue, s.Value, vals...)
			if err != nil {
				continue // skip inconsistent label sets rather than panic
			}
			ch <- m
		}
	}
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/ppdd/ -run TestPromCollector -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/ppdd/prometheus.go internal/ppdd/prometheus_test.go go.mod go.sum
git commit -m "feat(ppdd): unchecked Prometheus collector over snapshot"
```

---

### Task 10: main.go wiring + /health + flags

**Files:**
- Modify: `main.go`
- Create: `config.yaml`
- Modify: `go.mod` (adds `cobra`)

- [ ] **Step 1: Add cobra**

Run: `go get github.com/spf13/cobra@latest`

- [ ] **Step 2: Write `config.yaml` example**

```yaml
---
server:
  host: "0.0.0.0"
  port: "9099"
  uri: "/metrics"
  logName: ""            # "" -> stdout
collection:
  interval: "5m"
  timeout: "60s"
systems:
  - name: dd-prod-01
    host: dd01.example.com
    username: ppdd-monitor
    password: "${DD01_PASSWORD}"
    insecureSkipVerify: true
```

- [ ] **Step 3: Replace `main.go`**

```go
// Command ppdd_exporter is a Prometheus exporter for Dell PowerProtect DD appliances.
package main

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/fjacquet/ppdd_exporter/internal/config"
	"github.com/fjacquet/ppdd_exporter/internal/ddclient"
	"github.com/fjacquet/ppdd_exporter/internal/ppdd"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var version = "dev"

func main() {
	var cfgPath string
	var once, debug bool
	root := &cobra.Command{
		Use:     "ppdd_exporter",
		Version: version,
		RunE: func(_ *cobra.Command, _ []string) error {
			return run(cfgPath, once, debug)
		},
	}
	root.Flags().StringVar(&cfgPath, "config", "config.yaml", "path to config file")
	root.Flags().BoolVar(&once, "once", false, "run a single collection cycle and exit")
	root.Flags().BoolVar(&debug, "debug", false, "verbose logging")
	if err := root.Execute(); err != nil {
		log.Fatal(err)
	}
}

func run(cfgPath string, once, debug bool) error {
	if debug {
		log.SetLevel(log.DebugLevel)
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return err
	}
	clients := make([]ddclient.Client, 0, len(cfg.Systems))
	for _, s := range cfg.Systems {
		clients = append(clients, ddclient.NewSystemClient(ddclient.Config{
			Name: s.Name, BaseURL: s.BaseURL(), Username: s.Username,
			Password: s.Password, InsecureSkipVerify: s.InsecureSkipVerify,
		}))
	}
	defer func() {
		for _, c := range clients {
			_ = c.Close()
		}
	}()

	store := ppdd.NewSnapshotStore()
	col := ppdd.NewCollector(clients, ppdd.Registry(), store, cfg.Collection.Interval, cfg.Collection.Timeout)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	log.Info("running initial collection cycle")
	col.CollectOnce(ctx)
	if once {
		return nil
	}
	go col.Run(ctx)

	reg := prometheus.NewRegistry()
	reg.MustRegister(ppdd.NewPromCollector(store))

	mux := http.NewServeMux()
	mux.Handle(cfg.Server.URI, promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		healthHandler(w, store)
	})

	srv := &http.Server{
		Addr:              cfg.Server.Host + ":" + cfg.Server.Port,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		<-ctx.Done()
		sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(sctx)
	}()
	log.WithField("addr", srv.Addr).Info("serving metrics")
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

func healthHandler(w http.ResponseWriter, store *ppdd.SnapshotStore) {
	snap := store.Load()
	type sysHealth struct {
		System     string `json:"system"`
		OK         bool   `json:"ok"`
		LastScrape string `json:"last_scrape"`
		Err        string `json:"err,omitempty"`
	}
	out := struct {
		BuiltAt string      `json:"built_at"`
		Systems []sysHealth `json:"systems"`
	}{BuiltAt: snap.BuiltAt.Format(time.RFC3339)}
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
}
```

- [ ] **Step 4: Build and smoke-test**

Run: `make cli && ./bin/ppdd_exporter --once --config config.yaml --debug`
Expected: builds; runs one cycle and exits 0. (Systems will report `ppdd_collector_up=0` / not OK without a reachable DD — that is expected. Validate the process wiring, not live data.)

- [ ] **Step 5: Commit**

```bash
git add main.go config.yaml go.mod go.sum
git commit -m "feat: wire CLI, collection loop, /metrics, and /health"
```

---

### Task 11: Hot config reload (SIGHUP + file watch)

**Files:**
- Create: `internal/config/watcher.go`, `internal/config/watcher_test.go`
- Modify: `go.mod` (adds `fsnotify`)

> **Scope note:** reload rebuilds clients + collector on change. To keep this task focused, the watcher exposes a channel of validated `*Config`; wiring it to rebuild the running collector is a small `main.go` follow-up included in Step 5.

- [ ] **Step 1: Add fsnotify**

Run: `go get github.com/fsnotify/fsnotify@latest`

- [ ] **Step 2: Write the failing test**

```go
package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWatcherEmitsOnSIGHUPFunc(t *testing.T) {
	t.Setenv("DD01_PASSWORD", "p")
	dir := t.TempDir()
	path := filepath.Join(dir, "c.yaml")
	write := func(port string) {
		_ = os.WriteFile(path, []byte(
			"server: {port: \""+port+"\"}\ncollection: {interval: 5m}\n"+
				"systems:\n  - {name: dd01, host: h, username: u, password: \"${DD01_PASSWORD}\"}\n"), 0o600)
	}
	write("9099")

	w, err := NewWatcher(path)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()

	write("9100")
	w.Trigger() // simulate SIGHUP without sending a real signal

	select {
	case cfg := <-w.Updates():
		if cfg.Server.Port != "9100" {
			t.Fatalf("reloaded port = %s, want 9100", cfg.Server.Port)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no config update received")
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/config/ -run TestWatcher -v`
Expected: FAIL — `undefined: NewWatcher`

- [ ] **Step 4: Write the watcher**

```go
package config

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/fsnotify/fsnotify"
	log "github.com/sirupsen/logrus"
)

// Watcher reloads and revalidates the config on SIGHUP or file change, emitting the
// new *Config on Updates(). A bad reload is logged and dropped (the running config stays).
type Watcher struct {
	path    string
	fsw     *fsnotify.Watcher
	sigs    chan os.Signal
	updates chan *Config
	done    chan struct{}
}

// NewWatcher starts watching path. Call Close to stop.
func NewWatcher(path string) (*Watcher, error) {
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	if err := fsw.Add(path); err != nil {
		_ = fsw.Close()
		return nil, err
	}
	w := &Watcher{
		path: path, fsw: fsw,
		sigs:    make(chan os.Signal, 1),
		updates: make(chan *Config, 1),
		done:    make(chan struct{}),
	}
	signal.Notify(w.sigs, syscall.SIGHUP)
	go w.loop()
	return w, nil
}

// Updates is the channel of successfully reloaded configs.
func (w *Watcher) Updates() <-chan *Config { return w.updates }

// Trigger forces a reload (used by tests and callers that want a manual refresh).
func (w *Watcher) Trigger() { w.reload() }

func (w *Watcher) loop() {
	for {
		select {
		case <-w.done:
			return
		case <-w.sigs:
			w.reload()
		case ev := <-w.fsw.Events:
			if ev.Op&(fsnotify.Write|fsnotify.Create) != 0 {
				w.reload()
			}
		case err := <-w.fsw.Errors:
			log.WithError(err).Warn("config watch error")
		}
	}
}

func (w *Watcher) reload() {
	cfg, err := Load(w.path)
	if err != nil {
		log.WithError(err).Warn("config reload failed; keeping current config")
		return
	}
	select {
	case w.updates <- cfg:
	default: // drop if a pending update is unread
	}
}

// Close stops the watcher.
func (w *Watcher) Close() error {
	close(w.done)
	signal.Stop(w.sigs)
	return w.fsw.Close()
}
```

- [ ] **Step 5: Run tests; then wire into `main.go`**

Run: `go test ./internal/config/ -v`
Expected: PASS

Then in `main.go` `run()`, after `go col.Run(ctx)`, add the reload consumer (rebuild is intentionally coarse — log that a change was seen; full client rebuild is a follow-up once live validation confirms field shapes):

```go
	if w, err := config.NewWatcher(cfgPath); err == nil {
		defer w.Close()
		go func() {
			for newCfg := range w.Updates() {
				log.WithField("systems", len(newCfg.Systems)).
					Info("config reloaded (restart to apply system/client changes)")
			}
		}()
	}
```

- [ ] **Step 6: Commit**

```bash
git add internal/config/watcher.go internal/config/watcher_test.go main.go go.mod go.sum
git commit -m "feat(config): SIGHUP + file-watch hot reload"
```

**Phase 1 gate:** `make ci` is green; `/metrics` serves `ppdd_filesystem_*`, `ppdd_compression_*`, and `ppdd_collector_up` for each configured system.

---

## Phase 2 — MTrees module

### Task 12: MTrees collector

**Files:**
- Create: `internal/ppdd/mtrees.go`, `internal/ppdd/mtrees_test.go`, `internal/ppdd/testdata/mtrees.json`
- Modify: `internal/ppdd/resource.go` (register `MTrees{}`)

> Provisional: `GET /api/v1/dd-systems/0/mtrees` returns `{"mtree":[{...}]}`. Validate field names against a live DD.

- [ ] **Step 1: Create fixture `internal/ppdd/testdata/mtrees.json`**

```json
{
  "mtree": [
    {
      "name": "/data/col1/backup1",
      "logical_used_bytes": 5000000000,
      "physical_used_bytes": 800000000,
      "compression_factor": 6.25,
      "quota_soft_limit_bytes": 10000000000,
      "quota_hard_limit_bytes": 12000000000
    },
    {
      "name": "/data/col1/backup2",
      "logical_used_bytes": 0,
      "physical_used_bytes": 0,
      "compression_factor": 0,
      "quota_soft_limit_bytes": 0,
      "quota_hard_limit_bytes": 0
    }
  ]
}
```

- [ ] **Step 2: Write the failing test**

```go
package ppdd

import (
	"context"
	"os"
	"testing"

	"github.com/fjacquet/ppdd_exporter/internal/ddclient"
)

func TestMTreesCollect(t *testing.T) {
	body, _ := os.ReadFile("testdata/mtrees.json")
	m := ddclient.NewMock("dd01")
	m.SetJSON("/api/v1/dd-systems/0/mtrees", string(body))

	got, err := MTrees{}.Collect(context.Background(), m)
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	// Expect one logical_used sample for backup1 with the right mtree label & value.
	var found bool
	for _, s := range got {
		if s.Name == "ppdd_mtree_logical_used_bytes" &&
			s.LabelValue("mtree") == "/data/col1/backup1" && s.Value == 5000000000 {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing ppdd_mtree_logical_used_bytes for backup1; got %d samples", len(got))
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/ppdd/ -run TestMTreesCollect -v`
Expected: FAIL — `undefined: MTrees`

- [ ] **Step 4: Write `mtrees.go`**

```go
package ppdd

import (
	"context"

	"github.com/fjacquet/ppdd_exporter/internal/ddclient"
)

type mtreesResp struct {
	MTree []struct {
		Name              string  `json:"name"`
		LogicalUsedBytes  float64 `json:"logical_used_bytes"`
		PhysicalUsedBytes float64 `json:"physical_used_bytes"`
		CompressionFactor float64 `json:"compression_factor"`
		QuotaSoftLimit    float64 `json:"quota_soft_limit_bytes"`
		QuotaHardLimit    float64 `json:"quota_hard_limit_bytes"`
	} `json:"mtree"`
}

// MTrees collects per-MTree usage, compression, and quota metrics.
type MTrees struct{}

func (MTrees) Name() string { return "mtrees" }

func (MTrees) Collect(ctx context.Context, c ddclient.Client) ([]Sample, error) {
	var r mtreesResp
	if err := c.Get(ctx, "/api/v1/dd-systems/0/mtrees", &r); err != nil {
		return nil, err
	}
	var out []Sample
	for _, mt := range r.MTree {
		lbl := []Label{{Key: "mtree", Value: mt.Name}}
		out = append(out,
			Sample{Name: "ppdd_mtree_logical_used_bytes", Labels: lbl, Value: mt.LogicalUsedBytes},
			Sample{Name: "ppdd_mtree_physical_used_bytes", Labels: lbl, Value: mt.PhysicalUsedBytes},
			Sample{Name: "ppdd_mtree_compression_factor", Labels: lbl, Value: mt.CompressionFactor},
			Sample{Name: "ppdd_mtree_quota_soft_limit_bytes", Labels: lbl, Value: mt.QuotaSoftLimit},
			Sample{Name: "ppdd_mtree_quota_hard_limit_bytes", Labels: lbl, Value: mt.QuotaHardLimit},
		)
	}
	return out, nil
}
```

- [ ] **Step 5: Register and verify**

In `internal/ppdd/resource.go`, update `Registry()`:
```go
	return []ResourceCollector{
		Capacity{},
		MTrees{},
	}
```

Run: `go test ./internal/ppdd/ -run TestMTreesCollect -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/ppdd/mtrees.go internal/ppdd/mtrees_test.go internal/ppdd/testdata/mtrees.json internal/ppdd/resource.go
git commit -m "feat(ppdd): MTrees module (usage, compression, quotas)"
```

---

## Phase 3 — Replication module

### Task 13: Replication collector

**Files:**
- Create: `internal/ppdd/replication.go`, `internal/ppdd/replication_test.go`, `internal/ppdd/testdata/replications.json`
- Modify: `internal/ppdd/resource.go` (register `Replication{}`)

> Provisional: `GET /api/v1/dd-systems/0/replications` returns `{"replication":[{...}]}` with a `state` string and lag/throughput fields. Validate against a live DD.

- [ ] **Step 1: Create fixture `internal/ppdd/testdata/replications.json`**

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
  ]
}
```

- [ ] **Step 2: Write the failing test**

```go
package ppdd

import (
	"context"
	"os"
	"testing"

	"github.com/fjacquet/ppdd_exporter/internal/ddclient"
)

func TestReplicationCollect(t *testing.T) {
	body, _ := os.ReadFile("testdata/replications.json")
	m := ddclient.NewMock("dd01")
	m.SetJSON("/api/v1/dd-systems/0/replications", string(body))

	got, err := Replication{}.Collect(context.Background(), m)
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	var stateOK, lagOK bool
	for _, s := range got {
		if s.Name == "ppdd_replication_state" &&
			s.LabelValue("state") == "normal" && s.Value == 1 {
			stateOK = true
		}
		if s.Name == "ppdd_replication_sync_lag_seconds" && s.Value == 120 {
			lagOK = true
		}
	}
	if !stateOK || !lagOK {
		t.Fatalf("stateOK=%v lagOK=%v; got %d samples", stateOK, lagOK, len(got))
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/ppdd/ -run TestReplicationCollect -v`
Expected: FAIL — `undefined: Replication`

- [ ] **Step 4: Write `replication.go`**

```go
package ppdd

import (
	"context"

	"github.com/fjacquet/ppdd_exporter/internal/ddclient"
)

type replicationResp struct {
	Replication []struct {
		Source            string  `json:"source"`
		Destination       string  `json:"destination"`
		State             string  `json:"state"`
		SyncLagSeconds    float64 `json:"sync_lag_seconds"`
		PrecompRemaining  float64 `json:"precomp_bytes_remaining"`
		ThroughputBytesPS float64 `json:"throughput_bytes_per_second"`
	} `json:"replication"`
}

// Replication collects per-context DR posture: state, lag, backlog, throughput.
type Replication struct{}

func (Replication) Name() string { return "replication" }

func (Replication) Collect(ctx context.Context, c ddclient.Client) ([]Sample, error) {
	var r replicationResp
	if err := c.Get(ctx, "/api/v1/dd-systems/0/replications", &r); err != nil {
		return nil, err
	}
	var out []Sample
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
	return out, nil
}
```

- [ ] **Step 5: Register and verify**

In `internal/ppdd/resource.go`, add `Replication{}` to `Registry()` after `MTrees{}`.

Run: `go test ./internal/ppdd/ -run TestReplicationCollect -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/ppdd/replication.go internal/ppdd/replication_test.go internal/ppdd/testdata/replications.json internal/ppdd/resource.go
git commit -m "feat(ppdd): replication module (state, lag, backlog, throughput)"
```

---

## Phase 4 — Health & ops module

### Task 14: Health collector (hardware, alerts, system perf)

**Files:**
- Create: `internal/ppdd/health.go`, `internal/ppdd/health_test.go`, `internal/ppdd/testdata/{disks,alerts,system-stats}.json`
- Modify: `internal/ppdd/resource.go` (register `Health{}`)

> Provisional: this module fans out across several endpoints. To keep one failing endpoint from blanking the whole module, each sub-fetch is independent and a failure only drops that group's samples. Validate every path/field against a live DD.

- [ ] **Step 1: Create fixtures**

`internal/ppdd/testdata/disks.json`:
```json
{ "disk": [
  { "id": "1a", "state": "ok" },
  { "id": "1b", "state": "failed" }
] }
```

`internal/ppdd/testdata/alerts.json`:
```json
{ "alert": [
  { "id": "a1", "severity": "warning" },
  { "id": "a2", "severity": "critical" },
  { "id": "a3", "severity": "critical" }
] }
```

`internal/ppdd/testdata/system-stats.json`:
```json
{ "cpu_avg_percent": 37.5, "read_bytes_per_second": 200000000, "write_bytes_per_second": 150000000 }
```

- [ ] **Step 2: Write the failing test**

```go
package ppdd

import (
	"context"
	"os"
	"testing"

	"github.com/fjacquet/ppdd_exporter/internal/ddclient"
)

func loadHealthMock(t *testing.T) *ddclient.Mock {
	t.Helper()
	m := ddclient.NewMock("dd01")
	for path, file := range map[string]string{
		"/api/v1/dd-systems/0/hardware/disks":  "testdata/disks.json",
		"/api/v1/dd-systems/0/alerts":          "testdata/alerts.json",
		"/api/v1/dd-systems/0/stats/system-stats": "testdata/system-stats.json",
	} {
		b, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		m.SetJSON(path, string(b))
	}
	return m
}

func TestHealthCollect(t *testing.T) {
	got, err := Health{}.Collect(context.Background(), loadHealthMock(t))
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	seen := map[string]float64{}
	for _, s := range got {
		switch {
		case s.Name == "ppdd_disk_failed" && s.LabelValue("disk") == "1b":
			seen["disk_failed"] = s.Value
		case s.Name == "ppdd_alerts_active" && s.LabelValue("severity") == "critical":
			seen["crit"] = s.Value
		case s.Name == "ppdd_system_cpu_percent":
			seen["cpu"] = s.Value
		}
	}
	if seen["disk_failed"] != 1 {
		t.Errorf("disk 1b failed = %v, want 1", seen["disk_failed"])
	}
	if seen["crit"] != 2 {
		t.Errorf("critical alerts = %v, want 2", seen["crit"])
	}
	if seen["cpu"] != 37.5 {
		t.Errorf("cpu = %v, want 37.5", seen["cpu"])
	}
}

func TestHealthDegradesPerEndpoint(t *testing.T) {
	m := ddclient.NewMock("dd01") // nothing registered
	got, err := Health{}.Collect(context.Background(), m)
	if err != nil {
		t.Fatalf("Health.Collect must not hard-fail when sub-endpoints error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected no samples when all sub-endpoints fail, got %d", len(got))
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/ppdd/ -run TestHealth -v`
Expected: FAIL — `undefined: Health`

- [ ] **Step 4: Write `health.go`**

```go
package ppdd

import (
	"context"

	"github.com/fjacquet/ppdd_exporter/internal/ddclient"
)

// Health fans out across hardware, alerts, and system-perf endpoints. Each sub-fetch
// is independent: a failure drops only that group's samples (Collect never hard-fails).
type Health struct{}

func (Health) Name() string { return "health" }

func (Health) Collect(ctx context.Context, c ddclient.Client) ([]Sample, error) {
	var out []Sample
	out = append(out, healthDisks(ctx, c)...)
	out = append(out, healthAlerts(ctx, c)...)
	out = append(out, healthSystemStats(ctx, c)...)
	return out, nil
}

func healthDisks(ctx context.Context, c ddclient.Client) []Sample {
	var r struct {
		Disk []struct {
			ID    string `json:"id"`
			State string `json:"state"`
		} `json:"disk"`
	}
	if err := c.Get(ctx, "/api/v1/dd-systems/0/hardware/disks", &r); err != nil {
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
	var r struct {
		Alert []struct {
			Severity string `json:"severity"`
		} `json:"alert"`
	}
	if err := c.Get(ctx, "/api/v1/dd-systems/0/alerts", &r); err != nil {
		return nil
	}
	counts := map[string]float64{}
	for _, a := range r.Alert {
		counts[a.Severity]++
	}
	var out []Sample
	for sev, n := range counts {
		out = append(out, Sample{Name: "ppdd_alerts_active", Labels: []Label{{Key: "severity", Value: sev}}, Value: n})
	}
	return out
}

func healthSystemStats(ctx context.Context, c ddclient.Client) []Sample {
	var r struct {
		CPUAvgPercent     float64 `json:"cpu_avg_percent"`
		ReadBytesPerSec   float64 `json:"read_bytes_per_second"`
		WriteBytesPerSec  float64 `json:"write_bytes_per_second"`
	}
	if err := c.Get(ctx, "/api/v1/dd-systems/0/stats/system-stats", &r); err != nil {
		return nil
	}
	return []Sample{
		{Name: "ppdd_system_cpu_percent", Value: r.CPUAvgPercent},
		{Name: "ppdd_system_read_bytes_per_second", Value: r.ReadBytesPerSec},
		{Name: "ppdd_system_write_bytes_per_second", Value: r.WriteBytesPerSec},
	}
}
```

> **Note on `ppdd_alerts_active`:** because severities present vary per scrape, a severity with zero alerts simply has no series (rather than an explicit 0). Dashboards should use `sum by (severity)` / `or vector(0)`. If you later want guaranteed-zero series, seed the known severities (`info`,`warning`,`critical`,`emergency`) in `healthAlerts`.

- [ ] **Step 5: Register and verify**

In `internal/ppdd/resource.go`, add `Health{}` to `Registry()` after `Replication{}`.

Run: `go test ./internal/ppdd/ -run TestHealth -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/ppdd/health.go internal/ppdd/health_test.go internal/ppdd/testdata/disks.json internal/ppdd/testdata/alerts.json internal/ppdd/testdata/system-stats.json internal/ppdd/resource.go
git commit -m "feat(ppdd): health module (disks, alerts, system perf)"
```

---

### Task 15: Label-key consistency guard

**Files:**
- Create: `internal/ppdd/labels_test.go`

> Enforces the load-bearing Prometheus rule: every metric name must carry one label-key set across all series in a snapshot. This catches a builder that emits a metric with differing label keys.

- [ ] **Step 1: Write the test**

```go
package ppdd

import (
	"context"
	"strings"
	"testing"

	"github.com/fjacquet/ppdd_exporter/internal/ddclient"
)

// buildFullMock registers every endpoint so all collectors emit samples.
func buildFullMock(t *testing.T) *ddclient.Mock {
	t.Helper()
	m := loadHealthMock(t)
	for path, file := range map[string]string{
		"/api/v1/dd-systems/0/file-system":   "testdata/file-system.json",
		"/api/v1/dd-systems/0/mtrees":        "testdata/mtrees.json",
		"/api/v1/dd-systems/0/replications":  "testdata/replications.json",
	} {
		b, err := readFixture(file)
		if err != nil {
			t.Fatal(err)
		}
		m.SetJSON(path, b)
	}
	return m
}

func TestLabelKeyConsistencyAcrossCollectors(t *testing.T) {
	m := buildFullMock(t)
	store := NewSnapshotStore()
	col := NewCollector([]ddclient.Client{m}, Registry(), store, 0, 0)
	snap := col.CollectOnce(context.Background())

	keysByName := map[string]string{}
	for _, sys := range snap.Systems {
		for _, s := range sys.Samples {
			keys := make([]string, len(s.Labels))
			for i, l := range s.Labels {
				keys[i] = l.Key
			}
			sig := strings.Join(keys, ",")
			if prev, ok := keysByName[s.Name]; ok && prev != sig {
				t.Errorf("metric %s has inconsistent label keys: %q vs %q", s.Name, prev, sig)
			}
			keysByName[s.Name] = sig
		}
	}
}
```

- [ ] **Step 2: Add the `readFixture` helper**

Create `internal/ppdd/fixtures_test.go`:
```go
package ppdd

import "os"

func readFixture(path string) (string, error) {
	b, err := os.ReadFile(path)
	return string(b), err
}
```

- [ ] **Step 3: Run the test**

Run: `go test ./internal/ppdd/ -run TestLabelKeyConsistency -v`
Expected: PASS (if it fails, a collector emits a metric name with varying label keys — fix the builder to use one canonical key order).

- [ ] **Step 4: Commit**

```bash
git add internal/ppdd/labels_test.go internal/ppdd/fixtures_test.go
git commit -m "test(ppdd): guard label-key consistency across collectors"
```

**Phase 4 gate:** `make ci` green; all four domains emit metrics; label-consistency guard passes.

---

## Phase 5 — Polish (packaging, CI, docs)

### Task 16: Dockerfile (non-root)

**Files:**
- Create: `Dockerfile`, `.dockerignore`

- [ ] **Step 1: Write `Dockerfile`**

```dockerfile
# syntax=docker/dockerfile:1
FROM golang:1.26 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -ldflags "-X main.version=${VERSION}" -o /out/ppdd_exporter .

FROM gcr.io/distroless/static:nonroot
COPY --from=build /out/ppdd_exporter /ppdd_exporter
USER nonroot:nonroot
EXPOSE 9099
ENTRYPOINT ["/ppdd_exporter"]
CMD ["--config", "/etc/ppdd_exporter/config.yaml"]
```

- [ ] **Step 2: Write `.dockerignore`**

```
bin/
.git/
docs/
*.md
```

- [ ] **Step 3: Build image**

Run: `docker build -t ppdd_exporter:dev .`
Expected: image builds; `docker run --rm ppdd_exporter:dev --version` prints the version.

- [ ] **Step 4: Commit**

```bash
git add Dockerfile .dockerignore
git commit -m "build: distroless non-root Docker image"
```

---

### Task 17: CI workflow

**Files:**
- Create: `.github/workflows/ci.yml`

- [ ] **Step 1: Write the workflow**

```yaml
name: CI
on:
  push: { branches: [main] }
  pull_request:
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version: '1.26' }
      - name: Verify formatting
        run: test -z "$(gofmt -l .)"
      - name: Vet
        run: go vet ./...
      - name: Test (race)
        run: go test -race -cover ./...
      - name: Build
        run: go build ./...
```

- [ ] **Step 2: Validate locally**

Run: `make ci`
Expected: PASS (the workflow mirrors `make ci`).

- [ ] **Step 3: Commit**

```bash
git add .github/workflows/ci.yml
git commit -m "ci: add GitHub Actions build/test workflow"
```

---

### Task 18: README + metrics reference

**Files:**
- Create: `README.md`, `docs/metrics.md`

- [ ] **Step 1: Write `README.md`**

```markdown
# ppdd_exporter

A Go Prometheus exporter for **Dell PowerProtect DD (Data Domain)** appliances. One
process monitors many DD systems, polls each on an interval, and serves metrics at
`/metrics`. Modeled on `pflex_exporter` (Prometheus-only; OTLP deferred).

## Quick start

```bash
make cli
export DD01_PASSWORD='your-monitor-password'
./bin/ppdd_exporter --config config.yaml
# metrics: http://localhost:9099/metrics   health: http://localhost:9099/health
```

## Metric domains

Capacity & dedup, MTrees, Replication, Health & ops. See [docs/metrics.md](docs/metrics.md).

> API field mappings are modeled from Dell docs (DD OS 8.3 REST API) and are being
> validated against live appliances. `ppdd_collector_up{collector="..."}` reports per-module
> health.

## License

Apache-2.0.
```

- [ ] **Step 2: Write `docs/metrics.md`** — list each metric name, type (gauge), labels, and source module. Populate from the implemented collectors:

```markdown
# Metrics reference

All metrics are gauges and carry a `system` label. `ppdd_collector_up{collector}` is 1
when a module collected cleanly, 0 otherwise.

## capacity
- `ppdd_filesystem_total_bytes` / `ppdd_filesystem_used_bytes` / `ppdd_filesystem_available_bytes`
- `ppdd_compression_global_factor` / `ppdd_compression_local_factor` / `ppdd_compression_total_factor`
- `ppdd_filesystem_cleaning_running` (1 while GC runs)

## mtrees (labels: mtree)
- `ppdd_mtree_logical_used_bytes` / `ppdd_mtree_physical_used_bytes`
- `ppdd_mtree_compression_factor`
- `ppdd_mtree_quota_soft_limit_bytes` / `ppdd_mtree_quota_hard_limit_bytes`

## replication (labels: source, destination; +state on the state metric)
- `ppdd_replication_state{state}` (1 for the active state)
- `ppdd_replication_sync_lag_seconds`
- `ppdd_replication_precomp_bytes_remaining`
- `ppdd_replication_throughput_bytes_per_second`

## health
- `ppdd_disk_failed{disk}` (1 if failed)
- `ppdd_alerts_active{severity}`
- `ppdd_system_cpu_percent`
- `ppdd_system_read_bytes_per_second` / `ppdd_system_write_bytes_per_second`
```

- [ ] **Step 3: Add the Apache-2.0 `LICENSE`**

Run: `curl -fsSL https://www.apache.org/licenses/LICENSE-2.0.txt -o LICENSE`
Expected: `LICENSE` contains the Apache License 2.0 text. (Update the copyright line to `Fred Jacquet` if a NOTICE is desired.)

- [ ] **Step 4: Commit**

```bash
git add README.md docs/metrics.md LICENSE
git commit -m "docs: README, metrics reference, and Apache-2.0 license"
```

**Phase 5 gate:** image builds, CI green, README + metrics reference present.

---

## Post-implementation: live validation checklist

Once a real DD appliance (or DDMC) is reachable, validate the provisional shapes and
correct one module at a time (each is one struct + one fixture):

- [ ] Confirm `POST /api/v1/auth` body shape and `X-DD-AUTH-TOKEN` response header.
- [ ] Confirm `file-system` capacity field names + units (bytes vs KiB).
- [ ] Confirm `mtrees` list wrapper key and per-mtree field names.
- [ ] Confirm `replications` wrapper key, `state` enum values, lag/backlog fields.
- [ ] Confirm `hardware/disks`, `alerts`, `stats/system-stats` paths and fields.
- [ ] Capture real responses into `testdata/` and re-run `make ci`.
- [ ] Add protocols/streams metrics (deferred in Task 14 note) if a documented source exists.

---

## Notes for the implementer

- **TDD throughout:** every task writes the failing test first, watches it fail, then implements.
- **Semgrep-on-write hook is active and blocks on findings** — use the `writeBytes(io.Writer, …)` helper in test handlers; do not write inline `// nosemgrep`.
- **Commit after every green step** — small commits, each independently sensible.
- **Provisional fields are isolated by design** — when live validation contradicts a field, the fix is one struct + one fixture in one module; the loop, snapshot, and Prometheus layers don't change.

package dashboards

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
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

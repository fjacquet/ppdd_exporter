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
		"ppdd_mtree_logical_used_bytes":     5000000000,
		"ppdd_mtree_compression_factor":     6.25,
		"ppdd_mtree_physical_used_bytes":    800000000,
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

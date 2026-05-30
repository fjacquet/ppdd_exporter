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

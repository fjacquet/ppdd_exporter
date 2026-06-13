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
		pathSystem:      "testdata/system.json",
		pathFileSystem:  "testdata/file-systems.json",
		pathMTrees:      "testdata/mtrees.json",
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

func TestAlertsPaginationCollectsAllPages(t *testing.T) {
	m := ddclient.NewMock("dd01")
	// page_size 2, total 3 -> two pages. is_active=true is appended by the collector.
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

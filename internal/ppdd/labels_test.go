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
		"/api/v1/dd-systems/0/file-system":  "testdata/file-system.json",
		"/api/v1/dd-systems/0/mtrees":       "testdata/mtrees.json",
		"/api/v1/dd-systems/0/replications": "testdata/replications.json",
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

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

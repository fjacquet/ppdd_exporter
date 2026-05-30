package ppdd

import (
	"context"
	"os"
	"testing"

	"github.com/fjacquet/ppdd_exporter/internal/ddclient"
)

func TestReplicationCollect(t *testing.T) {
	body, err := os.ReadFile("testdata/replications.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
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

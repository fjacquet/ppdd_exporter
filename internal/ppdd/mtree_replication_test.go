package ppdd

import (
	"context"
	"os"
	"testing"

	"github.com/fjacquet/ppdd_exporter/internal/ddclient"
)

func TestMTreeReplicationCollect(t *testing.T) {
	body, err := os.ReadFile("testdata/mtree-replications.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	m := ddclient.NewMock("dd01")
	m.SetJSON(pathMTreeReplication, string(body))

	got, err := MTreeReplication{}.Collect(context.Background(), m)
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	var stateOK, connOK, enabledOK bool
	for _, s := range got {
		if s.Name == "ppdd_mtree_replication_state" &&
			s.LabelValue("state") == "NORMAL" &&
			s.LabelValue("source") == "dd01" &&
			s.LabelValue("destination") == "dd02" && s.Value == 1 {
			stateOK = true
		}
		if s.Name == "ppdd_mtree_replication_connected" && s.Value == 1 {
			connOK = true
		}
		if s.Name == "ppdd_mtree_replication_enabled" && s.Value == 1 {
			enabledOK = true
		}
	}
	if !stateOK || !connOK || !enabledOK {
		t.Fatalf("stateOK=%v connOK=%v enabledOK=%v; got %d samples", stateOK, connOK, enabledOK, len(got))
	}
}

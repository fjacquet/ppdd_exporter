package ppdd

import (
	"context"
	"os"
	"testing"

	"github.com/fjacquet/ppdd_exporter/internal/ddclient"
)

func TestFileReplicationCollect(t *testing.T) {
	body, err := os.ReadFile("testdata/file-replications.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	m := ddclient.NewMock("dd01")
	m.SetJSON(pathFileReplication, string(body))

	got, err := FileReplication{}.Collect(context.Background(), m)
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	seen := map[string]float64{}
	var statusOK bool
	for _, s := range got {
		if s.LabelValue("context") == "ctx1" {
			seen[s.Name] = s.Value
		}
		if s.Name == "ppdd_file_replication_status" && s.LabelValue("status") == "completed" && s.Value == 1 {
			statusOK = true
		}
	}
	if seen["ppdd_file_replication_network_bytes"] != 1048576 {
		t.Errorf("network_bytes = %v, want 1048576", seen["ppdd_file_replication_network_bytes"])
	}
	if seen["ppdd_file_replication_active_files"] != 3 {
		t.Errorf("active_files = %v, want 3", seen["ppdd_file_replication_active_files"])
	}
	if !statusOK {
		t.Errorf("expected status sample for completed=1")
	}
}

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

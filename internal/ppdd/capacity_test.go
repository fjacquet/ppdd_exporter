package ppdd

import (
	"context"
	"os"
	"testing"

	"github.com/fjacquet/ppdd_exporter/internal/ddclient"
)

func TestCapacityCollect(t *testing.T) {
	body, err := os.ReadFile("testdata/file-system.json")
	if err != nil {
		t.Fatal(err)
	}
	m := ddclient.NewMock("dd01")
	m.SetJSON("/api/v1/dd-systems/0/file-system", string(body))

	got, err := Capacity{}.Collect(context.Background(), m)
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	want := map[string]float64{
		"ppdd_filesystem_total_bytes":      100000000000,
		"ppdd_filesystem_used_bytes":       40000000000,
		"ppdd_filesystem_available_bytes":  60000000000,
		"ppdd_compression_global_factor":   5.5,
		"ppdd_compression_local_factor":    1.8,
		"ppdd_compression_total_factor":    9.9,
		"ppdd_filesystem_cleaning_running": 1,
	}
	seen := map[string]float64{}
	for _, s := range got {
		seen[s.Name] = s.Value
	}
	for name, v := range want {
		if seen[name] != v {
			t.Errorf("%s = %v, want %v", name, seen[name], v)
		}
	}
}

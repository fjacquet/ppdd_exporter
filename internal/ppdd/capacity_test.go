package ppdd

import (
	"context"
	"os"
	"testing"

	"github.com/fjacquet/ppdd_exporter/internal/ddclient"
)

func TestCapacityCollect(t *testing.T) {
	m := ddclient.NewMock("dd01")
	for path, file := range map[string]string{
		pathSystem:     "testdata/system.json",
		pathFileSystem: "testdata/file-system.json",
	} {
		b, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		m.SetJSON(path, string(b))
	}

	got, err := Capacity{}.Collect(context.Background(), m)
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	want := map[string]float64{
		"ppdd_filesystem_total_bytes":      100000000000,
		"ppdd_filesystem_used_bytes":       40000000000,
		"ppdd_filesystem_available_bytes":  60000000000,
		"ppdd_compression_factor":          9.9,
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

func TestCapacityProvisionalIsBestEffort(t *testing.T) {
	m := ddclient.NewMock("dd01")
	b, err := os.ReadFile("testdata/system.json")
	if err != nil {
		t.Fatal(err)
	}
	m.SetJSON(pathSystem, string(b)) // no file-system registered

	got, err := Capacity{}.Collect(context.Background(), m)
	if err != nil {
		t.Fatalf("Collect must not fail when provisional /file-system is absent: %v", err)
	}
	var hasDocumented bool
	for _, s := range got {
		if s.Name == "ppdd_filesystem_total_bytes" && s.Value == 100000000000 {
			hasDocumented = true
		}
	}
	if !hasDocumented {
		t.Fatal("documented capacity samples missing when only /system is available")
	}
}

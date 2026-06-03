package ppdd

import (
	"context"
	"os"
	"testing"

	"github.com/fjacquet/ppdd_exporter/internal/ddclient"
)

func loadHealthMock(t *testing.T) *ddclient.Mock {
	t.Helper()
	m := ddclient.NewMock("dd01")
	for path, file := range map[string]string{
		pathDisks:       "testdata/disks.json",
		pathAlerts:      "testdata/alerts.json",
		pathSystemStats: "testdata/system-stats.json",
	} {
		b, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		m.SetJSON(path, string(b))
	}
	return m
}

func TestHealthCollect(t *testing.T) {
	got, err := Health{}.Collect(context.Background(), loadHealthMock(t))
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	seen := map[string]float64{}
	for _, s := range got {
		switch {
		case s.Name == "ppdd_disk_failed" && s.LabelValue("disk") == "1b":
			seen["disk_failed"] = s.Value
		case s.Name == "ppdd_alerts_active" && s.LabelValue("severity") == "critical":
			seen["crit"] = s.Value
		case s.Name == "ppdd_system_cpu_percent":
			seen["cpu"] = s.Value
		}
	}
	if seen["disk_failed"] != 1 {
		t.Errorf("disk 1b failed = %v, want 1", seen["disk_failed"])
	}
	if seen["crit"] != 2 {
		t.Errorf("critical alerts = %v, want 2", seen["crit"])
	}
	if seen["cpu"] != 37.5 {
		t.Errorf("cpu = %v, want 37.5", seen["cpu"])
	}
}

func TestHealthDegradesPerEndpoint(t *testing.T) {
	m := ddclient.NewMock("dd01") // nothing registered
	got, err := Health{}.Collect(context.Background(), m)
	if err != nil {
		t.Fatalf("Health.Collect must not hard-fail when sub-endpoints error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected no samples when all sub-endpoints fail, got %d", len(got))
	}
}

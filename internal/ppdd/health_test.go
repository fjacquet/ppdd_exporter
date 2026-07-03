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
		pathPerformance: "testdata/performance.json",
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
	disks := map[string]int{} // device -> series count, to catch label collisions
	for _, s := range got {
		switch {
		case s.Name == "ppdd_disk_failed":
			disks[s.LabelValue("disk")]++
			seen["disk_"+s.LabelValue("disk")] = s.Value
		case s.Name == "ppdd_alerts_active" && s.LabelValue("severity") == "CRITICAL" && s.LabelValue("class") == "HardwareFailure":
			seen["crit"] = s.Value
		case s.Name == "ppdd_system_cpu_percent":
			seen["cpu"] = s.Value
		}
	}
	// device is the unique key: disks 1.1 and 2.1 share id "1" but must not collide.
	if len(disks) != 2 || disks["1.1"] != 1 || disks["2.1"] != 1 {
		t.Errorf("disk series = %v, want one each for 1.1 and 2.1", disks)
	}
	if seen["disk_2.1"] != 1 {
		t.Errorf("disk 2.1 failed = %v, want 1", seen["disk_2.1"])
	}
	if seen["disk_1.1"] != 0 {
		t.Errorf("disk 1.1 failed = %v, want 0", seen["disk_1.1"])
	}
	if seen["crit"] != 2 {
		t.Errorf("critical alerts = %v, want 2", seen["crit"])
	}
	if seen["cpu"] != 38 {
		t.Errorf("cpu = %v, want 38", seen["cpu"])
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

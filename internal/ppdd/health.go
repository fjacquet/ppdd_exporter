package ppdd

import (
	"context"

	"github.com/fjacquet/ppdd_exporter/internal/ddclient"
)

// Health fans out across hardware, alerts, and system-perf endpoints. Each sub-fetch
// is independent: a failure drops only that group's samples (Collect never hard-fails).
type Health struct{}

func (Health) Name() string { return "health" }

func (Health) Collect(ctx context.Context, c ddclient.Client) ([]Sample, error) {
	var out []Sample
	out = append(out, healthDisks(ctx, c)...)
	out = append(out, healthAlerts(ctx, c)...)
	out = append(out, healthSystemStats(ctx, c)...)
	return out, nil
}

func healthDisks(ctx context.Context, c ddclient.Client) []Sample {
	var r struct {
		Disk []struct {
			ID    string `json:"id"`
			State string `json:"state"`
		} `json:"disk"`
	}
	if err := c.Get(ctx, "/api/v1/dd-systems/0/hardware/disks", &r); err != nil {
		return nil
	}
	var out []Sample
	for _, d := range r.Disk {
		failed := 0.0
		if d.State == "failed" {
			failed = 1
		}
		out = append(out, Sample{Name: "ppdd_disk_failed", Labels: []Label{{Key: "disk", Value: d.ID}}, Value: failed})
	}
	return out
}

func healthAlerts(ctx context.Context, c ddclient.Client) []Sample {
	var r struct {
		Alert []struct {
			Severity string `json:"severity"`
		} `json:"alert"`
	}
	if err := c.Get(ctx, "/api/v1/dd-systems/0/alerts", &r); err != nil {
		return nil
	}
	counts := map[string]float64{}
	for _, a := range r.Alert {
		counts[a.Severity]++
	}
	var out []Sample
	for sev, n := range counts {
		out = append(out, Sample{Name: "ppdd_alerts_active", Labels: []Label{{Key: "severity", Value: sev}}, Value: n})
	}
	return out
}

func healthSystemStats(ctx context.Context, c ddclient.Client) []Sample {
	var r struct {
		CPUAvgPercent    float64 `json:"cpu_avg_percent"`
		ReadBytesPerSec  float64 `json:"read_bytes_per_second"`
		WriteBytesPerSec float64 `json:"write_bytes_per_second"`
	}
	if err := c.Get(ctx, "/api/v1/dd-systems/0/stats/system-stats", &r); err != nil {
		return nil
	}
	return []Sample{
		{Name: "ppdd_system_cpu_percent", Value: r.CPUAvgPercent},
		{Name: "ppdd_system_read_bytes_per_second", Value: r.ReadBytesPerSec},
		{Name: "ppdd_system_write_bytes_per_second", Value: r.WriteBytesPerSec},
	}
}

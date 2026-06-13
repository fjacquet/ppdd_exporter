package ppdd

import (
	"context"
	"encoding/json"

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
	out = append(out, healthSystemPerformance(ctx, c)...)
	return out, nil
}

func healthDisks(ctx context.Context, c ddclient.Client) []Sample {
	var r struct {
		DiskInfo []struct {
			ID     string `json:"id"`
			Status string `json:"status"` // enum DiskStatusEnum; FAILED == failed
		} `json:"diskInfo"`
	}
	if err := c.Get(ctx, pathDisks, &r); err != nil {
		return nil
	}
	var out []Sample
	for _, d := range r.DiskInfo {
		failed := 0.0
		if d.Status == "FAILED" {
			failed = 1
		}
		out = append(out, Sample{Name: "ppdd_disk_failed", Labels: []Label{{Key: "disk", Value: d.ID}}, Value: failed})
	}
	return out
}

func healthAlerts(ctx context.Context, c ddclient.Client) []Sample {
	type alertKey struct{ severity, class string }
	counts := map[alertKey]float64{}
	err := paginate(ctx, c, pathAlerts, "is_active=true", func(page json.RawMessage) (pagingInfo, error) {
		var r struct {
			Alerts []struct {
				Severity string `json:"severity"`
				Class    string `json:"class"`
			} `json:"alert_list"`
			PagingInfo pagingInfo `json:"paging_info"`
		}
		if err := json.Unmarshal(page, &r); err != nil {
			return pagingInfo{}, err
		}
		for _, a := range r.Alerts {
			counts[alertKey{a.Severity, a.Class}]++
		}
		return r.PagingInfo, nil
	})
	if err != nil {
		return nil
	}
	var out []Sample
	for k, n := range counts {
		out = append(out, Sample{
			Name:   "ppdd_alerts_active",
			Labels: []Label{{Key: "severity", Value: k.severity}, {Key: "class", Value: k.class}},
			Value:  n,
		})
	}
	return out
}

// healthSystemPerformance reads the latest-epoch system performance sample
// (best-effort: a failure or empty series drops only these samples).
func healthSystemPerformance(ctx context.Context, c ddclient.Client) []Sample {
	var r struct {
		StatsPerformance []struct {
			CollectionEpoch       int64   `json:"collectionEpoch"`
			AverageCPUUtilization float64 `json:"averageCpuUtilization"`
			Throughput            struct {
				Read  float64 `json:"read"`
				Write float64 `json:"write"`
			} `json:"throughput"`
		} `json:"statsPerformance"`
	}
	if err := c.Get(ctx, pathPerformance, &r); err != nil || len(r.StatsPerformance) == 0 {
		return nil
	}
	latest := r.StatsPerformance[0]
	for _, s := range r.StatsPerformance[1:] {
		if s.CollectionEpoch > latest.CollectionEpoch {
			latest = s
		}
	}
	return []Sample{
		{Name: "ppdd_system_cpu_percent", Value: latest.AverageCPUUtilization},
		{Name: "ppdd_system_read_bytes_per_second", Value: latest.Throughput.Read},
		{Name: "ppdd_system_write_bytes_per_second", Value: latest.Throughput.Write},
	}
}

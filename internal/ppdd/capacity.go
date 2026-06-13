package ppdd

import (
	"context"

	"github.com/fjacquet/ppdd_exporter/internal/ddclient"
)

// systemResp is the documented shape of GET /rest/v1.0/system (guide pp.11-12).
type systemResp struct {
	PhysicalCapacity struct {
		Total     float64 `json:"total"`
		Used      float64 `json:"used"`
		Available float64 `json:"available"`
	} `json:"physical_capacity"`
	CompressionFactor float64 `json:"compression_factor"`
}

// fileSystemResp is the validated 8.7.0 /file-systems shape (schema filesysInfo).
// Only the GC clean state is consumed; fetched best-effort.
type fileSystemResp struct {
	CleanStatus string `json:"fs_clean_status"` // enum: clean|running|inactive|waiting
}

// Capacity collects filesystem capacity and compression from the documented
// /system endpoint; the GC clean state comes from the validated /file-systems
// endpoint, fetched best-effort.
type Capacity struct{}

func (Capacity) Name() string { return "capacity" }

func (Capacity) Collect(ctx context.Context, c ddclient.Client) ([]Sample, error) {
	var sys systemResp
	if err := c.Get(ctx, pathSystem, &sys); err != nil {
		return nil, err
	}
	out := []Sample{
		{Name: "ppdd_filesystem_total_bytes", Value: sys.PhysicalCapacity.Total},
		{Name: "ppdd_filesystem_used_bytes", Value: sys.PhysicalCapacity.Used},
		{Name: "ppdd_filesystem_available_bytes", Value: sys.PhysicalCapacity.Available},
		{Name: "ppdd_compression_factor", Value: sys.CompressionFactor},
	}
	return append(out, capacityProvisional(ctx, c)...), nil
}

// capacityProvisional fetches the /file-systems clean state (best-effort: a failure
// drops only this sample, never the whole collector).
func capacityProvisional(ctx context.Context, c ddclient.Client) []Sample {
	var fs fileSystemResp
	if err := c.Get(ctx, pathFileSystem, &fs); err != nil {
		return nil
	}
	cleaning := 0.0
	if fs.CleanStatus == "running" {
		cleaning = 1
	}
	return []Sample{
		{Name: "ppdd_filesystem_cleaning_running", Value: cleaning},
	}
}

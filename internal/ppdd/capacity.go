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

// fileSystemResp is PROVISIONAL (not in the 7.3 guide). Retained only for the
// compression split and GC cleaning state, fetched best-effort.
type fileSystemResp struct {
	GlobalCompression float64 `json:"global_compression_factor"`
	LocalCompression  float64 `json:"local_compression_factor"`
	TotalCompression  float64 `json:"total_compression_factor"`
	Cleaning          struct {
		Status string `json:"status"`
	} `json:"cleaning"`
}

// Capacity collects filesystem capacity and compression. Documented values come
// from /system; the compression split and GC state are provisional best-effort.
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

// capacityProvisional fetches the undocumented /file-system extras (best-effort:
// a failure drops only these samples, never the whole collector).
func capacityProvisional(ctx context.Context, c ddclient.Client) []Sample {
	var fs fileSystemResp
	if err := c.Get(ctx, pathFileSystem, &fs); err != nil {
		return nil
	}
	cleaning := 0.0
	if fs.Cleaning.Status == "running" {
		cleaning = 1
	}
	return []Sample{
		{Name: "ppdd_compression_global_factor", Value: fs.GlobalCompression},
		{Name: "ppdd_compression_local_factor", Value: fs.LocalCompression},
		{Name: "ppdd_compression_total_factor", Value: fs.TotalCompression},
		{Name: "ppdd_filesystem_cleaning_running", Value: cleaning},
	}
}

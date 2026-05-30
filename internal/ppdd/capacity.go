package ppdd

import (
	"context"

	"github.com/fjacquet/ppdd_exporter/internal/ddclient"
)

// fileSystemResp is the provisional shape of GET .../dd-systems/0/file-system.
type fileSystemResp struct {
	PhysicalCapacityBytes  float64 `json:"physical_capacity_bytes"`
	PhysicalUsedBytes      float64 `json:"physical_used_bytes"`
	PhysicalAvailableBytes float64 `json:"physical_available_bytes"`
	GlobalCompression      float64 `json:"global_compression_factor"`
	LocalCompression       float64 `json:"local_compression_factor"`
	TotalCompression       float64 `json:"total_compression_factor"`
	Cleaning               struct {
		Status string `json:"status"`
	} `json:"cleaning"`
}

// Capacity collects filesystem capacity, dedup/compression factors, and GC state.
type Capacity struct{}

func (Capacity) Name() string { return "capacity" }

func (Capacity) Collect(ctx context.Context, c ddclient.Client) ([]Sample, error) {
	var r fileSystemResp
	if err := c.Get(ctx, "/api/v1/dd-systems/0/file-system", &r); err != nil {
		return nil, err
	}
	cleaning := 0.0
	if r.Cleaning.Status == "running" {
		cleaning = 1
	}
	return []Sample{
		{Name: "ppdd_filesystem_total_bytes", Value: r.PhysicalCapacityBytes},
		{Name: "ppdd_filesystem_used_bytes", Value: r.PhysicalUsedBytes},
		{Name: "ppdd_filesystem_available_bytes", Value: r.PhysicalAvailableBytes},
		{Name: "ppdd_compression_global_factor", Value: r.GlobalCompression},
		{Name: "ppdd_compression_local_factor", Value: r.LocalCompression},
		{Name: "ppdd_compression_total_factor", Value: r.TotalCompression},
		{Name: "ppdd_filesystem_cleaning_running", Value: cleaning},
	}, nil
}

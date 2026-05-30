package ppdd

import (
	"context"

	"github.com/fjacquet/ppdd_exporter/internal/ddclient"
)

type mtreesResp struct {
	MTree []struct {
		Name              string  `json:"name"`
		LogicalUsedBytes  float64 `json:"logical_used_bytes"`
		PhysicalUsedBytes float64 `json:"physical_used_bytes"`
		CompressionFactor float64 `json:"compression_factor"`
		QuotaSoftLimit    float64 `json:"quota_soft_limit_bytes"`
		QuotaHardLimit    float64 `json:"quota_hard_limit_bytes"`
	} `json:"mtree"`
}

// MTrees collects per-MTree usage, compression, and quota metrics.
type MTrees struct{}

func (MTrees) Name() string { return "mtrees" }

func (MTrees) Collect(ctx context.Context, c ddclient.Client) ([]Sample, error) {
	var r mtreesResp
	if err := c.Get(ctx, "/api/v1/dd-systems/0/mtrees", &r); err != nil {
		return nil, err
	}
	var out []Sample
	for _, mt := range r.MTree {
		lbl := []Label{{Key: "mtree", Value: mt.Name}}
		out = append(out,
			Sample{Name: "ppdd_mtree_logical_used_bytes", Labels: lbl, Value: mt.LogicalUsedBytes},
			Sample{Name: "ppdd_mtree_physical_used_bytes", Labels: lbl, Value: mt.PhysicalUsedBytes},
			Sample{Name: "ppdd_mtree_compression_factor", Labels: lbl, Value: mt.CompressionFactor},
			Sample{Name: "ppdd_mtree_quota_soft_limit_bytes", Labels: lbl, Value: mt.QuotaSoftLimit},
			Sample{Name: "ppdd_mtree_quota_hard_limit_bytes", Labels: lbl, Value: mt.QuotaHardLimit},
		)
	}
	return out, nil
}

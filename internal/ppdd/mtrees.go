package ppdd

import (
	"context"
	"encoding/json"

	"github.com/fjacquet/ppdd_exporter/internal/ddclient"
)

// mtreeListItem is the documented v3.0 mtree metadata. Quota fields are
// PROVISIONAL (not shown in the guide's mtree sample) and kept best-effort.
type mtreeListItem struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	IsDegraded    string `json:"is_degraded"`
	MTreeRLDetail struct {
		RLStatus string `json:"rl_status"`
	} `json:"mtree_rl_detail"`
	QuotaSoftLimit float64 `json:"quota_soft_limit_bytes"` // PROVISIONAL
	QuotaHardLimit float64 `json:"quota_hard_limit_bytes"` // PROVISIONAL
}

// mtreeStatsResp is the documented v2.0 per-MTree capacity stats (guide pp.34-36).
type mtreeStatsResp struct {
	StatsCapacity []struct {
		CollectionEpoch   int64 `json:"collection_epoch"`
		TierCapacityUsage []struct {
			LogicalCapacity struct {
				Used float64 `json:"used"`
			} `json:"logical_capacity"`
		} `json:"tier_capacity_usage"`
		TierDataWritten []struct {
			CompressionFactor float64 `json:"compression_factor"`
			PostCompWritten   float64 `json:"post_comp_written"`
		} `json:"tier_data_written"`
	} `json:"stats_capacity"`
}

// MTrees collects per-MTree metadata/health (v3.0 list) and usage (v2.0 stats).
type MTrees struct{}

func (MTrees) Name() string { return "mtrees" }

func (MTrees) Collect(ctx context.Context, c ddclient.Client) ([]Sample, error) {
	var items []mtreeListItem
	err := paginate(ctx, c, pathMTrees, "", func(page json.RawMessage) (pagingInfo, error) {
		var r struct {
			MTree      []mtreeListItem `json:"mtree"`
			PagingInfo pagingInfo      `json:"paging_info"`
		}
		if err := json.Unmarshal(page, &r); err != nil {
			return pagingInfo{}, err
		}
		items = append(items, r.MTree...)
		return r.PagingInfo, nil
	})
	if err != nil {
		return nil, err
	}

	var out []Sample
	for _, mt := range items {
		lbl := []Label{{Key: "mtree", Value: mt.Name}}
		degraded := 0.0
		if mt.IsDegraded == "degraded" {
			degraded = 1
		}
		rl := 0.0
		switch mt.MTreeRLDetail.RLStatus {
		case "", "never-enabled", "disabled":
			// retention lock not active
		default:
			rl = 1
		}
		out = append(out,
			Sample{Name: "ppdd_mtree_degraded", Labels: lbl, Value: degraded},
			Sample{Name: "ppdd_mtree_retention_lock_enabled", Labels: lbl, Value: rl},
			Sample{Name: "ppdd_mtree_quota_soft_limit_bytes", Labels: lbl, Value: mt.QuotaSoftLimit},
			Sample{Name: "ppdd_mtree_quota_hard_limit_bytes", Labels: lbl, Value: mt.QuotaHardLimit},
		)
		out = append(out, mtreeUsage(ctx, c, mt)...)
	}
	return out, nil
}

// mtreeUsage fetches documented per-MTree usage from the latest collection_epoch
// (best-effort: a failure drops only this MTree's usage samples). N+1 requests —
// one per MTree; bounded concurrency is a future optimization.
func mtreeUsage(ctx context.Context, c ddclient.Client, mt mtreeListItem) []Sample {
	var r mtreeStatsResp
	if err := c.Get(ctx, mtreeStatsPath(mt.ID), &r); err != nil || len(r.StatsCapacity) == 0 {
		return nil
	}
	latest := r.StatsCapacity[0]
	for _, s := range r.StatsCapacity[1:] {
		if s.CollectionEpoch > latest.CollectionEpoch {
			latest = s
		}
	}
	var logicalUsed, comp, postComp float64
	for _, t := range latest.TierCapacityUsage {
		logicalUsed += t.LogicalCapacity.Used
	}
	// Aggregate compression across tiers, weighted by post-comp bytes: the global
	// factor is total pre-comp / total post-comp = Σ(cf_i·post_i) / Σ(post_i).
	var weightedComp float64
	for _, t := range latest.TierDataWritten {
		postComp += t.PostCompWritten
		weightedComp += t.CompressionFactor * t.PostCompWritten
	}
	if postComp > 0 {
		comp = weightedComp / postComp
	}
	lbl := []Label{{Key: "mtree", Value: mt.Name}}
	return []Sample{
		{Name: "ppdd_mtree_logical_used_bytes", Labels: lbl, Value: logicalUsed},
		{Name: "ppdd_mtree_compression_factor", Labels: lbl, Value: comp},
		{Name: "ppdd_mtree_physical_used_bytes", Labels: lbl, Value: postComp}, // PROVISIONAL: post_comp_written
	}
}

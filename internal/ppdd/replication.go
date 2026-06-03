package ppdd

import (
	"context"
	"encoding/json"

	"github.com/fjacquet/ppdd_exporter/internal/ddclient"
)

// Replication collects per-context DR posture. PROVISIONAL: this endpoint and its
// fields are not in the 7.3 guide; only the prefix/version and pagination are aligned.
type Replication struct{}

func (Replication) Name() string { return "replication" }

func (Replication) Collect(ctx context.Context, c ddclient.Client) ([]Sample, error) {
	var out []Sample
	err := paginate(ctx, c, pathReplication, "", func(page json.RawMessage) (pagingInfo, error) {
		var r struct {
			Replication []struct {
				Source            string  `json:"source"`
				Destination       string  `json:"destination"`
				State             string  `json:"state"`
				SyncLagSeconds    float64 `json:"sync_lag_seconds"`
				PrecompRemaining  float64 `json:"precomp_bytes_remaining"`
				ThroughputBytesPS float64 `json:"throughput_bytes_per_second"`
			} `json:"replication"`
			PagingInfo pagingInfo `json:"paging_info"`
		}
		if err := json.Unmarshal(page, &r); err != nil {
			return pagingInfo{}, err
		}
		for _, ctxn := range r.Replication {
			id := []Label{{Key: "source", Value: ctxn.Source}, {Key: "destination", Value: ctxn.Destination}}
			stateLbl := append([]Label{{Key: "state", Value: ctxn.State}}, id...)
			out = append(out,
				Sample{Name: "ppdd_replication_state", Labels: stateLbl, Value: 1},
				Sample{Name: "ppdd_replication_sync_lag_seconds", Labels: id, Value: ctxn.SyncLagSeconds},
				Sample{Name: "ppdd_replication_precomp_bytes_remaining", Labels: id, Value: ctxn.PrecompRemaining},
				Sample{Name: "ppdd_replication_throughput_bytes_per_second", Labels: id, Value: ctxn.ThroughputBytesPS},
			)
		}
		return r.PagingInfo, nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

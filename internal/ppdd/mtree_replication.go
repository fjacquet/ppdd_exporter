package ppdd

import (
	"context"
	"encoding/json"

	"github.com/fjacquet/ppdd_exporter/internal/ddclient"
)

// MTreeReplication collects per-context MTree replication posture (state, connection,
// resync need). Validated against the 8.7.0 OpenAPI (schema MtreeReplicationInfos).
// Throughput/lag are not on this endpoint; file-replication stats carry those.
type MTreeReplication struct{}

func (MTreeReplication) Name() string { return "mtree_replication" }

func (MTreeReplication) Collect(ctx context.Context, c ddclient.Client) ([]Sample, error) {
	var out []Sample
	err := paginate(ctx, c, pathMTreeReplication, "", func(page json.RawMessage) (pagingInfo, error) {
		var r struct {
			Contexts []struct {
				State           string `json:"state"`
				SourceHost      string `json:"sourceHost"`
				DestinationHost string `json:"destinationHost"`
				Enabled         bool   `json:"enabled"`
				NeedResync      bool   `json:"needResync"`
				Connected       bool   `json:"connected"`
			} `json:"contexts"`
			PagingInfo pagingInfo `json:"paging_info"`
		}
		if err := json.Unmarshal(page, &r); err != nil {
			return pagingInfo{}, err
		}
		for _, ctxn := range r.Contexts {
			id := []Label{{Key: "source", Value: ctxn.SourceHost}, {Key: "destination", Value: ctxn.DestinationHost}}
			stateLbl := append([]Label{{Key: "state", Value: ctxn.State}}, id...)
			out = append(out,
				Sample{Name: "ppdd_mtree_replication_state", Labels: stateLbl, Value: 1},
				Sample{Name: "ppdd_mtree_replication_enabled", Labels: id, Value: boolGauge(ctxn.Enabled)},
				Sample{Name: "ppdd_mtree_replication_connected", Labels: id, Value: boolGauge(ctxn.Connected)},
				Sample{Name: "ppdd_mtree_replication_need_resync", Labels: id, Value: boolGauge(ctxn.NeedResync)},
			)
		}
		return r.PagingInfo, nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

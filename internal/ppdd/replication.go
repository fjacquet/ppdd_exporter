package ppdd

import (
	"context"

	"github.com/fjacquet/ppdd_exporter/internal/ddclient"
)

type replicationResp struct {
	Replication []struct {
		Source            string  `json:"source"`
		Destination       string  `json:"destination"`
		State             string  `json:"state"`
		SyncLagSeconds    float64 `json:"sync_lag_seconds"`
		PrecompRemaining  float64 `json:"precomp_bytes_remaining"`
		ThroughputBytesPS float64 `json:"throughput_bytes_per_second"`
	} `json:"replication"`
}

// Replication collects per-context DR posture: state, lag, backlog, throughput.
type Replication struct{}

func (Replication) Name() string { return "replication" }

func (Replication) Collect(ctx context.Context, c ddclient.Client) ([]Sample, error) {
	var r replicationResp
	if err := c.Get(ctx, "/api/v1/dd-systems/0/replications", &r); err != nil {
		return nil, err
	}
	var out []Sample
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
	return out, nil
}

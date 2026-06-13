package ppdd

import (
	"context"
	"encoding/json"

	"github.com/fjacquet/ppdd_exporter/internal/ddclient"
)

// FileReplication collects per-context file-replication stats. Validated against the
// 8.7.0 OpenAPI (schema fileReplicationList). Label `context` is the opaque context id.
type FileReplication struct{}

func (FileReplication) Name() string { return "file_replication" }

func (FileReplication) Collect(ctx context.Context, c ddclient.Client) ([]Sample, error) {
	var out []Sample
	err := paginate(ctx, c, pathFileReplication, "", func(page json.RawMessage) (pagingInfo, error) {
		var r struct {
			Context []struct {
				ID                string  `json:"id"`
				ActiveFiles       float64 `json:"active_files"`
				LogicalReplicated float64 `json:"logical_replicated"`
				NetworkBytes      float64 `json:"network_bytes"`
				ReplStatus        string  `json:"repl_status"`
			} `json:"context"`
			PagingInfo pagingInfo `json:"paging_info"`
		}
		if err := json.Unmarshal(page, &r); err != nil {
			return pagingInfo{}, err
		}
		for _, fc := range r.Context {
			lbl := []Label{{Key: "context", Value: fc.ID}}
			statusLbl := append([]Label{{Key: "status", Value: fc.ReplStatus}}, lbl...)
			out = append(out,
				Sample{Name: "ppdd_file_replication_network_bytes", Labels: lbl, Value: fc.NetworkBytes},
				Sample{Name: "ppdd_file_replication_logical_replicated_bytes", Labels: lbl, Value: fc.LogicalReplicated},
				Sample{Name: "ppdd_file_replication_active_files", Labels: lbl, Value: fc.ActiveFiles},
				Sample{Name: "ppdd_file_replication_status", Labels: statusLbl, Value: 1},
			)
		}
		return r.PagingInfo, nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

package ppdd

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/fjacquet/ppdd_exporter/internal/ddclient"
)

func TestPaginateFollowsAllPages(t *testing.T) {
	m := ddclient.NewMock("dd01")
	// Server reports page_size 2, total 3 → two pages.
	m.SetJSON("/things?page=0&size=200",
		`{"paging_info":{"current_page":0,"page_entries":2,"total_entries":3,"page_size":2},"thing":[{"v":1},{"v":2}]}`)
	m.SetJSON("/things?page=1&size=200",
		`{"paging_info":{"current_page":1,"page_entries":1,"total_entries":3,"page_size":2},"thing":[{"v":3}]}`)

	var vals []int
	err := paginate(context.Background(), m, "/things", "", func(page json.RawMessage) (pagingInfo, error) {
		var r struct {
			Thing []struct {
				V int `json:"v"`
			} `json:"thing"`
			PagingInfo pagingInfo `json:"paging_info"`
		}
		if err := json.Unmarshal(page, &r); err != nil {
			return pagingInfo{}, err
		}
		for _, t := range r.Thing {
			vals = append(vals, t.V)
		}
		return r.PagingInfo, nil
	})
	if err != nil {
		t.Fatalf("paginate: %v", err)
	}
	if len(vals) != 3 {
		t.Fatalf("collected %v, want 3 items across 2 pages", vals)
	}
}

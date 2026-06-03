package ppdd

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/fjacquet/ppdd_exporter/internal/ddclient"
	log "github.com/sirupsen/logrus"
)

const (
	pageSize = 200 // documented max page size
	maxPages = 100 // safety cap to bound a runaway list
)

// pagingInfo is the DD list-response envelope (guide pp.17,20).
type pagingInfo struct {
	CurrentPage  int `json:"current_page"`
	PageEntries  int `json:"page_entries"`
	TotalEntries int `json:"total_entries"`
	PageSize     int `json:"page_size"`
}

// paginate GETs basePath across all pages, handing each page's raw JSON to onPage.
// onPage decodes its own named array and returns that page's paging_info so the
// loop knows when to stop. extraQuery carries collector params (no leading &).
func paginate(ctx context.Context, c ddclient.Client, basePath, extraQuery string,
	onPage func(page json.RawMessage) (pagingInfo, error)) error {
	for p := 0; p < maxPages; p++ {
		path := fmt.Sprintf("%s?page=%d&size=%d", basePath, p, pageSize)
		if extraQuery != "" {
			path += "&" + extraQuery
		}
		var raw json.RawMessage
		if err := c.Get(ctx, path, &raw); err != nil {
			return err
		}
		pi, err := onPage(raw)
		if err != nil {
			return err
		}
		// Stop when the server reports no usable paging, or all entries covered.
		if pi.PageSize <= 0 || pi.PageEntries == 0 ||
			(pi.CurrentPage+1)*pi.PageSize >= pi.TotalEntries {
			return nil
		}
	}
	log.WithField("path", basePath).Warn("pagination hit maxPages cap; list may be truncated")
	return nil
}

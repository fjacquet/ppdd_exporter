package ppdd

import "fmt"

// DD REST API paths, validated against the PowerProtect DD 8.7.0 OpenAPI spec
// (docs/swagger/13345-8.7.0.json). The /api/ vs /rest/ prefix and the per-resource
// version token are both as the spec defines them. A future version bump re-validates
// against a new spec and edits THIS FILE ONLY.
const (
	pathSystem      = "/rest/v1.0/system"                      // capacity + compression_factor (validated 8.7.0)
	pathAlerts      = "/rest/v1.0/dd-systems/0/alerts"         // validated 8.7.0; query is_active=true
	pathMTrees      = "/rest/v3.0/dd-systems/0/mtrees"         // validated 8.7.0: v3.0 metadata list
	pathDisks       = "/api/v1/dd-systems/0/storage/disks"     // validated 8.7.0: schema DiskInfos
	pathPerformance = "/api/v3/dd-systems/0/stats/performance" // validated 8.7.0: schema SystemPerformance
	pathFileSystem  = "/rest/v1.0/dd-systems/0/file-systems"   // validated 8.7.0: filesysInfo (clean state)

	pathMTreeReplication = "/api/v1/dd-systems/0/mtree-replications"         // validated 8.7.0: schema MtreeReplicationInfos
	pathFileReplication  = "/rest/v1.0/dd-systems/0/stats/file-replications" // validated 8.7.0: schema fileReplicationList
)

// mtreeStatsPath returns the per-MTree capacity stats path (v2.0, validated 8.7.0).
// id is the URL-encoded MTree object ID from the v3.0 mtree list.
func mtreeStatsPath(id string) string {
	return fmt.Sprintf("/rest/v2.0/dd-systems/0/mtrees/%s/stats/capacity", id)
}

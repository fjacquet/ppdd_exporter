package ppdd

import "fmt"

// DD REST API paths. The /rest/ prefix is documented as supported across DD OS
// versions; the version token is PER-RESOURCE (NOT a uniform v1) per the
// PowerProtect DD 7.3 REST API guide. A future correction against a live
// appliance edits THIS FILE ONLY.
const (
	pathSystem      = "/rest/v1.0/system"                      // capacity + compression_factor (documented)
	pathAlerts      = "/rest/v1.0/dd-systems/0/alerts"         // documented; query is_active=true
	pathMTrees      = "/rest/v3.0/dd-systems/0/mtrees"         // documented; v3.0 metadata list
	pathDisks       = "/api/v1/dd-systems/0/storage/disks"     // validated 8.7.0: schema DiskInfos
	pathPerformance = "/api/v3/dd-systems/0/stats/performance" // validated 8.7.0: schema SystemPerformance
	pathFileSystem  = "/rest/v1.0/dd-systems/0/file-systems"   // validated 8.7.0: filesysInfo (clean state)

	pathMTreeReplication = "/api/v1/dd-systems/0/mtree-replications"         // validated 8.7.0: schema MtreeReplicationInfos
	pathFileReplication  = "/rest/v1.0/dd-systems/0/stats/file-replications" // validated 8.7.0: schema fileReplicationList
)

// mtreeStatsPath returns the per-MTree capacity stats path (v2.0, documented).
// id is the URL-encoded MTree object ID from the v3.0 mtree list.
func mtreeStatsPath(id string) string {
	return fmt.Sprintf("/rest/v2.0/dd-systems/0/mtrees/%s/stats/capacity", id)
}

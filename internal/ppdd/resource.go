package ppdd

import (
	"context"

	"github.com/fjacquet/ppdd_exporter/internal/ddclient"
)

// ResourceCollector collects one metric domain from a single DD system. It returns
// system-agnostic samples; the loop stamps the `system` label. Implementations own
// their endpoint path and JSON struct so the provisional API risk is localized.
type ResourceCollector interface {
	Name() string
	Collect(ctx context.Context, c ddclient.Client) ([]Sample, error)
}

// Registry is the ordered set of collectors run for every system.
func Registry() []ResourceCollector {
	return []ResourceCollector{
		Capacity{},
		MTrees{},
		Replication{},
		Health{},
	}
}

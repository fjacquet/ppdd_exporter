package ppdd

import "github.com/prometheus/client_golang/prometheus"

// NewBuildInfoCollector returns a collector exposing a single constant metric,
// `ppdd_exporter_build_info{version="..."} 1`, so a scrape reveals exactly which
// exporter build is running. This is the standard Prometheus build-info pattern
// (cf. node_exporter_build_info, prometheus_build_info): the value is always 1 and
// the useful data lives in the label, letting dashboards join on it or alert when
// an expected version disappears.
func NewBuildInfoCollector(version string) prometheus.Collector {
	g := prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace:   "ppdd_exporter",
		Name:        "build_info",
		Help:        "Exporter build information; constant 1, with the running version in the `version` label.",
		ConstLabels: prometheus.Labels{"version": version},
	})
	g.Set(1)
	return g
}

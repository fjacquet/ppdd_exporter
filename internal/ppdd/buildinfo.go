package ppdd

import "github.com/prometheus/client_golang/prometheus"

// NewBuildInfoCollector returns a collector exposing a single constant metric,
// `ppdd_exporter_build_info{version="...",goversion="..."} 1`, so a scrape reveals
// exactly which exporter build is running. This is the standard Prometheus build-info
// pattern (cf. node_exporter_build_info, prometheus_build_info): the value is always 1
// and the useful data lives in the labels, letting dashboards join on it or alert when
// an expected version disappears. version comes from the -X main.version ldflag;
// goversion is the compiler version (runtime.Version()), passed in for testability.
func NewBuildInfoCollector(version, goversion string) prometheus.Collector {
	g := prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace:   "ppdd_exporter",
		Name:        "build_info",
		Help:        "Exporter build information; constant 1, with the running version and Go version in the `version` and `goversion` labels.",
		ConstLabels: prometheus.Labels{"version": version, "goversion": goversion},
	})
	g.Set(1)
	return g
}

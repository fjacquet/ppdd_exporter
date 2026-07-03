package ppdd

import (
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestBuildInfoCollector(t *testing.T) {
	reg := prometheus.NewRegistry()
	reg.MustRegister(NewBuildInfoCollector("v1.2.3", "go1.99"))

	const want = `
# HELP ppdd_exporter_build_info Exporter build information; constant 1, with the running version and Go version in the ` + "`version`" + ` and ` + "`goversion`" + ` labels.
# TYPE ppdd_exporter_build_info gauge
ppdd_exporter_build_info{goversion="go1.99",version="v1.2.3"} 1
`
	if err := testutil.GatherAndCompare(reg, strings.NewReader(want), "ppdd_exporter_build_info"); err != nil {
		t.Fatal(err)
	}
}

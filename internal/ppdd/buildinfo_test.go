package ppdd

import (
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestBuildInfoCollector(t *testing.T) {
	reg := prometheus.NewRegistry()
	reg.MustRegister(NewBuildInfoCollector("v1.2.3"))

	const want = `
# HELP ppdd_exporter_build_info Exporter build information; constant 1, with the running version in the ` + "`version`" + ` label.
# TYPE ppdd_exporter_build_info gauge
ppdd_exporter_build_info{version="v1.2.3"} 1
`
	if err := testutil.GatherAndCompare(reg, strings.NewReader(want), "ppdd_exporter_build_info"); err != nil {
		t.Fatal(err)
	}
}

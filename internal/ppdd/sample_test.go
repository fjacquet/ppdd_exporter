package ppdd

import "testing"

func TestSampleLabelValueLookup(t *testing.T) {
	s := Sample{Name: "ppdd_filesystem_used_bytes", Value: 42,
		Labels: []Label{{Key: "system", Value: "dd01"}}}
	if got := s.LabelValue("system"); got != "dd01" {
		t.Fatalf("LabelValue(system) = %q, want dd01", got)
	}
	if got := s.LabelValue("missing"); got != "" {
		t.Fatalf("LabelValue(missing) = %q, want empty", got)
	}
}

func TestWithSystemPrependsLabel(t *testing.T) {
	s := Sample{Name: "x", Labels: []Label{{Key: "mtree", Value: "/data/col1/m1"}}}
	out := s.WithSystem("dd01")
	if out.Labels[0].Key != "system" || out.Labels[0].Value != "dd01" {
		t.Fatalf("WithSystem did not prepend system label: %+v", out.Labels)
	}
}

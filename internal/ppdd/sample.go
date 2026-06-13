// Package ppdd holds the DD metric model, snapshot store, modular collectors,
// and the Prometheus export path.
package ppdd

// Label is a single Prometheus label key/value.
type Label struct {
	Key   string
	Value string
}

// Sample is one metric data point: a name, an ordered label set, and a value.
type Sample struct {
	Name   string
	Labels []Label
	Value  float64
}

// LabelValue returns the value of the named label, or "" if absent.
func (s Sample) LabelValue(key string) string {
	for _, l := range s.Labels {
		if l.Key == key {
			return l.Value
		}
	}
	return ""
}

// boolGauge maps a boolean to the Prometheus 1/0 gauge convention.
func boolGauge(b bool) float64 {
	if b {
		return 1
	}
	return 0
}

// WithSystem returns a copy with a leading {system=name} label. Collectors emit
// system-agnostic samples; the collection loop stamps the system identity.
func (s Sample) WithSystem(name string) Sample {
	labels := make([]Label, 0, len(s.Labels)+1)
	labels = append(labels, Label{Key: "system", Value: name})
	labels = append(labels, s.Labels...)
	return Sample{Name: s.Name, Labels: labels, Value: s.Value}
}

package ddclient

import (
	"context"
	"encoding/json"
	"fmt"
)

// Mock is an in-memory Client that serves canned JSON bodies per path. Tests use it
// to drive collectors without a live appliance.
type Mock struct {
	name  string
	paths map[string]string
}

// NewMock returns an empty Mock for the named system.
func NewMock(name string) *Mock { return &Mock{name: name, paths: map[string]string{}} }

// SetJSON registers a response body for an exact path.
func (m *Mock) SetJSON(path, body string) { m.paths[path] = body }

func (m *Mock) Name() string { return m.name }

func (m *Mock) Get(_ context.Context, path string, out any) error {
	body, ok := m.paths[path]
	if !ok {
		return fmt.Errorf("mock: no response registered for %s", path)
	}
	return json.Unmarshal([]byte(body), out)
}

func (m *Mock) Close() error { return nil }

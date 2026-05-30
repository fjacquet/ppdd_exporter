package ddclient

import (
	"context"
	"testing"
)

func TestMockClientServesRegisteredPath(t *testing.T) {
	m := NewMock("dd01")
	m.SetJSON("/api/v1/dd-systems/0/file-system", `{"physical_used":10}`)

	var out struct {
		PhysicalUsed float64 `json:"physical_used"`
	}
	if err := m.Get(context.Background(), "/api/v1/dd-systems/0/file-system", &out); err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if out.PhysicalUsed != 10 {
		t.Fatalf("PhysicalUsed = %v, want 10", out.PhysicalUsed)
	}
}

func TestMockClientUnknownPathErrors(t *testing.T) {
	m := NewMock("dd01")
	var out map[string]any
	if err := m.Get(context.Background(), "/nope", &out); err == nil {
		t.Fatal("expected error for unregistered path")
	}
}

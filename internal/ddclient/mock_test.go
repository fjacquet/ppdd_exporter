package ddclient

import (
	"context"
	"testing"
)

func TestMockClientServesRegisteredPath(t *testing.T) {
	m := NewMock("dd01")
	m.SetJSON("/rest/v1.0/system", `{"physical_used":10}`)

	var out struct {
		PhysicalUsed float64 `json:"physical_used"`
	}
	if err := m.Get(context.Background(), "/rest/v1.0/system", &out); err != nil {
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

func TestMockClientFallsBackToPathWithoutQuery(t *testing.T) {
	m := NewMock("dd01")
	m.SetJSON("/rest/v1.0/dd-systems/0/alerts", `{"alert":[{"severity":"warning"}]}`)

	var out struct {
		Alert []struct {
			Severity string `json:"severity"`
		} `json:"alert"`
	}
	// Caller appends ?page=&size=; the clean path is what was registered.
	if err := m.Get(context.Background(), "/rest/v1.0/dd-systems/0/alerts?page=0&size=200", &out); err != nil {
		t.Fatalf("Get with query should fall back to clean path: %v", err)
	}
	if len(out.Alert) != 1 || out.Alert[0].Severity != "warning" {
		t.Fatalf("unexpected decode: %+v", out)
	}
}

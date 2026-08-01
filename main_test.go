package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/fjacquet/ppdd_exporter/internal/ppdd"
)

func TestLivezReturnsOK(t *testing.T) {
	rec := httptest.NewRecorder()
	staticOKHandler(rec, httptest.NewRequest(http.MethodGet, "/livez", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestReadyzReturnsOK(t *testing.T) {
	rec := httptest.NewRecorder()
	staticOKHandler(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestHealthReturns200WhenSystemUnhealthy(t *testing.T) {
	store := ppdd.NewSnapshotStore()
	store.Store(&ppdd.Snapshot{
		BuiltAt: time.Now(),
		Systems: []*ppdd.SystemSnapshot{
			{System: "dd01", OK: false, Err: "login POST: status 401"},
		},
	})

	rec := httptest.NewRecorder()
	healthHandler(rec, store)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var body struct {
		Systems []struct {
			System string `json:"system"`
			OK     bool   `json:"ok"`
			Err    string `json:"err"`
		} `json:"systems"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if len(body.Systems) != 1 || body.Systems[0].OK {
		t.Fatalf("systems = %+v, want one system with ok=false", body.Systems)
	}
}

func TestHealthReturns200WhenNoSystems(t *testing.T) {
	store := ppdd.NewSnapshotStore()

	rec := httptest.NewRecorder()
	healthHandler(rec, store)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

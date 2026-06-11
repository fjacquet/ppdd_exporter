package ddclient

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

const authToken = "test-token-123"

// writeBytes streams a fixture body to the fake-appliance response (test-only;
// no user input is reflected, so XSS does not apply).
func writeBytes(w http.ResponseWriter, b []byte) { _, _ = io.Copy(w, bytes.NewReader(b)) }

func newFakeDD(t *testing.T, logins *int32) *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/rest/v1.0/auth", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			atomic.AddInt32(logins, 1)
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if _, ok := body["username"]; !ok { // flat shape, NOT {auth_info:{...}}
				t.Errorf("auth body missing flat username field: %v", body)
			}
			w.Header().Set("X-DD-AUTH-TOKEN", authToken)
			w.WriteHeader(http.StatusCreated)
		case http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		}
	})
	mux.HandleFunc("/rest/v1.0/system", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-DD-AUTH-TOKEN") != authToken {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		writeBytes(w, []byte(`{"physical_used":99}`))
	})
	return httptest.NewTLSServer(mux)
}

func TestSystemClientAuthAndGet(t *testing.T) {
	var logins int32
	srv := newFakeDD(t, &logins)
	defer srv.Close()

	c := NewSystemClient(Config{
		Name: "dd01", BaseURL: srv.URL, Username: "u", Password: "p",
		InsecureSkipVerify: true, HTTPClient: srv.Client(),
	})
	t.Cleanup(func() {
		if err := c.Close(); err != nil {
			t.Errorf("SystemClient.Close: %v", err)
		}
	})

	var out struct {
		PhysicalUsed float64 `json:"physical_used"`
	}
	if err := c.Get(context.Background(), "/rest/v1.0/system", &out); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if out.PhysicalUsed != 99 {
		t.Fatalf("PhysicalUsed = %v, want 99", out.PhysicalUsed)
	}
	// Second Get reuses the token — no extra login.
	if err := c.Get(context.Background(), "/rest/v1.0/system", &out); err != nil {
		t.Fatalf("second Get: %v", err)
	}
	if logins != 1 {
		t.Fatalf("logins = %d, want 1 (token should be reused)", logins)
	}
}

func TestTraceDoesNotBreakCalls(t *testing.T) {
	var logins int32
	srv := newFakeDD(t, &logins)
	defer srv.Close()

	c := NewSystemClient(Config{
		Name: "dd01", BaseURL: srv.URL, Username: "u", Password: "p",
		HTTPClient: srv.Client(),
		Trace:      true,
	})
	t.Cleanup(func() {
		if err := c.Close(); err != nil {
			t.Errorf("SystemClient.Close: %v", err)
		}
	})

	var out struct {
		PhysicalUsed float64 `json:"physical_used"`
	}
	// Exercises the OnAfterResponse trace hook on both the auth call (skipped)
	// and the data call (logged); the decoded result must be unaffected.
	if err := c.Get(context.Background(), "/rest/v1.0/system", &out); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if out.PhysicalUsed != 99 {
		t.Fatalf("PhysicalUsed = %v, want 99", out.PhysicalUsed)
	}
}

func TestSystemClientReloginOn401(t *testing.T) {
	var logins int32
	var rotated atomic.Bool
	mux := http.NewServeMux()
	mux.HandleFunc("/rest/v1.0/auth", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&logins, 1)
		tok := "tok1"
		if rotated.Load() {
			tok = "tok2"
		}
		w.Header().Set("X-DD-AUTH-TOKEN", tok)
		w.WriteHeader(http.StatusCreated)
	})
	mux.HandleFunc("/rest/v1.0/system", func(w http.ResponseWriter, r *http.Request) {
		// Only "tok2" is accepted; first call (tok1) returns 401 and forces relogin.
		if r.Header.Get("X-DD-AUTH-TOKEN") != "tok2" {
			rotated.Store(true)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		writeBytes(w, []byte(`{"physical_used":1}`))
	})
	srv := httptest.NewTLSServer(mux)
	defer srv.Close()

	c := NewSystemClient(Config{Name: "dd01", BaseURL: srv.URL, HTTPClient: srv.Client()})
	t.Cleanup(func() {
		if err := c.Close(); err != nil {
			t.Errorf("SystemClient.Close: %v", err)
		}
	})
	var out map[string]any
	if err := c.Get(context.Background(), "/rest/v1.0/system", &out); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if logins != 2 {
		t.Fatalf("logins = %d, want 2 (one initial + one relogin)", logins)
	}
}

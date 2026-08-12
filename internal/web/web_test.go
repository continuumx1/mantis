package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The frontend must serve the embedded UI at / and proxy /api to the configured
// engine, keeping the browser same-origin.
func TestNewHandler_ServesUIAndProxiesAPI(t *testing.T) {
	// Stand in for mantis-engine: a backend that answers /api/graph.
	engine := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/graph" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ok":true}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer engine.Close()

	handler, err := NewHandler(engine.URL)
	if err != nil {
		t.Fatalf("NewHandler: %v", err)
	}

	// / serves the UI shell.
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET / status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "<title>Mantis") {
		t.Errorf("GET / did not serve the UI; body began: %.60q", rec.Body.String())
	}

	// /api/graph is proxied to the engine.
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/graph", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/graph status = %d, want 200", rec.Code)
	}
	if got := strings.TrimSpace(rec.Body.String()); got != `{"ok":true}` {
		t.Errorf("proxied body = %q, want engine response", got)
	}
}

// An engine URL without a scheme and host is a configuration error and must be
// rejected at startup rather than failing silently on the first request.
func TestNewHandler_RejectsBadEngineURL(t *testing.T) {
	for _, bad := range []string{"", "mantis-engine:8080", "://nope"} {
		if _, err := NewHandler(bad); err == nil {
			t.Errorf("NewHandler(%q) = nil error, want rejection", bad)
		}
	}
}

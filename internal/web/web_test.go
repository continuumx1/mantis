package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func testEngine(t *testing.T) *httptest.Server {
	t.Helper()
	engine := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/graph" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ok":true}`))
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(engine.Close)
	return engine
}

// login POSTs the hardcoded dev credentials and returns the session cookie the
// handler set, failing the test if login did not succeed.
func login(t *testing.T, handler http.Handler) *http.Cookie {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"username": "admin", "password": "admin"})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/login", bytes.NewReader(body))
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /login status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	cookies := rec.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatalf("POST /login set no cookies")
	}
	return cookies[0]
}

// Unauthenticated requests must not reach the UI or the proxied API: a browser
// navigation bounces to the login page, and an API fetch gets a plain 401.
func TestNewHandler_RequiresLogin(t *testing.T) {
	handler, err := NewHandler(testEngine(t).URL)
	if err != nil {
		t.Fatalf("NewHandler: %v", err)
	}

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusFound {
		t.Fatalf("GET / (no session) status = %d, want 302", rec.Code)
	}
	if loc := rec.Header().Get("Location"); !strings.HasPrefix(loc, "/login") {
		t.Errorf("GET / (no session) redirected to %q, want /login", loc)
	}

	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/graph", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("GET /api/graph (no session) status = %d, want 401", rec.Code)
	}

	// The login page itself must stay reachable without a session.
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/login", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /login status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Sign in") {
		t.Errorf("GET /login did not serve the login page; body began: %.60q", rec.Body.String())
	}
}

// The wrong credentials must not issue a session.
func TestNewHandler_LoginRejectsBadCredentials(t *testing.T) {
	handler, err := NewHandler(testEngine(t).URL)
	if err != nil {
		t.Fatalf("NewHandler: %v", err)
	}

	body, _ := json.Marshal(map[string]string{"username": "admin", "password": "wrong"})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/login", bytes.NewReader(body)))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("POST /login (bad creds) status = %d, want 401", rec.Code)
	}
	if len(rec.Result().Cookies()) != 0 {
		t.Errorf("POST /login (bad creds) set a cookie, want none")
	}
}

// With a valid session, / serves the UI shell and /api/ is proxied to the
// engine — the same behavior the pre-login-gate test covered.
func TestNewHandler_ServesUIAndProxiesAPIWhenLoggedIn(t *testing.T) {
	handler, err := NewHandler(testEngine(t).URL)
	if err != nil {
		t.Fatalf("NewHandler: %v", err)
	}
	cookie := login(t, handler)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(cookie)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET / (with session) status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "<title>Mantis") {
		t.Errorf("GET / did not serve the UI; body began: %.60q", rec.Body.String())
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/graph", nil)
	req.AddCookie(cookie)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/graph (with session) status = %d, want 200", rec.Code)
	}
	if got := strings.TrimSpace(rec.Body.String()); got != `{"ok":true}` {
		t.Errorf("proxied body = %q, want engine response", got)
	}
}

// Logging out revokes the session, so a subsequent request behaves as
// unauthenticated again.
func TestNewHandler_Logout(t *testing.T) {
	handler, err := NewHandler(testEngine(t).URL)
	if err != nil {
		t.Fatalf("NewHandler: %v", err)
	}
	cookie := login(t, handler)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/logout", nil)
	req.AddCookie(cookie)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /logout status = %d, want 200", rec.Code)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(cookie)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("GET / (after logout) status = %d, want 302", rec.Code)
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

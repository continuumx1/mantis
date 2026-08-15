package web

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Mantis has no real user store yet. devUsername/devPassword back a
// public-preview login screen — a single hardcoded credential pair and an
// in-memory session table — that stands a login gate up in front of the graph
// while a real auth story lands.
//
// admin/admin is fine as *temporary preview/demo access*, published as such on
// the login page itself. It must never quietly become "the" default once real
// authentication lands: whoever builds that should be deleting this file's
// login flow wholesale, not layering onto it. If Mantis is still checking
// these two constants outside of a preview build, that is a bug, not a
// deploy detail — grep for devPassword before calling any build "production."
const (
	devUsername = "admin"
	devPassword = "admin"

	sessionCookie = "mantis_session"
	sessionTTL    = 12 * time.Hour
)

// publicPaths lists routes reachable without a session: the login page and its
// API, logout (so a stale cookie can always be cleared), the liveness probe
// (Kubernetes must be able to reach it without a session), and the brand
// assets the login page itself renders.
var publicPaths = map[string]bool{
	"/login":               true,
	"/logout":              true,
	"/healthz":             true,
	"/favicon.ico":         true,
	"/mantis-mascot.webp":  true,
	"/mantis-login-bg.jpg": true,
}

// sessions tracks issued session tokens in memory. It resets on every process
// restart, which is fine for a development-only login gate — nobody expects
// "stay signed in" guarantees from a hardcoded admin/admin.
type sessions struct {
	mu     sync.Mutex
	tokens map[string]time.Time
}

func newSessions() *sessions {
	return &sessions{tokens: map[string]time.Time{}}
}

// issue mints a new session token and remembers its expiry.
func (s *sessions) issue() string {
	b := make([]byte, 24)
	tok := ""
	if _, err := rand.Read(b); err == nil {
		tok = hex.EncodeToString(b)
	} else {
		// crypto/rand failing is effectively unrecoverable in practice; fall back
		// to a timestamp-derived token rather than handing back an empty (and
		// thus permanently invalid) one.
		tok = hex.EncodeToString([]byte(time.Now().String()))
	}
	s.mu.Lock()
	s.tokens[tok] = time.Now().Add(sessionTTL)
	s.mu.Unlock()
	return tok
}

// valid reports whether tok is a live, unexpired session, pruning it if not.
func (s *sessions) valid(tok string) bool {
	if tok == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	exp, ok := s.tokens[tok]
	if !ok {
		return false
	}
	if time.Now().After(exp) {
		delete(s.tokens, tok)
		return false
	}
	return true
}

// revoke drops a session, if present.
func (s *sessions) revoke(tok string) {
	s.mu.Lock()
	delete(s.tokens, tok)
	s.mu.Unlock()
}

// loginRequest is the POST /login body the login page sends.
type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// handleLogin checks the hardcoded dev credentials and, on success, sets a
// session cookie. The comparisons are constant-time — cheap insurance even
// though the credentials are printed on the login page itself in this dev
// build.
func (s *sessions) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	userOK := subtle.ConstantTimeCompare([]byte(req.Username), []byte(devUsername)) == 1
	passOK := subtle.ConstantTimeCompare([]byte(req.Password), []byte(devPassword)) == 1
	if !userOK || !passOK {
		writeJSONError(w, http.StatusUnauthorized, "invalid username or password")
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    s.issue(),
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(sessionTTL.Seconds()),
	})
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}

// handleLogout revokes the caller's session and clears the cookie.
func (s *sessions) handleLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil {
		s.revoke(c.Value)
	}
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteLaxMode,
	})
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}

// gate wraps a handler so every request needs a valid session, except the
// publicPaths. A browser navigation without one is sent to the login page; an
// /api/ request — always a fetch() the SPA's JS made, never a navigation —
// gets a plain 401 instead, so the frontend can react (see index.html's
// fetchGraph) without a full-page redirect happening underneath it.
//
// The redirect always lands on plain "/login", not "/login?next=<original
// path>". Mantis is a single-page app — "/" is the only real destination —
// so bouncing back to whatever path the browser happened to request had no
// payoff and one sharp edge: a malformed or unexpected request path (e.g. a
// browser mangling "/login" into "/login.") would ride the "next" param
// through a successful login and land the user on that same broken path
// again, turning a one-off request quirk into a persistent post-login 404.
func (s *sessions) gate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if publicPaths[r.URL.Path] {
			next.ServeHTTP(w, r)
			return
		}
		if c, err := r.Cookie(sessionCookie); err == nil && s.valid(c.Value) {
			next.ServeHTTP(w, r)
			return
		}
		isNav := (r.Method == http.MethodGet || r.Method == http.MethodHead) && !strings.HasPrefix(r.URL.Path, "/api/")
		if isNav {
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}
		writeJSONError(w, http.StatusUnauthorized, "unauthorized")
	})
}

func writeJSONError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

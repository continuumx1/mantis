package web

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// Mantis signs in a single administrator account whose credentials the
// operator configures (see CredentialsFromEnv): there is no built-in default
// password. Sessions are random tokens held in memory by each mantis-web
// replica.
const (
	sessionCookie = "mantis_session"
	sessionTTL    = 12 * time.Hour

	// A client address that fails maxLoginFailures sign-ins within
	// loginFailureWindow is refused until that window has passed.
	maxLoginFailures   = 10
	loginFailureWindow = 10 * time.Minute
	minPasswordLength  = 8
)

// Credentials is the administrator account. The password is held only as a
// bcrypt hash.
type Credentials struct {
	Username string
	hash     []byte
}

// NewCredentials builds credentials from a plain password, hashing it.
func NewCredentials(username, password string) (Credentials, error) {
	if username == "" {
		return Credentials{}, errors.New("the admin username must not be empty")
	}
	if len([]rune(password)) < minPasswordLength {
		return Credentials{}, fmt.Errorf("the admin password must be at least %d characters", minPasswordLength)
	}
	h, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return Credentials{}, err
	}
	return Credentials{Username: username, hash: h}, nil
}

// NewCredentialsFromHash builds credentials from a bcrypt hash.
func NewCredentialsFromHash(username, hash string) (Credentials, error) {
	if username == "" {
		return Credentials{}, errors.New("the admin username must not be empty")
	}
	hash = strings.TrimSpace(hash)
	if _, err := bcrypt.Cost([]byte(hash)); err != nil {
		return Credentials{}, errors.New("MANTIS_ADMIN_PASSWORD_HASH is not a valid bcrypt hash")
	}
	return Credentials{Username: username, hash: []byte(hash)}, nil
}

// CredentialsFromEnv reads the administrator account:
//
//	MANTIS_ADMIN_USERNAME       account name (default "admin")
//	MANTIS_ADMIN_PASSWORD_HASH  bcrypt hash of the password (preferred), or
//	MANTIS_ADMIN_PASSWORD       the password itself, hashed at startup
//
// With neither password variable set it returns an error: mantis-web does
// not start with a guessable default.
func CredentialsFromEnv() (Credentials, error) {
	user := strings.TrimSpace(os.Getenv("MANTIS_ADMIN_USERNAME"))
	if user == "" {
		user = "admin"
	}
	if h := os.Getenv("MANTIS_ADMIN_PASSWORD_HASH"); strings.TrimSpace(h) != "" {
		return NewCredentialsFromHash(user, h)
	}
	if pw := os.Getenv("MANTIS_ADMIN_PASSWORD"); pw != "" {
		return NewCredentials(user, pw)
	}
	return Credentials{}, errors.New("no admin password configured: set MANTIS_ADMIN_PASSWORD_HASH (a bcrypt hash) or MANTIS_ADMIN_PASSWORD")
}

// check reports whether username and password match, in time that does not
// depend on which one is wrong.
func (c Credentials) check(username, password string) bool {
	userOK := subtle.ConstantTimeCompare([]byte(username), []byte(c.Username)) == 1
	passOK := bcrypt.CompareHashAndPassword(c.hash, []byte(password)) == nil
	return userOK && passOK
}

// publicPaths lists routes reachable without a session: the login page and its
// API, logout (so a stale cookie can always be cleared), the liveness probe
// (Kubernetes must be able to reach it without a session), and the brand
// assets the login page itself renders.
var publicPaths = map[string]bool{
	"/login":               true,
	"/logout":              true,
	"/healthz":             true,
	"/favicon.ico":         true,
	"/brand/favicon.svg":   true,
	"/mantis-mascot.webp":  true,
	"/mantis-login-bg.jpg": true,
}

// sessions tracks issued session tokens in memory, plus recent failed
// sign-ins per client address. A restart signs everyone out.
type sessions struct {
	creds    Credentials
	mu       sync.Mutex
	tokens   map[string]session
	failures map[string]*failures
}

type session struct {
	user    string
	expires time.Time
}

type failures struct {
	count int
	since time.Time
}

func newSessions(creds Credentials) *sessions {
	return &sessions{creds: creds, tokens: map[string]session{}, failures: map[string]*failures{}}
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
	s.tokens[tok] = session{user: s.creds.Username, expires: time.Now().Add(sessionTTL)}
	for t, v := range s.tokens {
		if time.Now().After(v.expires) {
			delete(s.tokens, t)
		}
	}
	s.mu.Unlock()
	return tok
}

// lookup returns the live session for tok, pruning it if expired.
func (s *sessions) lookup(tok string) (session, bool) {
	if tok == "" {
		return session{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.tokens[tok]
	if !ok {
		return session{}, false
	}
	if time.Now().After(v.expires) {
		delete(s.tokens, tok)
		return session{}, false
	}
	return v, true
}

// valid reports whether tok is a live, unexpired session.
func (s *sessions) valid(tok string) bool {
	_, ok := s.lookup(tok)
	return ok
}

// clientKey identifies the client for sign-in throttling: the connection's
// address (not X-Forwarded-For, which a client can set to anything).
func clientKey(r *http.Request) string {
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}

// throttled reports whether key has failed too many sign-ins recently.
func (s *sessions) throttled(key string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, ok := s.failures[key]
	if !ok {
		return false
	}
	if time.Since(f.since) > loginFailureWindow {
		delete(s.failures, key)
		return false
	}
	return f.count >= maxLoginFailures
}

func (s *sessions) recordFailure(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	for k, f := range s.failures {
		if now.Sub(f.since) > loginFailureWindow {
			delete(s.failures, k)
		}
	}
	f, ok := s.failures[key]
	if !ok {
		f = &failures{since: now}
		s.failures[key] = f
	}
	f.count++
}

func (s *sessions) clearFailures(key string) {
	s.mu.Lock()
	delete(s.failures, key)
	s.mu.Unlock()
}

// secureRequest reports whether the browser reached Mantis over HTTPS, directly
// or through a TLS-terminating proxy, so the session cookie can be marked
// Secure.
func secureRequest(r *http.Request) bool {
	return r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
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

// handleLogin checks the configured credentials and, on success, sets a
// session cookie.
func (s *sessions) handleLogin(w http.ResponseWriter, r *http.Request) {
	key := clientKey(r)
	if s.throttled(key) {
		slog.Warn("auth", "event", "login_throttled", "remote_addr", r.RemoteAddr)
		writeJSONError(w, http.StatusTooManyRequests, "too many failed sign-ins; try again in a few minutes")
		return
	}
	var req loginRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if !s.creds.check(req.Username, req.Password) {
		s.recordFailure(key)
		// Never log the submitted username/password — only that an attempt
		// failed and where from, enough to spot a brute-force pattern without
		// putting credentials (real or mistyped-but-sensitive) in the log stream.
		slog.Warn("auth", "event", "login_failed", "remote_addr", r.RemoteAddr)
		writeJSONError(w, http.StatusUnauthorized, "invalid username or password")
		return
	}
	s.clearFailures(key)
	slog.Info("auth", "event", "login_succeeded", "remote_addr", r.RemoteAddr)

	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    s.issue(),
		Path:     "/",
		HttpOnly: true,
		Secure:   secureRequest(r),
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(sessionTTL.Seconds()),
	})
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}

// handleSession returns who is signed in (GET /api/session), for the UI's
// user menu.
func (s *sessions) handleSession(w http.ResponseWriter, r *http.Request) {
	user := ""
	if c, err := r.Cookie(sessionCookie); err == nil {
		if v, ok := s.lookup(c.Value); ok {
			user = v.user
		}
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(map[string]string{"username": user, "role": "Administrator"})
}

// handleLogout revokes the caller's session and clears the cookie.
func (s *sessions) handleLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil {
		s.revoke(c.Value)
	}
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, Secure: secureRequest(r), SameSite: http.SameSiteLaxMode,
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
		// An /api/ request with no valid session is the SPA already running in
		// the browser suddenly finding its session gone (expired, or the
		// process restarted and dropped the in-memory table) — worth a log line,
		// unlike a first-time anonymous page load (isNav above), which is
		// routine and would otherwise flood this at every logged-out visit.
		slog.Warn("auth", "event", "session_invalid", "path", r.URL.Path)
		writeJSONError(w, http.StatusUnauthorized, "unauthorized")
	})
}

func writeJSONError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

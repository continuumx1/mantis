// Package web is the Mantis frontend: it serves the resource-graph UI and reverse-
// proxies /api requests to the engine backend, keeping the browser same-origin.
package web

import (
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"

	"github.com/continuumx1/mantis/internal/httpx"
)

// "all:" keeps files whose names start with "." or "_", which a bare
// "//go:embed ui" silently drops with no build-time signal.
//
//go:embed all:ui
var uiFiles embed.FS

// StaticHandler serves the embedded single-page UI. The UI ships inside the
// binary — no assets to mount at runtime.
func StaticHandler() (http.Handler, error) {
	ui, err := fs.Sub(uiFiles, "ui")
	if err != nil {
		return nil, fmt.Errorf("mount ui assets: %w", err)
	}
	return noCache(http.FileServer(http.FS(ui))), nil
}

// noCache makes browsers revalidate UI files on every load. Without it a
// browser can keep an old stylesheet while picking up new JavaScript after an
// upgrade, and render a mix of the two. The files are small and embedded, so
// always fetching current ones costs little.
func noCache(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache")
		h.ServeHTTP(w, r)
	})
}

// NoCache is noCache for other binaries serving the same UI (cmd/mantis-dev).
func NoCache(h http.Handler) http.Handler { return noCache(h) }

// NewHandler builds the frontend service: it serves the UI at / and reverse-
// proxies every /api/ request to the engine at engineURL. Keeping the browser
// same-origin means no CORS, and the engine never has to face the browser — it
// can stay a private ClusterIP reachable only from this service.
//
// Config configures mantis-web's handler.
type Config struct {
	// EngineURL is mantis-engine's base URL; /api/* is proxied to it.
	EngineURL string
	// ReportURL is mantis-report's base URL for /api/report. Empty means the
	// report service is not deployed: report requests get a clear 503.
	ReportURL string
	// Admin is the account that can sign in (see CredentialsFromEnv).
	Admin Credentials
}

// NewHandler builds mantis-web. Everything except the login page, logout, the
// liveness probe and the login page's own assets requires a signed-in session
// (see auth.go).
func NewHandler(cfg Config) (http.Handler, error) {
	engineURL := cfg.EngineURL
	if len(cfg.Admin.hash) == 0 {
		return nil, errors.New("no admin credentials configured")
	}
	target, err := url.Parse(engineURL)
	if err != nil {
		return nil, fmt.Errorf("parse engine url %q: %w", engineURL, err)
	}
	if target.Scheme == "" || target.Host == "" {
		return nil, fmt.Errorf("engine url %q must include scheme and host, e.g. http://mantis-engine:8080", engineURL)
	}

	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		slog.Error("proxy", "event", "proxy_failure", "path", r.URL.Path, "engine_url", engineURL, "error", err.Error())
		http.Error(w, "engine unreachable: "+err.Error(), http.StatusBadGateway)
	}

	static, err := StaticHandler()
	if err != nil {
		return nil, err
	}

	loginPage, err := uiFiles.ReadFile("ui/login.html")
	if err != nil {
		return nil, fmt.Errorf("read login page: %w", err)
	}
	favicon, err := uiFiles.ReadFile("ui/mantis-mascot.webp")
	if err != nil {
		return nil, fmt.Errorf("read favicon: %w", err)
	}

	sess := newSessions(cfg.Admin)

	mux := http.NewServeMux()
	mux.Handle("/api/", proxy)
	mux.Handle("/api/report", reportHandler(cfg.ReportURL))
	mux.HandleFunc("/api/session", sess.handleSession)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		httpx.WriteText(w, "ok")
	})
	// Browsers probe /favicon.ico unconditionally, before ever reading the
	// page's <link rel="icon">. Answering it directly (with the mascot PNG —
	// browsers don't care that the bytes aren't literally .ico) avoids a
	// spurious 404 on first load, logged in or not.
	mux.HandleFunc("/favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/webp")
		w.Header().Set("Cache-Control", "public, max-age=86400")
		_, _ = w.Write(favicon)
	})
	mux.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write(loginPage)
		case http.MethodPost:
			sess.handleLogin(w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc("/logout", sess.handleLogout)
	mux.Handle("/", static)
	return sess.gate(mux), nil
}

// reportHandler proxies report requests to mantis-report. Responses stream
// (progress events, then the document), so they are flushed immediately.
func reportHandler(reportURL string) http.Handler {
	if reportURL == "" {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"error":"Reports are not available: the mantis-report service is not deployed."}`))
		})
	}
	target, err := url.Parse(reportURL)
	if err != nil || target.Scheme == "" || target.Host == "" {
		slog.Error("startup", "event", "bad_report_url", "report_url", reportURL)
		return reportHandler("")
	}
	p := httputil.NewSingleHostReverseProxy(target)
	p.FlushInterval = -1
	p.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		slog.Error("proxy", "event", "report_proxy_failure", "error", err.Error())
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"error":"The report service is unreachable."}`))
	}
	return p
}

// Package web is the Mantis frontend: it serves the resource-graph UI and reverse-
// proxies /api requests to the engine backend, keeping the browser same-origin.
package web

import (
	"embed"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httputil"
	"net/url"

	"github.com/continuumx1/mantis/internal/httpx"
)

// "all:" is required, not cosmetic: a bare "//go:embed ui" silently drops
// any file/dir whose name starts with "." or "_" — which is exactly how the
// Playground's cluster-scoped fixtures are named (playground/data/*/resources
// /__Node__*.yaml, __PersistentVolume__*.yaml), so without it those 404 at
// runtime with no build-time signal that anything is missing.
//go:embed all:ui
var uiFiles embed.FS

// StaticHandler serves the embedded single-page UI. The UI ships inside the
// binary — no assets to mount at runtime.
func StaticHandler() (http.Handler, error) {
	ui, err := fs.Sub(uiFiles, "ui")
	if err != nil {
		return nil, fmt.Errorf("mount ui assets: %w", err)
	}
	return http.FileServer(http.FS(ui)), nil
}

// NewHandler builds the frontend service: it serves the UI at / and reverse-
// proxies every /api/ request to the engine at engineURL. Keeping the browser
// same-origin means no CORS, and the engine never has to face the browser — it
// can stay a private ClusterIP reachable only from this service.
//
// Everything except the login page, logout, and the liveness probe sits behind
// a public-preview login gate (see auth.go) — a hardcoded admin/admin and an
// in-memory session, not a real auth story, but enough to stand a login screen
// up in front of the graph.
func NewHandler(engineURL string) (http.Handler, error) {
	target, err := url.Parse(engineURL)
	if err != nil {
		return nil, fmt.Errorf("parse engine url %q: %w", engineURL, err)
	}
	if target.Scheme == "" || target.Host == "" {
		return nil, fmt.Errorf("engine url %q must include scheme and host, e.g. http://mantis-engine:8080", engineURL)
	}

	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
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

	sess := newSessions()

	mux := http.NewServeMux()
	mux.Handle("/api/", proxy)
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

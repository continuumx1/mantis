// Package web is the KNW frontend: it serves the resource-graph UI and reverse-
// proxies /api requests to the engine backend, keeping the browser same-origin.
package web

import (
	"embed"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httputil"
	"net/url"

	"github.com/continuumx1/knw/internal/httpx"
)

//go:embed ui
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
func NewHandler(engineURL string) (http.Handler, error) {
	target, err := url.Parse(engineURL)
	if err != nil {
		return nil, fmt.Errorf("parse engine url %q: %w", engineURL, err)
	}
	if target.Scheme == "" || target.Host == "" {
		return nil, fmt.Errorf("engine url %q must include scheme and host, e.g. http://knw-engine:8080", engineURL)
	}

	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		http.Error(w, "engine unreachable: "+err.Error(), http.StatusBadGateway)
	}

	static, err := StaticHandler()
	if err != nil {
		return nil, err
	}

	mux := http.NewServeMux()
	mux.Handle("/api/", proxy)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		httpx.WriteText(w, "ok")
	})
	mux.Handle("/", static)
	return mux, nil
}

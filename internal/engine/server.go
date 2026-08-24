// Package engine is the Mantis backend: it reads the cluster through a Kubernetes
// client and serves the resource graph as JSON. It holds no UI — the web package
// renders the graph and proxies to this service.
package engine

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/continuumx1/mantis/internal/httpx"
	mantiskube "github.com/continuumx1/mantis/internal/kubernetes"
)

// buildTimeout bounds a single graph build so a hung API server cannot hang a
// request. It is also, now, the ceiling on how long the very first
// /api/graph call ever waits — every request after the first snapshot exists
// returns instantly (see handleGraph).
const buildTimeout = 30 * time.Second

// Server is the backend service. A background loop (see sync.go) reads the
// cluster on its own schedule and publishes what it finds into cache;
// requests just read the cache. showAll controls whether system-managed
// noise (service-account/Helm Secrets, the root-CA ConfigMap) is included in
// the graph; it is wired to the MANTIS_SHOW_ALL env var.
type Server struct {
	client       *mantiskube.Client
	showAll      bool
	syncInterval time.Duration
	cache        *snapshotCache
	progress     *progressTracker
}

// New constructs the backend around a Kubernetes client. It does not start
// reading the cluster yet — call Start for that, once, before serving
// traffic. A syncInterval <= 0 falls back to defaultSyncInterval.
func New(client *mantiskube.Client, showAll bool, syncInterval time.Duration) *Server {
	if syncInterval <= 0 {
		syncInterval = defaultSyncInterval
	}
	return &Server{
		client:       client,
		showAll:      showAll,
		syncInterval: syncInterval,
		cache:        newSnapshotCache(),
		progress:     &progressTracker{},
	}
}

// Start launches the background sync loop that keeps the cache warm. It
// returns immediately; the loop runs until ctx is cancelled (or, in
// practice, until the process exits — see cmd/mantis-engine, which never
// cancels the context it passes here, the same "runs until the Pod dies"
// lifetime every other piece of this service already has).
func (s *Server) Start(ctx context.Context) {
	go s.syncLoop(ctx, s.syncInterval)
}

// Handler returns the backend's routes: the graph projection, the
// background sync's own progress, and the liveness/readiness probes.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/graph", s.handleGraph)
	mux.HandleFunc("/api/sync/status", s.handleSyncStatus)
	mux.HandleFunc("/api/resource", s.handleResource)
	mux.HandleFunc("/healthz", s.handleHealth)
	mux.HandleFunc("/readyz", s.handleReady)
	return mux
}

// handleGraph serves the current cached snapshot — no cluster read happens on
// this request path at all; the background loop (sync.go) is the only thing
// that ever talks to the Kubernetes API to build one. The one exception is
// the very first request after startup, before the background loop has
// published anything yet: that request waits (bounded by buildTimeout) for
// the cache to go ready, which happens as soon as the *first namespace*
// finishes — not the whole cluster — so even a first load on a large cluster
// does not have to wait for everything.
func (s *Server) handleGraph(w http.ResponseWriter, r *http.Request) {
	dto, ok := s.cache.get()
	if !ok {
		select {
		case <-s.cache.ready:
			dto, ok = s.cache.get()
		case <-time.After(buildTimeout):
		case <-r.Context().Done():
			return
		}
	}
	if !ok {
		http.Error(w, "cluster read failed: sync has not produced any data yet", http.StatusServiceUnavailable)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if err := json.NewEncoder(w).Encode(dto); err != nil {
		http.Error(w, "encode graph: "+err.Error(), http.StatusInternalServerError)
	}
}

// handleSyncStatus reports where the background sync loop is right now — the
// namespaces-done/total count, running resource/relationship totals, and the
// last completed pass's duration/error — so the UI can show real progress
// instead of a bare spinner while a large cluster's first pass is still
// filling in.
func (s *Server) handleSyncStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(s.progress.snapshot())
}

// handleHealth is the liveness probe: the process is up. It does not touch the
// cluster, so a cluster outage never restarts the Pod.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	httpx.WriteText(w, "ok")
}

// handleReady is the readiness probe: it confirms the Kubernetes API is
// reachable, so the Pod only takes traffic once it can actually serve graphs.
func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	if _, err := s.client.Clientset.Discovery().ServerVersion(); err != nil {
		slog.Warn("probe", "event", "readyz_failed", "error", err.Error())
		http.Error(w, "cluster unreachable: "+err.Error(), http.StatusServiceUnavailable)
		return
	}
	httpx.WriteText(w, "ready")
}

// Package engine is the Mantis backend: it reads the cluster through a Kubernetes
// client and serves the resource graph as JSON. It holds no UI — the web package
// renders the graph and proxies to this service.
package engine

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/continuumx1/mantis/internal/httpx"
	mantiskube "github.com/continuumx1/mantis/internal/kubernetes"
	"github.com/continuumx1/mantis/internal/snapshotapi"
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
	// published is set once the first complete pass has been published.
	// Until then every partial snapshot is published (so a large cluster's
	// first namespaces appear quickly); after it, only complete passes are,
	// so a re-sync never briefly replaces the whole graph with a half-walked
	// one.
	published atomic.Bool
	// mode is SyncModeWatch (default) or SyncModePoll; watch holds the
	// watch-mode state once its cache has synced.
	mode  atomic.Value // string: SyncModeWatch or SyncModePoll
	watch atomic.Pointer[watchState]
	// Short-lived caches for the application pages' live reads.
	events   ttlCache[EventsDTO]
	capacity ttlCache[CapacityDTO]
	// syncing guards syncOnce against ever running twice at once. The
	// scheduled loop (syncLoop) is already sequential by construction and
	// can't overlap itself, but this makes that guarantee an explicit,
	// enforced property of syncOnce itself rather than an accident of how
	// it's currently only ever called from one place.
	syncing atomic.Bool
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

// SetSyncMode selects how the engine keeps its graph current: SyncModeWatch
// (default) or SyncModePoll. Call before Start.
func (s *Server) SetSyncMode(mode string) {
	if mode == SyncModePoll {
		s.mode.Store(SyncModePoll)
	} else {
		s.mode.Store(SyncModeWatch)
	}
}

// syncMode reports the current mode (watch unless set or fallen back).
func (s *Server) syncMode() string {
	if m, ok := s.mode.Load().(string); ok && m != "" {
		return m
	}
	return SyncModeWatch
}

// Start launches the background sync loop that keeps the cache warm. It
// returns immediately; the loop runs until ctx is cancelled (or, in
// practice, until the process exits — see cmd/mantis-engine, which never
// cancels the context it passes here, the same "runs until the Pod dies"
// lifetime every other piece of this service already has).
func (s *Server) Start(ctx context.Context) {
	if s.syncMode() == SyncModePoll {
		go s.syncLoop(ctx, s.syncInterval)
		return
	}
	go s.watchLoop(ctx)
}

// Handler returns the backend's routes: the graph projection, the
// background sync's own progress, and the liveness/readiness probes.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/graph", s.handleGraph)
	mux.HandleFunc("/api/graph/search", s.handleSearch)
	mux.HandleFunc("/api/graph/neighbors", s.handleNeighbors)
	mux.HandleFunc("/api/events", s.handleEvents)
	mux.HandleFunc("/api/cluster/capacity", s.handleCapacity)
	mux.HandleFunc(snapshotapi.Path, s.handleInternalSnapshot)
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

	// ?since=<revision>: a client that already holds a snapshot gets only
	// what changed (or "unchanged"). When the delta history no longer reaches
	// back that far, it falls through to the full snapshot — always correct.
	if v := r.URL.Query().Get("since"); v != "" {
		if since, err := strconv.ParseUint(v, 10, 64); err == nil {
			if delta, ok := s.cache.since(since); ok {
				writeJSON(w, r, delta)
				return
			}
		}
	}
	writeJSON(w, r, dto)
}

// handleSearch serves /api/graph/search?q=&limit= against the in-memory
// index. See graphIndex.search for the query syntax.
func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	limit := 50
	if v, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && v > 0 && v <= 1000 {
		limit = v
	}
	var res SearchResult
	if !s.cache.withIndex(func(ix *graphIndex) { res = ix.search(q, limit) }) {
		http.Error(w, "no graph yet", http.StatusServiceUnavailable)
		return
	}
	writeJSON(w, r, res)
}

// handleNeighbors serves /api/graph/neighbors?id=&direction=in|out|both
// &depth=1..4&categories=ownership,storage — a resource's dependency
// neighborhood, computed from the in-memory incoming/outgoing indexes.
func (s *Server) handleNeighbors(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	id := q.Get("id")
	if id == "" {
		http.Error(w, "id is required", http.StatusBadRequest)
		return
	}
	direction := q.Get("direction")
	if direction != "in" && direction != "out" {
		direction = "both"
	}
	depth := 1
	if v, err := strconv.Atoi(q.Get("depth")); err == nil && v >= 1 && v <= 4 {
		depth = v
	}
	cats := map[string]bool{}
	for _, c := range strings.Split(q.Get("categories"), ",") {
		if c = strings.TrimSpace(c); c != "" {
			cats[c] = true
		}
	}
	var res Neighborhood
	found := false
	if !s.cache.withIndex(func(ix *graphIndex) { res, found = ix.neighbors(id, direction, depth, cats) }) {
		http.Error(w, "no graph yet", http.StatusServiceUnavailable)
		return
	}
	if !found {
		http.Error(w, "resource not found in graph", http.StatusNotFound)
		return
	}
	writeJSON(w, r, res)
}

// writeJSON encodes v, gzip-compressed when the client accepts it — a large
// cluster's graph is a few megabytes of highly repetitive JSON that
// compresses roughly tenfold.
func writeJSON(w http.ResponseWriter, r *http.Request, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Add("Vary", "Accept-Encoding")
	if strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
		w.Header().Set("Content-Encoding", "gzip")
		gz := gzip.NewWriter(w)
		defer gz.Close()
		if err := json.NewEncoder(gz).Encode(v); err != nil {
			slog.Error("encode", "event", "encode_failed", "error", err.Error())
		}
		return
	}
	if err := json.NewEncoder(w).Encode(v); err != nil {
		http.Error(w, "encode: "+err.Error(), http.StatusInternalServerError)
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
	p := s.progress.snapshot()
	p.Mode = s.syncMode()
	_ = json.NewEncoder(w).Encode(p)
}

// handleHealth is the liveness probe: the process is up. It does not touch the
// cluster, so a cluster outage never restarts the Pod.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	httpx.WriteText(w, "ok")
}

// handleReady is the readiness probe: the Pod only takes traffic once it can
// actually serve a graph. It checks the cache, not the live Kubernetes API —
// deliberately: an independent live call here would compete with the
// background sync's own, already rate-limited, API calls (see
// internal/kubernetes.tuneRateLimits) for the same budget, which is exactly
// the kind of interference that could make a probe flaky *during* a long
// sync on a large cluster, the one time it matters most that it isn't. Once
// the cache holds any snapshot — even a partial one from a sync still in
// progress — Mantis can serve a graph, so it is ready, full stop; readiness
// deliberately does not flip back off if a later sync fails or the cluster
// goes unreachable; a stale-but-served snapshot (with the sync pill's own
// "connection lost" warning right there in the UI) is more useful than
// mantis-engine dropping out of its Service's endpoints and taking
// mantis-web's proxy down with it.
func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.cache.get(); !ok {
		slog.Warn("probe", "event", "readyz_failed", "reason", "no_snapshot_yet")
		http.Error(w, "sync has not produced any data yet", http.StatusServiceUnavailable)
		return
	}
	httpx.WriteText(w, "ready")
}

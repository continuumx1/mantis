// Package engine is the Mantis backend: it reads the cluster through a Kubernetes
// client and serves the resource graph as JSON. It holds no UI — the web package
// renders the graph and proxies to this service.
package engine

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"sort"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/continuumx1/mantis/internal/graph"
	"github.com/continuumx1/mantis/internal/httpx"
	mantiskube "github.com/continuumx1/mantis/internal/kubernetes"
)

// buildTimeout bounds a single graph build so a hung API server cannot hang a
// request.
const buildTimeout = 30 * time.Second

// Server is the backend service. It reads the cluster and serves the graph
// projection as JSON.
type Server struct {
	client  *mantiskube.Client
	showAll bool
}

// New constructs the backend around a Kubernetes client. showAll controls
// whether system-managed noise (service-account/Helm Secrets, the root-CA
// ConfigMap) is included in the graph; it is wired to the MANTIS_SHOW_ALL env var.
func New(client *mantiskube.Client, showAll bool) *Server {
	return &Server{client: client, showAll: showAll}
}

// Handler returns the backend's routes: the graph projection and the
// liveness/readiness probes.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/graph", s.handleGraph)
	mux.HandleFunc("/api/resource", s.handleResource)
	mux.HandleFunc("/healthz", s.handleHealth)
	mux.HandleFunc("/readyz", s.handleReady)
	return mux
}

// handleGraph builds the whole-cluster graph and writes it as JSON. A cluster
// error becomes a 502 with a plain message so the UI can show it rather than a
// blank canvas.
func (s *Server) handleGraph(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), buildTimeout)
	defer cancel()

	start := time.Now()
	slog.Info("sync", "event", "sync_start")

	gctx, skipped, err := graph.BuildClusterGraph(ctx, s.client.Clientset, s.client.Dynamic, s.showAll)
	if err != nil {
		slog.Error("sync", "event", "sync_failed", "duration_ms", time.Since(start).Milliseconds(),
			"error", err.Error(), "kind", "kubernetes_api_failure")
		http.Error(w, "cluster read failed: "+err.Error(), http.StatusBadGateway)
		return
	}
	slog.Info("sync", "event", "sync_complete", "duration_ms", time.Since(start).Milliseconds(),
		"resources_discovered", len(gctx.Nodes()), "relationships_created", len(gctx.Relations),
		"kinds_skipped", skipped)

	meta := MetaDTO{
		Context:        s.client.Context,
		Server:         s.client.Server,
		Skipped:        skipped,
		NodeAutoscaler: graph.DetectNodeAutoscaler(ctx, s.client.Dynamic, s.client.Clientset),
	}
	meta.Namespaces = countNamespaces(gctx)
	// Prefer the authoritative namespace list from the API so the UI can show a
	// region for every namespace (even empty ones), with the count following it.
	// If the list call fails, fall back to counting namespaces seen on nodes.
	if nsList, err := s.client.Clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{}); err == nil {
		names := make([]string, 0, len(nsList.Items))
		for i := range nsList.Items {
			names = append(names, nsList.Items[i].Name)
		}
		sort.Strings(names)
		meta.NamespaceList = names
		meta.Namespaces = len(names)
	}
	if v, err := s.client.Clientset.Discovery().ServerVersion(); err == nil {
		meta.Version = v.GitVersion
	}

	dto := FromContext(gctx, meta)

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if err := json.NewEncoder(w).Encode(dto); err != nil {
		http.Error(w, "encode graph: "+err.Error(), http.StatusInternalServerError)
	}
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

// countNamespaces counts distinct namespaces represented among the graph's
// visible nodes, which is what the header's "N namespaces" reflects. Hidden
// nodes (system-managed noise, e.g. the kube-root-ca.crt ConfigMap every
// namespace carries) are skipped so the count matches the namespace regions the
// UI actually draws — a namespace whose only resources are hidden gets no region
// and must not inflate the count.
func countNamespaces(gctx *graph.Context) int {
	seen := map[string]struct{}{}
	for _, n := range gctx.Nodes() {
		if n.Hidden || n.Ref.Namespace == "" {
			continue
		}
		seen[n.Ref.Namespace] = struct{}{}
	}
	return len(seen)
}

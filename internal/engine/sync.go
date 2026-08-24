package engine

import (
	"context"
	"log/slog"
	"sort"
	"sync"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/continuumx1/mantis/internal/graph"
)

// defaultSyncInterval is how often the background sync loop rebuilds the
// graph from scratch once a pass completes. It is deliberately independent
// of how often the browser polls /api/graph (that's the UI's own "3s / 10s /
// 1m / ..." choice) — every poll just reads whatever this loop last
// published, so polling faster than this interval no longer costs the
// cluster anything extra.
const defaultSyncInterval = 20 * time.Second

// SyncProgress is the /api/sync/status payload: where the background sync
// loop is right now. Namespaces carries a true X/Y count — the total is
// known up front, from the one Namespaces List call every pass starts with.
// The resource/relationship counts are running totals with no denominator:
// how many resources live in namespaces this pass has not reached yet is
// genuinely unknown until it reaches them, and a fabricated "out of N" there
// would be more misleading than no denominator at all.
type SyncProgress struct {
	Running            bool       `json:"running"`
	NamespacesDone     int        `json:"namespacesDone"`
	NamespacesTotal    int        `json:"namespacesTotal"`
	ResourcesSoFar     int        `json:"resourcesSoFar"`
	RelationshipsSoFar int        `json:"relationshipsSoFar"`
	StartedAt          *time.Time `json:"startedAt,omitempty"`
	LastSyncAt         *time.Time `json:"lastSyncAt,omitempty"`
	LastDurationMs     int64      `json:"lastDurationMs,omitempty"`
	LastError          string     `json:"lastError,omitempty"`
}

// progressTracker is SyncProgress plus the mutex guarding it — read by
// Server.handleSyncStatus, written only from within syncOnce.
type progressTracker struct {
	mu    sync.Mutex
	state SyncProgress
}

func (p *progressTracker) start(t time.Time) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.state.Running = true
	started := t
	p.state.StartedAt = &started
	p.state.LastError = ""
}

func (p *progressTracker) update(snap graph.ClusterSnapshot, dto GraphDTO) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.state.NamespacesDone = snap.NamespacesDone
	p.state.NamespacesTotal = snap.NamespacesTotal
	p.state.ResourcesSoFar = dto.Meta.NodeCount
	p.state.RelationshipsSoFar = dto.Meta.EdgeCount
}

func (p *progressTracker) finish(d time.Duration, err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.state.Running = false
	now := time.Now()
	p.state.LastSyncAt = &now
	p.state.LastDurationMs = d.Milliseconds()
	if err != nil {
		p.state.LastError = err.Error()
	}
}

func (p *progressTracker) snapshot() SyncProgress {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.state
}

// syncLoop runs syncOnce immediately, then every interval, until ctx is
// cancelled. It is the only place that ever calls syncOnce, so two syncs can
// never overlap — the next tick's call doesn't start until the previous one
// has returned, by construction of this single sequential loop.
func (s *Server) syncLoop(ctx context.Context, interval time.Duration) {
	s.syncOnce(ctx)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.syncOnce(ctx)
		}
	}
}

// syncOnce runs one whole-cluster pass, publishing a partial snapshot into
// the cache after every namespace so /api/graph never has to wait for the
// slowest part of a large cluster to see the fastest part.
func (s *Server) syncOnce(ctx context.Context) {
	ctx, cancel := context.WithTimeout(ctx, buildTimeout)
	defer cancel()

	start := time.Now()
	s.progress.start(start)
	slog.Info("sync", "event", "sync_start")

	base := s.baseMeta(ctx)
	err := graph.BuildClusterGraphProgressive(ctx, s.client.Clientset, s.client.Dynamic, s.showAll,
		func(snap graph.ClusterSnapshot) {
			dto := s.dtoFor(base, snap)
			s.cache.publish(dto)
			s.progress.update(snap, dto)
		})

	duration := time.Since(start)
	if err != nil {
		slog.Error("sync", "event", "sync_failed", "duration_ms", duration.Milliseconds(),
			"error", err.Error(), "kind", "kubernetes_api_failure")
		s.progress.finish(duration, err)
		return
	}
	slog.Info("sync", "event", "sync_complete", "duration_ms", duration.Milliseconds(),
		"resources_discovered", s.progress.snapshot().ResourcesSoFar,
		"relationships_created", s.progress.snapshot().RelationshipsSoFar)
	s.progress.finish(duration, nil)
}

// baseMeta fetches the cluster-identity fields that don't change namespace by
// namespace (context, server version, the authoritative namespace list, the
// node-autoscaler badge) once per pass, rather than once per namespace —
// dtoFor reuses this same base for every snapshot published during the pass.
func (s *Server) baseMeta(ctx context.Context) MetaDTO {
	meta := MetaDTO{
		Context:        s.client.Context,
		Server:         s.client.Server,
		NodeAutoscaler: graph.DetectNodeAutoscaler(ctx, s.client.Dynamic, s.client.Clientset),
	}
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
	return meta
}

// dtoFor projects one ClusterSnapshot into the wire format, against the
// pass's shared base metadata. When the authoritative namespace list (in
// base) could not be fetched, meta.Namespaces falls back to counting
// namespaces actually represented in this snapshot's nodes — the same
// graceful degradation handleGraph always had.
func (s *Server) dtoFor(base MetaDTO, snap graph.ClusterSnapshot) GraphDTO {
	meta := base
	meta.Skipped = snap.Skipped
	if meta.NamespaceList == nil {
		meta.Namespaces = countNamespaces(snap.Context)
	}
	return FromContext(snap.Context, meta)
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

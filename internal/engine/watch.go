package engine

import (
	"context"
	"errors"
	"log/slog"
	"sort"
	"sync/atomic"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"

	"github.com/continuumx1/mantis/internal/graph"
)

// Sync modes.
//
//   - "watch" (default): list once per kind, then follow WATCH streams; the
//     graph is rebuilt from the in-memory cache when something changes. The
//     API server sees a handful of long-lived watches instead of a full
//     re-list of the cluster every interval.
//   - "poll": the original behaviour — re-list everything every interval.
//     Kept for restricted setups where cluster-wide list/watch is not
//     allowed but per-namespace lists are.
const (
	SyncModeWatch = "watch"
	SyncModePoll  = "poll"
)

const (
	// rebuildDebounce coalesces a burst of changes (a rollout touches many
	// objects) into one rebuild.
	rebuildDebounce = 750 * time.Millisecond
	// rebuildMinGap bounds how often the graph is rebuilt under constant churn.
	rebuildMinGap = 2 * time.Second
	// safetyRebuild rebuilds periodically even with no events, and refreshes
	// the server version; it costs no API list calls.
	safetyRebuild = 5 * time.Minute
	// watchStartRetry is the wait between attempts when the initial sync fails
	// (API server unreachable).
	watchStartRetry = 10 * time.Second
	// watchStartAttempts failed starts before falling back to poll mode.
	watchStartAttempts = 3
)

// watchState is what the watch loop shares with request handlers.
type watchState struct {
	wc       *watchCache
	cs       cachedClientset
	dyn      dynamic.Interface
	version  atomic.Value // string
	rebuilds atomic.Int64
}

// readClient is the client request handlers should use for reads the cache
// can answer (nodes for capacity): the cache in watch mode, the API otherwise.
func (s *Server) readClient() kubernetes.Interface {
	if w := s.watch.Load(); w != nil {
		return w.cs
	}
	return s.client.Clientset
}

// watchLoop starts the watch cache (retrying until the API server answers),
// then rebuilds and publishes the graph whenever the cache changes. If watch
// mode cannot work — RBAC allows list but not watch, or the cache cannot sync
// after several attempts — it falls back to poll mode rather than leave the
// UI without data.
func (s *Server) watchLoop(parent context.Context) {
	ctx, stopWatch := context.WithCancel(parent)
	defer stopWatch()
	fallback := func(reason string) {
		stopWatch() // stop every informer before polling starts
		slog.Warn("watch", "event", "falling_back_to_poll", "reason", reason)
		s.mode.Store(SyncModePoll)
		s.watch.Store(nil)
		s.syncLoop(parent, s.syncInterval)
	}
	start := time.Now()
	s.progress.start(start)
	var wc *watchCache
	for attempt := 1; ; attempt++ {
		var err error
		wc, err = startWatchCache(ctx, s.client.Clientset, s.client.Dynamic)
		if err == nil {
			break
		}
		slog.Error("watch", "event", "watch_start_failed", "attempt", attempt, "error", err.Error())
		s.progress.finish(time.Since(start), err)
		if errors.Is(err, errWatchDenied) || attempt >= watchStartAttempts {
			fallback(err.Error())
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(watchStartRetry):
		}
	}
	w := &watchState{wc: wc, cs: cachedClientset{Interface: s.client.Clientset, wc: wc}}
	if s.client.Dynamic != nil {
		w.dyn = cachedDynamic{Interface: s.client.Dynamic, wc: wc}
	}
	w.version.Store(s.serverVersion())
	s.watch.Store(w)
	slog.Info("watch", "event", "watch_synced", "kinds", len(wc.typed), "crds", len(wc.dyn),
		"not_watched", len(wc.denied), "initial_sync_ms", time.Since(start).Milliseconds())

	s.rebuild(ctx, w)
	safety := time.NewTicker(safetyRebuild)
	defer safety.Stop()
	last := time.Now()
	for {
		select {
		case <-ctx.Done():
			return
		case <-wc.watchDenied:
			fallback(errWatchDenied.Error())
			return
		case <-safety.C:
			w.version.Store(s.serverVersion())
		case <-wc.changed:
			// Debounce: let a burst settle, and keep at least rebuildMinGap
			// between rebuilds.
			wait := rebuildDebounce
			if gap := rebuildMinGap - time.Since(last); gap > wait {
				wait = gap
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(wait):
			}
			// Drain a notification that arrived during the wait; this
			// rebuild covers it.
			select {
			case <-wc.changed:
			default:
			}
		}
		s.rebuild(ctx, w)
		last = time.Now()
	}
}

// rebuild builds the graph from the watch cache (no API list calls) and
// publishes it.
func (s *Server) rebuild(ctx context.Context, w *watchState) {
	start := time.Now()
	s.progress.start(start)
	ctx, cancel := context.WithTimeout(ctx, buildTimeout)
	defer cancel()
	gctx, skipped, err := graph.BuildClusterGraph(ctx, w.cs, w.dyn, s.showAll)
	if err != nil {
		if !errors.Is(err, context.Canceled) {
			slog.Error("watch", "event", "rebuild_failed", "error", err.Error())
		}
		s.progress.finish(time.Since(start), err)
		return
	}
	meta := s.metaFrom(ctx, w.cs, w.dyn, w.version.Load().(string))
	meta.Skipped = mergeSorted(skipped, w.wc.skippedResources())
	if meta.NamespaceList == nil {
		meta.Namespaces = countNamespaces(gctx)
	}
	dto := FromContext(gctx, meta)
	s.cache.publish(dto)
	s.published.Store(true)
	n := w.rebuilds.Add(1)
	s.progress.update(graph.ClusterSnapshot{Complete: true}, dto)
	s.progress.finish(time.Since(start), nil)
	slog.Debug("watch", "event", "rebuild", "n", n, "duration_ms", time.Since(start).Milliseconds(),
		"resources", dto.Meta.NodeCount, "relationships", dto.Meta.EdgeCount, "changes_seen", w.wc.events.Load())
}

func (s *Server) serverVersion() string {
	if v, err := s.client.Clientset.Discovery().ServerVersion(); err == nil {
		return v.GitVersion
	}
	return ""
}

// metaFrom assembles cluster metadata from the given (possibly cached)
// clients. Shared by both sync modes.
func (s *Server) metaFrom(ctx context.Context, cs kubernetes.Interface, dyn dynamic.Interface, version string) MetaDTO {
	meta := MetaDTO{
		Context:        s.client.Context,
		Server:         s.client.Server,
		ClusterID:      clusterID(s.client.Server),
		Version:        version,
		NodeAutoscaler: graph.DetectNodeAutoscaler(ctx, dyn, cs),
	}
	if nsList, err := cs.CoreV1().Namespaces().List(ctx, metav1.ListOptions{}); err == nil {
		names := make([]string, 0, len(nsList.Items))
		for i := range nsList.Items {
			// kube-node-lease never contributes resources, so it is not
			// counted or drawn either.
			if name := nsList.Items[i].Name; !graph.IsHiddenNamespace(name) {
				names = append(names, name)
			}
		}
		sort.Strings(names)
		meta.NamespaceList = names
		meta.Namespaces = len(names)
	}
	return meta
}

func mergeSorted(a, b []string) []string {
	set := map[string]struct{}{}
	for _, x := range a {
		set[x] = struct{}{}
	}
	for _, x := range b {
		set[x] = struct{}{}
	}
	if len(set) == 0 {
		return nil
	}
	out := make([]string, 0, len(set))
	for x := range set {
		out = append(out, x)
	}
	sort.Strings(out)
	return out
}

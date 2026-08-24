package engine

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/continuumx1/mantis/internal/graph"
	mantiskube "github.com/continuumx1/mantis/internal/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
)

// handleGraph never touches s.client — it only reads s.cache — so a Server
// built with just a cache is enough to exercise its two fast paths without
// any Kubernetes fixtures.
func newTestServer() *Server {
	return &Server{cache: newSnapshotCache(), progress: &progressTracker{}}
}

// newTestServerWithClient builds a full Server around a fake Kubernetes
// client, for tests (syncOnce, its overlap guard, context cancellation) that
// need an actual cluster read to happen. internal/kubernetes.Client.Clientset
// is kubernetes.Interface specifically so fake.NewSimpleClientset satisfies
// it here without a real cluster.
func newTestServerWithClient(clientset *fake.Clientset) *Server {
	return &Server{
		client:   &mantiskube.Client{Clientset: clientset},
		cache:    newSnapshotCache(),
		progress: &progressTracker{},
	}
}

func TestHandleReady_NotReadyBeforeFirstSnapshot(t *testing.T) {
	s := newTestServer()
	rec := httptest.NewRecorder()
	s.handleReady(rec, httptest.NewRequest("GET", "/readyz", nil))
	if rec.Code != 503 {
		t.Errorf("status = %d, want 503 before any snapshot exists", rec.Code)
	}
}

func TestHandleReady_ReadyOnceAnySnapshotExists(t *testing.T) {
	s := newTestServer()
	s.cache.publish(GraphDTO{}) // even an empty/partial snapshot counts
	rec := httptest.NewRecorder()
	s.handleReady(rec, httptest.NewRequest("GET", "/readyz", nil))
	if rec.Code != 200 {
		t.Errorf("status = %d, want 200 once a snapshot exists", rec.Code)
	}
}

func TestHandleGraph_ServesAlreadyCachedSnapshotImmediately(t *testing.T) {
	s := newTestServer()
	s.cache.publish(GraphDTO{Meta: MetaDTO{Context: "already-cached"}})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/graph", nil)

	done := make(chan struct{})
	go func() { s.handleGraph(rec, req); close(done) }()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("handleGraph did not return promptly for an already-cached snapshot")
	}

	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	if want := `"context":"already-cached"`; !strings.Contains(rec.Body.String(), want) {
		t.Errorf("body = %s, want it to contain %q", rec.Body.String(), want)
	}
}

// A request that arrives before the very first sync has published anything
// must wait, then get served the moment a (possibly partial) snapshot lands
// — not the moment the whole cluster finishes, and not fail outright just
// because it was first in line.
func TestHandleGraph_WaitsForFirstPublish(t *testing.T) {
	s := newTestServer()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/graph", nil)

	done := make(chan struct{})
	go func() { s.handleGraph(rec, req); close(done) }()

	// Give handleGraph a moment to reach its wait, then publish — simulating
	// the background loop finishing the first namespace shortly after the
	// request arrived.
	time.Sleep(20 * time.Millisecond)
	s.cache.publish(GraphDTO{Meta: MetaDTO{Context: "first-namespace-only"}})

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("handleGraph did not unblock after the first publish")
	}

	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	if want := `"context":"first-namespace-only"`; !strings.Contains(rec.Body.String(), want) {
		t.Errorf("body = %s, want it to contain %q", rec.Body.String(), want)
	}
}

func TestHandleSyncStatus_ReportsProgressState(t *testing.T) {
	s := newTestServer()
	s.progress.start(time.Now())
	snap := graph.ClusterSnapshot{NamespacesDone: 3, NamespacesTotal: 5}
	s.progress.update(snap, GraphDTO{Meta: MetaDTO{NodeCount: 42, EdgeCount: 7}})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/sync/status", nil)
	s.handleSyncStatus(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{`"running":true`, `"namespacesDone":3`, `"namespacesTotal":5`, `"resourcesSoFar":42`, `"relationshipsSoFar":7`} {
		if !strings.Contains(body, want) {
			t.Errorf("body = %s, want it to contain %q", body, want)
		}
	}
}

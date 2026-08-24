package graph

import (
	"context"
	"errors"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
)

// multiNamespaceFixtures builds two namespaces, each with one Deployment, so
// a progressive build has more than one increment to report.
func multiNamespaceFixtures() []runtime.Object {
	return []runtime.Object{
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "ns-a"}},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "ns-b"}},
		&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "app-a", Namespace: "ns-a"}},
		&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "app-b", Namespace: "ns-b"}},
	}
}

func TestBuildClusterGraphProgressive_ReportsPartialThenComplete(t *testing.T) {
	clientset := fake.NewSimpleClientset(multiNamespaceFixtures()...)

	var reports []ClusterSnapshot
	err := BuildClusterGraphProgressive(context.Background(), clientset, nil, false, func(snap ClusterSnapshot) {
		reports = append(reports, snap)
	})
	if err != nil {
		t.Fatalf("BuildClusterGraphProgressive returned error: %v", err)
	}

	if len(reports) < 2 {
		t.Fatalf("expected at least 2 progress reports (one per namespace, plus a final one), got %d", len(reports))
	}

	// Every report but the last is partial; only the last is Complete, and it
	// alone reflects every namespace.
	for i, r := range reports[:len(reports)-1] {
		if r.Complete {
			t.Errorf("report %d: Complete = true, want false (not the last report)", i)
		}
		if r.NamespacesTotal != 2 {
			t.Errorf("report %d: NamespacesTotal = %d, want 2", i, r.NamespacesTotal)
		}
	}
	last := reports[len(reports)-1]
	if !last.Complete {
		t.Error("last report: Complete = false, want true")
	}
	if last.NamespacesDone != 2 || last.NamespacesTotal != 2 {
		t.Errorf("last report: NamespacesDone/Total = %d/%d, want 2/2", last.NamespacesDone, last.NamespacesTotal)
	}

	// NamespacesDone is monotonically non-decreasing across reports.
	prev := 0
	for i, r := range reports {
		if r.NamespacesDone < prev {
			t.Errorf("report %d: NamespacesDone regressed from %d to %d", i, prev, r.NamespacesDone)
		}
		prev = r.NamespacesDone
	}

	// The final report has both Deployments as nodes.
	wantA := ResourceRef{Kind: "Deployment", Name: "app-a", Namespace: "ns-a"}
	wantB := ResourceRef{Kind: "Deployment", Name: "app-b", Namespace: "ns-b"}
	if _, ok := nodeFor(last.Context, wantA); !ok {
		t.Errorf("final report missing %+v", wantA)
	}
	if _, ok := nodeFor(last.Context, wantB); !ok {
		t.Errorf("final report missing %+v", wantB)
	}

	// An early, partial report must not have grown extra nodes after the fact —
	// it was handed a snapshot, not a view onto state the walk keeps mutating.
	first := reports[0]
	firstNodeCount := len(first.Context.Nodes())
	if len(first.Context.Nodes()) != firstNodeCount {
		t.Error("first report's node count changed after later reports were produced")
	}
}

// BuildClusterGraph (the blocking, single-result form) must still produce
// exactly what the progressive form's final report does — it is now a thin
// wrapper around BuildClusterGraphProgressive, and this pins that down.
func TestBuildClusterGraph_MatchesProgressiveFinalReport(t *testing.T) {
	clientset := fake.NewSimpleClientset(multiNamespaceFixtures()...)

	gctx, skipped, err := BuildClusterGraph(context.Background(), clientset, nil, false)
	if err != nil {
		t.Fatalf("BuildClusterGraph returned error: %v", err)
	}
	if len(skipped) != 0 {
		t.Errorf("expected nothing skipped, got %v", skipped)
	}
	if len(gctx.Nodes()) == 0 {
		t.Fatal("expected a non-empty graph")
	}

	wantA := ResourceRef{Kind: "Deployment", Name: "app-a", Namespace: "ns-a"}
	wantB := ResourceRef{Kind: "Deployment", Name: "app-b", Namespace: "ns-b"}
	if _, ok := nodeFor(gctx, wantA); !ok {
		t.Errorf("missing %+v", wantA)
	}
	if _, ok := nodeFor(gctx, wantB); !ok {
		t.Errorf("missing %+v", wantB)
	}
}

// A pass must stop at the namespace boundary where its context gets
// cancelled, rather than continuing to spend API calls on a pass that's
// already been told to stop — this is what lets the engine's own timeout
// (see internal/engine/sync.go's syncOnce) actually bound a stuck sync
// instead of just wrapping it in a context nothing downstream ever checks.
func TestBuildClusterGraphProgressive_StopsOnContextCancellation(t *testing.T) {
	fixtures := []runtime.Object{
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "ns-a"}},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "ns-b"}},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "ns-c"}},
	}
	clientset := fake.NewSimpleClientset(fixtures...)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	reports := 0
	err := BuildClusterGraphProgressive(ctx, clientset, nil, false, func(snap ClusterSnapshot) {
		reports++
		if reports == 1 {
			cancel() // cancel right after the first namespace publishes
		}
	})

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
	if reports != 1 {
		t.Errorf("reports = %d, want exactly 1 (stopped after cancellation, before namespace 2)", reports)
	}
}

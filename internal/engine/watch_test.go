package engine

import (
	"context"
	"sort"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes/fake"
	clienttesting "k8s.io/client-go/testing"

	"github.com/continuumx1/mantis/internal/graph"
	mantiskube "github.com/continuumx1/mantis/internal/kubernetes"
	"github.com/continuumx1/mantis/internal/synth"
)

func graphKeys(dto GraphDTO) (nodes, edges []string) {
	for _, n := range dto.Nodes {
		nodes = append(nodes, n.ID+"|"+n.Status)
	}
	for _, e := range dto.Edges {
		edges = append(edges, e.key())
	}
	sort.Strings(nodes)
	sort.Strings(edges)
	return
}

func countVerb(cs *fake.Clientset, verb string) int {
	n := 0
	for _, a := range cs.Actions() {
		if a.GetVerb() == verb {
			n++
		}
	}
	return n
}

func startWatchServer(t *testing.T, cs *fake.Clientset) (*Server, context.CancelFunc) {
	t.Helper()
	s := &Server{client: &mantiskube.Client{Clientset: cs, Context: "test"}, cache: newSnapshotCache(), progress: &progressTracker{}}
	ctx, cancel := context.WithCancel(context.Background())
	go s.watchLoop(ctx)
	select {
	case <-s.cache.ready:
	case <-time.After(15 * time.Second):
		cancel()
		t.Fatal("watch mode never published a graph")
	}
	return s, cancel
}

// The graph built from the watch cache must be exactly the graph built by
// listing the API — same resources, same statuses, same relationships.
func TestWatchMode_GraphMatchesPoll(t *testing.T) {
	objs := synth.Generate(synth.Options{Resources: 1500, Seed: 4})
	gctx, _, err := graph.BuildClusterGraph(context.Background(), fake.NewSimpleClientset(objs...), nil, false)
	if err != nil {
		t.Fatal(err)
	}
	polled := FromContext(gctx, MetaDTO{})

	s, cancel := startWatchServer(t, fake.NewSimpleClientset(objs...))
	defer cancel()
	watched, _ := s.cache.get()

	pn, pe := graphKeys(polled)
	wn, we := graphKeys(watched)
	if len(pn) != len(wn) || len(pe) != len(we) {
		t.Fatalf("watch graph has %d nodes/%d edges, poll graph %d/%d", len(wn), len(we), len(pn), len(pe))
	}
	for i := range pn {
		if pn[i] != wn[i] {
			t.Fatalf("node mismatch: poll %s vs watch %s", pn[i], wn[i])
		}
	}
	for i := range pe {
		if pe[i] != we[i] {
			t.Fatalf("edge mismatch: poll %s vs watch %s", pe[i], we[i])
		}
	}
}

// After the initial sync, rebuilding the graph must not issue a single LIST
// against the API server — that is the whole point of watch mode. Poll mode's
// per-pass cost is measured alongside for comparison.
func TestWatchMode_NoListCallsAfterSync(t *testing.T) {
	objs := synth.Generate(synth.Options{Resources: 3000, Seed: 5})

	pollCS := fake.NewSimpleClientset(objs...)
	if _, _, err := graph.BuildClusterGraph(context.Background(), pollCS, nil, false); err != nil {
		t.Fatal(err)
	}
	perPass := countVerb(pollCS, "list")

	cs := fake.NewSimpleClientset(objs...)
	s, cancel := startWatchServer(t, cs)
	defer cancel()
	initial := countVerb(cs, "list")
	w := s.watch.Load()
	for i := 0; i < 5; i++ {
		s.rebuild(context.Background(), w)
	}
	after := countVerb(cs, "list")
	t.Logf("poll mode: %d LIST calls every pass; watch mode: %d LIST calls once at startup (probe + initial list per kind), %d watches, then %d for 5 rebuilds",
		perPass, initial, countVerb(cs, "watch"), after-initial)
	if after != initial {
		t.Errorf("rebuilding from the watch cache issued %d LIST calls, want 0", after-initial)
	}
	if initial >= perPass {
		t.Errorf("watch startup (%d lists) should cost less than a single poll pass (%d)", initial, perPass)
	}
}

// A change in the cluster reaches the published graph through the watch, as
// a new revision with a delta — no polling interval involved.
func TestWatchMode_ChangesArriveAsDeltas(t *testing.T) {
	cs := fake.NewSimpleClientset(synth.Generate(synth.Options{Resources: 300, Seed: 6})...)
	s, cancel := startWatchServer(t, cs)
	defer cancel()
	first, _ := s.cache.get()

	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "late-arrival", Namespace: "catalog"}, Spec: corev1.PodSpec{NodeName: "worker-01"}}
	if _, err := cs.CoreV1().Pods("catalog").Create(context.Background(), pod, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "pod to appear", func() bool { d, _ := s.cache.get(); return hasNode(d, "catalog/Pod/late-arrival") })
	d, ok := s.cache.since(first.Meta.Revision)
	if !ok || d.Delta == nil {
		t.Fatalf("no delta from revision %d", first.Meta.Revision)
	}
	found := false
	for _, n := range d.Delta.UpsertNodes {
		if n.ID == "catalog/Pod/late-arrival" {
			found = true
		}
	}
	if !found {
		t.Error("the new pod is not in the delta")
	}

	if err := cs.CoreV1().Pods("catalog").Delete(context.Background(), "late-arrival", metav1.DeleteOptions{}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "pod to disappear", func() bool { d, _ := s.cache.get(); return !hasNode(d, "catalog/Pod/late-arrival") })
}

// A kind this identity may not list is reported as skipped, its references
// are unverified — and the rest of the graph still works.
func TestWatchMode_ForbiddenKindIsSkipped(t *testing.T) {
	cs := fake.NewSimpleClientset(synth.Generate(synth.Options{Resources: 300, Seed: 7})...)
	cs.PrependReactor("list", "serviceaccounts", func(clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewForbidden(schema.GroupResource{Resource: "serviceaccounts"}, "", nil)
	})
	s, cancel := startWatchServer(t, cs)
	defer cancel()
	d, _ := s.cache.get()
	skipped := false
	for _, k := range d.Meta.Skipped {
		if k == "serviceaccounts" {
			skipped = true
		}
	}
	if !skipped {
		t.Errorf("meta.skipped = %v, want serviceaccounts", d.Meta.Skipped)
	}
	if len(d.Nodes) < 100 {
		t.Errorf("graph has only %d nodes; one forbidden kind must not empty it", len(d.Nodes))
	}
}

// Secret values must never be stored in the watch cache in the first place.
func TestWatchCache_StoresSecretsWithoutValues(t *testing.T) {
	cs := fake.NewSimpleClientset(
		&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "db", Namespace: "shop",
			Annotations: map[string]string{"kubectl.kubernetes.io/last-applied-configuration": `{"data":{"pw":"x"}}`}},
			Data: map[string][]byte{"pw": []byte("hunter2")}, StringData: map[string]string{"pw": "hunter2"}},
	)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	wc, err := startWatchCache(ctx, cs, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, o := range wc.typed["secrets"].GetStore().List() {
		sec := o.(*corev1.Secret)
		if sec.Data != nil || sec.StringData != nil {
			t.Errorf("secret %s stored with values", sec.Name)
		}
		if _, ok := sec.Annotations["kubectl.kubernetes.io/last-applied-configuration"]; ok {
			t.Errorf("secret %s stored with last-applied-configuration", sec.Name)
		}
	}
	for _, a := range cs.Actions() {
		if v := a.GetVerb(); v != "list" && v != "watch" && v != "get" {
			t.Errorf("watch cache made a non-read call: %s", v)
		}
	}
}

func hasNode(d GraphDTO, id string) bool {
	for _, n := range d.Nodes {
		if n.ID == id {
			return true
		}
	}
	return false
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

// RBAC that grants list but not watch (an older chart, a hand-written
// ClusterRole) must not leave Mantis hanging: it falls back to poll mode and
// still serves the graph.
func TestWatchMode_FallsBackToPollWhenWatchForbidden(t *testing.T) {
	cs := fake.NewSimpleClientset(synth.Generate(synth.Options{Resources: 200, Seed: 8})...)
	cs.PrependWatchReactor("pods", func(clienttesting.Action) (bool, watch.Interface, error) {
		return true, nil, apierrors.NewForbidden(schema.GroupResource{Resource: "pods"}, "", nil)
	})
	s := &Server{client: &mantiskube.Client{Clientset: cs, Context: "test"}, cache: newSnapshotCache(), progress: &progressTracker{}, syncInterval: time.Hour}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.watchLoop(ctx)
	select {
	case <-s.cache.ready:
	case <-time.After(20 * time.Second):
		t.Fatal("no graph published after a forbidden watch")
	}
	waitFor(t, "fallback to poll mode", func() bool { return s.syncMode() == SyncModePoll })
	d, _ := s.cache.get()
	if !hasNode(d, d.Nodes[0].ID) || len(d.Nodes) < 50 {
		t.Errorf("poll fallback graph too small: %d nodes", len(d.Nodes))
	}
}

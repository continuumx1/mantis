package engine

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/continuumx1/mantis/internal/graph"
	"github.com/continuumx1/mantis/internal/synth"
)

func dtoWith(nodes []NodeDTO, edges []EdgeDTO) GraphDTO {
	return GraphDTO{Meta: MetaDTO{Context: "t"}, Nodes: nodes, Edges: edges}
}

func node(id, status string) NodeDTO {
	return NodeDTO{ID: id, Kind: "Pod", Name: id, Namespace: "ns", Status: status, Resolved: true}
}

func TestSnapshotCache_IdenticalPublishKeepsRevision(t *testing.T) {
	c := newSnapshotCache()
	c.publish(dtoWith([]NodeDTO{node("a", "ok")}, nil))
	c.publish(dtoWith([]NodeDTO{node("a", "ok")}, nil))
	got, _ := c.get()
	if got.Meta.Revision != 1 {
		t.Errorf("revision = %d after identical republish, want 1", got.Meta.Revision)
	}
	d, ok := c.since(1)
	if !ok || !d.Unchanged {
		t.Errorf("since(current) = %+v ok=%v, want unchanged", d, ok)
	}
}

func TestSnapshotCache_DeltaCarriesOnlyChanges(t *testing.T) {
	c := newSnapshotCache()
	e := EdgeDTO{From: "a", To: "b", Type: "selects", Category: "networking", Basis: "selector"}
	c.publish(dtoWith([]NodeDTO{node("a", "ok"), node("b", "ok"), node("c", "ok")}, []EdgeDTO{e}))
	// rev 2: b changes status, c removed, d added, edge removed.
	c.publish(dtoWith([]NodeDTO{node("a", "ok"), node("b", "crit"), node("d", "ok")}, nil))
	d, ok := c.since(1)
	if !ok || d.Delta == nil {
		t.Fatalf("since(1) = %+v ok=%v, want a delta", d, ok)
	}
	ids := func(ns []NodeDTO) []string {
		var out []string
		for _, n := range ns {
			out = append(out, n.ID)
		}
		return out
	}
	if got := strings.Join(ids(d.Delta.UpsertNodes), ","); got != "b,d" {
		t.Errorf("upserts = %s, want b,d (a is unchanged and must not be resent)", got)
	}
	if got := strings.Join(d.Delta.RemoveNodes, ","); got != "c" {
		t.Errorf("removed = %s, want c", got)
	}
	if len(d.Delta.RemoveEdges) != 1 || len(d.Delta.AddEdges) != 0 {
		t.Errorf("edges add=%v remove=%v, want one removal", d.Delta.AddEdges, d.Delta.RemoveEdges)
	}
}

func TestSnapshotCache_MergesSeveralRevisions(t *testing.T) {
	c := newSnapshotCache()
	c.publish(dtoWith([]NodeDTO{node("a", "ok")}, nil))                    // 1
	c.publish(dtoWith([]NodeDTO{node("a", "ok"), node("x", "ok")}, nil))   // 2: x added
	c.publish(dtoWith([]NodeDTO{node("a", "crit")}, nil))                  // 3: x removed, a changed
	c.publish(dtoWith([]NodeDTO{node("a", "crit"), node("x", "ok")}, nil)) // 4: x back
	d, ok := c.since(1)
	if !ok {
		t.Fatal("since(1) not answerable")
	}
	if d.Revision != 4 || len(d.Delta.RemoveNodes) != 0 || len(d.Delta.UpsertNodes) != 2 {
		t.Errorf("merged delta = %+v, want a and x upserted, nothing removed", d.Delta)
	}
}

func TestSnapshotCache_TooOldFallsBackToFull(t *testing.T) {
	c := newSnapshotCache()
	for i := 0; i < maxDeltaHistory+5; i++ {
		c.publish(dtoWith([]NodeDTO{node("a", strings.Repeat("x", i+1))}, nil))
	}
	if _, ok := c.since(1); ok {
		t.Error("since(1) beyond the delta history should not be answerable")
	}
	if _, ok := c.since(0); ok {
		t.Error("since(0) means 'I have nothing' and must get the full snapshot")
	}
}

func TestHandleGraph_SinceAndGzip(t *testing.T) {
	s := newTestServer()
	s.cache.publish(dtoWith([]NodeDTO{node("a", "ok")}, nil))
	s.cache.publish(dtoWith([]NodeDTO{node("a", "crit")}, nil))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/graph?since=1", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	s.handleGraph(rec, req)
	if rec.Header().Get("Content-Encoding") != "gzip" {
		t.Fatal("response not gzip-encoded for a gzip-accepting client")
	}
	zr, err := gzip.NewReader(rec.Body)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(zr)
	var d GraphDeltaDTO
	if err := json.Unmarshal(body, &d); err != nil {
		t.Fatal(err)
	}
	if d.Revision != 2 || d.Delta == nil || len(d.Delta.UpsertNodes) != 1 {
		t.Errorf("delta response = %s", body)
	}

	// No since: the full, backward-compatible snapshot.
	rec = httptest.NewRecorder()
	s.handleGraph(rec, httptest.NewRequest("GET", "/api/graph", nil))
	var full GraphDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &full); err != nil || len(full.Nodes) != 1 || full.Meta.Revision != 2 {
		t.Errorf("full response = %s (err %v)", rec.Body.String(), err)
	}
}

func TestSearchAndNeighbors(t *testing.T) {
	nodes := []NodeDTO{
		{ID: "shop/Deployment/web", Kind: "Deployment", Name: "web", Namespace: "shop", Labels: map[string]string{"app": "web"}},
		{ID: "shop/Pod/web-1", Kind: "Pod", Name: "web-1", Namespace: "shop", Labels: map[string]string{"app": "web"}},
		{ID: "shop/ConfigMap/web-config", Kind: "ConfigMap", Name: "web-config", Namespace: "shop"},
		{ID: "other/Pod/web-1", Kind: "Pod", Name: "web-1", Namespace: "other"},
		{ID: "/Node/n1", Kind: "Node", Name: "n1"},
	}
	edges := []EdgeDTO{
		{From: "shop/Pod/web-1", To: "shop/Deployment/web", Type: "controlled-by", Category: "ownership"},
		{From: "shop/Pod/web-1", To: "shop/ConfigMap/web-config", Type: "references", Category: "configuration"},
		{From: "shop/Pod/web-1", To: "/Node/n1", Type: "runs-on", Category: "scheduling"},
		{From: "other/Pod/web-1", To: "/Node/n1", Type: "runs-on", Category: "scheduling"},
	}
	ix := buildIndex(&GraphDTO{Nodes: nodes, Edges: edges})

	if r := ix.search("web-1", 10); r.Total != 2 {
		t.Errorf("search web-1 total = %d, want 2", r.Total)
	}
	if r := ix.search("kind:pod ns:shop", 10); r.Total != 1 || r.Results[0].ID != "shop/Pod/web-1" {
		t.Errorf("search kind:pod ns:shop = %+v", r)
	}
	if r := ix.search("label:app=web", 10); r.Total != 2 {
		t.Errorf("label search total = %d, want 2", r.Total)
	}

	out, ok := ix.neighbors("shop/Pod/web-1", "out", 1, nil)
	if !ok || len(out.Nodes) != 4 {
		t.Errorf("out neighbors = %d nodes, want pod + deployment + configmap + node", len(out.Nodes))
	}
	// Incoming from the Node must not pull in the other namespace's Pod when
	// only outgoing edges are requested.
	for _, n := range out.Nodes {
		if n.ID == "other/Pod/web-1" {
			t.Error("outgoing walk leaked into a Pod that merely shares a Node")
		}
	}
	cfg, _ := ix.neighbors("shop/Pod/web-1", "both", 2, map[string]bool{"configuration": true})
	if len(cfg.Edges) != 1 || cfg.Edges[0].Category != "configuration" {
		t.Errorf("category-filtered neighbors = %+v", cfg.Edges)
	}
	if _, ok := ix.neighbors("nope", "both", 1, nil); ok {
		t.Error("unknown id should report not found")
	}
}

// TestPayload_NeverContainsSecretValuesOrAnnotations builds a real graph from
// a Secret with data and an annotation carrying it (as kubectl's
// last-applied-configuration does) and checks neither appears anywhere in the
// serialized payload.
func TestPayload_NeverContainsSecretValuesOrAnnotations(t *testing.T) {
	const secretValue = "hunter2-super-secret"
	cs := fake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "shop"}},
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "db", Namespace: "shop",
				Annotations: map[string]string{"kubectl.kubernetes.io/last-applied-configuration": `{"stringData":{"password":"` + secretValue + `"}}`}},
			Data:       map[string][]byte{"password": []byte(secretValue)},
			StringData: map[string]string{"password": secretValue},
		},
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "p", Namespace: "shop"}, Spec: corev1.PodSpec{Containers: []corev1.Container{{
			Name: "c", Env: []corev1.EnvVar{{Name: "PW", ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "db"}, Key: "password"}}}},
		}}}},
	)
	gctx, _, err := graph.BuildClusterGraph(context.Background(), cs, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	dto := FromContext(gctx, MetaDTO{})
	c := newSnapshotCache()
	c.publish(dto)
	b, _ := json.Marshal(dto)
	if bytes.Contains(b, []byte(secretValue)) || bytes.Contains(b, []byte("last-applied")) {
		t.Fatalf("payload leaks Secret data or annotations: %s", b)
	}
	found := false
	for _, e := range dto.Edges {
		if e.To == "shop/Secret/db" {
			found = true
		}
	}
	if !found {
		t.Error("the reference to the Secret itself should still be drawn")
	}
}

func TestFromContext_EdgesAreTypedAndUnique(t *testing.T) {
	cs := fake.NewSimpleClientset(synth.Generate(synth.Options{Resources: 300, Seed: 3})...)
	gctx, _, err := graph.BuildClusterGraph(context.Background(), cs, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	dto := FromContext(gctx, MetaDTO{})
	seen := map[string]bool{}
	ids := map[string]bool{}
	for _, n := range dto.Nodes {
		if ids[n.ID] {
			t.Errorf("duplicate node id %s", n.ID)
		}
		ids[n.ID] = true
		if (n.Namespace == "") != (n.Scope == "Cluster") {
			t.Errorf("%s: scope %q inconsistent with namespace %q", n.ID, n.Scope, n.Namespace)
		}
	}
	for _, e := range dto.Edges {
		if e.Category == "" || e.Basis == "" {
			t.Errorf("untyped edge %+v", e)
		}
		if seen[e.key()] {
			t.Errorf("duplicate edge %s", e.key())
		}
		seen[e.key()] = true
		if e.Type == "selects" || e.Type == "serves" || e.Type == "applies-to" {
			// Selector and endpoint relations never cross namespaces.
			if strings.SplitN(e.From, "/", 2)[0] != strings.SplitN(e.To, "/", 2)[0] {
				t.Errorf("selector/endpoint edge crosses namespaces: %+v", e)
			}
		}
	}
}

func benchGraph(b *testing.B, n int) *graph.Context {
	cs := fake.NewSimpleClientset(synth.Generate(synth.Options{Resources: n, Seed: 1})...)
	gctx, _, err := graph.BuildClusterGraph(context.Background(), cs, nil, false)
	if err != nil {
		b.Fatal(err)
	}
	return gctx
}

func BenchmarkFromContext10k(b *testing.B) {
	gctx := benchGraph(b, 10000)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		FromContext(gctx, MetaDTO{})
	}
}

func BenchmarkPublishAndDelta10k(b *testing.B) {
	gctx := benchGraph(b, 10000)
	dto := FromContext(gctx, MetaDTO{})
	c := newSnapshotCache()
	c.publish(dto)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		next := dto
		next.Nodes = append([]NodeDTO(nil), dto.Nodes...)
		next.Nodes[i%len(next.Nodes)].Status = "changed"
		c.publish(next)
	}
}

func BenchmarkSearch10k(b *testing.B) {
	dto := FromContext(benchGraph(b, 10000), MetaDTO{})
	ix := buildIndex(&dto)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ix.search("catalog api", 50)
	}
}

func BenchmarkNeighbors10k(b *testing.B) {
	dto := FromContext(benchGraph(b, 10000), MetaDTO{})
	ix := buildIndex(&dto)
	id := ""
	for _, n := range dto.Nodes {
		if n.Kind == "Deployment" {
			id = n.ID
			break
		}
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ix.neighbors(id, "both", 3, nil)
	}
}

func TestFromContext_DropsDanglingRefsOnlyHiddenResourcesMake(t *testing.T) {
	sysBinding := graph.ResourceRef{Kind: "RoleBinding", Name: "system:x", Namespace: "kube-system"}
	ghostSA := graph.ResourceRef{Kind: "ServiceAccount", Name: "gone", Namespace: "kube-system"}
	pod := graph.ResourceRef{Kind: "Pod", Name: "p", Namespace: "shop"}
	missingCM := graph.ResourceRef{Kind: "ConfigMap", Name: "missing", Namespace: "shop"}
	gctx := graph.NewFromNodes(graph.ResourceRef{Kind: "Cluster"},
		[]graph.Relation{{From: sysBinding, Type: graph.GrantsTo, To: ghostSA}, {From: pod, Type: graph.Mounts, To: missingCM}},
		[]graph.Node{{Ref: sysBinding, Resolved: true, Hidden: true}, {Ref: ghostSA}, {Ref: pod, Resolved: true}, {Ref: missingCM}})
	dto := FromContext(gctx, MetaDTO{})
	ids := map[string]bool{}
	for _, n := range dto.Nodes {
		ids[n.ID] = true
	}
	if ids[ghostSA.String()] {
		t.Error("a missing SA referenced only by a hidden binding should not be shown")
	}
	if !ids[missingCM.String()] {
		t.Error("a missing ConfigMap referenced by a visible Pod must still be shown as not found")
	}
}

func TestEventsAndCapacity(t *testing.T) {
	now := metav1.Now()
	cs := fake.NewSimpleClientset(
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n1"}, Status: corev1.NodeStatus{
			Conditions:  []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}},
			Allocatable: corev1.ResourceList{corev1.ResourceCPU: resourceMust("4"), corev1.ResourceMemory: resourceMust("8Gi"), corev1.ResourcePods: resourceMust("110")},
			Capacity:    corev1.ResourceList{corev1.ResourceCPU: resourceMust("4"), corev1.ResourceMemory: resourceMust("8Gi")},
		}},
		&corev1.Event{ObjectMeta: metav1.ObjectMeta{Name: "e1", Namespace: "shop"}, Type: "Warning", Reason: "BackOff", Message: "restarting",
			InvolvedObject: corev1.ObjectReference{Kind: "Pod", Name: "web-1", Namespace: "shop"}, LastTimestamp: now, Count: 3},
		&corev1.Event{ObjectMeta: metav1.ObjectMeta{Name: "e2", Namespace: "shop"}, Type: "Normal", Reason: "Pulled",
			InvolvedObject: corev1.ObjectReference{Kind: "Pod", Name: "web-1", Namespace: "shop"}, LastTimestamp: now},
	)
	s := newTestServerWithClient(cs)

	rec := httptest.NewRecorder()
	s.handleEvents(rec, httptest.NewRequest("GET", "/api/events?type=Warning", nil))
	var ev EventsDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &ev); err != nil {
		t.Fatal(err)
	}
	if ev.Total != 2 || ev.Warnings != 1 || len(ev.Events) != 1 || ev.Events[0].ObjectID != "shop/Pod/web-1" || ev.Events[0].Count != 3 {
		t.Errorf("events = %+v", ev)
	}

	rec = httptest.NewRecorder()
	s.handleCapacity(rec, httptest.NewRequest("GET", "/api/cluster/capacity", nil))
	var c CapacityDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &c); err != nil {
		t.Fatal(err)
	}
	if c.Nodes != 1 || c.ReadyNodes != 1 || c.CPUAllocatable != 4000 || c.PodCapacity != 110 {
		t.Errorf("capacity = %+v", c)
	}
	// No metrics-server in a fake cluster: usage must be absent, not invented.
	if c.Metrics || c.CPUUsage != 0 || c.MetricsReason == "" {
		t.Errorf("metrics without metrics-server = %+v, want unavailable with a reason", c)
	}
	for _, a := range cs.Actions() {
		if a.GetVerb() != "list" && a.GetVerb() != "get" {
			t.Errorf("non-read call %s", a.GetVerb())
		}
	}
}

func resourceMust(s string) resource.Quantity { return resource.MustParse(s) }

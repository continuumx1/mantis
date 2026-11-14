package synth

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"k8s.io/client-go/kubernetes/fake"

	"github.com/continuumx1/mantis/internal/graph"
)

// TestGenerate_IsDeterministic guards the property every benchmark and the
// dev server depend on: the same options always give the same cluster.
func TestGenerate_IsDeterministic(t *testing.T) {
	a := Generate(Options{Resources: 400, Seed: 7})
	b := Generate(Options{Resources: 400, Seed: 7})
	if len(a) != len(b) {
		t.Fatalf("len differs: %d vs %d", len(a), len(b))
	}
	for i := range a {
		ja, _ := json.Marshal(a[i])
		jb, _ := json.Marshal(b[i])
		if string(ja) != string(jb) {
			t.Fatalf("object %d differs between runs", i)
		}
	}
}

// TestGenerate_BuildsThroughRealResolvers checks the synthetic cluster flows
// through BuildClusterGraph and yields every relationship category.
func TestGenerate_BuildsThroughRealResolvers(t *testing.T) {
	cs := fake.NewSimpleClientset(Generate(Options{Resources: 600, Seed: 1})...)
	gctx, _, err := graph.BuildClusterGraph(context.Background(), cs, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	cats := map[graph.Category]int{}
	for _, r := range gctx.Relations {
		cats[r.Type.Category()]++
	}
	for _, c := range []graph.Category{graph.CategoryOwnership, graph.CategoryScheduling, graph.CategoryNetworking,
		graph.CategoryConfiguration, graph.CategoryStorage, graph.CategorySecurity, graph.CategoryAutoscaling} {
		if cats[c] == 0 {
			t.Errorf("no %s relationships generated (got %v)", c, cats)
		}
	}
	t.Logf("nodes=%d relations=%d categories=%v", len(gctx.Nodes()), len(gctx.Relations), cats)
}

func TestGenerate_Sizes(t *testing.T) {
	if testing.Short() {
		t.Skip("large build")
	}
	for _, n := range []int{100, 1000, 5000, 10000} {
		objs := Generate(Options{Resources: n, Seed: 1})
		cs := fake.NewSimpleClientset(objs...)
		start := time.Now()
		gctx, _, err := graph.BuildClusterGraph(context.Background(), cs, nil, false)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("target=%5d objects=%5d graph nodes=%5d relations=%5d build=%v", n, len(objs), len(gctx.Nodes()), len(gctx.Relations), time.Since(start).Round(time.Millisecond))
	}
}

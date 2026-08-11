package render

import (
	"testing"

	"github.com/continuumx1/knw/internal/graph"
)

// TestNamespaceTree_Golden locks the grouped layout for a small namespace that
// exercises the ownership forest, a runs-on annotation, service selection, a
// dangling ingress backend, and PVC binding.
func TestNamespaceTree_Golden(t *testing.T) {
	nsRef := graph.ResourceRef{Kind: "Namespace", Name: "default"}
	dep := graph.ResourceRef{Kind: "Deployment", Name: "web", Namespace: "default"}
	rs := graph.ResourceRef{Kind: "ReplicaSet", Name: "web-abc", Namespace: "default"}
	pod := graph.ResourceRef{Kind: "Pod", Name: "web-abc-1", Namespace: "default"}
	node := graph.ResourceRef{Kind: "Node", Name: "node-1"}
	svc := graph.ResourceRef{Kind: "Service", Name: "web", Namespace: "default"}
	ing := graph.ResourceRef{Kind: "Ingress", Name: "web", Namespace: "default"}
	missing := graph.ResourceRef{Kind: "Service", Name: "missing-svc", Namespace: "default"}
	cm := graph.ResourceRef{Kind: "ConfigMap", Name: "web-config", Namespace: "default"}
	pvc := graph.ResourceRef{Kind: "PersistentVolumeClaim", Name: "data", Namespace: "default"}
	pv := graph.ResourceRef{Kind: "PersistentVolume", Name: "pv-1"}

	relations := []graph.Relation{
		{From: pod, Type: graph.ControlledBy, To: rs},
		{From: rs, Type: graph.ControlledBy, To: dep},
		{From: pod, Type: graph.RunsOn, To: node},
		{From: svc, Type: graph.Selects, To: pod},
		{From: ing, Type: graph.RoutesTo, To: missing},
		{From: pvc, Type: graph.BoundTo, To: pv},
	}

	existence := map[graph.ResourceRef]bool{
		dep: true, rs: true, pod: true, node: true,
		svc: true, ing: true, cm: true, pvc: true, pv: true,
		missing: false,
	}

	want := "NAMESPACE/default\n\n" +
		"WORKLOADS\n" +
		"└── Deployment/web\n" +
		"    └── ReplicaSet/web-abc\n" +
		"        └── Pod/web-abc-1  (runs-on Node/node-1)\n" +
		"\n" +
		"NETWORKING\n" +
		"└── Service/web\n" +
		"    └── selects Pod/web-abc-1\n" +
		"└── Ingress/web\n" +
		"    └── routes-to Service/missing-svc (not found)\n" +
		"\n" +
		"CONFIG & STORAGE\n" +
		"└── ConfigMap/web-config\n" +
		"└── PersistentVolumeClaim/data\n" +
		"    └── bound-to PersistentVolume/pv-1\n"

	got := NamespaceTree("default", graph.New(nsRef, relations, existence))
	if got != want {
		t.Errorf("NamespaceTree output mismatch.\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

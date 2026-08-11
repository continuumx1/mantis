package render

import (
	"testing"

	"github.com/continuumx1/knw/internal/graph"
)

// TestNamespaceTree_Golden locks the grouped layout for a small namespace that
// exercises the ownership forest with a pod health attribute and runs-on note,
// real service endpoints (serves), a dangling ingress backend, ConfigMap
// used-by links, a hidden system resource, and PVC/PV attributes.
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
	sysSecret := graph.ResourceRef{Kind: "Secret", Name: "default-token", Namespace: "default"}
	pvc := graph.ResourceRef{Kind: "PersistentVolumeClaim", Name: "data", Namespace: "default"}
	pv := graph.ResourceRef{Kind: "PersistentVolume", Name: "pv-1"}

	relations := []graph.Relation{
		{From: pod, Type: graph.ControlledBy, To: rs},
		{From: rs, Type: graph.ControlledBy, To: dep},
		{From: pod, Type: graph.RunsOn, To: node},
		{From: svc, Type: graph.Serves, To: pod},
		{From: ing, Type: graph.RoutesTo, To: missing},
		{From: pvc, Type: graph.BoundTo, To: pv},
		{From: pod, Type: graph.References, To: cm},
	}

	nodes := []graph.Node{
		{Ref: dep, Resolved: true},
		{Ref: rs, Resolved: true},
		{Ref: pod, Resolved: true, Attributes: []string{"Running · 1/1"}},
		{Ref: node, Resolved: true},
		{Ref: svc, Resolved: true},
		{Ref: ing, Resolved: true},
		{Ref: cm, Resolved: true},
		{Ref: sysSecret, Resolved: true, Hidden: true},
		{Ref: pvc, Resolved: true, Attributes: []string{"Bound", "10Gi", "RWO"}},
		{Ref: pv, Resolved: true, Attributes: []string{"10Gi", "Retain"}},
		{Ref: missing, Resolved: false},
	}

	want := "NAMESPACE/default\n\n" +
		"WORKLOADS\n" +
		"└── Deployment/web\n" +
		"    └── ReplicaSet/web-abc\n" +
		"        └── Pod/web-abc-1 (Running · 1/1)  (runs-on Node/node-1)\n" +
		"\n" +
		"NETWORKING\n" +
		"└── Service/web\n" +
		"    └── serves Pod/web-abc-1 (Running · 1/1)\n" +
		"└── Ingress/web\n" +
		"    └── routes-to Service/missing-svc (not found)\n" +
		"\n" +
		"CONFIG & STORAGE\n" +
		"└── ConfigMap/web-config\n" +
		"    └── used-by Pod/web-abc-1\n" +
		"└── PersistentVolumeClaim/data (Bound · 10Gi · RWO)\n" +
		"    └── bound-to PersistentVolume/pv-1 (10Gi · Retain)\n"

	got := NamespaceTree("default", graph.NewFromNodes(nsRef, relations, nodes), nil)
	if got != want {
		t.Errorf("NamespaceTree output mismatch.\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

// TestNamespaceTree_Skipped verifies the footer listing kinds that could not be read.
func TestNamespaceTree_Skipped(t *testing.T) {
	nsRef := graph.ResourceRef{Kind: "Namespace", Name: "default"}
	c := graph.NewFromNodes(nsRef, nil, nil)

	got := NamespaceTree("default", c, []string{"secrets", "configmaps"})
	want := "NAMESPACE/default\n\nSkipped (no access): secrets, configmaps\n"
	if got != want {
		t.Errorf("skipped footer mismatch.\n--- got ---\n%q\n--- want ---\n%q", got, want)
	}
}

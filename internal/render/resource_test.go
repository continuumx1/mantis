package render

import (
	"testing"

	"github.com/continuumx1/knw/internal/graph"
)

// TestResourceGraph_Golden locks the subject-centric view rooted at a Pod: the
// controlled-by chain expands upward, config/scheduling edges radiate outward,
// and the Service appears as an incoming (reverse-verb) relationship.
func TestResourceGraph_Golden(t *testing.T) {
	nsRef := graph.ResourceRef{Kind: "Namespace", Name: "knw-demo"}
	pod := graph.ResourceRef{Kind: "Pod", Name: "web-1", Namespace: "knw-demo"}
	rs := graph.ResourceRef{Kind: "ReplicaSet", Name: "web-abc", Namespace: "knw-demo"}
	dep := graph.ResourceRef{Kind: "Deployment", Name: "web", Namespace: "knw-demo"}
	node := graph.ResourceRef{Kind: "Node", Name: "minikube"}
	cm := graph.ResourceRef{Kind: "ConfigMap", Name: "app-config", Namespace: "knw-demo"}
	secret := graph.ResourceRef{Kind: "Secret", Name: "app-secret", Namespace: "knw-demo"}
	svc := graph.ResourceRef{Kind: "Service", Name: "web", Namespace: "knw-demo"}

	relations := []graph.Relation{
		{From: pod, Type: graph.ControlledBy, To: rs},
		{From: rs, Type: graph.ControlledBy, To: dep},
		{From: pod, Type: graph.RunsOn, To: node},
		{From: pod, Type: graph.References, To: cm},
		{From: pod, Type: graph.Mounts, To: secret},
		{From: svc, Type: graph.Selects, To: pod},
		{From: svc, Type: graph.Serves, To: pod},
	}

	nodes := []graph.Node{
		{Ref: pod, Resolved: true, Attributes: []string{"Running · 1/1"}},
		{Ref: rs, Resolved: true},
		{Ref: dep, Resolved: true},
		{Ref: node, Resolved: true},
		{Ref: cm, Resolved: true},
		{Ref: secret, Resolved: true},
		{Ref: svc, Resolved: true},
	}

	want := "Pod/web-1\n" +
		"namespace: knw-demo\n" +
		"Running · 1/1\n" +
		"\n" +
		"Relationships:\n" +
		"└── controlled-by → ReplicaSet/web-abc\n" +
		"    └── controlled-by → Deployment/web\n" +
		"└── mounts → Secret/app-secret\n" +
		"└── references → ConfigMap/app-config\n" +
		"└── runs-on → Node/minikube\n" +
		"└── selected-by ← Service/web\n" +
		"└── served-by ← Service/web\n"

	got := ResourceGraph(pod, graph.NewFromNodes(nsRef, relations, nodes))
	if got != want {
		t.Errorf("ResourceGraph output mismatch.\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

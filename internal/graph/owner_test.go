package graph

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// A mirror (static) Pod is owned by its Node, which is cluster-scoped. The
// controlled-by edge must point at the Node with no namespace, never at a
// namespaced phantom that would show up as a "not found" ghost.
func TestOwnerEdges_ClusterScopedOwnerHasNoNamespace(t *testing.T) {
	pod := ResourceRef{Kind: "Pod", Name: "etcd-minikube", Namespace: "kube-system"}
	owners := []metav1.OwnerReference{{Kind: "Node", Name: "minikube"}}

	edges := ownerEdges(pod, owners, "kube-system")

	wantNode := ResourceRef{Kind: "Node", Name: "minikube"} // Namespace deliberately empty.
	if !hasRelation(edges, pod, ControlledBy, wantNode) {
		t.Errorf("expected controlled-by edge to cluster-scoped Node with empty namespace; got %+v", edges)
	}

	phantom := ResourceRef{Kind: "Node", Name: "minikube", Namespace: "kube-system"}
	if hasRelation(edges, pod, ControlledBy, phantom) {
		t.Errorf("Node owner was incorrectly namespaced to kube-system; got %+v", edges)
	}
}

// A namespaced owner (the ordinary controller case) keeps the child's namespace.
func TestOwnerEdges_NamespacedOwnerKeepsNamespace(t *testing.T) {
	pod := ResourceRef{Kind: "Pod", Name: "web-abc-123", Namespace: "shop"}
	owners := []metav1.OwnerReference{{Kind: "ReplicaSet", Name: "web-abc"}}

	edges := ownerEdges(pod, owners, "shop")

	wantRS := ResourceRef{Kind: "ReplicaSet", Name: "web-abc", Namespace: "shop"}
	if !hasRelation(edges, pod, ControlledBy, wantRS) {
		t.Errorf("expected controlled-by edge to same-namespace ReplicaSet; got %+v", edges)
	}
}

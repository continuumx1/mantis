package engine

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

// baseMeta's namespace list/count feeds meta.NamespaceList and meta.Namespaces
// — what the UI draws one region per, and the header's "N namespaces" — so
// kube-node-lease must never appear there, on top of never appearing in the
// graph itself (see graph/cluster_test.go's equivalent).
func TestBaseMeta_HidesKubeNodeLease(t *testing.T) {
	clientset := fake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "kube-system"}},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "kube-node-lease"}},
	)
	s := newTestServerWithClient(clientset)

	meta := s.baseMeta(context.Background())

	if meta.Namespaces != 2 {
		t.Errorf("meta.Namespaces = %d, want 2 (kube-node-lease excluded)", meta.Namespaces)
	}
	for _, ns := range meta.NamespaceList {
		if ns == "kube-node-lease" {
			t.Errorf("meta.NamespaceList contains kube-node-lease: %v", meta.NamespaceList)
		}
	}
}

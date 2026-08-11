package graph

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestVerifyExistence(t *testing.T) {
	present := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "present-svc", Namespace: "default"},
	}
	clientset := fake.NewSimpleClientset(present)

	presentRef := ResourceRef{Kind: "Service", Name: "present-svc", Namespace: "default"}
	missingRef := ResourceRef{Kind: "Service", Name: "missing-svc", Namespace: "default"}
	unknownRef := ResourceRef{Kind: "Gateway", Name: "gw", Namespace: "default"}

	existence, err := VerifyExistence(
		context.Background(),
		clientset,
		[]ResourceRef{presentRef, missingRef, unknownRef, presentRef},
	)
	if err != nil {
		t.Fatalf("VerifyExistence returned error: %v", err)
	}

	if got, ok := existence[presentRef]; !ok || !got {
		t.Errorf("expected present-svc to be verified as existing, got (%v, ok=%v)", got, ok)
	}
	if got, ok := existence[missingRef]; !ok || got {
		t.Errorf("expected missing-svc to be verified as absent, got (%v, ok=%v)", got, ok)
	}
	if _, ok := existence[unknownRef]; ok {
		t.Errorf("unknown kind must be absent from the map (not checkable), but it was present")
	}
	if len(existence) != 2 {
		t.Errorf("expected exactly 2 checked refs, got %d: %+v", len(existence), existence)
	}
}

func TestTargetRefs_DistinctInOrder(t *testing.T) {
	from := ResourceRef{Kind: "Ingress", Name: "web", Namespace: "default"}
	a := ResourceRef{Kind: "Service", Name: "a", Namespace: "default"}
	b := ResourceRef{Kind: "Service", Name: "b", Namespace: "default"}

	relations := []Relation{
		{From: from, Type: RoutesTo, To: a},
		{From: from, Type: RoutesTo, To: b},
		{From: from, Type: RoutesTo, To: a}, // duplicate target
	}

	refs := TargetRefs(relations)
	if len(refs) != 2 || refs[0] != a || refs[1] != b {
		t.Errorf("expected [a, b] distinct in order, got %+v", refs)
	}
}

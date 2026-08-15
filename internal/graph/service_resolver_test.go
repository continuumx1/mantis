package graph

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func service(name string, selector map[string]string) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		Spec:       corev1.ServiceSpec{Selector: selector},
	}
}

func labeledPod(name string, lbls map[string]string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default", Labels: lbls},
	}
}

func TestResolveService_SelectsMatchingPods(t *testing.T) {
	svc := service("nginx", map[string]string{"app": "nginx"})

	clientset := fake.NewSimpleClientset(
		labeledPod("nginx-b", map[string]string{"app": "nginx"}),
		labeledPod("nginx-a", map[string]string{"app": "nginx", "tier": "web"}),
		labeledPod("other", map[string]string{"app": "redis"}),
	)

	relations, err := ResolveService(context.Background(), clientset, svc)
	if err != nil {
		t.Fatalf("ResolveService returned error: %v", err)
	}

	svcRef := ResourceRef{Kind: "Service", Name: "nginx", Namespace: "default"}
	if !hasRelation(relations, svcRef, Selects, ResourceRef{Kind: "Pod", Name: "nginx-a", Namespace: "default"}) {
		t.Errorf("missing selects edge to nginx-a; got %+v", relations)
	}
	if !hasRelation(relations, svcRef, Selects, ResourceRef{Kind: "Pod", Name: "nginx-b", Namespace: "default"}) {
		t.Errorf("missing selects edge to nginx-b; got %+v", relations)
	}
	if len(relations) != 2 {
		t.Fatalf("expected 2 selects edges, got %d: %+v", len(relations), relations)
	}

	// Deterministic ordering by pod name.
	if relations[0].To.Name != "nginx-a" || relations[1].To.Name != "nginx-b" {
		t.Errorf("relations not sorted by pod name: %+v", relations)
	}
}

func TestResolveService_EmptySelector(t *testing.T) {
	svc := service("headless", nil)

	clientset := fake.NewSimpleClientset(
		labeledPod("nginx-a", map[string]string{"app": "nginx"}),
	)

	relations, err := ResolveService(context.Background(), clientset, svc)
	if err != nil {
		t.Fatalf("ResolveService returned error: %v", err)
	}
	if len(relations) != 0 {
		t.Errorf("expected no edges for empty selector, got %+v", relations)
	}
}

func TestResolveService_NoMatchingPods(t *testing.T) {
	svc := service("nginx", map[string]string{"app": "nginx"})

	clientset := fake.NewSimpleClientset(
		labeledPod("other", map[string]string{"app": "redis"}),
	)

	relations, err := ResolveService(context.Background(), clientset, svc)
	if err != nil {
		t.Fatalf("ResolveService returned error: %v", err)
	}
	if len(relations) != 0 {
		t.Errorf("expected no edges when selector matches nothing, got %+v", relations)
	}
}

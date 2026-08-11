package graph

import (
	"context"
	"testing"

	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func svcBackend(name string) networkingv1.IngressBackend {
	return networkingv1.IngressBackend{
		Service: &networkingv1.IngressServiceBackend{Name: name},
	}
}

func TestResolveIngress_RoutesToServices(t *testing.T) {
	ing := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "default"},
		Spec: networkingv1.IngressSpec{
			DefaultBackend: &networkingv1.IngressBackend{
				Service: &networkingv1.IngressServiceBackend{Name: "default-svc"},
			},
			Rules: []networkingv1.IngressRule{
				{
					IngressRuleValue: networkingv1.IngressRuleValue{
						HTTP: &networkingv1.HTTPIngressRuleValue{
							Paths: []networkingv1.HTTPIngressPath{
								{Backend: svcBackend("api")},
								{Backend: svcBackend("api")}, // duplicate, must dedupe
							},
						},
					},
				},
			},
		},
	}

	relations, err := ResolveIngress(context.Background(), fake.NewSimpleClientset(), ing)
	if err != nil {
		t.Fatalf("ResolveIngress returned error: %v", err)
	}

	ingRef := ResourceRef{Kind: "Ingress", Name: "web", Namespace: "default"}
	if !hasRelation(relations, ingRef, RoutesTo, ResourceRef{Kind: "Service", Name: "api", Namespace: "default"}) {
		t.Errorf("missing routes-to edge to api; got %+v", relations)
	}
	if !hasRelation(relations, ingRef, RoutesTo, ResourceRef{Kind: "Service", Name: "default-svc", Namespace: "default"}) {
		t.Errorf("missing routes-to edge to default-svc; got %+v", relations)
	}
	if len(relations) != 2 {
		t.Fatalf("expected 2 deduped routes-to edges, got %d: %+v", len(relations), relations)
	}

	// Deterministic ordering by service name (api < default-svc).
	if relations[0].To.Name != "api" || relations[1].To.Name != "default-svc" {
		t.Errorf("relations not sorted by service name: %+v", relations)
	}
}

func TestResolveIngress_NoServiceBackends(t *testing.T) {
	ing := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{Name: "empty", Namespace: "default"},
		Spec: networkingv1.IngressSpec{
			Rules: []networkingv1.IngressRule{
				{IngressRuleValue: networkingv1.IngressRuleValue{HTTP: nil}},
			},
		},
	}

	relations, err := ResolveIngress(context.Background(), fake.NewSimpleClientset(), ing)
	if err != nil {
		t.Fatalf("ResolveIngress returned error: %v", err)
	}
	if len(relations) != 0 {
		t.Errorf("expected no edges when there are no service backends, got %+v", relations)
	}
}

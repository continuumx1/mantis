package graph

import (
	"context"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func hasRelation(relations []Relation, from ResourceRef, t RelationType, to ResourceRef) bool {
	for _, r := range relations {
		if r.From == from && r.Type == t && r.To == to {
			return true
		}
	}
	return false
}

func TestResolvePod_ControlledByChainAndNode(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "payment-api",
			Namespace: "default",
			OwnerReferences: []metav1.OwnerReference{
				{Kind: "ReplicaSet", Name: "payment-api-abc123"},
			},
		},
		Spec: corev1.PodSpec{NodeName: "worker-01"},
	}
	rs := &appsv1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "payment-api-abc123",
			Namespace: "default",
			OwnerReferences: []metav1.OwnerReference{
				{Kind: "Deployment", Name: "payment-api"},
			},
		},
	}

	clientset := fake.NewSimpleClientset(rs)

	relations, err := ResolvePod(context.Background(), clientset, pod)
	if err != nil {
		t.Fatalf("ResolvePod returned error: %v", err)
	}

	podRef := ResourceRef{Kind: "Pod", Name: "payment-api", Namespace: "default"}
	rsRef := ResourceRef{Kind: "ReplicaSet", Name: "payment-api-abc123", Namespace: "default"}
	deployRef := ResourceRef{Kind: "Deployment", Name: "payment-api", Namespace: "default"}
	nodeRef := ResourceRef{Kind: "Node", Name: "worker-01"}

	if !hasRelation(relations, podRef, ControlledBy, rsRef) {
		t.Errorf("missing Pod -[controlled-by]-> ReplicaSet edge; got %+v", relations)
	}
	if !hasRelation(relations, rsRef, ControlledBy, deployRef) {
		t.Errorf("missing ReplicaSet -[controlled-by]-> Deployment edge; got %+v", relations)
	}
	if !hasRelation(relations, podRef, RunsOn, nodeRef) {
		t.Errorf("missing Pod -[runs-on]-> Node edge; got %+v", relations)
	}
	if len(relations) != 3 {
		t.Errorf("expected 3 relations, got %d: %+v", len(relations), relations)
	}
}

func TestResolvePod_DirectlyCreatedUnscheduled(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "standalone", Namespace: "default"},
	}

	clientset := fake.NewSimpleClientset()

	relations, err := ResolvePod(context.Background(), clientset, pod)
	if err != nil {
		t.Fatalf("ResolvePod returned error: %v", err)
	}

	if len(relations) != 0 {
		t.Errorf("expected no relations for directly-created unscheduled pod, got %+v", relations)
	}
}

func TestResolvePod_MissingReplicaSet(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "orphan",
			Namespace: "default",
			OwnerReferences: []metav1.OwnerReference{
				{Kind: "ReplicaSet", Name: "gone-rs"},
			},
		},
	}

	// Fake client has no ReplicaSet: the Pod -> ReplicaSet edge must still be
	// reported, but there is no Deployment edge to climb to.
	clientset := fake.NewSimpleClientset()

	relations, err := ResolvePod(context.Background(), clientset, pod)
	if err != nil {
		t.Fatalf("ResolvePod returned error: %v", err)
	}

	podRef := ResourceRef{Kind: "Pod", Name: "orphan", Namespace: "default"}
	rsRef := ResourceRef{Kind: "ReplicaSet", Name: "gone-rs", Namespace: "default"}

	if !hasRelation(relations, podRef, ControlledBy, rsRef) {
		t.Errorf("missing Pod -[controlled-by]-> ReplicaSet edge; got %+v", relations)
	}
	if len(relations) != 1 {
		t.Errorf("expected exactly 1 relation, got %d: %+v", len(relations), relations)
	}
}

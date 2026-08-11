package graph

import (
	"context"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestBuildNamespaceGraph(t *testing.T) {
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "default"},
	}
	replicaSet := &appsv1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "web-abc",
			Namespace:       "default",
			OwnerReferences: []metav1.OwnerReference{{Kind: "Deployment", Name: "web"}},
		},
	}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "web-abc-1",
			Namespace:       "default",
			Labels:          map[string]string{"app": "web"},
			OwnerReferences: []metav1.OwnerReference{{Kind: "ReplicaSet", Name: "web-abc"}},
		},
		Spec: corev1.PodSpec{NodeName: "node-1"},
	}
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "default"},
		Spec:       corev1.ServiceSpec{Selector: map[string]string{"app": "web"}},
	}
	ingress := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "default"},
		Spec: networkingv1.IngressSpec{
			DefaultBackend: &networkingv1.IngressBackend{
				Service: &networkingv1.IngressServiceBackend{Name: "missing-svc"},
			},
		},
	}
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "data", Namespace: "default"},
		Spec:       corev1.PersistentVolumeClaimSpec{VolumeName: "pv-1"},
	}
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-1"}}
	pv := &corev1.PersistentVolume{ObjectMeta: metav1.ObjectMeta{Name: "pv-1"}}

	clientset := fake.NewSimpleClientset(deployment, replicaSet, pod, service, ingress, pvc, node, pv)

	c, err := BuildNamespaceGraph(context.Background(), clientset, "default")
	if err != nil {
		t.Fatalf("BuildNamespaceGraph returned error: %v", err)
	}

	depRef := ResourceRef{Kind: "Deployment", Name: "web", Namespace: "default"}
	rsRef := ResourceRef{Kind: "ReplicaSet", Name: "web-abc", Namespace: "default"}
	podRef := ResourceRef{Kind: "Pod", Name: "web-abc-1", Namespace: "default"}
	svcRef := ResourceRef{Kind: "Service", Name: "web", Namespace: "default"}
	ingRef := ResourceRef{Kind: "Ingress", Name: "web", Namespace: "default"}
	pvcRef := ResourceRef{Kind: "PersistentVolumeClaim", Name: "data", Namespace: "default"}
	nodeRef := ResourceRef{Kind: "Node", Name: "node-1"}
	pvRef := ResourceRef{Kind: "PersistentVolume", Name: "pv-1"}
	missingSvc := ResourceRef{Kind: "Service", Name: "missing-svc", Namespace: "default"}

	rels := c.Relations
	if !hasRelation(rels, podRef, ControlledBy, rsRef) {
		t.Errorf("missing Pod -> ReplicaSet controlled-by edge")
	}
	if !hasRelation(rels, rsRef, ControlledBy, depRef) {
		t.Errorf("missing ReplicaSet -> Deployment controlled-by edge")
	}
	if !hasRelation(rels, podRef, RunsOn, nodeRef) {
		t.Errorf("missing Pod -> Node runs-on edge")
	}
	if !hasRelation(rels, svcRef, Selects, podRef) {
		t.Errorf("missing Service -> Pod selects edge")
	}
	if !hasRelation(rels, ingRef, RoutesTo, missingSvc) {
		t.Errorf("missing Ingress -> Service routes-to edge")
	}
	if !hasRelation(rels, pvcRef, BoundTo, pvRef) {
		t.Errorf("missing PVC -> PV bound-to edge")
	}

	// Existence: listed resources resolve; the ingress's target does not.
	for _, ref := range []ResourceRef{depRef, rsRef, podRef, svcRef, nodeRef, pvRef} {
		if resolved, checked := c.Existence(ref); !checked || !resolved {
			t.Errorf("%s/%s: expected resolved, got (resolved=%v checked=%v)", ref.Kind, ref.Name, resolved, checked)
		}
	}
	if resolved, checked := c.Existence(missingSvc); !checked || resolved {
		t.Errorf("missing-svc: expected verified-absent, got (resolved=%v checked=%v)", resolved, checked)
	}
}

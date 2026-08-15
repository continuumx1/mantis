package graph

import (
	"context"
	"errors"
	"reflect"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/fake"
	clienttesting "k8s.io/client-go/testing"
)

func nodeFor(c *Context, ref ResourceRef) (Node, bool) {
	for _, n := range c.Nodes() {
		if n.Ref == ref {
			return n, true
		}
	}
	return Node{}, false
}

func namespaceFixtures() []runtime.Object {
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
		Spec: corev1.PodSpec{
			NodeName:   "node-1",
			Containers: []corev1.Container{{Name: "app"}},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{{
				Name:  "app",
				Ready: true,
				State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}},
			}},
		},
	}
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "default"},
		Spec:       corev1.ServiceSpec{Selector: map[string]string{"app": "web"}},
	}
	slice := &discoveryv1.EndpointSlice{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "web-xyz",
			Namespace: "default",
			Labels:    map[string]string{"kubernetes.io/service-name": "web"},
		},
		Endpoints: []discoveryv1.Endpoint{{
			TargetRef: &corev1.ObjectReference{Kind: "Pod", Name: "web-abc-1"},
		}},
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
		Spec: corev1.PersistentVolumeClaimSpec{
			VolumeName:  "pv-1",
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("1Gi")},
			},
		},
		Status: corev1.PersistentVolumeClaimStatus{Phase: corev1.ClaimBound},
	}
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-1"}}
	pv := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{Name: "pv-1"},
		Spec: corev1.PersistentVolumeSpec{
			Capacity:                      corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("1Gi")},
			PersistentVolumeReclaimPolicy: corev1.PersistentVolumeReclaimRetain,
		},
		Status: corev1.PersistentVolumeStatus{Phase: corev1.VolumeBound},
	}
	systemCM := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "kube-root-ca.crt", Namespace: "default"}}

	return []runtime.Object{deployment, replicaSet, pod, service, slice, ingress, pvc, node, pv, systemCM}
}

func TestBuildNamespaceGraph(t *testing.T) {
	clientset := fake.NewSimpleClientset(namespaceFixtures()...)

	c, skipped, err := BuildNamespaceGraph(context.Background(), clientset, nil, "default", false)
	if err != nil {
		t.Fatalf("BuildNamespaceGraph returned error: %v", err)
	}
	if len(skipped) != 0 {
		t.Errorf("expected nothing skipped, got %v", skipped)
	}

	depRef := ResourceRef{Kind: "Deployment", Name: "web", Namespace: "default"}
	rsRef := ResourceRef{Kind: "ReplicaSet", Name: "web-abc", Namespace: "default"}
	podRef := ResourceRef{Kind: "Pod", Name: "web-abc-1", Namespace: "default"}
	svcRef := ResourceRef{Kind: "Service", Name: "web", Namespace: "default"}
	pvcRef := ResourceRef{Kind: "PersistentVolumeClaim", Name: "data", Namespace: "default"}
	pvRef := ResourceRef{Kind: "PersistentVolume", Name: "pv-1"}
	missingSvc := ResourceRef{Kind: "Service", Name: "missing-svc", Namespace: "default"}
	sysCMRef := ResourceRef{Kind: "ConfigMap", Name: "kube-root-ca.crt", Namespace: "default"}

	rels := c.Relations
	if !hasRelation(rels, podRef, ControlledBy, rsRef) || !hasRelation(rels, rsRef, ControlledBy, depRef) {
		t.Errorf("missing controlled-by chain")
	}
	if !hasRelation(rels, svcRef, Selects, podRef) {
		t.Errorf("missing Service -> Pod selects edge")
	}
	if !hasRelation(rels, svcRef, Serves, podRef) {
		t.Errorf("missing Service -> Pod serves (endpoint) edge")
	}
	if !hasRelation(rels, pvcRef, BoundTo, pvRef) {
		t.Errorf("missing PVC -> PV bound-to edge")
	}

	// Attributes.
	wantPodAttrs := []string{"Running · 1/1", "requests: none", "limits: none", "probes: none"}
	if got := c.Attributes(podRef); !reflect.DeepEqual(got, wantPodAttrs) {
		t.Errorf("pod attributes = %v, want %v", got, wantPodAttrs)
	}
	if got := c.Attributes(pvcRef); !reflect.DeepEqual(got, []string{"Bound", "1Gi", "RWO"}) {
		t.Errorf("pvc attributes = %v, want [Bound 1Gi RWO]", got)
	}
	if got := c.Attributes(pvRef); !reflect.DeepEqual(got, []string{"1Gi", "Retain", "Bound"}) {
		t.Errorf("pv attributes = %v, want [1Gi Retain Bound]", got)
	}

	// Existence still honest for a dangling ingress target.
	if resolved, checked := c.Existence(missingSvc); !checked || resolved {
		t.Errorf("missing-svc: expected verified-absent, got (resolved=%v checked=%v)", resolved, checked)
	}

	// System ConfigMap exists but is hidden by default.
	if n, ok := nodeFor(c, sysCMRef); !ok || !n.Resolved || !n.Hidden {
		t.Errorf("kube-root-ca.crt: expected resolved+hidden, got %+v (found=%v)", n, ok)
	}
}

func TestBuildNamespaceGraph_ShowAll(t *testing.T) {
	clientset := fake.NewSimpleClientset(namespaceFixtures()...)

	c, _, err := BuildNamespaceGraph(context.Background(), clientset, nil, "default", true)
	if err != nil {
		t.Fatalf("BuildNamespaceGraph returned error: %v", err)
	}

	sysCMRef := ResourceRef{Kind: "ConfigMap", Name: "kube-root-ca.crt", Namespace: "default"}
	if n, ok := nodeFor(c, sysCMRef); !ok || n.Hidden {
		t.Errorf("with --all, kube-root-ca.crt must not be hidden, got %+v (found=%v)", n, ok)
	}
}

func TestBuildNamespaceGraph_SkipsForbidden(t *testing.T) {
	clientset := fake.NewSimpleClientset(namespaceFixtures()...)
	clientset.PrependReactor("list", "secrets", func(clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewForbidden(schema.GroupResource{Resource: "secrets"}, "", errors.New("forbidden"))
	})

	c, skipped, err := BuildNamespaceGraph(context.Background(), clientset, nil, "default", false)
	if err != nil {
		t.Fatalf("BuildNamespaceGraph returned error: %v", err)
	}
	if c == nil {
		t.Fatal("expected a context despite forbidden secrets")
	}

	found := false
	for _, s := range skipped {
		if s == "secrets" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'secrets' in skipped, got %v", skipped)
	}
}

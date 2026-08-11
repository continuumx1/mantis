package render

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/continuumx1/knw/internal/graph"
)

// TestPodTree_Golden locks the exact rendered layout for a Pod that exercises
// ownership, configuration (references + mounts), a confirmed-missing target,
// scheduling, and status.
func TestPodTree_Golden(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: "default"},
		Status:     corev1.PodStatus{Phase: corev1.PodRunning},
	}

	podRef := graph.ResourceRef{Kind: "Pod", Name: "app", Namespace: "default"}
	rsRef := graph.ResourceRef{Kind: "ReplicaSet", Name: "app-rs", Namespace: "default"}
	deployRef := graph.ResourceRef{Kind: "Deployment", Name: "app", Namespace: "default"}
	cfgRef := graph.ResourceRef{Kind: "ConfigMap", Name: "app-config", Namespace: "default"}
	secretRef := graph.ResourceRef{Kind: "Secret", Name: "app-tls", Namespace: "default"}
	nodeRef := graph.ResourceRef{Kind: "Node", Name: "worker-1"}

	relations := []graph.Relation{
		{From: podRef, Type: graph.ControlledBy, To: rsRef},
		{From: rsRef, Type: graph.ControlledBy, To: deployRef},
		{From: podRef, Type: graph.References, To: cfgRef},
		{From: podRef, Type: graph.Mounts, To: secretRef},
		{From: podRef, Type: graph.RunsOn, To: nodeRef},
	}

	existence := map[graph.ResourceRef]bool{
		rsRef:     true,
		deployRef: true,
		cfgRef:    true,
		secretRef: false, // confirmed missing -> "(not found)"
		nodeRef:   true,
	}

	want := "POD/app\n\n" +
		"CONTEXT\n\n" +
		"Ownership\n" +
		"  └── ReplicaSet/app-rs\n" +
		"       └── Deployment/app\n\n" +
		"References\n" +
		"  └── ConfigMap/app-config\n\n" +
		"Mounts\n" +
		"  └── Secret/app-tls (not found)\n\n" +
		"Runs on\n" +
		"  └── Node/worker-1\n\n" +
		"Status\n" +
		"  └── Running\n"

	got := PodTree(pod, graph.New(podRef, relations, existence))
	if got != want {
		t.Errorf("PodTree output mismatch.\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

// TestPodTree_DirectlyCreatedUnscheduled locks the fallback layout for a Pod
// with no owners and no node, ensuring the pre-configuration output is
// unchanged.
func TestPodTree_DirectlyCreatedUnscheduled(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "standalone", Namespace: "default"},
		Status:     corev1.PodStatus{Phase: corev1.PodPending},
	}

	want := "POD/standalone\n\n" +
		"CONTEXT\n\n" +
		"Origin\n" +
		"  └── Directly created\n\n" +
		"Owner\n" +
		"  └── None\n\n" +
		"Runs on\n" +
		"  └── Not scheduled\n\n" +
		"Status\n" +
		"  └── Pending\n"

	subject := graph.ResourceRef{Kind: "Pod", Name: "standalone", Namespace: "default"}
	got := PodTree(pod, graph.New(subject, nil, nil))
	if got != want {
		t.Errorf("PodTree output mismatch.\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

package render

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

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
		"Health\n" +
		"  └── Phase: Running\n"

	got := PodTree(pod, graph.New(podRef, relations, existence))
	if got != want {
		t.Errorf("PodTree output mismatch.\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

// TestPodTree_DirectlyCreatedUnscheduled locks the fallback layout for a Pod
// with no owners and no node, ensuring the pre-configuration output is
// unchanged.
// TestPodTree_HealthResourcesProbes locks the Resources, Probes, and per-
// container Health sections for a fully populated pod.
func TestPodTree_HealthResourcesProbes(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: "default"},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{
				Name:  "app",
				Image: "nginx",
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("100m"),
						corev1.ResourceMemory: resource.MustParse("128Mi"),
					},
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("500m"),
						corev1.ResourceMemory: resource.MustParse("256Mi"),
					},
				},
				LivenessProbe: &corev1.Probe{
					ProbeHandler: corev1.ProbeHandler{
						HTTPGet: &corev1.HTTPGetAction{Port: intstr.FromInt(80), Path: "/healthz"},
					},
				},
				ReadinessProbe: &corev1.Probe{
					ProbeHandler: corev1.ProbeHandler{
						TCPSocket: &corev1.TCPSocketAction{Port: intstr.FromInt(80)},
					},
				},
			}},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{{
				Name:         "app",
				Ready:        true,
				RestartCount: 2,
				State:        corev1.ContainerState{Running: &corev1.ContainerStateRunning{}},
			}},
		},
	}

	subject := graph.ResourceRef{Kind: "Pod", Name: "app", Namespace: "default"}

	want := "POD/app\n\n" +
		"CONTEXT\n\n" +
		"Origin\n" +
		"  └── Directly created\n\n" +
		"Owner\n" +
		"  └── None\n\n" +
		"Resources\n" +
		"  └── app: requests cpu=100m mem=128Mi; limits cpu=500m mem=256Mi\n\n" +
		"Probes\n" +
		"  └── app: liveness http :80/healthz, readiness tcp :80\n\n" +
		"Runs on\n" +
		"  └── Not scheduled\n\n" +
		"Health\n" +
		"  └── Phase: Running\n" +
		"  └── app: running, ready, restarts: 2\n"

	got := PodTree(pod, graph.New(subject, nil, nil))
	if got != want {
		t.Errorf("PodTree output mismatch.\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

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
		"Health\n" +
		"  └── Phase: Pending\n"

	subject := graph.ResourceRef{Kind: "Pod", Name: "standalone", Namespace: "default"}
	got := PodTree(pod, graph.New(subject, nil, nil))
	if got != want {
		t.Errorf("PodTree output mismatch.\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

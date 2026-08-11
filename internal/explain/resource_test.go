package explain

import (
	"context"
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestMapResource_PodSubject(t *testing.T) {
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "demo"},
	}
	replicaSet := &appsv1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "web-abc",
			Namespace:       "demo",
			OwnerReferences: []metav1.OwnerReference{{Kind: "Deployment", Name: "web"}},
		},
	}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "web-abc-1",
			Namespace:       "demo",
			OwnerReferences: []metav1.OwnerReference{{Kind: "ReplicaSet", Name: "web-abc"}},
		},
		Spec: corev1.PodSpec{NodeName: "node-1"},
	}
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-1"}}

	clientset := fake.NewSimpleClientset(deployment, replicaSet, pod, node)

	// "po" is not a known alias; "pod" is.
	if _, err := MapResource(context.Background(), clientset, "demo", "widget", "x"); err == nil {
		t.Errorf("expected error for unsupported kind")
	}
	if _, err := MapResource(context.Background(), clientset, "demo", "pod", "does-not-exist"); err == nil {
		t.Errorf("expected error for missing resource")
	}

	out, err := MapResource(context.Background(), clientset, "demo", "pod", "web-abc-1")
	if err != nil {
		t.Fatalf("MapResource returned error: %v", err)
	}

	for _, want := range []string{
		"Pod/web-abc-1",
		"controlled-by → ReplicaSet/web-abc",
		"controlled-by → Deployment/web",
		"runs-on → Node/node-1",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("MapResource output missing %q:\n%s", want, out)
		}
	}
}

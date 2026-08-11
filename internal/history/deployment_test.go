package history

import (
	"context"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"
)

func boolPtr(b bool) *bool { return &b }

func replicaSet(name string, revision string, ownerUID types.UID, image string, changeCause string) *appsv1.ReplicaSet {
	annotations := map[string]string{"deployment.kubernetes.io/revision": revision}
	if changeCause != "" {
		annotations["kubernetes.io/change-cause"] = changeCause
	}
	return &appsv1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   "default",
			Annotations: annotations,
			OwnerReferences: []metav1.OwnerReference{
				{Kind: "Deployment", Name: "web", UID: ownerUID, Controller: boolPtr(true)},
			},
		},
		Spec: appsv1.ReplicaSetSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: "web", Image: image}},
				},
			},
		},
	}
}

func TestDeploymentRevisions(t *testing.T) {
	const uid = types.UID("dep-uid")

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "default", UID: uid},
	}
	rs1 := replicaSet("web-1", "1", uid, "nginx:1.24", "")
	rs2 := replicaSet("web-2", "2", uid, "nginx:1.25", "kubectl set image")

	// A ReplicaSet owned by a different deployment must be ignored.
	other := replicaSet("other-1", "1", types.UID("other-uid"), "redis:7", "")

	clientset := fake.NewSimpleClientset(deployment, rs1, rs2, other)

	revisions, err := DeploymentRevisions(context.Background(), clientset, "default", "web")
	if err != nil {
		t.Fatalf("DeploymentRevisions returned error: %v", err)
	}

	if len(revisions) != 2 {
		t.Fatalf("expected 2 revisions, got %d: %+v", len(revisions), revisions)
	}

	// Oldest first.
	if revisions[0].Number != 1 || !revisions[0].Initial {
		t.Errorf("revision 1 should be initial; got %+v", revisions[0])
	}
	if len(revisions[0].Changes) != 0 {
		t.Errorf("initial revision should have no changes; got %+v", revisions[0].Changes)
	}

	if revisions[1].Number != 2 {
		t.Errorf("expected revision 2 second, got %d", revisions[1].Number)
	}
	if revisions[1].ChangeCause != "kubectl set image" {
		t.Errorf("expected change cause, got %q", revisions[1].ChangeCause)
	}
	if len(revisions[1].Changes) != 1 || revisions[1].Changes[0] != "container web image: nginx:1.24 → nginx:1.25" {
		t.Errorf("expected image-change line, got %+v", revisions[1].Changes)
	}
}

func TestDiffTemplates_EnvChange(t *testing.T) {
	prev := &corev1.PodTemplateSpec{
		Spec: corev1.PodSpec{Containers: []corev1.Container{{
			Name:  "app",
			Image: "app:1",
			Env:   []corev1.EnvVar{{Name: "LOG_LEVEL", Value: "info"}},
		}}},
	}
	cur := &corev1.PodTemplateSpec{
		Spec: corev1.PodSpec{Containers: []corev1.Container{{
			Name:  "app",
			Image: "app:1",
			Env:   []corev1.EnvVar{{Name: "LOG_LEVEL", Value: "debug"}},
		}}},
	}

	changes := diffTemplates(prev, cur)
	if len(changes) != 1 || changes[0] != "env LOG_LEVEL: info → debug" {
		t.Errorf("expected env-change line, got %+v", changes)
	}
}

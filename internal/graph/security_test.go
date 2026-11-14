package graph

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	storagev1 "k8s.io/api/storage/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/fake"
	clienttesting "k8s.io/client-go/testing"
)

func securityFixtures() []runtime.Object {
	sc := "fast"
	return []runtime.Object{
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "api-1", Namespace: "shop", UID: "uid-api-1", Labels: map[string]string{"app": "api"},
				Annotations: map[string]string{"kubectl.kubernetes.io/last-applied-configuration": `{"data":{"password":"aHVudGVyMg=="}}`}},
			Spec: corev1.PodSpec{ServiceAccountName: "api", NodeName: "n1"},
		},
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "web-1", Namespace: "shop", Labels: map[string]string{"app": "web"}}},
		// Same labels, different namespace: a NetworkPolicy in "shop" must not select it.
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "api-1", Namespace: "other", Labels: map[string]string{"app": "api"}}},
		&corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "shop"}},
		&corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: "shop"}},
		&corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: "prometheus", Namespace: "monitoring"}},
		&rbacv1.Role{ObjectMeta: metav1.ObjectMeta{Name: "reader", Namespace: "shop"}, Rules: []rbacv1.PolicyRule{{Verbs: []string{"get", "list"}}}},
		&rbacv1.RoleBinding{
			ObjectMeta: metav1.ObjectMeta{Name: "api-reader", Namespace: "shop"},
			RoleRef:    rbacv1.RoleRef{Kind: "Role", Name: "reader"},
			Subjects: []rbacv1.Subject{
				{Kind: "ServiceAccount", Name: "api"},                                 // defaults to the binding's namespace
				{Kind: "ServiceAccount", Name: "prometheus", Namespace: "monitoring"}, // cross-namespace grant
				{Kind: "User", Name: "alice"},                                         // not a resource: no edge
			},
		},
		&rbacv1.RoleBinding{
			ObjectMeta: metav1.ObjectMeta{Name: "view", Namespace: "shop"},
			RoleRef:    rbacv1.RoleRef{Kind: "ClusterRole", Name: "view"},
			Subjects:   []rbacv1.Subject{{Kind: "ServiceAccount", Name: "api"}},
		},
		&networkingv1.NetworkPolicy{
			ObjectMeta: metav1.ObjectMeta{Name: "api-only", Namespace: "shop"},
			Spec:       networkingv1.NetworkPolicySpec{PodSelector: metav1.LabelSelector{MatchLabels: map[string]string{"app": "api"}}},
		},
		&corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "data", Namespace: "shop"}, Spec: corev1.PersistentVolumeClaimSpec{StorageClassName: &sc, VolumeName: "pv-1"}},
		&corev1.PersistentVolume{ObjectMeta: metav1.ObjectMeta{Name: "pv-1"}, Spec: corev1.PersistentVolumeSpec{StorageClassName: "fast"}},
		&storagev1.StorageClass{ObjectMeta: metav1.ObjectMeta{Name: "fast"}, Provisioner: "csi.example"},
		&rbacv1.ClusterRole{ObjectMeta: metav1.ObjectMeta{Name: "view"}},
		&rbacv1.ClusterRole{ObjectMeta: metav1.ObjectMeta{Name: "unused-aggregate"}},
		&rbacv1.ClusterRoleBinding{
			ObjectMeta: metav1.ObjectMeta{Name: "system:controller"},
			RoleRef:    rbacv1.RoleRef{Kind: "ClusterRole", Name: "unused-aggregate"},
			Subjects:   []rbacv1.Subject{{Kind: "ServiceAccount", Name: "x", Namespace: "kube-system"}},
		},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "shop"}},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "other"}},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "monitoring"}},
	}
}

func TestBuildNamespaceGraph_SecurityAndIdentityRelations(t *testing.T) {
	cs := fake.NewSimpleClientset(securityFixtures()...)
	gctx, _, err := BuildNamespaceGraph(context.Background(), cs, nil, "shop", false)
	if err != nil {
		t.Fatal(err)
	}
	pod := ResourceRef{Kind: "Pod", Name: "api-1", Namespace: "shop"}
	rb := ResourceRef{Kind: "RoleBinding", Name: "api-reader", Namespace: "shop"}
	cases := []struct {
		name     string
		from     ResourceRef
		t        RelationType
		to       ResourceRef
		expected bool
	}{
		{"pod runs as its service account", pod, UsesServiceAccount, ResourceRef{Kind: "ServiceAccount", Name: "api", Namespace: "shop"}, true},
		{"pod without serviceAccountName uses default", ResourceRef{Kind: "Pod", Name: "web-1", Namespace: "shop"}, UsesServiceAccount, ResourceRef{Kind: "ServiceAccount", Name: "default", Namespace: "shop"}, true},
		{"rolebinding binds its role", rb, BindsRole, ResourceRef{Kind: "Role", Name: "reader", Namespace: "shop"}, true},
		{"rolebinding grants to SA (implicit namespace)", rb, GrantsTo, ResourceRef{Kind: "ServiceAccount", Name: "api", Namespace: "shop"}, true},
		{"rolebinding grants cross-namespace", rb, GrantsTo, ResourceRef{Kind: "ServiceAccount", Name: "prometheus", Namespace: "monitoring"}, true},
		{"rolebinding to clusterrole is cluster-scoped", ResourceRef{Kind: "RoleBinding", Name: "view", Namespace: "shop"}, BindsRole, ResourceRef{Kind: "ClusterRole", Name: "view"}, true},
		{"networkpolicy selects matching pod", ResourceRef{Kind: "NetworkPolicy", Name: "api-only", Namespace: "shop"}, AppliesTo, pod, true},
		{"networkpolicy skips non-matching pod", ResourceRef{Kind: "NetworkPolicy", Name: "api-only", Namespace: "shop"}, AppliesTo, ResourceRef{Kind: "Pod", Name: "web-1", Namespace: "shop"}, false},
		{"networkpolicy never crosses namespaces", ResourceRef{Kind: "NetworkPolicy", Name: "api-only", Namespace: "shop"}, AppliesTo, ResourceRef{Kind: "Pod", Name: "api-1", Namespace: "other"}, false},
		{"pvc names its storage class", ResourceRef{Kind: "PersistentVolumeClaim", Name: "data", Namespace: "shop"}, UsesStorageClass, ResourceRef{Kind: "StorageClass", Name: "fast"}, true},
	}
	for _, c := range cases {
		if got := hasRelation(gctx.Relations, c.from, c.t, c.to); got != c.expected {
			t.Errorf("%s: has %s -%s-> %s = %v, want %v", c.name, c.from, c.t, c.to, got, c.expected)
		}
	}
	for _, r := range gctx.Relations {
		if r.Type == GrantsTo && r.To.Kind != "ServiceAccount" {
			t.Errorf("grants-to edge to a non-resource subject: %+v", r)
		}
	}

	// The default ServiceAccount is hidden noise unless showAll.
	if n, _ := nodeFor(gctx, ResourceRef{Kind: "ServiceAccount", Name: "default", Namespace: "shop"}); !n.Hidden {
		t.Error("default ServiceAccount should be hidden when showAll=false")
	}
	// StorageClass and ClusterRole are only listed by the cluster pass, so in
	// a namespace pass they are unverified — never claimed missing.
	if n, ok := nodeFor(gctx, ResourceRef{Kind: "StorageClass", Name: "fast"}); !ok || n.Resolved || !n.Unverified {
		t.Errorf("StorageClass in namespace pass = %+v, want unresolved and unverified", n)
	}
}

func TestBuildNamespaceGraph_CapturesIdentityButNeverAnnotations(t *testing.T) {
	cs := fake.NewSimpleClientset(securityFixtures()...)
	gctx, _, err := BuildNamespaceGraph(context.Background(), cs, nil, "shop", false)
	if err != nil {
		t.Fatal(err)
	}
	n, ok := nodeFor(gctx, ResourceRef{Kind: "Pod", Name: "api-1", Namespace: "shop"})
	if !ok {
		t.Fatal("pod missing")
	}
	if n.Meta.UID != "uid-api-1" || n.Meta.APIVersion != "v1" || n.Meta.Labels["app"] != "api" {
		t.Errorf("meta = %+v, want uid, apiVersion and labels captured", n.Meta)
	}
}

func TestBuildNamespaceGraph_ForbiddenKindIsUnverifiedNotMissing(t *testing.T) {
	cs := fake.NewSimpleClientset(securityFixtures()...)
	cs.PrependReactor("list", "serviceaccounts", func(clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewForbidden(schema.GroupResource{Resource: "serviceaccounts"}, "", nil)
	})
	gctx, skipped, err := BuildNamespaceGraph(context.Background(), cs, nil, "shop", false)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, s := range skipped {
		if s == "serviceaccounts" {
			found = true
		}
	}
	if !found {
		t.Errorf("skipped = %v, want serviceaccounts recorded", skipped)
	}
	n, ok := nodeFor(gctx, ResourceRef{Kind: "ServiceAccount", Name: "api", Namespace: "shop"})
	if !ok || n.Resolved || !n.Unverified {
		t.Errorf("SA when listing is forbidden = %+v (present=%v), want unresolved+unverified", n, ok)
	}
}

func TestBuildNamespaceGraph_UnknownOwnerKindIsUnverified(t *testing.T) {
	cs := fake.NewSimpleClientset(&corev1.Pod{ObjectMeta: metav1.ObjectMeta{
		Name: "rollout-abc", Namespace: "shop",
		OwnerReferences: []metav1.OwnerReference{{Kind: "Rollout", Name: "web"}}, // an Argo CRD Mantis does not list
	}})
	gctx, _, err := BuildNamespaceGraph(context.Background(), cs, nil, "shop", false)
	if err != nil {
		t.Fatal(err)
	}
	n, ok := nodeFor(gctx, ResourceRef{Kind: "Rollout", Name: "web", Namespace: "shop"})
	if !ok || n.Resolved || !n.Unverified {
		t.Errorf("CRD owner = %+v, want unverified (never checked), not 'not found'", n)
	}
}

func TestBuildClusterGraph_ClusterScopedResolutionAndVisibility(t *testing.T) {
	cs := fake.NewSimpleClientset(securityFixtures()...)
	gctx, _, err := BuildClusterGraph(context.Background(), cs, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	sc, _ := nodeFor(gctx, ResourceRef{Kind: "StorageClass", Name: "fast"})
	if !sc.Resolved {
		t.Errorf("StorageClass after cluster pass = %+v, want resolved", sc)
	}
	if !hasRelation(gctx.Relations, ResourceRef{Kind: "PersistentVolume", Name: "pv-1"}, UsesStorageClass, ResourceRef{Kind: "StorageClass", Name: "fast"}) {
		t.Error("PV -storage-class-> StorageClass edge missing")
	}
	if n, _ := nodeFor(gctx, ResourceRef{Kind: "ClusterRole", Name: "view"}); n.Hidden {
		t.Error("ClusterRole granted by a visible RoleBinding should be visible")
	}
	if n, _ := nodeFor(gctx, ResourceRef{Kind: "ClusterRole", Name: "unused-aggregate"}); !n.Hidden {
		t.Error("ClusterRole only used by a hidden system binding should be hidden")
	}
	if n, _ := nodeFor(gctx, ResourceRef{Kind: "ClusterRoleBinding", Name: "system:controller"}); !n.Hidden {
		t.Error("system: ClusterRoleBinding should be hidden")
	}
	// Duplicate prevention: every relation appears once.
	seen := map[string]bool{}
	for _, r := range gctx.Relations {
		if seen[r.Key()] {
			t.Errorf("duplicate relation %s", r.Key())
		}
		seen[r.Key()] = true
	}
}

func TestBuildClusterGraph_MissingStorageClassBecomesVerifiedAbsent(t *testing.T) {
	sc := "gone"
	cs := fake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "shop"}},
		&corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "d", Namespace: "shop"}, Spec: corev1.PersistentVolumeClaimSpec{StorageClassName: &sc}},
	)
	gctx, _, err := BuildClusterGraph(context.Background(), cs, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	n, ok := nodeFor(gctx, ResourceRef{Kind: "StorageClass", Name: "gone"})
	if !ok || n.Resolved || n.Unverified {
		t.Errorf("StorageClass listed and absent = %+v, want verified not-found", n)
	}
}

// TestBuildClusterGraph_IsReadOnly records every API call a full cluster read
// makes and fails on anything but list/get — Mantis must never write.
func TestBuildClusterGraph_IsReadOnly(t *testing.T) {
	cs := fake.NewSimpleClientset(securityFixtures()...)
	if _, _, err := BuildClusterGraph(context.Background(), cs, nil, true); err != nil {
		t.Fatal(err)
	}
	for _, a := range cs.Actions() {
		if v := a.GetVerb(); v != "list" && v != "get" {
			t.Errorf("non-read API call: %s %s", v, a.GetResource().Resource)
		}
	}
}

func TestRelationTypes_AllHaveCategoryBasisEvidence(t *testing.T) {
	for typ, m := range relationMeta {
		if m.category == "" || m.basis == "" || m.evidence == "" {
			t.Errorf("%s: incomplete metadata %+v", typ, m)
		}
	}
	if Serves.Basis() != BasisEndpoint || Selects.Basis() != BasisSelector || ControlledBy.Basis() != BasisOwnership {
		t.Error("basis mapping changed for core relation types")
	}
	if (Relation{Type: Selects, Certainty: Inferred}).Basis() != BasisInferred {
		t.Error("an inferred relation must report BasisInferred regardless of type")
	}
}

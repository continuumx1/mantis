package graph

import (
	"context"
	"fmt"
	"sort"
	"strings"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
)

// kindForResource maps the plural resource names recorded in "skipped" back
// to the Kind a reference carries, so a forbidden List can mark references to
// that kind as unverified instead of missing.
var kindForResource = map[string]string{
	"deployments":              "Deployment",
	"replicasets":              "ReplicaSet",
	"statefulsets":             "StatefulSet",
	"daemonsets":               "DaemonSet",
	"jobs":                     "Job",
	"cronjobs":                 "CronJob",
	"pods":                     "Pod",
	"services":                 "Service",
	"endpointslices":           "EndpointSlice",
	"ingresses":                "Ingress",
	"configmaps":               "ConfigMap",
	"secrets":                  "Secret",
	"persistentvolumeclaims":   "PersistentVolumeClaim",
	"persistentvolumes":        "PersistentVolume",
	"resourcequotas":           "ResourceQuota",
	"limitranges":              "LimitRange",
	"horizontalpodautoscalers": "HorizontalPodAutoscaler",
	"nodes":                    "Node",
	"serviceaccounts":          "ServiceAccount",
	"roles":                    "Role",
	"rolebindings":             "RoleBinding",
	"networkpolicies":          "NetworkPolicy",
	"storageclasses":           "StorageClass",
	"clusterroles":             "ClusterRole",
	"clusterrolebindings":      "ClusterRoleBinding",
}

// namespaceListedKinds is every kind BuildNamespaceGraph lists. A dangling
// reference to any other kind (a CRD owner such as an Argo Rollout, or a
// cluster-scoped kind only the cluster pass lists) was never checked by the
// namespace pass, so it must not be reported as verified-absent there.
var namespaceListedKinds = map[string]bool{
	"Deployment": true, "ReplicaSet": true, "StatefulSet": true, "DaemonSet": true,
	"Job": true, "CronJob": true, "Pod": true, "Service": true, "Ingress": true,
	"ConfigMap": true, "Secret": true, "PersistentVolumeClaim": true,
	"PersistentVolume": true, "ResourceQuota": true, "LimitRange": true,
	"HorizontalPodAutoscaler": true, "Node": true, "ServiceAccount": true,
	"Role": true, "RoleBinding": true, "NetworkPolicy": true,
}

// clusterListedKinds are the cluster-scoped kinds listed once per pass by
// collectClusterScoped rather than once per namespace.
var clusterListedKinds = map[string]bool{
	"StorageClass": true, "ClusterRole": true, "ClusterRoleBinding": true,
}

// metaOf copies the identity metadata Mantis keeps for a listed object. Only
// labels are copied — never annotations (see ResourceMeta).
func metaOf(apiVersion string, om *metav1.ObjectMeta) ResourceMeta {
	m := ResourceMeta{
		UID:             string(om.UID),
		APIVersion:      apiVersion,
		ResourceVersion: om.ResourceVersion,
	}
	if !om.CreationTimestamp.IsZero() {
		m.CreatedAt = om.CreationTimestamp.UTC().Format("2006-01-02T15:04:05Z")
	}
	if len(om.Labels) > 0 {
		m.Labels = make(map[string]string, len(om.Labels))
		for k, v := range om.Labels {
			m.Labels[k] = v
		}
	}
	return m
}

// podServiceAccount returns the ServiceAccount a Pod runs as. An empty
// spec.serviceAccountName means the namespace's "default" account.
func podServiceAccount(p *corev1.Pod) string {
	if p.Spec.ServiceAccountName != "" {
		return p.Spec.ServiceAccountName
	}
	if p.Spec.DeprecatedServiceAccount != "" {
		return p.Spec.DeprecatedServiceAccount
	}
	return "default"
}

// isSystemRBACName reports RBAC objects the control plane manages itself
// ("system:" prefix). They are hidden unless showAll, like other
// cluster-managed noise.
func isSystemRBACName(name string) bool {
	return strings.HasPrefix(name, "system:")
}

// collected is the partial graph a collector contributes.
type collected struct {
	nodes     map[ResourceRef]struct{}
	meta      map[ResourceRef]ResourceMeta
	attrs     map[ResourceRef][]string
	hidden    map[ResourceRef]struct{}
	relations []Relation
}

func newCollected() *collected {
	return &collected{
		nodes:  map[ResourceRef]struct{}{},
		meta:   map[ResourceRef]ResourceMeta{},
		attrs:  map[ResourceRef][]string{},
		hidden: map[ResourceRef]struct{}{},
	}
}

func (c *collected) add(r ResourceRef, m ResourceMeta, attrs []string, hidden bool) {
	c.nodes[r] = struct{}{}
	c.meta[r] = m
	if len(attrs) > 0 {
		c.attrs[r] = attrs
	}
	if hidden {
		c.hidden[r] = struct{}{}
	}
}

// collectNamespaceSecurity lists ServiceAccounts, Roles, RoleBindings and
// NetworkPolicies in one namespace. forbidden is BuildNamespaceGraph's own
// handler, so an RBAC denial is recorded as skipped exactly like every other
// kind rather than failing the map.
func collectNamespaceSecurity(
	ctx context.Context,
	clientset kubernetes.Interface,
	namespace string,
	pods []corev1.Pod,
	showAll bool,
	forbidden func(kind string, err error) bool,
) (*collected, error) {
	out := newCollected()
	opts := metav1.ListOptions{}

	if list, err := clientset.CoreV1().ServiceAccounts(namespace).List(ctx, opts); err != nil {
		if !forbidden("serviceaccounts", err) {
			return nil, fmt.Errorf("list serviceaccounts: %w", err)
		}
	} else {
		for i := range list.Items {
			sa := &list.Items[i]
			r := ResourceRef{Kind: "ServiceAccount", Name: sa.Name, Namespace: namespace}
			// Every namespace has a "default" account and most Pods run as
			// it; drawing those edges would bury every real identity.
			out.add(r, metaOf("v1", &sa.ObjectMeta), nil, !showAll && sa.Name == "default")
		}
	}

	if list, err := clientset.RbacV1().Roles(namespace).List(ctx, opts); err != nil {
		if !forbidden("roles", err) {
			return nil, fmt.Errorf("list roles: %w", err)
		}
	} else {
		for i := range list.Items {
			role := &list.Items[i]
			r := ResourceRef{Kind: "Role", Name: role.Name, Namespace: namespace}
			out.add(r, metaOf("rbac.authorization.k8s.io/v1", &role.ObjectMeta), rulesAttributes(role.Rules), !showAll && isSystemRBACName(role.Name))
		}
	}

	if list, err := clientset.RbacV1().RoleBindings(namespace).List(ctx, opts); err != nil {
		if !forbidden("rolebindings", err) {
			return nil, fmt.Errorf("list rolebindings: %w", err)
		}
	} else {
		for i := range list.Items {
			rb := &list.Items[i]
			r := ResourceRef{Kind: "RoleBinding", Name: rb.Name, Namespace: namespace}
			out.add(r, metaOf("rbac.authorization.k8s.io/v1", &rb.ObjectMeta), bindingAttributes(rb.RoleRef, rb.Subjects), !showAll && isSystemRBACName(rb.Name))
			out.relations = append(out.relations, bindingRelations(r, rb.RoleRef, rb.Subjects, namespace)...)
		}
	}

	if list, err := clientset.NetworkingV1().NetworkPolicies(namespace).List(ctx, opts); err != nil {
		if !forbidden("networkpolicies", err) {
			return nil, fmt.Errorf("list networkpolicies: %w", err)
		}
	} else {
		for i := range list.Items {
			np := &list.Items[i]
			r := ResourceRef{Kind: "NetworkPolicy", Name: np.Name, Namespace: namespace}
			attrs := []string{fmt.Sprintf("ingress rules: %d", len(np.Spec.Ingress)), fmt.Sprintf("egress rules: %d", len(np.Spec.Egress))}
			if len(np.Spec.PolicyTypes) > 0 {
				types := make([]string, 0, len(np.Spec.PolicyTypes))
				for _, t := range np.Spec.PolicyTypes {
					types = append(types, string(t))
				}
				attrs = append([]string{"policy types: " + strings.Join(types, ", ")}, attrs...)
			}
			out.add(r, metaOf("networking.k8s.io/v1", &np.ObjectMeta), attrs, false)
			sel, err := metav1.LabelSelectorAsSelector(&np.Spec.PodSelector)
			if err != nil {
				continue // malformed selector: no edges rather than wrong ones
			}
			out.relations = append(out.relations, selectorEdges(r, AppliesTo, sel, pods)...)
		}
	}

	return out, nil
}

// selectorEdges returns an edge of type t from src to every Pod in pods (which
// are all from src's own namespace — selectors never cross namespaces) whose
// labels match sel, sorted for deterministic output.
func selectorEdges(src ResourceRef, t RelationType, sel labels.Selector, pods []corev1.Pod) []Relation {
	var out []Relation
	for i := range pods {
		p := &pods[i]
		if p.Namespace != src.Namespace {
			continue
		}
		if sel.Matches(labels.Set(p.Labels)) {
			out = append(out, Relation{From: src, Type: t, To: ResourceRef{Kind: "Pod", Name: p.Name, Namespace: p.Namespace}})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].To.Name < out[j].To.Name })
	return out
}

// bindingRelations turns a (Cluster)RoleBinding into a binds-role edge to its
// roleRef and a grants-to edge to each ServiceAccount subject. A namespaced
// RoleBinding may reference a ClusterRole, which is cluster-scoped.
func bindingRelations(binding ResourceRef, roleRef rbacv1.RoleRef, subjects []rbacv1.Subject, bindingNamespace string) []Relation {
	var out []Relation
	roleNS := bindingNamespace
	if roleRef.Kind == "ClusterRole" {
		roleNS = ""
	}
	if roleRef.Name != "" {
		out = append(out, Relation{From: binding, Type: BindsRole, To: ResourceRef{Kind: roleRef.Kind, Name: roleRef.Name, Namespace: roleNS}})
	}
	for _, s := range subjects {
		if s.Kind != rbacv1.ServiceAccountKind {
			continue
		}
		ns := s.Namespace
		if ns == "" {
			ns = bindingNamespace
		}
		if ns == "" {
			continue // a ClusterRoleBinding SA subject must name its namespace
		}
		out = append(out, Relation{From: binding, Type: GrantsTo, To: ResourceRef{Kind: "ServiceAccount", Name: s.Name, Namespace: ns}})
	}
	return out
}

func bindingAttributes(roleRef rbacv1.RoleRef, subjects []rbacv1.Subject) []string {
	attrs := []string{"role: " + roleRef.Kind + "/" + roleRef.Name}
	counts := map[string]int{}
	for _, s := range subjects {
		counts[s.Kind]++
	}
	kinds := make([]string, 0, len(counts))
	for k := range counts {
		kinds = append(kinds, k)
	}
	sort.Strings(kinds)
	parts := make([]string, 0, len(kinds))
	for _, k := range kinds {
		parts = append(parts, fmt.Sprintf("%d %s", counts[k], k))
	}
	if len(parts) > 0 {
		attrs = append(attrs, "subjects: "+strings.Join(parts, ", "))
	}
	return attrs
}

func rulesAttributes(rules []rbacv1.PolicyRule) []string {
	verbs := map[string]struct{}{}
	for _, r := range rules {
		for _, v := range r.Verbs {
			verbs[v] = struct{}{}
		}
	}
	vs := make([]string, 0, len(verbs))
	for v := range verbs {
		vs = append(vs, v)
	}
	sort.Strings(vs)
	attrs := []string{fmt.Sprintf("rules: %d", len(rules))}
	if len(vs) > 0 {
		attrs = append(attrs, "verbs: "+strings.Join(vs, ", "))
	}
	return attrs
}

// clusterScoped is what collectClusterScoped contributes once per pass.
type clusterScoped struct {
	*collected
	skipped        []string
	forbiddenKinds map[string]bool
}

// collectClusterScoped lists the cluster-scoped kinds that only need reading
// once per pass — StorageClasses, ClusterRoles and ClusterRoleBindings — plus
// each PersistentVolume's storage-class edge. It is best-effort: a Forbidden
// List is recorded as skipped, and any other error skips that kind too rather
// than failing a pass whose namespaced data is already complete.
func collectClusterScoped(ctx context.Context, clientset kubernetes.Interface, showAll bool) *clusterScoped {
	out := &clusterScoped{collected: newCollected(), forbiddenKinds: map[string]bool{}}
	opts := metav1.ListOptions{}
	fail := func(resource string, err error) {
		if apierrors.IsForbidden(err) {
			out.skipped = append(out.skipped, resource)
		}
		out.forbiddenKinds[kindForResource[resource]] = true
	}

	if list, err := clientset.StorageV1().StorageClasses().List(ctx, opts); err != nil {
		fail("storageclasses", err)
	} else {
		for i := range list.Items {
			sc := &list.Items[i]
			attrs := []string{"provisioner: " + sc.Provisioner}
			if sc.ReclaimPolicy != nil {
				attrs = append(attrs, "reclaim: "+string(*sc.ReclaimPolicy))
			}
			if sc.Annotations["storageclass.kubernetes.io/is-default-class"] == "true" {
				attrs = append(attrs, "default: true")
			}
			out.add(ResourceRef{Kind: "StorageClass", Name: sc.Name}, metaOf("storage.k8s.io/v1", &sc.ObjectMeta), attrs, false)
		}
	}

	if list, err := clientset.CoreV1().PersistentVolumes().List(ctx, opts); err == nil {
		for i := range list.Items {
			pv := &list.Items[i]
			if pv.Spec.StorageClassName != "" {
				out.relations = append(out.relations, Relation{
					From: ResourceRef{Kind: "PersistentVolume", Name: pv.Name},
					Type: UsesStorageClass,
					To:   ResourceRef{Kind: "StorageClass", Name: pv.Spec.StorageClassName},
				})
			}
		}
	}

	if list, err := clientset.RbacV1().ClusterRoles().List(ctx, opts); err != nil {
		fail("clusterroles", err)
	} else {
		for i := range list.Items {
			cr := &list.Items[i]
			// Visibility is decided later, once every binding is known: a
			// ClusterRole is only worth drawing when a visible binding uses it.
			out.add(ResourceRef{Kind: "ClusterRole", Name: cr.Name}, metaOf("rbac.authorization.k8s.io/v1", &cr.ObjectMeta), rulesAttributes(cr.Rules), false)
		}
	}

	if list, err := clientset.RbacV1().ClusterRoleBindings().List(ctx, opts); err != nil {
		fail("clusterrolebindings", err)
	} else {
		for i := range list.Items {
			crb := &list.Items[i]
			r := ResourceRef{Kind: "ClusterRoleBinding", Name: crb.Name}
			rels := bindingRelations(r, crb.RoleRef, crb.Subjects, "")
			hasSA := false
			for _, rel := range rels {
				if rel.Type == GrantsTo {
					hasSA = true
				}
			}
			// A binding to users/groups only has nothing in the graph to
			// connect to; system bindings are control-plane noise.
			hide := !showAll && (isSystemRBACName(crb.Name) || !hasSA)
			out.add(r, metaOf("rbac.authorization.k8s.io/v1", &crb.ObjectMeta), bindingAttributes(crb.RoleRef, crb.Subjects), hide)
			out.relations = append(out.relations, rels...)
		}
	}

	sort.Strings(out.skipped)
	return out
}

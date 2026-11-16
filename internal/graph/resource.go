package graph

// ResourceRef identifies a single Kubernetes resource within the cluster.
//
// It is deliberately kind-agnostic: the relationship engine works with
// ResourceRef values rather than concrete Kubernetes API types so that new
// resource kinds can participate in the graph without changing its shape.
type ResourceRef struct {
	Kind      string
	Name      string
	Namespace string
}

// String renders the reference as "<namespace>/<Kind>/<Name>" — the same
// stable, collision-free identifier the wire format uses. Cluster-scoped
// resources carry an empty namespace segment ("/Node/worker-1").
func (r ResourceRef) String() string {
	return r.Namespace + "/" + r.Kind + "/" + r.Name
}

// IsClusterScoped reports whether the reference names a cluster-scoped
// resource (one with no namespace).
func (r ResourceRef) IsClusterScoped() bool {
	return r.Namespace == ""
}

// ResourceMeta is the identity and bookkeeping metadata Mantis keeps for a
// resource it actually listed. Graph identity stays ResourceRef (kind,
// namespace, name) — that is what every spec reference in Kubernetes points
// by, and what a dangling reference (which has no UID) can still be named
// by — while UID records which concrete incarnation of that name was seen,
// so a delete-and-recreate is visible as a UID change.
//
// Annotations are deliberately absent: they routinely carry
// kubectl.kubernetes.io/last-applied-configuration, which for a Secret
// contains its data verbatim. Labels are safe and are what selectors use.
type ResourceMeta struct {
	UID             string
	APIVersion      string
	Labels          map[string]string
	CreatedAt       string // RFC 3339
	ResourceVersion string
}

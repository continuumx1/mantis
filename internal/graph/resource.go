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

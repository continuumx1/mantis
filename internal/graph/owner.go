package graph

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// ownerEdges turns an object's ownerReferences into controlled-by edges from the
// object to each owner. Applied to every object in a namespace, this single
// helper reconstructs the entire controller topology — Deployment -> ReplicaSet
// -> Pod, StatefulSet/DaemonSet -> Pod, CronJob -> Job -> Pod — because each
// link is just an ownerReference on the child.
//
// A namespaced owner lives in the same namespace as the object, but an owner can
// also be cluster-scoped: a mirror (static) Pod is owned by its Node. Such owners
// carry no namespace, so stamping the child's namespace onto them would fabricate
// a reference to a resource that does not exist. Cluster-scoped owner kinds are
// therefore emitted with an empty namespace.
func ownerEdges(subject ResourceRef, owners []metav1.OwnerReference, namespace string) []Relation {
	relations := make([]Relation, 0, len(owners))
	for _, owner := range owners {
		ownerNamespace := namespace
		if isClusterScopedKind(owner.Kind) {
			ownerNamespace = ""
		}
		relations = append(relations, Relation{
			From: subject,
			Type: ControlledBy,
			To: ResourceRef{
				Kind:      owner.Kind,
				Name:      owner.Name,
				Namespace: ownerNamespace,
			},
		})
	}
	return relations
}

// isClusterScopedKind reports whether a Kubernetes kind lives outside any
// namespace, so a reference to it must never inherit a namespace. This is the
// set of cluster-scoped kinds that can appear as an ownerReference on a
// namespaced object (a static Pod's Node being the real-world case).
func isClusterScopedKind(kind string) bool {
	switch kind {
	case "Node", "PersistentVolume", "StorageClass", "Namespace", "CustomResourceDefinition":
		return true
	default:
		return false
	}
}

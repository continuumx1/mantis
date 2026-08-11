package graph

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// ownerEdges turns an object's ownerReferences into controlled-by edges from the
// object to each owner. Applied to every object in a namespace, this single
// helper reconstructs the entire controller topology — Deployment -> ReplicaSet
// -> Pod, StatefulSet/DaemonSet -> Pod, CronJob -> Job -> Pod — because each
// link is just an ownerReference on the child.
//
// Owners are assumed to live in the same namespace as the object, which holds
// for the controllers KNW maps (a namespaced object cannot be owned by a
// resource in another namespace).
func ownerEdges(subject ResourceRef, owners []metav1.OwnerReference, namespace string) []Relation {
	relations := make([]Relation, 0, len(owners))
	for _, owner := range owners {
		relations = append(relations, Relation{
			From: subject,
			Type: ControlledBy,
			To: ResourceRef{
				Kind:      owner.Kind,
				Name:      owner.Name,
				Namespace: namespace,
			},
		})
	}
	return relations
}

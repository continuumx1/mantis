package graph

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// ResolvePod discovers the relationships for a single Pod and returns them as
// graph edges. It does not render anything and does not fetch the Pod itself —
// the caller passes an already-loaded Pod, which keeps the resolver a pure
// relationship function that is easy to test with a fake client.
//
// Currently discovered relationships:
//
//	Pod -[controlled-by]-> ReplicaSet -[controlled-by]-> Deployment
//	Pod -[runs-on]-> Node
//
// A missing controlling ReplicaSet is not an error: the Pod -> ReplicaSet edge
// is still reported, only the ReplicaSet -> Deployment edge is omitted.
func ResolvePod(
	ctx context.Context,
	clientset kubernetes.Interface,
	pod *corev1.Pod,
) ([]Relation, error) {
	podRef := ResourceRef{
		Kind:      "Pod",
		Name:      pod.Name,
		Namespace: pod.Namespace,
	}

	var relations []Relation

	// Ownership: every ownerReference becomes a controlled-by edge. When the
	// owner is a ReplicaSet we climb one level to surface its Deployment.
	for _, owner := range pod.OwnerReferences {
		ownerRef := ResourceRef{
			Kind:      owner.Kind,
			Name:      owner.Name,
			Namespace: pod.Namespace,
		}

		relations = append(relations, Relation{
			From: podRef,
			Type: ControlledBy,
			To:   ownerRef,
		})

		if owner.Kind == "ReplicaSet" {
			rs, err := clientset.AppsV1().
				ReplicaSets(pod.Namespace).
				Get(ctx, owner.Name, metav1.GetOptions{})
			if err == nil {
				for _, rsOwner := range rs.OwnerReferences {
					relations = append(relations, Relation{
						From: ownerRef,
						Type: ControlledBy,
						To: ResourceRef{
							Kind:      rsOwner.Kind,
							Name:      rsOwner.Name,
							Namespace: pod.Namespace,
						},
					})
				}
			}
		}
	}

	// Scheduling: a Pod with an assigned node runs-on that Node. Nodes are
	// cluster-scoped, so the reference carries no namespace.
	if pod.Spec.NodeName != "" {
		relations = append(relations, Relation{
			From: podRef,
			Type: RunsOn,
			To:   ResourceRef{Kind: "Node", Name: pod.Spec.NodeName},
		})
	}

	return relations, nil
}

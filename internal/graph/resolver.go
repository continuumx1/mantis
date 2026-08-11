package graph

import (
	"context"
	"sort"

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

	// Configuration: ConfigMaps and Secrets the Pod consumes, via the
	// environment (references) or mounted volumes (mounts).
	relations = append(relations, podConfigurationRelations(podRef, pod)...)

	return relations, nil
}

// podConfigurationRelations reads the Pod spec for the ConfigMaps and Secrets it
// consumes and returns them as references (environment) and mounts (volume)
// edges. It performs no API calls — the referenced names come straight from the
// spec, and whether the targets exist is a separate concern (see
// VerifyExistence).
func podConfigurationRelations(podRef ResourceRef, pod *corev1.Pod) []Relation {
	ns := pod.Namespace

	refConfigMaps := map[string]struct{}{}
	refSecrets := map[string]struct{}{}
	mountConfigMaps := map[string]struct{}{}
	mountSecrets := map[string]struct{}{}

	collectEnv := func(containers []corev1.Container) {
		for _, c := range containers {
			for _, envFrom := range c.EnvFrom {
				if envFrom.ConfigMapRef != nil {
					refConfigMaps[envFrom.ConfigMapRef.Name] = struct{}{}
				}
				if envFrom.SecretRef != nil {
					refSecrets[envFrom.SecretRef.Name] = struct{}{}
				}
			}
			for _, env := range c.Env {
				if env.ValueFrom == nil {
					continue
				}
				if env.ValueFrom.ConfigMapKeyRef != nil {
					refConfigMaps[env.ValueFrom.ConfigMapKeyRef.Name] = struct{}{}
				}
				if env.ValueFrom.SecretKeyRef != nil {
					refSecrets[env.ValueFrom.SecretKeyRef.Name] = struct{}{}
				}
			}
		}
	}
	collectEnv(pod.Spec.InitContainers)
	collectEnv(pod.Spec.Containers)

	for _, vol := range pod.Spec.Volumes {
		if vol.ConfigMap != nil {
			mountConfigMaps[vol.ConfigMap.Name] = struct{}{}
		}
		if vol.Secret != nil {
			mountSecrets[vol.Secret.SecretName] = struct{}{}
		}
	}

	var relations []Relation
	relations = append(relations, edgesTo(podRef, References, "ConfigMap", ns, refConfigMaps)...)
	relations = append(relations, edgesTo(podRef, References, "Secret", ns, refSecrets)...)
	relations = append(relations, edgesTo(podRef, Mounts, "ConfigMap", ns, mountConfigMaps)...)
	relations = append(relations, edgesTo(podRef, Mounts, "Secret", ns, mountSecrets)...)
	return relations
}

// edgesTo builds sorted edges from a single source to a set of named resources
// of one kind, so output is deterministic regardless of map iteration order.
func edgesTo(from ResourceRef, t RelationType, kind, namespace string, names map[string]struct{}) []Relation {
	sorted := make([]string, 0, len(names))
	for name := range names {
		sorted = append(sorted, name)
	}
	sort.Strings(sorted)

	out := make([]Relation, 0, len(sorted))
	for _, name := range sorted {
		out = append(out, Relation{
			From: from,
			Type: t,
			To:   ResourceRef{Kind: kind, Name: name, Namespace: namespace},
		})
	}
	return out
}

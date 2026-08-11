package graph

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// VerifyExistence checks whether each referenced resource actually exists in the
// cluster and returns a map from ResourceRef to existence.
//
// Only kinds KNW knows how to look up are included in the result. A ref of an
// unknown kind is deliberately absent from the map rather than reported as
// missing: KNW annotates a reference as "not found" only when it has actually
// checked and confirmed absence, never by assumption. This keeps a discovered
// fact (the reference exists in a spec) separate from a verified fact (the
// target does or does not exist).
//
// Existence is a property of the node, not of the edge that points at it, which
// is why this lives alongside the graph rather than on Relation.
func VerifyExistence(
	ctx context.Context,
	clientset kubernetes.Interface,
	refs []ResourceRef,
) (map[ResourceRef]bool, error) {
	result := make(map[ResourceRef]bool)

	for _, ref := range refs {
		if _, already := result[ref]; already {
			continue
		}

		exists, checkable, err := getExists(ctx, clientset, ref)
		if err != nil {
			return nil, err
		}
		if checkable {
			result[ref] = exists
		}
	}

	return result, nil
}

// getExists reports whether ref exists. checkable is false for kinds KNW cannot
// look up, in which case exists is meaningless and the caller must not treat the
// resource as missing.
func getExists(
	ctx context.Context,
	clientset kubernetes.Interface,
	ref ResourceRef,
) (exists bool, checkable bool, err error) {
	opts := metav1.GetOptions{}

	var getErr error
	switch ref.Kind {
	case "Pod":
		_, getErr = clientset.CoreV1().Pods(ref.Namespace).Get(ctx, ref.Name, opts)
	case "Service":
		_, getErr = clientset.CoreV1().Services(ref.Namespace).Get(ctx, ref.Name, opts)
	case "ConfigMap":
		_, getErr = clientset.CoreV1().ConfigMaps(ref.Namespace).Get(ctx, ref.Name, opts)
	case "Secret":
		_, getErr = clientset.CoreV1().Secrets(ref.Namespace).Get(ctx, ref.Name, opts)
	case "Node":
		_, getErr = clientset.CoreV1().Nodes().Get(ctx, ref.Name, opts)
	case "ReplicaSet":
		_, getErr = clientset.AppsV1().ReplicaSets(ref.Namespace).Get(ctx, ref.Name, opts)
	case "Deployment":
		_, getErr = clientset.AppsV1().Deployments(ref.Namespace).Get(ctx, ref.Name, opts)
	case "Ingress":
		_, getErr = clientset.NetworkingV1().Ingresses(ref.Namespace).Get(ctx, ref.Name, opts)
	default:
		return false, false, nil
	}

	switch {
	case getErr == nil:
		return true, true, nil
	case apierrors.IsNotFound(getErr):
		return false, true, nil
	default:
		return false, true, fmt.Errorf("check existence of %s/%s: %w", ref.Kind, ref.Name, getErr)
	}
}

// TargetRefs returns the distinct To endpoints of the given relations, in order
// of first appearance.
func TargetRefs(relations []Relation) []ResourceRef {
	seen := make(map[ResourceRef]struct{}, len(relations))
	var refs []ResourceRef
	for _, r := range relations {
		if _, ok := seen[r.To]; ok {
			continue
		}
		seen[r.To] = struct{}{}
		refs = append(refs, r.To)
	}
	return refs
}

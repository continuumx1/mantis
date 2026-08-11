package explain

import (
	"context"
	"fmt"

	"k8s.io/client-go/kubernetes"

	"github.com/continuumx1/knw/internal/graph"
	"github.com/continuumx1/knw/internal/render"
)

// MapNamespace builds the relationship graph for every resource KNW understands
// in a namespace and renders it as a grouped tree.
func MapNamespace(
	ctx context.Context,
	clientset kubernetes.Interface,
	namespace string,
) (string, error) {
	investigation, err := graph.BuildNamespaceGraph(ctx, clientset, namespace)
	if err != nil {
		return "", fmt.Errorf("map namespace %q: %w", namespace, err)
	}

	return render.NamespaceTree(namespace, investigation), nil
}

package explain

import (
	"context"
	"fmt"

	"k8s.io/client-go/kubernetes"

	"github.com/continuumx1/knw/internal/graph"
	"github.com/continuumx1/knw/internal/render"
)

// MapNamespace builds the relationship graph for every resource KNW understands
// in a namespace and renders it as a grouped tree. When showAll is false,
// cluster-managed noise is hidden.
func MapNamespace(
	ctx context.Context,
	clientset kubernetes.Interface,
	namespace string,
	showAll bool,
) (string, error) {
	investigation, skipped, err := graph.BuildNamespaceGraph(ctx, clientset, namespace, showAll)
	if err != nil {
		return "", fmt.Errorf("map namespace %q: %w", namespace, err)
	}

	return render.NamespaceTree(namespace, investigation, skipped), nil
}

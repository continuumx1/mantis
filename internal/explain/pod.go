package explain

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/continuumx1/knw/internal/graph"
	"github.com/continuumx1/knw/internal/render"
)

// PodWhy explains a Pod by loading it, resolving its relationships through the
// graph engine, and rendering the result. The investigation logic lives in the
// graph package and the presentation logic in the render package; this function
// only wires them together for the CLI.
func PodWhy(
	ctx context.Context,
	clientset kubernetes.Interface,
	namespace string,
	name string,
) (string, error) {
	pod, err := clientset.CoreV1().
		Pods(namespace).
		Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("get pod %q: %w", name, err)
	}

	relations, err := graph.ResolvePod(ctx, clientset, pod)
	if err != nil {
		return "", fmt.Errorf("resolve relationships for pod %q: %w", name, err)
	}

	existence, err := graph.VerifyExistence(ctx, clientset, graph.TargetRefs(relations))
	if err != nil {
		return "", fmt.Errorf("verify pod %q targets: %w", name, err)
	}

	return render.PodTree(pod, relations, existence), nil
}

package explain

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/continuumx1/knw/internal/graph"
	"github.com/continuumx1/knw/internal/render"
)

// ServiceWhy explains a Service by loading it, resolving which Pods it selects
// through the graph engine, and rendering the result.
func ServiceWhy(
	ctx context.Context,
	clientset kubernetes.Interface,
	namespace string,
	name string,
) (string, error) {
	svc, err := clientset.CoreV1().
		Services(namespace).
		Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("get service %q: %w", name, err)
	}

	relations, err := graph.ResolveService(ctx, clientset, svc)
	if err != nil {
		return "", fmt.Errorf("resolve relationships for service %q: %w", name, err)
	}

	return render.ServiceTree(svc, relations), nil
}

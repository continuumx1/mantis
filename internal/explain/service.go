package explain

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/continuumx1/knw/internal/graph"
	"github.com/continuumx1/knw/internal/render"
)

// InspectService explains a Service by loading it, resolving both the Pods it
// selects and the Pods actually backing it (its endpoints), then rendering the
// result.
func InspectService(
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

	endpoints, err := graph.ResolveServiceEndpoints(ctx, clientset, svc)
	if err != nil {
		return "", fmt.Errorf("resolve endpoints for service %q: %w", name, err)
	}
	relations = append(relations, endpoints...)

	subject := graph.ResourceRef{Kind: "Service", Name: svc.Name, Namespace: svc.Namespace}
	investigation, err := graph.Build(ctx, clientset, subject, relations)
	if err != nil {
		return "", fmt.Errorf("build context for service %q: %w", name, err)
	}

	return render.ServiceTree(svc, investigation), nil
}

package explain

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/continuumx1/knw/internal/graph"
	"github.com/continuumx1/knw/internal/render"
)

// IngressWhy explains an Ingress by loading it, resolving which Services it
// routes to through the graph engine, and rendering the result.
func IngressWhy(
	ctx context.Context,
	clientset kubernetes.Interface,
	namespace string,
	name string,
) (string, error) {
	ing, err := clientset.NetworkingV1().
		Ingresses(namespace).
		Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("get ingress %q: %w", name, err)
	}

	relations, err := graph.ResolveIngress(ctx, clientset, ing)
	if err != nil {
		return "", fmt.Errorf("resolve relationships for ingress %q: %w", name, err)
	}

	subject := graph.ResourceRef{Kind: "Ingress", Name: ing.Name, Namespace: ing.Namespace}
	investigation, err := graph.Build(ctx, clientset, subject, relations)
	if err != nil {
		return "", fmt.Errorf("build context for ingress %q: %w", name, err)
	}

	return render.IngressTree(ing, investigation), nil
}

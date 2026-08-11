package graph

import (
	"context"
	"fmt"
	"sort"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
)

// ResolveService discovers the Pods a Service selects and returns them as
// selects edges. Like ResolvePod it performs no rendering.
//
//	Service -[selects]-> Pod
//
// A Service with an empty selector selects no Pods by label (its endpoints are
// managed manually); ResolveService returns no edges in that case. The empty
// selector itself is a property of the Service, so it is the renderer — which
// also receives the Service — that distinguishes "no selector" from "selector
// matched nothing".
func ResolveService(
	ctx context.Context,
	clientset kubernetes.Interface,
	svc *corev1.Service,
) ([]Relation, error) {
	if len(svc.Spec.Selector) == 0 {
		return nil, nil
	}

	svcRef := ResourceRef{
		Kind:      "Service",
		Name:      svc.Name,
		Namespace: svc.Namespace,
	}

	podList, err := clientset.CoreV1().
		Pods(svc.Namespace).
		List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list pods in namespace %q: %w", svc.Namespace, err)
	}

	selector := labels.SelectorFromSet(svc.Spec.Selector)

	var relations []Relation
	for i := range podList.Items {
		pod := &podList.Items[i]
		if selector.Matches(labels.Set(pod.Labels)) {
			relations = append(relations, Relation{
				From: svcRef,
				Type: Selects,
				To: ResourceRef{
					Kind:      "Pod",
					Name:      pod.Name,
					Namespace: pod.Namespace,
				},
			})
		}
	}

	// Deterministic output regardless of listing order.
	sort.Slice(relations, func(i, j int) bool {
		return relations[i].To.Name < relations[j].To.Name
	})

	return relations, nil
}

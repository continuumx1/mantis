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

	return selectsRelations(svcRef, svc.Spec.Selector, podList.Items), nil
}

// selectsRelations matches a label selector against a set of Pods and returns
// sorted selects edges from svcRef to each matching Pod in the same namespace.
// It is shared by ResolveService (which lists the Pods) and the namespace
// builder (which already holds them), so selection logic lives in one place.
func selectsRelations(svcRef ResourceRef, selector map[string]string, pods []corev1.Pod) []Relation {
	if len(selector) == 0 {
		return nil
	}

	sel := labels.SelectorFromSet(selector)

	var relations []Relation
	for i := range pods {
		pod := &pods[i]
		if pod.Namespace != svcRef.Namespace {
			continue
		}
		if sel.Matches(labels.Set(pod.Labels)) {
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

	sort.Slice(relations, func(i, j int) bool {
		return relations[i].To.Name < relations[j].To.Name
	})

	return relations
}

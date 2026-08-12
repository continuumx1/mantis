package graph

import (
	"context"
	"fmt"
	"sort"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// serviceNameLabel is the EndpointSlice label that ties a slice to its Service.
const serviceNameLabel = "kubernetes.io/service-name"

// ResolveServiceEndpoints discovers the Pods actually backing a Service from its
// EndpointSlices and returns them as serves edges. Unlike ResolveService (which
// matches the label selector), this reflects the real traffic path.
//
//	Service -[serves]-> Pod
func ResolveServiceEndpoints(
	ctx context.Context,
	clientset kubernetes.Interface,
	svc *corev1.Service,
) ([]Relation, error) {
	sliceList, err := clientset.DiscoveryV1().
		EndpointSlices(svc.Namespace).
		List(ctx, metav1.ListOptions{LabelSelector: serviceNameLabel + "=" + svc.Name})
	if err != nil {
		return nil, fmt.Errorf("list endpointslices for service %q: %w", svc.Name, err)
	}

	svcRef := ResourceRef{Kind: "Service", Name: svc.Name, Namespace: svc.Namespace}
	return servesFromSlices(svcRef, sliceList.Items), nil
}

// servesFromSlices builds sorted, deduplicated serves edges from the Pod targets
// of the given EndpointSlices. It is shared by ResolveServiceEndpoints (which
// lists per-service) and the namespace builder (which lists once and dispatches
// by the service-name label).
func servesFromSlices(svcRef ResourceRef, slices []discoveryv1.EndpointSlice) []Relation {
	seen := map[string]struct{}{}
	var relations []Relation

	for i := range slices {
		slice := &slices[i]
		if slice.Labels[serviceNameLabel] != svcRef.Name {
			continue
		}
		for _, endpoint := range slice.Endpoints {
			if endpoint.TargetRef == nil || endpoint.TargetRef.Kind != "Pod" {
				continue
			}
			name := endpoint.TargetRef.Name
			if _, ok := seen[name]; ok {
				continue
			}
			seen[name] = struct{}{}
			relations = append(relations, Relation{
				From: svcRef,
				Type: Serves,
				To:   ResourceRef{Kind: "Pod", Name: name, Namespace: svcRef.Namespace},
			})
		}
	}

	sort.Slice(relations, func(i, j int) bool {
		return relations[i].To.Name < relations[j].To.Name
	})
	return relations
}

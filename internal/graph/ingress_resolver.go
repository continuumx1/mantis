package graph

import (
	"context"
	"sort"

	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/client-go/kubernetes"
)

// ResolveIngress discovers the Services an Ingress routes to and returns them as
// routes-to edges.
//
//	Ingress -[routes-to]-> Service
//
// Service names are collected from the default backend and from every rule
// path backend, deduplicated, and sorted for deterministic output. Backends
// that reference a Resource rather than a Service are ignored.
//
// The edges are read directly from the Ingress spec: a routes-to edge states
// only what the Ingress declares, not that the target Service exists. Verifying
// target existence (to surface a dangling reference) is deferred until the graph
// models node properties; clientset is accepted now to keep the resolver family
// uniform and ready for that.
func ResolveIngress(
	_ context.Context,
	_ kubernetes.Interface,
	ing *networkingv1.Ingress,
) ([]Relation, error) {
	ingRef := ResourceRef{
		Kind:      "Ingress",
		Name:      ing.Name,
		Namespace: ing.Namespace,
	}

	names := map[string]struct{}{}

	if db := ing.Spec.DefaultBackend; db != nil && db.Service != nil {
		names[db.Service.Name] = struct{}{}
	}

	for _, rule := range ing.Spec.Rules {
		if rule.HTTP == nil {
			continue
		}
		for _, path := range rule.HTTP.Paths {
			if path.Backend.Service != nil {
				names[path.Backend.Service.Name] = struct{}{}
			}
		}
	}

	sorted := make([]string, 0, len(names))
	for name := range names {
		sorted = append(sorted, name)
	}
	sort.Strings(sorted)

	var relations []Relation
	for _, name := range sorted {
		relations = append(relations, Relation{
			From: ingRef,
			Type: RoutesTo,
			To: ResourceRef{
				Kind:      "Service",
				Name:      name,
				Namespace: ing.Namespace,
			},
		})
	}

	return relations, nil
}

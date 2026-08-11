package render

import (
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"

	"github.com/continuumx1/knw/internal/graph"
)

// PodTree renders a human-readable explanation of a Pod from the Pod itself and
// its resolved relationships. Relationship structure comes from the graph;
// intrinsic properties (name, status) come from the Pod.
func PodTree(pod *corev1.Pod, relations []graph.Relation) string {
	podRef := graph.ResourceRef{
		Kind:      "Pod",
		Name:      pod.Name,
		Namespace: pod.Namespace,
	}

	var b strings.Builder

	fmt.Fprintf(&b, "POD/%s\n\n", pod.Name)
	b.WriteString("WHY\n\n")

	// Ownership
	owners := relationsFrom(relations, podRef, graph.ControlledBy)
	if len(owners) == 0 {
		b.WriteString("Origin\n")
		b.WriteString("  └── Directly created\n\n")
		b.WriteString("Owner\n")
		b.WriteString("  └── None\n\n")
	} else {
		b.WriteString("Ownership\n")
		for _, owner := range owners {
			fmt.Fprintf(&b, "  └── %s/%s\n", owner.To.Kind, owner.To.Name)

			for _, grandOwner := range relationsFrom(relations, owner.To, graph.ControlledBy) {
				fmt.Fprintf(&b, "       └── %s/%s\n", grandOwner.To.Kind, grandOwner.To.Name)
			}
		}
		b.WriteString("\n")
	}

	// Node
	b.WriteString("Runs on\n")
	if nodes := relationsFrom(relations, podRef, graph.RunsOn); len(nodes) > 0 {
		fmt.Fprintf(&b, "  └── %s/%s\n\n", nodes[0].To.Kind, nodes[0].To.Name)
	} else {
		b.WriteString("  └── Not scheduled\n\n")
	}

	// Status
	b.WriteString("Status\n")
	fmt.Fprintf(&b, "  └── %s\n", pod.Status.Phase)

	return b.String()
}

// ServiceTree renders a human-readable explanation of a Service from the
// Service itself and its resolved relationships. It distinguishes a Service
// with no selector (endpoints managed manually) from a selector that simply
// matched no Pods.
func ServiceTree(svc *corev1.Service, relations []graph.Relation) string {
	svcRef := graph.ResourceRef{
		Kind:      "Service",
		Name:      svc.Name,
		Namespace: svc.Namespace,
	}

	var b strings.Builder

	fmt.Fprintf(&b, "SERVICE/%s\n\n", svc.Name)
	b.WriteString("WHY\n\n")

	b.WriteString("Selects\n")
	switch {
	case len(svc.Spec.Selector) == 0:
		b.WriteString("  └── No selector (endpoints managed manually)\n")
	case len(relationsFrom(relations, svcRef, graph.Selects)) == 0:
		b.WriteString("  └── No matching pods\n")
	default:
		for _, sel := range relationsFrom(relations, svcRef, graph.Selects) {
			fmt.Fprintf(&b, "  └── %s/%s\n", sel.To.Kind, sel.To.Name)
		}
	}

	return b.String()
}

// IngressTree renders a human-readable explanation of an Ingress from the
// Ingress itself and its resolved relationships.
func IngressTree(ing *networkingv1.Ingress, relations []graph.Relation) string {
	ingRef := graph.ResourceRef{
		Kind:      "Ingress",
		Name:      ing.Name,
		Namespace: ing.Namespace,
	}

	var b strings.Builder

	fmt.Fprintf(&b, "INGRESS/%s\n\n", ing.Name)
	b.WriteString("WHY\n\n")

	b.WriteString("Routes to\n")
	if routes := relationsFrom(relations, ingRef, graph.RoutesTo); len(routes) > 0 {
		for _, route := range routes {
			fmt.Fprintf(&b, "  └── %s/%s\n", route.To.Kind, route.To.Name)
		}
	} else {
		b.WriteString("  └── No service backends\n")
	}

	return b.String()
}

// relationsFrom returns the relations of the given type originating at from.
func relationsFrom(relations []graph.Relation, from graph.ResourceRef, t graph.RelationType) []graph.Relation {
	var out []graph.Relation
	for _, r := range relations {
		if r.From == from && r.Type == t {
			out = append(out, r)
		}
	}
	return out
}

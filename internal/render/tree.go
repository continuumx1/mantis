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
//
// existence carries verified target existence (see IngressTree): a target
// confirmed absent is rendered with a "(not found)" suffix.
func PodTree(pod *corev1.Pod, relations []graph.Relation, existence map[graph.ResourceRef]bool) string {
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
			fmt.Fprintf(&b, "  └── %s\n", label(owner.To, existence))

			for _, grandOwner := range relationsFrom(relations, owner.To, graph.ControlledBy) {
				fmt.Fprintf(&b, "       └── %s\n", label(grandOwner.To, existence))
			}
		}
		b.WriteString("\n")
	}

	// Configuration
	writeSection(&b, "References", relationsFrom(relations, podRef, graph.References), existence)
	writeSection(&b, "Mounts", relationsFrom(relations, podRef, graph.Mounts), existence)

	// Node
	b.WriteString("Runs on\n")
	if nodes := relationsFrom(relations, podRef, graph.RunsOn); len(nodes) > 0 {
		fmt.Fprintf(&b, "  └── %s\n\n", label(nodes[0].To, existence))
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
//
// existence reports which target refs were verified against the cluster: a ref
// mapped to false was checked and is absent (rendered as "not found"); a ref
// absent from the map was not verified and is rendered plainly. This keeps a
// declared reference distinct from a confirmed-missing target.
func IngressTree(ing *networkingv1.Ingress, relations []graph.Relation, existence map[graph.ResourceRef]bool) string {
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
			fmt.Fprintf(&b, "  └── %s\n", label(route.To, existence))
		}
	} else {
		b.WriteString("  └── No service backends\n")
	}

	return b.String()
}

// writeSection renders a titled block of relation targets, one per line, and
// nothing at all when there are no relations. A trailing blank line separates it
// from the next section.
func writeSection(b *strings.Builder, header string, rels []graph.Relation, existence map[graph.ResourceRef]bool) {
	if len(rels) == 0 {
		return
	}
	b.WriteString(header + "\n")
	for _, r := range rels {
		fmt.Fprintf(b, "  └── %s\n", label(r.To, existence))
	}
	b.WriteString("\n")
}

// label renders a resource reference as "Kind/Name", appending "(not found)"
// only when existence has verified the target and confirmed it absent. Refs that
// were not verified are rendered plainly.
func label(ref graph.ResourceRef, existence map[graph.ResourceRef]bool) string {
	if exists, checked := existence[ref]; checked && !exists {
		return fmt.Sprintf("%s/%s (not found)", ref.Kind, ref.Name)
	}
	return fmt.Sprintf("%s/%s", ref.Kind, ref.Name)
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

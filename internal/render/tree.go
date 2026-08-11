package render

import (
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"

	"github.com/continuumx1/knw/internal/graph"
)

// PodTree renders a human-readable explanation of a Pod from the Pod itself and
// its investigation context. Relationship structure and verified node existence
// come from the Context; intrinsic properties (name, status) come from the Pod.
func PodTree(pod *corev1.Pod, c *graph.Context) string {
	podRef := graph.ResourceRef{
		Kind:      "Pod",
		Name:      pod.Name,
		Namespace: pod.Namespace,
	}

	var b strings.Builder

	fmt.Fprintf(&b, "POD/%s\n\n", pod.Name)
	b.WriteString("WHY\n\n")

	// Ownership
	owners := c.From(podRef, graph.ControlledBy)
	if len(owners) == 0 {
		b.WriteString("Origin\n")
		b.WriteString("  └── Directly created\n\n")
		b.WriteString("Owner\n")
		b.WriteString("  └── None\n\n")
	} else {
		b.WriteString("Ownership\n")
		for _, owner := range owners {
			fmt.Fprintf(&b, "  └── %s\n", label(owner.To, c))

			for _, grandOwner := range c.From(owner.To, graph.ControlledBy) {
				fmt.Fprintf(&b, "       └── %s\n", label(grandOwner.To, c))
			}
		}
		b.WriteString("\n")
	}

	// Configuration
	writeSection(&b, "References", c.From(podRef, graph.References), c)
	writeSection(&b, "Mounts", c.From(podRef, graph.Mounts), c)

	// Node
	b.WriteString("Runs on\n")
	if nodes := c.From(podRef, graph.RunsOn); len(nodes) > 0 {
		fmt.Fprintf(&b, "  └── %s\n\n", label(nodes[0].To, c))
	} else {
		b.WriteString("  └── Not scheduled\n\n")
	}

	// Status
	b.WriteString("Status\n")
	fmt.Fprintf(&b, "  └── %s\n", pod.Status.Phase)

	return b.String()
}

// ServiceTree renders a human-readable explanation of a Service from the
// Service itself and its investigation context. It distinguishes a Service with
// no selector (endpoints managed manually) from a selector that simply matched
// no Pods.
func ServiceTree(svc *corev1.Service, c *graph.Context) string {
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
	case len(c.From(svcRef, graph.Selects)) == 0:
		b.WriteString("  └── No matching pods\n")
	default:
		for _, sel := range c.From(svcRef, graph.Selects) {
			fmt.Fprintf(&b, "  └── %s\n", label(sel.To, c))
		}
	}

	return b.String()
}

// IngressTree renders a human-readable explanation of an Ingress from the
// Ingress itself and its investigation context. A target confirmed absent by
// the Context is rendered with a "(not found)" suffix; a declared reference that
// was not verified is rendered plainly.
func IngressTree(ing *networkingv1.Ingress, c *graph.Context) string {
	ingRef := graph.ResourceRef{
		Kind:      "Ingress",
		Name:      ing.Name,
		Namespace: ing.Namespace,
	}

	var b strings.Builder

	fmt.Fprintf(&b, "INGRESS/%s\n\n", ing.Name)
	b.WriteString("WHY\n\n")

	b.WriteString("Routes to\n")
	if routes := c.From(ingRef, graph.RoutesTo); len(routes) > 0 {
		for _, route := range routes {
			fmt.Fprintf(&b, "  └── %s\n", label(route.To, c))
		}
	} else {
		b.WriteString("  └── No service backends\n")
	}

	return b.String()
}

// writeSection renders a titled block of relation targets, one per line, and
// nothing at all when there are no relations. A trailing blank line separates it
// from the next section.
func writeSection(b *strings.Builder, header string, rels []graph.Relation, c *graph.Context) {
	if len(rels) == 0 {
		return
	}
	b.WriteString(header + "\n")
	for _, r := range rels {
		fmt.Fprintf(b, "  └── %s\n", label(r.To, c))
	}
	b.WriteString("\n")
}

// label renders a resource reference as "Kind/Name", appending "(not found)"
// only when the Context verified the target and confirmed it absent. Refs that
// were not verified are rendered plainly.
func label(ref graph.ResourceRef, c *graph.Context) string {
	if resolved, checked := c.Existence(ref); checked && !resolved {
		return fmt.Sprintf("%s/%s (not found)", ref.Kind, ref.Name)
	}
	return fmt.Sprintf("%s/%s", ref.Kind, ref.Name)
}

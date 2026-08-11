package render

import (
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"

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

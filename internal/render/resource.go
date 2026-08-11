package render

import (
	"fmt"
	"sort"
	"strings"

	"github.com/continuumx1/knw/internal/graph"
)

// maxGraphDepth bounds how far the subject-centric view follows edges outward,
// keeping the output focused and terminating on cycles.
const maxGraphDepth = 4

// reverseVerb phrases an incoming edge from the subject's point of view, so a
// "Service selects Pod" edge reads as "selected-by ← Service" when the Pod is
// the subject. Missing entries fall back to the forward relation type.
var reverseVerb = map[graph.RelationType]string{
	graph.ControlledBy: "controls",
	graph.RunsOn:       "hosts",
	graph.Selects:      "selected-by",
	graph.Serves:       "served-by",
	graph.RoutesTo:     "routed-from",
	graph.References:   "used-by",
	graph.Mounts:       "mounted-by",
	graph.Claims:       "claimed-by",
	graph.BoundTo:      "bound-from",
}

// ResourceGraph renders a subject-centric view of the graph: it starts from one
// resource and follows its relationships outward in both directions, phrasing
// each as a human-readable edge (e.g. "runs-on → Node/minikube", "selected-by ←
// Service/web"). It is a different view of the same graph the namespace tree
// renders — the model is shared; only the traversal differs.
func ResourceGraph(root graph.ResourceRef, c *graph.Context) string {
	var b strings.Builder

	fmt.Fprintf(&b, "%s/%s\n", root.Kind, root.Name)
	if root.Namespace != "" {
		fmt.Fprintf(&b, "namespace: %s\n", root.Namespace)
	}
	for _, attr := range c.Attributes(root) {
		fmt.Fprintf(&b, "%s\n", attr)
	}
	b.WriteString("\nRelationships:\n")

	outgoing := map[graph.ResourceRef][]graph.Relation{}
	incoming := map[graph.ResourceRef][]graph.Relation{}
	for _, r := range c.Relations {
		outgoing[r.From] = append(outgoing[r.From], r)
		incoming[r.To] = append(incoming[r.To], r)
	}

	visited := map[graph.ResourceRef]bool{root: true}

	var walk func(node, parent graph.ResourceRef, depth int)
	walk = func(node, parent graph.ResourceRef, depth int) {
		if depth >= maxGraphDepth {
			return
		}

		edges := edgeLines(node, parent, outgoing, incoming)
		for _, e := range edges {
			marker := ""
			if e.inferred {
				marker = " (inferred)"
			}
			fmt.Fprintf(&b, "%s└── %s %s %s%s\n",
				strings.Repeat("    ", depth), e.verb, e.arrow, labelWithAttrs(e.neighbor, c), marker)

			if !visited[e.neighbor] {
				visited[e.neighbor] = true
				walk(e.neighbor, node, depth+1)
			}
		}
	}
	walk(root, graph.ResourceRef{}, 0)

	return b.String()
}

// edgeLine is one rendered relationship from a node.
type edgeLine struct {
	verb     string
	arrow    string
	neighbor graph.ResourceRef
	inferred bool
}

// edgeLines gathers a node's relationships (outgoing and incoming), skips the
// edge back to the parent, and returns them in a deterministic order.
func edgeLines(node, parent graph.ResourceRef, outgoing, incoming map[graph.ResourceRef][]graph.Relation) []edgeLine {
	var lines []edgeLine

	for _, r := range outgoing[node] {
		if r.To == parent {
			continue
		}
		lines = append(lines, edgeLine{verb: string(r.Type), arrow: "→", neighbor: r.To, inferred: r.IsInferred()})
	}
	for _, r := range incoming[node] {
		if r.From == parent {
			continue
		}
		verb := reverseVerb[r.Type]
		if verb == "" {
			verb = string(r.Type)
		}
		lines = append(lines, edgeLine{verb: verb, arrow: "←", neighbor: r.From, inferred: r.IsInferred()})
	}

	sort.Slice(lines, func(i, j int) bool {
		if lines[i].verb != lines[j].verb {
			return lines[i].verb < lines[j].verb
		}
		if lines[i].neighbor.Kind != lines[j].neighbor.Kind {
			return lines[i].neighbor.Kind < lines[j].neighbor.Kind
		}
		return lines[i].neighbor.Name < lines[j].neighbor.Name
	})

	return lines
}

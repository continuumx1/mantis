package render

import (
	"fmt"
	"sort"
	"strings"

	"github.com/continuumx1/knw/internal/graph"
)

// workloadKinds are the resource kinds rendered in the ownership forest. All of
// them participate in controlled-by chains.
var workloadKinds = map[string]bool{
	"Deployment":            true,
	"StatefulSet":           true,
	"DaemonSet":             true,
	"CronJob":               true,
	"Job":                   true,
	"ReplicaSet":            true,
	"ReplicationController": true,
	"Pod":                   true,
}

// NamespaceTree renders the whole-namespace graph as grouped sections: the
// ownership forest (WORKLOADS), the traffic path (NETWORKING), and configuration
// and storage. Ownership is the primary spine; other relationships appear as
// annotations so a DAG stays readable in a terminal.
func NamespaceTree(namespace string, c *graph.Context) string {
	var b strings.Builder
	fmt.Fprintf(&b, "NAMESPACE/%s\n\n", namespace)

	nodes := resolvedRefs(c)

	// Index the controller hierarchy from controlled-by edges (child -> parent).
	childrenOf := map[graph.ResourceRef][]graph.ResourceRef{}
	hasParent := map[graph.ResourceRef]bool{}
	for _, r := range c.Relations {
		if r.Type == graph.ControlledBy {
			childrenOf[r.To] = append(childrenOf[r.To], r.From)
			hasParent[r.From] = true
		}
	}

	// WORKLOADS: forest rooted at workloads nothing in the namespace controls.
	var roots []graph.ResourceRef
	for _, ref := range nodes {
		if workloadKinds[ref.Kind] && !hasParent[ref] {
			roots = append(roots, ref)
		}
	}
	if len(roots) > 0 {
		b.WriteString("WORKLOADS\n")
		for _, root := range roots {
			writeWorkload(&b, c, root, childrenOf, 0)
		}
		b.WriteString("\n")
	}

	// NETWORKING: services and ingresses with their targets.
	services := refsOfKind(nodes, "Service")
	ingresses := refsOfKind(nodes, "Ingress")
	if len(services)+len(ingresses) > 0 {
		b.WriteString("NETWORKING\n")
		for _, svc := range services {
			fmt.Fprintf(&b, "└── %s\n", label(svc, c))
			for _, sel := range c.From(svc, graph.Selects) {
				fmt.Fprintf(&b, "    └── selects %s\n", label(sel.To, c))
			}
		}
		for _, ing := range ingresses {
			fmt.Fprintf(&b, "└── %s\n", label(ing, c))
			for _, route := range c.From(ing, graph.RoutesTo) {
				fmt.Fprintf(&b, "    └── routes-to %s\n", label(route.To, c))
			}
		}
		b.WriteString("\n")
	}

	// CONFIG & STORAGE.
	configMaps := refsOfKind(nodes, "ConfigMap")
	secrets := refsOfKind(nodes, "Secret")
	pvcs := refsOfKind(nodes, "PersistentVolumeClaim")
	if len(configMaps)+len(secrets)+len(pvcs) > 0 {
		b.WriteString("CONFIG & STORAGE\n")
		for _, cm := range configMaps {
			fmt.Fprintf(&b, "└── %s\n", label(cm, c))
		}
		for _, sec := range secrets {
			fmt.Fprintf(&b, "└── %s\n", label(sec, c))
		}
		for _, pvc := range pvcs {
			fmt.Fprintf(&b, "└── %s\n", label(pvc, c))
			for _, bound := range c.From(pvc, graph.BoundTo) {
				fmt.Fprintf(&b, "    └── bound-to %s\n", label(bound.To, c))
			}
		}
		b.WriteString("\n")
	}

	return strings.TrimRight(b.String(), "\n") + "\n"
}

// writeWorkload prints a resource and, recursively, everything it controls.
func writeWorkload(
	b *strings.Builder,
	c *graph.Context,
	ref graph.ResourceRef,
	childrenOf map[graph.ResourceRef][]graph.ResourceRef,
	depth int,
) {
	line := label(ref, c)
	if ref.Kind == "Pod" {
		if runsOn := c.From(ref, graph.RunsOn); len(runsOn) > 0 {
			line += fmt.Sprintf("  (runs-on %s)", label(runsOn[0].To, c))
		}
	}
	fmt.Fprintf(b, "%s└── %s\n", strings.Repeat("    ", depth), line)

	children := childrenOf[ref]
	sortRefs(children)
	for _, child := range children {
		writeWorkload(b, c, child, childrenOf, depth+1)
	}
}

// resolvedRefs returns the refs of all verified-existing nodes, sorted.
func resolvedRefs(c *graph.Context) []graph.ResourceRef {
	var out []graph.ResourceRef
	for _, n := range c.Nodes() {
		if n.Resolved {
			out = append(out, n.Ref)
		}
	}
	return out
}

// refsOfKind filters refs by kind, preserving order.
func refsOfKind(refs []graph.ResourceRef, kind string) []graph.ResourceRef {
	var out []graph.ResourceRef
	for _, ref := range refs {
		if ref.Kind == kind {
			out = append(out, ref)
		}
	}
	return out
}

// sortRefs orders refs by kind then name for deterministic output.
func sortRefs(refs []graph.ResourceRef) {
	sort.Slice(refs, func(i, j int) bool {
		if refs[i].Kind != refs[j].Kind {
			return refs[i].Kind < refs[j].Kind
		}
		return refs[i].Name < refs[j].Name
	})
}

package engine

import (
	"strconv"
	"strings"

	"github.com/continuumx1/mantis/internal/graph"
)

// GraphDTO is the JSON payload the web UI fetches: cluster metadata plus a flat
// list of nodes and edges. It is a pure presentation projection of a
// graph.Context — the engine stays unaware of the transport.
type GraphDTO struct {
	Meta  MetaDTO   `json:"meta"`
	Nodes []NodeDTO `json:"nodes"`
	Edges []EdgeDTO `json:"edges"`
}

// MetaDTO carries the cluster identity and headline counts shown in the header.
// NamespaceList is every namespace in the cluster (sorted), so the UI can draw a
// region for each one — including namespaces that are empty or hold only hidden
// resources — and Namespaces counts them, keeping header and canvas consistent.
type MetaDTO struct {
	Context       string   `json:"context"`
	Server        string   `json:"server"`
	Version       string   `json:"version"`
	Namespaces    int      `json:"namespaces"`
	NamespaceList []string `json:"namespaceList,omitempty"`
	NodeCount     int      `json:"nodeCount"`
	EdgeCount     int      `json:"edgeCount"`
	Skipped       []string `json:"skipped,omitempty"`
	// NodeAutoscaler names the cluster-level node autoscaler in use ("Karpenter",
	// "cluster-autoscaler") or is empty when none is detected.
	NodeAutoscaler string `json:"nodeAutoscaler,omitempty"`
}

// NodeDTO is one resource in the graph. ID is namespace-qualified so resources
// that share a kind and name across namespaces never collide.
type NodeDTO struct {
	ID         string   `json:"id"`
	Kind       string   `json:"kind"`
	Name       string   `json:"name"`
	Namespace  string   `json:"ns"`
	Status     string   `json:"status,omitempty"`
	Resolved   bool     `json:"resolved"`
	Attributes []string `json:"attributes,omitempty"`
}

// EdgeDTO is one directed relationship, referencing nodes by their DTO IDs.
type EdgeDTO struct {
	From string `json:"from"`
	To   string `json:"to"`
	Type string `json:"type"`
}

// refID builds a stable, collision-free identifier for a resource reference:
// "<namespace>/<Kind>/<Name>". Cluster-scoped resources carry an empty namespace
// segment, which is still unique because namespaces themselves are unique.
func refID(ref graph.ResourceRef) string {
	return ref.Namespace + "/" + ref.Kind + "/" + ref.Name
}

// FromContext projects a resolved graph.Context into the wire format. Hidden
// nodes (system-managed noise) and any edges touching them are dropped so the
// live map stays as clean as possible; upstream showAll controls whether
// anything is hidden at all.
func FromContext(gctx *graph.Context, meta MetaDTO) GraphDTO {
	visible := map[graph.ResourceRef]bool{}

	nodes := make([]NodeDTO, 0)
	for _, n := range gctx.Nodes() {
		if n.Hidden {
			continue
		}
		visible[n.Ref] = true
		nodes = append(nodes, NodeDTO{
			ID:         refID(n.Ref),
			Kind:       n.Ref.Kind,
			Name:       n.Ref.Name,
			Namespace:  n.Ref.Namespace,
			Status:     statusFor(n.Ref.Kind, n.Attributes),
			Resolved:   n.Resolved,
			Attributes: n.Attributes,
		})
	}

	edges := make([]EdgeDTO, 0, len(gctx.Relations))
	for _, r := range gctx.Relations {
		// An unresolved target is never a node in the visible set but is still a
		// real edge worth drawing as a "not found" ghost, so only hidden
		// endpoints suppress an edge.
		if isHidden(gctx, r.From) || isHidden(gctx, r.To) {
			continue
		}
		edges = append(edges, EdgeDTO{
			From: refID(r.From),
			To:   refID(r.To),
			Type: string(r.Type),
		})
	}

	meta.NodeCount = len(nodes)
	meta.EdgeCount = len(edges)
	return GraphDTO{Meta: meta, Nodes: nodes, Edges: edges}
}

// isHidden reports whether a reference is a node the graph marked hidden. A
// reference that is not a node at all (a dangling target) is not hidden.
func isHidden(gctx *graph.Context, ref graph.ResourceRef) bool {
	for _, n := range gctx.Nodes() {
		if n.Ref == ref {
			return n.Hidden
		}
	}
	return false
}

// statusFor derives a coarse health signal for the status ring. Only Pods carry
// one today: their first attribute is a health note of the form
// "Headline · ready/total[ · restarts N]". A Pod that is currently running (or
// finished cleanly) with every container ready is "ok"; anything else is "crit"
// so problems draw attention. Non-Pod kinds carry no ring.
func statusFor(kind string, attrs []string) string {
	if kind != "Pod" || len(attrs) == 0 {
		return ""
	}

	parts := strings.Split(attrs[0], " · ")
	headline := strings.TrimSpace(parts[0])

	ready, total, ok := 0, 0, false
	if len(parts) > 1 {
		if r, t, parsed := parseReady(parts[1]); parsed {
			ready, total, ok = r, t, true
		}
	}

	healthy := headline == "Running" || headline == "Succeeded" || headline == "Completed"
	if healthy && ok && total > 0 && ready == total {
		return "ok"
	}
	return "crit"
}

// parseReady reads a "ready/total" fragment (e.g. "1/1") into its two counts.
func parseReady(fragment string) (ready, total int, ok bool) {
	slash := strings.IndexByte(fragment, '/')
	if slash < 0 {
		return 0, 0, false
	}
	r, err1 := strconv.Atoi(strings.TrimSpace(fragment[:slash]))
	t, err2 := strconv.Atoi(strings.TrimSpace(fragment[slash+1:]))
	if err1 != nil || err2 != nil {
		return 0, 0, false
	}
	return r, t, true
}

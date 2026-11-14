package engine

import (
	"strconv"
	"strings"

	"github.com/continuumx1/mantis/internal/graph"
)

// GraphDTO is the JSON payload the web UI fetches: cluster metadata plus a flat
// list of nodes and edges. It is a pure presentation projection of a
// graph.Context — the engine stays unaware of the transport. It is the
// canonical graph: one entry per resource, whatever view the UI draws it in.
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
	// ClusterID is a stable, opaque identifier for the cluster this graph was
	// read from (derived from the API server address), so a client holding
	// graphs from more than one cluster can never mix their resource IDs.
	ClusterID string `json:"clusterId,omitempty"`
	// Revision increases by one every time the published graph's content
	// changes. A client passes the revision it holds as ?since= to receive
	// only what changed (see GraphDeltaDTO).
	Revision uint64 `json:"revision"`
	// Partial is true while the very first sync pass is still walking
	// namespaces — the graph is real but not yet the whole cluster.
	Partial bool `json:"partial,omitempty"`
}

// NodeDTO is one resource in the graph. ID is namespace-qualified so resources
// that share a kind and name across namespaces never collide.
//
// ID ("<namespace>/<Kind>/<name>") rather than UID is the graph identity on
// purpose: every reference in a Kubernetes spec is by name, a dangling
// reference has no UID at all, and a resource deleted and recreated under the
// same name should keep its place on the map. UID is still reported, so a
// recreate is visible as a UID change.
type NodeDTO struct {
	ID         string            `json:"id"`
	Kind       string            `json:"kind"`
	Name       string            `json:"name"`
	Namespace  string            `json:"ns"`
	Scope      string            `json:"scope"`
	Status     string            `json:"status,omitempty"`
	Resolved   bool              `json:"resolved"`
	Unverified bool              `json:"unverified,omitempty"`
	Attributes []string          `json:"attributes,omitempty"`
	UID        string            `json:"uid,omitempty"`
	APIVersion string            `json:"apiVersion,omitempty"`
	Labels     map[string]string `json:"labels,omitempty"`
	Created    string            `json:"created,omitempty"`
	RV         string            `json:"rv,omitempty"`
}

// EdgeDTO is one directed relationship, referencing nodes by their DTO IDs.
// Category is the family it is styled and filtered by; Basis is the kind of
// evidence it rests on (ownership, reference, selector, endpoint, inferred) and
// Evidence the API field it was read from — so the UI can say exactly why an
// edge exists, and never present a selector match as observed traffic.
type EdgeDTO struct {
	From     string `json:"from"`
	To       string `json:"to"`
	Type     string `json:"type"`
	Category string `json:"category"`
	Basis    string `json:"basis"`
	Evidence string `json:"evidence,omitempty"`
	Inferred bool   `json:"inferred,omitempty"`
}

// key identifies an edge for set operations (deltas, deduplication).
func (e EdgeDTO) key() string { return e.From + "|" + e.Type + "|" + e.To }

// refID builds a stable, collision-free identifier for a resource reference:
// "<namespace>/<Kind>/<Name>". Cluster-scoped resources carry an empty namespace
// segment, which is still unique because namespaces themselves are unique.
func refID(ref graph.ResourceRef) string {
	return ref.String()
}

// FromContext projects a resolved graph.Context into the wire format. Hidden
// nodes (system-managed noise) and any edges touching them are dropped so the
// live map stays as clean as possible; upstream showAll controls whether
// anything is hidden at all.
//
// It runs in O(nodes + edges): hidden-ness is looked up in a map built once,
// not by rescanning (and re-sorting) every node for every edge, which made a
// 10,000-resource cluster cost hundreds of millions of comparisons per sync.
func FromContext(gctx *graph.Context, meta MetaDTO) GraphDTO {
	all := gctx.Nodes()
	hidden := make(map[graph.ResourceRef]bool, len(all))
	for _, n := range all {
		if n.Hidden {
			hidden[n.Ref] = true
		}
	}
	// A dangling reference is only worth showing if something visible makes
	// it: a missing ServiceAccount named only by a hidden system: binding is
	// control-plane noise, not a broken reference anyone can act on.
	referenced := make(map[graph.ResourceRef]bool)
	for _, r := range gctx.Relations {
		if !hidden[r.From] {
			referenced[r.To] = true
		}
	}

	nodes := make([]NodeDTO, 0, len(all))
	for _, n := range all {
		if n.Hidden || (!n.Resolved && !referenced[n.Ref]) {
			continue
		}
		scope := "Namespaced"
		if n.Ref.IsClusterScoped() {
			scope = "Cluster"
		}
		nodes = append(nodes, NodeDTO{
			ID:         refID(n.Ref),
			Kind:       n.Ref.Kind,
			Name:       n.Ref.Name,
			Namespace:  n.Ref.Namespace,
			Scope:      scope,
			Status:     statusFor(n.Ref.Kind, n.Attributes),
			Resolved:   n.Resolved,
			Unverified: !n.Resolved && n.Unverified,
			Attributes: n.Attributes,
			UID:        n.Meta.UID,
			APIVersion: n.Meta.APIVersion,
			Labels:     n.Meta.Labels,
			Created:    n.Meta.CreatedAt,
			RV:         n.Meta.ResourceVersion,
		})
	}

	edges := make([]EdgeDTO, 0, len(gctx.Relations))
	seen := make(map[string]struct{}, len(gctx.Relations))
	for _, r := range gctx.Relations {
		// An unresolved target is never a node in the visible set but is still a
		// real edge worth drawing as a "not found" ghost, so only hidden
		// endpoints suppress an edge.
		if hidden[r.From] || hidden[r.To] {
			continue
		}
		e := EdgeDTO{
			From:     refID(r.From),
			To:       refID(r.To),
			Type:     string(r.Type),
			Category: string(r.Type.Category()),
			Basis:    string(r.Basis()),
			Evidence: r.Type.Evidence(),
			Inferred: r.IsInferred(),
		}
		if _, dup := seen[e.key()]; dup {
			continue
		}
		seen[e.key()] = struct{}{}
		edges = append(edges, e)
	}

	meta.NodeCount = len(nodes)
	meta.EdgeCount = len(edges)
	return GraphDTO{Meta: meta, Nodes: nodes, Edges: edges}
}

// statusFor derives a coarse health signal for the status ring. Pods and
// Nodes carry one; every other kind carries no ring at all — not because
// they're assumed healthy, but because Mantis has no per-kind health model
// for them yet (see the project roadmap).
func statusFor(kind string, attrs []string) string {
	switch kind {
	case "Pod":
		return podStatus(attrs)
	case "Node":
		return nodeStatus(attrs)
	default:
		return ""
	}
}

// podStatus reads a Pod's health note (its first attribute, of the form
// "Headline · ready/total[ · restarts N]" — see graph.podHealthNote). A Pod
// that is currently running (or finished cleanly) with every container ready
// is "ok"; anything else is "crit" so problems draw attention.
func podStatus(attrs []string) string {
	if len(attrs) == 0 {
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

// nodeStatus reads a Node's readiness (its first attribute, "status:
// Ready|NotReady|Unknown" — see graph.nodeAttributes, which reports the
// NodeReady condition's actual value rather than assuming a Node is healthy
// whenever it isn't confirmed otherwise). Ready is "ok", NotReady is "crit",
// and Unknown (the kubelet has stopped reporting, or the condition was never
// verified) is "warn" — a real third state, not silently folded into either
// of the other two.
func nodeStatus(attrs []string) string {
	if len(attrs) == 0 {
		return ""
	}
	const prefix = "status: "
	if !strings.HasPrefix(attrs[0], prefix) {
		return ""
	}
	switch strings.TrimPrefix(attrs[0], prefix) {
	case "Ready":
		return "ok"
	case "NotReady":
		return "crit"
	default: // "Unknown", or anything unexpected
		return "warn"
	}
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

package report

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
)

// LayoutPage is one topology diagram as laid out by the Mantis UI's own
// deterministic layout engine (graph-engine.js) — the same placement, the
// same overlap-free routing the Topology page uses, so the report shows the
// picture people already know. The browser supplies only geometry and IDs;
// every name, kind, status and relationship drawn comes from the engine's
// snapshot, and anything the snapshot does not contain is rejected.
type LayoutPage struct {
	Title     string     `json:"title"`
	Kind      string     `json:"kind"` // full | overview | namespace
	Namespace string     `json:"namespace,omitempty"`
	Groups    []LGroup   `json:"groups"`
	Nodes     []LNode    `json:"nodes"`
	Edges     []LEdge    `json:"edges"`
	Bounds    [4]float64 `json:"bounds"` // x0, y0, x1, y1
	Folded    int        `json:"folded"` // resources folded into aggregates on this page
}

// LGroup is a namespace (or infrastructure) container.
type LGroup struct {
	Key       string  `json:"key"`
	X0        float64 `json:"x0"`
	Y0        float64 `json:"y0"`
	X1        float64 `json:"x1"`
	Y1        float64 `json:"y1"`
	Collapsed bool    `json:"collapsed"`
}

// LNode is a card: a resource, an aggregate ("12 Pods"), or a collapsed
// group's card.
type LNode struct {
	VID     string   `json:"vid"`
	Type    string   `json:"type"` // resource | aggregate | groupcard
	ID      string   `json:"id,omitempty"`
	Members []string `json:"members,omitempty"`
	Group   string   `json:"group"`
	X       float64  `json:"x"`
	Y       float64  `json:"y"`
	W       float64  `json:"w"`
	H       float64  `json:"h"`
	Badges  []string `json:"badges,omitempty"`
}

// LEdge is a drawn line standing for one or more canonical relationships.
type LEdge struct {
	From   string       `json:"from"`
	To     string       `json:"to"`
	Cross  bool         `json:"cross"`
	Keys   []string     `json:"keys"`
	Points [][2]float64 `json:"points"`
}

// page is a validated LayoutPage with everything resolved from the snapshot.
type page struct {
	LayoutPage
	nodes    map[string]*vnode
	edges    []vedge
	drawn    int // resources drawn as individual cards
	inFolded int
	inGroups int
	external map[string]int // namespace -> relationships leaving this page's namespace
}

type vnode struct {
	LNode
	kind, name, ns, status   string
	resolved, unverified     bool
	label                    string
	members                  []*Node
	groupLabel               string
	bad, total, pods, broken int
}

type vedge struct {
	LEdge
	category string
	types    []string
	ghost    bool
}

const maxCoord = 1e7

func finite(v ...float64) bool {
	for _, x := range v {
		if math.IsNaN(x) || math.IsInf(x, 0) || math.Abs(x) > maxCoord {
			return false
		}
	}
	return true
}

// validatePage checks a submitted layout against the snapshot and resolves
// every drawn element from it. It fails on anything the snapshot does not
// contain, so a report can only ever show the cluster as Mantis observed it.
func validatePage(lp LayoutPage, s *Snapshot) (*page, error) {
	p := &page{LayoutPage: lp, nodes: map[string]*vnode{}, external: map[string]int{}}
	if !finite(lp.Bounds[:]...) || lp.Bounds[2] <= lp.Bounds[0] || lp.Bounds[3] <= lp.Bounds[1] {
		return nil, errors.New("topology page has invalid bounds")
	}
	nsSet := map[string]bool{}
	for _, ns := range s.Graph.Namespaces {
		nsSet[ns] = true
	}
	groupLabel := func(key string) (string, error) {
		switch {
		case key == "infra":
			return "cluster infrastructure", nil
		case strings.HasPrefix(key, "ns:") && nsSet[key[3:]]:
			return key[3:], nil
		}
		return "", fmt.Errorf("unknown group %q", key)
	}
	for _, g := range lp.Groups {
		if !finite(g.X0, g.Y0, g.X1, g.Y1) {
			return nil, errors.New("group has invalid geometry")
		}
		if _, err := groupLabel(g.Key); err != nil {
			return nil, err
		}
	}
	for _, ln := range lp.Nodes {
		if !finite(ln.X, ln.Y, ln.W, ln.H) || ln.W <= 0 || ln.H <= 0 {
			return nil, fmt.Errorf("card %q has invalid geometry", ln.VID)
		}
		v := &vnode{LNode: ln}
		switch ln.Type {
		case "resource":
			n := s.Node(ln.ID)
			if n == nil || ln.VID != ln.ID {
				return nil, fmt.Errorf("resource %q is not in the cluster snapshot", ln.ID)
			}
			v.kind, v.name, v.ns, v.status, v.resolved, v.unverified = n.Kind, n.Name, n.Namespace, n.Status, n.Resolved, n.Unverified
			p.drawn++
		case "aggregate":
			if len(ln.Members) == 0 {
				return nil, fmt.Errorf("aggregate %q has no members", ln.VID)
			}
			for _, id := range ln.Members {
				n := s.Node(id)
				if n == nil {
					return nil, fmt.Errorf("aggregate member %q is not in the cluster snapshot", id)
				}
				v.members = append(v.members, n)
				if n.Status == "crit" {
					v.bad++
				}
			}
			v.kind = v.members[0].Kind
			v.label = fmt.Sprintf("%d %s", len(v.members), plural(v.kind))
			p.inFolded += len(v.members)
		case "groupcard":
			lbl, err := groupLabel(ln.Group)
			if err != nil {
				return nil, err
			}
			v.groupLabel = lbl
			ns := ""
			if strings.HasPrefix(ln.Group, "ns:") {
				ns = ln.Group[3:]
			}
			for _, n := range s.Graph.Nodes {
				if n.Namespace != ns {
					continue
				}
				v.total++
				if n.Kind == "Pod" {
					v.pods++
					if n.Status == "crit" {
						v.bad++
					}
				}
				if !n.Resolved && !n.Unverified {
					v.broken++
				}
			}
			p.inGroups += v.total
		default:
			return nil, fmt.Errorf("unknown card type %q", ln.Type)
		}
		p.nodes[ln.VID] = v
	}
	for _, le := range lp.Edges {
		if p.nodes[le.From] == nil || p.nodes[le.To] == nil || len(le.Points) < 2 || len(le.Keys) == 0 {
			return nil, errors.New("line references cards not on the page")
		}
		for _, pt := range le.Points {
			if !finite(pt[0], pt[1]) {
				return nil, errors.New("line has invalid geometry")
			}
		}
		ve := vedge{LEdge: le}
		types := map[string]bool{}
		for _, k := range le.Keys {
			e := s.Edge(k)
			if e == nil {
				return nil, fmt.Errorf("relationship %q is not in the cluster snapshot", k)
			}
			ve.category = e.Category
			types[e.Type] = true
			if t := s.Node(e.To); t != nil && !t.Resolved && !t.Unverified {
				ve.ghost = true
			}
		}
		for t := range types {
			ve.types = append(ve.types, t)
		}
		sort.Strings(ve.types)
		p.edges = append(p.edges, ve)
	}
	if lp.Kind == "namespace" && lp.Namespace != "" {
		for _, e := range s.Graph.Edges {
			a, b := s.Node(e.From), s.Node(e.To)
			if a == nil || b == nil {
				continue
			}
			if a.Namespace == lp.Namespace && b.Namespace != "" && b.Namespace != lp.Namespace {
				p.external[b.Namespace]++
			} else if b.Namespace == lp.Namespace && a.Namespace != "" && a.Namespace != lp.Namespace {
				p.external[a.Namespace]++
			}
		}
	}
	return p, nil
}

func plural(k string) string {
	switch {
	case strings.HasSuffix(k, "y"):
		return k[:len(k)-1] + "ies"
	case strings.HasSuffix(k, "s"):
		return k + "es"
	}
	return k + "s"
}

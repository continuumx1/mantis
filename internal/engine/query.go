package engine

import (
	"sort"
	"strings"
)

// graphIndex is the read-side index over one published snapshot: resources
// by ID, namespace and kind, plus incoming and outgoing adjacency. It is built
// once per publish (O(nodes + edges)) so every query below is answered without
// scanning the whole graph.
type graphIndex struct {
	nodes  []NodeDTO
	edges  []EdgeDTO
	byID   map[string]int
	out    map[string][]int // node ID -> indexes into edges where From == ID
	in     map[string][]int // node ID -> indexes into edges where To == ID
	byNS   map[string][]int
	byKind map[string][]int
	// searchKey[i] is the lower-cased "name kind ns" text for nodes[i].
	searchKey []string
}

func buildIndex(dto *GraphDTO) *graphIndex {
	ix := &graphIndex{
		nodes:     dto.Nodes,
		edges:     dto.Edges,
		byID:      make(map[string]int, len(dto.Nodes)),
		out:       make(map[string][]int, len(dto.Nodes)),
		in:        make(map[string][]int, len(dto.Nodes)),
		byNS:      map[string][]int{},
		byKind:    map[string][]int{},
		searchKey: make([]string, len(dto.Nodes)),
	}
	for i, n := range dto.Nodes {
		ix.byID[n.ID] = i
		ix.byNS[n.Namespace] = append(ix.byNS[n.Namespace], i)
		ix.byKind[strings.ToLower(n.Kind)] = append(ix.byKind[strings.ToLower(n.Kind)], i)
		ix.searchKey[i] = strings.ToLower(n.Name)
	}
	for i, e := range dto.Edges {
		ix.out[e.From] = append(ix.out[e.From], i)
		ix.in[e.To] = append(ix.in[e.To], i)
	}
	return ix
}

// SearchResult is one /api/graph/search hit.
type SearchResult struct {
	Total   int       `json:"total"`
	Results []NodeDTO `json:"results"`
}

// search matches resources against a query of whitespace-separated terms.
// Qualified terms filter: "kind:Pod", "ns:catalog", "label:app=web" (or
// "label:app" for presence). Bare terms match the resource name as a
// substring. Results rank exact name, then prefix, then substring, and are
// capped at limit (Total still reports every match).
func (ix *graphIndex) search(q string, limit int) SearchResult {
	var kind, ns string
	var labelK, labelV string
	var hasLabel, hasLabelV bool
	var words []string
	for _, term := range strings.Fields(q) {
		low := strings.ToLower(term)
		switch {
		case strings.HasPrefix(low, "kind:"):
			kind = low[len("kind:"):]
		case strings.HasPrefix(low, "ns:"):
			ns = term[len("ns:"):]
		case strings.HasPrefix(low, "label:"):
			kv := term[len("label:"):]
			hasLabel = true
			if eq := strings.IndexByte(kv, '='); eq >= 0 {
				labelK, labelV, hasLabelV = kv[:eq], kv[eq+1:], true
			} else {
				labelK = kv
			}
		default:
			words = append(words, low)
		}
	}

	// Narrow the candidate set with the cheapest available index first.
	var candidates []int
	switch {
	case ns != "":
		candidates = ix.byNS[ns]
	case kind != "":
		candidates = ix.byKind[kind]
	default:
		candidates = make([]int, len(ix.nodes))
		for i := range candidates {
			candidates[i] = i
		}
	}

	type hit struct {
		i     int
		score int
	}
	var hits []hit
	for _, i := range candidates {
		n := ix.nodes[i]
		if kind != "" && strings.ToLower(n.Kind) != kind {
			continue
		}
		if ns != "" && n.Namespace != ns {
			continue
		}
		if hasLabel {
			v, ok := n.Labels[labelK]
			if !ok || (hasLabelV && v != labelV) {
				continue
			}
		}
		score, ok := 0, true
		for _, w := range words {
			name := ix.searchKey[i]
			switch {
			case name == w:
			case strings.HasPrefix(name, w):
				score += 1
			case strings.Contains(name, w):
				score += 2
			default:
				ok = false
			}
		}
		if ok {
			hits = append(hits, hit{i, score})
		}
	}
	sort.Slice(hits, func(a, b int) bool {
		if hits[a].score != hits[b].score {
			return hits[a].score < hits[b].score
		}
		return ix.nodes[hits[a].i].ID < ix.nodes[hits[b].i].ID
	})
	res := SearchResult{Total: len(hits), Results: []NodeDTO{}}
	for k := 0; k < len(hits) && k < limit; k++ {
		res.Results = append(res.Results, ix.nodes[hits[k].i])
	}
	return res
}

// Neighborhood is a dependency query result: the focus resource, every
// resource within depth hops in the requested direction, and the edges among
// them. Depth maps each returned node to its hop distance from the focus.
type Neighborhood struct {
	Focus string         `json:"focus"`
	Nodes []NodeDTO      `json:"nodes"`
	Edges []EdgeDTO      `json:"edges"`
	Depth map[string]int `json:"depth"`
}

// neighbors walks outgoing ("out"), incoming ("in") or both directions from
// focus, up to depth hops, following only the given categories (all when
// empty). Only edges actually traversed are returned, so the result reads as
// the dependency chain rather than every relationship among the nodes.
func (ix *graphIndex) neighbors(focus, direction string, depth int, categories map[string]bool) (Neighborhood, bool) {
	if _, ok := ix.byID[focus]; !ok {
		return Neighborhood{}, false
	}
	allowed := func(e EdgeDTO) bool { return len(categories) == 0 || categories[e.Category] }
	dist := map[string]int{focus: 0}
	usedEdges := map[int]bool{}
	frontier := []string{focus}
	for d := 0; d < depth && len(frontier) > 0; d++ {
		var next []string
		for _, id := range frontier {
			if direction != "in" {
				for _, ei := range ix.out[id] {
					e := ix.edges[ei]
					if !allowed(e) {
						continue
					}
					usedEdges[ei] = true
					if _, seen := dist[e.To]; !seen {
						dist[e.To] = d + 1
						next = append(next, e.To)
					}
				}
			}
			if direction != "out" {
				for _, ei := range ix.in[id] {
					e := ix.edges[ei]
					if !allowed(e) {
						continue
					}
					usedEdges[ei] = true
					if _, seen := dist[e.From]; !seen {
						dist[e.From] = d + 1
						next = append(next, e.From)
					}
				}
			}
		}
		sort.Strings(next)
		frontier = next
	}

	res := Neighborhood{Focus: focus, Depth: dist, Nodes: []NodeDTO{}, Edges: []EdgeDTO{}}
	ids := make([]string, 0, len(dist))
	for id := range dist {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		if i, ok := ix.byID[id]; ok {
			res.Nodes = append(res.Nodes, ix.nodes[i])
		}
	}
	eis := make([]int, 0, len(usedEdges))
	for ei := range usedEdges {
		eis = append(eis, ei)
	}
	sort.Ints(eis)
	for _, ei := range eis {
		res.Edges = append(res.Edges, ix.edges[ei])
	}
	return res, true
}

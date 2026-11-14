package engine

import (
	"encoding/json"
	"sort"
	"sync"
)

// maxDeltaHistory is how many revisions back a client can ask for a delta.
// A client further behind than this (or one that has never loaded) gets the
// full snapshot instead, which is always correct, just bigger.
const maxDeltaHistory = 64

// snapshotCache holds the most recently published graph — the canonical,
// in-memory resource graph — plus what is needed to serve it incrementally.
// The background sync loop (see sync.go) is the only writer; every request is
// a reader. Requests never touch the Kubernetes API themselves.
//
// Each publish whose content differs from the previous one gets the next
// revision number and a GraphDelta describing the change, so a client that
// already holds revision N can ask for "?since=N" and receive only what
// changed instead of the whole cluster again. A publish with identical
// content keeps the current revision: no delta, no client work.
type snapshotCache struct {
	mu    sync.RWMutex
	dto   *GraphDTO
	rev   uint64
	index *graphIndex
	// fingerprints of the current snapshot, used to diff the next one.
	nodeFP map[string]string
	edges  map[string]EdgeDTO
	// deltas[i] takes revision deltas[i].From to deltas[i].To (= From+1).
	deltas []GraphDelta
	ready  chan struct{} // closed once, the first time any snapshot (even partial) is published
	once   sync.Once
}

func newSnapshotCache() *snapshotCache {
	return &snapshotCache{ready: make(chan struct{})}
}

// GraphDelta is the change between two revisions. Upserted nodes carry their
// full new state (added or changed); removed nodes and edges are named only.
type GraphDelta struct {
	From         uint64    `json:"from"`
	To           uint64    `json:"to"`
	UpsertNodes  []NodeDTO `json:"upsertNodes,omitempty"`
	RemoveNodes  []string  `json:"removeNodes,omitempty"`
	AddEdges     []EdgeDTO `json:"addEdges,omitempty"`
	RemoveEdges  []EdgeDTO `json:"removeEdges,omitempty"`
	ChangedCount int       `json:"changedCount"`
}

// GraphDeltaDTO is what /api/graph?since=N returns when it can answer with a
// delta: the current metadata plus every change from N up to Revision. When
// Unchanged is true the client is already current and nothing else is set.
type GraphDeltaDTO struct {
	Meta      MetaDTO     `json:"meta"`
	Since     uint64      `json:"since"`
	Revision  uint64      `json:"revision"`
	Unchanged bool        `json:"unchanged,omitempty"`
	Delta     *GraphDelta `json:"delta,omitempty"`
}

// publish stores dto as the current snapshot and, the first time only, marks
// the cache ready — unblocking any request that arrived before the very
// first namespace had finished syncing (see Server.handleGraph).
func (c *snapshotCache) publish(dto GraphDTO) {
	nodeFP := make(map[string]string, len(dto.Nodes))
	for _, n := range dto.Nodes {
		nodeFP[n.ID] = fingerprint(n)
	}
	edges := make(map[string]EdgeDTO, len(dto.Edges))
	for _, e := range dto.Edges {
		edges[e.key()] = e
	}

	c.mu.Lock()
	defer func() {
		c.mu.Unlock()
		c.once.Do(func() { close(c.ready) })
	}()

	if c.dto != nil {
		d := diff(c.dto.Nodes, c.nodeFP, nodeFP, c.edges, edges, dto.Nodes)
		if d.ChangedCount == 0 && sameMeta(c.dto.Meta, dto.Meta) {
			return // identical content: keep the revision, nothing to send
		}
		d.From, d.To = c.rev, c.rev+1
		c.deltas = append(c.deltas, d)
		if len(c.deltas) > maxDeltaHistory {
			c.deltas = c.deltas[len(c.deltas)-maxDeltaHistory:]
		}
	}
	c.rev++
	dto.Meta.Revision = c.rev
	c.dto = &dto
	c.nodeFP = nodeFP
	c.edges = edges
	c.index = buildIndex(&dto)
}

// get returns the current snapshot, or ok=false if nothing has been
// published yet.
func (c *snapshotCache) get() (GraphDTO, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.dto == nil {
		return GraphDTO{}, false
	}
	return *c.dto, true
}

// since answers a client that already holds revision `since`: unchanged when
// it is current, a merged delta when the history still reaches back that far,
// or ok=false when it does not (the caller then serves the full snapshot).
func (c *snapshotCache) since(since uint64) (GraphDeltaDTO, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.dto == nil || since > c.rev || since == 0 {
		return GraphDeltaDTO{}, false
	}
	out := GraphDeltaDTO{Meta: c.dto.Meta, Since: since, Revision: c.rev}
	if since == c.rev {
		out.Unchanged = true
		return out, true
	}
	if len(c.deltas) == 0 || c.deltas[0].From > since {
		return GraphDeltaDTO{}, false
	}
	merged := mergeDeltas(c.deltas, since)
	out.Delta = &merged
	return out, true
}

// withIndex runs f against the current index under the read lock.
func (c *snapshotCache) withIndex(f func(*graphIndex)) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.index == nil {
		return false
	}
	f(c.index)
	return true
}

// fingerprint is a node's content identity: any field change — status,
// attributes, labels, resourceVersion — changes it.
func fingerprint(n NodeDTO) string {
	b, _ := json.Marshal(n)
	return string(b)
}

func sameMeta(a, b MetaDTO) bool {
	a.Revision, b.Revision = 0, 0
	ab, _ := json.Marshal(a)
	bb, _ := json.Marshal(b)
	return string(ab) == string(bb)
}

// diff computes the delta from the previous snapshot to the next.
func diff(prevNodes []NodeDTO, prevFP, nextFP map[string]string, prevEdges, nextEdges map[string]EdgeDTO, nextNodes []NodeDTO) GraphDelta {
	var d GraphDelta
	for _, n := range nextNodes {
		if old, ok := prevFP[n.ID]; !ok || old != nextFP[n.ID] {
			d.UpsertNodes = append(d.UpsertNodes, n)
		}
	}
	for _, n := range prevNodes {
		if _, ok := nextFP[n.ID]; !ok {
			d.RemoveNodes = append(d.RemoveNodes, n.ID)
		}
	}
	for k, e := range nextEdges {
		if _, ok := prevEdges[k]; !ok {
			d.AddEdges = append(d.AddEdges, e)
		}
	}
	for k, e := range prevEdges {
		if _, ok := nextEdges[k]; !ok {
			d.RemoveEdges = append(d.RemoveEdges, e)
		}
	}
	sortEdges(d.AddEdges)
	sortEdges(d.RemoveEdges)
	sort.Strings(d.RemoveNodes)
	d.ChangedCount = len(d.UpsertNodes) + len(d.RemoveNodes) + len(d.AddEdges) + len(d.RemoveEdges)
	return d
}

// mergeDeltas folds every delta from revision `since` onward into one, so a
// client a few revisions behind applies a single change set. Later changes win:
// a node upserted then removed ends up removed, and vice versa.
func mergeDeltas(deltas []GraphDelta, since uint64) GraphDelta {
	upserts := map[string]NodeDTO{}
	removed := map[string]bool{}
	addE := map[string]EdgeDTO{}
	remE := map[string]EdgeDTO{}
	var last uint64
	for _, d := range deltas {
		if d.From < since {
			continue
		}
		last = d.To
		for _, id := range d.RemoveNodes {
			delete(upserts, id)
			removed[id] = true
		}
		for _, n := range d.UpsertNodes {
			delete(removed, n.ID)
			upserts[n.ID] = n
		}
		for _, e := range d.RemoveEdges {
			delete(addE, e.key())
			remE[e.key()] = e
		}
		for _, e := range d.AddEdges {
			delete(remE, e.key())
			addE[e.key()] = e
		}
	}
	out := GraphDelta{From: since, To: last}
	for _, n := range upserts {
		out.UpsertNodes = append(out.UpsertNodes, n)
	}
	sort.Slice(out.UpsertNodes, func(i, j int) bool { return out.UpsertNodes[i].ID < out.UpsertNodes[j].ID })
	for id := range removed {
		out.RemoveNodes = append(out.RemoveNodes, id)
	}
	sort.Strings(out.RemoveNodes)
	for _, e := range addE {
		out.AddEdges = append(out.AddEdges, e)
	}
	for _, e := range remE {
		out.RemoveEdges = append(out.RemoveEdges, e)
	}
	sortEdges(out.AddEdges)
	sortEdges(out.RemoveEdges)
	out.ChangedCount = len(out.UpsertNodes) + len(out.RemoveNodes) + len(out.AddEdges) + len(out.RemoveEdges)
	return out
}

func sortEdges(es []EdgeDTO) {
	sort.Slice(es, func(i, j int) bool { return es[i].key() < es[j].key() })
}

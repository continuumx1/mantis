package engine

import "sync"

// snapshotCache holds the most recently published graph. The background sync
// loop (see sync.go) is the only writer; every /api/graph request is a
// reader. Requests never touch the Kubernetes API themselves — they read
// whatever the background loop has assembled so far, which is what lets
// /api/graph answer instantly regardless of cluster size, instead of a slow
// cluster read happening inline on every single poll.
type snapshotCache struct {
	mu    sync.RWMutex
	dto   *GraphDTO
	ready chan struct{} // closed once, the first time any snapshot (even partial) is published
	once  sync.Once
}

func newSnapshotCache() *snapshotCache {
	return &snapshotCache{ready: make(chan struct{})}
}

// publish stores dto as the current snapshot and, the first time only, marks
// the cache ready — unblocking any request that arrived before the very
// first namespace had finished syncing (see Server.handleGraph).
func (c *snapshotCache) publish(dto GraphDTO) {
	c.mu.Lock()
	c.dto = &dto
	c.mu.Unlock()
	c.once.Do(func() { close(c.ready) })
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

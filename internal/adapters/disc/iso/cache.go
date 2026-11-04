package iso

import "sync"

const (
	// maxCachedDirs and maxCachedEntries bound the listing cache; it is cleared when either is hit.
	maxCachedDirs    = 256
	maxCachedEntries = 1 << 16
)

// nodeID identifies a directory inside the image (UDF: partition and block, ISO 9660: first
// extent offset and length). It detects cycles and keys the listing cache.
type nodeID struct{ first, second int64 }

// listCache keeps parsed directory listings so each directory is parsed once, not once per
// path component of every lookup. It is safe for concurrent use.
type listCache struct {
	mu      sync.Mutex
	dirs    map[nodeID][]entry
	entries int
}

func newListCache() *listCache {
	return &listCache{dirs: map[nodeID][]entry{}}
}

// get returns the shared listing; callers must not modify it.
func (c *listCache) get(id nodeID) ([]entry, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	list, ok := c.dirs[id]
	return list, ok
}

func (c *listCache) put(id nodeID, list []entry) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.dirs) >= maxCachedDirs || c.entries+len(list) > maxCachedEntries {
		c.dirs = map[nodeID][]entry{}
		c.entries = 0
	}
	c.dirs[id] = list
	c.entries += len(list)
}

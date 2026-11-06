package auth

import (
	"sync"
	"time"
)

// sessionCache keeps resolved tokens for CacheTTL, keyed by token hash, so HLS segment
// fetches do not hit the database on every request (ADR 0032 §3).
type sessionCache struct {
	mutex   sync.Mutex
	entries map[string]cacheEntry
	// generation increases with every eviction. A Resolve that loaded from the store before
	// an eviction must not put its now possibly revoked result back afterwards.
	generation uint64
}

type cacheEntry struct {
	resolution Resolution
	storedAt   time.Time
}

func newSessionCache() *sessionCache {
	return &sessionCache{entries: map[string]cacheEntry{}}
}

// get returns a fresh entry for tokenHash.
func (cache *sessionCache) get(tokenHash string, now time.Time) (Resolution, bool) {
	cache.mutex.Lock()
	defer cache.mutex.Unlock()

	entry, ok := cache.entries[tokenHash]
	if !ok || !entry.fresh(now) {
		return Resolution{}, false
	}

	return entry.resolution, true
}

// currentGeneration returns the generation to pass to put after a store read.
func (cache *sessionCache) currentGeneration() uint64 {
	cache.mutex.Lock()
	defer cache.mutex.Unlock()

	return cache.generation
}

// put stores resolution unless an eviction happened since generation was read.
func (cache *sessionCache) put(tokenHash string, resolution Resolution, now time.Time, generation uint64) {
	cache.mutex.Lock()
	defer cache.mutex.Unlock()

	if generation != cache.generation {
		return
	}

	if len(cache.entries) >= pruneThreshold {
		cache.pruneLocked(now)
	}

	cache.entries[tokenHash] = cacheEntry{resolution: resolution, storedAt: now}
}

// evict drops every entry whose resolution matches.
func (cache *sessionCache) evict(matches func(Resolution) bool) {
	cache.mutex.Lock()
	defer cache.mutex.Unlock()

	cache.generation++

	for tokenHash, entry := range cache.entries {
		if matches(entry.resolution) {
			delete(cache.entries, tokenHash)
		}
	}
}

// prune drops stale entries.
func (cache *sessionCache) prune(now time.Time) {
	cache.mutex.Lock()
	defer cache.mutex.Unlock()

	cache.pruneLocked(now)
}

func (cache *sessionCache) pruneLocked(now time.Time) {
	for tokenHash, entry := range cache.entries {
		if !entry.fresh(now) {
			delete(cache.entries, tokenHash)
		}
	}
}

func (entry cacheEntry) fresh(now time.Time) bool {
	age := now.Sub(entry.storedAt)

	return age >= 0 && age < CacheTTL
}

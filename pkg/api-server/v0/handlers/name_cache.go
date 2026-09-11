package handlers

import (
	"math/rand"
	"sync"
	"time"
)

// An in-process cache of object names keyed by type and id. There is no
// invalidation path; a short TTL is the freshness bound.

// nameCacheTTL is how long a resolved object name is reused before the next lookup.
const nameCacheTTL = 30 * time.Second

// nameCacheSweepInterval is how often expired entries are removed. Twice the
// TTL so expiry is reclaimed by the sweeper, not at read time.
const nameCacheSweepInterval = 60 * time.Second

// nameCacheMaxEntries is the cap on cached names. Random eviction bounds memory.
const nameCacheMaxEntries = 5000

// nameCacheKey is the lookup key for one cached name.
type nameCacheKey struct {
	ObjType string
	ID      uint
}

// nameCacheEntry is one cached name and the time it becomes a miss.
type nameCacheEntry struct {
	Name      string
	ExpiresAt time.Time
}

// nameCache is a bounded, TTL map of object names.
type nameCache struct {
	mu      sync.RWMutex
	entries map[nameCacheKey]nameCacheEntry
}

// moduleNameCache is the process-wide cache of resolved object names.
var moduleNameCache = &nameCache{
	entries: make(map[nameCacheKey]nameCacheEntry, nameCacheMaxEntries),
}

// Get returns a cached name that has not expired. An expired entry is a miss
// and stays in the map until the next sweep.
func (c *nameCache) Get(objType string, id uint) (string, bool) {
	c.mu.RLock()
	entry, ok := c.entries[nameCacheKey{ObjType: objType, ID: id}]
	c.mu.RUnlock()
	if !ok {
		return "", false
	}
	// treat an expired entry as a miss without taking the write lock
	if time.Now().After(entry.ExpiresAt) {
		return "", false
	}
	return entry.Name, true
}

// Put stores a name with a fresh expiry. Evicts at random if the map is at cap.
func (c *nameCache) Put(objType string, id uint, name string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	// make room for this insert when the map is already at cap
	if len(c.entries) >= nameCacheMaxEntries {
		c.evictRandomLocked(len(c.entries) - nameCacheMaxEntries + 1)
	}
	c.entries[nameCacheKey{ObjType: objType, ID: id}] = nameCacheEntry{
		Name:      name,
		ExpiresAt: time.Now().Add(nameCacheTTL),
	}
}

// sweep deletes expired entries and randomly evicts down to the cap if still over.
func (c *nameCache) sweep() {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now()
	// drop entries whose TTL has elapsed
	for k, entry := range c.entries {
		if now.After(entry.ExpiresAt) {
			delete(c.entries, k)
		}
	}
	// hold the cap if expiry alone did not
	if len(c.entries) > nameCacheMaxEntries {
		c.evictRandomLocked(len(c.entries) - nameCacheMaxEntries)
	}
}

// evictRandomLocked drops n entries. Map iteration order supplies the randomness.
// The caller must hold c.mu for write.
func (c *nameCache) evictRandomLocked(n int) {
	if n <= 0 {
		return
	}
	dropped := 0
	for k := range c.entries {
		delete(c.entries, k)
		dropped++
		if dropped >= n {
			return
		}
	}
}

// init starts the background sweeper for the process lifetime.
func init() {
	go func() {
		// delay the first sweep so processes started together do not lockstep
		jitter := time.Duration(rand.Int63n(int64(nameCacheSweepInterval)))
		time.Sleep(jitter)
		ticker := time.NewTicker(nameCacheSweepInterval)
		defer ticker.Stop()
		for range ticker.C {
			moduleNameCache.sweep()
		}
	}()
}

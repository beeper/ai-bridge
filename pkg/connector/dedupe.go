package connector

import (
	"sync"
	"time"
)

const (
	// DefaultDedupeTTL is the time-to-live for deduplication entries (20 minutes like clawdbot)
	DefaultDedupeTTL = 20 * time.Minute
	// DefaultDedupeMaxSize is the maximum number of entries in the cache
	DefaultDedupeMaxSize = 5000
)

// DedupeCache is a thread-safe LRU cache with TTL for message deduplication.
// Based on clawdbot's dedupe.ts implementation.
type DedupeCache struct {
	mu      sync.Mutex
	entries map[string]int64 // key → timestamp (unix ms)
	ttl     time.Duration
	maxSize int
}

// NewDedupeCache creates a new deduplication cache with the given TTL and max size.
func NewDedupeCache(ttl time.Duration, maxSize int) *DedupeCache {
	if ttl <= 0 {
		ttl = DefaultDedupeTTL
	}
	if maxSize <= 0 {
		maxSize = DefaultDedupeMaxSize
	}
	return &DedupeCache{
		entries: make(map[string]int64),
		ttl:     ttl,
		maxSize: maxSize,
	}
}

// Check returns true if key is a duplicate (seen within TTL).
// Also records the key for future checks.
func (c *DedupeCache) Check(key string) bool {
	if key == "" {
		return false
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now().UnixNano()
	cutoff := now - c.ttl.Nanoseconds()

	// Check if exists and not expired
	if ts, ok := c.entries[key]; ok && ts > cutoff {
		c.touch(key, now)
		return true // Duplicate
	}

	// Record and prune
	c.touch(key, now)
	c.prune(cutoff)
	return false // First time
}

// touch updates the timestamp for a key, moving it to the end of the LRU order.
func (c *DedupeCache) touch(key string, now int64) {
	delete(c.entries, key)
	c.entries[key] = now
}

// prune removes expired entries and evicts oldest if over max size.
func (c *DedupeCache) prune(cutoff int64) {
	// Expire old entries
	for k, ts := range c.entries {
		if ts < cutoff {
			delete(c.entries, k)
		}
	}
	// LRU eviction if over max size
	for len(c.entries) > c.maxSize {
		var oldest string
		var oldestTs int64 = 1<<63 - 1
		for k, ts := range c.entries {
			if ts < oldestTs {
				oldest, oldestTs = k, ts
			}
		}
		if oldest != "" {
			delete(c.entries, oldest)
		}
	}
}

// Size returns the current number of entries in the cache.
func (c *DedupeCache) Size() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.entries)
}

// Clear removes all entries from the cache.
func (c *DedupeCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = make(map[string]int64)
}

package cache

import (
	"sync"
	"time"
)

type entry[T any] struct {
	data      T
	expiresAt time.Time
}

// Cache is a generic in-memory TTL cache, safe for concurrent use.
type Cache[T any] struct {
	mu      sync.RWMutex
	entries map[string]entry[T]
	ttl     time.Duration

	hits   int64
	misses int64
}

func New[T any](ttl time.Duration) *Cache[T] {
	c := &Cache[T]{
		entries: make(map[string]entry[T]),
		ttl:     ttl,
	}
	go c.cleanup()
	return c
}

func (c *Cache[T]) Get(key string) (T, bool) {
	var zero T

	c.mu.RLock()
	defer c.mu.RUnlock()

	e, exists := c.entries[key]
	if !exists {
		c.mu.RUnlock()
		c.mu.Lock()
		c.misses++
		c.mu.Unlock()
		c.mu.RLock()
		return zero, false
	}

	if time.Now().After(e.expiresAt) {
		c.mu.RUnlock()
		c.mu.Lock()
		c.misses++
		c.mu.Unlock()
		c.mu.RLock()
		return zero, false
	}

	c.mu.RUnlock()
	c.mu.Lock()
	c.hits++
	c.mu.Unlock()
	c.mu.RLock()

	return e.data, true
}

func (c *Cache[T]) Set(key string, data T) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries[key] = entry[T]{
		data:      data,
		expiresAt: time.Now().Add(c.ttl),
	}
}

func (c *Cache[T]) Stats() (hits, misses int64) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.hits, c.misses
}

func (c *Cache[T]) cleanup() {
	ticker := time.NewTicker(c.ttl)
	defer ticker.Stop()

	for range ticker.C {
		c.mu.Lock()
		now := time.Now()
		for key, e := range c.entries {
			if now.After(e.expiresAt) {
				delete(c.entries, key)
			}
		}
		c.mu.Unlock()
	}
}

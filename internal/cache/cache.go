package cache

import (
	"sync"
	"time"

	"github.com/jobrunner/hostus/internal/taxonomy"
)

type entry struct {
	data      []taxonomy.TaxonSuggestion
	expiresAt time.Time
}

type Cache struct {
	mu      sync.RWMutex
	entries map[string]entry
	ttl     time.Duration

	hits   int64
	misses int64
}

func New(ttl time.Duration) *Cache {
	c := &Cache{
		entries: make(map[string]entry),
		ttl:     ttl,
	}
	go c.cleanup()
	return c
}

func (c *Cache) Get(key string) ([]taxonomy.TaxonSuggestion, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	e, exists := c.entries[key]
	if !exists {
		c.mu.RUnlock()
		c.mu.Lock()
		c.misses++
		c.mu.Unlock()
		c.mu.RLock()
		return nil, false
	}

	if time.Now().After(e.expiresAt) {
		c.mu.RUnlock()
		c.mu.Lock()
		c.misses++
		c.mu.Unlock()
		c.mu.RLock()
		return nil, false
	}

	c.mu.RUnlock()
	c.mu.Lock()
	c.hits++
	c.mu.Unlock()
	c.mu.RLock()

	return e.data, true
}

func (c *Cache) Set(key string, data []taxonomy.TaxonSuggestion) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries[key] = entry{
		data:      data,
		expiresAt: time.Now().Add(c.ttl),
	}
}

func (c *Cache) Stats() (hits, misses int64) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.hits, c.misses
}

func (c *Cache) cleanup() {
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

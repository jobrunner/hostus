package cache

import (
	"testing"
	"time"

	"github.com/jobrunner/hostus/internal/taxonomy"
)

func TestCache_SetAndGet(t *testing.T) {
	c := New(time.Minute)

	data := []taxonomy.TaxonSuggestion{
		{AcceptedKey: 1, AcceptedName: "Test"},
	}

	c.Set("key1", data)

	result, ok := c.Get("key1")
	if !ok {
		t.Error("expected cache hit")
	}
	if len(result) != 1 {
		t.Errorf("expected 1 item, got %d", len(result))
	}
	if result[0].AcceptedKey != 1 {
		t.Errorf("expected AcceptedKey 1, got %d", result[0].AcceptedKey)
	}
}

func TestCache_Miss(t *testing.T) {
	c := New(time.Minute)

	_, ok := c.Get("nonexistent")
	if ok {
		t.Error("expected cache miss")
	}
}

func TestCache_Expiration(t *testing.T) {
	c := New(50 * time.Millisecond)

	data := []taxonomy.TaxonSuggestion{
		{AcceptedKey: 1, AcceptedName: "Test"},
	}

	c.Set("key1", data)

	// Should be found immediately
	_, ok := c.Get("key1")
	if !ok {
		t.Error("expected cache hit before expiration")
	}

	// Wait for expiration
	time.Sleep(100 * time.Millisecond)

	_, ok = c.Get("key1")
	if ok {
		t.Error("expected cache miss after expiration")
	}
}

func TestCache_Stats(t *testing.T) {
	c := New(time.Minute)

	data := []taxonomy.TaxonSuggestion{
		{AcceptedKey: 1, AcceptedName: "Test"},
	}

	c.Set("key1", data)

	// Hit
	c.Get("key1")
	// Miss
	c.Get("nonexistent")

	hits, misses := c.Stats()
	if hits != 1 {
		t.Errorf("expected 1 hit, got %d", hits)
	}
	if misses != 1 {
		t.Errorf("expected 1 miss, got %d", misses)
	}
}

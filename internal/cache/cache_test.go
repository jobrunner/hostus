package cache

import (
	"testing"
	"time"
)

type testItem struct {
	Key  int
	Name string
}

func TestCache_SetAndGet(t *testing.T) {
	c := New[[]testItem](time.Minute)

	data := []testItem{
		{Key: 1, Name: "Test"},
	}

	c.Set("key1", data)

	result, ok := c.Get("key1")
	if !ok {
		t.Error("expected cache hit")
	}
	if len(result) != 1 {
		t.Errorf("expected 1 item, got %d", len(result))
	}
	if result[0].Key != 1 {
		t.Errorf("expected Key 1, got %d", result[0].Key)
	}
}

func TestCache_Miss(t *testing.T) {
	c := New[[]testItem](time.Minute)

	_, ok := c.Get("nonexistent")
	if ok {
		t.Error("expected cache miss")
	}
}

func TestCache_Expiration(t *testing.T) {
	c := New[[]testItem](50 * time.Millisecond)

	data := []testItem{
		{Key: 1, Name: "Test"},
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
	c := New[[]testItem](time.Minute)

	data := []testItem{
		{Key: 1, Name: "Test"},
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

package middleware

import (
	"testing"
	"time"
)

func TestRateLimiter_AllowsWithinLimit(t *testing.T) {
	rl := NewRateLimiter(10)

	for i := 0; i < 10; i++ {
		if !rl.Allow() {
			t.Errorf("expected Allow to return true for request %d", i+1)
		}
	}
}

func TestRateLimiter_RejectsOverLimit(t *testing.T) {
	rl := NewRateLimiter(5)

	for i := 0; i < 5; i++ {
		rl.Allow()
	}

	if rl.Allow() {
		t.Error("expected Allow to return false when over limit")
	}
}

func TestRateLimiter_RefillsOverTime(t *testing.T) {
	rl := NewRateLimiter(10)

	// Use all tokens
	for i := 0; i < 10; i++ {
		rl.Allow()
	}

	if rl.Allow() {
		t.Error("expected no tokens immediately after exhaustion")
	}

	// Wait for refill
	time.Sleep(150 * time.Millisecond)

	if !rl.Allow() {
		t.Error("expected tokens to refill after waiting")
	}
}

func TestRateLimiter_TracksRejects(t *testing.T) {
	rl := NewRateLimiter(2)

	rl.Allow()
	rl.Allow()

	if rl.Rejects() != 0 {
		t.Errorf("expected 0 rejects initially, got %d", rl.Rejects())
	}

	rl.Allow() // Should be rejected
	rl.Allow() // Should be rejected

	if rl.Rejects() != 2 {
		t.Errorf("expected 2 rejects, got %d", rl.Rejects())
	}
}

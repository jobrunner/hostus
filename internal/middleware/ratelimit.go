package middleware

import (
	"net/http"
	"sync"
	"time"

	"github.com/jobrunner/hostus/internal/httperr"
)

type RateLimiter struct {
	mu         sync.Mutex
	tokens     float64
	maxTokens  float64
	refillRate float64
	lastRefill time.Time
	rejects    int64
}

func NewRateLimiter(requestsPerSecond int) *RateLimiter {
	return &RateLimiter{
		tokens:     float64(requestsPerSecond),
		maxTokens:  float64(requestsPerSecond),
		refillRate: float64(requestsPerSecond),
		lastRefill: time.Now(),
	}
}

func (rl *RateLimiter) Allow() bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(rl.lastRefill).Seconds()
	rl.tokens += elapsed * rl.refillRate
	if rl.tokens > rl.maxTokens {
		rl.tokens = rl.maxTokens
	}
	rl.lastRefill = now

	if rl.tokens >= 1 {
		rl.tokens--
		return true
	}

	rl.rejects++
	return false
}

func (rl *RateLimiter) Rejects() int64 {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	return rl.rejects
}

func RateLimit(limiter *RateLimiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !limiter.Allow() {
				httperr.RateLimitError(w)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

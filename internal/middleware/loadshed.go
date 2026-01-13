package middleware

import (
	"net/http"
	"sync"
	"time"

	"github.com/jobrunner/hostus/internal/httperr"
)

type LoadShedder struct {
	mu                sync.RWMutex
	consecutiveErrors int
	threshold         int
	backoff           time.Duration
	lastErrorTime     time.Time
	shedding          bool
}

func NewLoadShedder(threshold int, backoff time.Duration) *LoadShedder {
	return &LoadShedder{
		threshold: threshold,
		backoff:   backoff,
	}
}

func (ls *LoadShedder) RecordError() {
	ls.mu.Lock()
	defer ls.mu.Unlock()

	ls.consecutiveErrors++
	ls.lastErrorTime = time.Now()

	if ls.consecutiveErrors >= ls.threshold {
		ls.shedding = true
	}
}

func (ls *LoadShedder) RecordSuccess() {
	ls.mu.Lock()
	defer ls.mu.Unlock()

	ls.consecutiveErrors = 0
	ls.shedding = false
}

func (ls *LoadShedder) ShouldShed() bool {
	ls.mu.RLock()
	defer ls.mu.RUnlock()

	if !ls.shedding {
		return false
	}

	// Allow probe request after backoff
	if time.Since(ls.lastErrorTime) > ls.backoff {
		return false
	}

	return true
}

func (ls *LoadShedder) IsShedding() bool {
	ls.mu.RLock()
	defer ls.mu.RUnlock()
	return ls.shedding
}

func (ls *LoadShedder) ConsecutiveErrors() int {
	ls.mu.RLock()
	defer ls.mu.RUnlock()
	return ls.consecutiveErrors
}

func LoadShed(shedder *LoadShedder) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if shedder.ShouldShed() {
				httperr.UpstreamOverloadedError(w)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

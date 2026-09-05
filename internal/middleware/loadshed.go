// Package middleware provides HTTP middleware for request handling, including
// load shedding to protect upstream services.
//
// LoadShed was a no-op during v2 migration (RecordError had no callers);
// as of 2026-09-05, the middleware observes response status and triggers
// the breaker on consecutive 5xx errors. It is a safety valve, not a
// latency regulator.
package middleware

import (
	"net/http"
	"sync"
	"time"

	"github.com/jobrunner/hostus/internal/httperr"
)

// statusCapturingWriter wraps http.ResponseWriter to capture the status code.
type statusCapturingWriter struct {
	http.ResponseWriter
	status int
}

func (scw *statusCapturingWriter) WriteHeader(status int) {
	scw.status = status
	scw.ResponseWriter.WriteHeader(status)
}

func (scw *statusCapturingWriter) Write(b []byte) (int, error) {
	// Ensure WriteHeader is called before Write to capture status.
	// If Write is called without WriteHeader, the ResponseWriter
	// defaults to http.StatusOK (200).
	if scw.status == 0 {
		scw.status = http.StatusOK
	}
	return scw.ResponseWriter.Write(b)
}

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
		LoadSheddingActive.Set(1)
	}
}

func (ls *LoadShedder) RecordSuccess() {
	ls.mu.Lock()
	defer ls.mu.Unlock()

	ls.consecutiveErrors = 0
	ls.shedding = false
	LoadSheddingActive.Set(0)
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
				// The shed short-circuit itself records NOTHING: counting our
				// own 503s as errors would keep the breaker latched forever.
				httperr.UpstreamOverloadedError(w)
				return
			}

			scw := &statusCapturingWriter{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(scw, r)

			// Record error for 5xx, success otherwise (including 4xx client errors).
			if scw.status >= http.StatusInternalServerError {
				shedder.RecordError()
			} else {
				shedder.RecordSuccess()
			}
		})
	}
}

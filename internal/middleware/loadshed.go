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
	"strings"
	"sync"
	"time"

	"github.com/jobrunner/hostus/internal/httperr"
)

// healthMetricsAllowlist defines paths that bypass the load shedder and are
// not recorded in error statistics. Health and metrics endpoints must remain
// available even when the breaker is latched; including them in recording
// statistics can create failure loops (probes fail → container killed →
// restart → probe fails again). Metrics also becomes invisible just when
// needed most. These paths bypass recording entirely (neither RecordError
// nor RecordSuccess) to prevent probes from resetting the error counter.
var healthMetricsAllowlist = []string{
	"/metrics", // Exact match for Prometheus metrics
	"/health/", // Prefix match for all health endpoints (/health/live, /health/ready, etc.)
}

// statusRecorder wraps http.ResponseWriter to capture the HTTP response status.
// It is shared across Metrics and LoadShed middleware to avoid duplication;
// both need only the status code to drive their respective logic (metrics
// buckets and circuit-breaker decisions). Logging middleware extends this
// with size tracking via its own responseWriter type.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (sr *statusRecorder) WriteHeader(status int) {
	sr.status = status
	sr.ResponseWriter.WriteHeader(status)
}

// isHealthMetricsPath checks if a request path should bypass load shedding
// and error recording (health checks and metrics).
func isHealthMetricsPath(path string) bool {
	for _, allowed := range healthMetricsAllowlist {
		if allowed == path || strings.HasPrefix(path, allowed) {
			return true
		}
	}
	return false
}

// recordResponse evaluates the handler response and updates load shedder state.
// It is called from a defer block to ensure panics are caught and recorded.
func recordResponse(shedder *LoadShedder, sr *statusRecorder, r *http.Request, isHealthMetrics bool, panicRecovered bool) {
	if panicRecovered {
		// Handler panicked; we recorded it in the panic handler.
		return
	}

	// Health/metrics paths bypass recording entirely: they should not
	// reset the error counter or change the breaker state, even if
	// they return 2xx during an outage.
	if isHealthMetrics {
		return
	}

	// Client context cancellations (abort, timeout) must not feed
	// the server breaker; client events are orthogonal to upstream
	// health. Check this after the handler, because a handler may
	// inspect r.Context().Err() after returning.
	if r.Context().Err() != nil {
		return
	}

	// Record error for 5xx, success otherwise (including 4xx).
	if sr.status >= http.StatusInternalServerError {
		shedder.RecordError()
	} else {
		shedder.RecordSuccess()
	}
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
			// Health and metrics paths bypass shedding to prevent restart loops
			// when the breaker is latched; the orchestrator needs liveness probes
			// and metrics to function even during outages.
			isHealthMetrics := isHealthMetricsPath(r.URL.Path)

			if !isHealthMetrics && shedder.ShouldShed() {
				// The shed short-circuit itself records NOTHING: counting our
				// own 503s as errors would keep the breaker latched forever.
				httperr.UpstreamOverloadedError(w)
				return
			}

			sr := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
			panicRecovered := false

			// Deferred recording ensures we capture all outcomes: normal
			// responses, panics (recorded as errors), and client aborts
			// (not recorded, as they don't reflect upstream health).
			defer func() {
				recordResponse(shedder, sr, r, isHealthMetrics, panicRecovered)
			}()

			// Call the handler; panics are caught, recorded as errors, then re-panicked.
			func() {
				defer func() {
					if err := recover(); err != nil {
						panicRecovered = true
						// Record the panic as an error; sr.status was never set by
						// the panicking handler, so it stays at http.StatusOK.
						// Treat it as an error for breaker purposes.
						shedder.RecordError()
						// Re-panic to let the default net/http panic handler run
						panic(err)
					}
				}()
				next.ServeHTTP(sr, r)
			}()
		})
	}
}

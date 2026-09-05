package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestLoadShedder_InitialState(t *testing.T) {
	ls := NewLoadShedder(3, time.Second)

	if ls.IsShedding() {
		t.Error("expected not shedding initially")
	}
	if ls.ShouldShed() {
		t.Error("expected ShouldShed to return false initially")
	}
	if ls.ConsecutiveErrors() != 0 {
		t.Errorf("expected 0 consecutive errors, got %d", ls.ConsecutiveErrors())
	}
}

func TestLoadShedder_ActivatesAfterThreshold(t *testing.T) {
	ls := NewLoadShedder(3, time.Second)

	ls.RecordError()
	ls.RecordError()
	if ls.IsShedding() {
		t.Error("expected not shedding after 2 errors")
	}

	ls.RecordError()
	if !ls.IsShedding() {
		t.Error("expected shedding after 3 errors")
	}
	if !ls.ShouldShed() {
		t.Error("expected ShouldShed to return true")
	}
}

func TestLoadShedder_ResetOnSuccess(t *testing.T) {
	ls := NewLoadShedder(3, time.Second)

	ls.RecordError()
	ls.RecordError()
	ls.RecordError()

	if !ls.IsShedding() {
		t.Error("expected shedding after errors")
	}

	ls.RecordSuccess()

	if ls.IsShedding() {
		t.Error("expected not shedding after success")
	}
	if ls.ConsecutiveErrors() != 0 {
		t.Errorf("expected 0 consecutive errors after success, got %d", ls.ConsecutiveErrors())
	}
}

func TestLoadShedder_AllowsProbeAfterBackoff(t *testing.T) {
	ls := NewLoadShedder(3, 50*time.Millisecond)

	ls.RecordError()
	ls.RecordError()
	ls.RecordError()

	if !ls.ShouldShed() {
		t.Error("expected ShouldShed immediately after errors")
	}

	time.Sleep(100 * time.Millisecond)

	if ls.ShouldShed() {
		t.Error("expected ShouldShed to return false after backoff (allowing probe)")
	}
}

func TestLoadShedder_PartialErrorsDoNotActivate(t *testing.T) {
	ls := NewLoadShedder(5, time.Second)

	ls.RecordError()
	ls.RecordError()
	ls.RecordSuccess() // Reset
	ls.RecordError()
	ls.RecordError()

	if ls.IsShedding() {
		t.Error("expected not shedding with intermittent successes")
	}
	if ls.ConsecutiveErrors() != 2 {
		t.Errorf("expected 2 consecutive errors, got %d", ls.ConsecutiveErrors())
	}
}

// TestLoadShed_Middleware_5xx_increments verifies that 5xx responses
// increment the error counter.
func TestLoadShed_Middleware_5xx_increments(t *testing.T) {
	ls := NewLoadShedder(3, 10*time.Millisecond)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	})

	middleware := LoadShed(ls)
	wrappedHandler := middleware(handler)

	// Make 3 requests returning 500 each
	for i := 0; i < 3; i++ {
		resp := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/", nil)
		wrappedHandler.ServeHTTP(resp, req)

		if resp.Code != 500 {
			t.Errorf("Request %d: expected 500, got %d", i+1, resp.Code)
		}
	}

	if ls.ConsecutiveErrors() != 3 {
		t.Errorf("expected 3 consecutive errors, got %d", ls.ConsecutiveErrors())
	}
	if !ls.IsShedding() {
		t.Errorf("expected shedding after threshold reached")
	}
}

// TestLoadShed_Middleware_non5xx_resets verifies that non-5xx responses
// reset the error counter.
func TestLoadShed_Middleware_non5xx_resets(t *testing.T) {
	ls := NewLoadShedder(3, 10*time.Millisecond)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	})

	middleware := LoadShed(ls)
	wrappedHandler := middleware(handler)

	// Record some errors manually
	ls.RecordError()
	ls.RecordError()
	if ls.ConsecutiveErrors() != 2 {
		t.Errorf("expected 2 errors before reset, got %d", ls.ConsecutiveErrors())
	}

	// Make request returning 200
	resp := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	wrappedHandler.ServeHTTP(resp, req)

	if ls.ConsecutiveErrors() != 0 {
		t.Errorf("expected 0 errors after 200 response, got %d", ls.ConsecutiveErrors())
	}
}

// TestLoadShed_Middleware_4xx_success verifies that 4xx responses are
// counted as success (not errors).
func TestLoadShed_Middleware_4xx_success(t *testing.T) {
	ls := NewLoadShedder(2, 10*time.Millisecond)

	statusCodes := []int{500, 404}
	idx := 0

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(statusCodes[idx])
		idx++
	})

	middleware := LoadShed(ls)
	wrappedHandler := middleware(handler)

	// Request 1: 500 (error)
	resp := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	wrappedHandler.ServeHTTP(resp, req)
	if ls.ConsecutiveErrors() != 1 {
		t.Errorf("expected 1 error after 500, got %d", ls.ConsecutiveErrors())
	}

	// Request 2: 404 (success, not error)
	resp = httptest.NewRecorder()
	req = httptest.NewRequest("GET", "/", nil)
	wrappedHandler.ServeHTTP(resp, req)
	if ls.ConsecutiveErrors() != 0 {
		t.Errorf("expected 0 errors after 404, got %d", ls.ConsecutiveErrors())
	}
}

// TestLoadShed_Middleware_shed_blocks verifies that shedding responses (503)
// do not increment the error counter.
func TestLoadShed_Middleware_shed_blocks(t *testing.T) {
	ls := NewLoadShedder(1, 50*time.Millisecond)

	statusCodes := []int{500, 200}
	callIdx := 0

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if callIdx < len(statusCodes) {
			w.WriteHeader(statusCodes[callIdx])
			callIdx++
		}
	})

	middleware := LoadShed(ls)
	wrappedHandler := middleware(handler)

	// Request 1: 500 (error) - activates shedding
	resp := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	wrappedHandler.ServeHTTP(resp, req)
	if resp.Code != 500 {
		t.Errorf("request 1: expected 500, got %d", resp.Code)
	}
	if ls.ConsecutiveErrors() != 1 {
		t.Errorf("request 1: expected 1 error, got %d", ls.ConsecutiveErrors())
	}

	// Request 2: should be shed (503 short-circuit, no handler call)
	resp = httptest.NewRecorder()
	req = httptest.NewRequest("GET", "/", nil)
	wrappedHandler.ServeHTTP(resp, req)
	if resp.Code != 503 {
		t.Errorf("request 2 (shed): expected 503, got %d", resp.Code)
	}
	// Important: error count should NOT increase (short-circuit doesn't count as error)
	if ls.ConsecutiveErrors() != 1 {
		t.Errorf("request 2 (shed): errors should stay at 1, got %d", ls.ConsecutiveErrors())
	}

	// Wait for backoff to allow probe
	time.Sleep(60 * time.Millisecond)

	// Request 3: probe after backoff (should reach handler, returns 200)
	resp = httptest.NewRecorder()
	req = httptest.NewRequest("GET", "/", nil)
	wrappedHandler.ServeHTTP(resp, req)
	if resp.Code != 200 {
		t.Errorf("request 3 (probe): expected 200, got %d", resp.Code)
	}
	// 200 resets errors
	if ls.ConsecutiveErrors() != 0 {
		t.Errorf("request 3: expected 0 errors after 200, got %d", ls.ConsecutiveErrors())
	}
	if ls.IsShedding() {
		t.Errorf("request 3: expected shedding to be off after probe success")
	}
}

// TestLoadShed_Middleware_health_metrics_allowlist verifies that health and
// metrics paths bypass the breaker and do not contribute to error recording.
func TestLoadShed_Middleware_health_metrics_allowlist(t *testing.T) {
	ls := NewLoadShedder(1, 50*time.Millisecond)

	// Artificially latch the breaker
	ls.RecordError()
	if !ls.IsShedding() {
		t.Fatal("expected breaker to be latched")
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	})

	middleware := LoadShed(ls)
	wrappedHandler := middleware(handler)

	// Test /health/live: should reach handler and respond normally (not 503)
	resp := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/health/live", nil)
	wrappedHandler.ServeHTTP(resp, req)
	if resp.Code != 200 {
		t.Errorf("/health/live: expected 200, got %d", resp.Code)
	}
	// Should not change error count
	if ls.ConsecutiveErrors() != 1 {
		t.Errorf("/health/live: errors should stay at 1, got %d", ls.ConsecutiveErrors())
	}

	// Test /health/ready: should reach handler and respond normally (not 503)
	resp = httptest.NewRecorder()
	req = httptest.NewRequest("GET", "/health/ready", nil)
	wrappedHandler.ServeHTTP(resp, req)
	if resp.Code != 200 {
		t.Errorf("/health/ready: expected 200, got %d", resp.Code)
	}
	// Should not change error count
	if ls.ConsecutiveErrors() != 1 {
		t.Errorf("/health/ready: errors should stay at 1, got %d", ls.ConsecutiveErrors())
	}

	// Test /metrics: should reach handler and respond normally (not 503)
	resp = httptest.NewRecorder()
	req = httptest.NewRequest("GET", "/metrics", nil)
	wrappedHandler.ServeHTTP(resp, req)
	if resp.Code != 200 {
		t.Errorf("/metrics: expected 200, got %d", resp.Code)
	}
	// Should not change error count
	if ls.ConsecutiveErrors() != 1 {
		t.Errorf("/metrics: errors should stay at 1, got %d", ls.ConsecutiveErrors())
	}

	// Test a normal path: should be shed (503)
	resp = httptest.NewRecorder()
	req = httptest.NewRequest("GET", "/v1/suggest", nil)
	wrappedHandler.ServeHTTP(resp, req)
	if resp.Code != 503 {
		t.Errorf("/v1/suggest: expected 503 when breaker is latched, got %d", resp.Code)
	}
}

// TestLoadShed_Middleware_context_abort verifies that client context aborts
// do not contribute to error recording.
func TestLoadShed_Middleware_context_abort(t *testing.T) {
	ls := NewLoadShedder(2, 10*time.Millisecond)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate handler waiting on context and aborting
		<-r.Context().Done()
		w.WriteHeader(500) // Handler writes 500 after abort
	})

	middleware := LoadShed(ls)
	wrappedHandler := middleware(handler)

	// Create request with canceled context
	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest("GET", "/", nil).WithContext(ctx)
	cancel() // Cancel immediately

	resp := httptest.NewRecorder()
	wrappedHandler.ServeHTTP(resp, req)

	// Even though handler wrote 500, the abort context means it shouldn't count
	if ls.ConsecutiveErrors() != 0 {
		t.Errorf("expected 0 errors after aborted context, got %d", ls.ConsecutiveErrors())
	}
}

// TestLoadShed_Middleware_handler_panic verifies that handler panics are
// recorded as errors.
func TestLoadShed_Middleware_handler_panic(t *testing.T) {
	ls := NewLoadShedder(2, 10*time.Millisecond)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("handler panic")
	})

	middleware := LoadShed(ls)
	wrappedHandler := middleware(handler)

	// Wrap the call to catch the panic
	didPanic := false
	func() {
		defer func() {
			if recover() != nil {
				didPanic = true
			}
		}()

		resp := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/", nil)
		wrappedHandler.ServeHTTP(resp, req)
	}()

	if !didPanic {
		t.Error("expected panic to propagate")
	}

	// Panic should have been recorded as an error
	if ls.ConsecutiveErrors() != 1 {
		t.Errorf("expected 1 error after panic, got %d", ls.ConsecutiveErrors())
	}
}

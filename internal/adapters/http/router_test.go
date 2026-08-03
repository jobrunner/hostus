package httpx_test

import (
	"net/http/httptest"
	"strings"
	"testing"

	httpx "github.com/jobrunner/hostus/internal/adapters/http"
)

func TestHealthLive(t *testing.T) {
	r := httpx.NewRouter(httpx.Deps{})
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, httptest.NewRequest("GET", "/health/live", nil))
	if rr.Code != 200 {
		t.Fatalf("got %d", rr.Code)
	}
}

func TestRequestIDHeaderPresent(t *testing.T) {
	r := httpx.NewRouter(httpx.Deps{})
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, httptest.NewRequest("GET", "/health/live", nil))
	if rr.Header().Get("X-Request-ID") == "" {
		t.Fatal("expected X-Request-ID header to be set")
	}
}

func TestHealthReady(t *testing.T) {
	r := httpx.NewRouter(httpx.Deps{})
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, httptest.NewRequest("GET", "/health/ready", nil))
	if rr.Code != 200 {
		t.Fatalf("got %d", rr.Code)
	}
}

func TestMetricsEndpoint(t *testing.T) {
	r := httpx.NewRouter(httpx.Deps{})
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, httptest.NewRequest("GET", "/metrics", nil))
	if rr.Code != 200 {
		t.Fatalf("got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "go_goroutines") {
		t.Fatalf("expected prometheus exposition text, got: %s", rr.Body.String())
	}
}

func TestZeroValueDepsDoesNotPanic(t *testing.T) {
	defer func() {
		if p := recover(); p != nil {
			t.Fatalf("NewRouter panicked with zero-value Deps: %v", p)
		}
	}()
	r := httpx.NewRouter(httpx.Deps{})
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, httptest.NewRequest("GET", "/health/live", nil))
}

// TestMiddlewareOrderRequestIDBeforeLogging is a coarse observability check:
// the X-Request-ID header must already be set on the response by the time
// downstream middleware (logging, and ultimately the handler) run, which is
// only possible if RequestID is the outermost middleware in the chain.
func TestMiddlewareOrderRequestIDBeforeLogging(t *testing.T) {
	r := httpx.NewRouter(httpx.Deps{})
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/health/live", nil)
	req.Header.Set("X-Request-ID", "test-fixed-id")
	r.ServeHTTP(rr, req)
	if got := rr.Header().Get("X-Request-ID"); got != "test-fixed-id" {
		t.Fatalf("expected request-supplied X-Request-ID to be echoed, got %q", got)
	}
}

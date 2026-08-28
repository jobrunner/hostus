package httpx_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	httpx "github.com/jobrunner/hostus/internal/adapters/http"
	"github.com/jobrunner/hostus/internal/adapters/sqlite"
)

// TestRouter_TraitsRouteRemoved pins the removal of the traits subsystem
// (Task 12): GET /v1/concept/{id}/traits must no longer be a registered
// route, so unmatched requests fall through to gorilla/mux's default 404.
func TestRouter_TraitsRouteRemoved(t *testing.T) {
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("sqlite.Open(:memory:): unexpected error: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	router := httpx.NewRouter(httpx.Deps{Repo: db})
	req := httptest.NewRequest(http.MethodGet, "/v1/concept/wcvp:concept:1/traits", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404 (Route soll entfernt sein)", w.Code)
	}
}

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

// TestHealthReady_NoRepo_Returns503 pins the readiness gate's default: a
// zero-value Deps (no injected repository, e.g. serve started without a
// configured/openable SQLite database) must report not-ready, not silently
// claim to be healthy.
func TestHealthReady_NoRepo_Returns503(t *testing.T) {
	r := httpx.NewRouter(httpx.Deps{})
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, httptest.NewRequest("GET", "/health/ready", nil))
	if rr.Code != 503 {
		t.Fatalf("got %d, want 503 (no repo injected)", rr.Code)
	}
}

// TestHealthReady_RepoWithoutBackboneVersions_Returns503 pins the "opened
// but empty" case: a repo is injected (the database opened successfully)
// but no backbone has ever been ingested into it, so there is nothing yet
// worth serving.
func TestHealthReady_RepoWithoutBackboneVersions_Returns503(t *testing.T) {
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("sqlite.Open(:memory:): unexpected error: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	r := httpx.NewRouter(httpx.Deps{Repo: db})
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, httptest.NewRequest("GET", "/health/ready", nil))
	if rr.Code != 503 {
		t.Fatalf("got %d, want 503 (no backbone_version rows yet)", rr.Code)
	}
}

// TestHealthReady_RepoWithBackboneVersions_Returns200 pins the positive
// path: once at least one backbone has been ingested, readiness must flip
// to 200.
func TestHealthReady_RepoWithBackboneVersions_Returns200(t *testing.T) {
	repo := seededRepo(t)

	r := httpx.NewRouter(httpx.Deps{Repo: repo})
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, httptest.NewRequest("GET", "/health/ready", nil))
	if rr.Code != 200 {
		t.Fatalf("got %d, want 200 (body: %s)", rr.Code, rr.Body.String())
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

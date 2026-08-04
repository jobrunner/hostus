package httpx_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	httpx "github.com/jobrunner/hostus/internal/adapters/http"
)

// uiAssetPaths are paths a browser would plausibly request once the SP8
// console exists (Task 2). With the UI switched off none of them may be
// reachable: the router registers nothing at all under the UI, so every
// one of them falls through to mux's 404.
var uiAssetPaths = []string{
	"/",
	"/index.html",
	"/assets/app.js",
	"/assets/app.css",
	"/ui/",
}

// TestUIDisabledRegistersNothing pins the off-path: not just "/" but any
// asset path under it must 404. This is the path an operator relies on and
// the one nobody exercises by hand, so it is pinned first.
func TestUIDisabledRegistersNothing(t *testing.T) {
	r := httpx.NewRouter(httpx.Deps{UIEnabled: false})
	for _, path := range uiAssetPaths {
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, path, nil))
		if rr.Code != http.StatusNotFound {
			t.Errorf("GET %s with UI disabled: got %d, want 404", path, rr.Code)
		}
	}
}

// TestZeroValueDepsDisablesUI pins the wiring default of Deps itself: a
// zero-value Deps (used by tests and by any caller that has not wired the
// configuration yet) must not silently expose the console. "Default on"
// lives in config.Defaults(), not in the router's zero value.
func TestZeroValueDepsDisablesUI(t *testing.T) {
	r := httpx.NewRouter(httpx.Deps{})
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", nil))
	if rr.Code != http.StatusNotFound {
		t.Fatalf("GET / with zero-value Deps: got %d, want 404", rr.Code)
	}
}

// TestUIEnabledServesRoot pins the on-path: "/" serves an HTML document.
func TestUIEnabledServesRoot(t *testing.T) {
	r := httpx.NewRouter(httpx.Deps{UIEnabled: true})
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("GET / with UI enabled: got %d, want 200", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Fatalf("GET /: Content-Type %q, want text/html...", ct)
	}
	if rr.Body.Len() == 0 {
		t.Fatal("GET /: empty body, want an HTML document")
	}
}

// TestUIRouteIsInsideMiddlewareChain pins the deliberate decision that the
// UI route sits INSIDE the fixed middleware chain rather than beside it: a
// UI response must carry the same X-Request-ID contract as every other
// route, so the console is observable (logs, spans, metrics) exactly like
// the API it drives.
func TestUIRouteIsInsideMiddlewareChain(t *testing.T) {
	r := httpx.NewRouter(httpx.Deps{UIEnabled: true})
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", nil))
	if rr.Header().Get("X-Request-ID") == "" {
		t.Fatal("GET /: no X-Request-ID — the UI route bypasses the middleware chain")
	}
}

// apiProbe is one request replayed against a UI-on and a UI-off router.
type apiProbe struct {
	method string
	path   string
	body   string
}

// TestAPISurfaceIdenticalWithAndWithoutUI is the regression guard the brief
// asks for: turning the console on must not change the shape, status or
// body of a single API response. Both routers are built over the same
// seeded repository and every probe is compared byte for byte.
func TestAPISurfaceIdenticalWithAndWithoutUI(t *testing.T) {
	repo := seededRepo(t)

	probes := []apiProbe{
		{http.MethodGet, "/health/live", ""},
		{http.MethodGet, "/health/ready", ""},
		{http.MethodGet, "/v1/concept/" + corynephorusConceptID, ""},
		{http.MethodGet, "/v1/concept/does-not-exist", ""},
		{http.MethodGet, "/v1/concept/" + corynephorusConceptID + "/traits", ""},
		{http.MethodGet, "/v1/concept/" + corynephorusConceptID + "/synonyms", ""},
		{http.MethodGet, "/v1/suggest?q=Coryne&limit=5", ""},
		{http.MethodGet, "/v1/xref?authority=powo&id=nonexistent", ""},
		{http.MethodPost, "/v1/match", `{"names":["Corynephorus canescens"]}`},
		{http.MethodPost, "/v1/translate", `{"concept_ids":["` + corynephorusConceptID + `"],"target":"wcvp"}`},
		{http.MethodGet, "/openapi", ""},
	}

	off := httpx.NewRouter(httpx.Deps{Repo: repo, UIEnabled: false})
	on := httpx.NewRouter(httpx.Deps{Repo: repo, UIEnabled: true})

	for _, p := range probes {
		gotOff := replay(t, off, p)
		gotOn := replay(t, on, p)
		if gotOff.Code != gotOn.Code {
			t.Errorf("%s %s: status %d with UI off vs %d with UI on", p.method, p.path, gotOff.Code, gotOn.Code)
			continue
		}
		if gotOff.Body.String() != gotOn.Body.String() {
			t.Errorf("%s %s: body differs with UI on\n off: %s\n  on: %s", p.method, p.path, gotOff.Body.String(), gotOn.Body.String())
		}
		if gotOff.Header().Get("Content-Type") != gotOn.Header().Get("Content-Type") {
			t.Errorf("%s %s: Content-Type %q with UI off vs %q with UI on",
				p.method, p.path, gotOff.Header().Get("Content-Type"), gotOn.Header().Get("Content-Type"))
		}
	}
}

func replay(t *testing.T, r http.Handler, p apiProbe) *httptest.ResponseRecorder {
	t.Helper()
	var req *http.Request
	if p.body == "" {
		req = httptest.NewRequest(p.method, p.path, nil)
	} else {
		req = httptest.NewRequest(p.method, p.path, strings.NewReader(p.body))
		req.Header.Set("Content-Type", "application/json")
	}
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	return rr
}

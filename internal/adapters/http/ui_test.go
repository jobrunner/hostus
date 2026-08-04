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

// TestUIServesCSPWithoutExternalOrigin pins that the console's security
// header survives the trip through the router: a page whose CSP admitted
// an external origin would quietly stop being an offline-first artifact.
func TestUIServesCSPWithoutExternalOrigin(t *testing.T) {
	r := httpx.NewRouter(httpx.Deps{UIEnabled: true})
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", nil))

	csp := rr.Header().Get("Content-Security-Policy")
	if csp == "" {
		t.Fatal("GET /: no Content-Security-Policy header")
	}
	if !strings.Contains(csp, "default-src 'self'") {
		t.Errorf("CSP lacks default-src 'self': %q", csp)
	}
	for _, forbidden := range []string{"http:", "https:", "*", "'unsafe-inline'", "'unsafe-eval'"} {
		if strings.Contains(csp, forbidden) {
			t.Errorf("CSP contains %q: %q", forbidden, csp)
		}
	}
}

// uiDeepLinks are SPA routes the console itself can produce. None of them
// is a registered route, and each must still land on the page.
var uiDeepLinks = []string{
	"/konzept/wcvp-12345",
	"/suggest",
	"/index.html",
	"/deep/link/with/segments",
}

// TestUIDeepLinkServesPage pins the SPA rule: an unknown path that is not
// under an API prefix serves the console.
func TestUIDeepLinkServesPage(t *testing.T) {
	r := httpx.NewRouter(httpx.Deps{UIEnabled: true})

	root := httptest.NewRecorder()
	r.ServeHTTP(root, httptest.NewRequest(http.MethodGet, "/", nil))

	for _, path := range uiDeepLinks {
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, path, nil))
		if rr.Code != http.StatusOK {
			t.Errorf("GET %s: got %d, want 200", path, rr.Code)
			continue
		}
		if ct := rr.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
			t.Errorf("GET %s: Content-Type %q, want text/html...", path, ct)
		}
		if rr.Body.String() != root.Body.String() {
			t.Errorf("GET %s: served something other than the console document", path)
		}
	}
}

// TestSPAFallbackIsInsideMiddlewareChain: gorilla/mux assigns
// NotFoundHandler WITHOUT running Use()-registered middleware, so the
// fallback has to be wrapped explicitly. If it ever isn't, a deep link
// becomes the one request in the system that produces no request id, no
// log line, no span and no metric — inside the tool built for looking at
// the system.
func TestSPAFallbackIsInsideMiddlewareChain(t *testing.T) {
	r := httpx.NewRouter(httpx.Deps{UIEnabled: true})
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/konzept/abc", nil))
	if rr.Header().Get("X-Request-ID") == "" {
		t.Fatal("GET /konzept/abc: no X-Request-ID — the SPA fallback bypasses the middleware chain")
	}
}

// apiPrefixMisses are unknown paths UNDER an API prefix. The API contract
// must not change shape because a UI exists, so each keeps its 404.
var apiPrefixMisses = []string{
	"/v1",
	"/v1/nope",
	"/v1/concept",
	"/health",
	"/health/nope",
	"/metrics/nope",
	"/openapi",
	"/openapi/nope",
}

func TestUnknownAPIPathStays404WithUI(t *testing.T) {
	off := httpx.NewRouter(httpx.Deps{UIEnabled: false})
	on := httpx.NewRouter(httpx.Deps{UIEnabled: true})

	for _, path := range apiPrefixMisses {
		gotOff := replay(t, off, apiProbe{http.MethodGet, path, ""})
		gotOn := replay(t, on, apiProbe{http.MethodGet, path, ""})

		if gotOn.Code != http.StatusNotFound {
			t.Errorf("GET %s with UI on: got %d, want 404", path, gotOn.Code)
		}
		if gotOff.Body.String() != gotOn.Body.String() {
			t.Errorf("GET %s: 404 body changed with the UI on\n off: %q\n  on: %q", path, gotOff.Body.String(), gotOn.Body.String())
		}
		if gotOff.Header().Get("Content-Type") != gotOn.Header().Get("Content-Type") {
			t.Errorf("GET %s: 404 Content-Type changed with the UI on", path)
		}
	}
}

// TestNonGETUnknownPathIsNot404Page pins that the SPA fallback answers GET
// (and HEAD) only. A POST to an unrouted path must keep the plain 404 it
// has today; serving an HTML page in reply to a POST would be a new,
// unasked-for behavior.
func TestNonGETUnknownPathIsNot404Page(t *testing.T) {
	off := httpx.NewRouter(httpx.Deps{UIEnabled: false})
	on := httpx.NewRouter(httpx.Deps{UIEnabled: true})

	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
		gotOff := replay(t, off, apiProbe{method, "/kein-pfad", ""})
		gotOn := replay(t, on, apiProbe{method, "/kein-pfad", ""})
		if gotOff.Code != gotOn.Code || gotOff.Body.String() != gotOn.Body.String() {
			t.Errorf("%s /kein-pfad: %d/%q with UI off vs %d/%q with UI on",
				method, gotOff.Code, gotOff.Body.String(), gotOn.Code, gotOn.Body.String())
		}
	}
}

// TestMethodMismatchStays405WithUI: a UI fallback that swallowed
// method-mismatch matches would turn every 405 into a 404 (or into the
// console). mux reports ErrMethodMismatch before it consults
// NotFoundHandler; this pins that we did not disturb that.
func TestMethodMismatchStays405WithUI(t *testing.T) {
	off := httpx.NewRouter(httpx.Deps{UIEnabled: false})
	on := httpx.NewRouter(httpx.Deps{UIEnabled: true})

	gotOff := replay(t, off, apiProbe{http.MethodPost, "/health/live", ""})
	gotOn := replay(t, on, apiProbe{http.MethodPost, "/health/live", ""})
	if gotOff.Code != gotOn.Code {
		t.Errorf("POST /health/live: %d with UI off vs %d with UI on", gotOff.Code, gotOn.Code)
	}
}

// TestUIAssetsAreServedWithTheirOwnContentType pins the per-extension
// Content-Type rule for the individually reachable assets.
func TestUIAssetsAreServedWithTheirOwnContentType(t *testing.T) {
	r := httpx.NewRouter(httpx.Deps{UIEnabled: true})
	tests := []struct{ path, want string }{
		{"/assets/app.js", "text/javascript; charset=utf-8"},
		{"/assets/style.css", "text/css; charset=utf-8"},
	}
	for _, tc := range tests {
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, tc.path, nil))
		if rr.Code != http.StatusOK {
			t.Errorf("GET %s: got %d, want 200", tc.path, rr.Code)
			continue
		}
		if ct := rr.Header().Get("Content-Type"); ct != tc.want {
			t.Errorf("GET %s: Content-Type %q, want %q", tc.path, ct, tc.want)
		}
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

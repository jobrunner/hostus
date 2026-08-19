package httpx

import (
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

// These tests are internal (package httpx, not httpx_test) on purpose: the
// console's composition — the document, the CSP and the two raw assets — is
// an implementation detail of this adapter, not part of its API. Exporting
// four symbols only so an external test could see them would widen the
// package's surface for no caller.

// TestUIDocumentIsSelfContained pins the single-response rule: the console
// must ship as ONE document. The rate limiter is a single global bucket
// shared with the API (20 rps), so a page that pulled its CSS and JS as
// separate requests would eat the very budget the tester is trying to
// observe latency in. Both assets must therefore appear inlined.
func TestUIDocumentIsSelfContained(t *testing.T) {
	doc := buildUIDocument("")

	if !strings.Contains(doc, uiStyleCSS) {
		t.Error("document does not contain the stylesheet inline")
	}
	if !strings.Contains(doc, uiAppJS) {
		t.Error("document does not contain the script inline")
	}
	if strings.Contains(doc, "/*hostus:") {
		t.Error("document still contains an unreplaced sentinel")
	}
}

// uiExternalRef matches any absolute or protocol-relative URL in the
// document. The service is offline-first by design (UC1 is field use with
// an offline bundle); a console that reached for a CDN or a webfont would
// contradict the product, and would fail silently only in the field.
var uiExternalRef = regexp.MustCompile(`(?i)(https?:)?//[a-z0-9.-]+\.[a-z]{2,}`)

func TestUIDocumentReferencesNothingExternal(t *testing.T) {
	if m := uiExternalRef.FindString(buildUIDocument("")); m != "" {
		t.Errorf("document references an external origin: %q", m)
	}
}

// uiInlineHandler matches an HTML inline event handler attribute
// (onclick=, onload=, ...). They are forbidden both because
// `default-src 'self'` with a script hash would refuse to run them and
// because they are the classic injection surface.
var uiInlineHandler = regexp.MustCompile(`(?i)<[^>]*\son[a-z]+\s*=`)

func TestUIDocumentHasNoInlineHandlersOrEval(t *testing.T) {
	doc := buildUIDocument("")
	if m := uiInlineHandler.FindString(doc); m != "" {
		t.Errorf("document contains an inline event handler: %q", m)
	}
	for _, forbidden := range []string{"eval(", "new Function(", "javascript:"} {
		if strings.Contains(doc, forbidden) {
			t.Errorf("document contains %q", forbidden)
		}
	}
}

func uiSHA256Source(t *testing.T, s string) string {
	t.Helper()
	sum := sha256.Sum256([]byte(s))
	return "'sha256-" + base64.StdEncoding.EncodeToString(sum[:]) + "'"
}

// TestUICSPForbidsExternalOrigins is the security property and the
// structural guarantee of the offline claim in one header.
func TestUICSPForbidsExternalOrigins(t *testing.T) {
	csp := buildUICSP()

	if !strings.Contains(csp, "default-src 'self'") {
		t.Errorf("CSP lacks default-src 'self': %q", csp)
	}
	for _, forbidden := range []string{"http:", "https:", "*", "'unsafe-inline'", "'unsafe-eval'", "data:script"} {
		if strings.Contains(csp, forbidden) {
			t.Errorf("CSP contains %q, which admits something it must not: %q", forbidden, csp)
		}
	}
}

// TestUICSPHashesMatchTheInlinedAssets pins the pair that makes inlining
// compatible with a hash-based policy: if an asset changes and the policy
// does not, the browser silently stops executing the console.
func TestUICSPHashesMatchTheInlinedAssets(t *testing.T) {
	csp := buildUICSP()

	wantScript := uiSHA256Source(t, uiAppJS)
	if !strings.Contains(csp, "script-src "+wantScript) {
		t.Errorf("CSP script-src does not carry the hash of the inlined script\n got: %q\nwant hash: %s", csp, wantScript)
	}
	wantStyle := uiSHA256Source(t, uiStyleCSS)
	if !strings.Contains(csp, "style-src "+wantStyle) {
		t.Errorf("CSP style-src does not carry the hash of the inlined stylesheet\n got: %q\nwant hash: %s", csp, wantStyle)
	}
}

func TestHandleUIServesTheDocument(t *testing.T) {
	rr := httptest.NewRecorder()
	handleUI("")(rr, httptest.NewRequest(http.MethodGet, "/", nil))

	if rr.Code != http.StatusOK {
		t.Fatalf("GET /: got %d, want 200", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); ct != "text/html; charset=utf-8" {
		t.Errorf("Content-Type %q, want text/html; charset=utf-8", ct)
	}
	if got := rr.Header().Get("Content-Security-Policy"); got != buildUICSP() {
		t.Errorf("Content-Security-Policy %q, want %q", got, buildUICSP())
	}
	if rr.Header().Get("ETag") == "" {
		t.Error("no ETag: a reload would re-transfer the whole document")
	}
	if rr.Body.String() != buildUIDocument("") {
		t.Error("body is not the composed document")
	}
}

// TestHandleUIRevalidatesCheaply pins the ETag contract: a reload costs a
// 304, not another full document.
func TestHandleUIRevalidatesCheaply(t *testing.T) {
	first := httptest.NewRecorder()
	handleUI("")(first, httptest.NewRequest(http.MethodGet, "/", nil))
	etag := first.Header().Get("ETag")

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("If-None-Match", etag)
	second := httptest.NewRecorder()
	handleUI("")(second, req)

	if second.Code != http.StatusNotModified {
		t.Fatalf("conditional GET /: got %d, want 304", second.Code)
	}
	if second.Body.Len() != 0 {
		t.Errorf("304 carried a %d byte body", second.Body.Len())
	}
}

// TestHandleUIAssetContentTypes pins the per-extension Content-Type. The
// assets are also reachable individually (so a tester can read the JS the
// page runs) even though the page itself never fetches them.
func TestHandleUIAssetContentTypes(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"/assets/app.js", "text/javascript; charset=utf-8"},
		{"/assets/style.css", "text/css; charset=utf-8"},
	}
	for _, tc := range tests {
		rr := httptest.NewRecorder()
		handleUIAsset(rr, httptest.NewRequest(http.MethodGet, tc.path, nil))
		if rr.Code != http.StatusOK {
			t.Errorf("GET %s: got %d, want 200", tc.path, rr.Code)
			continue
		}
		if ct := rr.Header().Get("Content-Type"); ct != tc.want {
			t.Errorf("GET %s: Content-Type %q, want %q", tc.path, ct, tc.want)
		}
		if rr.Header().Get("ETag") == "" {
			t.Errorf("GET %s: no ETag", tc.path)
		}
		if rr.Body.Len() == 0 {
			t.Errorf("GET %s: empty body", tc.path)
		}
	}
}

func TestHandleUIAssetRejectsUnknownName(t *testing.T) {
	for _, path := range []string{"/assets/app.css", "/assets/index.html", "/assets/../ui.go", "/assets/"} {
		rr := httptest.NewRecorder()
		handleUIAsset(rr, httptest.NewRequest(http.MethodGet, path, nil))
		if rr.Code != http.StatusNotFound {
			t.Errorf("GET %s: got %d, want 404", path, rr.Code)
		}
	}
}

// TestUIPanelsArePresent pins that the console actually carries the four
// panels it exists for. Without them the page is a shell and the suggest
// rework stays unjudgeable.
func TestUIPanelsArePresent(t *testing.T) {
	doc := buildUIDocument("")
	for _, id := range []string{"panel-suggest", "panel-concept", "panel-match", "panel-translate"} {
		if !strings.Contains(doc, `id="`+id+`"`) {
			t.Errorf("document lacks panel %q", id)
		}
	}
}

// TestUIDoesNotCacheAPIResponses pins the instrument's core promise: what
// the page shows is what the API just answered.
func TestUIDoesNotCacheAPIResponses(t *testing.T) {
	if !strings.Contains(uiAppJS, `cache: "no-store"`) {
		t.Error(`the fetch wrapper does not pass cache: "no-store"`)
	}
	for _, persisted := range []string{"localStorage", "sessionStorage", "caches."} {
		if strings.Contains(uiAppJS, persisted) {
			t.Errorf("the console uses %s; it must observe live behavior only", persisted)
		}
	}
}

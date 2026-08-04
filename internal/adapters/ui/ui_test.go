package ui_test

import (
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/jobrunner/hostus/internal/adapters/ui"
)

// TestDocumentIsSelfContained pins the single-response rule: the console
// must ship as ONE document. The rate limiter is a single global bucket
// shared with the API (20 rps), so a page that pulled its CSS and JS as
// separate requests would eat the very budget the tester is trying to
// observe. Both assets must therefore appear inlined in the document.
func TestDocumentIsSelfContained(t *testing.T) {
	doc := string(ui.Document())

	if !strings.Contains(doc, ui.StyleSource()) {
		t.Error("document does not contain the stylesheet inline")
	}
	if !strings.Contains(doc, ui.ScriptSource()) {
		t.Error("document does not contain the script inline")
	}
	if strings.Contains(doc, "/*hostus:") {
		t.Error("document still contains an unreplaced sentinel")
	}
}

// externalRef matches any absolute or protocol-relative URL in the
// document. The service is offline-first by design (UC1 is field use with
// an offline bundle); a console that reaches for a CDN or a webfont would
// contradict the product, and would fail silently only in the field.
var externalRef = regexp.MustCompile(`(?i)(https?:)?//[a-z0-9.-]+\.[a-z]{2,}`)

func TestDocumentReferencesNothingExternal(t *testing.T) {
	doc := string(ui.Document())
	if m := externalRef.FindString(doc); m != "" {
		t.Errorf("document references an external origin: %q", m)
	}
}

// inlineHandler matches an HTML inline event handler attribute
// (onclick=, onload=, ...). They are forbidden both because
// `default-src 'self'` with a script hash would refuse to run them and
// because they are the classic injection surface.
var inlineHandler = regexp.MustCompile(`(?i)<[^>]*\son[a-z]+\s*=`)

func TestDocumentHasNoInlineHandlersOrEval(t *testing.T) {
	doc := string(ui.Document())
	if m := inlineHandler.FindString(doc); m != "" {
		t.Errorf("document contains an inline event handler: %q", m)
	}
	for _, forbidden := range []string{"eval(", "new Function(", "javascript:"} {
		if strings.Contains(doc, forbidden) {
			t.Errorf("document contains %q", forbidden)
		}
	}
}

func sha256Source(t *testing.T, s string) string {
	t.Helper()
	sum := sha256.Sum256([]byte(s))
	return "'sha256-" + base64.StdEncoding.EncodeToString(sum[:]) + "'"
}

// TestCSPForbidsExternalOrigins is the security property and the
// structural guarantee of the offline claim in one header.
func TestCSPForbidsExternalOrigins(t *testing.T) {
	csp := ui.ContentSecurityPolicy()

	if !strings.Contains(csp, "default-src 'self'") {
		t.Errorf("CSP lacks default-src 'self': %q", csp)
	}
	for _, forbidden := range []string{"http:", "https:", "*", "'unsafe-inline'", "'unsafe-eval'", "data:script"} {
		if strings.Contains(csp, forbidden) {
			t.Errorf("CSP contains %q, which admits something it must not: %q", forbidden, csp)
		}
	}
}

// TestCSPHashesMatchTheInlinedAssets pins the pair that makes inlining
// compatible with a hash-based policy: if an asset changes and the policy
// does not, the browser silently stops executing the console.
func TestCSPHashesMatchTheInlinedAssets(t *testing.T) {
	csp := ui.ContentSecurityPolicy()

	wantScript := sha256Source(t, ui.ScriptSource())
	if !strings.Contains(csp, "script-src "+wantScript) {
		t.Errorf("CSP script-src does not carry the hash of the inlined script\n got: %q\nwant hash: %s", csp, wantScript)
	}
	wantStyle := sha256Source(t, ui.StyleSource())
	if !strings.Contains(csp, "style-src "+wantStyle) {
		t.Errorf("CSP style-src does not carry the hash of the inlined stylesheet\n got: %q\nwant hash: %s", csp, wantStyle)
	}
}

func TestHandlerServesTheDocument(t *testing.T) {
	rr := httptest.NewRecorder()
	ui.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", nil))

	if rr.Code != http.StatusOK {
		t.Fatalf("GET /: got %d, want 200", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); ct != "text/html; charset=utf-8" {
		t.Errorf("Content-Type %q, want text/html; charset=utf-8", ct)
	}
	if got := rr.Header().Get("Content-Security-Policy"); got != ui.ContentSecurityPolicy() {
		t.Errorf("Content-Security-Policy %q, want %q", got, ui.ContentSecurityPolicy())
	}
	if rr.Header().Get("ETag") == "" {
		t.Error("no ETag: a reload would re-transfer the whole document")
	}
	if rr.Body.String() != string(ui.Document()) {
		t.Error("body is not the composed document")
	}
}

// TestHandlerRevalidatesCheaply pins the ETag contract: a reload costs a
// 304, not another full document.
func TestHandlerRevalidatesCheaply(t *testing.T) {
	first := httptest.NewRecorder()
	ui.Handler().ServeHTTP(first, httptest.NewRequest(http.MethodGet, "/", nil))
	etag := first.Header().Get("ETag")

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("If-None-Match", etag)
	second := httptest.NewRecorder()
	ui.Handler().ServeHTTP(second, req)

	if second.Code != http.StatusNotModified {
		t.Fatalf("conditional GET /: got %d, want 304", second.Code)
	}
	if second.Body.Len() != 0 {
		t.Errorf("304 carried a %d byte body", second.Body.Len())
	}
}

// TestAssetHandlerContentTypes pins the per-extension Content-Type. The
// assets are also reachable individually (so a tester can read the JS the
// page runs) even though the page itself never fetches them.
func TestAssetHandlerContentTypes(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"/assets/app.js", "text/javascript; charset=utf-8"},
		{"/assets/style.css", "text/css; charset=utf-8"},
	}
	for _, tc := range tests {
		rr := httptest.NewRecorder()
		ui.AssetHandler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, tc.path, nil))
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

func TestAssetHandlerRejectsUnknownName(t *testing.T) {
	for _, path := range []string{"/assets/app.css", "/assets/index.html", "/assets/../embed.go", "/assets/"} {
		rr := httptest.NewRecorder()
		ui.AssetHandler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, path, nil))
		if rr.Code != http.StatusNotFound {
			t.Errorf("GET %s: got %d, want 404", path, rr.Code)
		}
	}
}

// TestPanelsArePresent pins that the console actually carries the four
// panels it exists for. Without them the page is a shell and the suggest
// rework stays unjudgeable.
func TestPanelsArePresent(t *testing.T) {
	doc := string(ui.Document())
	for _, id := range []string{"panel-suggest", "panel-concept", "panel-match", "panel-translate"} {
		if !strings.Contains(doc, `id="`+id+`"`) {
			t.Errorf("document lacks panel %q", id)
		}
	}
}

// TestConsoleDoesNotCacheAPIResponses pins the instrument's core promise:
// what the page shows is what the API just answered.
func TestConsoleDoesNotCacheAPIResponses(t *testing.T) {
	js := ui.ScriptSource()
	if !strings.Contains(js, `cache: "no-store"`) {
		t.Error(`the fetch wrapper does not pass cache: "no-store"`)
	}
	if strings.Contains(js, "localStorage") || strings.Contains(js, "sessionStorage") || strings.Contains(js, "caches.") {
		t.Error("the console persists API data; it must observe live behavior only")
	}
}

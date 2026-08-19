package httpx_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	httpx "github.com/jobrunner/hostus/internal/adapters/http"
)

// TestHandleUI_FooterShowsVersion: the console reports which hostus build is
// answering. A tester comparing two deployments needs that on the page itself,
// not from a terminal they may not have.
func TestHandleUI_FooterShowsVersion(t *testing.T) {
	r := httpx.NewRouter(httpx.Deps{UIEnabled: true, Version: "v2.3.0-alpha.0"})

	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", nil))

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "v2.3.0-alpha.0") {
		t.Errorf("document does not contain the version; body tail: %s", tail(body))
	}
	if !strings.Contains(body, "<footer") {
		t.Errorf("document has no footer element; body tail: %s", tail(body))
	}
}

// TestHandleUI_VersionIsHTMLEscaped: the version string arrives from build-time
// ldflags, so it is not structurally trusted — it must not be able to open a
// tag in the document.
func TestHandleUI_VersionIsHTMLEscaped(t *testing.T) {
	r := httpx.NewRouter(httpx.Deps{UIEnabled: true, Version: `<script>alert(1)</script>`})

	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", nil))

	body := rr.Body.String()
	if strings.Contains(body, "<script>alert(1)</script>") {
		t.Error("version was injected as raw markup, want it HTML-escaped")
	}
	if !strings.Contains(body, "&lt;script&gt;") {
		t.Errorf("escaped version not found in the document; body tail: %s", tail(body))
	}
}

// TestHandleUI_MissingVersionFallsBackToDev: a zero-value Deps must still
// render a sensible footer rather than an empty gap.
func TestHandleUI_MissingVersionFallsBackToDev(t *testing.T) {
	r := httpx.NewRouter(httpx.Deps{UIEnabled: true})

	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", nil))

	if body := rr.Body.String(); !strings.Contains(body, "dev") {
		t.Errorf("empty Version did not fall back to %q; body tail: %s", "dev", tail(body))
	}
}

// TestHandleUI_VersionChangesTheETag: two builds must not share a cache entry,
// or a browser would keep showing the previous version's footer.
func TestHandleUI_VersionChangesTheETag(t *testing.T) {
	etag := func(version string) string {
		r := httpx.NewRouter(httpx.Deps{UIEnabled: true, Version: version})
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", nil))
		return rr.Header().Get("ETag")
	}
	a, b := etag("v1.0.0"), etag("v2.0.0")
	if a == "" {
		t.Fatal("no ETag on the console document")
	}
	if a == b {
		t.Errorf("ETag %q is identical for two versions; a cached page would keep the old footer", a)
	}
}

func tail(s string) string {
	if len(s) > 400 {
		return s[len(s)-400:]
	}
	return s
}

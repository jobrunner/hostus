package httpx_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	httpx "github.com/jobrunner/hostus/internal/adapters/http"
)

// TestHandleSuggest_KnownEntryBackboneIsAcceptedAndPlumbedThrough: a known
// entry_backbone reaches the application layer and still serves results. The
// fixture holds WCVP concepts only, so this pins the plumbing and the
// accept-path; that the option actually excludes other backbones is proven
// against a two-backbone index in
// internal/adapters/sqlite/suggest_backbone_test.go.
func TestHandleSuggest_KnownEntryBackboneIsAcceptedAndPlumbedThrough(t *testing.T) {
	repo := seededRepo(t)
	r := httpx.NewRouter(httpx.Deps{Repo: repo})

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/suggest?q=coryn&entry_backbone=wcvp", nil)
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rr.Code, rr.Body.String())
	}
	got := decodeJSON[suggestResponse](t, rr.Body)
	if len(got.Results) == 0 {
		t.Fatal("results empty, want the WCVP Corynephorus concepts")
	}
	for _, it := range got.Results {
		if !strings.HasPrefix(it.ConceptID, "wcvp:") {
			t.Errorf("concept %q leaked into an entry_backbone=wcvp result", it.ConceptID)
		}
	}
}

// TestHandleSuggest_UnknownEntryBackboneIsInvalidQuery: an entry_backbone that
// is not ingested is a caller error, reported exactly as POST /v1/match does —
// a silent empty result would look like "no such plant" instead of "no such
// backbone".
func TestHandleSuggest_UnknownEntryBackboneIsInvalidQuery(t *testing.T) {
	repo := seededRepo(t)
	r := httpx.NewRouter(httpx.Deps{Repo: repo})

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/suggest?q=coryn&entry_backbone=bogus", nil)
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (body: %s)", rr.Code, rr.Body.String())
	}
	if body := rr.Body.String(); !strings.Contains(body, "INVALID_QUERY") || !strings.Contains(body, "bogus") {
		t.Errorf("body = %s, want an INVALID_QUERY envelope naming the offending value", body)
	}
}

// TestHandleSuggest_UnknownTargetSpaceIsInvalidQuery: an un-ingested target
// space is a caller error, reported like an unknown entry_backbone. Silence
// would be worse here than elsewhere — the whole point of the parameter is
// "can I carry this concept into that space", so an empty column would read
// as "no, none of these" instead of "that space does not exist here".
func TestHandleSuggest_UnknownTargetSpaceIsInvalidQuery(t *testing.T) {
	repo := seededRepo(t)
	r := httpx.NewRouter(httpx.Deps{Repo: repo})

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/suggest?q=coryn&target_space=bogus", nil)
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (body: %s)", rr.Code, rr.Body.String())
	}
	if body := rr.Body.String(); !strings.Contains(body, "INVALID_QUERY") || !strings.Contains(body, "bogus") {
		t.Errorf("body = %s, want an INVALID_QUERY envelope naming the offending value", body)
	}
}

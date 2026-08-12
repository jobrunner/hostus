package httpx_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	httpx "github.com/jobrunner/hostus/internal/adapters/http"
	"github.com/jobrunner/hostus/internal/domain"
	"github.com/jobrunner/hostus/internal/ports/output"
)

// stubSecRepo is a minimal output.Repository that only implements
// SecReferences (every other method panics via the embedded nil interface if
// called) — a focused double for the GET /v1/sec handler, mirroring the
// failingSynonymsRepo pattern in synonyms_test.go.
type stubSecRepo struct {
	output.Repository
	refs []domain.SecReference
	err  error
}

func (s stubSecRepo) SecReferences(context.Context) ([]domain.SecReference, error) {
	return s.refs, s.err
}

type secListResponse struct {
	SecReferences []struct {
		ID    string `json:"id"`
		Title string `json:"title"`
	} `json:"sec_references"`
}

func TestHandleSec_ListsReferences(t *testing.T) {
	repo := stubSecRepo{refs: []domain.SecReference{
		{ID: "uuid-a", Title: "Flora A 1998"},
		{ID: "uuid-b", Title: "Flora B 2011"},
	}}
	r := httpx.NewRouter(httpx.Deps{Repo: repo})

	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/v1/sec", nil))

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rr.Code, rr.Body.String())
	}
	assertJSONContentType(t, rr)

	got := decodeJSON[secListResponse](t, rr.Body)
	if len(got.SecReferences) != 2 {
		t.Fatalf("got %d sec references, want 2 (body: %s)", len(got.SecReferences), rr.Body.String())
	}
	if got.SecReferences[0].ID != "uuid-a" || got.SecReferences[0].Title != "Flora A 1998" {
		t.Errorf("first = %+v, want {uuid-a, Flora A 1998}", got.SecReferences[0])
	}
	if got.SecReferences[1].ID != "uuid-b" || got.SecReferences[1].Title != "Flora B 2011" {
		t.Errorf("second = %+v, want {uuid-b, Flora B 2011}", got.SecReferences[1])
	}
}

func TestHandleSec_Empty_ReturnsEmptyArrayNotNull(t *testing.T) {
	repo := stubSecRepo{refs: nil}
	r := httpx.NewRouter(httpx.Deps{Repo: repo})

	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/v1/sec", nil))

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rr.Code, rr.Body.String())
	}
	// A consumer parsing the list must never have to distinguish [] from null:
	// an index with no sec spaces is a valid empty list, not a missing field.
	if body := rr.Body.String(); !strings.Contains(body, `"sec_references":[]`) {
		t.Errorf("body = %s, want an empty JSON array, not null", body)
	}
}

func TestHandleSec_RepoError_Returns500(t *testing.T) {
	repo := stubSecRepo{err: errors.New("boom")}
	r := httpx.NewRouter(httpx.Deps{Repo: repo})

	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/v1/sec", nil))

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500 (body: %s)", rr.Code, rr.Body.String())
	}
	// Not just the status: the body must carry the project's INTERNAL_ERROR
	// envelope, so a regression that returned a bare 500 or a wrong code is
	// caught too.
	if body := rr.Body.String(); !strings.Contains(body, "INTERNAL_ERROR") {
		t.Errorf("body = %s, want an INTERNAL_ERROR error envelope", body)
	}
}

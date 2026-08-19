package httpx_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	httpx "github.com/jobrunner/hostus/internal/adapters/http"
)

type backboneDTO struct {
	ID      string `json:"id"`
	Version string `json:"version"`
}

type backboneListResponse struct {
	Backbones []backboneDTO `json:"backbones"`
}

type spaceDTO struct {
	ID      string `json:"id"`
	Version string `json:"version"`
}

type spaceListResponse struct {
	Spaces []spaceDTO `json:"spaces"`
}

// TestHandleBackbones_ListsIngestedBackbones: the console has to offer the
// entry_backbone choices BEFORE the first query, so it cannot read them off a
// result envelope. This is the same role /v1/sec and /v1/areas already play
// for their pickers.
func TestHandleBackbones_ListsIngestedBackbones(t *testing.T) {
	repo := seededRepo(t)
	r := httpx.NewRouter(httpx.Deps{Repo: repo})

	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/v1/backbones", nil))

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rr.Code, rr.Body.String())
	}
	assertJSONContentType(t, rr)

	got := decodeJSON[backboneListResponse](t, rr.Body)
	if len(got.Backbones) == 0 {
		t.Fatal("backbones empty, want the fixture's wcvp backbone")
	}
	found := false
	for _, b := range got.Backbones {
		if b.ID == "wcvp" {
			found = true
			if b.Version == "" {
				t.Error("wcvp carries no version; the console shows it to tell two deployments apart")
			}
		}
	}
	if !found {
		t.Errorf("backbones = %+v, want one with id wcvp", got.Backbones)
	}
}

// TestHandleSpaces_ListsIngestedNameSpaces: the target_space picker must offer
// exactly what is ingested. Hard-coding the list in the frontend would go wrong
// the moment a deployment ingests a different set — which is the normal case,
// since name spaces are optional manifest entries.
func TestHandleSpaces_ListsIngestedNameSpaces(t *testing.T) {
	repo := seededRepo(t)
	r := httpx.NewRouter(httpx.Deps{Repo: repo})

	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/v1/spaces", nil))

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rr.Code, rr.Body.String())
	}
	assertJSONContentType(t, rr)

	// The fixture ingests no name space, so this pins the empty case: a list,
	// never null, or the console's picker would crash instead of showing
	// "no target spaces in this index".
	body := rr.Body.String()
	if !contains(body, `"spaces":[`) {
		t.Errorf("body = %s, want a spaces array", body)
	}
	if got := decodeJSON[spaceListResponse](t, rr.Body); got.Spaces == nil {
		t.Error("spaces is null; the wire contract is a list, possibly empty")
	}
}

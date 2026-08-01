package httpx_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	httpx "github.com/jobrunner/hostus/internal/adapters/http"
)

type suggestItemResponse struct {
	ConceptID    string  `json:"concept_id"`
	Display      string  `json:"display"`
	Canonical    string  `json:"canonical"`
	VernacularDE string  `json:"vernacular_de"`
	Rank         string  `json:"rank"`
	Status       string  `json:"status"`
	InArea       bool    `json:"in_area"`
	Score        float64 `json:"score"`
}

type suggestResponse struct {
	BackboneVersions map[string]string     `json:"backbone_versions"`
	Results          []suggestItemResponse `json:"results"`
}

func findSuggestResult(results []suggestItemResponse, conceptID string) *suggestItemResponse {
	for i := range results {
		if results[i].ConceptID == conceptID {
			return &results[i]
		}
	}
	return nil
}

// TestHandleSuggest_ReturnsResultsWithInArea exercises the happy path
// against the real WCVP fixture: querying a prefix of "Corynephorus" with
// area=AUT (the fixture's only distributed area code for concept 405825 —
// see the taxa_test.go fixture-area note; the fixture has no GER rows)
// should surface the Corynephorus canescens concept with in_area:true.
func TestHandleSuggest_ReturnsResultsWithInArea(t *testing.T) {
	repo := seededRepo(t)
	r := httpx.NewRouter(httpx.Deps{Repo: repo})

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/suggest?q=coryn&area=AUT", nil)
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rr.Code, rr.Body.String())
	}
	assertJSONContentType(t, rr)

	got := decodeJSON[suggestResponse](t, rr.Body)
	if got.BackboneVersions["wcvp"] != "2026-06-15" {
		t.Errorf("backbone_versions[wcvp] = %q, want %q", got.BackboneVersions["wcvp"], "2026-06-15")
	}
	if len(got.Results) == 0 {
		t.Fatal("results = empty, want at least the Corynephorus canescens concept")
	}

	coryn := findSuggestResult(got.Results, corynephorusConceptID)
	if coryn == nil {
		t.Fatalf("results = %+v, want an entry for %q", got.Results, corynephorusConceptID)
	}
	if coryn.Canonical != "Corynephorus canescens" {
		t.Errorf("canonical = %q, want %q", coryn.Canonical, "Corynephorus canescens")
	}
	if coryn.Rank != "SPECIES" {
		t.Errorf("rank = %q, want %q", coryn.Rank, "SPECIES")
	}
	if coryn.Status != "ACCEPTED" {
		t.Errorf("status = %q, want %q", coryn.Status, "ACCEPTED")
	}
	if !coryn.InArea {
		t.Error("in_area = false, want true for area=AUT")
	}
}

// TestHandleSuggest_NoAreaMeansNotInArea documents the InArea=false
// counterpart: with no area filter, every result reports in_area:false
// (Repository.Suggest's "empty Area means no filter, so nothing is 'in'"
// convention — see the Repository.Suggest doc comment).
func TestHandleSuggest_NoAreaMeansNotInArea(t *testing.T) {
	repo := seededRepo(t)
	r := httpx.NewRouter(httpx.Deps{Repo: repo})

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/suggest?q=coryn", nil)
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rr.Code, rr.Body.String())
	}
	got := decodeJSON[suggestResponse](t, rr.Body)
	coryn := findSuggestResult(got.Results, corynephorusConceptID)
	if coryn == nil {
		t.Fatalf("results = %+v, want an entry for %q", got.Results, corynephorusConceptID)
	}
	if coryn.InArea {
		t.Error("in_area = true, want false with no area query parameter")
	}
}

func TestHandleSuggest_MissingOrEmptyQ_Returns400InvalidQuery(t *testing.T) {
	cases := []string{"", "?area=AUT", "?q=", "?q=%20%20"}
	for _, qs := range cases {
		t.Run(qs, func(t *testing.T) {
			repo := seededRepo(t)
			r := httpx.NewRouter(httpx.Deps{Repo: repo})

			rr := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/v1/suggest"+qs, nil)
			r.ServeHTTP(rr, req)

			if rr.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400 (body: %s)", rr.Code, rr.Body.String())
			}
			assertJSONContentType(t, rr)

			got := decodeJSON[errorEnvelope](t, rr.Body)
			if got.Error.Code != "INVALID_QUERY" {
				t.Errorf("error.code = %q, want %q", got.Error.Code, "INVALID_QUERY")
			}
		})
	}
}

// TestHandleSuggest_RankFilter asserts rank=species restricts results to
// only the SPECIES-ranked Corynephorus canescens concept, excluding the
// GENUS-ranked Corynephorus concept the unfiltered query also matches (see
// TestHandleSuggest_ReturnsResultsWithInArea's sibling assertions and the
// wcvp_taxon.csv fixture: both 451295 (Corynephorus, GENUS) and 405825
// (Corynephorus canescens, SPECIES) match the "coryn" FTS prefix).
func TestHandleSuggest_RankFilter(t *testing.T) {
	repo := seededRepo(t)
	r := httpx.NewRouter(httpx.Deps{Repo: repo})

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/suggest?q=coryn&rank=species", nil)
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rr.Code, rr.Body.String())
	}
	got := decodeJSON[suggestResponse](t, rr.Body)
	if len(got.Results) == 0 {
		t.Fatal("results = empty, want at least the Corynephorus canescens concept")
	}
	for _, res := range got.Results {
		if res.Rank != "SPECIES" {
			t.Errorf("result %+v: rank = %q, want %q (rank=species filter)", res, res.Rank, "SPECIES")
		}
	}
	if findSuggestResult(got.Results, corynephorusConceptID) == nil {
		t.Fatalf("results = %+v, want an entry for %q", got.Results, corynephorusConceptID)
	}
}

func TestHandleSuggest_UnknownRank_Returns400InvalidQuery(t *testing.T) {
	repo := seededRepo(t)
	r := httpx.NewRouter(httpx.Deps{Repo: repo})

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/suggest?q=coryn&rank=bogus", nil)
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (body: %s)", rr.Code, rr.Body.String())
	}
	assertJSONContentType(t, rr)
	got := decodeJSON[errorEnvelope](t, rr.Body)
	if got.Error.Code != "INVALID_QUERY" {
		t.Errorf("error.code = %q, want %q", got.Error.Code, "INVALID_QUERY")
	}
	// The message must name the offending token cleanly and must not leak
	// domain.ParseRank's own error string (`domain: unknown taxon rank
	// "foo"`) via naive concatenation — see internal/adapters/http/suggest.go's
	// parseSuggestRanks doc comment.
	if strings.Contains(got.Error.Message, "domain:") {
		t.Errorf("error.message = %q, must not leak the domain error string", got.Error.Message)
	}
	if want := `unknown rank "bogus"`; got.Error.Message != want {
		t.Errorf("error.message = %q, want %q", got.Error.Message, want)
	}
}

// TestHandleSuggest_RankListMixesValidAndUnknown asserts a comma-separated
// rank list is split and each token parsed individually: a valid token
// alongside an unknown one still 400s (the whole list is rejected, not
// just the bad token silently dropped).
func TestHandleSuggest_RankListMixesValidAndUnknown(t *testing.T) {
	repo := seededRepo(t)
	r := httpx.NewRouter(httpx.Deps{Repo: repo})

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/suggest?q=coryn&rank=species,bogus", nil)
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (body: %s)", rr.Code, rr.Body.String())
	}
	got := decodeJSON[errorEnvelope](t, rr.Body)
	if got.Error.Code != "INVALID_QUERY" {
		t.Errorf("error.code = %q, want %q", got.Error.Code, "INVALID_QUERY")
	}
}

// TestHandleSuggest_RankListCommaSeparated asserts rank=genus,species
// admits both the GENUS and SPECIES concepts the "coryn" prefix matches
// (i.e. the comma-split actually produces multiple domain.Rank values, not
// just the first token).
func TestHandleSuggest_RankListCommaSeparated(t *testing.T) {
	repo := seededRepo(t)
	r := httpx.NewRouter(httpx.Deps{Repo: repo})

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/suggest?q=coryn&rank=genus,species", nil)
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rr.Code, rr.Body.String())
	}
	got := decodeJSON[suggestResponse](t, rr.Body)
	seenGenus, seenSpecies := false, false
	for _, res := range got.Results {
		switch res.Rank {
		case "GENUS":
			seenGenus = true
		case "SPECIES":
			seenSpecies = true
		default:
			t.Errorf("result %+v: rank = %q, want GENUS or SPECIES", res, res.Rank)
		}
	}
	if !seenGenus || !seenSpecies {
		t.Errorf("seenGenus=%v seenSpecies=%v, want both true", seenGenus, seenSpecies)
	}
}

// TestHandleSuggest_LimitTruncates asserts limit=1 truncates the "coryn"
// query's (at least two, see TestHandleSuggest_RankFilter) matching
// concepts down to exactly one result.
func TestHandleSuggest_LimitTruncates(t *testing.T) {
	repo := seededRepo(t)
	r := httpx.NewRouter(httpx.Deps{Repo: repo})

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/suggest?q=coryn&limit=1", nil)
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rr.Code, rr.Body.String())
	}
	got := decodeJSON[suggestResponse](t, rr.Body)
	if len(got.Results) != 1 {
		t.Fatalf("len(results) = %d, want 1 (limit=1)", len(got.Results))
	}
}

func TestHandleSuggest_MalformedLimit_Returns400InvalidQuery(t *testing.T) {
	repo := seededRepo(t)
	r := httpx.NewRouter(httpx.Deps{Repo: repo})

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/suggest?q=coryn&limit=abc", nil)
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (body: %s)", rr.Code, rr.Body.String())
	}
	assertJSONContentType(t, rr)
	got := decodeJSON[errorEnvelope](t, rr.Body)
	if got.Error.Code != "INVALID_QUERY" {
		t.Errorf("error.code = %q, want %q", got.Error.Code, "INVALID_QUERY")
	}
}

// TestHandleSuggest_NoMatches asserts a prefix nothing in the fixture
// matches returns 200 with an empty results array (not 404 or an error) —
// "no autosuggest candidates" is a normal, successful outcome.
func TestHandleSuggest_NoMatches(t *testing.T) {
	repo := seededRepo(t)
	r := httpx.NewRouter(httpx.Deps{Repo: repo})

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/suggest?q="+url.QueryEscape("zzznomatch"), nil)
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rr.Code, rr.Body.String())
	}
	got := decodeJSON[suggestResponse](t, rr.Body)
	if len(got.Results) != 0 {
		t.Errorf("len(results) = %d, want 0", len(got.Results))
	}
}

package httpx_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strings"
	"testing"

	httpx "github.com/jobrunner/hostus/internal/adapters/http"
	"github.com/jobrunner/hostus/internal/domain"
	"github.com/jobrunner/hostus/internal/ports/output"
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
	MatchedName  *struct {
		Canonical          string `json:"canonical"`
		Authorship         string `json:"authorship"`
		Role               string `json:"role"`
		NomStatus          string `json:"nom_status"`
		NomStatusJudgement string `json:"nom_status_judgement"`
	} `json:"matched_name"`
}

type suggestResponse struct {
	BackboneVersions map[string]string     `json:"backbone_versions"`
	Results          []suggestItemResponse `json:"results"`
}

// corynephorusGenusConceptID is the GENUS-ranked concept the "coryn" prefix
// matches alongside corynephorusConceptID (the SPECIES) — see the
// wcvp_taxon.csv fixture.
const corynephorusGenusConceptID = "wcvp:concept:451295"

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

// TestHandleSuggest_MatchModeValid asserts every accepted match_mode token
// ("", "name_start", "anywhere") passes through to a normal 200 response —
// none of them is itself rejected as invalid.
func TestHandleSuggest_MatchModeValid(t *testing.T) {
	for _, mode := range []string{"", "name_start", "anywhere"} {
		t.Run("mode="+mode, func(t *testing.T) {
			repo := seededRepo(t)
			r := httpx.NewRouter(httpx.Deps{Repo: repo})

			url := "/v1/suggest?q=coryn"
			if mode != "" {
				url += "&match_mode=" + mode
			}
			rr := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, url, nil)
			r.ServeHTTP(rr, req)

			if rr.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200 (body: %s)", rr.Code, rr.Body.String())
			}
		})
	}
}

// TestHandleSuggest_UnknownMatchMode_Returns400InvalidQuery asserts an
// unrecognized match_mode token 400s rather than silently falling back to
// the default — a caller's typo must not be hidden behind a behavior
// change it didn't ask for.
func TestHandleSuggest_UnknownMatchMode_Returns400InvalidQuery(t *testing.T) {
	repo := seededRepo(t)
	r := httpx.NewRouter(httpx.Deps{Repo: repo})

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/suggest?q=coryn&match_mode=bogus", nil)
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

// TestSuggest_RequireTargetSpaceWithoutTargetSpaceIs400 asserts
// require_target_space=true without a target_space is rejected rather than
// quietly ignored: there is no space to require an entry in, so the request
// asks for a filter that cannot exist. The message must name BOTH parameters
// — the caller's mistake is the combination, not either one alone.
func TestSuggest_RequireTargetSpaceWithoutTargetSpaceIs400(t *testing.T) {
	repo := seededRepo(t)
	r := httpx.NewRouter(httpx.Deps{Repo: repo})

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/suggest?q=coryn&require_target_space=true", nil)
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (body: %s)", rr.Code, rr.Body.String())
	}
	assertJSONContentType(t, rr)
	got := decodeJSON[errorEnvelope](t, rr.Body)
	if got.Error.Code != "INVALID_QUERY" {
		t.Errorf("error.code = %q, want %q", got.Error.Code, "INVALID_QUERY")
	}
	for _, want := range []string{"require_target_space", "target_space"} {
		if !strings.Contains(got.Error.Message, want) {
			t.Errorf("error.message = %q, want it to name %q", got.Error.Message, want)
		}
	}
}

// TestSuggest_UnparsableRequireTargetSpaceIs400 asserts a non-boolean
// require_target_space value 400s instead of silently defaulting to false —
// a silently ineffective filter is exactly the failure class this branch
// exists to remove.
func TestSuggest_UnparsableRequireTargetSpaceIs400(t *testing.T) {
	repo := seededRepo(t)
	r := httpx.NewRouter(httpx.Deps{Repo: repo})

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/suggest?q=coryn&require_target_space=vielleicht", nil)
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (body: %s)", rr.Code, rr.Body.String())
	}
	got := decodeJSON[errorEnvelope](t, rr.Body)
	if got.Error.Code != "INVALID_QUERY" {
		t.Errorf("error.code = %q, want %q", got.Error.Code, "INVALID_QUERY")
	}
	if !strings.Contains(got.Error.Message, "require_target_space") {
		t.Errorf("error.message = %q, want it to name require_target_space", got.Error.Message)
	}
}

// TestSuggest_RequireTargetSpaceWithSpaceIsAccepted asserts the parsed
// parameter reaches the query path (200) when a target_space accompanies it:
// the fixture carries no name-space entries, so the filter legitimately
// empties the result — the point here is that a well-formed combination is
// not rejected.
func TestSuggest_RequireTargetSpaceWithSpaceIsAccepted(t *testing.T) {
	repo := seededRepo(t)
	r := httpx.NewRouter(httpx.Deps{Repo: repo})

	for _, raw := range []string{"false", "0"} {
		t.Run("require_target_space="+raw, func(t *testing.T) {
			rr := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/v1/suggest?q=coryn&require_target_space="+raw, nil)
			r.ServeHTTP(rr, req)

			if rr.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200 (body: %s)", rr.Code, rr.Body.String())
			}
			got := decodeJSON[suggestResponse](t, rr.Body)
			// Both fixture concepts the "coryn" prefix matches, to prove the
			// false/0 flag narrowed nothing at all.
			for _, id := range []string{corynephorusConceptID, corynephorusGenusConceptID} {
				if findSuggestResult(got.Results, id) == nil {
					t.Errorf("results = %+v, want the unfiltered entry for %q", got.Results, id)
				}
			}
		})
	}
}

// TestSuggest_UnknownAreaIs400 pins the behavior change: an area value the
// index knows nothing about used to return 200 with a silently UNFILTERED
// result list (area is a ranking signal, so an unknown code simply never
// matched a distribution row). A typo'd area must not read as "this plant
// occurs nowhere" — it is a bad request, and the message says which value.
func TestSuggest_UnknownAreaIs400(t *testing.T) {
	repo := seededRepo(t)
	r := httpx.NewRouter(httpx.Deps{Repo: repo})

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/suggest?q=coryn&area=QUATSCH", nil)
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (body: %s)", rr.Code, rr.Body.String())
	}
	assertJSONContentType(t, rr)
	got := decodeJSON[errorEnvelope](t, rr.Body)
	if got.Error.Code != "INVALID_QUERY" {
		t.Errorf("error.code = %q, want %q", got.Error.Code, "INVALID_QUERY")
	}
	if !strings.Contains(got.Error.Message, "QUATSCH") {
		t.Errorf("error.message = %q, want it to name the offending value %q", got.Error.Message, "QUATSCH")
	}
	// The wording matters as much as the code: a rejected value may be a real
	// WGSRPD code this index simply holds no data for, so the message says
	// "not available in this index" and must not claim the code is unknown —
	// a reader who only sees the first clause (log line, toast, truncated
	// message) would go hunting for a typo that isn't there.
	if want := `area "QUATSCH" is not available in this index`; !strings.HasPrefix(got.Error.Message, want) {
		t.Errorf("error.message = %q, want it to start with %q", got.Error.Message, want)
	}
	if strings.Contains(got.Error.Message, "unknown") {
		t.Errorf("error.message = %q, must not claim the area is unknown — it may exist and just carry no data here", got.Error.Message)
	}
	if !strings.Contains(got.Error.Message, "/v1/areas") {
		t.Errorf("error.message = %q, want it to point at GET /v1/areas", got.Error.Message)
	}
}

// TestSuggest_KnownAreaAliasStillWorks guards the console against the
// validation added above: a documented convenience alias ("DE") and a raw
// WGSRPD L3 code that carries data ("AUT") both stay valid. "DE" is the
// harder half — the fixture has no GER distribution row at all, and an alias
// is part of the published API surface, so it must be accepted on the
// strength of the alias table rather than on the strength of the ingested
// data.
func TestSuggest_KnownAreaAliasStillWorks(t *testing.T) {
	repo := seededRepo(t)
	r := httpx.NewRouter(httpx.Deps{Repo: repo})

	for _, area := range []string{"DE", "de", "AUT"} {
		t.Run("area="+area, func(t *testing.T) {
			rr := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/v1/suggest?q=coryn&area="+area, nil)
			r.ServeHTTP(rr, req)

			if rr.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200 (body: %s)", rr.Code, rr.Body.String())
			}
			got := decodeJSON[suggestResponse](t, rr.Body)
			if findSuggestResult(got.Results, corynephorusConceptID) == nil {
				t.Errorf("results = %+v, want an entry for %q", got.Results, corynephorusConceptID)
			}
		})
	}
}

// TestSuggest_MatchedNameIsRendered asserts the name that actually triggered
// the hit reaches the wire, authorship included — the half that tells two
// homonymous spellings apart when the concept's own accepted name
// (canonical/display) cannot.
func TestSuggest_MatchedNameIsRendered(t *testing.T) {
	repo := seededRepo(t)
	r := httpx.NewRouter(httpx.Deps{Repo: repo})

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/suggest?q="+url.QueryEscape("Corynephorus canescens"), nil)
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rr.Code, rr.Body.String())
	}
	got := decodeJSON[suggestResponse](t, rr.Body)
	coryn := findSuggestResult(got.Results, corynephorusConceptID)
	if coryn == nil {
		t.Fatalf("results = %+v, want an entry for %q", got.Results, corynephorusConceptID)
	}
	if coryn.MatchedName == nil {
		t.Fatal("matched_name is absent, want the triggering name")
	}
	if coryn.MatchedName.Canonical != "Corynephorus canescens" {
		t.Errorf("matched_name.canonical = %q, want %q", coryn.MatchedName.Canonical, "Corynephorus canescens")
	}
	if coryn.MatchedName.Authorship != "(L.) P.Beauv." {
		t.Errorf("matched_name.authorship = %q, want %q", coryn.MatchedName.Authorship, "(L.) P.Beauv.")
	}
	if coryn.MatchedName.Role != "accepted" {
		t.Errorf("matched_name.role = %q, want %q", coryn.MatchedName.Role, "accepted")
	}
}

// stubSuggestRepo is a minimal output.Repository that hands handleSuggest a
// fixed candidate list, so a test can pin the wire rendering of a
// domain.MatchedName the WCVP sample fixture does not contain (a later
// illegitimate homonym) without seeding a second corpus. It embeds a nil
// output.Repository: any method beyond the two application.Suggest needs
// panics rather than silently answering.
type stubSuggestRepo struct {
	output.Repository
	items []domain.SuggestItem
}

func (s stubSuggestRepo) Suggest(context.Context, string, output.SuggestOpts) ([]domain.SuggestItem, error) {
	return s.items, nil
}

func (s stubSuggestRepo) BackboneVersions(context.Context) ([]domain.BackboneVersion, error) {
	return []domain.BackboneVersion{{ID: "wcvp", Version: "2026-06-15"}}, nil
}

// suggestRawMatchedName returns results[0].matched_name as a raw key set, so
// a test can assert a key is ABSENT — which a decode into a struct cannot
// distinguish from an empty string.
func suggestRawMatchedName(t *testing.T, body []byte) (map[string]json.RawMessage, bool) {
	t.Helper()
	var decoded struct {
		Results []map[string]json.RawMessage `json:"results"`
	}
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("decoding response: %v (body: %s)", err, body)
	}
	if len(decoded.Results) == 0 {
		t.Fatalf("results = empty, want one candidate (body: %s)", body)
	}
	raw, ok := decoded.Results[0]["matched_name"]
	if !ok {
		return nil, false
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatalf("decoding matched_name: %v (body: %s)", err, body)
	}
	return fields, true
}

// TestSuggest_MatchedNameRendersNomStatusJudgement asserts the verdict is on
// the wire even for a name the source recorded nothing about: the judgement
// then reads `absent` and nom_status is omitted. The pair is the point —
// "nothing was recorded" must be readable as such, not as "checked and found
// clean" (see matchedNameDTO, which inherits synonymDetailDTO's rationale).
func TestSuggest_MatchedNameRendersNomStatusJudgement(t *testing.T) {
	repo := seededRepo(t)
	r := httpx.NewRouter(httpx.Deps{Repo: repo})

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/suggest?q="+url.QueryEscape("Corynephorus canescens"), nil)
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rr.Code, rr.Body.String())
	}
	raw := append([]byte(nil), rr.Body.Bytes()...)
	got := decodeJSON[suggestResponse](t, rr.Body)
	coryn := findSuggestResult(got.Results, corynephorusConceptID)
	if coryn == nil {
		t.Fatalf("results = %+v, want an entry for %q", got.Results, corynephorusConceptID)
	}
	if coryn.MatchedName == nil {
		t.Fatal("matched_name is absent, want the triggering name")
	}
	if coryn.MatchedName.NomStatusJudgement != "absent" {
		t.Errorf("matched_name.nom_status_judgement = %q, want %q",
			coryn.MatchedName.NomStatusJudgement, "absent")
	}

	fields, ok := suggestRawMatchedName(t, raw)
	if !ok {
		t.Fatal("matched_name is absent from the raw response, want the object")
	}
	if _, present := fields["nom_status"]; present {
		t.Errorf("matched_name.nom_status is present (%s), want it omitted when the source recorded nothing",
			fields["nom_status"])
	}
	if _, present := fields["nom_status_judgement"]; !present {
		t.Error("matched_name.nom_status_judgement is absent, want it rendered always")
	}
}

// TestSuggest_DisqualifiedMatchedNameRendersBothFields is the reported case:
// "Inula hirta Pollich" is a later illegitimate homonym of Inula hirta L.
// The response must say so — the raw status AND the verdict derived from it,
// since the ranking demotes the row on exactly that ground and a demotion
// without a visible reason is indistinguishable from a broken sort.
func TestSuggest_DisqualifiedMatchedNameRendersBothFields(t *testing.T) {
	repo := stubSuggestRepo{items: []domain.SuggestItem{{
		ConceptID: "wcvp:concept:1",
		Display:   "Pentanema britannicum (L.) D.Gut.Larr.",
		Canonical: "Pentanema britannicum",
		Rank:      domain.RankSpecies,
		Status:    domain.StatusAccepted,
		MatchedName: domain.MatchedName{
			Canonical:          "Inula hirta",
			Authorship:         "Pollich",
			Role:               "synonym",
			NomStatus:          "nom. illeg. homonym. post.",
			NomStatusJudgement: domain.JudgementDisqualifying,
		},
		MatchedNameDisqualified: true,
	}}}
	r := httpx.NewRouter(httpx.Deps{Repo: repo})

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/suggest?q="+url.QueryEscape("Inula hirta"), nil)
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rr.Code, rr.Body.String())
	}
	got := decodeJSON[suggestResponse](t, rr.Body)
	if len(got.Results) != 1 {
		t.Fatalf("results = %+v, want exactly one candidate", got.Results)
	}
	matched := got.Results[0].MatchedName
	if matched == nil {
		t.Fatal("matched_name is absent, want the triggering name")
	}
	if matched.NomStatus != "nom. illeg. homonym. post." {
		t.Errorf("matched_name.nom_status = %q, want %q", matched.NomStatus, "nom. illeg. homonym. post.")
	}
	if matched.NomStatusJudgement != "disqualifying" {
		t.Errorf("matched_name.nom_status_judgement = %q, want %q", matched.NomStatusJudgement, "disqualifying")
	}
}

// TestSuggest_NoMatchedNameOmitsTheWholeObject pins the decision left open by
// the adapter task: where no name of the concept could be identified as the
// trigger (match_mode=anywhere), domain.MatchedName stays zero — including an
// EMPTY judgement, which is not domain.JudgementAbsent but "not computed".
// Rather than invent a verdict about a name nobody named, the whole object is
// omitted: a judgement needs something to judge.
func TestSuggest_NoMatchedNameOmitsTheWholeObject(t *testing.T) {
	repo := stubSuggestRepo{items: []domain.SuggestItem{{
		ConceptID: "wcvp:concept:1",
		Display:   "Pentanema hirtum (L.) D.Gut.Larr.",
		Canonical: "Pentanema hirtum",
		Rank:      domain.RankSpecies,
		Status:    domain.StatusAccepted,
	}}}
	r := httpx.NewRouter(httpx.Deps{Repo: repo})

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/suggest?q=hirt&match_mode=anywhere", nil)
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rr.Code, rr.Body.String())
	}
	if fields, ok := suggestRawMatchedName(t, rr.Body.Bytes()); ok {
		t.Errorf("matched_name is present (%v), want the whole object omitted when no triggering name was determined", fields)
	}
}

// TestSuggest_PresentMatchedNameNeverRendersAnEmptyJudgement is the other
// half of that decision: once there IS a name, the wire must carry a verdict
// about it, never `"nom_status_judgement": ""`. A named name with nothing
// recorded IS domain.JudgementAbsent, so the empty verdict normalizes there.
func TestSuggest_PresentMatchedNameNeverRendersAnEmptyJudgement(t *testing.T) {
	repo := stubSuggestRepo{items: []domain.SuggestItem{{
		ConceptID: "wcvp:concept:1",
		Display:   "Pentanema hirtum (L.) D.Gut.Larr.",
		Canonical: "Pentanema hirtum",
		Rank:      domain.RankSpecies,
		Status:    domain.StatusAccepted,
		MatchedName: domain.MatchedName{
			Canonical:  "Inula hirta",
			Authorship: "L.",
			Role:       "synonym",
		},
	}}}
	r := httpx.NewRouter(httpx.Deps{Repo: repo})

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/suggest?q="+url.QueryEscape("Inula hirta"), nil)
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rr.Code, rr.Body.String())
	}
	got := decodeJSON[suggestResponse](t, rr.Body)
	if len(got.Results) != 1 {
		t.Fatalf("results = %+v, want exactly one candidate", got.Results)
	}
	matched := got.Results[0].MatchedName
	if matched == nil {
		t.Fatal("matched_name is absent, want the triggering name")
	}
	if matched.NomStatusJudgement != "absent" {
		t.Errorf("matched_name.nom_status_judgement = %q, want %q", matched.NomStatusJudgement, "absent")
	}
}

// TestSuggest_ResponseUnchangedWithoutNewParams asserts this task added
// exactly ONE field to the wire shape: a plain query's result objects carry
// the SP1/SP2 keys plus (optionally) matched_name, and nothing else — the new
// ranking signals (exact_hit/target_space_hit) stay internal to
// domain.RankSuggestions, as PrefixHit already does.
func TestSuggest_ResponseUnchangedWithoutNewParams(t *testing.T) {
	repo := seededRepo(t)
	r := httpx.NewRouter(httpx.Deps{Repo: repo})

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/suggest?q=coryn&area=AUT", nil)
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rr.Code, rr.Body.String())
	}

	var body struct {
		Results []map[string]json.RawMessage `json:"results"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("decoding response: %v (body: %s)", err, rr.Body.String())
	}
	if len(body.Results) == 0 {
		t.Fatal("results = empty, want at least one candidate")
	}

	known := map[string]bool{
		"concept_id": true, "display": true, "canonical": true,
		"vernacular_de": true, "rank": true, "status": true,
		"in_area": true, "score": true, "aggregate": true,
		"sec": true, "target_space_name": true, "matched_name": true,
	}
	for i, item := range body.Results {
		var unexpected []string
		for key := range item {
			if !known[key] {
				unexpected = append(unexpected, key)
			}
		}
		sort.Strings(unexpected)
		if len(unexpected) != 0 {
			t.Errorf("results[%d]: unexpected keys %v — the wire shape must be unchanged apart from matched_name", i, unexpected)
		}
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

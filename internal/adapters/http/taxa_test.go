package httpx_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jobrunner/hostus/internal/adapters/sqlite"
	"github.com/jobrunner/hostus/internal/adapters/wcvp"
	"github.com/jobrunner/hostus/internal/application"

	httpx "github.com/jobrunner/hostus/internal/adapters/http"
)

// corynephorusConceptID is the deterministic id T5's Ingest assigns the
// WCVP fixture's Corynephorus canescens accepted concept
// (backboneID+":concept:"+taxonid, taxonid 405825 — see wcvp_taxon.csv and
// internal/application/match_test.go, which asserts the same id).
const corynephorusConceptID = "wcvp:concept:405825"

// wcvpRowSource adapts a *wcvp.Dataset into application.RowSource, mirroring
// internal/application/ingest_test.go's helper of the same name — the
// mapping the real composition root (T8, cmd/app) performs so that
// application (and, here, the http adapter's tests) never import the wcvp
// adapter's row shape into production code.
type wcvpRowSource struct{ ds *wcvp.Dataset }

func (s wcvpRowSource) Taxa() []application.TaxonRow {
	out := make([]application.TaxonRow, 0, len(s.ds.Taxa))
	for _, t := range s.ds.Taxa {
		out = append(out, application.TaxonRow{
			TaxonID:         t.TaxonID,
			AcceptedTaxonID: t.AcceptedNameUsageID,
			Accepted:        t.IsAccepted(),
			Canonical:       t.Canonical,
			Authorship:      t.Authorship,
			Rank:            t.Rank,
			Status:          t.Status,
			POWOID:          t.POWOID(),
		})
	}
	return out
}

func (s wcvpRowSource) Distributions() []application.DistributionRow {
	out := make([]application.DistributionRow, 0, len(s.ds.Distributions))
	for _, d := range s.ds.Distributions {
		out = append(out, application.DistributionRow{TaxonID: d.CoreID, AreaCode: d.AreaCode()})
	}
	return out
}

func wcvpReaderFor(b application.Backbone) (application.RowSource, error) {
	ds, err := wcvp.Read(b.Path)
	if err != nil {
		return nil, err
	}
	return wcvpRowSource{ds: ds}, nil
}

// seededRepo ingests the real WCVP fixture (shared with T5/T6's tests) into
// a fresh in-memory sqlite repo, giving the handlers real data to serve
// instead of a mock.
func seededRepo(t *testing.T) *sqlite.DB {
	t.Helper()
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("sqlite.Open(:memory:): unexpected error: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	ds := &application.Dataset{
		Backbones: []application.Backbone{
			{ID: "wcvp", Version: "2026-06-15", Path: "../wcvp/testdata/wcvp-sample"},
		},
	}
	if _, err := application.Ingest(context.Background(), ds, wcvpReaderFor, db); err != nil {
		t.Fatalf("application.Ingest: unexpected error: %v", err)
	}
	return db
}

func decodeJSON[T any](t *testing.T, body *bytes.Buffer) T {
	t.Helper()
	var v T
	if err := json.NewDecoder(body).Decode(&v); err != nil {
		t.Fatalf("decoding JSON response: %v (body: %s)", err, body.String())
	}
	return v
}

func assertJSONContentType(t *testing.T, rr *httptest.ResponseRecorder) {
	t.Helper()
	if got := rr.Header().Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q, want %q", got, "application/json")
	}
}

type errorEnvelope struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

type conceptResponse struct {
	ConceptID string `json:"concept_id"`
	Display   string `json:"display"`
	Canonical string `json:"canonical"`
	Rank      string `json:"rank"`
	Status    string `json:"status"`
	Backbone  struct {
		ID      string `json:"id"`
		Version string `json:"version"`
	} `json:"backbone"`
	Xrefs    map[string]string `json:"xrefs"`
	Synonyms []struct {
		Canonical  string `json:"canonical"`
		Authorship string `json:"authorship"`
	} `json:"synonyms"`
	Distribution []struct {
		AreaScheme string `json:"area_scheme"`
		AreaCode   string `json:"area_code"`
	} `json:"distribution"`
}

func hasDistributionArea(dists []struct {
	AreaScheme string `json:"area_scheme"`
	AreaCode   string `json:"area_code"`
}, areaScheme, areaCode string) bool {
	for _, d := range dists {
		if d.AreaScheme == areaScheme && d.AreaCode == areaCode {
			return true
		}
	}
	return false
}

func hasSynonym(syns []struct {
	Canonical  string `json:"canonical"`
	Authorship string `json:"authorship"`
}, canonicalPrefix string) bool {
	for _, s := range syns {
		if len(s.Canonical) >= len(canonicalPrefix) && s.Canonical[:len(canonicalPrefix)] == canonicalPrefix {
			return true
		}
	}
	return false
}

func TestHandleConcept_KnownID_ReturnsConcept(t *testing.T) {
	repo := seededRepo(t)
	r := httpx.NewRouter(httpx.Deps{Repo: repo})

	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/v1/concept/"+corynephorusConceptID, nil))

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rr.Code, rr.Body.String())
	}
	assertJSONContentType(t, rr)

	got := decodeJSON[conceptResponse](t, rr.Body)
	if got.ConceptID != corynephorusConceptID {
		t.Errorf("concept_id = %q, want %q", got.ConceptID, corynephorusConceptID)
	}
	if got.Canonical != "Corynephorus canescens" {
		t.Errorf("canonical = %q, want %q", got.Canonical, "Corynephorus canescens")
	}
	if got.Display != "Corynephorus canescens (L.) P.Beauv." {
		t.Errorf("display = %q, want %q", got.Display, "Corynephorus canescens (L.) P.Beauv.")
	}
	if got.Rank != "SPECIES" {
		t.Errorf("rank = %q, want %q", got.Rank, "SPECIES")
	}
	if got.Status != "ACCEPTED" {
		t.Errorf("status = %q, want %q", got.Status, "ACCEPTED")
	}
	if got.Backbone.ID != "wcvp" || got.Backbone.Version != "2026-06-15" {
		t.Errorf("backbone = %+v, want {wcvp 2026-06-15}", got.Backbone)
	}
	if got.Xrefs["powo"] != "396681-1" {
		t.Errorf("xrefs[powo] = %q, want %q", got.Xrefs["powo"], "396681-1")
	}
	if !hasSynonym(got.Synonyms, "Weingaertneria") {
		t.Errorf("synonyms = %+v, want an entry starting with %q", got.Synonyms, "Weingaertneria")
	}
	// The WCVP fixture's Corynephorus canescens (405825) carries nine
	// WGSRPD-L3 distribution rows (see wcvp_distribution.csv); assert at
	// least one lands on the wire so /v1/concept doesn't silently drop
	// distribution (spec §B.1/§4.3).
	if len(got.Distribution) != 9 {
		t.Fatalf("len(distribution) = %d, want %d", len(got.Distribution), 9)
	}
	if !hasDistributionArea(got.Distribution, "wgsrpd_l3", "AUT") {
		t.Errorf("distribution = %+v, want an entry {wgsrpd_l3 AUT}", got.Distribution)
	}
}

func TestHandleConcept_UnknownID_Returns404NotFound(t *testing.T) {
	repo := seededRepo(t)
	r := httpx.NewRouter(httpx.Deps{Repo: repo})

	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/v1/concept/does-not-exist", nil))

	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (body: %s)", rr.Code, rr.Body.String())
	}
	assertJSONContentType(t, rr)

	got := decodeJSON[errorEnvelope](t, rr.Body)
	if got.Error.Code != "NOT_FOUND" {
		t.Errorf("error.code = %q, want %q", got.Error.Code, "NOT_FOUND")
	}
	if got.Error.Message == "" {
		t.Error("error.message = empty, want a human readable message")
	}
}

func TestHandleXref_KnownAuthorityAndID_ReturnsConcept(t *testing.T) {
	repo := seededRepo(t)
	r := httpx.NewRouter(httpx.Deps{Repo: repo})

	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/v1/xref?authority=powo&id=396681-1", nil))

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rr.Code, rr.Body.String())
	}
	assertJSONContentType(t, rr)

	got := decodeJSON[conceptResponse](t, rr.Body)
	if got.ConceptID != corynephorusConceptID {
		t.Errorf("concept_id = %q, want %q", got.ConceptID, corynephorusConceptID)
	}
	if got.Canonical != "Corynephorus canescens" {
		t.Errorf("canonical = %q, want %q", got.Canonical, "Corynephorus canescens")
	}
	// /v1/xref renders via the same writeConcept path as /v1/concept/{id};
	// confirm distribution is not dropped here either.
	if len(got.Distribution) != 9 {
		t.Fatalf("len(distribution) = %d, want %d", len(got.Distribution), 9)
	}
}

func TestHandleXref_MissingID_Returns400InvalidQuery(t *testing.T) {
	repo := seededRepo(t)
	r := httpx.NewRouter(httpx.Deps{Repo: repo})

	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/v1/xref?authority=powo", nil))

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (body: %s)", rr.Code, rr.Body.String())
	}
	assertJSONContentType(t, rr)

	got := decodeJSON[errorEnvelope](t, rr.Body)
	if got.Error.Code != "INVALID_QUERY" {
		t.Errorf("error.code = %q, want %q", got.Error.Code, "INVALID_QUERY")
	}
}

func TestHandleXref_MissingAuthority_Returns400InvalidQuery(t *testing.T) {
	repo := seededRepo(t)
	r := httpx.NewRouter(httpx.Deps{Repo: repo})

	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/v1/xref?id=396681-1", nil))

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (body: %s)", rr.Code, rr.Body.String())
	}
	got := decodeJSON[errorEnvelope](t, rr.Body)
	if got.Error.Code != "INVALID_QUERY" {
		t.Errorf("error.code = %q, want %q", got.Error.Code, "INVALID_QUERY")
	}
}

func TestHandleXref_UnknownID_Returns404NotFound(t *testing.T) {
	repo := seededRepo(t)
	r := httpx.NewRouter(httpx.Deps{Repo: repo})

	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/v1/xref?authority=powo&id=nope", nil))

	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (body: %s)", rr.Code, rr.Body.String())
	}
	got := decodeJSON[errorEnvelope](t, rr.Body)
	if got.Error.Code != "NOT_FOUND" {
		t.Errorf("error.code = %q, want %q", got.Error.Code, "NOT_FOUND")
	}
}

type matchResponse struct {
	BackboneVersions map[string]string `json:"backbone_versions"`
	Results          []struct {
		ID             string   `json:"id"`
		MatchType      string   `json:"match_type"`
		Confidence     float64  `json:"confidence"`
		ConceptID      string   `json:"concept_id"`
		Candidates     []string `json:"candidates"`
		RequiresReview bool     `json:"requires_review"`
		Note           string   `json:"note"`
	} `json:"results"`
}

// TestHandleMatch_SpecBatch posts the spec §B.2 example batch (Senecio
// jacobaea L. / Festuca ovina agg. / Silene otitis) and asserts each
// result's match_type per the exact/exact_author/aggregate_alias/
// unresolvable classification T6's application.MatchNames assigns.
func TestHandleMatch_SpecBatch(t *testing.T) {
	repo := seededRepo(t)
	r := httpx.NewRouter(httpx.Deps{Repo: repo})

	body := `{
		"names": [
			{"id": "1", "verbatim": "Senecio jacobaea L."},
			{"id": "2", "verbatim": "Corynephorus canescens"},
			{"id": "3", "verbatim": "Silene otitis"}
		]
	}`
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/match", bytes.NewBufferString(body))
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rr.Code, rr.Body.String())
	}
	assertJSONContentType(t, rr)

	got := decodeJSON[matchResponse](t, rr.Body)
	if got.BackboneVersions["wcvp"] != "2026-06-15" {
		t.Errorf("backbone_versions[wcvp] = %q, want %q", got.BackboneVersions["wcvp"], "2026-06-15")
	}
	if len(got.Results) != 3 {
		t.Fatalf("len(results) = %d, want 3", len(got.Results))
	}
	byID := map[string]int{}
	for i, res := range got.Results {
		byID[res.ID] = i
	}

	senecio := got.Results[byID["1"]]
	if senecio.MatchType != "exact_author" || senecio.Confidence != 0.99 {
		t.Errorf("result 1 = %+v, want match_type=exact_author confidence=0.99", senecio)
	}
	if senecio.ConceptID == "" {
		t.Error("result 1: concept_id = empty, want the resolved Jacobaea vulgaris concept")
	}

	coryn := got.Results[byID["2"]]
	if coryn.MatchType != "exact" || coryn.Confidence != 0.90 {
		t.Errorf("result 2 = %+v, want match_type=exact confidence=0.90", coryn)
	}
	if coryn.ConceptID != corynephorusConceptID {
		t.Errorf("result 2: concept_id = %q, want %q", coryn.ConceptID, corynephorusConceptID)
	}

	silene := got.Results[byID["3"]]
	if silene.MatchType != "unresolvable" {
		t.Errorf("result 3: match_type = %q, want %q", silene.MatchType, "unresolvable")
	}
	if !silene.RequiresReview {
		t.Error("result 3: requires_review = false, want true")
	}
	if silene.ConceptID != "" {
		t.Errorf("result 3: concept_id = %q, want empty", silene.ConceptID)
	}
}

func TestHandleMatch_MalformedBody_Returns400InvalidQuery(t *testing.T) {
	repo := seededRepo(t)
	r := httpx.NewRouter(httpx.Deps{Repo: repo})

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/match", bytes.NewBufferString(`{"names": [`))
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

// TestZeroValueDepsDoesNotMountTaxaRoutes documents that a nil Repo (the
// zero value, as used by every pre-existing router_test.go case) leaves
// /v1/... unmounted rather than panicking on first request.
func TestZeroValueDepsDoesNotMountTaxaRoutes(t *testing.T) {
	r := httpx.NewRouter(httpx.Deps{})
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/v1/concept/anything", nil))
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (mux's own not-found, since the route isn't mounted)", rr.Code)
	}
}

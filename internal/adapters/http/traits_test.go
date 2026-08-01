package httpx_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jobrunner/hostus/internal/adapters/sqlite"
	"github.com/jobrunner/hostus/internal/adapters/traits"
	"github.com/jobrunner/hostus/internal/application"
	"github.com/jobrunner/hostus/internal/domain"

	httpx "github.com/jobrunner/hostus/internal/adapters/http"
)

// rawFieldPresent decodes body's raw JSON structure (bypassing the typed
// traitValueResponse, whose *T fields cannot distinguish "absent from the
// wire" from "present and null") and reports whether field is a key of the
// values[].{dim==dim} object inside the traits[].{vocab==vocab} entry.
func rawFieldPresent(t *testing.T, body []byte, vocab, dim, field string) bool {
	t.Helper()
	var raw struct {
		Traits []struct {
			Vocab  string                   `json:"vocab"`
			Values []map[string]interface{} `json:"values"`
		} `json:"traits"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		t.Fatalf("unmarshal raw body: %v", err)
	}
	for _, set := range raw.Traits {
		if set.Vocab != vocab {
			continue
		}
		for _, v := range set.Values {
			if v["dim"] == dim {
				_, present := v[field]
				return present
			}
		}
	}
	t.Fatalf("no value found for vocab=%q dim=%q in raw body", vocab, dim)
	return false
}

// traitsRowSource adapts a *traits.Dataset into application.TraitRowSource,
// mirroring internal/application/traits_ingest_test.go's helper of the same
// name — application (and, here, the http adapter's tests) never imports
// internal/adapters/traits' row shape into production code.
type traitsRowSource struct{ ds *traits.Dataset }

func (s traitsRowSource) Rows() []application.TraitRow {
	out := make([]application.TraitRow, 0, len(s.ds.Rows))
	for _, r := range s.ds.Rows {
		out = append(out, application.TraitRow{
			Taxon:        r.Taxon,
			Vocab:        r.Vocab,
			VocabVersion: r.VocabVersion,
			Dim:          r.Dim,
			Value:        r.Value,
			NicheWidth:   r.NicheWidth,
			NSystems:     r.NSystems,
		})
	}
	return out
}

var traitsEIVEMeta = domain.TraitVocabMeta{
	Vocab:     domain.VocabEIVE,
	Version:   "1.0",
	Taxonomy:  "euromed-aligned",
	License:   "CC-BY-4.0",
	SourceURL: "https://example.org/eive",
}

var traitsTichyMeta = domain.TraitVocabMeta{
	Vocab:     domain.VocabTichy,
	Version:   "2.0",
	Taxonomy:  "floraveg-eunis-aligned",
	License:   "CC-BY-4.0",
	SourceURL: "https://example.org/tichy",
}

// seededTraitsRepo ingests the WCVP fixture (via seededRepo) plus the EIVE
// and Tichý trait fixtures, giving the handler real, dual-vocabulary trait
// data for corynephorusConceptID to serve — the same real-ingest-path
// fixture setup internal/application/traits_ingest_test.go uses.
func seededTraitsRepo(t *testing.T) *sqlite.DB {
	t.Helper()
	db := seededRepo(t)
	ctx := context.Background()

	eiveDS, err := traits.Read("../traits/testdata/eive-sample.csv")
	if err != nil {
		t.Fatalf("traits.Read(eive-sample.csv): unexpected error: %v", err)
	}
	if _, err := application.IngestTraits(ctx, db, traitsRowSource{ds: eiveDS}, traitsEIVEMeta); err != nil {
		t.Fatalf("IngestTraits(eive): unexpected error: %v", err)
	}

	tichyDS, err := traits.Read("../traits/testdata/tichy-sample.csv")
	if err != nil {
		t.Fatalf("traits.Read(tichy-sample.csv): unexpected error: %v", err)
	}
	if _, err := application.IngestTraits(ctx, db, traitsRowSource{ds: tichyDS}, traitsTichyMeta); err != nil {
		t.Fatalf("IngestTraits(tichy): unexpected error: %v", err)
	}

	return db
}

type traitScaleResponse struct {
	Min        float64 `json:"min"`
	Max        float64 `json:"max"`
	Normalized bool    `json:"normalized"`
}

type traitValueResponse struct {
	Dim        string             `json:"dim"`
	Value      float64            `json:"value"`
	NicheWidth *float64           `json:"niche_width"`
	NSystems   *int               `json:"n_systems"`
	Resolution string             `json:"resolution"`
	Scale      traitScaleResponse `json:"scale"`
}

type traitSetResponse struct {
	Vocab        string               `json:"vocab"`
	VocabVersion string               `json:"vocab_version"`
	Taxonomy     string               `json:"taxonomy"`
	Values       []traitValueResponse `json:"values"`
}

type traitsResponse struct {
	ConceptID string             `json:"concept_id"`
	Traits    []traitSetResponse `json:"traits"`
}

func traitSetByVocab(sets []traitSetResponse, vocab string) *traitSetResponse {
	for i := range sets {
		if sets[i].Vocab == vocab {
			return &sets[i]
		}
	}
	return nil
}

func traitValueByDim(values []traitValueResponse, dim string) *traitValueResponse {
	for i := range values {
		if values[i].Dim == dim {
			return &values[i]
		}
	}
	return nil
}

func TestHandleTraits_KnownConcept_ReturnsBothVocabSets(t *testing.T) {
	repo := seededTraitsRepo(t)
	r := httpx.NewRouter(httpx.Deps{Repo: repo})

	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/v1/concept/"+corynephorusConceptID+"/traits", nil))

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rr.Code, rr.Body.String())
	}
	assertJSONContentType(t, rr)

	got := decodeJSON[traitsResponse](t, rr.Body)
	if got.ConceptID != corynephorusConceptID {
		t.Errorf("concept_id = %q, want %q", got.ConceptID, corynephorusConceptID)
	}
	if len(got.Traits) != 2 {
		t.Fatalf("len(traits) = %d, want 2 (EIVE and Tichy, never merged)", len(got.Traits))
	}

	eive := traitSetByVocab(got.Traits, "eive")
	if eive == nil {
		t.Fatal("no eive trait set found")
	}
	if eive.VocabVersion != "1.0" {
		t.Errorf("eive.vocab_version = %q, want %q", eive.VocabVersion, "1.0")
	}
	if eive.Taxonomy != "euromed-aligned" {
		t.Errorf("eive.taxonomy = %q, want %q", eive.Taxonomy, "euromed-aligned")
	}

	tichy := traitSetByVocab(got.Traits, "tichy2023")
	if tichy == nil {
		t.Fatal("no tichy2023 trait set found")
	}
	if tichy.VocabVersion != "2.0" {
		t.Errorf("tichy.vocab_version = %q, want %q", tichy.VocabVersion, "2.0")
	}
	if tichy.Taxonomy != "floraveg-eunis-aligned" {
		t.Errorf("tichy.taxonomy = %q, want %q", tichy.Taxonomy, "floraveg-eunis-aligned")
	}
}

// TestHandleTraits_EIVEValueCarriesNicheWidthTichyOmits pins the nil-vs-0
// wire contract: EIVE populates niche_width/n_systems on every value, Tichý
// populates neither — the response must OMIT those fields for Tichý, not
// render them as zero.
func TestHandleTraits_EIVEValueCarriesNicheWidthTichyOmits(t *testing.T) {
	repo := seededTraitsRepo(t)
	r := httpx.NewRouter(httpx.Deps{Repo: repo})

	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/v1/concept/"+corynephorusConceptID+"/traits", nil))
	rawBody := append([]byte(nil), rr.Body.Bytes()...)
	got := decodeJSON[traitsResponse](t, rr.Body)

	eive := traitSetByVocab(got.Traits, "eive")
	if eive == nil {
		t.Fatal("no eive trait set found")
	}
	m := traitValueByDim(eive.Values, "M")
	if m == nil {
		t.Fatal("no eive M value found")
	}
	if m.NicheWidth == nil {
		t.Error("eive M niche_width = nil, want populated (EIVE always provides niche_width)")
	}
	if m.NSystems == nil {
		t.Error("eive M n_systems = nil, want populated (EIVE always provides n_systems)")
	}

	// Confirm it round-trips through raw JSON too: the field must be
	// absent from the wire, not merely nil after unmarshal (which cannot
	// distinguish "omitted" from "explicit null" on its own).
	if !rawFieldPresent(t, rawBody, "eive", "M", "niche_width") {
		t.Error("eive M: niche_width field missing from raw JSON, want present")
	}

	tichy := traitSetByVocab(got.Traits, "tichy2023")
	if tichy == nil {
		t.Fatal("no tichy2023 trait set found")
	}
	tm := traitValueByDim(tichy.Values, "M")
	if tm == nil {
		t.Fatal("no tichy M value found")
	}
	if tm.NicheWidth != nil {
		t.Errorf("tichy M niche_width = %v, want nil (Tichy never provides niche_width)", *tm.NicheWidth)
	}
	if tm.NSystems != nil {
		t.Errorf("tichy M n_systems = %v, want nil (Tichy never provides n_systems)", *tm.NSystems)
	}
	if rawFieldPresent(t, rawBody, "tichy2023", "M", "niche_width") {
		t.Error("tichy M: niche_width field present in raw JSON, want omitted")
	}
}

// TestHandleTraits_TichyScalesDifferPerDim pins the cross-dim honesty
// requirement: Tichý's T (1-12) and L (1-9) scales genuinely differ, so a
// single per-set scale would misrepresent one of them. Both dimensions
// exist for Corynephorus canescens in the fixture.
func TestHandleTraits_TichyScalesDifferPerDim(t *testing.T) {
	repo := seededTraitsRepo(t)
	r := httpx.NewRouter(httpx.Deps{Repo: repo})

	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/v1/concept/"+corynephorusConceptID+"/traits?vocab=tichy2023", nil))
	got := decodeJSON[traitsResponse](t, rr.Body)

	if len(got.Traits) != 1 {
		t.Fatalf("len(traits) = %d, want 1 (vocab=tichy2023 filter)", len(got.Traits))
	}
	tichy := got.Traits[0]

	l := traitValueByDim(tichy.Values, "L")
	if l == nil {
		t.Fatal("no tichy L value found")
	}
	tv := traitValueByDim(tichy.Values, "T")
	if tv == nil {
		t.Fatal("no tichy T value found")
	}

	if l.Scale.Normalized {
		t.Error("tichy L scale.normalized = true, want false")
	}
	if l.Scale.Min != 1 || l.Scale.Max != 9 {
		t.Errorf("tichy L scale = {min:%v max:%v}, want {min:1 max:9}", l.Scale.Min, l.Scale.Max)
	}
	if tv.Scale.Min != 1 || tv.Scale.Max != 12 {
		t.Errorf("tichy T scale = {min:%v max:%v}, want {min:1 max:12}", tv.Scale.Min, tv.Scale.Max)
	}
	if l.Scale == tv.Scale {
		t.Error("tichy L and T scales are identical, want them to differ (1-9 vs 1-12)")
	}
}

// TestHandleTraits_EIVEScaleIsNormalizedZeroToTen confirms EIVE's uniform,
// normalized scale is rendered per value.
func TestHandleTraits_EIVEScaleIsNormalizedZeroToTen(t *testing.T) {
	repo := seededTraitsRepo(t)
	r := httpx.NewRouter(httpx.Deps{Repo: repo})

	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/v1/concept/"+corynephorusConceptID+"/traits?vocab=eive", nil))
	got := decodeJSON[traitsResponse](t, rr.Body)

	if len(got.Traits) != 1 {
		t.Fatalf("len(traits) = %d, want 1 (vocab=eive filter)", len(got.Traits))
	}
	m := traitValueByDim(got.Traits[0].Values, "M")
	if m == nil {
		t.Fatal("no eive M value found")
	}
	if !m.Scale.Normalized {
		t.Error("eive M scale.normalized = false, want true")
	}
	if m.Scale.Min != 0 || m.Scale.Max != 10 {
		t.Errorf("eive M scale = {min:%v max:%v}, want {min:0 max:10}", m.Scale.Min, m.Scale.Max)
	}
}

func TestHandleTraits_VocabFilter_ReturnsOnlyRequestedVocab(t *testing.T) {
	repo := seededTraitsRepo(t)
	r := httpx.NewRouter(httpx.Deps{Repo: repo})

	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/v1/concept/"+corynephorusConceptID+"/traits?vocab=eive", nil))

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rr.Code, rr.Body.String())
	}
	got := decodeJSON[traitsResponse](t, rr.Body)
	if len(got.Traits) != 1 {
		t.Fatalf("len(traits) = %d, want 1", len(got.Traits))
	}
	if got.Traits[0].Vocab != "eive" {
		t.Errorf("traits[0].vocab = %q, want %q", got.Traits[0].Vocab, "eive")
	}
}

func TestHandleTraits_UnknownVocabToken_Returns400InvalidQuery(t *testing.T) {
	repo := seededTraitsRepo(t)
	r := httpx.NewRouter(httpx.Deps{Repo: repo})

	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/v1/concept/"+corynephorusConceptID+"/traits?vocab=bogus", nil))

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (body: %s)", rr.Code, rr.Body.String())
	}
	assertJSONContentType(t, rr)
	got := decodeJSON[errorEnvelope](t, rr.Body)
	if got.Error.Code != "INVALID_QUERY" {
		t.Errorf("error.code = %q, want %q", got.Error.Code, "INVALID_QUERY")
	}
}

func TestHandleTraits_UnknownConcept_Returns404NotFound(t *testing.T) {
	repo := seededTraitsRepo(t)
	r := httpx.NewRouter(httpx.Deps{Repo: repo})

	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/v1/concept/does-not-exist/traits", nil))

	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (body: %s)", rr.Code, rr.Body.String())
	}
	assertJSONContentType(t, rr)
	got := decodeJSON[errorEnvelope](t, rr.Body)
	if got.Error.Code != "NOT_FOUND" {
		t.Errorf("error.code = %q, want %q", got.Error.Code, "NOT_FOUND")
	}
}

// TestHandleTraits_ConceptWithNoTraits_Returns200EmptyArray uses
// seededRepo (WCVP only, no trait ingest) so the concept exists but has no
// trait_value rows at all — must be 200 + empty array, never 404.
func TestHandleTraits_ConceptWithNoTraits_Returns200EmptyArray(t *testing.T) {
	repo := seededRepo(t)
	r := httpx.NewRouter(httpx.Deps{Repo: repo})

	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/v1/concept/"+corynephorusConceptID+"/traits", nil))

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rr.Code, rr.Body.String())
	}
	assertJSONContentType(t, rr)
	got := decodeJSON[traitsResponse](t, rr.Body)
	if got.Traits == nil {
		t.Error("traits = nil, want a non-nil empty slice")
	}
	if len(got.Traits) != 0 {
		t.Errorf("len(traits) = %d, want 0", len(got.Traits))
	}
}

func TestZeroValueDepsDoesNotMountTraitsRoute(t *testing.T) {
	r := httpx.NewRouter(httpx.Deps{})
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/v1/concept/anything/traits", nil))
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (mux's own not-found, since the route isn't mounted)", rr.Code)
	}
}

// TestBackboneVersionsExcludeTraitVocabularies pins the provenance block of
// both endpoints that carry one. backbone_versions is generated straight
// from Repository.BackboneVersions, so a trait ingest that recorded itself
// there (as "trait:eive") would tell every /v1/suggest and /v1/match client
// that a trait vocabulary is a taxonomic backbone. After a full ingest —
// one real backbone plus two trait vocabularies — the map must contain the
// backbone and nothing else.
func TestBackboneVersionsExcludeTraitVocabularies(t *testing.T) {
	repo := seededTraitsRepo(t)
	r := httpx.NewRouter(httpx.Deps{Repo: repo})

	assertOnlyRealBackbones := func(t *testing.T, endpoint string, versions map[string]string) {
		t.Helper()
		if versions["wcvp"] != "2026-06-15" {
			t.Errorf("%s: backbone_versions[wcvp] = %q, want %q", endpoint, versions["wcvp"], "2026-06-15")
		}
		if len(versions) != 1 {
			t.Errorf("%s: backbone_versions = %v, want exactly the wcvp backbone", endpoint, versions)
		}
		for id := range versions {
			if strings.HasPrefix(id, "trait:") {
				t.Errorf("%s: backbone_versions contains %q; a trait vocabulary is not a backbone", endpoint, id)
			}
		}
	}

	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/v1/suggest?q=Coryne", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("/v1/suggest status = %d, want 200 (body: %s)", rr.Code, rr.Body.String())
	}
	assertOnlyRealBackbones(t, "/v1/suggest", decodeJSON[suggestResponse](t, rr.Body).BackboneVersions)

	rr = httptest.NewRecorder()
	body := `{"names": [{"id": "1", "verbatim": "Corynephorus canescens"}]}`
	r.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/v1/match", bytes.NewBufferString(body)))
	if rr.Code != http.StatusOK {
		t.Fatalf("/v1/match status = %d, want 200 (body: %s)", rr.Code, rr.Body.String())
	}
	assertOnlyRealBackbones(t, "/v1/match", decodeJSON[matchResponse](t, rr.Body).BackboneVersions)
}

// TestHealthReady_TraitVocabularyAloneDoesNotSatisfyReadiness pins the
// readiness gate against the same confusion: /health/ready only counts
// backbone_version rows, so a database holding trait vocabularies but no
// backbone at all — nothing to serve /v1/suggest or /v1/concepts from —
// must stay 503 rather than being waved through by a synthetic trait row.
func TestHealthReady_TraitVocabularyAloneDoesNotSatisfyReadiness(t *testing.T) {
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("sqlite.Open(:memory:): unexpected error: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	ds, err := traits.Read("../traits/testdata/eive-sample.csv")
	if err != nil {
		t.Fatalf("traits.Read(eive-sample.csv): unexpected error: %v", err)
	}
	if _, err := application.IngestTraits(context.Background(), db, traitsRowSource{ds: ds}, traitsEIVEMeta); err != nil {
		t.Fatalf("IngestTraits: unexpected error: %v", err)
	}
	vocabs, err := db.TraitVocabularies(context.Background())
	if err != nil {
		t.Fatalf("TraitVocabularies: unexpected error: %v", err)
	}
	if len(vocabs) != 1 {
		t.Fatalf("len(TraitVocabularies) = %d, want 1 — the fixture must really have been ingested", len(vocabs))
	}

	r := httpx.NewRouter(httpx.Deps{Repo: db})
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/health/ready", nil))
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503 (a trait vocabulary is not a backbone)", rr.Code)
	}
}

// fakeTraitRowSource feeds hand-written rows to IngestTraits so a test can
// exercise a specific crosswalk outcome (here: a normalised, flagged match)
// without needing a fixture CSV that happens to contain the right spelling.
type fakeTraitRowSource struct{ rows []application.TraitRow }

func (s fakeTraitRowSource) Rows() []application.TraitRow { return s.rows }

var traitsMidoloMeta = domain.TraitVocabMeta{
	Vocab:     domain.VocabMidolo,
	Version:   "3",
	Taxonomy:  "floraveg-eunis-aligned",
	License:   "CC-BY-4.0",
	SourceURL: "https://example.org/midolo",
}

// TestHandleTraits_NormalisedValueCarriesResolutionExactOmitsIt is Hardening
// Task 5's "visible in the DATA, not only on stdout" requirement at the wire
// boundary.
//
// Two Midolo rows land on the SAME concept: one whose taxon name matches the
// backbone exactly, and one written as an aggregate ("Corynephorus canescens
// aggr.") that only resolves through the flagged
// aggregate-to-nominate-species fallback. The aggregate's value is a
// collective mean the vocabulary never asserted about the nominate species,
// so it MUST be distinguishable on the wire; the exact one asserts no
// normalisation and must therefore omit the field entirely rather than
// render an invented "exact" — the same nil-vs-0 discipline
// niche_width/n_systems already follow.
func TestHandleTraits_NormalisedValueCarriesResolutionExactOmitsIt(t *testing.T) {
	repo := seededTraitsRepo(t)
	ctx := context.Background()

	src := fakeTraitRowSource{rows: []application.TraitRow{
		{Taxon: "Corynephorus canescens", Vocab: "midolo2023", VocabVersion: "3", Dim: "disturbance_severity", Value: 1.5},
		{Taxon: "Corynephorus canescens aggr.", Vocab: "midolo2023", VocabVersion: "3", Dim: "disturbance_frequency", Value: 2.5},
	}}
	report, err := application.IngestTraits(ctx, repo, src, traitsMidoloMeta)
	if err != nil {
		t.Fatalf("IngestTraits(midolo): unexpected error: %v", err)
	}
	if report.Matched != 2 {
		t.Fatalf("report.Matched = %d, want 2 (both rows must land on the concept)", report.Matched)
	}

	r := httpx.NewRouter(httpx.Deps{Repo: repo})
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/v1/concept/"+corynephorusConceptID+"/traits", nil))
	rawBody := append([]byte(nil), rr.Body.Bytes()...)
	got := decodeJSON[traitsResponse](t, rr.Body)

	midolo := traitSetByVocab(got.Traits, "midolo2023")
	if midolo == nil {
		t.Fatal("no midolo2023 trait set found")
	}

	exact := traitValueByDim(midolo.Values, "disturbance_severity")
	if exact == nil {
		t.Fatal("no midolo disturbance_severity value found")
	}
	if exact.Resolution != "" {
		t.Errorf("exactly-matched value: resolution = %q, want empty", exact.Resolution)
	}
	// Key absence on the RAW JSON: unmarshalling alone cannot tell "omitted"
	// from "present but empty".
	if rawFieldPresent(t, rawBody, "midolo2023", "disturbance_severity", "resolution") {
		t.Error("exactly-matched value: resolution key present in raw JSON, want omitted")
	}

	normalised := traitValueByDim(midolo.Values, "disturbance_frequency")
	if normalised == nil {
		t.Fatal("no midolo disturbance_frequency value found")
	}
	if normalised.Resolution != string(domain.RuleAggregateToNominate) {
		t.Errorf("aggregate-resolved value: resolution = %q, want %q", normalised.Resolution, domain.RuleAggregateToNominate)
	}
	if !rawFieldPresent(t, rawBody, "midolo2023", "disturbance_frequency", "resolution") {
		t.Error("aggregate-resolved value: resolution key missing from raw JSON, want present")
	}
}

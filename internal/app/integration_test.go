//go:build integration

// Package app_test's integration suite drives the REAL composition root
// end to end: `hostus ingest` (app.Ingest) against the WCVP fixture into a
// throwaway SQLite file, then `hostus serve`'s exact router (app.New) in
// front of a real net/http listener (httptest.Server), exercised purely
// over HTTP — no in-process shortcuts, no mocks. It is gated behind the
// `integration` build tag (see `make test-integration`) rather than
// folded into the default `make test`/mutation gate, since it exercises a
// real SQLite file and a real TCP listener and is slower than the unit
// suite it complements (internal/app/readiness_test.go and
// internal/adapters/http/taxa_test.go already cover the same endpoints at
// the in-process ServeHTTP level).
package app_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jobrunner/hostus/internal/adapters/sqlite"
	"github.com/jobrunner/hostus/internal/app"
	"github.com/jobrunner/hostus/internal/application"
	"github.com/jobrunner/hostus/internal/domain"
)

// seedFestucaOvinaAggregate writes a synthetic aggregate/collective-species
// concept ("Festuca ovina agg.") directly through the output.Repository
// port that app.New wired into the router, mirroring
// internal/application/match_test.go's identically named helper. WCVP
// backbones carry no aggregate concepts, so a real ingest run never
// produces one — this is the shape a future aggregate-vocabulary source
// would supply; seeding it here lets the integration test exercise
// MatchAggregateAlias end to end over real HTTP, not just the exact/
// exact_author/unresolvable paths the fixture already covers natively.
func seedFestucaOvinaAggregate(t *testing.T, a *app.App) string {
	t.Helper()
	ctx := context.Background()
	tx, err := a.Repo.BeginIngest(ctx, domain.BackboneVersion{ID: "test-agg", Version: "v1"})
	if err != nil {
		t.Fatalf("BeginIngest: unexpected error: %v", err)
	}
	name := domain.Name{ID: "test-agg:name:festuca-ovina-agg", Canonical: "Festuca ovina agg.", Rank: domain.RankSpecies}
	concept := domain.Concept{ID: "test-agg:concept:festuca-ovina-agg", BackboneID: "test-agg", AcceptedName: name, Rank: domain.RankSpecies, Status: domain.StatusAccepted}
	if err := tx.UpsertName(name); err != nil {
		t.Fatalf("UpsertName: unexpected error: %v", err)
	}
	if err := tx.UpsertConcept(concept); err != nil {
		t.Fatalf("UpsertConcept: unexpected error: %v", err)
	}
	if err := tx.LinkName(concept.ID, name.ID, "accepted", nil); err != nil {
		t.Fatalf("LinkName: unexpected error: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: unexpected error: %v", err)
	}
	return concept.ID
}

type integrationConceptResponse struct {
	ConceptID string              `json:"concept_id"`
	Display   string              `json:"display"`
	Canonical string              `json:"canonical"`
	Rank      string              `json:"rank"`
	Status    string              `json:"status"`
	Xrefs     map[string][]string `json:"xrefs"`
	Synonyms  []struct {
		Canonical  string `json:"canonical"`
		Authorship string `json:"authorship"`
	} `json:"synonyms"`
	Distribution []struct {
		AreaScheme string `json:"area_scheme"`
		AreaCode   string `json:"area_code"`
	} `json:"distribution"`
}

type integrationMatchResponse struct {
	BackboneVersions map[string]string `json:"backbone_versions"`
	Results          []struct {
		ID             string  `json:"id"`
		MatchType      string  `json:"match_type"`
		Confidence     float64 `json:"confidence"`
		ConceptID      string  `json:"concept_id"`
		RequiresReview bool    `json:"requires_review"`
	} `json:"results"`
}

func hasSynonymPrefix(t *testing.T, syns []struct {
	Canonical  string `json:"canonical"`
	Authorship string `json:"authorship"`
}, prefix string) bool {
	t.Helper()
	for _, s := range syns {
		if strings.HasPrefix(s.Canonical, prefix) {
			return true
		}
	}
	return false
}

// TestIntegration_EndToEndIngestServeQuery is the SP1 foundation's
// end-to-end smoke test: ingest the WCVP fixture into a fresh on-disk
// SQLite database via the exact code path `hostus ingest` uses
// (app.Ingest), then serve it through the exact code path `hostus serve`
// uses (app.New + app.Router) behind a real HTTP listener, and drive GET
// /v1/concept/{id}, GET /v1/xref, POST /v1/match, GET /health/ready and GET
// /metrics purely as an HTTP client would. Named with the "Integration"
// prefix (rather than just a build tag) so `make test-integration`'s
// `-run Integration` filter selects it.
func TestIntegration_EndToEndIngestServeQuery(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "hostus.sqlite")
	manifestPath := "testdata/dataset.yaml"

	ctx := context.Background()
	reports, err := app.Ingest(ctx, manifestPath, dbPath)
	if err != nil {
		t.Fatalf("app.Ingest: unexpected error: %v", err)
	}
	if len(reports.Backbone.Backbones) == 0 {
		t.Fatal("app.Ingest: empty report, want at least the wcvp backbone")
	}

	cfg := testConfig()
	cfg.SQLite.Path = dbPath

	a, err := app.New(cfg)
	if err != nil {
		t.Fatalf("app.New: unexpected error: %v", err)
	}
	t.Cleanup(func() { _ = a.Shutdown(context.Background()) })
	if a.Repo == nil {
		t.Fatal("app.New: Repo is nil, want it wired to the just-ingested database")
	}

	aggConceptID := seedFestucaOvinaAggregate(t, a)

	ts := httptest.NewServer(a.Router)
	defer ts.Close()
	client := ts.Client()

	assertHealthReady(t, client, ts.URL)
	assertConceptByID(t, client, ts.URL)
	assertConceptByXref(t, client, ts.URL)
	assertConceptXrefsMultipleAuthorities(t, client, ts.URL)
	assertXrefResolvesInat(t, client, ts.URL)
	assertMatchBatch(t, client, ts.URL, aggConceptID)
	assertSuggest(t, client, ts.URL)
	assertMetricsExposed(t, client, ts.URL)
}

// assertHealthReady confirms /health/ready is 200 once app.Ingest has
// written at least one backbone_version row (internal/app/readiness_test.go
// already pins the pre-ingest 503 case at the in-process level).
func assertHealthReady(t *testing.T, client *http.Client, baseURL string) {
	t.Helper()
	resp, err := client.Get(baseURL + "/health/ready")
	if err != nil {
		t.Fatalf("GET /health/ready: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /health/ready: status = %d, want 200", resp.StatusCode)
	}
}

// assertConceptByID drives GET /v1/concept/{corynephorus-concept-id} over
// real HTTP and checks the canonical name, the powo xref, and the synonym
// set the WCVP fixture actually carries. The fixture's Corynephorus
// canescens synonyms are Weingaertneria canescens var. pallida and three
// Corynephorus-genus infraspecific synonyms (var. montana, f. pallidus,
// subsp. maritimus) — see internal/adapters/wcvp/testdata/wcvp-sample/
// wcvp_taxon.csv. It does not include an "Aira" synonym; asserting one
// would test data the fixture doesn't carry.
func assertConceptByID(t *testing.T, client *http.Client, baseURL string) {
	t.Helper()
	resp, err := client.Get(baseURL + "/v1/concept/" + corynephorusConceptID)
	if err != nil {
		t.Fatalf("GET /v1/concept: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /v1/concept: status = %d, want 200", resp.StatusCode)
	}
	var concept integrationConceptResponse
	if err := json.NewDecoder(resp.Body).Decode(&concept); err != nil {
		t.Fatalf("decoding /v1/concept response: %v", err)
	}
	_ = resp.Body.Close()

	if concept.ConceptID != corynephorusConceptID {
		t.Errorf("concept_id = %q, want %q", concept.ConceptID, corynephorusConceptID)
	}
	if concept.Canonical != "Corynephorus canescens" {
		t.Errorf("canonical = %q, want %q", concept.Canonical, "Corynephorus canescens")
	}
	if got := concept.Xrefs["powo"]; len(got) != 1 || got[0] != "396681-1" {
		t.Errorf("xrefs[powo] = %v, want [396681-1]", got)
	}
	if !hasSynonymPrefix(t, concept.Synonyms, "Weingaertneria") {
		t.Errorf("synonyms = %+v, want an entry starting with %q", concept.Synonyms, "Weingaertneria")
	}
	if len(concept.Synonyms) < 3 {
		t.Errorf("len(synonyms) = %d, want >= 3 (the fixture's four Corynephorus canescens synonyms)", len(concept.Synonyms))
	}
	// The WCVP fixture's Corynephorus canescens carries nine WGSRPD-L3
	// distribution rows (see wcvp_distribution.csv); assert they survive
	// the full ingest -> serve -> HTTP round trip, not just the in-process
	// handler test.
	if len(concept.Distribution) != 9 {
		t.Errorf("len(distribution) = %d, want %d", len(concept.Distribution), 9)
	}
}

// assertConceptByXref drives GET /v1/xref?authority=powo&id=396681-1 and
// checks it resolves to the same concept as assertConceptByID.
func assertConceptByXref(t *testing.T, client *http.Client, baseURL string) {
	t.Helper()
	resp, err := client.Get(baseURL + "/v1/xref?authority=powo&id=396681-1")
	if err != nil {
		t.Fatalf("GET /v1/xref: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /v1/xref: status = %d, want 200", resp.StatusCode)
	}
	var xrefConcept integrationConceptResponse
	if err := json.NewDecoder(resp.Body).Decode(&xrefConcept); err != nil {
		t.Fatalf("decoding /v1/xref response: %v", err)
	}
	_ = resp.Body.Close()
	if xrefConcept.ConceptID != corynephorusConceptID {
		t.Errorf("xref concept_id = %q, want %q", xrefConcept.ConceptID, corynephorusConceptID)
	}
}

// assertConceptXrefsMultipleAuthorities is SP4's end-to-end proof that
// application.IngestXrefs (T2) really runs as part of the real hostus
// ingest CLI path (app.Ingest -> ingestXrefSource), not just at the unit
// level: the manifest (testdata/dataset.yaml) pins a "wikidata" xref_source
// at internal/adapters/xref/testdata/wikidata-sample.csv, whose join_id
// 396681-1 IS the fixture's powo id for Corynephorus canescens (see that
// fixture's README.md). GET /v1/concept/{corynephorusConceptID} must
// therefore come back over real HTTP with every authority that real row
// carries, not just the WCVP-native powo xref assertConceptByID already
// checked.
func assertConceptXrefsMultipleAuthorities(t *testing.T, client *http.Client, baseURL string) {
	t.Helper()
	resp, err := client.Get(baseURL + "/v1/concept/" + corynephorusConceptID)
	if err != nil {
		t.Fatalf("GET /v1/concept: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /v1/concept: status = %d, want 200", resp.StatusCode)
	}
	var concept integrationConceptResponse
	if err := json.NewDecoder(resp.Body).Decode(&concept); err != nil {
		t.Fatalf("decoding /v1/concept response: %v", err)
	}
	_ = resp.Body.Close()

	// The fixture's join_id 396681-1 row carries wikidata, gbif, colxr,
	// floraveg, wfo and inat in addition to the WCVP-native powo xref
	// already asserted by assertConceptByID — that's "multiple authorities"
	// on the SAME concept, exactly what SP4's coverage measurement is about.
	want := map[string]string{
		"wikidata": "Q159953",
		"gbif":     "5290194",
		"colxr":    "YQW8",
		"floraveg": "Corynephorus canescens",
		"wfo":      "wfo-0000860632",
		"inat":     "160927",
	}
	for authority, id := range want {
		got := concept.Xrefs[authority]
		found := false
		for _, v := range got {
			if v == id {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("xrefs[%q] = %v, want it to contain %q", authority, got, id)
		}
	}
	if len(concept.Xrefs) < len(want)+1 { // +1 for the pre-existing powo xref
		t.Errorf("xrefs = %+v, want at least %d authorities (powo + %v)", concept.Xrefs, len(want)+1, want)
	}
}

// assertXrefResolvesInat drives GET /v1/xref?authority=inat&id=160927 (the
// same fixture row assertConceptXrefsMultipleAuthorities checked from the
// concept side) and confirms it resolves back to the SAME concept — the
// reverse-lookup half of SP4's Wikidata-bridge xref ingest, and the concrete
// UC2 shape (spec: iNaturalist taxon id -> hostus concept).
func assertXrefResolvesInat(t *testing.T, client *http.Client, baseURL string) {
	t.Helper()
	resp, err := client.Get(baseURL + "/v1/xref?authority=inat&id=160927")
	if err != nil {
		t.Fatalf("GET /v1/xref: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /v1/xref: status = %d, want 200", resp.StatusCode)
	}
	var concept integrationConceptResponse
	if err := json.NewDecoder(resp.Body).Decode(&concept); err != nil {
		t.Fatalf("decoding /v1/xref response: %v", err)
	}
	_ = resp.Body.Close()
	if concept.ConceptID != corynephorusConceptID {
		t.Errorf("xref(inat, 160927) concept_id = %q, want %q", concept.ConceptID, corynephorusConceptID)
	}
}

// assertMatchBatch posts a spec-§B.2-style batch to POST /v1/match and
// checks each item's classification: exact_author for a WCVP-native
// synonym (Senecio jacobaea L. -> the accepted Jacobaea vulgaris concept),
// aggregate_alias for the seeded Festuca aggregate, and unresolvable for a
// name absent from the index.
func assertMatchBatch(t *testing.T, client *http.Client, baseURL, aggConceptID string) {
	t.Helper()
	const matchBody = `{
		"names": [
			{"id": "1", "verbatim": "Senecio jacobaea L."},
			{"id": "2", "verbatim": "Festuca ovina agg."},
			{"id": "3", "verbatim": "Silene otitis"}
		]
	}`
	resp, err := client.Post(baseURL+"/v1/match", "application/json", bytes.NewBufferString(matchBody))
	if err != nil {
		t.Fatalf("POST /v1/match: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /v1/match: status = %d, want 200", resp.StatusCode)
	}
	var match integrationMatchResponse
	if err := json.NewDecoder(resp.Body).Decode(&match); err != nil {
		t.Fatalf("decoding /v1/match response: %v", err)
	}
	_ = resp.Body.Close()

	if match.BackboneVersions["wcvp"] != "2026-06-15" {
		t.Errorf("backbone_versions[wcvp] = %q, want %q", match.BackboneVersions["wcvp"], "2026-06-15")
	}
	if len(match.Results) != 3 {
		t.Fatalf("len(results) = %d, want 3", len(match.Results))
	}
	byID := make(map[string]int, len(match.Results))
	for i, r := range match.Results {
		byID[r.ID] = i
	}

	senecio := match.Results[byID["1"]]
	if senecio.MatchType != "exact_author" {
		t.Errorf("Senecio jacobaea L.: match_type = %q, want %q", senecio.MatchType, "exact_author")
	}
	if senecio.ConceptID == "" {
		t.Error("Senecio jacobaea L.: concept_id = empty, want the resolved Jacobaea vulgaris concept")
	}

	festuca := match.Results[byID["2"]]
	if festuca.MatchType != "aggregate_alias" {
		t.Errorf("Festuca ovina agg.: match_type = %q, want %q", festuca.MatchType, "aggregate_alias")
	}
	if festuca.ConceptID != aggConceptID {
		t.Errorf("Festuca ovina agg.: concept_id = %q, want %q", festuca.ConceptID, aggConceptID)
	}

	silene := match.Results[byID["3"]]
	if silene.MatchType != "unresolvable" {
		t.Errorf("Silene otitis: match_type = %q, want %q", silene.MatchType, "unresolvable")
	}
	if silene.ConceptID != "" {
		t.Errorf("Silene otitis: concept_id = %q, want empty", silene.ConceptID)
	}
}

// assertMetricsExposed confirms GET /metrics reflects the HTTP calls the
// earlier assertions made, via the same hostus_http_requests_total counter
// internal/middleware/metrics.go registers.
func assertMetricsExposed(t *testing.T, client *http.Client, baseURL string) {
	t.Helper()
	resp, err := client.Get(baseURL + "/metrics")
	if err != nil {
		t.Fatalf("GET /metrics: %v", err)
	}
	metricsBody := new(bytes.Buffer)
	if _, err := metricsBody.ReadFrom(resp.Body); err != nil {
		t.Fatalf("reading /metrics body: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /metrics: status = %d, want 200", resp.StatusCode)
	}
	if !strings.Contains(metricsBody.String(), "hostus_http_requests_total") {
		t.Error("/metrics: want hostus_http_requests_total to be exposed after the calls above")
	}
	// assertSuggest (above) drove GET /v1/suggest at least twice (a 200 and
	// a 400); path is recorded verbatim (r.URL.Path, no route templating —
	// see internal/middleware/metrics.go), so the counter series for the
	// suggest endpoint must be present too, not just the metric family
	// name.
	if !strings.Contains(metricsBody.String(), `path="/v1/suggest"`) {
		t.Error(`/metrics: want a hostus_http_requests_total series with path="/v1/suggest" after assertSuggest's calls`)
	}
}

// integrationSuggestResponse mirrors internal/adapters/http.suggestResponseDTO
// (see suggest.go), trimmed to the fields this test asserts on.
type integrationSuggestResponse struct {
	BackboneVersions map[string]string `json:"backbone_versions"`
	Results          []struct {
		ConceptID string `json:"concept_id"`
		Canonical string `json:"canonical"`
		InArea    bool   `json:"in_area"`
	} `json:"results"`
}

// assertSuggest drives GET /v1/suggest over real HTTP: q=coryn&area=AUT
// must resolve to 200 with the Corynephorus canescens concept ranked in
// (in_area:true) — AUT is the only WGSRPD-L3 area the WCVP fixture
// actually carries for concept 405825 (see corynephorusConceptID's doc
// comment and internal/adapters/http/suggest_test.go's identical
// fixture-area note; the fixture has no GER distribution row). A missing
// `q` must 400.
func assertSuggest(t *testing.T, client *http.Client, baseURL string) {
	t.Helper()

	resp, err := client.Get(baseURL + "/v1/suggest?q=coryn&area=AUT")
	if err != nil {
		t.Fatalf("GET /v1/suggest: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /v1/suggest: status = %d, want 200", resp.StatusCode)
	}
	var suggest integrationSuggestResponse
	if err := json.NewDecoder(resp.Body).Decode(&suggest); err != nil {
		t.Fatalf("decoding /v1/suggest response: %v", err)
	}
	_ = resp.Body.Close()

	if suggest.BackboneVersions["wcvp"] != "2026-06-15" {
		t.Errorf("backbone_versions[wcvp] = %q, want %q", suggest.BackboneVersions["wcvp"], "2026-06-15")
	}
	var coryn *struct {
		ConceptID string `json:"concept_id"`
		Canonical string `json:"canonical"`
		InArea    bool   `json:"in_area"`
	}
	for i := range suggest.Results {
		if suggest.Results[i].ConceptID == corynephorusConceptID {
			coryn = &suggest.Results[i]
			break
		}
	}
	if coryn == nil {
		t.Fatalf("results = %+v, want an entry for %q", suggest.Results, corynephorusConceptID)
	}
	if coryn.Canonical != "Corynephorus canescens" {
		t.Errorf("canonical = %q, want %q", coryn.Canonical, "Corynephorus canescens")
	}
	if !coryn.InArea {
		t.Error("in_area = false, want true for area=AUT (the fixture's only distributed area for this concept)")
	}

	missingQ, err := client.Get(baseURL + "/v1/suggest")
	if err != nil {
		t.Fatalf("GET /v1/suggest (no q): %v", err)
	}
	_ = missingQ.Body.Close()
	if missingQ.StatusCode != http.StatusBadRequest {
		t.Fatalf("GET /v1/suggest (no q): status = %d, want 400", missingQ.StatusCode)
	}
}

// TestIntegration_OfflineBundleServesSuggestOffline proves the SP2 offline
// field-use capability end to end: export a `sqlite.ExportBundle` (the
// exact path `hostus bundle` uses, see internal/app/bundle.go) scoped to
// area=AUT from the just-ingested database into a standalone bundle file,
// then open ONLY that bundle file via sqlite.Open — never touching the
// original database again — and call application.Suggest (the same use
// case GET /v1/suggest's handler calls) directly against it. No HTTP
// server, no upstream, no original database: if this resolves Corynephorus
// canescens, the bundle is genuinely self-contained and field-usable
// without connectivity.
func TestIntegration_OfflineBundleServesSuggestOffline(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "hostus.sqlite")
	bundlePath := filepath.Join(dir, "bundle.sqlite")

	if _, err := app.Ingest(ctx, "testdata/dataset.yaml", dbPath); err != nil {
		t.Fatalf("app.Ingest: unexpected error: %v", err)
	}

	src, err := sqlite.Open(dbPath)
	if err != nil {
		t.Fatalf("sqlite.Open(source): unexpected error: %v", err)
	}

	report, err := sqlite.ExportBundle(ctx, src, bundlePath, sqlite.BundleOpts{Area: "AUT", SnapshotVersion: "v1"})
	if err != nil {
		_ = src.Close()
		t.Fatalf("sqlite.ExportBundle: unexpected error: %v", err)
	}
	if err := src.Close(); err != nil {
		t.Fatalf("closing source db: %v", err)
	}
	if report.Concepts == 0 {
		t.Fatal("sqlite.ExportBundle: report.Concepts = 0, want at least the AUT-scoped Corynephorus canescens concept")
	}

	// Open ONLY the bundle from here on — dbPath is never referenced again,
	// proving the bundle file alone is queryable.
	bundle, err := sqlite.Open(bundlePath)
	if err != nil {
		t.Fatalf("sqlite.Open(bundle): unexpected error: %v", err)
	}
	defer func() { _ = bundle.Close() }()

	resp, err := application.Suggest(ctx, bundle, application.SuggestRequest{Q: "coryn", Area: "AUT"})
	if err != nil {
		t.Fatalf("application.Suggest against bundle: unexpected error: %v", err)
	}
	if resp.BackboneVersions["wcvp"] != "2026-06-15" {
		t.Errorf("bundle backbone_versions[wcvp] = %q, want %q", resp.BackboneVersions["wcvp"], "2026-06-15")
	}

	var coryn *domain.SuggestItem
	for i := range resp.Results {
		if resp.Results[i].ConceptID == corynephorusConceptID {
			coryn = &resp.Results[i]
			break
		}
	}
	if coryn == nil {
		t.Fatalf("bundle results = %+v, want an entry for %q (offline suggest)", resp.Results, corynephorusConceptID)
	}
	if coryn.Canonical != "Corynephorus canescens" {
		t.Errorf("bundle canonical = %q, want %q", coryn.Canonical, "Corynephorus canescens")
	}
	if !coryn.InArea {
		t.Error("bundle in_area = false, want true for area=AUT")
	}
}

// traitsIntegrationResponse mirrors internal/adapters/http.traitsResponseDTO
// (see traits.go), trimmed to the fields this test asserts on.
type traitsIntegrationResponse struct {
	ConceptID string `json:"concept_id"`
	Traits    []struct {
		Vocab        string `json:"vocab"`
		VocabVersion string `json:"vocab_version"`
		Values       []struct {
			Dim   string  `json:"dim"`
			Value float64 `json:"value"`
			Scale struct {
				Min        float64 `json:"min"`
				Max        float64 `json:"max"`
				Normalized bool    `json:"normalized"`
			} `json:"scale"`
		} `json:"values"`
	} `json:"traits"`
}

// TestIntegration_TraitsFuzzyClassification is SP3's end-to-end proof: it
// ingests the WCVP fixture together with the EIVE and Tichý trait fixtures
// (testdata/dataset-traits.yaml — a manifest dedicated to this test so the
// SP1/SP4 fixture manifest testdata/dataset.yaml, shared with
// internal/app/ingest_test.go's len(reports.Traits)==1 assertion, stays
// untouched) through the real CLI/app path, then drives three real-HTTP
// guarantees SP3 added on top of SP1/SP2:
//
//  1. GET /v1/concept/{id}/traits returns BOTH ingested vocabularies with
//     distinct vocab_version strings, and Tichý's T and L dimensions carry
//     DIFFERENT scale ranges in the same response — the per-value-scale
//     honesty guarantee (domain.ScaleFor's doc comment: Tichý T is 1-12,
//     L is 1-9, so one Set-wide scale would misrepresent one of them).
//  2. POST /v1/match with a single-letter typo of a name the fixture
//     actually carries (Corynephorus canescens -> "canescans") resolves
//     match_type:"fuzzy" with requires_review:true — mandatory on every
//     fuzzy hit regardless of how high the similarity score is.
//  3. GET /v1/concept/{id} renders a non-empty classification chain: the
//     WCVP fixture's Corynephorus canescens (405825) has parent_id ==
//     Corynephorus (451295), a genus-level concept the fixture also
//     carries.
func TestIntegration_TraitsFuzzyClassification(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "hostus.sqlite")

	ctx := context.Background()
	reports, err := app.Ingest(ctx, "testdata/dataset-traits.yaml", dbPath)
	if err != nil {
		t.Fatalf("app.Ingest: unexpected error: %v", err)
	}
	if len(reports.Backbone.Backbones) == 0 {
		t.Fatal("app.Ingest: empty backbone report, want at least the wcvp backbone")
	}
	if len(reports.Traits) != 2 {
		t.Fatalf("len(reports.Traits) = %d, want 2 (eive, tichy2023)", len(reports.Traits))
	}
	for _, tr := range reports.Traits {
		if tr.Matched == 0 {
			t.Errorf("trait vocab %q: Matched = 0, want the fixture's WCVP-resolvable rows to have been written", tr.Vocab)
		}
	}

	cfg := testConfig()
	cfg.SQLite.Path = dbPath
	a, err := app.New(cfg)
	if err != nil {
		t.Fatalf("app.New: unexpected error: %v", err)
	}
	t.Cleanup(func() { _ = a.Shutdown(context.Background()) })

	ts := httptest.NewServer(a.Router)
	defer ts.Close()
	client := ts.Client()

	assertTraitsEndToEnd(t, client, ts.URL)
	assertFuzzyMatch(t, client, ts.URL)
	assertClassificationEndToEnd(t, client, ts.URL)
}

// assertTraitsEndToEnd drives GET /v1/concept/{corynephorus}/traits over
// real HTTP and checks both fixture vocabularies are present with distinct
// versions, and that Tichý's T and L dimensions carry different scales in
// the very same response — proving the per-VALUE (not per-set) scale
// rendering end to end, not just at the in-process handler-test level
// internal/adapters/http/traits_test.go already covers.
func assertTraitsEndToEnd(t *testing.T, client *http.Client, baseURL string) {
	t.Helper()
	resp, err := client.Get(baseURL + "/v1/concept/" + corynephorusConceptID + "/traits")
	if err != nil {
		t.Fatalf("GET /v1/concept/{id}/traits: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /v1/concept/{id}/traits: status = %d, want 200", resp.StatusCode)
	}
	var body traitsIntegrationResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decoding /v1/concept/{id}/traits response: %v", err)
	}
	_ = resp.Body.Close()

	if body.ConceptID != corynephorusConceptID {
		t.Errorf("concept_id = %q, want %q", body.ConceptID, corynephorusConceptID)
	}
	if len(body.Traits) != 2 {
		t.Fatalf("len(traits) = %d, want 2 (eive, tichy2023)", len(body.Traits))
	}

	byVocab := make(map[string]int, len(body.Traits))
	for i, ts := range body.Traits {
		byVocab[ts.Vocab] = i
	}
	eiveIdx, haveEive := byVocab["eive"]
	tichyIdx, haveTichy := byVocab["tichy2023"]
	if !haveEive || !haveTichy {
		t.Fatalf("traits = %+v, want both %q and %q vocabularies", body.Traits, "eive", "tichy2023")
	}

	eive, tichy := body.Traits[eiveIdx], body.Traits[tichyIdx]
	if eive.VocabVersion == tichy.VocabVersion {
		t.Errorf("eive.vocab_version == tichy2023.vocab_version == %q, want distinct versions", eive.VocabVersion)
	}
	if eive.VocabVersion != "1.0" {
		t.Errorf("eive.vocab_version = %q, want %q", eive.VocabVersion, "1.0")
	}
	if tichy.VocabVersion != "2.0" {
		t.Errorf("tichy2023.vocab_version = %q, want %q", tichy.VocabVersion, "2.0")
	}

	var tScale, lScale struct {
		Min, Max   float64
		Normalized bool
		found      bool
	}
	for _, v := range tichy.Values {
		switch v.Dim {
		case "T":
			tScale.Min, tScale.Max, tScale.Normalized, tScale.found = v.Scale.Min, v.Scale.Max, v.Scale.Normalized, true
		case "L":
			lScale.Min, lScale.Max, lScale.Normalized, lScale.found = v.Scale.Min, v.Scale.Max, v.Scale.Normalized, true
		}
	}
	if !tScale.found || !lScale.found {
		t.Fatalf("tichy2023.values = %+v, want both T and L dims present", tichy.Values)
	}
	if tScale.Max == lScale.Max {
		t.Errorf("Tichý T.scale.max == L.scale.max == %v, want them to differ (T: 1-12, L: 1-9 — domain.ScaleFor)", tScale.Max)
	}
	if tScale.Max != 12 {
		t.Errorf("Tichý T.scale.max = %v, want 12", tScale.Max)
	}
	if lScale.Max != 9 {
		t.Errorf("Tichý L.scale.max = %v, want 9", lScale.Max)
	}
}

// assertFuzzyMatch posts a single-letter typo of the fixture's
// "Corynephorus canescens" ("canescans") to POST /v1/match and checks it
// resolves match_type:"fuzzy" with requires_review:true — mandatory on
// every fuzzy hit per spec §B.2, regardless of the similarity score, and
// resolves to the SAME concept the correctly-spelled name would.
func assertFuzzyMatch(t *testing.T, client *http.Client, baseURL string) {
	t.Helper()
	const matchBody = `{"names": [{"id": "typo", "verbatim": "Corynephorus canescans"}]}`
	resp, err := client.Post(baseURL+"/v1/match", "application/json", bytes.NewBufferString(matchBody))
	if err != nil {
		t.Fatalf("POST /v1/match: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /v1/match: status = %d, want 200", resp.StatusCode)
	}
	var match integrationMatchResponse
	if err := json.NewDecoder(resp.Body).Decode(&match); err != nil {
		t.Fatalf("decoding /v1/match response: %v", err)
	}
	_ = resp.Body.Close()

	if len(match.Results) != 1 {
		t.Fatalf("len(results) = %d, want 1", len(match.Results))
	}
	r := match.Results[0]
	if r.MatchType != "fuzzy" {
		t.Errorf("match_type = %q, want %q", r.MatchType, "fuzzy")
	}
	if r.ConceptID != corynephorusConceptID {
		t.Errorf("concept_id = %q, want %q (the typo'd name's correctly-spelled match)", r.ConceptID, corynephorusConceptID)
	}
	if !r.RequiresReview {
		t.Error("requires_review = false, want true (mandatory on every fuzzy hit per spec §B.2, regardless of similarity score)")
	}
}

// assertClassificationEndToEnd drives GET /v1/concept/{corynephorus} over
// real HTTP and checks the classification chain the WCVP fixture actually
// carries: Corynephorus canescens' (405825) parent is the genus concept
// Corynephorus (451295), also present in the fixture — see
// wcvp_taxon.csv's parentNameUsageID column.
func assertClassificationEndToEnd(t *testing.T, client *http.Client, baseURL string) {
	t.Helper()
	resp, err := client.Get(baseURL + "/v1/concept/" + corynephorusConceptID)
	if err != nil {
		t.Fatalf("GET /v1/concept: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /v1/concept: status = %d, want 200", resp.StatusCode)
	}
	var body struct {
		Classification []struct {
			ConceptID string `json:"concept_id"`
			Canonical string `json:"canonical"`
			Rank      string `json:"rank"`
		} `json:"classification"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decoding /v1/concept response: %v", err)
	}
	_ = resp.Body.Close()

	if len(body.Classification) == 0 {
		t.Fatal("classification is empty, want at least the Corynephorus genus ancestor")
	}
	parent := body.Classification[len(body.Classification)-1]
	if parent.Canonical != "Corynephorus" {
		t.Errorf("classification's last (immediate parent) entry canonical = %q, want %q", parent.Canonical, "Corynephorus")
	}
	if parent.Rank != "GENUS" {
		t.Errorf("classification's last entry rank = %q, want %q", parent.Rank, "GENUS")
	}
}

// TestIntegration_OfflineBundleConceptSuggestTraitsOffline is the SP2
// landmine (see internal/app/integration_test.go's CHANGELOG entry on
// ExportBundle's self-referencing FK handling), now covered end to end
// WITH the traits SP3 added: it exports an area=AUT bundle from a database
// carrying both WCVP and the trait fixtures, where Corynephorus canescens'
// parent (the genus concept, 451295) is deliberately OUT OF SCOPE — the
// WCVP fixture never gives the genus-level concept its own AUT distribution
// row — and then opens ONLY that bundle file (never touching the source
// database again) to prove Concept, Classification, Suggest and Traits all
// still work against it standalone. If ExportBundle's out-of-scope
// self-reference nulling (T7) ever regressed, Concept/Classification would
// either 500 (dangling FK) or the bundle write itself would fail a FK
// constraint — this test would catch either.
func TestIntegration_OfflineBundleConceptSuggestTraitsOffline(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "hostus.sqlite")
	bundlePath := filepath.Join(dir, "bundle.sqlite")

	if _, err := app.Ingest(ctx, "testdata/dataset-traits.yaml", dbPath); err != nil {
		t.Fatalf("app.Ingest: unexpected error: %v", err)
	}

	src, err := sqlite.Open(dbPath)
	if err != nil {
		t.Fatalf("sqlite.Open(source): unexpected error: %v", err)
	}

	// Confirm the landmine precondition on the SOURCE db before exporting:
	// the genus parent must actually be present but WITHOUT an AUT
	// distribution row, so scopeConceptIDs genuinely excludes it (a stale
	// fixture that gave the genus an AUT row too would make this test
	// exercise nothing).
	genusConceptID := "wcvp:concept:451295"
	if _, _, _, _, err := src.Concept(ctx, genusConceptID); err != nil {
		_ = src.Close()
		t.Fatalf("source db: Concept(%q): unexpected error: %v (fixture must carry the genus concept)", genusConceptID, err)
	}

	report, err := sqlite.ExportBundle(ctx, src, bundlePath, sqlite.BundleOpts{Area: "AUT", SnapshotVersion: "v1"})
	if err != nil {
		_ = src.Close()
		t.Fatalf("sqlite.ExportBundle: unexpected error: %v", err)
	}
	if err := src.Close(); err != nil {
		t.Fatalf("closing source db: %v", err)
	}
	if report.Concepts == 0 {
		t.Fatal("sqlite.ExportBundle: report.Concepts = 0, want at least the AUT-scoped Corynephorus canescens concept")
	}

	// Open ONLY the bundle from here on.
	bundle, err := sqlite.Open(bundlePath)
	if err != nil {
		t.Fatalf("sqlite.Open(bundle): unexpected error: %v", err)
	}
	defer func() { _ = bundle.Close() }()

	// The genus parent was out of scope: it must NOT have been copied into
	// the bundle at all.
	if _, _, _, _, err := bundle.Concept(ctx, genusConceptID); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("bundle: Concept(%q) err = %v, want %v (out-of-scope genus must not be in the bundle)", genusConceptID, err, domain.ErrNotFound)
	}

	// Concept: Corynephorus canescens itself must resolve, with its
	// out-of-scope self-reference (parent_id) NULLed rather than the
	// bundle write failing an FK constraint or Concept() erroring.
	concept, _, _, _, err := bundle.Concept(ctx, corynephorusConceptID)
	if err != nil {
		t.Fatalf("bundle: Concept(%q): unexpected error: %v", corynephorusConceptID, err)
	}
	if concept.AcceptedName.Canonical != "Corynephorus canescens" {
		t.Errorf("bundle: concept canonical = %q, want %q", concept.AcceptedName.Canonical, "Corynephorus canescens")
	}

	// Classification: must not error just because the parent it would have
	// walked to is out of scope — an empty chain is the correct, honest
	// result (Classification's doc comment: a NULL parent_id stops the
	// walk without error).
	classification, err := bundle.Classification(ctx, corynephorusConceptID)
	if err != nil {
		t.Fatalf("bundle: Classification(%q): unexpected error: %v", corynephorusConceptID, err)
	}
	if len(classification) != 0 {
		t.Errorf("bundle: classification = %+v, want empty (the only parent was out of scope and must have been NULLed)", classification)
	}

	// Suggest: unchanged from TestIntegration_OfflineBundleServesSuggestOffline.
	suggestResp, err := application.Suggest(ctx, bundle, application.SuggestRequest{Q: "coryn", Area: "AUT"})
	if err != nil {
		t.Fatalf("application.Suggest against bundle: unexpected error: %v", err)
	}
	var coryn *domain.SuggestItem
	for i := range suggestResp.Results {
		if suggestResp.Results[i].ConceptID == corynephorusConceptID {
			coryn = &suggestResp.Results[i]
			break
		}
	}
	if coryn == nil {
		t.Fatalf("bundle suggest results = %+v, want an entry for %q", suggestResp.Results, corynephorusConceptID)
	}

	// Traits: the bundle must carry both trait vocabularies for the
	// AUT-scoped Corynephorus canescens concept (sqlite.copyConceptScopedTables
	// copies trait_value scoped by the same concept id set, plus
	// trait_vocabulary in full).
	traitSets, err := bundle.Traits(ctx, corynephorusConceptID, nil)
	if err != nil {
		t.Fatalf("bundle: Traits(%q): unexpected error: %v", corynephorusConceptID, err)
	}
	if len(traitSets) != 2 {
		t.Fatalf("bundle: len(traitSets) = %d, want 2 (eive, tichy2023) — traits must survive the offline bundle export", len(traitSets))
	}
}

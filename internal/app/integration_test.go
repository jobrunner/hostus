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
	"io"
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
	assertSynonymsNomStatusFilter(t, client, ts.URL)
	assertSynonymsRankFilter(t, client, ts.URL)
	assertMetricsExposed(t, client, ts.URL)
}

// festucaOvinaConceptID is the WCVP fixture's Festuca ovina L. (415853). It
// is the one fixture concept whose synonyms carry a populated
// `nomenclaturalstatus`, which is what makes it the UC5 nom_status case:
// Avena dura Salisb. is ", nom. illeg. superfl." and Festuca ovina var.
// vulgaris Schrad. is ", not validly publ." — see
// internal/adapters/wcvp/testdata/wcvp-sample/wcvp_taxon.csv.
const festucaOvinaConceptID = "wcvp:concept:415853"

// integrationSynonymsResponse mirrors internal/adapters/http.synonymsResponseDTO
// (see synonyms.go), trimmed to the fields these assertions read.
type integrationSynonymsResponse struct {
	ConceptID       string `json:"concept_id"`
	Relevance       string `json:"relevance"`
	PublicationRank string `json:"publication_rank"`
	Synonyms        []struct {
		Position           int    `json:"position"`
		NameID             string `json:"name_id"`
		Canonical          string `json:"canonical"`
		Rank               string `json:"rank"`
		Typification       string `json:"typification"`
		NomStatus          string `json:"nom_status"`
		NomStatusJudgement string `json:"nom_status_judgement"`
		Publishable        bool   `json:"publishable"`
		Exclusion          string `json:"exclusion"`
		Reason             string `json:"reason"`
	} `json:"synonyms"`
	Summary struct {
		Total                int            `json:"total"`
		Publishable          int            `json:"publishable"`
		Returned             int            `json:"returned"`
		Absent               int            `json:"absent"`
		Excluded             map[string]int `json:"excluded"`
		UnclassifiedStatuses []string       `json:"unclassified_statuses"`
	} `json:"summary"`
}

// getSynonyms drives GET /v1/concept/{id}/synonyms over real HTTP and
// decodes the envelope.
func getSynonyms(t *testing.T, client *http.Client, baseURL, conceptID, query string) integrationSynonymsResponse {
	t.Helper()
	url := baseURL + "/v1/concept/" + conceptID + "/synonyms"
	if query != "" {
		url += "?" + query
	}
	resp, err := client.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s: status = %d, want 200", url, resp.StatusCode)
	}
	var out integrationSynonymsResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decoding %s response: %v", url, err)
	}
	return out
}

// synonymNames returns the canonical names of a response's `synonyms`, in
// response order.
func synonymNames(res integrationSynonymsResponse) []string {
	out := make([]string, 0, len(res.Synonyms))
	for _, s := range res.Synonyms {
		out = append(out, s.Canonical)
	}
	return out
}

// containsName reports whether names contains want.
func containsName(names []string, want string) bool {
	for _, n := range names {
		if n == want {
			return true
		}
	}
	return false
}

// synonymReason returns the exclusion rule and reason the response gave for
// one canonical name, and whether that name was present at all.
func synonymReason(res integrationSynonymsResponse, canonical string) (string, string, bool) {
	for _, s := range res.Synonyms {
		if s.Canonical == canonical {
			return s.Exclusion, s.Reason, true
		}
	}
	return "", "", false
}

// assertSynonymsNomStatusFilter is SP6's end-to-end proof that UC5's
// NOMENCLATURAL-STATUS criterion survives the full ingest -> serve -> HTTP
// round trip, and that the response EXPLAINS the difference it made.
//
// Festuca ovina L. carries five synonyms in the fixture. Two of them have a
// populated nomenclaturalstatus that domain.ClassifyNomStatus judges
// disqualifying — Avena dura Salisb. (", nom. illeg. superfl.") and Festuca
// ovina var. vulgaris Schrad. (", not validly publ.") — so
// relevance=publication must return exactly the other three, BY NAME, and
// the summary must attribute both removals to the nom_status rule rather
// than merely coming back shorter. The unfiltered list must still carry all
// five, each with its verdict: the filter is opt-in (see
// application.RelevanceAll).
func assertSynonymsNomStatusFilter(t *testing.T, client *http.Client, baseURL string) {
	t.Helper()

	wantAll := []string{
		"Avena dura", "Avena ovina", "Bromus ovinus",
		"Festuca duriuscula", "Festuca ovina var. vulgaris",
	}
	wantPublishable := []string{"Avena ovina", "Bromus ovinus", "Festuca duriuscula"}
	wantGone := []string{"Avena dura", "Festuca ovina var. vulgaris"}

	all := getSynonyms(t, client, baseURL, festucaOvinaConceptID, "")
	if all.Relevance != "all" {
		t.Errorf("unfiltered relevance = %q, want %q", all.Relevance, "all")
	}
	assertSynonymSet(t, "unfiltered", all, wantAll, nil)

	pub := getSynonyms(t, client, baseURL, festucaOvinaConceptID, "relevance=publication&rank=species")
	if pub.Relevance != "publication" {
		t.Errorf("filtered relevance = %q, want %q", pub.Relevance, "publication")
	}
	if pub.PublicationRank != "species" {
		t.Errorf("publication_rank = %q, want %q", pub.PublicationRank, "species")
	}
	// The point of the whole endpoint: SHORTER, and shorter for a stated
	// reason. A count assertion alone would pass on the wrong three names.
	assertShorterThan(t, pub, all)
	assertSynonymSet(t, "filtered", pub, wantPublishable, wantGone)

	// The exclusion summary must EXPLAIN the two missing rows, and it must
	// describe the concept (five synonyms), not the returned page.
	if pub.Summary.Total != len(wantAll) {
		t.Errorf("summary.total = %d, want %d (the concept's synonyms, not the page)", pub.Summary.Total, len(wantAll))
	}
	if pub.Summary.Publishable != len(wantPublishable) {
		t.Errorf("summary.publishable = %d, want %d", pub.Summary.Publishable, len(wantPublishable))
	}
	if got := pub.Summary.Excluded["nom_status"]; got != len(wantGone) {
		t.Errorf("summary.excluded[nom_status] = %d, want %d (Avena dura + Festuca ovina var. vulgaris)", got, len(wantGone))
	}
	if len(pub.Summary.UnclassifiedStatuses) != 0 {
		t.Errorf("summary.unclassified_statuses = %v, want empty (both statuses are classified)", pub.Summary.UnclassifiedStatuses)
	}
	if pub.Summary.Absent != len(wantPublishable) {
		t.Errorf("summary.absent = %d, want %d — every published synonym here rests on an EMPTY nom_status",
			pub.Summary.Absent, len(wantPublishable))
	}

	// ... and the list must name the rule per synonym, so a caller can audit
	// the difference without re-deriving it.
	assertSynonymExclusion(t, all, "Avena dura", "nom_status", "nom. illeg. superfl.", "disqualifying")
}

// assertSynonymSet checks a response's `synonyms` by NAME: every entry in
// wantPresent must be there, no entry in wantAbsent may be, and the list
// must hold exactly len(wantPresent) rows. Names rather than counts is the
// whole point — a count assertion passes on the wrong three synonyms.
func assertSynonymSet(t *testing.T, label string, res integrationSynonymsResponse, wantPresent, wantAbsent []string) {
	t.Helper()
	got := synonymNames(res)
	for _, want := range wantPresent {
		if !containsName(got, want) {
			t.Errorf("%s synonyms = %v, want it to contain %q", label, got, want)
		}
	}
	for _, gone := range wantAbsent {
		if containsName(got, gone) {
			t.Errorf("%s synonyms = %v, want %q to be excluded", label, got, gone)
		}
	}
	if len(got) != len(wantPresent) {
		t.Errorf("len(%s synonyms) = %d (%v), want %d", label, len(got), got, len(wantPresent))
	}
}

// assertShorterThan pins the relation the endpoint exists for: filtering
// must actually remove rows.
func assertShorterThan(t *testing.T, filtered, unfiltered integrationSynonymsResponse) {
	t.Helper()
	if len(filtered.Synonyms) >= len(unfiltered.Synonyms) {
		t.Errorf("filtered list (%d: %v) must be shorter than the unfiltered one (%d: %v)",
			len(filtered.Synonyms), synonymNames(filtered),
			len(unfiltered.Synonyms), synonymNames(unfiltered))
	}
}

// assertSynonymExclusion checks that one named synonym carries the expected
// exclusion rule and that its reason mentions every wantInReason fragment.
func assertSynonymExclusion(t *testing.T, res integrationSynonymsResponse, canonical, wantExclusion string, wantInReason ...string) {
	t.Helper()
	exclusion, reason, ok := synonymReason(res, canonical)
	if !ok {
		t.Fatalf("synonyms = %v, want an entry for %q", synonymNames(res), canonical)
	}
	if exclusion != wantExclusion {
		t.Errorf("%s: exclusion = %q, want %q", canonical, exclusion, wantExclusion)
	}
	for _, fragment := range wantInReason {
		if !strings.Contains(reason, fragment) {
			t.Errorf("%s: reason = %q, want it to mention %q", canonical, reason, fragment)
		}
	}
}

// assertSynonymsRankFilter is the RANK half of UC5, on Corynephorus
// canescens — and it pins the gap the how-to documents rather than papering
// over it. The fixture's four synonyms are two VARIETY rows, one FORM row
// and one SUBSPECIES row, none of them with a nomenclatural status.
// rank=species therefore withholds three, and the SUBSPECIES survives: UC5
// names VARIETY/SUBVARIETY/FORM/SUBFORM and NOT subspecies
// (domain.RanksBelowSpecies), so "Corynephorus canescens subsp. maritimus"
// is published. If that ever changes, this assertion is the one that has to
// be argued with.
func assertSynonymsRankFilter(t *testing.T, client *http.Client, baseURL string) {
	t.Helper()

	// `relevance=all&rank=species` is the same JUDGEMENT as the filtered
	// call, without the filtering: every synonym comes back, each carrying
	// the verdict rank=species gave it. That is what makes the two lists
	// comparable — a bare `relevance=all` passes no rank at all, so nothing
	// would be judged excluded and the exclusion reasons below would be
	// empty for a reason that has nothing to do with the data.
	all := getSynonyms(t, client, baseURL, corynephorusConceptID, "relevance=all&rank=species")
	pub := getSynonyms(t, client, baseURL, corynephorusConceptID, "relevance=publication&rank=species")
	assertShorterThan(t, pub, all)
	// The SUBSPECIES survivor is asserted BY NAME as the only publishable
	// entry: that is the documented gap, not an accident.
	assertSynonymSet(t, "filtered",
		pub,
		[]string{"Corynephorus canescens subsp. maritimus"},
		[]string{
			"Corynephorus canescens var. montana",
			"Corynephorus canescens f. pallidus",
			"Weingaertneria canescens var. pallida",
		})

	if got := pub.Summary.Excluded["rank"]; got != 3 {
		t.Errorf("summary.excluded[rank] = %d, want 3 (two VARIETY + one FORM)", got)
	}
	if got := pub.Summary.Excluded["nom_status"]; got != 0 {
		t.Errorf("summary.excluded[nom_status] = %d, want 0 — none of these four carries a status", got)
	}

	assertSynonymExclusion(t, all, "Corynephorus canescens f. pallidus", "rank", "FORM")
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

	// dataset-no-namespace.yaml, not dataset.yaml: this test is about the
	// bundle being self-contained offline, and every source it pins is
	// redistribution=allowed. The FloraVeg name space (unknown) would be
	// refused here by design — that refusal is pinned by
	// internal/app's TestBundle_RefusesNameSpaceByDefault instead.
	if _, err := app.Ingest(ctx, "testdata/dataset-no-namespace.yaml", dbPath); err != nil {
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
	ConceptID string                `json:"concept_id"`
	Traits    []traitSetIntegration `json:"traits"`
}

type traitSetIntegration struct {
	Vocab        string                  `json:"vocab"`
	VocabVersion string                  `json:"vocab_version"`
	Values       []traitValueIntegration `json:"values"`
}

type traitValueIntegration struct {
	Dim   string  `json:"dim"`
	Value float64 `json:"value"`
	Scale struct {
		Min        float64 `json:"min"`
		Max        float64 `json:"max"`
		Normalized bool    `json:"normalized"`
	} `json:"scale"`
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

	eive, tichy := assertTraitVocabsPresent(t, body.Traits)
	assertTraitVocabVersions(t, eive, tichy)
	assertTichyScales(t, tichy)
}

// assertTraitVocabsPresent locates the eive and tichy2023 trait sets within
// traits by vocab name, failing fast if either vocabulary is missing.
func assertTraitVocabsPresent(t *testing.T, traits []traitSetIntegration) (eive, tichy traitSetIntegration) {
	t.Helper()
	byVocab := make(map[string]int, len(traits))
	for i, ts := range traits {
		byVocab[ts.Vocab] = i
	}
	eiveIdx, haveEive := byVocab["eive"]
	tichyIdx, haveTichy := byVocab["tichy2023"]
	if !haveEive || !haveTichy {
		t.Fatalf("traits = %+v, want both %q and %q vocabularies", traits, "eive", "tichy2023")
	}
	return traits[eiveIdx], traits[tichyIdx]
}

// assertTraitVocabVersions checks eive and tichy2023 carry distinct,
// fixture-pinned vocab_version strings.
func assertTraitVocabVersions(t *testing.T, eive, tichy traitSetIntegration) {
	t.Helper()
	if eive.VocabVersion == tichy.VocabVersion {
		t.Errorf("eive.vocab_version == tichy2023.vocab_version == %q, want distinct versions", eive.VocabVersion)
	}
	if eive.VocabVersion != "1.0" {
		t.Errorf("eive.vocab_version = %q, want %q", eive.VocabVersion, "1.0")
	}
	if tichy.VocabVersion != "2.0" {
		t.Errorf("tichy2023.vocab_version = %q, want %q", tichy.VocabVersion, "2.0")
	}
}

// assertTichyScales checks Tichý's T and L dimensions carry different scale
// ranges in the same response (domain.ScaleFor: T is 1-12, L is 1-9).
func assertTichyScales(t *testing.T, tichy traitSetIntegration) {
	t.Helper()
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
	genusConceptID := "wcvp:concept:451295"

	if _, err := app.Ingest(ctx, "testdata/dataset-traits.yaml", dbPath); err != nil {
		t.Fatalf("app.Ingest: unexpected error: %v", err)
	}

	exportOfflineBundleWithGenusPrecondition(t, dbPath, bundlePath, genusConceptID)

	// Open ONLY the bundle from here on.
	bundle, err := sqlite.Open(bundlePath)
	if err != nil {
		t.Fatalf("sqlite.Open(bundle): unexpected error: %v", err)
	}
	defer func() { _ = bundle.Close() }()

	assertBundleGenusOutOfScope(t, bundle, genusConceptID)
	assertBundleConceptSurvivesExport(t, bundle)
	assertBundleClassificationEmptyAfterExport(t, bundle)
	assertBundleSuggestFindsCorynephorus(t, bundle)
	assertBundleTraitsSurviveExport(t, bundle)
}

// exportOfflineBundleWithGenusPrecondition opens the source db, confirms the
// landmine precondition on it (genusConceptID must be present but WITHOUT an
// AUT distribution row, so scopeConceptIDs genuinely excludes it — a stale
// fixture that gave the genus an AUT row too would make the rest of this
// test exercise nothing), exports the AUT-scoped bundle, and closes the
// source db.
func exportOfflineBundleWithGenusPrecondition(t *testing.T, dbPath, bundlePath, genusConceptID string) {
	t.Helper()
	ctx := context.Background()

	src, err := sqlite.Open(dbPath)
	if err != nil {
		t.Fatalf("sqlite.Open(source): unexpected error: %v", err)
	}
	defer func() {
		if err := src.Close(); err != nil {
			t.Errorf("closing source db: %v", err)
		}
	}()

	if _, _, _, _, err := src.Concept(ctx, genusConceptID); err != nil {
		t.Fatalf("source db: Concept(%q): unexpected error: %v (fixture must carry the genus concept)", genusConceptID, err)
	}

	report, err := sqlite.ExportBundle(ctx, src, bundlePath, sqlite.BundleOpts{Area: "AUT", SnapshotVersion: "v1"})
	if err != nil {
		t.Fatalf("sqlite.ExportBundle: unexpected error: %v", err)
	}
	if report.Concepts == 0 {
		t.Fatal("sqlite.ExportBundle: report.Concepts = 0, want at least the AUT-scoped Corynephorus canescens concept")
	}
}

// assertBundleGenusOutOfScope checks the out-of-scope genus concept was not
// copied into the bundle at all.
func assertBundleGenusOutOfScope(t *testing.T, bundle *sqlite.DB, genusConceptID string) {
	t.Helper()
	ctx := context.Background()
	if _, _, _, _, err := bundle.Concept(ctx, genusConceptID); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("bundle: Concept(%q) err = %v, want %v (out-of-scope genus must not be in the bundle)", genusConceptID, err, domain.ErrNotFound)
	}
}

// bundleConceptOnly wraps bundle.Concept for call sites that need only the
// resolved concept, not its synonyms/xrefs/distributions.
func bundleConceptOnly(ctx context.Context, bundle *sqlite.DB, id string) (*domain.Concept, error) {
	concept, synonyms, xrefs, distributions, err := bundle.Concept(ctx, id)
	_ = synonyms
	_ = xrefs
	_ = distributions
	if err != nil {
		return nil, err
	}
	return concept, nil
}

// assertBundleConceptSurvivesExport checks Corynephorus canescens itself
// resolves from the bundle, with its out-of-scope self-reference
// (parent_id) NULLed rather than the bundle write failing an FK constraint
// or Concept() erroring.
func assertBundleConceptSurvivesExport(t *testing.T, bundle *sqlite.DB) {
	t.Helper()
	concept, err := bundleConceptOnly(context.Background(), bundle, corynephorusConceptID)
	if err != nil {
		t.Fatalf("bundle: Concept(%q): unexpected error: %v", corynephorusConceptID, err)
	}
	if concept.AcceptedName.Canonical != "Corynephorus canescens" {
		t.Errorf("bundle: concept canonical = %q, want %q", concept.AcceptedName.Canonical, "Corynephorus canescens")
	}
}

// assertBundleClassificationEmptyAfterExport checks Classification does not
// error just because the parent it would have walked to is out of scope —
// an empty chain is the correct, honest result (Classification's doc
// comment: a NULL parent_id stops the walk without error).
func assertBundleClassificationEmptyAfterExport(t *testing.T, bundle *sqlite.DB) {
	t.Helper()
	classification, err := bundle.Classification(context.Background(), corynephorusConceptID)
	if err != nil {
		t.Fatalf("bundle: Classification(%q): unexpected error: %v", corynephorusConceptID, err)
	}
	if len(classification) != 0 {
		t.Errorf("bundle: classification = %+v, want empty (the only parent was out of scope and must have been NULLed)", classification)
	}
}

// assertBundleSuggestFindsCorynephorus is unchanged from
// TestIntegration_OfflineBundleServesSuggestOffline.
func assertBundleSuggestFindsCorynephorus(t *testing.T, bundle *sqlite.DB) {
	t.Helper()
	ctx := context.Background()
	suggestResp, err := application.Suggest(ctx, bundle, application.SuggestRequest{Q: "coryn", Area: "AUT"})
	if err != nil {
		t.Fatalf("application.Suggest against bundle: unexpected error: %v", err)
	}
	for i := range suggestResp.Results {
		if suggestResp.Results[i].ConceptID == corynephorusConceptID {
			return
		}
	}
	t.Fatalf("bundle suggest results = %+v, want an entry for %q", suggestResp.Results, corynephorusConceptID)
}

// assertBundleTraitsSurviveExport checks the bundle carries both trait
// vocabularies for the AUT-scoped Corynephorus canescens concept
// (sqlite.copyConceptScopedTables copies trait_value scoped by the same
// concept id set, plus trait_vocabulary in full).
func assertBundleTraitsSurviveExport(t *testing.T, bundle *sqlite.DB) {
	t.Helper()
	traitSets, err := bundle.Traits(context.Background(), corynephorusConceptID, nil)
	if err != nil {
		t.Fatalf("bundle: Traits(%q): unexpected error: %v", corynephorusConceptID, err)
	}
	if len(traitSets) != 2 {
		t.Fatalf("bundle: len(traitSets) = %d, want 2 (eive, tichy2023) — traits must survive the offline bundle export", len(traitSets))
	}
}

// The three CDM fixture concepts and the two sec. reference spaces SP5's
// end-to-end assertions pin. All five ids come from
// pipelines/cdm/fixtures/cdm-{concepts,relations}-fixture.csv, which is a
// verbatim slice of the real crawl — so an id asserted here is an id the
// upstream source actually issued, not a test invention.
const (
	// cdmAbiesWisskirchen is Abies alba Mill. sec. Wisskirchen & Haeupler
	// 1998, the crawl's reference concept.
	cdmAbiesWisskirchen = "cdm:concept:872088a4-95f4-472c-ae79-a29028bb3fbf"
	// cdmAbiesFloraEuropaea is the SAME NAME in a different sec. space —
	// the pair that makes the sec. separation visible.
	cdmAbiesFloraEuropaea = "cdm:concept:90ee17be-d455-4564-949d-9c53e27a6a6f"
	// cdmPinusAbiesAuct is Pinus abies L. 1753 sec. "Andere Referenzen (fuer
	// auct. Synonyme)". It is the end of BOTH a congruent row and a
	// misapplied-name row against cdmAbiesWisskirchen; only the former may
	// ever surface.
	cdmPinusAbiesAuct = "cdm:concept:122053a6-abb7-4d4c-9f87-b7b8f6d1afef"
	// cdmAconitumNapellusWisskirchen "Includes" cdmAconitumNeomontanum —
	// the non-congruent, direction-bearing case.
	cdmAconitumNapellusWisskirchen = "cdm:concept:6a24cc06-c5fc-4b29-aaea-ee21be1130a6"
	cdmAconitumNeomontanum         = "cdm:concept:568cacd9-c027-43f2-958c-af7ea6e0baef"

	secFloraEuropaea = "6eeeeacc-1da9-4839-98d6-3169c4237ecd"
	secEhrendorfer   = "790d6644-39f3-4173-ae6c-ec6cbf951a10"
	secAuctSynonyme  = "be62f034-87fd-40e2-bd16-da024e05eaf5"
)

// integrationTranslateResponse mirrors internal/adapters/http.translateResponseDTO
// (see translate.go), trimmed to the fields these assertions read.
type integrationTranslateResponse struct {
	Source struct {
		ConceptID string `json:"concept_id"`
		Canonical string `json:"canonical"`
		Sec       struct {
			ID    string `json:"id"`
			Title string `json:"title"`
		} `json:"sec"`
	} `json:"source"`
	TargetSpace struct {
		ID    string `json:"id"`
		Title string `json:"title"`
	} `json:"target_space"`
	MaxHops    int    `json:"max_hops"`
	Result     string `json:"result"`
	Candidates []struct {
		ConceptID          string  `json:"concept_id"`
		Canonical          string  `json:"canonical"`
		Rank               string  `json:"rank"`
		StoredRelation     string  `json:"stored_relation"`
		RelationFromSource *string `json:"relation_from_source"`
		HasInverse         bool    `json:"has_inverse"`
		Direction          string  `json:"direction"`
		Statement          struct {
			From     string `json:"from"`
			Relation string `json:"relation"`
			To       string `json:"to"`
		} `json:"statement"`
		IsEquality bool   `json:"is_equality"`
		Hops       int    `json:"hops"`
		Source     string `json:"source"`
		Sec        struct {
			ID    string `json:"id"`
			Title string `json:"title"`
		} `json:"sec"`
	} `json:"candidates"`
	Note             string            `json:"note"`
	BackboneVersions map[string]string `json:"backbone_versions"`
}

// postTranslate drives POST /v1/translate over real HTTP, asserts the given
// status, and decodes the envelope.
func postTranslate(t *testing.T, client *http.Client, baseURL, body string, wantStatus int) integrationTranslateResponse {
	t.Helper()
	resp, err := client.Post(baseURL+"/v1/translate", "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("POST /v1/translate %s: %v", body, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != wantStatus {
		t.Fatalf("POST /v1/translate %s: status = %d, want %d", body, resp.StatusCode, wantStatus)
	}
	var out integrationTranslateResponse
	if wantStatus != http.StatusOK {
		return out
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decoding /v1/translate response: %v", err)
	}
	return out
}

// TestIntegration_TranslateBetweenSecSpaces is SP5's end-to-end proof.
// It ingests the WCVP fixture TOGETHER with the CDM concept-source fixture
// through the exact code path `hostus ingest` uses (app.Ingest), serves the
// result through the exact code path `hostus serve` uses (app.New +
// app.Router) behind a real listener, and drives POST /v1/translate purely
// as an HTTP client would.
//
// What it pins, beyond "200 OK":
//
//  1. The sec. separation itself: one name (Abies alba Mill.) is TWO
//     distinct concepts, and translating between them names both ids.
//  2. The relation TYPE and its direction, per candidate — congruent
//     (is_equality) in one direction, includes (NOT equality) in the other.
//  3. That the dropped misapplied-name row is genuinely absent while the
//     congruent row between the same two concepts is present.
//  4. That a WCVP concept translates to an explicit, empty
//     "no_relation_recorded" answer rather than to a name guess. This is not
//     a corner case: it is the measured UC6 reality at full scale (see
//     docs/research/reality-check.md, SP5) — the WCVP and CDM namespaces
//     share no ids, so no WCVP concept carries a concept_relation row.
func TestIntegration_TranslateBetweenSecSpaces(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "hostus.sqlite")

	ctx := context.Background()
	reports, err := app.Ingest(ctx, "testdata/dataset-cdm.yaml", dbPath)
	if err != nil {
		t.Fatalf("app.Ingest: unexpected error: %v", err)
	}
	if len(reports.ConceptSources) != 1 {
		t.Fatalf("len(reports.ConceptSources) = %d, want 1 (the cdm concept source)", len(reports.ConceptSources))
	}
	if reports.ConceptSources[0].RelationsWritten == 0 {
		t.Fatal("cdm ingest wrote no relations — /v1/translate cannot be exercised")
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

	assertTranslateCongruentIntoFloraEuropaea(t, client, ts.URL)
	assertTranslateIncludesIsNotEquality(t, client, ts.URL)
	assertTranslateDropsMisappliedRow(t, client, ts.URL)
	assertTranslateWCVPHasNoRelation(t, client, ts.URL)
	assertTranslateUnknownTargetSpace(t, client, ts.URL)
}

// assertTranslateCongruentIntoFloraEuropaea translates Abies alba Mill. sec.
// Wisskirchen & Haeupler 1998 into TUTIN et al.: Flora Europaea and pins the
// whole answer: exactly one candidate, its id, its sec. space, the stored
// triple verbatim, the direction (the row points AT the source), the
// direction-safe reading, and is_equality.
func assertTranslateCongruentIntoFloraEuropaea(t *testing.T, client *http.Client, baseURL string) {
	t.Helper()
	res := postTranslate(t, client, baseURL,
		`{"concept_id":"`+cdmAbiesWisskirchen+`","target_space":"`+secFloraEuropaea+`"}`, http.StatusOK)

	if res.Result != "translated" {
		t.Fatalf("result = %q, want %q", res.Result, "translated")
	}
	if res.Source.ConceptID != cdmAbiesWisskirchen {
		t.Errorf("source.concept_id = %q, want %q", res.Source.ConceptID, cdmAbiesWisskirchen)
	}
	if res.Source.Sec.Title != "Wisskirchen & Haeupler 1998: Standardliste der Farn- und Blütenpflanzen Deutschlands" {
		t.Errorf("source.sec.title = %q, want the Wisskirchen & Haeupler 1998 Standardliste", res.Source.Sec.Title)
	}
	if res.TargetSpace.Title != "TUTIN et al.: Flora Europaea" {
		t.Errorf("target_space.title = %q, want %q", res.TargetSpace.Title, "TUTIN et al.: Flora Europaea")
	}
	if res.MaxHops != 1 {
		t.Errorf("max_hops = %d, want 1", res.MaxHops)
	}
	if res.BackboneVersions["cdm"] != "2026-08-02" {
		t.Errorf("backbone_versions[cdm] = %q, want %q", res.BackboneVersions["cdm"], "2026-08-02")
	}
	if len(res.Candidates) != 1 {
		t.Fatalf("len(candidates) = %d, want exactly 1 (the Flora Europaea Abies alba)", len(res.Candidates))
	}
	assertCongruentCandidate(t, res)
}

// assertCongruentCandidate pins the single candidate of the congruent
// translation above. It is split out from its caller purely so neither
// function carries the whole envelope-plus-candidate assertion set (gocognit).
func assertCongruentCandidate(t *testing.T, res integrationTranslateResponse) {
	t.Helper()
	c := res.Candidates[0]
	// The candidate is a DIFFERENT concept carrying the SAME name: that is
	// the sec. separation SP5 exists to preserve.
	if c.ConceptID != cdmAbiesFloraEuropaea {
		t.Errorf("candidate.concept_id = %q, want %q", c.ConceptID, cdmAbiesFloraEuropaea)
	}
	if c.ConceptID == res.Source.ConceptID {
		t.Error("candidate and source are the same concept — the two sec. spaces were merged")
	}
	if c.Canonical != "Abies alba" || res.Source.Canonical != "Abies alba" {
		t.Errorf("canonicals = %q / %q, want both %q", res.Source.Canonical, c.Canonical, "Abies alba")
	}
	if c.Sec.ID != secFloraEuropaea {
		t.Errorf("candidate.sec.id = %q, want %q", c.Sec.ID, secFloraEuropaea)
	}
	if c.StoredRelation != string(domain.RelationCongruent) {
		t.Errorf("candidate.stored_relation = %q, want %q", c.StoredRelation, domain.RelationCongruent)
	}
	if !c.IsEquality {
		t.Error("candidate.is_equality = false, want true for a congruent relation")
	}
	if c.Hops != 1 {
		t.Errorf("candidate.hops = %d, want 1", c.Hops)
	}
	if c.Source != "cdm" {
		t.Errorf("candidate.source = %q, want %q", c.Source, "cdm")
	}
	assertCongruentCandidateDirection(t, res)
}

// assertCongruentCandidateDirection pins the direction half of the same
// candidate: the stored row reads "Flora Europaea congruent Wisskirchen",
// i.e. it points AT the source, and the response must say so rather than
// silently re-pointing it.
func assertCongruentCandidateDirection(t *testing.T, res integrationTranslateResponse) {
	t.Helper()
	c := res.Candidates[0]
	if c.Direction != "target_to_source" {
		t.Errorf("candidate.direction = %q, want %q", c.Direction, "target_to_source")
	}
	if c.Statement.From != cdmAbiesFloraEuropaea || c.Statement.To != cdmAbiesWisskirchen {
		t.Errorf("candidate.statement = %s -> %s, want %s -> %s",
			c.Statement.From, c.Statement.To, cdmAbiesFloraEuropaea, cdmAbiesWisskirchen)
	}
	if c.Statement.Relation != string(domain.RelationCongruent) {
		t.Errorf("candidate.statement.relation = %q, want %q", c.Statement.Relation, domain.RelationCongruent)
	}
	// congruent is its own inverse, so the direction-safe reading exists.
	if !c.HasInverse || c.RelationFromSource == nil || *c.RelationFromSource != string(domain.RelationCongruent) {
		t.Errorf("candidate.relation_from_source = %v (has_inverse=%v), want %q",
			c.RelationFromSource, c.HasInverse, domain.RelationCongruent)
	}
}

// assertTranslateIncludesIsNotEquality translates Aconitum napellus subsp.
// napellus sec. Wisskirchen into EHRENDORFER's list, where the stored row
// reads "Wisskirchen INCLUDES Ehrendorfer" — an OUTGOING, non-congruent
// edge. It is the counter-case to the congruent one above: same endpoint,
// same shape, different relation, and is_equality MUST be false.
func assertTranslateIncludesIsNotEquality(t *testing.T, client *http.Client, baseURL string) {
	t.Helper()
	res := postTranslate(t, client, baseURL,
		`{"concept_id":"`+cdmAconitumNapellusWisskirchen+`","target_space":"`+secEhrendorfer+`"}`, http.StatusOK)

	if res.Result != "translated" {
		t.Fatalf("result = %q, want %q", res.Result, "translated")
	}
	if len(res.Candidates) != 1 {
		t.Fatalf("len(candidates) = %d, want exactly 1", len(res.Candidates))
	}
	c := res.Candidates[0]
	if c.ConceptID != cdmAconitumNeomontanum {
		t.Errorf("candidate.concept_id = %q, want %q", c.ConceptID, cdmAconitumNeomontanum)
	}
	if c.StoredRelation != string(domain.RelationIncludes) {
		t.Errorf("candidate.stored_relation = %q, want %q", c.StoredRelation, domain.RelationIncludes)
	}
	if c.IsEquality {
		t.Error("candidate.is_equality = true for an `includes` relation — that would claim identity the source never asserted")
	}
	if c.Direction != "source_to_target" {
		t.Errorf("candidate.direction = %q, want %q", c.Direction, "source_to_target")
	}
	if c.Statement.From != cdmAconitumNapellusWisskirchen || c.Statement.To != cdmAconitumNeomontanum {
		t.Errorf("candidate.statement = %s -> %s, want %s -> %s",
			c.Statement.From, c.Statement.To, cdmAconitumNapellusWisskirchen, cdmAconitumNeomontanum)
	}
	if c.RelationFromSource == nil || *c.RelationFromSource != string(domain.RelationIncludes) {
		t.Errorf("candidate.relation_from_source = %v, want %q", c.RelationFromSource, domain.RelationIncludes)
	}
	if c.Rank != string(domain.RankSubspecies) {
		t.Errorf("candidate.rank = %q, want %q", c.Rank, domain.RankSubspecies)
	}
}

// assertTranslateDropsMisappliedRow translates into the "Andere Referenzen
// (fuer auct. Synonyme)" space, which the fixture connects to the source by
// TWO rows for the same concept pair: a congruent one and a misapplied-name
// one (is_concept_relation=false). The misapplied row is dropped at ingest,
// so exactly one candidate may come back and its relation must be congruent
// — this is where a regression in that drop rule would show up as a claim
// the source never made about circumscriptions.
func assertTranslateDropsMisappliedRow(t *testing.T, client *http.Client, baseURL string) {
	t.Helper()
	res := postTranslate(t, client, baseURL,
		`{"concept_id":"`+cdmAbiesWisskirchen+`","target_space":"`+secAuctSynonyme+`"}`, http.StatusOK)

	if len(res.Candidates) != 1 {
		t.Fatalf("len(candidates) = %d, want exactly 1 — the misapplied-name row must not reach /v1/translate", len(res.Candidates))
	}
	c := res.Candidates[0]
	if c.ConceptID != cdmPinusAbiesAuct {
		t.Errorf("candidate.concept_id = %q, want %q", c.ConceptID, cdmPinusAbiesAuct)
	}
	if c.StoredRelation != string(domain.RelationCongruent) {
		t.Errorf("candidate.stored_relation = %q, want %q", c.StoredRelation, domain.RelationCongruent)
	}
	if c.StoredRelation == string(domain.RelationMisapplied) {
		t.Error("a misapplied-name row surfaced as a translation candidate")
	}
}

// assertTranslateWCVPHasNoRelation drives the honest empty answer: a WCVP
// concept has no concept_relation row at all, so translating it into a CDM
// sec. space is a 200 with an EMPTY candidates array and a note — never a
// same-name concept dressed up as a translation.
func assertTranslateWCVPHasNoRelation(t *testing.T, client *http.Client, baseURL string) {
	t.Helper()
	res := postTranslate(t, client, baseURL,
		`{"concept_id":"`+festucaOvinaConceptID+`","target_space":"`+secFloraEuropaea+`"}`, http.StatusOK)

	if res.Result != "no_relation_recorded" {
		t.Errorf("result = %q, want %q", res.Result, "no_relation_recorded")
	}
	if len(res.Candidates) != 0 {
		t.Errorf("len(candidates) = %d, want 0 — a WCVP concept carries no concept_relation row", len(res.Candidates))
	}
	if res.Note == "" {
		t.Error("note is empty, want the explicit \"keine erfasste Relation\" statement")
	}
}

// assertTranslateUnknownTargetSpace pins that a typo'd sec. space is a 404,
// not an empty translation — the two answers mean entirely different things.
func assertTranslateUnknownTargetSpace(t *testing.T, client *http.Client, baseURL string) {
	t.Helper()
	postTranslate(t, client, baseURL,
		`{"concept_id":"`+cdmAbiesWisskirchen+`","target_space":"not-a-sec-space"}`, http.StatusNotFound)
}

// --- SP8 Task 3: the embedded test console, end to end ---

// consoleProbe is one request replayed against a UI-on and a UI-off server.
type consoleProbe struct {
	method string
	path   string
	body   string
}

// apiProbesAcrossToggle covers every serving endpoint plus the two shapes
// most at risk from a UI: an unknown path under an API prefix (must stay a
// 404) and a known path with the wrong method (must stay a 405).
var apiProbesAcrossToggle = []consoleProbe{
	{http.MethodGet, "/health/live", ""},
	{http.MethodGet, "/health/ready", ""},
	{http.MethodGet, "/metrics", ""},
	{http.MethodGet, "/v1/concept/" + festucaOvinaConceptID, ""},
	{http.MethodGet, "/v1/concept/" + festucaOvinaConceptID + "/traits", ""},
	{http.MethodGet, "/v1/concept/" + festucaOvinaConceptID + "/synonyms", ""},
	{http.MethodGet, "/v1/concept/gibt-es-nicht", ""},
	{http.MethodGet, "/v1/suggest?q=Festuca&limit=5", ""},
	{http.MethodGet, "/v1/xref?authority=powo&id=nonexistent", ""},
	{http.MethodPost, "/v1/match", `{"names":[{"id":"1","verbatim":"Festuca ovina"}]}`},
	{http.MethodPost, "/v1/translate", `{"concept_id":"` + festucaOvinaConceptID + `","target_space":"nope"}`},
	{http.MethodGet, "/v1/nope", ""},
	{http.MethodGet, "/openapi", ""},
	{http.MethodPost, "/health/live", ""},
}

// serveWithConsole boots the REAL composition root against dbPath with the
// console toggled, in front of a real TCP listener. Nothing about the UI is
// stubbed: this is exactly what `hostus serve` builds.
func serveWithConsole(t *testing.T, dbPath string, uiEnabled bool) *httptest.Server {
	t.Helper()
	cfg := testConfig()
	cfg.SQLite.Path = dbPath
	cfg.UI.Enabled = uiEnabled

	a, err := app.New(cfg)
	if err != nil {
		t.Fatalf("app.New(ui.enabled=%v): unexpected error: %v", uiEnabled, err)
	}
	t.Cleanup(func() { _ = a.Shutdown(context.Background()) })

	ts := httptest.NewServer(a.Router)
	t.Cleanup(ts.Close)
	return ts
}

// doProbe runs one probe over real HTTP and returns status, body and
// Content-Type.
func doProbe(t *testing.T, client *http.Client, baseURL string, p consoleProbe) (int, string, string) {
	t.Helper()
	var req *http.Request
	var err error
	if p.body == "" {
		req, err = http.NewRequestWithContext(context.Background(), p.method, baseURL+p.path, nil)
	} else {
		req, err = http.NewRequestWithContext(context.Background(), p.method, baseURL+p.path, strings.NewReader(p.body))
	}
	if err != nil {
		t.Fatalf("%s %s: building request: %v", p.method, p.path, err)
	}
	if p.body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := client.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", p.method, p.path, err)
	}
	defer func() { _ = res.Body.Close() }()
	raw, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("%s %s: reading body: %v", p.method, p.path, err)
	}
	return res.StatusCode, string(raw), res.Header.Get("Content-Type")
}

// getConsole is the GET-only shorthand the console assertions use.
func getConsole(t *testing.T, client *http.Client, baseURL, path string) (int, string, http.Header) {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, baseURL+path, nil)
	if err != nil {
		t.Fatalf("GET %s: building request: %v", path, err)
	}
	res, err := client.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	defer func() { _ = res.Body.Close() }()
	raw, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("GET %s: reading body: %v", path, err)
	}
	return res.StatusCode, string(raw), res.Header
}

// TestIntegration_TestConsoleToggle drives the console over real HTTP from
// the real composition root, in both switch positions, against a real
// ingested database. The unit suite covers the router in process; what this
// adds is the path config.UI.Enabled -> app.New -> httpx.Deps -> a live
// listener, which is the only path an operator ever uses.
func TestIntegration_TestConsoleToggle(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "hostus.sqlite")

	if _, err := app.Ingest(context.Background(), "testdata/dataset.yaml", dbPath); err != nil {
		t.Fatalf("app.Ingest: unexpected error: %v", err)
	}

	on := serveWithConsole(t, dbPath, true)
	off := serveWithConsole(t, dbPath, false)

	assertConsoleServed(t, on)
	assertConsoleCSP(t, on)
	assertConsoleDeepLink(t, on)
	assertConsoleAssets(t, on)
	assertConsoleAbsent(t, off)

	// A FRESH pair for the identity comparison, deliberately.
	//
	// The rate limiter is a single global 20 rps bucket shared by the
	// console and the API (the UI route sits inside the fixed middleware
	// chain — see httpx.NewRouter). The eight console requests above
	// therefore leave the UI-on server with a visibly emptier bucket than
	// the UI-off one, and the first run of this test proved it: probe 13
	// came back 429 on one server and 404 on the other. That asymmetry is
	// real behavior worth knowing about (a page load costs API budget),
	// but it is not what this assertion is about, and comparing two
	// servers with different bucket levels would be comparing the limiter,
	// not the API surface.
	assertAPIUnchangedAcrossToggle(t,
		serveWithConsole(t, dbPath, true),
		serveWithConsole(t, dbPath, false))
}

// assertConsoleServed: with the console on, "/" is the page.
func assertConsoleServed(t *testing.T, ts *httptest.Server) {
	t.Helper()
	status, body, header := getConsole(t, ts.Client(), ts.URL, "/")

	if status != http.StatusOK {
		t.Fatalf("GET / with the console on: got %d, want 200", status)
	}
	if ct := header.Get("Content-Type"); ct != "text/html; charset=utf-8" {
		t.Errorf("GET /: Content-Type %q, want text/html; charset=utf-8", ct)
	}
	if header.Get("ETag") == "" {
		t.Error("GET /: no ETag — a reload would re-transfer the whole document")
	}
	for _, panel := range []string{"panel-suggest", "panel-concept", "panel-match", "panel-translate"} {
		if !strings.Contains(body, `id="`+panel+`"`) {
			t.Errorf("GET /: served document lacks panel %q", panel)
		}
	}
}

// assertConsoleCSP: the header that makes the offline claim structural must
// survive the whole composition root, not just the handler.
func assertConsoleCSP(t *testing.T, ts *httptest.Server) {
	t.Helper()
	_, _, header := getConsole(t, ts.Client(), ts.URL, "/")

	csp := header.Get("Content-Security-Policy")
	if csp == "" {
		t.Fatal("GET /: no Content-Security-Policy header")
	}
	if !strings.Contains(csp, "default-src 'self'") {
		t.Errorf("CSP lacks default-src 'self': %q", csp)
	}
	if !strings.Contains(csp, "script-src 'sha256-") || !strings.Contains(csp, "style-src 'sha256-") {
		t.Errorf("CSP does not admit the inlined blocks by hash: %q", csp)
	}
	for _, forbidden := range []string{"http:", "https:", "*", "'unsafe-inline'", "'unsafe-eval'"} {
		if strings.Contains(csp, forbidden) {
			t.Errorf("CSP contains %q, which admits something it must not: %q", forbidden, csp)
		}
	}
}

// assertConsoleDeepLink: an unrouted path outside the API prefixes is an
// SPA deep link and serves the same document as "/".
func assertConsoleDeepLink(t *testing.T, ts *httptest.Server) {
	t.Helper()
	_, root, _ := getConsole(t, ts.Client(), ts.URL, "/")

	for _, path := range []string{"/konzept/wcvp-12345", "/suggest", "/deep/link"} {
		status, body, header := getConsole(t, ts.Client(), ts.URL, path)
		if status != http.StatusOK {
			t.Errorf("GET %s: got %d, want 200", path, status)
			continue
		}
		if !strings.HasPrefix(header.Get("Content-Type"), "text/html") {
			t.Errorf("GET %s: Content-Type %q, want text/html...", path, header.Get("Content-Type"))
		}
		if body != root {
			t.Errorf("GET %s: served something other than the console document", path)
		}
	}
}

// assertConsoleAssets: the individually addressable assets keep their own
// Content-Type. The page never fetches them — it inlines both — but they
// stay readable so a tester can see the exact JS the console runs.
func assertConsoleAssets(t *testing.T, ts *httptest.Server) {
	t.Helper()
	tests := []struct{ path, want string }{
		{"/assets/app.js", "text/javascript; charset=utf-8"},
		{"/assets/style.css", "text/css; charset=utf-8"},
	}
	for _, tc := range tests {
		status, body, header := getConsole(t, ts.Client(), ts.URL, tc.path)
		if status != http.StatusOK {
			t.Errorf("GET %s: got %d, want 200", tc.path, status)
			continue
		}
		if ct := header.Get("Content-Type"); ct != tc.want {
			t.Errorf("GET %s: Content-Type %q, want %q", tc.path, ct, tc.want)
		}
		if body == "" {
			t.Errorf("GET %s: empty body", tc.path)
		}
	}
}

// assertConsoleAbsent: with the console off the router registers NOTHING —
// not "/", not an asset, not a deep link. This is the path an operator
// relies on for a deployment and the one nobody exercises by hand.
func assertConsoleAbsent(t *testing.T, ts *httptest.Server) {
	t.Helper()
	for _, path := range []string{"/", "/index.html", "/assets/app.js", "/assets/style.css", "/konzept/wcvp-12345"} {
		status, _, _ := getConsole(t, ts.Client(), ts.URL, path)
		if status != http.StatusNotFound {
			t.Errorf("GET %s with the console off: got %d, want 404", path, status)
		}
	}
}

// assertAPIUnchangedAcrossToggle is the regression guard: turning the
// console on must not change the status, body or Content-Type of a single
// API response. Both servers run against the same ingested database, so any
// difference is the console's doing.
func assertAPIUnchangedAcrossToggle(t *testing.T, on, off *httptest.Server) {
	t.Helper()
	for _, p := range apiProbesAcrossToggle {
		statusOn, bodyOn, ctypeOn := doProbe(t, on.Client(), on.URL, p)
		statusOff, bodyOff, ctypeOff := doProbe(t, off.Client(), off.URL, p)

		if statusOn != statusOff {
			t.Errorf("%s %s: status %d with the console on vs %d with it off", p.method, p.path, statusOn, statusOff)
			continue
		}
		// /metrics is a live counter and differs between two processes by
		// construction; only its status and shape are comparable.
		if p.path == "/metrics" {
			continue
		}
		if bodyOn != bodyOff {
			t.Errorf("%s %s: body differs with the console on\n  on: %s\n off: %s", p.method, p.path, bodyOn, bodyOff)
		}
		if ctypeOn != ctypeOff {
			t.Errorf("%s %s: Content-Type %q with the console on vs %q with it off", p.method, p.path, ctypeOn, ctypeOff)
		}
	}
}

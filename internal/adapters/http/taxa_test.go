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
	"github.com/jobrunner/hostus/internal/adapters/wcvp"
	"github.com/jobrunner/hostus/internal/application"
	"github.com/jobrunner/hostus/internal/domain"

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
			ParentTaxonID:   t.ParentNameUsageID,
			BasionymTaxonID: t.OriginalNameUsageID,
			// PublishedIn/NomStatus mirror internal/app/ingest.go's real
			// mapping. They matter here because SP6's publication filter is
			// defined on nom_status: a test row source that dropped it would
			// make every fixture synonym look nomenclaturally clean.
			PublishedIn: t.PublishedIn,
			NomStatus:   t.NomenclaturalStatus,
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
	// application.Ingest does not build distribution_effective itself (the
	// production CLI wiring in internal/app/ingest.go does, once, after all
	// backbones are ingested); Suggest's in_area now reads that closure
	// table, so this shared fixture must build it explicitly.
	if err := db.BuildDistributionClosure(context.Background()); err != nil {
		t.Fatalf("BuildDistributionClosure: unexpected error: %v", err)
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

// assertXrefSlice checks xrefs[authority] equals want exactly (order
// included — conceptXrefs orders rows by (authority, ext_id), so the wire
// slice is always in that sorted order, never ingest/query order).
func assertXrefSlice(t *testing.T, xrefs map[string][]string, authority string, want []string) {
	t.Helper()
	got := xrefs[authority]
	if len(got) != len(want) {
		t.Fatalf("xrefs[%s] = %v, want %v", authority, got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("xrefs[%s] = %v, want %v", authority, got, want)
			return
		}
	}
}

// addXref writes one (authority, ext_id) cross-reference for
// corynephorusConceptID directly via the repository's IngestTx, the same
// write path application.IngestXrefs uses — a minimal way for a test to
// seed xrefs beyond what the WCVP fixture's own POWO id carries, without
// going through a full XrefRowSource/Wikidata-bridge round trip. Every
// caller in this file targets that one concept, so it is not parameterized
// on concept id.
func addXref(t *testing.T, db *sqlite.DB, authority, extID string) {
	t.Helper()
	tx, err := db.BeginTraitIngest(context.Background())
	if err != nil {
		t.Fatalf("BeginTraitIngest: %v", err)
	}
	if err := tx.AddXref(corynephorusConceptID, domain.Xref{Authority: authority, ExtID: extID}, ""); err != nil {
		t.Fatalf("AddXref(%s, %s:%s): %v", corynephorusConceptID, authority, extID, err)
	}
	if err := tx.Finalize(); err != nil {
		t.Fatalf("Finalize: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}
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
	ConceptID    string `json:"concept_id"`
	Display      string `json:"display"`
	Canonical    string `json:"canonical"`
	Rank         string `json:"rank"`
	RankVerbatim string `json:"rank_verbatim"`
	Status       string `json:"status"`
	Backbone     struct {
		ID      string `json:"id"`
		Version string `json:"version"`
	} `json:"backbone"`
	Xrefs       map[string][]string `json:"xrefs"`
	ParentChain []struct {
		ConceptID string `json:"concept_id"`
		Canonical string `json:"canonical"`
		Rank      string `json:"rank"`
	} `json:"parent_chain"`
	Synonyms []struct {
		Canonical  string `json:"canonical"`
		Authorship string `json:"authorship"`
		Homotypic  *bool  `json:"homotypic"`
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
	Homotypic  *bool  `json:"homotypic"`
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
	assertXrefSlice(t, got.Xrefs, "powo", []string{"396681-1"})
	if !hasSynonym(got.Synonyms, "Weingaertneria") {
		t.Errorf("synonyms = %+v, want an entry starting with %q", got.Synonyms, "Weingaertneria")
	}
	assertCorynephorusCanescensDistribution(t, got)
	assertCorynephorusCanescensClassification(t, got)
}

// assertCorynephorusCanescensDistribution checks the WCVP fixture's
// Corynephorus canescens (405825) carries its nine WGSRPD-L3 distribution
// rows (see wcvp_distribution.csv) on the wire, so /v1/concept doesn't
// silently drop distribution (spec §B.1/§4.3). Split out of
// TestHandleConcept_KnownID_ReturnsConcept purely to keep that test's
// cyclomatic complexity down.
func assertCorynephorusCanescensDistribution(t *testing.T, got conceptResponse) {
	t.Helper()
	if len(got.Distribution) != 9 {
		t.Fatalf("len(distribution) = %d, want %d", len(got.Distribution), 9)
	}
	if !hasDistributionArea(got.Distribution, "wgsrpd_l3", "AUT") {
		t.Errorf("distribution = %+v, want an entry {wgsrpd_l3 AUT}", got.Distribution)
	}
}

// assertCorynephorusCanescensClassification checks the fixture's
// Corynephorus canescens (405825, parentnameusageid 451295) renders exactly
// one classification entry: the Corynephorus genus (451295), itself
// accepted in the same fixture and with no further ancestor of its own.
func assertCorynephorusCanescensClassification(t *testing.T, got conceptResponse) {
	t.Helper()
	if len(got.ParentChain) != 1 {
		t.Fatalf("len(parent_chain) = %d, want 1 (the Corynephorus genus ancestor)", len(got.ParentChain))
	}
	if got.ParentChain[0].ConceptID != "wcvp:concept:451295" || got.ParentChain[0].Canonical != "Corynephorus" || got.ParentChain[0].Rank != "GENUS" {
		t.Errorf("parent_chain[0] = %+v, want {wcvp:concept:451295 Corynephorus GENUS}", got.ParentChain[0])
	}
}

// TestHandleConcept_HomotypicSynonymRendersTrueOthersOmitted proves
// synonymDTO.Homotypic renders exactly what T7's ingest homotypic rule
// proved, on the real wire response: Bromus ovinus (a synonym of Festuca
// ovina, 415853) is a recombination of the accepted name itself
// (originalnameusageid=415853), so it renders homotypic:true; Festuca
// duriuscula (also a synonym of Festuca ovina, no basionym linkage at all)
// must have its "homotypic" key OMITTED from the JSON — never rendered as
// a literal false.
func TestHandleConcept_HomotypicSynonymRendersTrueOthersOmitted(t *testing.T) {
	repo := seededRepo(t)
	r := httpx.NewRouter(httpx.Deps{Repo: repo})

	const festucaOvinaConceptID = "wcvp:concept:415853"
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/v1/concept/"+festucaOvinaConceptID, nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rr.Code, rr.Body.String())
	}

	var raw struct {
		Synonyms []map[string]any `json:"synonyms"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &raw); err != nil {
		t.Fatalf("decoding raw JSON: %v (body: %s)", err, rr.Body.String())
	}

	var bromus, duriuscula map[string]any
	for _, s := range raw.Synonyms {
		switch s["canonical"] {
		case "Bromus ovinus":
			bromus = s
		case "Festuca duriuscula":
			duriuscula = s
		}
	}
	if bromus == nil {
		t.Fatalf("synonyms = %+v, want an entry for %q", raw.Synonyms, "Bromus ovinus")
	}
	if h, ok := bromus["homotypic"].(bool); !ok || !h {
		t.Errorf("Bromus ovinus[\"homotypic\"] = %v (present=%v), want true", bromus["homotypic"], ok)
	}
	if duriuscula == nil {
		t.Fatalf("synonyms = %+v, want an entry for %q", raw.Synonyms, "Festuca duriuscula")
	}
	if _, present := duriuscula["homotypic"]; present {
		t.Errorf("Festuca duriuscula[\"homotypic\"] = %v, want the key OMITTED entirely (unproven, never a literal false)", duriuscula["homotypic"])
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

// TestHandleConcept_MultipleXrefsSameAuthority_RendersSlice proves the
// documented fix for the Task 2 defect: a concept legitimately carrying
// TWO distinct ext_ids for the same authority (measured on the real index:
// e.g. powo join id 44903-1 has iNat ids 486076 AND 556571 — see
// task-3-brief.md) must render xrefs.inat as a SORTED slice of BOTH ids,
// never silently keep only the last one written by a map[string]string.
func TestHandleConcept_MultipleXrefsSameAuthority_RendersSlice(t *testing.T) {
	repo := seededRepo(t)
	addXref(t, repo, "inat", "556571")
	addXref(t, repo, "inat", "486076")
	r := httpx.NewRouter(httpx.Deps{Repo: repo})

	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/v1/concept/"+corynephorusConceptID, nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rr.Code, rr.Body.String())
	}

	got := decodeJSON[conceptResponse](t, rr.Body)
	// conceptXrefs orders by (authority, ext_id) — "486076" < "556571"
	// lexically — so the slice must come back in that order regardless of
	// the write order above.
	assertXrefSlice(t, got.Xrefs, "inat", []string{"486076", "556571"})
	// The pre-existing single-valued powo xref must still render as a
	// one-element slice, unchanged in substance by the shape change.
	assertXrefSlice(t, got.Xrefs, "powo", []string{"396681-1"})
}

// TestHandleXref_ResolvesEveryEnrichedAuthority proves GET /v1/xref now
// resolves every authority SP4's Wikidata-bridge ingest can populate, not
// just the pre-existing powo join key — one subtest per authority listed
// in task-3-brief.md/T2's measured coverage.
func TestHandleXref_ResolvesEveryEnrichedAuthority(t *testing.T) {
	authorities := []string{"wikidata", "gbif", "wfo", "colxr", "inat", "floraveg", "euromed"}
	for _, authority := range authorities {
		t.Run(authority, func(t *testing.T) {
			repo := seededRepo(t)
			extID := "test-" + authority + "-id"
			addXref(t, repo, authority, extID)
			r := httpx.NewRouter(httpx.Deps{Repo: repo})

			rr := httptest.NewRecorder()
			r.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/v1/xref?authority="+authority+"&id="+extID, nil))
			if rr.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200 (body: %s)", rr.Code, rr.Body.String())
			}
			assertJSONContentType(t, rr)

			got := decodeJSON[conceptResponse](t, rr.Body)
			if got.ConceptID != corynephorusConceptID {
				t.Errorf("concept_id = %q, want %q", got.ConceptID, corynephorusConceptID)
			}
			assertXrefSlice(t, got.Xrefs, authority, []string{extID})
		})
	}
}

// TestHandleConcept_UC2Shape_InatXrefReachesAcceptedConcept proves UC2's
// concrete requirement (spec §UC2): the response must carry the ACCEPTED
// concept's inat id (never a synonym's — hostus concepts are already
// accepted-keyed, synonyms group under them, so there is no separate
// synonym-keyed inat id to confuse this with), reachable at xrefs.inat[0],
// giving a client everything it needs to link to iNaturalist observations.
func TestHandleConcept_UC2Shape_InatXrefReachesAcceptedConcept(t *testing.T) {
	repo := seededRepo(t)
	addXref(t, repo, "inat", "486076")
	r := httpx.NewRouter(httpx.Deps{Repo: repo})

	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/v1/concept/"+corynephorusConceptID, nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rr.Code, rr.Body.String())
	}

	got := decodeJSON[conceptResponse](t, rr.Body)
	if got.Status != "ACCEPTED" {
		t.Fatalf("status = %q, want ACCEPTED (UC2 requires the accepted concept's inat id)", got.Status)
	}
	inatIDs := got.Xrefs["inat"]
	if len(inatIDs) != 1 || inatIDs[0] != "486076" {
		t.Errorf("xrefs[inat] = %v, want [486076]", inatIDs)
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
		Classification *struct {
			Family string `json:"family"`
			Order  string `json:"order"`
			Class  string `json:"class"`
		} `json:"classification"`
		AggregateResolution *struct {
			RequestedNameSpace string `json:"requested_name_space"`
			Status             string `json:"status"`
			MemberCount        int    `json:"member_count"`
			Options            []struct {
				NameSpace          string `json:"name_space"`
				Status             string `json:"status"`
				AggregateConceptID string `json:"aggregate_concept_id"`
				MemberCount        int    `json:"member_count"`
			} `json:"options"`
			Agreement string `json:"agreement"`
		} `json:"aggregate_resolution"`
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

// TestHandleMatch_ClassificationAndAggregateResolution exercises Task 10's
// two new /v1/match fields: `classification` (present, unconditional on
// target_space, once a concept resolves and carries one) and
// `aggregate_resolution` (present only for an aggregate/collective-rank
// hit, with one option per known name space and an `agreement` once both
// eurosl and germansl resolve `known`).
func TestHandleMatch_ClassificationAndAggregateResolution(t *testing.T) {
	repo := seededRepo(t)

	// Corynephorus canescens gets a classification via the same
	// UpsertClassification path a real crosswalk ingest would use.
	tx, err := repo.BeginIngest(context.Background(), domain.BackboneVersion{ID: "wcvp", Version: "2026-06-15"})
	if err != nil {
		t.Fatalf("BeginIngest: unexpected error: %v", err)
	}
	if err := tx.UpsertClassification(corynephorusConceptID, "Poaceae", "Poales", "Liliopsida"); err != nil {
		t.Fatalf("UpsertClassification: unexpected error: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: unexpected error: %v", err)
	}

	// A native SPECIES_AGGREGATE concept for "Salsola kali", known in BOTH
	// eurosl and germansl with the same member, so ComputeConceptAgreement
	// reports "identical" and both options resolve Known.
	seedNativeAggregateForHTTPTest(t, repo, "eurosl:concept:salsola-kali-agg", "Salsola kali agg.", []string{"wcvp:concept:salsola-kali-1"})
	seedNativeAggregateForHTTPTest(t, repo, "germansl:concept:salsola-kali-agg", "Salsola kali s.l.", []string{"wcvp:concept:salsola-kali-1"})
	report, err := application.ComputeConceptAgreement(context.Background(), repo)
	if err != nil {
		t.Fatalf("ComputeConceptAgreement: unexpected error: %v", err)
	}
	if err := repo.WriteConceptAgreement(context.Background(), report.Pairs); err != nil {
		t.Fatalf("WriteConceptAgreement: unexpected error: %v", err)
	}

	r := httpx.NewRouter(httpx.Deps{Repo: repo})
	body := `{
		"names": [
			{"id": "1", "verbatim": "Corynephorus canescens"},
			{"id": "2", "verbatim": "Salsola kali agg."}
		]
	}`
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/match", bytes.NewBufferString(body))
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rr.Code, rr.Body.String())
	}
	got := decodeJSON[matchResponse](t, rr.Body)
	byID := map[string]int{}
	for i, res := range got.Results {
		byID[res.ID] = i
	}

	coryn := got.Results[byID["1"]]
	if coryn.Classification == nil {
		t.Fatal("result 1: classification = nil, want non-nil")
	}
	if coryn.Classification.Family != "Poaceae" {
		t.Errorf("result 1: classification.family = %q, want %q", coryn.Classification.Family, "Poaceae")
	}
	if coryn.AggregateResolution != nil {
		t.Errorf("result 1: aggregate_resolution = %+v, want nil (not an aggregate)", coryn.AggregateResolution)
	}

	salsola := got.Results[byID["2"]]
	if salsola.AggregateResolution == nil {
		t.Fatal("result 2: aggregate_resolution = nil, want non-nil for an aggregate match")
	}
	if len(salsola.AggregateResolution.Options) != 3 {
		t.Fatalf("result 2: len(options) = %d, want 3", len(salsola.AggregateResolution.Options))
	}
	if salsola.AggregateResolution.Agreement != "identical" {
		t.Errorf("result 2: agreement = %q, want %q", salsola.AggregateResolution.Agreement, "identical")
	}
	if salsola.AggregateResolution.RequestedNameSpace != "eurosl" {
		t.Errorf("result 2: requested_name_space = %q, want %q", salsola.AggregateResolution.RequestedNameSpace, "eurosl")
	}
}

// seedNativeAggregateForHTTPTest writes aggregateConceptID as a native
// SPECIES_AGGREGATE concept (canonical name) under its own backbone
// (derived from the id's "<backbone>:concept:<id>" shape) and links it to
// memberConceptIDs via concept_aggregate, creating each member as a minimal
// SPECIES concept first — mirrors internal/application/
// concept_agreement_test.go's seedAggregateWithMembers, duplicated here
// rather than exported since it is test-only fixture setup.
func seedNativeAggregateForHTTPTest(t *testing.T, repo *sqlite.DB, aggregateConceptID, canonical string, memberConceptIDs []string) {
	t.Helper()
	for _, m := range memberConceptIDs {
		seedMemberConceptForHTTPTest(t, repo, m)
	}

	backbone, sourceID, ok := splitConceptID(aggregateConceptID)
	if !ok {
		t.Fatalf("seedNativeAggregateForHTTPTest: %q is not a <backbone>:concept:<id> shape", aggregateConceptID)
	}
	tx, err := repo.BeginIngest(context.Background(), domain.BackboneVersion{ID: backbone, Version: "test", Redistribution: domain.RedistributionUnknown})
	if err != nil {
		t.Fatalf("BeginIngest(%q): unexpected error: %v", backbone, err)
	}
	name := domain.Name{ID: aggregateConceptID + ":name:" + sourceID, Canonical: canonical, Rank: domain.RankSpeciesAggregate}
	concept := domain.Concept{ID: aggregateConceptID, BackboneID: backbone, AcceptedName: name, Rank: domain.RankSpeciesAggregate, Status: domain.StatusAccepted}
	if err := tx.UpsertName(name); err != nil {
		t.Fatalf("UpsertName: unexpected error: %v", err)
	}
	if err := tx.UpsertConcept(concept); err != nil {
		t.Fatalf("UpsertConcept: unexpected error: %v", err)
	}
	if err := tx.LinkName(concept.ID, name.ID, "accepted", nil); err != nil {
		t.Fatalf("LinkName: unexpected error: %v", err)
	}
	for _, m := range memberConceptIDs {
		if err := tx.AddAggregateMember(aggregateConceptID, m); err != nil {
			t.Fatalf("AddAggregateMember(%q, %q): unexpected error: %v", aggregateConceptID, m, err)
		}
	}
	if err := tx.Finalize(); err != nil {
		t.Fatalf("Finalize: unexpected error: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: unexpected error: %v", err)
	}
}

// seedMemberConceptForHTTPTest writes memberConceptID as a minimal
// SPECIES-rank concept under its own backbone (derived from the id's
// "<backbone>:concept:<id>" shape) — the member half of
// seedNativeAggregateForHTTPTest, split out to keep both functions' own
// cognitive complexity low.
func seedMemberConceptForHTTPTest(t *testing.T, repo *sqlite.DB, memberConceptID string) {
	t.Helper()
	backbone, sourceID, ok := splitConceptID(memberConceptID)
	if !ok {
		t.Fatalf("seedMemberConceptForHTTPTest: %q is not a <backbone>:concept:<id> shape", memberConceptID)
	}
	tx, err := repo.BeginIngest(context.Background(), domain.BackboneVersion{ID: backbone, Version: "test", Redistribution: domain.RedistributionUnknown})
	if err != nil {
		t.Fatalf("BeginIngest(%q): unexpected error: %v", backbone, err)
	}
	name := domain.Name{ID: memberConceptID + ":name", Canonical: "Member " + sourceID, Rank: domain.RankSpecies}
	concept := domain.Concept{ID: memberConceptID, BackboneID: backbone, AcceptedName: name, Rank: domain.RankSpecies, Status: domain.StatusAccepted}
	if err := tx.UpsertName(name); err != nil {
		t.Fatalf("UpsertName: unexpected error: %v", err)
	}
	if err := tx.UpsertConcept(concept); err != nil {
		t.Fatalf("UpsertConcept: unexpected error: %v", err)
	}
	if err := tx.LinkName(concept.ID, name.ID, "accepted", nil); err != nil {
		t.Fatalf("LinkName: unexpected error: %v", err)
	}
	if err := tx.Finalize(); err != nil {
		t.Fatalf("Finalize: unexpected error: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: unexpected error: %v", err)
	}
}

// splitConceptID splits a "<backbone>:concept:<id>" string into its
// backbone and source-id parts.
func splitConceptID(conceptID string) (backbone, sourceID string, ok bool) {
	return strings.Cut(conceptID, ":concept:")
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

// sliceRowSource is a minimal application.RowSource backed by an in-memory
// slice, for tests that need specific TaxonRows (e.g. an exotic rank) that
// the shared WCVP fixture doesn't happen to carry.
type sliceRowSource struct{ taxa []application.TaxonRow }

func (s sliceRowSource) Taxa() []application.TaxonRow                 { return s.taxa }
func (s sliceRowSource) Distributions() []application.DistributionRow { return nil }

// otherRankRepo ingests one ordinary "Species" concept and one "lusus"
// concept (WCVP's real exotic rank that made hostus 2.0's full ingest
// abort — see docs/research/reality-check.md's M1.0) into a fresh
// in-memory repo, so /v1/concept's rank_verbatim rendering can be tested
// against both a canonical and an OTHER-ranked concept without depending
// on the shared WCVP fixture carrying an exotic rank itself.
func otherRankRepo(t *testing.T) *sqlite.DB {
	t.Helper()
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("sqlite.Open(:memory:): unexpected error: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	ds := &application.Dataset{Backbones: []application.Backbone{{ID: "wcvp-other", Version: "v1"}}, ManifestSHA: "x"}
	taxa := []application.TaxonRow{
		{TaxonID: "1", AcceptedTaxonID: "1", Accepted: true, Canonical: "Ordinary species", Rank: "Species", Status: "Accepted"},
		{TaxonID: "2", AcceptedTaxonID: "2", Accepted: true, Canonical: "Paeonia corallina lusus ovatifolia", Authorship: "Rouy & Foucaud", Rank: "lusus", Status: "Synonym"},
	}
	readerFor := func(application.Backbone) (application.RowSource, error) {
		return sliceRowSource{taxa: taxa}, nil
	}
	if _, err := application.Ingest(context.Background(), ds, readerFor, db); err != nil {
		t.Fatalf("application.Ingest: unexpected error: %v", err)
	}
	return db
}

// TestHandleConcept_OtherRank_RendersRankVerbatim proves Hardening Task 1's
// fix-round-1 requirement end to end: a concept whose rank degraded to
// domain.RankOther (WCVP's "lusus") must render BOTH `"rank":"OTHER"` and
// `"rank_verbatim":"lusus"` on the wire — the whole point of persisting
// RankVerbatim through the ingest is so a nomenclature service doesn't
// forget which exotic rank a concept actually had (spec §A.1) — while a
// canonically-ranked concept must OMIT the "rank_verbatim" key entirely
// (checked on the raw JSON, like the existing niche_width/homotypic
// omitempty tests), never render it as an empty string.
func TestHandleConcept_OtherRank_RendersRankVerbatim(t *testing.T) {
	repo := otherRankRepo(t)
	r := httpx.NewRouter(httpx.Deps{Repo: repo})

	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/v1/concept/wcvp-other:concept:2", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rr.Code, rr.Body.String())
	}

	got := decodeJSON[conceptResponse](t, bytes.NewBuffer(rr.Body.Bytes()))
	if got.Rank != "OTHER" {
		t.Errorf("rank = %q, want %q", got.Rank, "OTHER")
	}
	if got.RankVerbatim != "lusus" {
		t.Errorf("rank_verbatim = %q, want %q", got.RankVerbatim, "lusus")
	}

	var raw map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &raw); err != nil {
		t.Fatalf("decoding raw JSON: %v (body: %s)", err, rr.Body.String())
	}
	if _, present := raw["rank_verbatim"]; !present {
		t.Errorf("raw JSON = %s, want a \"rank_verbatim\" key present for an OTHER-ranked concept", rr.Body.String())
	}

	rr2 := httptest.NewRecorder()
	r.ServeHTTP(rr2, httptest.NewRequest(http.MethodGet, "/v1/concept/wcvp-other:concept:1", nil))
	if rr2.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rr2.Code, rr2.Body.String())
	}

	ordinary := decodeJSON[conceptResponse](t, bytes.NewBuffer(rr2.Body.Bytes()))
	if ordinary.Rank != "SPECIES" {
		t.Errorf("rank = %q, want %q", ordinary.Rank, "SPECIES")
	}

	var rawOrdinary map[string]any
	if err := json.Unmarshal(rr2.Body.Bytes(), &rawOrdinary); err != nil {
		t.Fatalf("decoding raw JSON: %v (body: %s)", err, rr2.Body.String())
	}
	if _, present := rawOrdinary["rank_verbatim"]; present {
		t.Errorf("raw JSON = %s, want the \"rank_verbatim\" key OMITTED entirely for a canonically-ranked concept", rr2.Body.String())
	}
}

// seedAggregateFixture writes a minimal, valid fixture directly via the low
// level IngestTx API (mirroring internal/adapters/sqlite's
// namespace_agreement_internal_test.go's seedTestConcept): a WCVP species
// concept (Salsola kali) plus its eurosl Fall-B aggregate (Salsola kali
// aggr.), linked via concept_aggregate — Task 6's shape — with a
// name_space_entry marking the WCVP concept's own eurosl spelling as an
// aggregate alias, so both members[] (on the aggregate) and
// aggregate_memberships[] (on the species) have real data to render.
func seedAggregateFixture(t *testing.T, db *sqlite.DB) {
	t.Helper()
	seedAggregateFixtureWCVP(t, db)
	seedAggregateFixtureEurosl(t, db)
}

// seedAggregateFixtureWCVP writes the fixture's WCVP half: the Salsola kali
// species concept plus its eurosl NameSpaceEntry flagged Aggregate == true
// (the Fall-A back-reference aggregate_memberships[] renders from).
func seedAggregateFixtureWCVP(t *testing.T, db *sqlite.DB) {
	t.Helper()
	ctx := context.Background()
	tx, err := db.BeginIngest(ctx, domain.BackboneVersion{ID: "wcvp", Version: "test", Redistribution: domain.RedistributionUnknown})
	if err != nil {
		t.Fatalf("BeginIngest(wcvp): unexpected error: %v", err)
	}
	speciesName := domain.Name{ID: "wcvp:name:sp1", Canonical: "Salsola kali", Rank: domain.RankSpecies}
	species := domain.Concept{ID: "wcvp:concept:sp1", BackboneID: "wcvp", AcceptedName: speciesName, Rank: domain.RankSpecies, Status: domain.StatusAccepted}
	mustSeed(t, "UpsertName(species)", tx.UpsertName(speciesName))
	mustSeed(t, "UpsertConcept(species)", tx.UpsertConcept(species))
	mustSeed(t, "LinkName(species)", tx.LinkName(species.ID, speciesName.ID, "accepted", nil))
	mustSeed(t, "UpsertNameSpace(eurosl)", tx.UpsertNameSpace(domain.NameSpaceMeta{ID: "eurosl", Version: "test", Redistribution: domain.RedistributionUnknown}))
	mustSeed(t, "AddNameSpaceEntry", tx.AddNameSpaceEntry(species.ID, domain.NameSpaceEntry{
		Space: "eurosl", ExtID: "agg1", Name: "Salsola kali aggr.", Aggregate: true, Status: "accepted",
	}))
	mustSeed(t, "Finalize(wcvp)", tx.Finalize())
	mustSeed(t, "Commit(wcvp)", tx.Commit())
}

// seedAggregateFixtureEurosl writes the fixture's eurosl half: the Fall-B
// aggregate concept (Salsola kali aggr.), linked to the WCVP species via
// concept_aggregate (Task 6's shape) — the members[] source.
func seedAggregateFixtureEurosl(t *testing.T, db *sqlite.DB) {
	t.Helper()
	ctx := context.Background()
	tx, err := db.BeginIngest(ctx, domain.BackboneVersion{ID: "eurosl", Version: "test", Redistribution: domain.RedistributionUnknown})
	if err != nil {
		t.Fatalf("BeginIngest(eurosl): unexpected error: %v", err)
	}
	aggName := domain.Name{ID: "eurosl:name:agg1", Canonical: "Salsola kali aggr.", Rank: domain.RankSpeciesAggregate}
	agg := domain.Concept{ID: "eurosl:concept:agg1", BackboneID: "eurosl", AcceptedName: aggName, Rank: domain.RankSpeciesAggregate, Status: domain.StatusAccepted}
	mustSeed(t, "UpsertName(agg)", tx.UpsertName(aggName))
	mustSeed(t, "UpsertConcept(agg)", tx.UpsertConcept(agg))
	mustSeed(t, "LinkName(agg)", tx.LinkName(agg.ID, aggName.ID, "accepted", nil))
	mustSeed(t, "AddAggregateMember", tx.AddAggregateMember(agg.ID, "wcvp:concept:sp1"))
	mustSeed(t, "Finalize(eurosl)", tx.Finalize())
	mustSeed(t, "Commit(eurosl)", tx.Commit())
}

// mustSeed fails the test immediately if a fixture-seeding step errored,
// naming which step failed — the shared assertion seedAggregateFixture's two
// halves use for every IngestTx call.
func mustSeed(t *testing.T, step string, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("%s: unexpected error: %v", step, err)
	}
}

// seededFullRepo opens a fresh in-memory sqlite repo and seeds it with
// seedAggregateFixture's Salsola kali (WCVP) / Salsola kali aggr. (eurosl)
// pair — the shared fixture for this file's classification/aggregate
// response tests.
func seededFullRepo(t *testing.T) *sqlite.DB {
	t.Helper()
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("sqlite.Open(:memory:): unexpected error: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	seedAggregateFixture(t, db)
	return db
}

// TestHandleConcept_IncludesClassificationAndAggregateMembers is the Task 9
// brief's Step 1 test: a Fall-B aggregate concept (rank SPECIES_AGGREGATE)
// must render members[] naming its one WCVP member.
func TestHandleConcept_IncludesClassificationAndAggregateMembers(t *testing.T) {
	repo := seededFullRepo(t)
	r := httpx.NewRouter(httpx.Deps{Repo: repo})

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/v1/concept/eurosl:concept:agg1", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body: %s", w.Code, w.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	members, ok := body["members"].([]any)
	if !ok || len(members) != 1 {
		t.Fatalf("body[\"members\"] = %v, want 1 entry", body["members"])
	}
	member, ok := members[0].(map[string]any)
	if !ok || member["concept_id"] != "wcvp:concept:sp1" || member["name"] != "Salsola kali" {
		t.Fatalf("members[0] = %v, want {concept_id: wcvp:concept:sp1, name: Salsola kali}", members[0])
	}
}

// TestHandleConcept_SpeciesRendersAggregateMemberships proves the Fall-A
// back-reference: a SPECIES concept with an eurosl NameSpaceEntry flagged
// Aggregate == true renders aggregate_memberships[], including the resolved
// aggregate_concept_id (via Repository.AggregatesByMember).
func TestHandleConcept_SpeciesRendersAggregateMemberships(t *testing.T) {
	repo := seededFullRepo(t)
	r := httpx.NewRouter(httpx.Deps{Repo: repo})

	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/v1/concept/wcvp:concept:sp1", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body: %s", rr.Code, rr.Body.String())
	}

	var body map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	memberships, ok := body["aggregate_memberships"].([]any)
	if !ok || len(memberships) != 1 {
		t.Fatalf("body[\"aggregate_memberships\"] = %v, want 1 entry", body["aggregate_memberships"])
	}
	m, ok := memberships[0].(map[string]any)
	if !ok || m["name_space"] != "eurosl" || m["aggregate_name"] != "Salsola kali aggr." || m["aggregate_concept_id"] != "eurosl:concept:agg1" {
		t.Fatalf("aggregate_memberships[0] = %v, want {name_space: eurosl, aggregate_name: Salsola kali aggr., aggregate_concept_id: eurosl:concept:agg1}", memberships[0])
	}
}

// TestHandleConcept_ClassificationObjectRendersFamilyOrderClass proves the
// NEW "classification" object (spec §4, distinct from "parent_chain") is
// populated from domain.Concept.Family/OrderName/ClassName and omitted
// entirely when all three are unknown.
func TestHandleConcept_ClassificationObjectRendersFamilyOrderClass(t *testing.T) {
	repo := seededFullRepo(t)
	ctx := context.Background()
	tx, err := repo.BeginTraitIngest(ctx)
	if err != nil {
		t.Fatalf("BeginTraitIngest: unexpected error: %v", err)
	}
	if err := tx.UpsertClassification("wcvp:concept:sp1", "Chenopodiaceae", "Caryophyllales", "Magnoliopsida"); err != nil {
		t.Fatalf("UpsertClassification: unexpected error: %v", err)
	}
	if err := tx.Finalize(); err != nil {
		t.Fatalf("Finalize: unexpected error: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: unexpected error: %v", err)
	}

	r := httpx.NewRouter(httpx.Deps{Repo: repo})

	rrWith := httptest.NewRecorder()
	r.ServeHTTP(rrWith, httptest.NewRequest(http.MethodGet, "/v1/concept/wcvp:concept:sp1", nil))
	var withClassification map[string]any
	if err := json.Unmarshal(rrWith.Body.Bytes(), &withClassification); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	classification, ok := withClassification["classification"].(map[string]any)
	if !ok || classification["family"] != "Chenopodiaceae" || classification["order"] != "Caryophyllales" || classification["class"] != "Magnoliopsida" {
		t.Fatalf("classification = %v, want {family: Chenopodiaceae, order: Caryophyllales, class: Magnoliopsida}", withClassification["classification"])
	}

	rrWithout := httptest.NewRecorder()
	r.ServeHTTP(rrWithout, httptest.NewRequest(http.MethodGet, "/v1/concept/eurosl:concept:agg1", nil))
	var withoutClassification map[string]any
	if err := json.Unmarshal(rrWithout.Body.Bytes(), &withoutClassification); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, present := withoutClassification["classification"]; present {
		t.Errorf("classification = %v, want the key OMITTED for a concept with no family/order/class", withoutClassification["classification"])
	}
}

// TestHandleConcept_VernacularNamesRenderWithHardcodedSource proves
// vernacular_names[] renders every ingested vernacular row with source
// hardcoded to "germansl" (Task 9's ruling — see vernacularNameDTO's doc
// comment).
func TestHandleConcept_VernacularNamesRenderWithHardcodedSource(t *testing.T) {
	repo := seededFullRepo(t)
	ctx := context.Background()
	tx, err := repo.BeginTraitIngest(ctx)
	if err != nil {
		t.Fatalf("BeginTraitIngest: unexpected error: %v", err)
	}
	if err := tx.AddVernacularName("wcvp:concept:sp1", domain.VernacularName{Language: "de", Name: "Kali-Salzkraut"}); err != nil {
		t.Fatalf("AddVernacularName: unexpected error: %v", err)
	}
	if err := tx.Finalize(); err != nil {
		t.Fatalf("Finalize: unexpected error: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: unexpected error: %v", err)
	}

	r := httpx.NewRouter(httpx.Deps{Repo: repo})
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/v1/concept/wcvp:concept:sp1", nil))

	var body map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	names, ok := body["vernacular_names"].([]any)
	if !ok || len(names) != 1 {
		t.Fatalf("body[\"vernacular_names\"] = %v, want 1 entry", body["vernacular_names"])
	}
	name, ok := names[0].(map[string]any)
	if !ok || name["language"] != "de" || name["name"] != "Kali-Salzkraut" || name["source"] != "germansl" {
		t.Fatalf("vernacular_names[0] = %v, want {language: de, name: Kali-Salzkraut, source: germansl}", names[0])
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

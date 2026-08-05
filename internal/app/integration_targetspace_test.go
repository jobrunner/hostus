//go:build integration

package app_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/jobrunner/hostus/internal/app"
	"github.com/jobrunner/hostus/internal/domain"
)

// targetSpaceMatchResponse mirrors internal/adapters/http.matchResponseDTO
// including the three SP9/UC4 fields, so the e2e can assert them over real
// HTTP (the SP1 integrationMatchResponse deliberately omits them).
type targetSpaceMatchResult struct {
	ID                     string `json:"id"`
	MatchType              string `json:"match_type"`
	ConceptID              string `json:"concept_id"`
	TargetSpaceName        string `json:"target_space_name"`
	AggregatePolicy        string `json:"aggregate_policy"`
	ESyDiagnosticRelevance string `json:"esy_diagnostic_relevance"`
}

type targetSpaceMatchResponse struct {
	BackboneVersions map[string]string        `json:"backbone_versions"`
	Results          []targetSpaceMatchResult `json:"results"`
}

// attachFloraVegEntry attaches ONE floraveg spelling to conceptID through the
// real ingest tx. It is used to place a floraveg AGGREGATE spelling on the
// concept an aggregate query actually resolves to — the `known` precondition
// the source document's Festuca ovina example assumes (a backbone that carries
// the aggregate as a taxon of its own). The floraveg name space itself is
// already ingested by testdata/dataset.yaml; UpsertNameSpace here is an
// idempotent INSERT OR REPLACE of that same provenance row.
func attachFloraVegEntry(t *testing.T, a *app.App, conceptID string, e domain.NameSpaceEntry) {
	t.Helper()
	tx, err := a.Repo.BeginTraitIngest(context.Background())
	if err != nil {
		t.Fatalf("BeginTraitIngest: %v", err)
	}
	if err := tx.UpsertNameSpace(domain.NameSpaceMeta{
		ID: "floraveg", Version: "2023-01-03", ManifestSHA: "x",
		Redistribution: domain.RedistributionUnknown,
	}); err != nil {
		t.Fatalf("UpsertNameSpace: %v", err)
	}
	if err := tx.AddNameSpaceEntry(conceptID, e); err != nil {
		t.Fatalf("AddNameSpaceEntry: %v", err)
	}
	if err := tx.Finalize(); err != nil {
		t.Fatalf("Finalize: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}
}

// seedRumexAcetosellaAggregate seeds a SECOND aggregate concept, deliberately
// WITHOUT any floraveg entry, so an aggregate query resolving onto it exercises
// the "unresolvable" branch — the case the source document calls the most
// important and the easiest to miss.
func seedRumexAcetosellaAggregate(t *testing.T, a *app.App) string {
	t.Helper()
	ctx := context.Background()
	tx, err := a.Repo.BeginIngest(ctx, domain.BackboneVersion{ID: "test-agg2", Version: "v1"})
	if err != nil {
		t.Fatalf("BeginIngest: %v", err)
	}
	name := domain.Name{ID: "test-agg2:name:rumex-acetosella-agg", Canonical: "Rumex acetosella agg.", Rank: domain.RankSpecies}
	concept := domain.Concept{ID: "test-agg2:concept:rumex-acetosella-agg", BackboneID: "test-agg2", AcceptedName: name, Rank: domain.RankSpecies, Status: domain.StatusAccepted}
	if err := tx.UpsertName(name); err != nil {
		t.Fatalf("UpsertName: %v", err)
	}
	if err := tx.UpsertConcept(concept); err != nil {
		t.Fatalf("UpsertConcept: %v", err)
	}
	if err := tx.LinkName(concept.ID, name.ID, "accepted", nil); err != nil {
		t.Fatalf("LinkName: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	return concept.ID
}

// TestIntegration_MatchTargetSpaceFloraVeg drives the buildable half of UC4 end
// to end over real HTTP: ingest WCVP + FloraVeg (testdata/dataset.yaml), then
// POST /v1/match with target_space=floraveg over a small relevé, asserting the
// SPECIFIC names and policies each of the three tri-state branches produces —
// not just a 200.
//
//   - "Festuca ovina agg." resolves onto an aggregate concept carrying a
//     floraveg aggregate spelling            -> known, "Festuca ovina aggr."
//   - "Festuca ovina" (plain) resolves onto the WCVP nominate whose floraveg
//     entries were crosswalked by the real ingest -> NO policy, "Festuca ovina"
//   - "Rumex acetosella agg." resolves onto an aggregate concept with no
//     floraveg entry                          -> unresolvable, no name
//
// and esy_diagnostic_relevance is the conspicuous sentinel on EVERY result,
// resolved or not.
func TestIntegration_MatchTargetSpaceFloraVeg(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "hostus.sqlite")
	ctx := context.Background()
	if _, err := app.Ingest(ctx, "testdata/dataset.yaml", dbPath); err != nil {
		t.Fatalf("app.Ingest: %v", err)
	}

	cfg := testConfig()
	cfg.SQLite.Path = dbPath
	a, err := app.New(cfg)
	if err != nil {
		t.Fatalf("app.New: %v", err)
	}
	t.Cleanup(func() { _ = a.Shutdown(context.Background()) })

	aggConceptID := seedFestucaOvinaAggregate(t, a)
	attachFloraVegEntry(t, a, aggConceptID, domain.NameSpaceEntry{
		Space: "floraveg", ExtID: "agg-5648", Name: "Festuca ovina aggr.", Aggregate: true,
	})
	seedRumexAcetosellaAggregate(t, a)

	ts := httptest.NewServer(a.Router)
	defer ts.Close()
	client := ts.Client()

	const body = `{
		"target_space": "floraveg",
		"names": [
			{"id": "agg", "verbatim": "Festuca ovina agg."},
			{"id": "plain", "verbatim": "Festuca ovina"},
			{"id": "unres", "verbatim": "Rumex acetosella agg."}
		]
	}`
	resp, err := client.Post(ts.URL+"/v1/match", "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("POST /v1/match: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /v1/match: status = %d, want 200", resp.StatusCode)
	}
	var match targetSpaceMatchResponse
	if err := json.NewDecoder(resp.Body).Decode(&match); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	_ = resp.Body.Close()

	byID := make(map[string]int, len(match.Results))
	for i, r := range match.Results {
		byID[r.ID] = i
	}
	get := func(id string) targetSpaceMatchResult { return match.Results[byID[id]] }

	// Each branch of the tri-state, asserted by name and policy (not just 200).
	assertTargetSpace(t, "agg", get("agg"), "known", "Festuca ovina aggr.")
	assertTargetSpace(t, "plain", get("plain"), "", "Festuca ovina")
	assertTargetSpace(t, "unres", get("unres"), "unresolvable", "")

	if got := get("plain").ConceptID; got != festucaOvinaConceptID {
		t.Errorf("plain: concept_id = %q, want %q", got, festucaOvinaConceptID)
	}

	// esy_diagnostic_relevance is the conspicuous sentinel on EVERY result.
	for _, r := range match.Results {
		if r.ESyDiagnosticRelevance != "not_determinable" {
			t.Errorf("%s: esy_diagnostic_relevance = %q, want %q", r.ID, r.ESyDiagnosticRelevance, "not_determinable")
		}
	}
}

// assertTargetSpace checks one result's aggregate_policy and target_space_name.
func assertTargetSpace(t *testing.T, label string, r targetSpaceMatchResult, wantPolicy, wantName string) {
	t.Helper()
	if r.AggregatePolicy != wantPolicy {
		t.Errorf("%s: aggregate_policy = %q, want %q", label, r.AggregatePolicy, wantPolicy)
	}
	if r.TargetSpaceName != wantName {
		t.Errorf("%s: target_space_name = %q, want %q", label, r.TargetSpaceName, wantName)
	}
}

// TestIntegration_MatchUnknownTargetSpaceReturns400 pins that an un-ingested
// target space is a 400 INVALID_QUERY naming it, over the real router.
func TestIntegration_MatchUnknownTargetSpaceReturns400(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "hostus.sqlite")
	ctx := context.Background()
	if _, err := app.Ingest(ctx, "testdata/dataset.yaml", dbPath); err != nil {
		t.Fatalf("app.Ingest: %v", err)
	}
	cfg := testConfig()
	cfg.SQLite.Path = dbPath
	a, err := app.New(cfg)
	if err != nil {
		t.Fatalf("app.New: %v", err)
	}
	t.Cleanup(func() { _ = a.Shutdown(context.Background()) })

	ts := httptest.NewServer(a.Router)
	defer ts.Close()

	resp, err := ts.Client().Post(ts.URL+"/v1/match", "application/json",
		bytes.NewBufferString(`{"target_space":"germansl","names":[{"id":"1","verbatim":"Festuca ovina"}]}`))
	if err != nil {
		t.Fatalf("POST /v1/match: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
	var env struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		t.Fatalf("decoding error envelope: %v", err)
	}
	if env.Error.Code != "INVALID_QUERY" {
		t.Errorf("error.code = %q, want INVALID_QUERY", env.Error.Code)
	}
	if !bytes.Contains([]byte(env.Error.Message), []byte("germansl")) {
		t.Errorf("error.message = %q, want it to name %q", env.Error.Message, "germansl")
	}
}

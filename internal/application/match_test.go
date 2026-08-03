package application_test

import (
	"context"
	"testing"

	"github.com/jobrunner/hostus/internal/adapters/sqlite"
	"github.com/jobrunner/hostus/internal/application"
	"github.com/jobrunner/hostus/internal/domain"
)

// seedFestucaOvinaAggregate adds a synthetic aggregate/collective-species
// concept ("Festuca ovina agg.") directly through the Repository port,
// bypassing the WCVP reader — WCVP backbones don't carry aggregate
// concepts, so a real ingest run never produces one; this is data the
// (future) aggregate-vocabulary source would supply. Kept minimal: just
// enough of a Concept for MatchExact to find and MatchNames to resolve to.
func seedFestucaOvinaAggregate(t *testing.T, repo *sqlite.DB) string {
	t.Helper()
	ctx := context.Background()
	tx, err := repo.BeginIngest(ctx, domain.BackboneVersion{ID: "test-agg", Version: "v1"})
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

// seededMatchRepo ingests the real WCVP fixture (T5's Ingest use case,
// exercised the same way ingest_test.go does) into a fresh in-memory repo,
// giving MatchNames real candidates to classify rather than a mock.
func seededMatchRepo(t *testing.T) *sqlite.DB {
	t.Helper()
	ds := loadDataset(t)
	repo := openMemoryRepo(t)
	if _, err := application.Ingest(context.Background(), ds, wcvpReaderFor, repo); err != nil {
		t.Fatalf("Ingest: unexpected error: %v", err)
	}
	return repo
}

// wantMatch is the subset of a MatchResult one spec-batch case cares about.
type wantMatch struct {
	matchType      domain.MatchType
	confidence     float64
	conceptID      string
	note           string // checked verbatim if non-empty, else just "non-empty" if noteNonEmpty is set
	noteNonEmpty   bool
	requiresReview bool
}

func assertMatchResult(t *testing.T, r application.MatchResult, want wantMatch) {
	t.Helper()
	if r.MatchType != want.matchType {
		t.Errorf("MatchType = %q, want %q", r.MatchType, want.matchType)
	}
	if r.Confidence != want.confidence {
		t.Errorf("Confidence = %v, want %v", r.Confidence, want.confidence)
	}
	if r.ConceptID != want.conceptID {
		t.Errorf("ConceptID = %q, want %q", r.ConceptID, want.conceptID)
	}
	if r.RequiresReview != want.requiresReview {
		t.Errorf("RequiresReview = %v, want %v", r.RequiresReview, want.requiresReview)
	}
	if want.note != "" && r.Note != want.note {
		t.Errorf("Note = %q, want %q", r.Note, want.note)
	}
	if want.noteNonEmpty && r.Note == "" {
		t.Error("Note = empty, want an explanation for the reviewer")
	}
}

func TestMatchNames_SpecBatch(t *testing.T) {
	repo := seededMatchRepo(t)
	jacobaeaVulgarisConceptID := "wcvp:concept:3082777"
	corynephorusConceptID := "wcvp:concept:405825"
	aggConceptID := seedFestucaOvinaAggregate(t, repo)

	cases := []struct {
		name     string
		verbatim string
		want     wantMatch
	}{
		{
			name:     "synonym query resolves to accepted concept via exact_author",
			verbatim: "Senecio jacobaea L.",
			want:     wantMatch{matchType: domain.MatchExactAuthor, confidence: 0.99, conceptID: jacobaeaVulgarisConceptID},
		},
		{
			name:     "aggregate resolves to the seeded aggregate concept",
			verbatim: "Festuca ovina agg.",
			want:     wantMatch{matchType: domain.MatchAggregateAlias, confidence: 0.95, conceptID: aggConceptID, note: "Aggregat, keine Kleinartauflösung"},
		},
		{
			name:     "typo (no fuzzy) is UNRESOLVABLE",
			verbatim: "Silene otitis",
			want:     wantMatch{requiresReview: true, noteNonEmpty: true},
		},
		{
			name:     "bare name with no author is exact",
			verbatim: "Corynephorus canescens",
			want:     wantMatch{matchType: domain.MatchExact, confidence: 0.90, conceptID: corynephorusConceptID},
		},
		{
			name:     "full name with matching author is exact_author",
			verbatim: "Corynephorus canescens (L.) P.Beauv.",
			want:     wantMatch{matchType: domain.MatchExactAuthor, confidence: 0.99, conceptID: corynephorusConceptID},
		},
	}

	reqs := make([]application.MatchRequest, len(cases))
	for i, c := range cases {
		reqs[i] = application.MatchRequest{ID: c.name, Verbatim: c.verbatim}
	}
	results, err := application.MatchNames(context.Background(), repo, reqs)
	if err != nil {
		t.Fatalf("MatchNames: unexpected error: %v", err)
	}
	if len(results) != len(reqs) {
		t.Fatalf("len(results) = %d, want %d", len(results), len(reqs))
	}
	byID := make(map[string]application.MatchResult, len(results))
	for _, r := range results {
		byID[r.ID] = r
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assertMatchResult(t, byID[c.name], c.want)
		})
	}
}

func TestMatchNames_AuthorMismatchIsUnresolvableAndListsCandidate(t *testing.T) {
	repo := seededMatchRepo(t)

	results, err := application.MatchNames(context.Background(), repo, []application.MatchRequest{
		{ID: "1", Verbatim: "Corynephorus canescens (Mill.) Someone"},
	})
	if err != nil {
		t.Fatalf("MatchNames: unexpected error: %v", err)
	}
	r := results[0]
	if r.MatchType != "" {
		t.Errorf("MatchType = %q, want empty (UNRESOLVABLE on author mismatch)", r.MatchType)
	}
	if !r.RequiresReview {
		t.Error("RequiresReview = false, want true")
	}
	if len(r.Candidates) != 1 || r.Candidates[0] != "Corynephorus canescens" {
		t.Errorf("Candidates = %v, want [%q] (the canonical-matching candidate whose author didn't classify)", r.Candidates, "Corynephorus canescens")
	}
}

func TestMatchNames_AggregateWithoutKnownConceptIsUnresolvable(t *testing.T) {
	repo := seededMatchRepo(t)

	results, err := application.MatchNames(context.Background(), repo, []application.MatchRequest{
		{ID: "1", Verbatim: "Nonexistentus bogus agg."},
	})
	if err != nil {
		t.Fatalf("MatchNames: unexpected error: %v", err)
	}
	r := results[0]
	if r.MatchType != "" {
		t.Errorf("MatchType = %q, want empty (UNRESOLVABLE)", r.MatchType)
	}
	if r.ConceptID != "" {
		t.Errorf("ConceptID = %q, want empty", r.ConceptID)
	}
	if !r.RequiresReview {
		t.Error("RequiresReview = false, want true")
	}
	if r.Note == "" {
		t.Error("Note = empty, want an explanation for the reviewer")
	}
}

func TestMatchNames_EmptyRequestsReturnsEmptyResults(t *testing.T) {
	repo := seededMatchRepo(t)

	results, err := application.MatchNames(context.Background(), repo, nil)
	if err != nil {
		t.Fatalf("MatchNames: unexpected error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("len(results) = %d, want 0", len(results))
	}
}

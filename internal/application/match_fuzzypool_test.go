package application_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/jobrunner/hostus/internal/adapters/sqlite"
	"github.com/jobrunner/hostus/internal/application"
	"github.com/jobrunner/hostus/internal/domain"
)

// seedCrowdedFuzzyPool ingests the target plus `decoys` names that share the
// query's genus AND its exact length, sorting alphabetically ahead of it.
//
// The length has to match: the prefilter orders by length-difference before
// anything else, so decoys of another length sort behind the target and cannot
// crowd it out — the test would pass without a fix.
func seedCrowdedFuzzyPool(t *testing.T, repo *sqlite.DB, decoys int) string {
	t.Helper()
	ctx := context.Background()
	tx, err := repo.BeginIngest(ctx, domain.BackboneVersion{ID: "crowd", Version: "v1"})
	if err != nil {
		t.Fatalf("BeginIngest: unexpected error: %v", err)
	}

	target := domain.Name{ID: "crowd:name:target", Canonical: "Festuca ovona", Rank: domain.RankSpecies}
	concept := domain.Concept{ID: "crowd:concept:target", BackboneID: "crowd", AcceptedName: target, Rank: domain.RankSpecies, Status: domain.StatusAccepted}
	if err := tx.UpsertName(target); err != nil {
		t.Fatalf("UpsertName(target): unexpected error: %v", err)
	}
	if err := tx.UpsertConcept(concept); err != nil {
		t.Fatalf("UpsertConcept(target): unexpected error: %v", err)
	}
	if err := tx.LinkName(concept.ID, target.ID, "accepted", nil); err != nil {
		t.Fatalf("LinkName(target): unexpected error: %v", err)
	}

	for i := range decoys {
		canon := fmt.Sprintf("Festuca a%04d", i)
		name := domain.Name{ID: fmt.Sprintf("crowd:name:%04d", i), Canonical: canon, Rank: domain.RankSpecies}
		c := domain.Concept{ID: fmt.Sprintf("crowd:concept:%04d", i), BackboneID: "crowd", AcceptedName: name, Rank: domain.RankSpecies, Status: domain.StatusAccepted}
		if err := tx.UpsertName(name); err != nil {
			t.Fatalf("UpsertName(decoy %d): unexpected error: %v", i, err)
		}
		if err := tx.UpsertConcept(c); err != nil {
			t.Fatalf("UpsertConcept(decoy %d): unexpected error: %v", i, err)
		}
		if err := tx.LinkName(c.ID, name.ID, "accepted", nil); err != nil {
			t.Fatalf("LinkName(decoy %d): unexpected error: %v", i, err)
		}
	}
	if err := tx.Finalize(); err != nil {
		t.Fatalf("Finalize: unexpected error: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: unexpected error: %v", err)
	}
	return concept.ID
}

// TestMatchNames_FuzzyResolvesOutOfACrowdedCandidatePool is the end-to-end
// regression for the measured 0.0 % recall — and it has to live HERE, not only
// in the repository tests, because the budget is the caller's: the application
// passes it in. A repository fixed in isolation while the application still
// asks for 20 candidates is a repository that changed nothing.
//
// One transposed letter in the epithet, 500 same-length names sorting ahead of
// the target. Measured against a real index, the old code resolved this class
// in 0.0 % of cases (docs/research/fuzzy-prefilter.md).
func TestMatchNames_FuzzyResolvesOutOfACrowdedCandidatePool(t *testing.T) {
	repo := openMemoryRepo(t)
	want := seedCrowdedFuzzyPool(t, repo, 500)

	results, err := application.MatchNames(context.Background(), repo, []application.MatchRequest{
		{ID: "1", Verbatim: "Festuca ovina"},
	})
	if err != nil {
		t.Fatalf("MatchNames: unexpected error: %v", err)
	}
	r := results[0]

	if r.MatchType != domain.MatchFuzzy {
		t.Errorf("MatchType = %q, want %q — the candidate budget went to the 500 decoys again", r.MatchType, domain.MatchFuzzy)
	}
	if r.ConceptID != want {
		t.Errorf("ConceptID = %q, want %q", r.ConceptID, want)
	}
}

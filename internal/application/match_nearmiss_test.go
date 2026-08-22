package application_test

import (
	"context"
	"strings"
	"testing"

	"github.com/jobrunner/hostus/internal/adapters/sqlite"
	"github.com/jobrunner/hostus/internal/application"
	"github.com/jobrunner/hostus/internal/domain"
)

// seedGenusSynonymyPair ingests a concept under the CURRENT genus, so a query
// written with a segregate genus that the backbone does not use produces the
// shape issue #67 measures as its biggest class-3 sub-group: Astracantha was
// split off Astragalus, and WCVP keeps the species under Astragalus.
// Similarity("astracantha diphtherites", "astragalus diphtherites") is 0.792 —
// meaningful, but under the 0.85 resolve threshold.
func seedGenusSynonymyPair(t *testing.T, repo *sqlite.DB) string {
	t.Helper()
	ctx := context.Background()
	tx, err := repo.BeginIngest(ctx, domain.BackboneVersion{ID: "test-genus", Version: "v1"})
	if err != nil {
		t.Fatalf("BeginIngest: unexpected error: %v", err)
	}
	name := domain.Name{ID: "test-genus:name:astragalus", Canonical: "Astragalus diphtherites", Rank: domain.RankSpecies}
	concept := domain.Concept{ID: "test-genus:concept:astragalus", BackboneID: "test-genus", AcceptedName: name, Rank: domain.RankSpecies, Status: domain.StatusAccepted}
	for _, step := range []struct {
		what string
		err  error
	}{
		{"UpsertName", tx.UpsertName(name)},
		{"UpsertConcept", tx.UpsertConcept(concept)},
		{"LinkName", tx.LinkName(concept.ID, name.ID, "accepted", nil)},
		{"Commit", tx.Commit()},
	} {
		if step.err != nil {
			t.Fatalf("%s: unexpected error: %v", step.what, step.err)
		}
	}
	return concept.ID
}

// TestMatchNames_NearMissCandidatesSurviveForReview is issue #67's class 3,
// first step. A name that clears no threshold used to come back UNRESOLVABLE
// with NOTHING attached — the near-misses fuzzy had just scored were computed
// and then thrown away. Handing them back lets the caller curate instead of
// losing the row; it deliberately does NOT resolve anything.
func TestMatchNames_NearMissCandidatesSurviveForReview(t *testing.T) {
	repo := seededMatchRepo(t)
	seedGenusSynonymyPair(t, repo)

	results, err := application.MatchNames(context.Background(), repo, []application.MatchRequest{
		{ID: "1", Verbatim: "Astracantha diphtherites"},
	})
	if err != nil {
		t.Fatalf("MatchNames: unexpected error: %v", err)
	}
	r := results[0]

	if r.ConceptID != "" {
		t.Errorf("ConceptID = %q, want empty: a near miss must never be presented as resolved", r.ConceptID)
	}
	if r.MatchType != "" {
		t.Errorf("MatchType = %q, want empty for an unresolved near miss", r.MatchType)
	}
	if !r.RequiresReview {
		t.Error("RequiresReview = false, want true")
	}
	if len(r.Candidates) == 0 {
		t.Fatal("Candidates empty: the near misses fuzzy scored were dropped again")
	}
	found := false
	for _, c := range r.Candidates {
		if c == "Astragalus diphtherites" {
			found = true
		}
	}
	if !found {
		t.Errorf("Candidates = %v, want the near miss %q among them", r.Candidates, "Astragalus diphtherites")
	}
	if r.Note == "" {
		t.Error("Note = empty, want the reviewer told what these candidates are")
	}
}

// TestMatchNames_HopelessQueryStaysBare pins the other half: a query with no
// plausible neighbor must NOT come back with a list of unrelated names.
// Measured against the issue's own example, Similarity("bellidiastrum
// michelii", "aster bellidiastrum") is 0.318 — string distance cannot find
// that one, and pretending otherwise would bury the real near misses in noise.
func TestMatchNames_HopelessQueryStaysBare(t *testing.T) {
	repo := seededMatchRepo(t)

	results, err := application.MatchNames(context.Background(), repo, []application.MatchRequest{
		{ID: "1", Verbatim: "Zzzzzzzz qqqqqqqq"},
	})
	if err != nil {
		t.Fatalf("MatchNames: unexpected error: %v", err)
	}
	r := results[0]
	if r.ConceptID != "" || !r.RequiresReview {
		t.Errorf("got ConceptID=%q RequiresReview=%v, want an unresolved review", r.ConceptID, r.RequiresReview)
	}
	if len(r.Candidates) != 0 {
		t.Errorf("Candidates = %v, want none: nothing here is a plausible neighbor", r.Candidates)
	}
}

// TestMatchNames_NearMissCandidatesAreOrderedBestFirst pins that the list is
// useful rather than merely present: a curator reads the top of it.
func TestMatchNames_NearMissCandidatesAreOrderedBestFirst(t *testing.T) {
	repo := seededMatchRepo(t)
	seedNearMissLadder(t, repo)

	results, err := application.MatchNames(context.Background(), repo, []application.MatchRequest{
		{ID: "1", Verbatim: "Astracantha diphtherites"},
	})
	if err != nil {
		t.Fatalf("MatchNames: unexpected error: %v", err)
	}
	cands := results[0].Candidates
	if len(cands) < 2 {
		t.Fatalf("Candidates = %v, want at least two to compare an order", cands)
	}
	best := domain.Similarity(domain.Canonicalize("Astracantha diphtherites"), domain.Canonicalize(cands[0]))
	for _, c := range cands[1:] {
		if s := domain.Similarity(domain.Canonicalize("Astracantha diphtherites"), domain.Canonicalize(c)); s > best {
			t.Errorf("candidate %q (%.3f) scores higher than the first entry %q (%.3f); the list must be best-first",
				c, s, cands[0], best)
		}
	}
}

// seedNearMissLadder ingests two names at MEASURED distances from the query
// "Astracantha diphtherites": 0.792 and 0.750. Both sit in the review band
// (over the floor, under the resolve threshold), so neither resolves and their
// ORDER in the candidate list is observable.
func seedNearMissLadder(t *testing.T, repo *sqlite.DB) {
	t.Helper()
	ctx := context.Background()
	tx, err := repo.BeginIngest(ctx, domain.BackboneVersion{ID: "test-ladder", Version: "v1"})
	if err != nil {
		t.Fatalf("BeginIngest: unexpected error: %v", err)
	}
	for i, canonical := range []string{"Astragalus diphtheritus", "Astragalus diphtherites"} {
		name := domain.Name{ID: "test-ladder:name:" + strings.Repeat("x", i+1), Canonical: canonical, Rank: domain.RankSpecies}
		concept := domain.Concept{ID: "test-ladder:concept:" + strings.Repeat("x", i+1), BackboneID: "test-ladder", AcceptedName: name, Rank: domain.RankSpecies, Status: domain.StatusAccepted}
		if err := tx.UpsertName(name); err != nil {
			t.Fatalf("UpsertName: %v", err)
		}
		if err := tx.UpsertConcept(concept); err != nil {
			t.Fatalf("UpsertConcept: %v", err)
		}
		if err := tx.LinkName(concept.ID, name.ID, "accepted", nil); err != nil {
			t.Fatalf("LinkName: %v", err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}
}

// TestMatchNames_NearMissTiesAreOrderedDeterministically pins the tiebreaker.
// Candidates are collected through a map, whose iteration order Go randomizes
// on purpose, so equally-near names would come back in a different order on
// every identical request — a poor thing to hand a reviewer, and untestable
// besides. Ties therefore sort by name.
//
// The three seeded names are MEASURED to score exactly 0.769231 against the
// query — inside the review band, so none resolves and all three are listed.
func TestMatchNames_NearMissTiesAreOrderedDeterministically(t *testing.T) {
	repo := seededMatchRepo(t)
	seedEqualDistanceTrio(t, repo)

	var first []string
	for run := range 4 {
		results, err := application.MatchNames(context.Background(), repo, []application.MatchRequest{
			{ID: "1", Verbatim: "Zzzaaa bbbccc"},
		})
		if err != nil {
			t.Fatalf("MatchNames run %d: unexpected error: %v", run, err)
		}
		got := results[0].Candidates
		if len(got) < 3 {
			t.Fatalf("run %d: Candidates = %v, want all three equally-near names", run, got)
		}
		if run == 0 {
			first = got
			// Equal scores must fall back to name order.
			for i := 1; i < len(got); i++ {
				if got[i-1] > got[i] {
					t.Errorf("Candidates not name-sorted within the tie: %v", got)
				}
			}
			continue
		}
		if strings.Join(got, "|") != strings.Join(first, "|") {
			t.Errorf("run %d returned %v, run 0 returned %v — identical requests must give an identical order", run, got, first)
		}
	}
}

// seedEqualDistanceTrio ingests three names at the SAME measured distance
// (0.769231) from "Zzzaaa bbbccc", so only the tiebreaker decides their order.
//
// All three share the query's first four runes on purpose. That is not
// cosmetic: the prefilter pins a 4-rune prefix, so a name agreeing with the
// query in only three ("Zzzxxx bbbccc", which this fixture used to carry) is
// legitimately no longer a candidate at all — and the trio would silently
// become a pair, testing the tiebreaker on fewer ties than it claims.
func seedEqualDistanceTrio(t *testing.T, repo *sqlite.DB) {
	t.Helper()
	ctx := context.Background()
	tx, err := repo.BeginIngest(ctx, domain.BackboneVersion{ID: "test-tie", Version: "v1"})
	if err != nil {
		t.Fatalf("BeginIngest: unexpected error: %v", err)
	}
	for i, canonical := range []string{"Zzzaaa xxxccc", "Zzzaaa bbbxyz", "Zzzaaa bbbxxx"} {
		id := string(rune('a' + i))
		name := domain.Name{ID: "test-tie:name:" + id, Canonical: canonical, Rank: domain.RankSpecies}
		concept := domain.Concept{ID: "test-tie:concept:" + id, BackboneID: "test-tie", AcceptedName: name, Rank: domain.RankSpecies, Status: domain.StatusAccepted}
		if err := tx.UpsertName(name); err != nil {
			t.Fatalf("UpsertName: %v", err)
		}
		if err := tx.UpsertConcept(concept); err != nil {
			t.Fatalf("UpsertConcept: %v", err)
		}
		if err := tx.LinkName(concept.ID, name.ID, "accepted", nil); err != nil {
			t.Fatalf("LinkName: %v", err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}
}

// TestMatchNames_AggregateNearMissStaysUnresolvedInPractice pins the seam the
// review flagged: matchFuzzy is shared, so an aggregate query can now in
// principle come back with near-miss candidates instead of the plain
// "unresolved aggregate" note. In practice it cannot, and the arithmetic is the
// reason: matchFuzzy is called with the MARKER-INCLUDED canonical, the
// repository prefilter admits candidates within 3 runes of its length, and
// " aggr." adds six. A bare near neighbor — the realistic shape, since
// backbones carry unmarked names — is therefore always outside the window.
//
// Pinned rather than merely commented because it is the difference between
// "documented limitation" and "we assumed and never checked".
func TestMatchNames_AggregateNearMissStaysUnresolvedInPractice(t *testing.T) {
	repo := seededMatchRepo(t)
	// A near neighbor of the BARE name, 0.769 away — well inside the review
	// floor, and irrelevant here because the marker pushes the query out of the
	// prefilter's length window.
	seedEqualDistanceTrio(t, repo)

	results, err := application.MatchNames(context.Background(), repo, []application.MatchRequest{
		{ID: "1", Verbatim: "Zzzaaa bbbccc aggr."},
	})
	if err != nil {
		t.Fatalf("MatchNames: unexpected error: %v", err)
	}
	r := results[0]
	if r.ConceptID != "" {
		t.Errorf("ConceptID = %q, want empty", r.ConceptID)
	}
	if len(r.Candidates) != 0 {
		t.Errorf("Candidates = %v, want none: the marker puts every bare neighbor outside the prefilter window", r.Candidates)
	}
	if !strings.Contains(r.Note, "Aggregat") {
		t.Errorf("Note = %q, want it to still say this was an aggregate query", r.Note)
	}
}

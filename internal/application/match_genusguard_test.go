package application_test

import (
	"context"
	"strings"
	"testing"

	"github.com/jobrunner/hostus/internal/adapters/sqlite"
	"github.com/jobrunner/hostus/internal/application"
	"github.com/jobrunner/hostus/internal/domain"
)

// seedIndexedConcept ingests one accepted concept AND finalizes the ingest, so
// its canonical is in fts_name — which the epithet prefilter route needs. The
// other match fixtures skip Finalize because the prefix route never reads it.
//
// The backbone is fixed to "wcvp" because every caller matches with
// entry_backbone unset, and a resolution filter is not what these tests are
// about.
func seedIndexedConcept(t *testing.T, repo *sqlite.DB, canonical string) string {
	t.Helper()
	const backbone = "wcvp"
	ctx := context.Background()
	tx, err := repo.BeginIngest(ctx, domain.BackboneVersion{ID: backbone, Version: "v1"})
	if err != nil {
		t.Fatalf("BeginIngest: unexpected error: %v", err)
	}
	name := domain.Name{ID: backbone + ":name:1", Canonical: canonical, Rank: domain.RankSpecies}
	concept := domain.Concept{ID: backbone + ":concept:1", BackboneID: backbone, AcceptedName: name, Rank: domain.RankSpecies, Status: domain.StatusAccepted}
	for _, step := range []struct {
		what string
		err  error
	}{
		{"UpsertName", tx.UpsertName(name)},
		{"UpsertConcept", tx.UpsertConcept(concept)},
		{"LinkName", tx.LinkName(concept.ID, name.ID, "accepted", nil)},
		{"Finalize", tx.Finalize()},
		{"Commit", tx.Commit()},
	} {
		if step.err != nil {
			t.Fatalf("%s: unexpected error: %v", step.what, step.err)
		}
	}
	return concept.ID
}

// TestMatchNames_DoesNotResolveAcrossUnrelatedGenera is the precision half of
// the fuzzy fix, and the reason the epithet prefilter route cannot ship on its
// own. Reaching a name through its epithet means an IDENTICAL epithet is
// enough to enter the candidate set — and an epithet is the longer half of a
// binomial, so the whole-string score clears the threshold while the two names
// are unrelated plants.
//
// Measured against a full index: 19 of the 62 ESy names that cleared the
// threshold this way were wrong, all of them bryophyte or lichen genera the
// vascular backbone does not carry at all (docs/research/fuzzy-prefilter.md).
// "Sphagnum platyphyllum" is a peat moss; "Solanum platyphyllum" is a
// nightshade; the two score 0.857 together.
func TestMatchNames_DoesNotResolveAcrossUnrelatedGenera(t *testing.T) {
	repo := openMemoryRepo(t)
	seedIndexedConcept(t, repo, "Solanum platyphyllum")

	results, err := application.MatchNames(context.Background(), repo, []application.MatchRequest{
		{ID: "1", Verbatim: "Sphagnum platyphyllum"},
	})
	if err != nil {
		t.Fatalf("MatchNames: unexpected error: %v", err)
	}
	r := results[0]

	if whole := domain.Similarity("sphagnum platyphyllum", "solanum platyphyllum"); whole < domain.FuzzyThreshold {
		t.Fatalf("test setup: the pair scores %v, below the threshold — then nothing here is being guarded", whole)
	}
	if r.ConceptID != "" {
		t.Errorf("ConceptID = %q, want empty: a shared epithet across unrelated genera must not resolve", r.ConceptID)
	}
	if r.MatchType != "" {
		t.Errorf("MatchType = %q, want empty", r.MatchType)
	}
	if !r.RequiresReview {
		t.Error("RequiresReview = false, want true")
	}
	// Blocked from resolving, NOT hidden: a curator seeing the candidate can
	// tell "wrong genus" at a glance, which is more than an empty answer says.
	found := false
	for _, c := range r.Candidates {
		if c == "Solanum platyphyllum" {
			found = true
		}
	}
	if !found {
		t.Errorf("Candidates = %v, want the rejected candidate listed for review", r.Candidates)
	}
}

// TestMatchNames_GenusMismatchSaysSoInsteadOfClaimingNothingWasCloseEnough:
// the note a curator reads has to be true. The near-miss note says "no hit
// above the similarity threshold", which is exactly wrong here — the hit WAS
// above it and was refused because the genus is a different plant. Telling a
// curator "nothing was close" while listing a 0.857 candidate teaches them to
// stop trusting the note.
func TestMatchNames_GenusMismatchSaysSoInsteadOfClaimingNothingWasCloseEnough(t *testing.T) {
	repo := openMemoryRepo(t)
	seedIndexedConcept(t, repo, "Solanum platyphyllum")

	results, err := application.MatchNames(context.Background(), repo, []application.MatchRequest{
		{ID: "1", Verbatim: "Sphagnum platyphyllum"},
	})
	if err != nil {
		t.Fatalf("MatchNames: unexpected error: %v", err)
	}
	if note := results[0].Note; !strings.Contains(note, "Gattung") {
		t.Errorf("Note = %q, want it to name the genus as the reason", note)
	}
}

// TestMatchNames_GenusMismatchNoteAppliesAtTheExactThreshold pins the note
// choice at the boundary. Which of the two unresolved notes applies is decided
// by "did any candidate reach the threshold", and FuzzyThreshold is inclusive
// — so a candidate landing EXACTLY on it was close enough, and the note has to
// say "wrong genus", not "nothing was close enough". Without a case sitting
// precisely on 0.85 the two comparisons are indistinguishable; the mutation
// run showed that survivor.
//
// The pair is engineered rather than botanical (like the existing
// "Zzzaaa bbbccc" fixtures): 20 runes each, 3 substitutions all inside the
// genus -> whole-string exactly 0.850000, genus 0.625.
func TestMatchNames_GenusMismatchNoteAppliesAtTheExactThreshold(t *testing.T) {
	repo := openMemoryRepo(t)
	seedIndexedConcept(t, repo, "Abcdxyzh ijklmnopqrs")

	const query = "Abcdefgh ijklmnopqrs"
	if got := domain.Similarity(domain.Canonicalize(query), "abcdxyzh ijklmnopqrs"); got != domain.FuzzyThreshold {
		t.Fatalf("test setup: Similarity = %v, want exactly %v", got, domain.FuzzyThreshold)
	}

	results, err := application.MatchNames(context.Background(), repo, []application.MatchRequest{
		{ID: "1", Verbatim: query},
	})
	if err != nil {
		t.Fatalf("MatchNames: unexpected error: %v", err)
	}
	if note := results[0].Note; !strings.Contains(note, "Gattung") {
		t.Errorf("Note = %q, want the genus named as the reason even at exactly the threshold", note)
	}
}

// TestMatchNames_StillResolvesAMisspelledGenus is the other side: the guard
// must not cost the cases the fix exists for. A genuine misspelling moves the
// genus by a letter, and the epithet route is what finds it at all — the
// 4-rune prefix cannot ("coch" vs "cochl" agree, but "arctostaphylos"-class
// typos generally do not).
func TestMatchNames_StillResolvesAMisspelledGenus(t *testing.T) {
	repo := openMemoryRepo(t)
	conceptID := seedIndexedConcept(t, repo, "Cochlearia amana")

	results, err := application.MatchNames(context.Background(), repo, []application.MatchRequest{
		{ID: "1", Verbatim: "Cochleria amana"},
	})
	if err != nil {
		t.Fatalf("MatchNames: unexpected error: %v", err)
	}
	r := results[0]

	if r.MatchType != domain.MatchFuzzy {
		t.Errorf("MatchType = %q, want %q", r.MatchType, domain.MatchFuzzy)
	}
	if r.ConceptID != conceptID {
		t.Errorf("ConceptID = %q, want %q", r.ConceptID, conceptID)
	}
	if !r.RequiresReview {
		t.Error("RequiresReview = false, want true: a fuzzy hit always needs review")
	}
}

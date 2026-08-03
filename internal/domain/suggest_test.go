package domain_test

import (
	"testing"

	"github.com/jobrunner/hostus/internal/domain"
)

// TestRankSuggestions_InAreaBeatsAccepted is the brief's pinned regression:
// in_area (priority 2) must dominate accepted-vs-synonym (priority 3), even
// though the out-of-area item is accepted and has a "better" (lower) score.
func TestRankSuggestions_InAreaBeatsAccepted(t *testing.T) {
	items := []domain.SuggestItem{
		{ConceptID: "a", InArea: false, Status: domain.StatusAccepted, PrefixHit: true, Score: 0.1},
		{ConceptID: "b", InArea: true, Status: domain.StatusSynonym, PrefixHit: true, Score: 0.9},
	}
	got := domain.RankSuggestions(items)
	if got[0].ConceptID != "b" {
		t.Fatalf("in_area must outrank accepted: %v", got)
	}
}

// TestRankSuggestions_PrefixHitBeatsInArea isolates priority 1 (PrefixHit)
// over priority 2 (InArea): the non-prefix-hit item is in_area, accepted,
// lower rank order, and better score — yet must still lose.
func TestRankSuggestions_PrefixHitBeatsInArea(t *testing.T) {
	items := []domain.SuggestItem{
		{ConceptID: "a", PrefixHit: false, InArea: true, Status: domain.StatusAccepted, Rank: domain.RankSpecies, Score: 0.1},
		{ConceptID: "b", PrefixHit: true, InArea: false, Status: domain.StatusSynonym, Rank: domain.RankForm, Score: 0.9},
	}
	got := domain.RankSuggestions(items)
	if got[0].ConceptID != "b" {
		t.Fatalf("prefix hit must outrank in_area: %v", got)
	}
}

// TestRankSuggestions_AcceptedBeatsRankOrder isolates priority 3 (accepted
// status) over priority 4 (rank order): the synonym item has a better
// (lower) rank order and score, but must still lose to the accepted item.
func TestRankSuggestions_AcceptedBeatsRankOrder(t *testing.T) {
	items := []domain.SuggestItem{
		{ConceptID: "a", PrefixHit: true, InArea: true, Status: domain.StatusSynonym, Rank: domain.RankSpecies, Score: 0.1},
		{ConceptID: "b", PrefixHit: true, InArea: true, Status: domain.StatusAccepted, Rank: domain.RankForm, Score: 0.9},
	}
	got := domain.RankSuggestions(items)
	if got[0].ConceptID != "b" {
		t.Fatalf("accepted must outrank lower rank order: %v", got)
	}
}

// TestRankSuggestions_RankOrderBeatsScore isolates priority 4 (rank order)
// over priority 5 (score): the higher-rank-order item has a better (lower)
// score, but must still lose to the lower-rank-order item.
func TestRankSuggestions_RankOrderBeatsScore(t *testing.T) {
	items := []domain.SuggestItem{
		{ConceptID: "a", PrefixHit: true, InArea: true, Status: domain.StatusAccepted, Rank: domain.RankForm, Score: 0.1},
		{ConceptID: "b", PrefixHit: true, InArea: true, Status: domain.StatusAccepted, Rank: domain.RankSpecies, Score: 0.9},
	}
	got := domain.RankSuggestions(items)
	if got[0].ConceptID != "b" {
		t.Fatalf("lower rank order must outrank higher rank order: %v", got)
	}
}

// TestRankSuggestions_ScoreAscending isolates priority 5: with all higher
// keys equal, lower Score (SQLite bm25: lower = more relevant) wins.
func TestRankSuggestions_ScoreAscending(t *testing.T) {
	items := []domain.SuggestItem{
		{ConceptID: "a", PrefixHit: true, InArea: true, Status: domain.StatusAccepted, Rank: domain.RankSpecies, Score: 5.0},
		{ConceptID: "b", PrefixHit: true, InArea: true, Status: domain.StatusAccepted, Rank: domain.RankSpecies, Score: 1.0},
	}
	got := domain.RankSuggestions(items)
	if got[0].ConceptID != "b" {
		t.Fatalf("lower score must sort first: %v", got)
	}
}

// TestRankSuggestions_StableForEqualKeys verifies sort.SliceStable is used:
// items with identical priority keys keep their input order.
func TestRankSuggestions_StableForEqualKeys(t *testing.T) {
	items := []domain.SuggestItem{
		{ConceptID: "first", PrefixHit: true, InArea: true, Status: domain.StatusAccepted, Rank: domain.RankSpecies, Score: 1.0},
		{ConceptID: "second", PrefixHit: true, InArea: true, Status: domain.StatusAccepted, Rank: domain.RankSpecies, Score: 1.0},
		{ConceptID: "third", PrefixHit: true, InArea: true, Status: domain.StatusAccepted, Rank: domain.RankSpecies, Score: 1.0},
	}
	got := domain.RankSuggestions(items)
	if got[0].ConceptID != "first" || got[1].ConceptID != "second" || got[2].ConceptID != "third" {
		t.Fatalf("equal keys must keep input order: %v", got)
	}
}

// TestRankSuggestions_DoesNotMutateInput guards against surprising aliasing
// bugs: callers should be able to rely on RankSuggestions returning a new
// slice without reordering the caller's backing array as a side effect that
// could be missed if only the return value is inspected.
func TestRankSuggestions_DoesNotMutateInput(t *testing.T) {
	items := []domain.SuggestItem{
		{ConceptID: "a", PrefixHit: false, Score: 1.0},
		{ConceptID: "b", PrefixHit: true, Score: 1.0},
	}
	original := append([]domain.SuggestItem(nil), items...)
	_ = domain.RankSuggestions(items)
	for i := range items {
		if items[i].ConceptID != original[i].ConceptID {
			t.Fatalf("RankSuggestions must not mutate its input slice: got %v, want %v", items, original)
		}
	}
}

// TestRankSuggestions_Empty guards the zero-length edge case.
func TestRankSuggestions_Empty(t *testing.T) {
	got := domain.RankSuggestions(nil)
	if len(got) != 0 {
		t.Fatalf("expected empty result, got %v", got)
	}
}

func TestRankOrder(t *testing.T) {
	tests := []struct {
		rank domain.Rank
		want int
	}{
		{domain.RankFamily, 0},
		{domain.RankGenus, 1},
		{domain.RankSpecies, 2},
		{domain.RankSubspecies, 3},
		{domain.RankNothosubspecies, 4},
		{domain.RankVariety, 5},
		{domain.RankSubvariety, 6},
		{domain.RankNothovariety, 7},
		{domain.RankForm, 8},
		{domain.RankSubform, 9},
		{domain.RankNothoform, 10},
		{domain.RankOther, 11},
	}
	for _, tt := range tests {
		t.Run(string(tt.rank), func(t *testing.T) {
			if got := domain.RankOrder(tt.rank); got != tt.want {
				t.Fatalf("RankOrder(%s) = %d, want %d", tt.rank, got, tt.want)
			}
		})
	}

	// Ensure the ordering is monotonically increasing on the brief's
	// pinned sequence: family < genus < species < subspecies < variety <
	// subvariety < form < subform < other (§B.1 step 4, extended for the
	// full WCVP rank vocabulary). The nothotaxon ranks are deliberately
	// left out of this specific chain (see rankOrder's doc comment for
	// where they're interleaved) since the brief doesn't pin their
	// relative order.
	ranks := []domain.Rank{
		domain.RankFamily, domain.RankGenus, domain.RankSpecies,
		domain.RankSubspecies, domain.RankVariety, domain.RankSubvariety,
		domain.RankForm, domain.RankSubform, domain.RankOther,
	}
	for i := 1; i < len(ranks); i++ {
		if domain.RankOrder(ranks[i-1]) >= domain.RankOrder(ranks[i]) {
			t.Fatalf("RankOrder must be strictly increasing: %s(%d) >= %s(%d)",
				ranks[i-1], domain.RankOrder(ranks[i-1]), ranks[i], domain.RankOrder(ranks[i]))
		}
	}
}

func TestRankOrder_UnknownRankIsWorstOrder(t *testing.T) {
	// An unrecognized/empty Rank must sort after all known ranks, not be
	// mistaken for FAMILY (ordinal 0) or silently accepted anywhere in the
	// middle of the ordering.
	if got := domain.RankOrder(domain.Rank("")); got <= domain.RankOrder(domain.RankForm) {
		t.Fatalf("RankOrder of unknown rank must exceed RankOrder(FORM), got %d", got)
	}
}

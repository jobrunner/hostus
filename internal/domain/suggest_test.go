package domain_test

import (
	"testing"

	"github.com/jobrunner/hostus/internal/domain"
)

// TestRankSuggestions_TargetSpaceHitBeatsBetterScore pins the measured
// Inula-hirta case: both concepts carry the queried name (Inula hirta L.
// for Pentanema hirtum, Inula hirta Pollich for P. britannica), so the
// exact-hit key ties and the target-space key decides. Before this key
// existed, bm25 alone ordered them and put britannica — whose eurosl name
// is "Inula britannica", i.e. NOT what was typed — first.
func TestRankSuggestions_TargetSpaceHitBeatsBetterScore(t *testing.T) {
	britannica := domain.SuggestItem{
		ConceptID: "wcvp:concept:3217682", Canonical: "Pentanema britannica",
		Rank: domain.RankSpecies, Status: domain.StatusAccepted,
		ExactHit: true, TargetSpaceHit: false, PrefixHit: true, Score: 1.0,
	}
	hirtum := domain.SuggestItem{
		ConceptID: "wcvp:concept:3217689", Canonical: "Pentanema hirtum",
		Rank: domain.RankSpecies, Status: domain.StatusAccepted,
		ExactHit: true, TargetSpaceHit: true, PrefixHit: true, Score: 2.0,
	}

	got := domain.RankSuggestions([]domain.SuggestItem{britannica, hirtum})

	if got[0].ConceptID != hirtum.ConceptID {
		t.Fatalf("first = %q, want the target-space hit %q", got[0].ConceptID, hirtum.ConceptID)
	}
}

// TestRankSuggestions_ExactHitBeatsBetterScore isolates the new leading
// criterion (ExactHit) over Score: the exact hit has a worse (higher)
// score, but must still win, since a caller who typed the full name wants
// no longer match ahead of it.
func TestRankSuggestions_ExactHitBeatsBetterScore(t *testing.T) {
	items := []domain.SuggestItem{
		{ConceptID: "a", ExactHit: false, PrefixHit: true, InArea: true, Status: domain.StatusAccepted, Rank: domain.RankSpecies, Score: 0.1},
		{ConceptID: "b", ExactHit: true, PrefixHit: true, InArea: true, Status: domain.StatusAccepted, Rank: domain.RankSpecies, Score: 0.9},
	}
	got := domain.RankSuggestions(items)
	if got[0].ConceptID != "b" {
		t.Fatalf("exact hit must outrank a better score: %v", got)
	}
}

// TestRankSuggestions_ExistingOrderUnchangedWhenNewFieldsEqual guards that
// the two new criteria (ExactHit, TargetSpaceHit) do not disturb the
// pre-existing priority chain (PrefixHit, InArea, accepted, rank order,
// score) when both new fields are equal across items.
func TestRankSuggestions_ExistingOrderUnchangedWhenNewFieldsEqual(t *testing.T) {
	items := []domain.SuggestItem{
		{ConceptID: "a", ExactHit: true, TargetSpaceHit: true, PrefixHit: false, InArea: true, Status: domain.StatusAccepted, Rank: domain.RankSpecies, Score: 0.1},
		{ConceptID: "b", ExactHit: true, TargetSpaceHit: true, PrefixHit: true, InArea: false, Status: domain.StatusSynonym, Rank: domain.RankForm, Score: 0.9},
	}
	got := domain.RankSuggestions(items)
	if got[0].ConceptID != "b" {
		t.Fatalf("with new fields equal, PrefixHit must still outrank in_area: %v", got)
	}
}

// TestRankSuggestions_ExactHitBeatsPrefixHit pins that ExactHit (priority 1)
// outranks PrefixHit (priority 3) even when PrefixHit DIFFERS between the
// two items — unlike TestRankSuggestions_ExactHitBeatsBetterScore, which
// keeps PrefixHit true on both sides and so cannot catch the two leading
// keys being reordered relative to PrefixHit. This is the scenario Task 2
// makes real once match_mode=anywhere lets PrefixHit vary: an exact hit
// that is NOT also a prefix hit must still beat a mere prefix hit.
func TestRankSuggestions_ExactHitBeatsPrefixHit(t *testing.T) {
	items := []domain.SuggestItem{
		{ConceptID: "a", ExactHit: false, PrefixHit: true, InArea: true, Status: domain.StatusAccepted, Rank: domain.RankSpecies, Score: 0.1},
		{ConceptID: "b", ExactHit: true, PrefixHit: false, InArea: true, Status: domain.StatusAccepted, Rank: domain.RankSpecies, Score: 0.9},
	}
	got := domain.RankSuggestions(items)
	if got[0].ConceptID != "b" {
		t.Fatalf("exact hit must outrank a mere prefix hit: %v", got)
	}
}

// TestRankSuggestions_TargetSpaceHitBeatsPrefixHit pins that TargetSpaceHit
// (priority 2) outranks PrefixHit (priority 3) with PrefixHit differing
// between items and ExactHit tied, so only the ExactHit/TargetSpaceHit vs.
// PrefixHit ordering can make this pass.
func TestRankSuggestions_TargetSpaceHitBeatsPrefixHit(t *testing.T) {
	items := []domain.SuggestItem{
		{ConceptID: "a", ExactHit: true, TargetSpaceHit: false, PrefixHit: true, InArea: true, Status: domain.StatusAccepted, Rank: domain.RankSpecies, Score: 0.1},
		{ConceptID: "b", ExactHit: true, TargetSpaceHit: true, PrefixHit: false, InArea: true, Status: domain.StatusAccepted, Rank: domain.RankSpecies, Score: 0.9},
	}
	got := domain.RankSuggestions(items)
	if got[0].ConceptID != "b" {
		t.Fatalf("target-space hit must outrank a mere prefix hit: %v", got)
	}
}

// TestRankSuggestions_InAreaBeatsAccepted is the brief's pinned regression:
// in_area (priority 4) must dominate accepted-vs-synonym (priority 5), even
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

// TestRankSuggestions_PrefixHitBeatsInArea isolates priority 3 (PrefixHit)
// over priority 4 (InArea): the non-prefix-hit item is in_area, accepted,
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

// TestRankSuggestions_AcceptedBeatsRankOrder isolates priority 5 (accepted
// status) over priority 6 (rank order): the synonym item has a better
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

// TestRankSuggestions_RankOrderBeatsScore isolates priority 6 (rank order)
// over priority 7 (score): the higher-rank-order item has a better (lower)
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

// TestRankSuggestions_ScoreAscending isolates priority 7: with all higher
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

// TestRankSuggestions_DisqualifiedMatchedNameLosesToLegitimate pins the
// reported Inula-hirta case as the user hits it: entry_backbone=wcvp and NO
// target_space, so the target-space key cannot decide. "Inula hirta Pollich"
// is a later illegitimate homonym (WCVP nom_status ", nom. illeg. homonym.
// post.") of Pentanema britannica; "Inula hirta L." is the legitimate name
// of Pentanema hirtum. Before this key existed, bm25 alone ordered them.
func TestRankSuggestions_DisqualifiedMatchedNameLosesToLegitimate(t *testing.T) {
	britannica := domain.SuggestItem{
		ConceptID: "wcvp:concept:3217682", Rank: domain.RankSpecies, Status: domain.StatusAccepted,
		ExactHit: true, MatchedNameDisqualified: true, PrefixHit: true, Score: 1.0,
	}
	hirtum := domain.SuggestItem{
		ConceptID: "wcvp:concept:3217689", Rank: domain.RankSpecies, Status: domain.StatusAccepted,
		ExactHit: true, MatchedNameDisqualified: false, PrefixHit: true, Score: 2.0,
	}

	got := domain.RankSuggestions([]domain.SuggestItem{britannica, hirtum})

	if got[0].ConceptID != hirtum.ConceptID {
		t.Fatalf("first = %q, want the legitimate name %q", got[0].ConceptID, hirtum.ConceptID)
	}
}

// TestRankSuggestions_LegitimacyOutranksTargetSpaceHit pins the POSITION of
// the new key: it sits ahead of TargetSpaceHit, so a disqualified name does
// not win just because it happens to be the spelling used in the requested
// space. Both keys differ between the items — otherwise swapping them would
// leave this test green and pin nothing.
func TestRankSuggestions_LegitimacyOutranksTargetSpaceHit(t *testing.T) {
	disqualifiedInSpace := domain.SuggestItem{
		ConceptID: "a", Rank: domain.RankSpecies, Status: domain.StatusAccepted, ExactHit: true,
		MatchedNameDisqualified: true, TargetSpaceHit: true, Score: 0.1,
	}
	legitimateOutsideSpace := domain.SuggestItem{
		ConceptID: "b", Rank: domain.RankSpecies, Status: domain.StatusAccepted, ExactHit: true,
		MatchedNameDisqualified: false, TargetSpaceHit: false, Score: 0.9,
	}

	got := domain.RankSuggestions([]domain.SuggestItem{disqualifiedInSpace, legitimateOutsideSpace})

	if got[0].ConceptID != "b" {
		t.Fatalf("first = %q, want the legitimate name %q", got[0].ConceptID, "b")
	}
}

// TestRankSuggestions_OnlyDisqualifyingJudgementLowersRank guards Spec
// decision 2: of the four NomStatusJudgement values, only
// JudgementDisqualifying may down-rank a candidate. JudgementAbsent (nothing
// recorded), JudgementAcceptable (e.g. nom. cons.) and JudgementUnclassified
// (e.g. "sensu auct.", "fossil name" — cases where the existing rule table
// deliberately withholds a verdict rather than treat uncertainty as a
// defect) must all map to MatchedNameDisqualified == false, exactly as the
// adapter would derive it from MatchedName.NomStatusJudgement. Three items
// differing only in that derived bool must keep their input order.
func TestRankSuggestions_OnlyDisqualifyingJudgementLowersRank(t *testing.T) {
	judgements := []domain.NomStatusJudgement{
		domain.JudgementAbsent, domain.JudgementAcceptable, domain.JudgementUnclassified,
	}
	ids := []string{"absent", "acceptable", "unclassified"}
	items := make([]domain.SuggestItem, len(judgements))
	for i, j := range judgements {
		items[i] = domain.SuggestItem{
			ConceptID: ids[i], Rank: domain.RankSpecies, Status: domain.StatusAccepted, ExactHit: true,
			MatchedNameDisqualified: j == domain.JudgementDisqualifying,
			MatchedName:             domain.MatchedName{NomStatusJudgement: j},
			Score:                   1.0,
		}
	}

	got := domain.RankSuggestions(items)

	for i, want := range ids {
		if got[i].ConceptID != want {
			t.Fatalf("order changed: got %v, want input order preserved %v (no non-disqualifying judgement may down-rank)", got, ids)
		}
	}
}

func TestRankOrderPriority(t *testing.T) {
	tests := []struct {
		rank domain.Rank
		want int
	}{
		{domain.RankFamily, 8},
		{domain.RankGenus, 11},
		{domain.RankSpecies, 18},
		{domain.RankSubspecies, 19},
		{domain.RankNothosubspecies, 20},
		{domain.RankVariety, 21},
		{domain.RankSubvariety, 22},
		{domain.RankNothovariety, 23},
		{domain.RankForm, 24},
		{domain.RankSubform, 25},
		{domain.RankNothoform, 26},
		{domain.RankOther, 35},
	}
	for _, tt := range tests {
		t.Run(string(tt.rank), func(t *testing.T) {
			if got := domain.RankOrderPriority(tt.rank); got != tt.want {
				t.Fatalf("RankOrderPriority(%s) = %d, want %d", tt.rank, got, tt.want)
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
		if domain.RankOrderPriority(ranks[i-1]) >= domain.RankOrderPriority(ranks[i]) {
			t.Fatalf("RankOrderPriority must be strictly increasing: %s(%d) >= %s(%d)",
				ranks[i-1], domain.RankOrderPriority(ranks[i-1]), ranks[i], domain.RankOrderPriority(ranks[i]))
		}
	}
}

func TestRankOrderPriority_UnknownRankIsWorstOrder(t *testing.T) {
	// An unrecognized/empty Rank must sort after all known ranks, not be
	// mistaken for FAMILY (ordinal 0) or silently accepted anywhere in the
	// middle of the ordering.
	if got := domain.RankOrderPriority(domain.Rank("")); got <= domain.RankOrderPriority(domain.RankForm) {
		t.Fatalf("RankOrderPriority of unknown rank must exceed RankOrderPriority(FORM), got %d", got)
	}
}

// TestRankOrderPriority_ExtendedRankSetPrioritized is the final-review
// Important-8 regression test: before this fix, every one of Task 1's 26
// EuroSL/GermanSL ranks above/around GENUS (ORDER, CLASS, FAMILY,
// SPECIES_AGGREGATE, ...) fell through RankOrderPriority to
// unknownRankOrder — the same bucket as RankOther — and so sorted dead
// last in /v1/suggest instead of near their taxonomically appropriate
// position. This pins that each of a representative sample now sorts
// STRICTLY BEFORE unknownRankOrder, and that the broad-to-narrow relative
// order the finding asked for holds: ROOT < FAMILY < GENUS <
// SPECIES_AGGREGATE < SPECIES < COLL_SPECIES.
func TestRankOrderPriority_ExtendedRankSetPrioritized(t *testing.T) {
	unknown := domain.RankOrderPriority(domain.RankOther)
	for _, r := range []domain.Rank{
		domain.RankRoot, domain.RankPhylum, domain.RankSubdivision, domain.RankInformalClade,
		domain.RankClass, domain.RankSubclass, domain.RankSuperorder, domain.RankOrder,
		domain.RankSubfamily, domain.RankTribe,
		domain.RankSubgenus, domain.RankSection, domain.RankSubsection, domain.RankSeries,
		domain.RankSpeciesAggregate, domain.RankGenusAggregate,
		domain.RankCollSpecies, domain.RankSubspeciesGroup, domain.RankProles, domain.RankRace,
		domain.RankConvar, domain.RankGrex, domain.RankUnrankedInfrageneric, domain.RankUnrankedInfraspecific,
	} {
		if got := domain.RankOrderPriority(r); got >= unknown {
			t.Errorf("RankOrderPriority(%s) = %d, want strictly less than unknownRankOrder (%d)", r, got, unknown)
		}
	}

	chain := []domain.Rank{
		domain.RankRoot, domain.RankFamily, domain.RankGenus,
		domain.RankSpeciesAggregate, domain.RankSpecies, domain.RankCollSpecies,
	}
	for i := 1; i < len(chain); i++ {
		if domain.RankOrderPriority(chain[i-1]) >= domain.RankOrderPriority(chain[i]) {
			t.Fatalf("RankOrderPriority must be strictly increasing along %s -> %s: %d >= %d",
				chain[i-1], chain[i], domain.RankOrderPriority(chain[i-1]), domain.RankOrderPriority(chain[i]))
		}
	}
}

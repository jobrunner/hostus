package domain_test

import (
	"testing"

	"github.com/jobrunner/hostus/internal/domain"
)

// TestResolveTargetSpace_PrefersTheRankCongruentSpelling is the fix for a
// measured defect, reproduced against the real eurosl 2024-11-03 index.
//
// Euro+Med accepts TEN names under Bromopsis erecta: the species itself plus
// nine subspecies. The crosswalk attaches several of them to WCVP's single
// Bromus erectus concept, and every one carries status "accepted" in that
// space — so the accepted-preference (see
// TestResolveTargetSpace_PrefersTheAcceptedSpelling) cannot separate them and
// the ext_id order decides. subsp. permixta's TaxonUsageID sorts first:
//
//	10d4f969-…  Bromopsis erecta subsp. permixta   Subspecies   <- was reported
//	39477ce7-…  Bromopsis erecta                   Species      <- correct
//
// /v1/translate therefore answered "Bromopsis erecta subsp. permixta" for a
// plain Bromus erectus, with requires_review false. Downstream that is not a
// cosmetic problem: expertus stores the reported spelling in the relevé and
// sends it to habitatus, so a subspecies nobody recorded enters the
// classification.
func TestResolveTargetSpace_PrefersTheRankCongruentSpelling(t *testing.T) {
	entries := []domain.NameSpaceEntry{
		{
			Space: "eurosl", ExtID: "10d4f969-7dd7-41de-bc36-afb1351068be",
			Name: "Bromopsis erecta subsp. permixta", Status: "accepted", Rank: domain.RankSubspecies,
		},
		{
			Space: "eurosl", ExtID: "39477ce7-1503-41ab-b8dc-1e71bfeb7036",
			Name: "Bromopsis erecta", Status: "accepted", Rank: domain.RankSpecies,
		},
	}
	query := domain.TargetSpaceQuery{SourceRank: domain.RankSpecies}

	choice, _ := domain.ResolveTargetSpace(query, entries)

	if choice.Name != "Bromopsis erecta" {
		t.Errorf("name = %q, want the species spelling for a species concept", choice.Name)
	}
	if choice.ExtID != "39477ce7-1503-41ab-b8dc-1e71bfeb7036" {
		t.Errorf("ext_id = %q, want the species entry's own id", choice.ExtID)
	}
}

// TestResolveTargetSpace_PrefersTheInfraspecificSpellingForAnInfraspecificSource
// pins the other direction, so the rule reads as congruence rather than as
// "species always wins": a source concept that IS a subspecies must not be
// lifted to the species spelling just because one is on offer.
func TestResolveTargetSpace_PrefersTheInfraspecificSpellingForAnInfraspecificSource(t *testing.T) {
	entries := []domain.NameSpaceEntry{
		{Space: "eurosl", ExtID: "1", Name: "Bromopsis erecta", Status: "accepted", Rank: domain.RankSpecies},
		{Space: "eurosl", ExtID: "2", Name: "Bromopsis erecta subsp. permixta", Status: "accepted", Rank: domain.RankSubspecies},
	}
	query := domain.TargetSpaceQuery{SourceRank: domain.RankSubspecies}

	choice, _ := domain.ResolveTargetSpace(query, entries)

	if choice.Name != "Bromopsis erecta subsp. permixta" {
		t.Errorf("name = %q, want the infraspecific spelling for an infraspecific concept", choice.Name)
	}
}

// TestResolveTargetSpace_RankCongruenceIsCoarse pins that congruence is
// decided in CLASSES, not by rank equality. A space is free to rank what WCVP
// calls a variety as a subspecies — that disagreement is a taxonomic opinion,
// not an error, and demanding equality would reject the only sensible
// spelling on offer. Both are infraspecific, so they are congruent.
func TestResolveTargetSpace_RankCongruenceIsCoarse(t *testing.T) {
	entries := []domain.NameSpaceEntry{
		{Space: "eurosl", ExtID: "1", Name: "Bromopsis condensata", Status: "accepted", Rank: domain.RankSpecies},
		{Space: "eurosl", ExtID: "2", Name: "Bromopsis condensata subsp. microtricha", Status: "accepted", Rank: domain.RankSubspecies},
	}
	query := domain.TargetSpaceQuery{SourceRank: domain.RankVariety}

	choice, _ := domain.ResolveTargetSpace(query, entries)

	if choice.Name != "Bromopsis condensata subsp. microtricha" {
		t.Errorf("name = %q, want the infraspecific spelling for an infraspecific source of any rank", choice.Name)
	}
}

// TestResolveTargetSpace_FallsBackWhenNoRankIsKnown pins the migration story,
// exactly as TestResolveTargetSpace_FallsBackWhenNoStatusIsKnown does for
// Status: entries ingested before the rank column carry none, and must keep
// behaving as they did rather than resolving to nothing. Re-ingest is what
// fixes them.
func TestResolveTargetSpace_FallsBackWhenNoRankIsKnown(t *testing.T) {
	entries := []domain.NameSpaceEntry{
		{Space: "eurosl", ExtID: "1", Name: "Bromopsis erecta subsp. permixta", Status: "accepted"},
		{Space: "eurosl", ExtID: "2", Name: "Bromopsis erecta", Status: "accepted"},
	}
	query := domain.TargetSpaceQuery{SourceRank: domain.RankSpecies}

	choice, _ := domain.ResolveTargetSpace(query, entries)

	if choice.Name != "Bromopsis erecta subsp. permixta" {
		t.Errorf("name = %q, want the first accepted entry as before when no rank is known", choice.Name)
	}
}

// TestResolveTargetSpace_UnknownSourceRankKeepsEveryEntryEligible pins the
// mirror case: the SOURCE rank can be absent too (RankOther covers the empty
// string, see ParseRankLenient). Filtering against an unknown source rank
// would discard every candidate, so the rule must stand down instead.
func TestResolveTargetSpace_UnknownSourceRankKeepsEveryEntryEligible(t *testing.T) {
	entries := []domain.NameSpaceEntry{
		{Space: "eurosl", ExtID: "1", Name: "Bromopsis erecta subsp. permixta", Status: "accepted", Rank: domain.RankSubspecies},
		{Space: "eurosl", ExtID: "2", Name: "Bromopsis erecta", Status: "accepted", Rank: domain.RankSpecies},
	}

	choice, _ := domain.ResolveTargetSpace(domain.TargetSpaceQuery{}, entries)

	if choice.Name != "Bromopsis erecta subsp. permixta" {
		t.Errorf("name = %q, want the first accepted entry when the source rank is unknown", choice.Name)
	}
}

// TestResolveTargetSpace_AcceptedStillOutranksRankCongruence pins the order of
// the two preferences. A space's accepted spelling is a nomenclatural fact;
// rank congruence is a tie-break among equally accepted candidates. Letting a
// rank-congruent SYNONYM beat the accepted spelling would undo the defect
// TestResolveTargetSpace_PrefersTheAcceptedSpelling fixed.
func TestResolveTargetSpace_AcceptedStillOutranksRankCongruence(t *testing.T) {
	entries := []domain.NameSpaceEntry{
		{Space: "eurosl", ExtID: "1", Name: "Bromus erectus", Status: "synonymobjective", Rank: domain.RankSpecies},
		{Space: "eurosl", ExtID: "2", Name: "Bromopsis erecta subsp. permixta", Status: "accepted", Rank: domain.RankSubspecies},
	}
	query := domain.TargetSpaceQuery{SourceRank: domain.RankSpecies}

	choice, _ := domain.ResolveTargetSpace(query, entries)

	if choice.Name != "Bromopsis erecta subsp. permixta" {
		t.Errorf("name = %q, want the accepted spelling even when a synonym is rank-congruent", choice.Name)
	}
}

// TestResolveTargetSpace_AggregateRuleIsUntouchedByRank pins that the
// aggregate branch still answers UC4's question rather than the rank
// question: an aggregate query resolves to the aggregate spelling, whose rank
// ("Species Aggregate" in Euro+Med) is congruent with no plain species.
func TestResolveTargetSpace_AggregateRuleIsUntouchedByRank(t *testing.T) {
	entries := []domain.NameSpaceEntry{
		{Space: "eurosl", ExtID: "1", Name: "Achillea millefolium", Status: "accepted", Rank: domain.RankSpecies},
		{
			Space: "eurosl", ExtID: "2", Name: "Achillea millefolium aggr.",
			Aggregate: true, Status: "accepted", Rank: domain.RankSpeciesAggregate,
		},
	}
	query := domain.TargetSpaceQuery{IsAggregate: true, SourceRank: domain.RankSpecies}

	choice, policy := domain.ResolveTargetSpace(query, entries)

	if choice.Name != "Achillea millefolium aggr." {
		t.Errorf("name = %q, want the aggregate spelling for an aggregate query", choice.Name)
	}
	if policy != domain.AggregatePolicyKnown {
		t.Errorf("policy = %q, want %q", policy, domain.AggregatePolicyKnown)
	}
}

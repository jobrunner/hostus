package domain_test

import (
	"testing"

	"github.com/jobrunner/hostus/internal/domain"
)

// brmousErectusEntries is the real eurosl 2024-11-03 situation for WCVP's
// Bromus erectus concept, reduced to the entries that decide the answer.
// Measured on a full ingest (2026-10-07): the crosswalk attaches THIRTY eurosl
// entries to this one concept, eight of them status "accepted" and belonging
// to several different Euro+Med taxa.
//
// Neither status nor rank separates those eight — Bromopsis zangezura and
// Bromopsis erecta are both accepted species — so the pick fell to ext_id
// order, which is why the rank fix alone moved the wrong answer from
// "Bromopsis erecta subsp. permixta" to "Bromopsis zangezura" instead of
// fixing it.
//
// The entry that settles it is the one Euro+Med spells exactly like the
// source concept: "Bromus erectus", which that space itself marks as a
// synonym OF Bromopsis erecta. Following that link is source evidence, not a
// heuristic over the candidate pool.
func bromusErectusEntries() []domain.NameSpaceEntry {
	return []domain.NameSpaceEntry{
		{
			Space: "eurosl", ExtID: "2a9bf23c", Name: "Bromopsis zangezura",
			Status: "accepted", Rank: domain.RankSpecies,
		},
		{
			Space: "eurosl", ExtID: "10d4f969", Name: "Bromopsis erecta subsp. permixta",
			Status: "accepted", Rank: domain.RankSubspecies,
		},
		{
			Space: "eurosl", ExtID: "39477ce7", Name: "Bromopsis erecta",
			Status: "accepted", Rank: domain.RankSpecies,
		},
		{
			Space: "eurosl", ExtID: "ebf08a09", Name: "Bromus erectus",
			Status: "synonymobjective", Rank: domain.RankSpecies,
			AcceptedName: "Bromopsis erecta",
		},
	}
}

// TestResolveTargetSpace_FollowsTheSourcesOwnSynonymyFromTheMatchingSpelling is
// the end of the Bromus erectus defect: the answer is reached through the
// space's own statement about the source's spelling, not by ranking the pool.
func TestResolveTargetSpace_FollowsTheSourcesOwnSynonymyFromTheMatchingSpelling(t *testing.T) {
	query := domain.TargetSpaceQuery{SourceName: "Bromus erectus", SourceRank: domain.RankSpecies}

	choice, _ := domain.ResolveTargetSpace(query, bromusErectusEntries())

	if choice.Name != "Bromopsis erecta" {
		t.Errorf("name = %q, want the accepted name of the taxon the space files the source's spelling under", choice.Name)
	}
	if choice.ExtID != "39477ce7" {
		t.Errorf("ext_id = %q, want the accepted entry's own id 39477ce7", choice.ExtID)
	}
	if choice.Status != "accepted" {
		t.Errorf("status = %q, want the accepted entry's status, not the synonym's", choice.Status)
	}
}

// TestResolveTargetSpace_MatchingSpellingThatIsItselfAcceptedWins pins the
// short case: when the space accepts the source's own spelling, the chain ends
// there and no synonymy hop happens.
func TestResolveTargetSpace_MatchingSpellingThatIsItselfAcceptedWins(t *testing.T) {
	entries := []domain.NameSpaceEntry{
		{Space: "eurosl", ExtID: "1", Name: "Quercus estremadurensis", Status: "accepted", Rank: domain.RankSpecies},
		{Space: "eurosl", ExtID: "2", Name: "Quercus robur", Status: "accepted", Rank: domain.RankSpecies},
	}
	query := domain.TargetSpaceQuery{SourceName: "Quercus robur", SourceRank: domain.RankSpecies}

	choice, _ := domain.ResolveTargetSpace(query, entries)

	if choice.Name != "Quercus robur" {
		t.Errorf("name = %q, want the space's own accepted spelling of the source name", choice.Name)
	}
}

// TestResolveTargetSpace_SpellingMatchIsCanonicalised pins that the anchor is
// found through domain.Canonicalize, like every other name comparison in this
// codebase — a space writing "BROMUS  ERECTUS" still anchors.
func TestResolveTargetSpace_SpellingMatchIsCanonicalised(t *testing.T) {
	entries := []domain.NameSpaceEntry{
		{Space: "eurosl", ExtID: "1", Name: "Bromopsis zangezura", Status: "accepted", Rank: domain.RankSpecies},
		{
			Space: "eurosl", ExtID: "2", Name: "BROMUS  ERECTUS",
			Status: "synonymobjective", Rank: domain.RankSpecies, AcceptedName: "bromopsis  erecta",
		},
		{Space: "eurosl", ExtID: "3", Name: "Bromopsis erecta", Status: "accepted", Rank: domain.RankSpecies},
	}
	query := domain.TargetSpaceQuery{SourceName: "Bromus erectus", SourceRank: domain.RankSpecies}

	choice, _ := domain.ResolveTargetSpace(query, entries)

	if choice.Name != "Bromopsis erecta" {
		t.Errorf("name = %q, want the anchor to survive spelling noise", choice.Name)
	}
}

// TestResolveTargetSpace_DanglingSynonymyFallsBackRatherThanReportingTheSynonym
// pins the honest failure mode. If the space files the source's spelling under
// a taxon whose accepted entry is NOT attached to this concept, the chain is
// broken — and the synonym's own spelling must not be handed back as "the name
// in that space". Falling back to the ranked pool answers with something the
// space does accept.
func TestResolveTargetSpace_DanglingSynonymyFallsBackRatherThanReportingTheSynonym(t *testing.T) {
	entries := []domain.NameSpaceEntry{
		{Space: "eurosl", ExtID: "1", Name: "Bromopsis erecta", Status: "accepted", Rank: domain.RankSpecies},
		{
			Space: "eurosl", ExtID: "2", Name: "Bromus erectus",
			Status: "synonymobjective", Rank: domain.RankSpecies,
			AcceptedName: "Bromopsis nowhere", // no entry carries this name
		},
	}
	query := domain.TargetSpaceQuery{SourceName: "Bromus erectus", SourceRank: domain.RankSpecies}

	choice, _ := domain.ResolveTargetSpace(query, entries)

	if choice.Name != "Bromopsis erecta" {
		t.Errorf("name = %q, want the ranked fallback rather than the synonym spelling", choice.Name)
	}
}

// TestResolveTargetSpace_NoAnchorKeepsTheRankedPreference pins that the anchor
// is an ADDITION, not a replacement: a concept whose name the space does not
// spell at all still resolves through accepted-then-rank-congruent as before.
func TestResolveTargetSpace_NoAnchorKeepsTheRankedPreference(t *testing.T) {
	entries := []domain.NameSpaceEntry{
		{Space: "eurosl", ExtID: "1", Name: "Bromopsis erecta subsp. permixta", Status: "accepted", Rank: domain.RankSubspecies},
		{Space: "eurosl", ExtID: "2", Name: "Bromopsis erecta", Status: "accepted", Rank: domain.RankSpecies},
	}
	query := domain.TargetSpaceQuery{SourceName: "Nothing the space spells", SourceRank: domain.RankSpecies}

	choice, _ := domain.ResolveTargetSpace(query, entries)

	if choice.Name != "Bromopsis erecta" {
		t.Errorf("name = %q, want the rank-congruent accepted entry when no anchor exists", choice.Name)
	}
}

// TestResolveTargetSpace_AnchorRespectsTheAggregateBranch pins that the anchor
// does not reach across UC4's aggregate split: an aggregate query still gets an
// aggregate spelling, and a plain query never gets one.
func TestResolveTargetSpace_AnchorRespectsTheAggregateBranch(t *testing.T) {
	entries := []domain.NameSpaceEntry{
		{
			Space: "eurosl", ExtID: "1", Name: "Festuca ovina", Aggregate: false,
			Status: "synonymobjective", Rank: domain.RankSpecies, AcceptedName: "Festuca guestfalica",
		},
		{Space: "eurosl", ExtID: "2", Name: "Festuca guestfalica", Status: "accepted", Rank: domain.RankSpecies},
		{
			Space: "eurosl", ExtID: "3", Name: "Festuca ovina aggr.", Aggregate: true,
			Status: "accepted", Rank: domain.RankSpeciesAggregate,
		},
	}
	query := domain.TargetSpaceQuery{IsAggregate: true, SourceName: "Festuca ovina", SourceRank: domain.RankSpecies}

	choice, policy := domain.ResolveTargetSpace(query, entries)

	if choice.Name != "Festuca ovina aggr." {
		t.Errorf("name = %q, want the aggregate spelling — the anchor must not cross the aggregate split", choice.Name)
	}
	if policy != domain.AggregatePolicyKnown {
		t.Errorf("policy = %q, want %q", policy, domain.AggregatePolicyKnown)
	}
}

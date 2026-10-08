package domain_test

import (
	"testing"

	"github.com/jobrunner/hostus/internal/domain"
)

// bromusErectusEntries is the real eurosl 2024-11-03 situation for WCVP's
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

// TestResolveTargetSpace_DanglingAnchorDoesNotHideALaterValidOne pins that the
// anchor inspects EVERY row carrying the source spelling instead of stopping at
// the first. A space may hold the same spelling twice under different ids — the
// entry is keyed by (Space, ExtID), not by name — and the rows arrive in ext_id
// order, so stopping early let an arbitrary one decide. Worse, a first row
// whose hop dangles suppressed a later row whose hop resolves.
//
// Measured honestly: the real eurosl/floraveg/germansl indexes carry ZERO
// concepts with a repeated spelling today (2026-10-07). This rule is therefore
// precaution, not a fix for observed data — but the order it removes from the
// decision was never evidence, and a name list is re-harvested.
func TestResolveTargetSpace_DanglingAnchorDoesNotHideALaterValidOne(t *testing.T) {
	entries := []domain.NameSpaceEntry{
		{
			Space: "eurosl", ExtID: "a-dangling", Name: "Bromus erectus",
			Status: "synonym", Rank: domain.RankSpecies, AcceptedName: "Bromopsis nowhere",
		},
		{
			Space: "eurosl", ExtID: "b-resolving", Name: "Bromus erectus",
			Status: "synonymobjective", Rank: domain.RankSpecies, AcceptedName: "Bromopsis erecta",
		},
		// Deliberately rank-INcongruent, with a congruent decoy beside it:
		// otherwise the ranked fallback would answer "Bromopsis erecta" as
		// well and this test would pass without the anchor ever working.
		{Space: "eurosl", ExtID: "c-accepted", Name: "Bromopsis erecta", Status: "accepted", Rank: domain.RankSubspecies},
		{Space: "eurosl", ExtID: "d-decoy", Name: "Bromopsis aliena", Status: "accepted", Rank: domain.RankSpecies},
	}
	query := domain.TargetSpaceQuery{SourceName: "Bromus erectus", SourceRank: domain.RankSpecies}

	choice, _ := domain.ResolveTargetSpace(query, entries)

	if choice.Name != "Bromopsis erecta" {
		t.Errorf("name = %q, want the anchor whose hop resolves rather than the first one listed (the ranked fallback would answer %q)",
			choice.Name, "Bromopsis aliena")
	}
}

// TestResolveTargetSpace_ConflictingAnchorsFallBackRatherThanGuess pins the
// other half: when two rows carry the source spelling and their hops resolve to
// DIFFERENT accepted entries, the space is telling us two incompatible things.
// Picking either by ext_id order would dress an arbitrary choice as source
// evidence, which is the very defect the anchor exists to end — so the anchor
// stands down and the ranked preferences decide.
func TestResolveTargetSpace_ConflictingAnchorsFallBackRatherThanGuess(t *testing.T) {
	entries := []domain.NameSpaceEntry{
		{
			Space: "eurosl", ExtID: "a-one", Name: "Bromus erectus",
			Status: "synonym", Rank: domain.RankSpecies, AcceptedName: "Bromopsis erecta",
		},
		{
			Space: "eurosl", ExtID: "b-two", Name: "Bromus erectus",
			Status: "synonym", Rank: domain.RankSpecies, AcceptedName: "Bromopsis zangezura",
		},
		{Space: "eurosl", ExtID: "c-sub", Name: "Bromopsis erecta", Status: "accepted", Rank: domain.RankSubspecies},
		{Space: "eurosl", ExtID: "d-zan", Name: "Bromopsis zangezura", Status: "accepted", Rank: domain.RankSpecies},
	}
	query := domain.TargetSpaceQuery{SourceName: "Bromus erectus", SourceRank: domain.RankSpecies}

	choice, _ := domain.ResolveTargetSpace(query, entries)

	// The ranked fallback: among the two accepted entries the rank-congruent
	// one wins. That it coincides with one of the conflicting hops is beside
	// the point — it is reached by the ranked rules, not by picking a hop.
	if choice.Name != "Bromopsis zangezura" {
		t.Errorf("name = %q, want the ranked fallback when two anchors disagree", choice.Name)
	}
}

// TestResolveTargetSpace_AcceptedAnchorWinsOverASiblingSynonymRow pins the
// precedence among several rows of the source spelling: one the space accepts
// outright is stronger evidence than a sibling row that only points elsewhere,
// regardless of which sorts first.
func TestResolveTargetSpace_AcceptedAnchorWinsOverASiblingSynonymRow(t *testing.T) {
	entries := []domain.NameSpaceEntry{
		{
			Space: "eurosl", ExtID: "a-synonym", Name: "Festuca ovina",
			Status: "synonym", Rank: domain.RankSpecies, AcceptedName: "Festuca guestfalica",
		},
		{Space: "eurosl", ExtID: "b-accepted", Name: "Festuca ovina", Status: "accepted", Rank: domain.RankSpecies},
		{Space: "eurosl", ExtID: "c-other", Name: "Festuca guestfalica", Status: "accepted", Rank: domain.RankSpecies},
	}
	query := domain.TargetSpaceQuery{SourceName: "Festuca ovina", SourceRank: domain.RankSpecies}

	choice, _ := domain.ResolveTargetSpace(query, entries)

	if choice.Name != "Festuca ovina" {
		t.Errorf("name = %q, want the row the space accepts outright", choice.Name)
	}
}

// TestResolveTargetSpace_DirectAnchorNeedsNoNewColumns pins the one part of
// this change that takes effect WITHOUT a re-ingest, and it exists because the
// rollout note first claimed the opposite.
//
// AcceptedInSpace() reads only Status, which shipped long before Rank and
// AcceptedName. So when a legacy row carries the source concept's own spelling
// and the space accepts it, the anchor returns it the moment the new binary
// starts — no new column involved. Measured on the real eurosl index
// (2026-10-07): 2.732 concepts hold several directly accepted entries, and for
// 1.303 of them this changes the answer away from the lowest ext_id
// immediately.
//
// That is an improvement, not a regression — the space's own spelling of the
// concept beats a row that merely sorts first — and gating it would withhold
// 1.303 correct answers for nothing, with no reliable marker to gate on
// (an accepted row legitimately has an empty accepted_name, and rank may be
// RankOther). It is documented as an exception instead; the synonym HOP, which
// is what the Bromus erectus case needs, genuinely waits for accepted_name.
func TestResolveTargetSpace_DirectAnchorNeedsNoNewColumns(t *testing.T) {
	// A legacy pool: status present, rank and accepted_name empty throughout.
	entries := []domain.NameSpaceEntry{
		{Space: "eurosl", ExtID: "a-first", Name: "Bromopsis erecta subsp. permixta", Status: "accepted"},
		{Space: "eurosl", ExtID: "b-source", Name: "Festuca ovina", Status: "accepted"},
	}
	query := domain.TargetSpaceQuery{SourceName: "Festuca ovina", SourceRank: domain.RankSpecies}

	choice, _ := domain.ResolveTargetSpace(query, entries)

	if choice.Name != "Festuca ovina" {
		t.Errorf("name = %q, want the space's own accepted spelling of the source concept — the direct anchor needs no re-ingest", choice.Name)
	}
}

// TestResolveTargetSpace_SynonymHopDoesWaitForReIngest is the counterpart that
// keeps the rollout note honest in the other direction: the hop reads
// AcceptedName, so on a legacy row it finds nothing and the ranked
// preferences answer — which is exactly why Bromus erectus keeps returning
// the old name until the name spaces are re-ingested.
func TestResolveTargetSpace_SynonymHopDoesWaitForReIngest(t *testing.T) {
	// The Bromus pool as a LEGACY index holds it: no rank, no accepted_name.
	entries := []domain.NameSpaceEntry{
		{Space: "eurosl", ExtID: "10d4f969", Name: "Bromopsis erecta subsp. permixta", Status: "accepted"},
		{Space: "eurosl", ExtID: "39477ce7", Name: "Bromopsis erecta", Status: "accepted"},
		{Space: "eurosl", ExtID: "ebf08a09", Name: "Bromus erectus", Status: "synonymobjective"},
	}
	query := domain.TargetSpaceQuery{SourceName: "Bromus erectus", SourceRank: domain.RankSpecies}

	choice, _ := domain.ResolveTargetSpace(query, entries)

	if choice.Name != "Bromopsis erecta subsp. permixta" {
		t.Errorf("name = %q, want the pre-re-ingest answer — without accepted_name the hop has nothing to follow", choice.Name)
	}
}

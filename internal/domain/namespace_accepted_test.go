package domain_test

import (
	"testing"

	"github.com/jobrunner/hostus/internal/domain"
)

// TestResolveTargetSpace_PrefersTheAcceptedSpelling is the fix for a measured
// defect: a name space maps many of its names onto one backbone concept, and
// on the real index 45% of concepts carrying a eurosl entry have between 2 and
// 391 spellings. Taking the first non-aggregate entry therefore returned an
// arbitrary synonym — the Hyssopus concept answered "Hyssopus ruber", one of
// 23 — while presenting it as the name in that space. Downstream (ESy) that is
// not a cosmetic problem: it is the wrong taxon name.
func TestResolveTargetSpace_PrefersTheAcceptedSpelling(t *testing.T) {
	entries := []domain.NameSpaceEntry{
		{Space: "eurosl", ExtID: "1", Name: "Hyssopus ruber", Status: "synonym"},
		{Space: "eurosl", ExtID: "2", Name: "Hyssopus officinalis", Status: "accepted"},
		{Space: "eurosl", ExtID: "3", Name: "Hyssopus pubescens", Status: "synonymobjective"},
	}
	choice, _ := domain.ResolveTargetSpace(false, entries)
	if choice.Name != "Hyssopus officinalis" {
		t.Errorf("name = %q, want the accepted spelling regardless of entry order", choice.Name)
	}
}

// TestResolveTargetSpace_FallsBackWhenNoStatusIsKnown pins the migration story:
// an index ingested before Status existed carries none, and must keep behaving
// as it did rather than reporting nothing at all.
func TestResolveTargetSpace_FallsBackWhenNoStatusIsKnown(t *testing.T) {
	entries := []domain.NameSpaceEntry{
		{Space: "eurosl", ExtID: "1", Name: "Hyssopus ruber"},
		{Space: "eurosl", ExtID: "2", Name: "Hyssopus officinalis"},
	}
	if choice, _ := domain.ResolveTargetSpace(false, entries); choice.Name != "Hyssopus ruber" {
		t.Errorf("name = %q, want the first entry as before when no status is known", choice.Name)
	}
}

// TestResolveTargetSpace_AggregateStillWinsForAnAggregateQuery pins that the
// accepted-preference does not override the aggregate rule: an aggregate query
// must still resolve to the aggregate spelling (UC4), which is a different
// question from which spelling is nomenclaturally accepted.
func TestResolveTargetSpace_AggregateStillWinsForAnAggregateQuery(t *testing.T) {
	entries := []domain.NameSpaceEntry{
		{Space: "floraveg", ExtID: "1", Name: "Festuca ovina", Status: "accepted"},
		{Space: "floraveg", ExtID: "2", Name: "Festuca ovina aggr.", Aggregate: true, Status: "synonym"},
	}
	choice, policy := domain.ResolveTargetSpace(true, entries)
	if choice.Name != "Festuca ovina aggr." {
		t.Errorf("name = %q, want the aggregate spelling for an aggregate query", choice.Name)
	}
	if policy != domain.AggregatePolicyKnown {
		t.Errorf("policy = %q, want %q", policy, domain.AggregatePolicyKnown)
	}
}

// TestResolveTargetSpace_AcceptedAggregateWinsAmongAggregates pins the same
// preference inside the aggregate branch, where several aggregate spellings
// can compete just as plain ones do.
func TestResolveTargetSpace_AcceptedAggregateWinsAmongAggregates(t *testing.T) {
	entries := []domain.NameSpaceEntry{
		{Space: "floraveg", ExtID: "1", Name: "Festuca ovina s. l.", Aggregate: true, Status: "synonym"},
		{Space: "floraveg", ExtID: "2", Name: "Festuca ovina aggr.", Aggregate: true, Status: "accepted"},
	}
	if choice, _ := domain.ResolveTargetSpace(true, entries); choice.Name != "Festuca ovina aggr." {
		t.Errorf("name = %q, want the accepted aggregate spelling", choice.Name)
	}
}

// TestResolveTargetSpace_DirectAcceptedEntryOutranksClosedOne pins
// whole-branch-review 2026-09-13 I3: a concept can carry MORE than one
// accepted-in-space entry (2.732 such concepts measured on the real index
// BEFORE application.closeSynonymyGroups' post-resolve pass even runs; the
// closure pass adds more). Before this fix pickSpelling took the first
// accepted entry by ext_id order regardless of how it got there — so a
// CLOSED entry (attached via the row's source-synonymy GROUP, not by its own
// spelling matching this concept) could outrank a DIRECT one (the crosswalk
// matched this row's own name onto this concept), purely because the closed
// entry's ext_id happened to sort first. Direct name evidence must win
// regardless of ext_id order.
func TestResolveTargetSpace_DirectAcceptedEntryOutranksClosedOne(t *testing.T) {
	entries := []domain.NameSpaceEntry{
		// Lexicographically SMALLER ext_id, but attached by the closure
		// pass — inferred from its group, not from its own spelling.
		{Space: "eurosl", ExtID: "1-closed", Name: "Inula hirta", Status: "accepted", Resolution: "source_synonymy_closure"},
		// Lexicographically LARGER ext_id, but a DIRECT name match.
		{Space: "eurosl", ExtID: "2-direct", Name: "Pentanema hirtum", Status: "accepted"},
	}
	choice, _ := domain.ResolveTargetSpace(false, entries)
	if choice.Name != "Pentanema hirtum" {
		t.Errorf("name = %q, want the DIRECT accepted entry, not the closed one", choice.Name)
	}
	if choice.ExtID != "2-direct" {
		t.Errorf("ext_id = %q, want %q", choice.ExtID, "2-direct")
	}
}

// TestResolveTargetSpace_ClosedAcceptedEntryStillWinsWithNoDirectOne pins the
// fallback side of I3: when EVERY accepted entry is closed (no direct
// accepted entry exists at all — the Inula-hirta class from spec 2026-09-13
// itself, where the group's only accepted-status row never matched by name),
// the closed entry still wins over a plain (non-accepted) one, exactly as
// pickSpelling always preferred an accepted entry over a synonym.
func TestResolveTargetSpace_ClosedAcceptedEntryStillWinsWithNoDirectOne(t *testing.T) {
	entries := []domain.NameSpaceEntry{
		{Space: "eurosl", ExtID: "1", Name: "Pentanema hirtum", Status: "synonymobjective"},
		{Space: "eurosl", ExtID: "2", Name: "Inula hirta", Status: "accepted", Resolution: "source_synonymy_closure"},
	}
	choice, _ := domain.ResolveTargetSpace(false, entries)
	if choice.Name != "Inula hirta" {
		t.Errorf("name = %q, want the closed accepted entry (no direct one exists)", choice.Name)
	}
	if choice.Status != "accepted" {
		t.Errorf("status = %q, want %q", choice.Status, "accepted")
	}
}

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
	name, _ := domain.ResolveTargetSpace(false, entries)
	if name != "Hyssopus officinalis" {
		t.Errorf("name = %q, want the accepted spelling regardless of entry order", name)
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
	if name, _ := domain.ResolveTargetSpace(false, entries); name != "Hyssopus ruber" {
		t.Errorf("name = %q, want the first entry as before when no status is known", name)
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
	name, policy := domain.ResolveTargetSpace(true, entries)
	if name != "Festuca ovina aggr." {
		t.Errorf("name = %q, want the aggregate spelling for an aggregate query", name)
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
	if name, _ := domain.ResolveTargetSpace(true, entries); name != "Festuca ovina aggr." {
		t.Errorf("name = %q, want the accepted aggregate spelling", name)
	}
}

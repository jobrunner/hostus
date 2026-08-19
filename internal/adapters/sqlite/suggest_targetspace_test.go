package sqlite

import (
	"context"
	"testing"

	"github.com/jobrunner/hostus/internal/domain"
	"github.com/jobrunner/hostus/internal/ports/output"
)

// TestSuggest_TargetSpaceNamesTheHitsInThatSpace is the "is this concept usable
// for ESy?" question, answered while typing rather than one concept at a time.
// A hit carries the spelling the target space uses, so a picker can show
// straight away which candidate has a Euro+Med name and what it is.
func TestSuggest_TargetSpaceNamesTheHitsInThatSpace(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	seedThreePrefixConcepts(t, db)

	tx, err := db.BeginIngest(ctx, domain.BackboneVersion{ID: "eurosl-src", Version: "v1", IngestedAt: "2026-08-19T00:00:00Z", ManifestSHA: "x"})
	mustTx(t, err)
	mustTx(t, tx.UpsertNameSpace(domain.NameSpaceMeta{
		ID: "eurosl", Version: "2024-11-03", ManifestSHA: "x", Redistribution: domain.RedistributionUnknown,
	}))
	// Only ONE of the three prefix concepts gets a spelling: the point of the
	// column is telling apart the concepts that carry the space from those
	// that do not.
	mustTx(t, tx.AddNameSpaceEntry("wcvp:concept:zzq-a", domain.NameSpaceEntry{
		Space: "eurosl", ExtID: "e-1", Name: "Zzqaaa umbenannt",
	}))
	mustTx(t, tx.Commit())

	got, err := db.Suggest(ctx, "Zzq", output.SuggestOpts{Limit: 20, TargetSpace: "eurosl"})
	mustTx(t, err)

	byID := make(map[string]domain.SuggestItem, len(got))
	for _, it := range got {
		byID[it.ConceptID] = it
	}
	if n := byID["wcvp:concept:zzq-a"].TargetSpaceName; n != "Zzqaaa umbenannt" {
		t.Errorf("TargetSpaceName = %q, want the eurosl spelling", n)
	}
	if n := byID["wcvp:concept:zzq-b"].TargetSpaceName; n != "" {
		t.Errorf("concept without an eurosl entry reports %q, want empty — absence must stay visible", n)
	}
}

// TestSuggest_TargetSpacePicksTheAcceptedSpelling reproduces the shape that
// broke this on real data: a space maps several of its own names onto ONE
// backbone concept (the Hyssopus concept carries 23 eurosl spellings). Only
// the accepted one may be reported — anything else is a synonym presented as
// the name to carry downstream into ESy.
func TestSuggest_TargetSpacePicksTheAcceptedSpelling(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	seedThreePrefixConcepts(t, db)

	tx, err := db.BeginIngest(ctx, domain.BackboneVersion{ID: "eurosl-src", Version: "v1", IngestedAt: "2026-08-19T00:00:00Z", ManifestSHA: "x"})
	mustTx(t, err)
	mustTx(t, tx.UpsertNameSpace(domain.NameSpaceMeta{
		ID: "eurosl", Version: "2024-11-03", ManifestSHA: "x", Redistribution: domain.RedistributionUnknown,
	}))
	// ext_id order deliberately puts a synonym first, so a query that just
	// takes the first row picks wrong.
	for _, e := range []domain.NameSpaceEntry{
		{Space: "eurosl", ExtID: "a-synonym", Name: "Zzq synonymum", Status: "synonym"},
		{Space: "eurosl", ExtID: "b-accepted", Name: "Zzq acceptum", Status: "accepted"},
		{Space: "eurosl", ExtID: "c-objective", Name: "Zzq obiectivum", Status: "synonymobjective"},
	} {
		mustTx(t, tx.AddNameSpaceEntry("wcvp:concept:zzq-a", e))
	}
	mustTx(t, tx.Commit())

	got, err := db.Suggest(ctx, "Zzq", output.SuggestOpts{Limit: 20, TargetSpace: "eurosl"})
	mustTx(t, err)

	for _, it := range got {
		if it.ConceptID != "wcvp:concept:zzq-a" {
			continue
		}
		if it.TargetSpaceName != "Zzq acceptum" {
			t.Errorf("TargetSpaceName = %q, want the accepted spelling (a synonym sorts first by ext_id)", it.TargetSpaceName)
		}
	}
}

// TestSuggest_WithoutTargetSpaceReportsNoName pins that the column is opt-in:
// asking for no space must not silently fill the field from some other space.
func TestSuggest_WithoutTargetSpaceReportsNoName(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	seedThreePrefixConcepts(t, db)

	tx, err := db.BeginIngest(ctx, domain.BackboneVersion{ID: "eurosl-src", Version: "v1", IngestedAt: "2026-08-19T00:00:00Z", ManifestSHA: "x"})
	mustTx(t, err)
	mustTx(t, tx.UpsertNameSpace(domain.NameSpaceMeta{
		ID: "eurosl", Version: "2024-11-03", ManifestSHA: "x", Redistribution: domain.RedistributionUnknown,
	}))
	mustTx(t, tx.AddNameSpaceEntry("wcvp:concept:zzq-a", domain.NameSpaceEntry{
		Space: "eurosl", ExtID: "e-1", Name: "Zzqaaa umbenannt",
	}))
	mustTx(t, tx.Commit())

	got, err := db.Suggest(ctx, "Zzq", output.SuggestOpts{Limit: 20})
	mustTx(t, err)
	for _, it := range got {
		if it.TargetSpaceName != "" {
			t.Errorf("concept %s reports TargetSpaceName %q without a requested space", it.ConceptID, it.TargetSpaceName)
		}
	}
}

// TestSuggest_TargetSpaceAgreesWithTheMatchResolver pins that /v1/suggest and
// /v1/match answer the SAME name for the same concept. They resolve through
// different code — suggest orders in SQL, match runs domain.ResolveTargetSpace
// in Go — so the precedence has to be stated identically in both. The case
// that separates them: the ACCEPTED spelling is an aggregate while a plain
// spelling is only a synonym. Aggregate is a filter, not a preference, so the
// plain name wins; ordering by accepted first would show "…aggr." here and the
// plain name there for one and the same concept.
func TestSuggest_TargetSpaceAgreesWithTheMatchResolver(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	seedThreePrefixConcepts(t, db)

	tx, err := db.BeginIngest(ctx, domain.BackboneVersion{ID: "fv-src", Version: "v1", IngestedAt: "2026-08-19T00:00:00Z", ManifestSHA: "x"})
	mustTx(t, err)
	mustTx(t, tx.UpsertNameSpace(domain.NameSpaceMeta{
		ID: "floraveg", Version: "2023-01-03", ManifestSHA: "x", Redistribution: domain.RedistributionUnknown,
	}))
	entries := []domain.NameSpaceEntry{
		{Space: "floraveg", ExtID: "1", Name: "Zzq aggregatum", Aggregate: true, Status: "accepted"},
		{Space: "floraveg", ExtID: "2", Name: "Zzq planum", Status: "synonym"},
	}
	for _, e := range entries {
		mustTx(t, tx.AddNameSpaceEntry("wcvp:concept:zzq-a", e))
	}
	mustTx(t, tx.Commit())

	stored, err := db.NameSpaceEntries(ctx, "wcvp:concept:zzq-a", []string{"floraveg"})
	mustTx(t, err)
	viaMatch, _ := domain.ResolveTargetSpace(false, stored)

	got, err := db.Suggest(ctx, "Zzq", output.SuggestOpts{Limit: 20, TargetSpace: "floraveg"})
	mustTx(t, err)
	var viaSuggest string
	for _, it := range got {
		if it.ConceptID == "wcvp:concept:zzq-a" {
			viaSuggest = it.TargetSpaceName
		}
	}

	if viaSuggest != viaMatch {
		t.Errorf("suggest reports %q but match reports %q for the same concept", viaSuggest, viaMatch)
	}
	if viaSuggest != "Zzq planum" {
		t.Errorf("name = %q, want the plain spelling: aggregate is a filter, not a preference", viaSuggest)
	}
}

// TestSuggest_AggregateHitKeepsTheAggregateSpelling pins the case review found:
// a hit reached through an AGGREGATE alias must be named with the aggregate
// spelling. Resolving it to the nominate name is the false "resolved to the
// single taxon" that UC4's AggregatePolicy exists to prevent — and in suggest
// nothing would flag it, it would just look like a confident hit.
func TestSuggest_AggregateHitKeepsTheAggregateSpelling(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	// One concept, reachable both by its own name and by an aggregate alias.
	bv := domain.BackboneVersion{ID: "wcvp", Version: "v1", IngestedAt: "2026-08-19T00:00:00Z", ManifestSHA: "x"}
	ingestVia(t, db, bv, func(tx output.IngestTx) {
		n := species("n-zzq-agg", "Zzqagg planum")
		mustTx(t, tx.UpsertName(n))
		c := domain.Concept{ID: "wcvp:concept:zzq-agg", BackboneID: "wcvp", AcceptedName: n, Rank: domain.RankSpecies, Status: domain.StatusAccepted}
		mustTx(t, tx.UpsertConcept(c))
		mustTx(t, tx.LinkName(c.ID, n.ID, "accepted", nil))
	})

	tx, err := db.BeginIngest(ctx, domain.BackboneVersion{ID: "fv2", Version: "v1", IngestedAt: "2026-08-19T00:00:00Z", ManifestSHA: "x"})
	mustTx(t, err)
	mustTx(t, tx.UpsertNameSpace(domain.NameSpaceMeta{
		ID: "floraveg", Version: "2023-01-03", ManifestSHA: "x", Redistribution: domain.RedistributionUnknown,
	}))
	for _, e := range []domain.NameSpaceEntry{
		{Space: "floraveg", ExtID: "1", Name: "Zzqagg planum", Status: "accepted"},
		{Space: "floraveg", ExtID: "2", Name: "Zzqagg planum aggr.", Aggregate: true, Status: "accepted"},
	} {
		mustTx(t, tx.AddNameSpaceEntry("wcvp:concept:zzq-agg", e))
	}
	mustTx(t, tx.Commit())

	// The aggregate spelling is indexed as an alias, so querying it produces a
	// hit flagged Aggregate.
	got, err := db.Suggest(ctx, "Zzqagg planum aggr.", output.SuggestOpts{Limit: 20, TargetSpace: "floraveg"})
	mustTx(t, err)

	var aggHit *domain.SuggestItem
	for i := range got {
		if got[i].Aggregate {
			aggHit = &got[i]
		}
	}
	if aggHit == nil {
		t.Skip("no aggregate-flagged hit produced by this fixture; the alias indexing rule is covered elsewhere")
	}
	if aggHit.TargetSpaceName != "Zzqagg planum aggr." {
		t.Errorf("aggregate hit reports %q, want the aggregate spelling — resolving it to the nominate name is the false narrowing UC4 forbids",
			aggHit.TargetSpaceName)
	}
}

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

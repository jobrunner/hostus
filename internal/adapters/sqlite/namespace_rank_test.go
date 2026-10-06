package sqlite

import (
	"context"
	"testing"

	"github.com/jobrunner/hostus/internal/domain"
)

// TestNameSpaceEntries_RoundTripsTheRank pins the storage half of the rank
// fix. domain.ResolveTargetSpace can only prefer a rank-congruent spelling if
// the rank survives write→read; dropping it here is exactly how the defect
// arose in the first place (the ingest read the source's rank column and then
// built a NameSpaceEntry without it).
func TestNameSpaceEntries_RoundTripsTheRank(t *testing.T) {
	db := openSeededDB(t)
	seedRankedEntries(t, db)

	got, err := db.NameSpaceEntries(context.Background(), corynephorusID, []string{"eurosl"})
	if err != nil {
		t.Fatalf("NameSpaceEntries: unexpected error: %v", err)
	}
	want := []domain.NameSpaceEntry{
		{
			Space: "eurosl", ExtID: "10d4f969", Name: "Bromopsis erecta subsp. permixta",
			Status: "accepted", Rank: domain.RankSubspecies,
		},
		{
			Space: "eurosl", ExtID: "39477ce7", Name: "Bromopsis erecta",
			Status: "accepted", Rank: domain.RankSpecies,
		},
	}
	if len(got) != len(want) {
		t.Fatalf("NameSpaceEntries = %d entries, want %d (%+v)", len(got), len(want), got)
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("entry %d = %+v, want %+v", i, got[i], w)
		}
	}
}

// TestNameSpaceEntries_MissingRankReadsBackEmpty pins the migration story at
// the storage layer, as TestNameSpaceEntries_ExactMatchStoresNullResolution
// does for resolution: an entry written without a rank reads back with the
// zero value, which ResolveTargetSpace treats as "unknown" and stands its
// congruence rule down for.
func TestNameSpaceEntries_MissingRankReadsBackEmpty(t *testing.T) {
	db := openSeededDB(t)
	seedFloraVegEntries(t, db)

	got, err := db.NameSpaceEntries(context.Background(), corynephorusID, []string{"floraveg"})
	if err != nil {
		t.Fatalf("NameSpaceEntries: unexpected error: %v", err)
	}
	if len(got) == 0 {
		t.Fatal("NameSpaceEntries returned nothing for the seeded concept")
	}
	for _, e := range got {
		if e.Rank != "" {
			t.Errorf("entry %s rank = %q, want the zero value for an entry written without one", e.ExtID, e.Rank)
		}
	}
}

// seedRankedEntries writes the two Euro+Med spellings the Bromus erectus
// defect turns on: the accepted species and one of its accepted subspecies,
// the subspecies first so that ext_id order alone would pick it.
func seedRankedEntries(t *testing.T, db *DB) {
	t.Helper()
	ctx := context.Background()

	tx, err := db.BeginIngest(ctx, seedBackboneVersion)
	if err != nil {
		t.Fatalf("BeginIngest: unexpected error: %v", err)
	}
	if err := tx.UpsertNameSpace(domain.NameSpaceMeta{
		ID: "eurosl", Version: "2024-11-03",
		SourceURL:   "https://example.org/eurosl",
		ManifestSHA: "deadbeef", Redistribution: domain.RedistributionUnknown,
	}); err != nil {
		t.Fatalf("UpsertNameSpace: unexpected error: %v", err)
	}
	entries := []domain.NameSpaceEntry{
		{
			Space: "eurosl", ExtID: "10d4f969", Name: "Bromopsis erecta subsp. permixta",
			Status: "accepted", Rank: domain.RankSubspecies,
		},
		{
			Space: "eurosl", ExtID: "39477ce7", Name: "Bromopsis erecta",
			Status: "accepted", Rank: domain.RankSpecies,
		},
	}
	for _, e := range entries {
		if err := tx.AddNameSpaceEntry(corynephorusID, e); err != nil {
			t.Fatalf("AddNameSpaceEntry(%s): unexpected error: %v", e.ExtID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: unexpected error: %v", err)
	}
}

package sqlite

import (
	"context"
	"testing"

	"github.com/jobrunner/hostus/internal/domain"
)

// seedEuroslCrosswalkFixture builds, on top of openSeededDB's WCVP concept
// (corynephorusID), exactly one Fall-A row, one Fall-B native concept, and
// one concept_aggregate edge — enough for all three new query methods to
// each return exactly one row.
func seedEuroslCrosswalkFixture(t *testing.T, db *DB) {
	t.Helper()
	ctx := context.Background()

	tx, err := db.BeginIngest(ctx, domain.BackboneVersion{
		ID: "eurosl", Version: "2026-08-27", IngestedAt: "2026-08-27T00:00:00Z",
		ManifestSHA: "deadbeef",
	})
	if err != nil {
		t.Fatalf("BeginIngest: unexpected error: %v", err)
	}
	if err := tx.UpsertNameSpace(domain.NameSpaceMeta{
		ID: "eurosl", Version: "2026-08-27", ManifestSHA: "deadbeef",
		Redistribution: domain.RedistributionUnknown,
	}); err != nil {
		t.Fatalf("UpsertNameSpace: unexpected error: %v", err)
	}
	// Fall A: a plain eurosl spelling of the already-seeded WCVP concept.
	if err := tx.AddNameSpaceEntry(corynephorusID, domain.NameSpaceEntry{
		Space: "eurosl", ExtID: "e1", Name: "Corynephorus canescens",
	}); err != nil {
		t.Fatalf("AddNameSpaceEntry: unexpected error: %v", err)
	}
	// Fall B: a native eurosl aggregate concept, own name, own id.
	aggName := domain.Name{
		ID: "eurosl:name:agg1", Canonical: "Corynephorus canescens agg.",
		Rank: domain.RankSpeciesAggregate,
	}
	aggConcept := domain.Concept{
		ID: "eurosl:concept:agg1", BackboneID: "eurosl", AcceptedName: aggName,
		Rank: domain.RankSpeciesAggregate, Status: domain.StatusAccepted,
	}
	if err := tx.UpsertName(aggName); err != nil {
		t.Fatalf("UpsertName: unexpected error: %v", err)
	}
	if err := tx.UpsertConcept(aggConcept); err != nil {
		t.Fatalf("UpsertConcept: unexpected error: %v", err)
	}
	if err := tx.LinkName(aggConcept.ID, aggName.ID, "accepted", nil); err != nil {
		t.Fatalf("LinkName: unexpected error: %v", err)
	}
	// concept_aggregate edge: the native aggregate's WCVP member.
	if err := tx.AddAggregateMember(aggConcept.ID, corynephorusID); err != nil {
		t.Fatalf("AddAggregateMember: unexpected error: %v", err)
	}
	if err := tx.Finalize(); err != nil {
		t.Fatalf("Finalize: unexpected error: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: unexpected error: %v", err)
	}
}

func TestEuroslCrosswalkEntries_ReturnsFallARows(t *testing.T) {
	db := openSeededDB(t)
	seedEuroslCrosswalkFixture(t, db)

	got, err := db.EuroslCrosswalkEntries(context.Background())
	if err != nil {
		t.Fatalf("EuroslCrosswalkEntries: unexpected error: %v", err)
	}
	want := []CrosswalkEntry{{Name: "Corynephorus canescens", ConceptID: corynephorusID}}
	if len(got) != 1 || got[0] != want[0] {
		t.Errorf("EuroslCrosswalkEntries = %+v, want %+v", got, want)
	}
}

func TestEuroslCrosswalkEntries_NoEuroslEntries_ReturnsEmptyNotNil(t *testing.T) {
	db := openSeededDB(t)

	got, err := db.EuroslCrosswalkEntries(context.Background())
	if err != nil {
		t.Fatalf("EuroslCrosswalkEntries: unexpected error: %v", err)
	}
	if got == nil || len(got) != 0 {
		t.Errorf("EuroslCrosswalkEntries = %+v, want an empty, non-nil slice", got)
	}
}

func TestNativeEuroslConcepts_ReturnsFallBRows(t *testing.T) {
	db := openSeededDB(t)
	seedEuroslCrosswalkFixture(t, db)

	got, err := db.NativeEuroslConcepts(context.Background())
	if err != nil {
		t.Fatalf("NativeEuroslConcepts: unexpected error: %v", err)
	}
	want := []CrosswalkEntry{{Name: "Corynephorus canescens agg.", ConceptID: "eurosl:concept:agg1"}}
	if len(got) != 1 || got[0] != want[0] {
		t.Errorf("NativeEuroslConcepts = %+v, want %+v", got, want)
	}
}

func TestAllAggregateMembers_ReturnsJoinedMemberName(t *testing.T) {
	db := openSeededDB(t)
	seedEuroslCrosswalkFixture(t, db)

	got, err := db.AllAggregateMembers(context.Background())
	if err != nil {
		t.Fatalf("AllAggregateMembers: unexpected error: %v", err)
	}
	want := []AggregateMemberRow{{
		AggregateConceptID: "eurosl:concept:agg1",
		MemberConceptID:    corynephorusID,
		MemberName:         "corynephorus canescens",
	}}
	if len(got) != 1 || got[0] != want[0] {
		t.Errorf("AllAggregateMembers = %+v, want %+v", got, want)
	}
}

func TestAllAggregateMembers_NoAggregates_ReturnsEmptyNotNil(t *testing.T) {
	db := openSeededDB(t)

	got, err := db.AllAggregateMembers(context.Background())
	if err != nil {
		t.Fatalf("AllAggregateMembers: unexpected error: %v", err)
	}
	if got == nil || len(got) != 0 {
		t.Errorf("AllAggregateMembers = %+v, want an empty, non-nil slice", got)
	}
}

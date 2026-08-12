package sqlite

import (
	"context"
	"testing"

	"github.com/jobrunner/hostus/internal/domain"
)

// TestAreas_ListsCodesWithDataAndNames exercises Areas + UpsertArea directly
// (the application-package ingest test covers the wiring; this covers the
// adapter's own query/guard branches). seed.sql distributes corynephorus to
// GER and FRA with no names; UpsertArea then names GER, and an empty-name call
// is a no-op.
func TestAreas_ListsCodesWithDataAndNames(t *testing.T) {
	db := openSeededDB(t)
	ctx := context.Background()

	// Before naming: both codes present, names empty (COALESCE NULL -> "").
	before, err := db.Areas(ctx)
	if err != nil {
		t.Fatalf("Areas: %v", err)
	}
	want := []domain.Area{
		{Scheme: "wgsrpd_l3", Code: "FRA", Name: ""},
		{Scheme: "wgsrpd_l3", Code: "GER", Name: ""},
	}
	if len(before) != len(want) {
		t.Fatalf("Areas = %+v, want %+v", before, want)
	}
	for i, w := range want {
		if before[i] != w {
			t.Errorf("Areas[%d] = %+v, want %+v", i, before[i], w)
		}
	}

	// Name GER; an empty-name UpsertArea writes nothing (guard); a code with no
	// distribution data must not appear in Areas even if named.
	tx, err := db.BeginIngest(ctx, seedBackboneVersion)
	if err != nil {
		t.Fatalf("BeginIngest: %v", err)
	}
	if err := tx.UpsertArea(domain.Area{Scheme: "wgsrpd_l3", Code: "GER", Name: "Germany"}); err != nil {
		t.Fatalf("UpsertArea(GER): %v", err)
	}
	if err := tx.UpsertArea(domain.Area{Scheme: "wgsrpd_l3", Code: "FRA", Name: ""}); err != nil {
		t.Fatalf("UpsertArea(empty name): %v", err)
	}
	if err := tx.UpsertArea(domain.Area{Scheme: "wgsrpd_l3", Code: "NODATA", Name: "Nowhere"}); err != nil {
		t.Fatalf("UpsertArea(NODATA): %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	after, err := db.Areas(ctx)
	if err != nil {
		t.Fatalf("Areas after naming: %v", err)
	}
	wantAfter := []domain.Area{
		{Scheme: "wgsrpd_l3", Code: "FRA", Name: ""},        // empty-name UpsertArea was a no-op
		{Scheme: "wgsrpd_l3", Code: "GER", Name: "Germany"}, // named
		// NODATA is absent: named but carries no distribution row.
	}
	if len(after) != len(wantAfter) {
		t.Fatalf("Areas after = %+v, want %+v", after, wantAfter)
	}
	for i, w := range wantAfter {
		if after[i] != w {
			t.Errorf("Areas after[%d] = %+v, want %+v", i, after[i], w)
		}
	}
}

// TestAreas_ClosedDBSurfacesError pins the query and Exec error paths that no
// happy-path test reaches (same technique as the name-space test).
func TestAreas_ClosedDBSurfacesError(t *testing.T) {
	db := openSeededDB(t)
	ctx := context.Background()

	// UpsertArea against a finished tx surfaces the Exec error path.
	tx, err := db.BeginIngest(ctx, seedBackboneVersion)
	if err != nil {
		t.Fatalf("BeginIngest: %v", err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatalf("Rollback: %v", err)
	}
	if err := tx.UpsertArea(domain.Area{Scheme: "wgsrpd_l3", Code: "GER", Name: "Germany"}); err == nil {
		t.Error("UpsertArea after rollback: want an error, got nil")
	}

	// Areas on a closed database surfaces the query error path.
	if err := db.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, err := db.Areas(ctx); err == nil {
		t.Error("Areas on a closed database: want an error, got nil")
	}
}

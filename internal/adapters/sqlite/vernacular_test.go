package sqlite

import (
	"context"
	"reflect"
	"testing"

	"github.com/jobrunner/hostus/internal/domain"
)

// TestVernacularNames_ReturnsOrderedByLangThenName pins VernacularNames'
// (lang, name) ordering and its empty-non-nil-slice contract for a concept
// with none — this function had no test coverage at all before.
func TestVernacularNames_ReturnsOrderedByLangThenName(t *testing.T) {
	db := openSeededDB(t)
	ctx := context.Background()

	tx, err := db.BeginIngest(ctx, seedBackboneVersion)
	if err != nil {
		t.Fatalf("BeginIngest: unexpected error: %v", err)
	}
	names := []domain.VernacularName{
		{Language: "en", Name: "Gray hair-grass"},
		{Language: "de", Name: "Silbergras"},
		{Language: "de", Name: "Grau-Silbergras"},
	}
	for _, v := range names {
		if err := tx.AddVernacularName(corynephorusID, v); err != nil {
			t.Fatalf("AddVernacularName(%+v): unexpected error: %v", v, err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: unexpected error: %v", err)
	}

	got, err := db.VernacularNames(ctx, corynephorusID)
	if err != nil {
		t.Fatalf("VernacularNames: unexpected error: %v", err)
	}
	want := []domain.VernacularName{
		{Language: "de", Name: "Grau-Silbergras"},
		{Language: "de", Name: "Silbergras"},
		{Language: "en", Name: "Gray hair-grass"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("VernacularNames = %+v, want %+v (ordered by lang, name)", got, want)
	}

	empty, err := db.VernacularNames(ctx, jacobaeaID)
	if err != nil {
		t.Fatalf("VernacularNames(no vernacular names): unexpected error: %v", err)
	}
	if empty == nil || len(empty) != 0 {
		t.Errorf("VernacularNames(no vernacular names) = %+v, want an empty, non-nil slice", empty)
	}
}

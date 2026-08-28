package sqlite_test

import (
	"context"
	"strings"
	"testing"

	"github.com/jobrunner/hostus/internal/ports/output"
)

// TestSuggest_NameStartExcludesEpithetMatch pins the SP7 finding: the
// default (name_start) match mode must not surface a concept whose ONLY
// matching name is reached via a species epithet token (e.g. "ca" matching
// "canescens" inside "Corynephorus canescens"), since the genus itself does
// not start with the query prefix.
func TestSuggest_NameStartExcludesEpithetMatch(t *testing.T) {
	db := ingestWCVPFixture(t)
	ctx := context.Background()

	got, err := db.Suggest(ctx, "ca", output.SuggestOpts{Limit: 50, MatchMode: "name_start"})
	if err != nil {
		t.Fatalf("Suggest: unexpected error: %v", err)
	}
	for _, item := range got {
		if strings.HasPrefix(strings.ToLower(item.Canonical), "corynephorus") {
			t.Errorf("Suggest(%q, name_start) returned %q, want no epithet-only match", "ca", item.Canonical)
		}
	}
}

// TestSuggest_AnywhereStillMatchesEpithet pins that explicitly requesting
// "anywhere" preserves today's behavior: an epithet-only token match still
// surfaces the concept.
func TestSuggest_AnywhereStillMatchesEpithet(t *testing.T) {
	db := ingestWCVPFixture(t)
	ctx := context.Background()

	got, err := db.Suggest(ctx, "canescens", output.SuggestOpts{Limit: 50, MatchMode: "anywhere"})
	if err != nil {
		t.Fatalf("Suggest: unexpected error: %v", err)
	}
	if len(conceptIDs(got)) == 0 {
		t.Error(`Suggest("canescens", anywhere) = empty, want Corynephorus canescens via epithet match`)
	}
}

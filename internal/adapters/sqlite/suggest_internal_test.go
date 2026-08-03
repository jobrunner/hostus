package sqlite

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/jobrunner/hostus/internal/domain"
	"github.com/jobrunner/hostus/internal/ports/output"
)

// TestFetchBudget_ExactValues pins fetchBudget's arithmetic and both of its
// branch boundaries with exact expected numbers (not just "some value >=
// Limit"), since only asserting inequalities left the limit<=0 clamp and
// the multiply-then-floor arithmetic unverified — gremlins' mutation run
// found exactly that gap (see the task report for the full mutation
// summary): a limit<=0 clamp mutated to "always skip" is unobservable
// through an inequality-only assertion because the post-clamp floor logic
// happens to produce a value >= Limit either way, but it changes the EXACT
// number (80 with the clamp vs 20 without it for limit=0), which this
// table catches.
//
// limit=5 is a deliberate exception: suggestFetchFloor is 20 and
// 5*suggestFetchMultiplier(4) is exactly 20, so the n > suggestFetchFloor
// boundary's two branches ("return n" vs "return suggestFetchFloor")
// return the identical number at that one input — a genuinely equivalent
// mutant for CONDITIONALS_BOUNDARY at that comparison (documented in the
// task report), not a gap this test could ever close.
func TestFetchBudget_ExactValues(t *testing.T) {
	cases := []struct {
		limit int
		want  int
	}{
		{-5, 80}, // clamped to suggestFetchFloor(20), then *4
		{0, 80},  // clamped to suggestFetchFloor(20), then *4
		{1, 20},  // 1*4=4 < floor(20) -> floor wins
		{5, 20},  // 5*4=20 == floor(20) -> equivalent-mutant boundary, see doc above
		{10, 40}, // 10*4=40 > floor(20) -> the multiplied value wins
	}
	for _, tc := range cases {
		if got := fetchBudget(tc.limit); got != tc.want {
			t.Errorf("fetchBudget(%d) = %d, want %d", tc.limit, got, tc.want)
		}
	}
}

// TestFtsPrefixToken_TwoRuneBoundary pins the exact minQueryRunes boundary:
// a 1-rune canonicalized query returns "" (too short), a 2-rune one does
// not. Only ever testing a 1-rune query and separately a much-longer one
// left the exact boundary (2 runes, the smallest ACCEPTED length)
// unverified.
func TestFtsPrefixToken_TwoRuneBoundary(t *testing.T) {
	if got := ftsPrefixToken("a"); got != "" {
		t.Errorf(`ftsPrefixToken("a") = %q, want "" (1 rune, below minQueryRunes)`, got)
	}
	if got := ftsPrefixToken("ab"); got == "" {
		t.Errorf(`ftsPrefixToken("ab") = "", want non-empty (2 runes, exactly minQueryRunes)`)
	}
}

// seedCorynephorusConcept ingests one accepted concept ("Corynephorus
// canescens") with one synonym ("Weingaertneria canescens") and calls
// Finalize, so tests can assert directly on fts_name/fts_name_map row
// counts without going through application.Ingest (which would require
// importing internal/application here and creating an import cycle,
// since application imports this package).
func seedCorynephorusConcept(t *testing.T, db *DB) (conceptID string) {
	t.Helper()
	ctx := context.Background()
	tx, err := db.BeginIngest(ctx, domain.BackboneVersion{ID: "wcvp", Version: "v1", IngestedAt: "2026-07-31T00:00:00Z", ManifestSHA: "x"})
	if err != nil {
		t.Fatalf("BeginIngest: %v", err)
	}

	accepted := domain.Name{ID: "n-corynephorus-canescens", Canonical: "Corynephorus canescens", Rank: domain.RankSpecies}
	synonym := domain.Name{ID: "n-weingaertneria-canescens", Canonical: "Weingaertneria canescens", Rank: domain.RankSpecies}
	concept := domain.Concept{ID: "c-corynephorus-canescens", BackboneID: "wcvp", AcceptedName: accepted, Rank: domain.RankSpecies, Status: domain.StatusAccepted}

	if err := tx.UpsertName(accepted); err != nil {
		t.Fatalf("UpsertName(accepted): %v", err)
	}
	if err := tx.UpsertName(synonym); err != nil {
		t.Fatalf("UpsertName(synonym): %v", err)
	}
	if err := tx.UpsertConcept(concept); err != nil {
		t.Fatalf("UpsertConcept: %v", err)
	}
	if err := tx.LinkName(concept.ID, accepted.ID, "accepted", nil); err != nil {
		t.Fatalf("LinkName(accepted): %v", err)
	}
	if err := tx.LinkName(concept.ID, synonym.ID, "synonym", nil); err != nil {
		t.Fatalf("LinkName(synonym): %v", err)
	}
	if err := tx.Finalize(); err != nil {
		t.Fatalf("Finalize: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	return concept.ID
}

func TestFinalize_IndexesAcceptedAndSynonymNames(t *testing.T) {
	db := openTestDB(t)
	conceptID := seedCorynephorusConcept(t, db)

	if got, want := rowCount(t, db, "fts_name"), 2; got != want {
		t.Fatalf("fts_name row count = %d, want %d (one per accepted+synonym name)", got, want)
	}
	if got, want := rowCount(t, db, "fts_name_map"), 2; got != want {
		t.Fatalf("fts_name_map row count = %d, want %d", got, want)
	}

	var n int
	if err := db.sql.QueryRow(`
		SELECT count(*) FROM fts_name_map WHERE concept_id = ?`, conceptID).Scan(&n); err != nil {
		t.Fatalf("querying fts_name_map: %v", err)
	}
	if n != 2 {
		t.Fatalf("fts_name_map rows mapped to %q = %d, want 2 (both accepted and synonym map to the same accepted concept)", conceptID, n)
	}
}

// TestFinalize_ReingestAppendsRatherThanReplaces pins down the documented
// limitation in Finalize's doc comment: fts_name is a contentless FTS5
// table (content=”, schema.sql) and rejects plain DELETE, so Finalize
// cannot clean up a backbone's previously-indexed rows before re-adding
// them on a re-ingest (BeginIngest itself is INSERT OR REPLACE, a
// supported operation). Suggest still behaves correctly afterward (it
// GROUP BYs on tc.id), so this only pins the index-size side effect, not a
// correctness bug — if a future schema revision adds contentless_delete=1
// and Finalize starts cleaning up first, this test's expected count should
// drop back to 2 and the doc comment above should be updated to match.
func TestFinalize_ReingestAppendsRatherThanReplaces(t *testing.T) {
	db := openTestDB(t)
	seedCorynephorusConcept(t, db)
	seedCorynephorusConcept(t, db)

	if got, want := rowCount(t, db, "fts_name"), 4; got != want {
		t.Fatalf("fts_name row count after re-ingest = %d, want %d (each Finalize call appends; see its doc comment)", got, want)
	}
	if got, want := rowCount(t, db, "fts_name_map"), 4; got != want {
		t.Fatalf("fts_name_map row count after re-ingest = %d, want %d", got, want)
	}
}

// seedFetchBudgetOverflowFixture seeds noiseCount "noise" concepts sharing
// the "zzqfiltest" prefix with a short 2-word canonical (a good, tightly
// clustered bm25 score against that prefix), plus exactly one additional
// "target" concept that also matches the prefix but whose canonical is
// padded with a large number of unrelated filler words. FTS5's bm25 is
// normalized by document length (the well-known BM25 length-normalization
// term), so padding the target's document far past every noise document's
// length reliably gives it the worst (largest/least-negative) bm25 score
// of the whole set — it sorts dead last under a bm25-only ORDER BY. The
// target is the only concept in this fixture with a distribution row (in
// areaCode), so it is the only in_area candidate. Returns the target
// concept's ID.
//
// This reproduces the Fix-A bug: with noiseCount chosen so noiseCount+1
// exceeds fetchBudget(limit), a bm25-only "ORDER BY score ASC LIMIT
// budget" truncates the SQL result set before the target row (dead last by
// score) ever reaches it, even though spec §B.1 ranks in_area (priority 2)
// above bm25 score (priority 5) — the target should survive into the
// candidate set and be surfaced, not silently dropped by the SQL layer.
func seedFetchBudgetOverflowFixture(t *testing.T, db *DB, noiseCount int, areaCode string) (targetConceptID string) {
	t.Helper()
	ctx := context.Background()
	tx, err := db.BeginIngest(ctx, domain.BackboneVersion{ID: "wcvp", Version: "v1", IngestedAt: "2026-08-01T00:00:00Z", ManifestSHA: "x"})
	if err != nil {
		t.Fatalf("BeginIngest: %v", err)
	}

	for i := 0; i < noiseCount; i++ {
		id := fmt.Sprintf("c-zzqfiltest-noise-%02d", i)
		nameID := fmt.Sprintf("n-zzqfiltest-noise-%02d", i)
		name := domain.Name{ID: nameID, Canonical: fmt.Sprintf("Zzqfiltest noise%02d", i), Rank: domain.RankSpecies}
		concept := domain.Concept{ID: id, BackboneID: "wcvp", AcceptedName: name, Rank: domain.RankSpecies, Status: domain.StatusAccepted}
		if err := tx.UpsertName(name); err != nil {
			t.Fatalf("UpsertName(noise %d): %v", i, err)
		}
		if err := tx.UpsertConcept(concept); err != nil {
			t.Fatalf("UpsertConcept(noise %d): %v", i, err)
		}
		if err := tx.LinkName(concept.ID, name.ID, "accepted", nil); err != nil {
			t.Fatalf("LinkName(noise %d): %v", i, err)
		}
	}

	targetConceptID = "c-zzqfiltest-target"
	targetNameID := "n-zzqfiltest-target"
	filler := make([]string, 200)
	for i := range filler {
		filler[i] = fmt.Sprintf("padding%03d", i)
	}
	targetCanonical := "Zzqfiltest " + strings.Join(filler, " ")
	targetName := domain.Name{ID: targetNameID, Canonical: targetCanonical, Rank: domain.RankSpecies}
	targetConcept := domain.Concept{ID: targetConceptID, BackboneID: "wcvp", AcceptedName: targetName, Rank: domain.RankSpecies, Status: domain.StatusAccepted}
	if err := tx.UpsertName(targetName); err != nil {
		t.Fatalf("UpsertName(target): %v", err)
	}
	if err := tx.UpsertConcept(targetConcept); err != nil {
		t.Fatalf("UpsertConcept(target): %v", err)
	}
	if err := tx.LinkName(targetConcept.ID, targetName.ID, "accepted", nil); err != nil {
		t.Fatalf("LinkName(target): %v", err)
	}
	if err := tx.AddDistribution(targetConceptID, domain.Distribution{AreaScheme: "wgsrpd_l3", AreaCode: areaCode}); err != nil {
		t.Fatalf("AddDistribution(target): %v", err)
	}

	if err := tx.Finalize(); err != nil {
		t.Fatalf("Finalize: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	return targetConceptID
}

// TestSuggest_InAreaCandidateSurvivesFetchBudgetOverflow is the Fix-A
// regression test (see the task report): it seeds 25 "noise" concepts plus
// one in-area "target" concept, all matching the "zzqfiltest" prefix — 26
// matches total, comfortably more than fetchBudget(1) == 20 (see
// TestFetchBudget_ExactValues: 1*4=4 < floor 20, so budget is exactly 20).
// The target's canonical is padded so its bm25 score is the worst of the
// 26 (see seedFetchBudgetOverflowFixture's doc comment) — under a
// bm25-only "ORDER BY score ASC LIMIT 20" it would be truncated away
// before domain.RankSuggestions (an application-layer concern Suggest
// itself never runs) ever saw it, even though it is the one in_area
// candidate and spec §B.1 ranks in_area above score. Ordering by in_area
// DESC first (the fix) keeps it inside the budget window regardless of
// its score.
func TestSuggest_InAreaCandidateSurvivesFetchBudgetOverflow(t *testing.T) {
	db := openTestDB(t)
	const areaCode = "ZZQ"
	targetID := seedFetchBudgetOverflowFixture(t, db, 25, areaCode)

	got, err := db.Suggest(context.Background(), "zzqfiltest", output.SuggestOpts{Limit: 1, Area: areaCode})
	if err != nil {
		t.Fatalf("Suggest: unexpected error: %v", err)
	}

	found := false
	for _, item := range got {
		if item.ConceptID == targetID {
			found = true
			if !item.InArea {
				t.Errorf("target item %+v: InArea = false, want true", item)
			}
		}
	}
	if !found {
		t.Fatalf("Suggest(%q) = %d items, want the in_area target concept %q to survive the fetch budget (got IDs: %v)", "zzqfiltest", len(got), targetID, conceptIDsList(got))
	}
}

func conceptIDsList(items []domain.SuggestItem) []string {
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = it.ConceptID
	}
	return out
}

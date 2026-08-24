package sqlite

import (
	"context"
	"fmt"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"

	"github.com/jobrunner/hostus/internal/domain"
)

// ingestLargeFuzzyPool writes one target plus `decoys` names that all share
// the query's genus, all sit inside the length window, and all sort
// ALPHABETICALLY BEFORE the target.
//
// This is the fixture shape every previous fuzzy test lacked, and the lack is
// why the prefilter could return the target in 0.0 % of real cases while the
// suite stayed green (docs/research/fuzzy-prefilter.md): with a handful of
// rows a prefilter returns everything, so any selection rule looks correct.
// Only a pool larger than the candidate budget can tell a working prefilter
// from one whose budget is consumed by whatever happens to sort first.
func ingestLargeFuzzyPool(t *testing.T, db *DB, decoys int) domain.Name {
	t.Helper()
	ctx := context.Background()
	tx, err := db.BeginIngest(ctx, domain.BackboneVersion{ID: "wcvp", Version: "v1", IngestedAt: "2026-08-22T00:00:00Z", ManifestSHA: "x"})
	if err != nil {
		t.Fatalf("BeginIngest: unexpected error: %v", err)
	}

	// "festuca ovona" is a one-letter typo away from the query and sorts
	// after every "festuca a…" decoy below.
	target := domain.Name{ID: "n-pool-target", Canonical: "Festuca ovona", Rank: domain.RankSpecies}
	if len(domain.Canonicalize(target.Canonical)) != len("festuca ovina") {
		t.Fatalf("fixture: target %q must be the query's length so length-ordering cannot save it", target.Canonical)
	}
	concept := domain.Concept{ID: "c-pool-target", BackboneID: "wcvp", AcceptedName: target, Rank: domain.RankSpecies, Status: domain.StatusAccepted}
	if err := tx.UpsertName(target); err != nil {
		t.Fatalf("UpsertName(target): unexpected error: %v", err)
	}
	if err := tx.UpsertConcept(concept); err != nil {
		t.Fatalf("UpsertConcept(target): unexpected error: %v", err)
	}
	if err := tx.LinkName(concept.ID, target.ID, "accepted", nil); err != nil {
		t.Fatalf("LinkName(target): unexpected error: %v", err)
	}

	for i := 0; i < decoys; i++ {
		// Same genus, EXACTLY the query's length (13 runes), sorting before
		// the target. The length has to match: the prefilter orders by
		// length-difference FIRST, so decoys of a different length sort
		// behind the target and the budget never truncates it away — the
		// test would then pass without the fix, for the wrong reason.
		canon := fmt.Sprintf("Festuca a%04d", i)
		name := domain.Name{ID: fmt.Sprintf("n-pool-decoy-%05d", i), Canonical: canon, Rank: domain.RankSpecies}
		c := domain.Concept{ID: fmt.Sprintf("c-pool-decoy-%05d", i), BackboneID: "wcvp", AcceptedName: name, Rank: domain.RankSpecies, Status: domain.StatusAccepted}
		if err := tx.UpsertName(name); err != nil {
			t.Fatalf("UpsertName(decoy %d): unexpected error: %v", i, err)
		}
		if err := tx.UpsertConcept(c); err != nil {
			t.Fatalf("UpsertConcept(decoy %d): unexpected error: %v", i, err)
		}
		if err := tx.LinkName(c.ID, name.ID, "accepted", nil); err != nil {
			t.Fatalf("LinkName(decoy %d): unexpected error: %v", i, err)
		}
	}

	if err := tx.Finalize(); err != nil {
		t.Fatalf("Finalize: unexpected error: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: unexpected error: %v", err)
	}
	return target
}

// TestMatchFuzzyCandidates_ReturnsTheTargetOutOfALargeAlphabeticallyEarlierPool
// is THE regression for the measured 0.0 % recall. The prefilter ordered its
// candidates by length-difference and then ALPHABETICALLY before applying a
// budget of 20 — so on a pool of any real size the budget went to whatever
// sorted first and the target was never scored. Measured on a full index:
// zero hits in every error class, including a single transposed letter.
func TestMatchFuzzyCandidates_ReturnsTheTargetOutOfALargeAlphabeticallyEarlierPool(t *testing.T) {
	db := openTestDB(t)
	target := ingestLargeFuzzyPool(t, db, 500)

	// limit 0 = the adapter's own default budget, i.e. what the service uses
	// when the caller states no preference.
	got, err := db.MatchFuzzyCandidates(context.Background(), "festuca ovina", 0, "", "")
	if err != nil {
		t.Fatalf("MatchFuzzyCandidates: unexpected error: %v", err)
	}
	for _, c := range got {
		if c.MatchedName.ID == target.ID {
			return
		}
	}
	t.Errorf("MatchFuzzyCandidates(%q) returned %d candidates, none of them the target %q: the budget went to alphabetically earlier names again",
		"festuca ovina", len(got), target.Canonical)
}

// TestMatchFuzzyCandidates_ExcludesNamesSharingOnlyTheFirstLetter pins the
// other half of the fix. Dropping the budget alone would leave a pool of
// ~41.000 rows per query on a real index (measured), because a one-rune GLOB
// prefix admits every name starting with that letter. Narrowing the prefix is
// what makes the un-budgeted scan affordable: 597 rows and 14 ms instead of
// 41.545 rows and 743 ms.
func TestMatchFuzzyCandidates_ExcludesNamesSharingOnlyTheFirstLetter(t *testing.T) {
	db := openTestDB(t)
	ingestLargeFuzzyPool(t, db, 1)

	ctx := context.Background()
	tx, err := db.BeginIngest(ctx, domain.BackboneVersion{ID: "wcvp2", Version: "v1", IngestedAt: "2026-08-22T00:00:00Z", ManifestSHA: "x"})
	if err != nil {
		t.Fatalf("BeginIngest: unexpected error: %v", err)
	}
	// Shares only "f" with the query, and is 15 runes against the query's 13
	// — inside the +-3 length window, so ONLY the prefix can exclude it.
	far := domain.Name{ID: "n-far-genus", Canonical: "Fagus sylvatica", Rank: domain.RankSpecies}
	c := domain.Concept{ID: "c-far-genus", BackboneID: "wcvp2", AcceptedName: far, Rank: domain.RankSpecies, Status: domain.StatusAccepted}
	if err := tx.UpsertName(far); err != nil {
		t.Fatalf("UpsertName: unexpected error: %v", err)
	}
	if err := tx.UpsertConcept(c); err != nil {
		t.Fatalf("UpsertConcept: unexpected error: %v", err)
	}
	if err := tx.LinkName(c.ID, far.ID, "accepted", nil); err != nil {
		t.Fatalf("LinkName: unexpected error: %v", err)
	}
	if err := tx.Finalize(); err != nil {
		t.Fatalf("Finalize: unexpected error: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: unexpected error: %v", err)
	}

	got, err := db.MatchFuzzyCandidates(ctx, "festuca ovina", 0, "", "")
	if err != nil {
		t.Fatalf("MatchFuzzyCandidates: unexpected error: %v", err)
	}
	for _, cand := range got {
		if cand.MatchedName.ID == far.ID {
			t.Errorf("MatchFuzzyCandidates(%q) returned %q: a name sharing only the first letter must not enter the candidate pool",
				"festuca ovina", far.Canonical)
		}
	}
}

// counterValue reads a counter's current value straight off the collector.
//
// Deliberately not via prometheus/client_golang's testutil: that package pulls
// in a further module (kylelemons/godebug) that nothing else here needs, and
// this repo's dependency list is a closed set (CLAUDE.md). client_model is
// already a dependency, and one Write call is the whole of what testutil
// would have done.
func counterValue(t *testing.T, c prometheus.Counter) float64 {
	t.Helper()
	var m dto.Metric
	if err := c.Write(&m); err != nil {
		t.Fatalf("reading counter: %v", err)
	}
	return m.GetCounter().GetValue()
}

// TestMatchFuzzyCandidates_CountsWhenTheCandidateCapBites is the observability
// half of the fix. The cap is set high enough never to decide anything on real
// data — but if a query ever does hit it, recall is silently degraded exactly
// the way it was before, and nothing would say so. That invisibility, not the
// truncation itself, is what let 0.0 % recall survive in production. So a cap
// hit is counted.
//
// Driven through the caller-supplied budget rather than the 20.000-row
// default: the behavior under test is "the pool was truncated", and forcing
// 20.001 rows through an ingest to reach it would test SQLite's insert speed,
// not this.
func TestMatchFuzzyCandidates_CountsWhenTheCandidateCapBites(t *testing.T) {
	db := openTestDB(t)
	ingestLargeFuzzyPool(t, db, 5)

	before := counterValue(t, fuzzyCandidateCapHits)
	if _, err := db.MatchFuzzyCandidates(context.Background(), "festuca ovina", 2, "", ""); err != nil {
		t.Fatalf("MatchFuzzyCandidates: unexpected error: %v", err)
	}
	if got := counterValue(t, fuzzyCandidateCapHits) - before; got != 1 {
		t.Errorf("cap-hit counter advanced by %v, want 1: a truncated candidate pool must be visible", got)
	}
}

// TestMatchFuzzyCandidates_DoesNotCountWhenThePoolFitsTheCap: a counter that
// also fires when nothing was lost would report a permanent problem and be
// ignored within a week.
func TestMatchFuzzyCandidates_DoesNotCountWhenThePoolFitsTheCap(t *testing.T) {
	db := openTestDB(t)
	ingestLargeFuzzyPool(t, db, 5)

	before := counterValue(t, fuzzyCandidateCapHits)
	if _, err := db.MatchFuzzyCandidates(context.Background(), "festuca ovina", 0, "", ""); err != nil {
		t.Fatalf("MatchFuzzyCandidates: unexpected error: %v", err)
	}
	if got := counterValue(t, fuzzyCandidateCapHits) - before; got != 0 {
		t.Errorf("cap-hit counter advanced by %v, want 0: the pool of 6 rows fits the cap", got)
	}
}

// TestMatchFuzzyCandidates_FindsATargetWhoseGenusIsMisspelled covers the gap
// the prefix narrowing leaves behind, and it is a measured gap, not a
// theoretical one: with a 4-rune prefix a typo in the genus itself drops
// recall to 1,2 %, because the query no longer shares its prefix with the
// name it means (docs/research/fuzzy-prefilter.md).
//
// The way out is a second route on the EPITHET. fts_name is tokenized with
// unicode61, so every WORD of a canonical name is an indexed token — the
// epithet is reachable without a suffix scan, and it is the half of a
// binomial a genus typo leaves intact.
func TestMatchFuzzyCandidates_FindsATargetWhoseGenusIsMisspelled(t *testing.T) {
	db := openTestDB(t)
	target := ingestLargeFuzzyPool(t, db, 0)

	// "fsetuca" transposes the query's 2nd and 3rd rune, so the 4-rune
	// prefix ("fset") cannot reach "festuca ovona" — only the shared
	// epithet can.
	const q = "fsetuca ovona"
	if globPrefix(q, fuzzyCandidatePrefixRunes) == globPrefix("festuca ovona", fuzzyCandidatePrefixRunes) {
		t.Fatalf("test setup: %q must NOT share its prefix with the target, or the prefix route would find it", q)
	}

	got, err := db.MatchFuzzyCandidates(context.Background(), q, 0, "", "")
	if err != nil {
		t.Fatalf("MatchFuzzyCandidates: unexpected error: %v", err)
	}
	for _, c := range got {
		if c.MatchedName.ID == target.ID {
			return
		}
	}
	t.Errorf("MatchFuzzyCandidates(%q) returned %d candidates, none of them %q: a typo in the genus makes the target unreachable",
		q, len(got), target.Canonical)
}

// TestFuzzyEpithetToken_SkipsALeadingHybridMarker: for a nothotaxon the second
// whitespace-separated field is the GENUS, not the epithet ("× ammocalamagrostis
// baltica"). Keying the epithet route on it means the route cannot find the
// genus typo it exists for, and instead MATCHes every same-genus name inside
// the length window — a bigger pool doing the wrong job. domain's genus token
// already skips the marker; the two have to agree on where a name's words are.
func TestFuzzyEpithetToken_SkipsALeadingHybridMarker(t *testing.T) {
	for _, c := range []struct {
		in, want string
	}{
		{"× ammocalamagrostis baltica", "baltica"},
		{"x ammocalamagrostis baltica", "baltica"},
		{"festuca ovina", "ovina"},
		{"festuca", ""},
		{"× festuca", ""},
	} {
		if got := fuzzyEpithetToken(c.in); got != c.want {
			t.Errorf("fuzzyEpithetToken(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestMatchFuzzyCandidates_CapBoundsTheWholeLookupNotEachRoute: the cap is a
// load guard, and a guard that each of two routes applies separately bounds
// nothing at 2x. The enrichment join that follows is per (name, concept) pair,
// so the row count it produces is larger still.
func TestMatchFuzzyCandidates_CapBoundsTheWholeLookupNotEachRoute(t *testing.T) {
	db := openTestDB(t)
	ingestLargeFuzzyPool(t, db, 40)

	const limit = 5
	got, err := db.MatchFuzzyCandidates(context.Background(), "festuca ovina", limit, "", "")
	if err != nil {
		t.Fatalf("MatchFuzzyCandidates: unexpected error: %v", err)
	}
	names := make(map[string]bool, len(got))
	for _, c := range got {
		names[c.MatchedName.ID] = true
	}
	if len(names) > limit {
		t.Errorf("MatchFuzzyCandidates(limit=%d) returned %d distinct names: the cap must bound the lookup, not each route separately",
			limit, len(names))
	}
}

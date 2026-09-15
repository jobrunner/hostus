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

// TestFtsPrefixToken_StripsAggregateMarker pins that the FTS query is
// marker-insensitive: an aggregate spelling produces the same prefix token as
// the bare base, so "Achillea millefolium agg./aggr./s.l." all search the base.
func TestFtsPrefixToken_StripsAggregateMarker(t *testing.T) {
	base := ftsPrefixToken("Achillea millefolium")
	for _, q := range []string{
		"Achillea millefolium agg.",
		"Achillea millefolium aggr.",
		"Achillea millefolium s. l.",
	} {
		if got := ftsPrefixToken(q); got != base {
			t.Errorf("ftsPrefixToken(%q) = %q, want %q (same as the bare base)", q, got, base)
		}
	}
}

// seedSelfReferentialAggregateAlias attaches ONE aggregate name-space alias
// onto conceptID that carries the SAME genus+epithet as the concept's own
// accepted name, only with a marker appended (e.g. "Corynephorus canescens
// agg.") — the only shape a RESOLVED aggregate alias actually takes in
// production, per internal/application/namespace_ingest.go's aggregate-to-
// nominate rule (a resolved aggregate spelling always resolves to its own
// nominate species, never to an unrelated one).
//
// This is deliberately a SEPARATE helper from namespace_test.go's
// seedFloraVegEntries, which attaches a cross-species alias ("Festuca
// ovina...") onto the Corynephorus concept for a narrower, unrelated
// purpose (pinning fts_name/fts_name_map row counts) and must not be
// repurposed to stand in for realistic aggregate-suggest semantics — doing
// so once already hid a real bug (see this file's Suggest doc comment on
// nameStartFilter, "second EXISTS arm" note).
func seedSelfReferentialAggregateAlias(t *testing.T, db *DB, conceptID, aliasName string) {
	t.Helper()
	ctx := context.Background()
	tx, err := db.BeginIngest(ctx, seedBackboneVersion)
	if err != nil {
		t.Fatalf("BeginIngest: unexpected error: %v", err)
	}
	if err := tx.UpsertNameSpace(domain.NameSpaceMeta{
		ID: "floraveg", Version: "2023-01-03",
		License: "", SourceURL: "https://example.org/floraveg",
		ManifestSHA: "deadbeef", Redistribution: domain.RedistributionUnknown,
	}); err != nil {
		t.Fatalf("UpsertNameSpace: unexpected error: %v", err)
	}
	if err := tx.AddNameSpaceEntry(conceptID, domain.NameSpaceEntry{
		Space: "floraveg", ExtID: "self-ref-1", Name: aliasName,
		Aggregate: true, Resolution: string(domain.RuleAggregateToNominate),
	}); err != nil {
		t.Fatalf("AddNameSpaceEntry: unexpected error: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: unexpected error: %v", err)
	}
}

// TestSuggest_AggregateSpellingFindsConceptAndFlagsIt pins the two halves of
// marker-insensitive aggregate suggest end to end: an aggregate spelling
// (marker-stripped in the query) reaches the concept via its indexed aggregate
// alias and the result carries Aggregate=true; a query matching only the
// concept's own (non-aggregate) name carries Aggregate=false. The aggregate
// alias is self-referential ("Corynephorus canescens agg.", same genus+
// epithet as the concept's own accepted name), matching real production
// semantics (see seedSelfReferentialAggregateAlias's doc comment).
func TestSuggest_AggregateSpellingFindsConceptAndFlagsIt(t *testing.T) {
	db := openTestDB(t)
	conceptID := seedCorynephorusConcept(t, db) // "Corynephorus canescens" WITH its own name in FTS (is_aggregate=0)
	seedSelfReferentialAggregateAlias(t, db, conceptID, "Corynephorus canescens agg.")
	ctx := context.Background()

	find := func(q string) (domain.SuggestItem, bool) {
		items, err := db.Suggest(ctx, q, output.SuggestOpts{Limit: 10})
		if err != nil {
			t.Fatalf("Suggest(%q): %v", q, err)
		}
		for _, it := range items {
			if it.ConceptID == conceptID {
				return it, true
			}
		}
		return domain.SuggestItem{}, false
	}

	agg, ok := find("Corynephorus canescens agg.")
	if !ok {
		t.Fatal(`Suggest("Corynephorus canescens agg.") did not reach the concept via its aggregate alias`)
	}
	if !agg.Aggregate {
		t.Error("Aggregate = false, want true (matched via an aggregate alias)")
	}

	// A self-referential aggregate alias repeats its concept's own
	// genus+epithet verbatim (only with a marker appended), so any query
	// that is itself a prefix of that shared genus+epithet (e.g.
	// "Corynephorus can") ALSO matches the alias's fts_name row — carrying
	// Aggregate=true is then correct, not a bug (see
	// SuggestItem.Aggregate's doc comment: "a concept that owns any
	// aggregate alias carries it even when the query matched only the
	// plain name" is true only when the plain-name query does not ALSO
	// overlap the alias text; here it does). To exercise a query that
	// matches ONLY a non-aggregate name, this uses the concept's SYNONYM
	// ("Weingaertneria canescens" — a different genus the aggregate alias
	// never touches), not its accepted name.
	own, ok := find("Weingaertneria can")
	if !ok {
		t.Fatal(`Suggest("Weingaertneria can") did not reach the concept via its synonym name`)
	}
	if own.Aggregate {
		t.Error("Aggregate = true, want false (matched only the concept's own non-aggregate synonym name)")
	}
}

// TestSuggest_NameStart_AggregateAliasDoesNotExemptBareEpithetQuery is the
// Fix-Round-1 regression test the code review demanded: a concept carrying a
// self-referential aggregate alias ("Corynephorus canescens agg.") must
// still be EXCLUDED from a name_start suggest for a bare epithet query
// ("canescens") — the epithet token only ever matches mid-document (in
// either the concept's own name OR the aggregate alias, which repeats the
// same genus+epithet), never a name/alias that itself STARTS with
// "canescens". An earlier version of nameStartFilter added an EXISTS arm
// that exempted a concept from name_start whenever it had ANY is_aggregate=1
// FTS match, regardless of whether that match's own text started with the
// query prefix — reopening the SP7 bug for every concept with an aggregate
// alias. This pins that regression closed for good.
func TestSuggest_NameStart_AggregateAliasDoesNotExemptBareEpithetQuery(t *testing.T) {
	db := openTestDB(t)
	conceptID := seedCorynephorusConcept(t, db)
	seedSelfReferentialAggregateAlias(t, db, conceptID, "Corynephorus canescens agg.")
	ctx := context.Background()

	got, err := db.Suggest(ctx, "canescens", output.SuggestOpts{Limit: 10, MatchMode: "name_start"})
	if err != nil {
		t.Fatalf("Suggest: unexpected error: %v", err)
	}
	for _, it := range got {
		if it.ConceptID == conceptID {
			t.Errorf("Suggest(%q, name_start) = %+v, want the concept excluded (bare epithet is not a name_start prefix of either the concept's name or its aggregate alias)", "canescens", it)
		}
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
	if err := db.BuildDistributionClosure(ctx); err != nil {
		t.Fatalf("BuildDistributionClosure: %v", err)
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

// seedInulaHomonyms reproduces the spec's motivating fixture (spec
// 2026-09-14) in miniature: the "Inula hirta" homonym as WCVP carries it —
// two DIFFERENT names that share one canonical and differ only in
// authorship, each a synonym of a different accepted concept — plus a third
// concept whose only name merely STARTS with the query.
//
//   - pentanema-hirtum:      accepted "Pentanema hirtum",      synonym "Inula hirta L."
//   - pentanema-britannica:  accepted "Pentanema britannica",  synonym "Inula hirta Pollich"
//   - inula-hirta-var:       accepted "Inula hirta var. hirtella" (prefix, never equal)
//
// "Inula hirta Pollich" carries WCVP's real nom_status cell
// (illegitimateHomonymStatus) while "Inula hirta L." carries none — the
// asymmetry the 2026-09-15 spec ranks on. It is part of the SHARED fixture
// rather than a private one because it is not an extra case bolted onto the
// homonym: it is what makes the pair a homonym in the first place, and a
// fixture that left it out showed two names that look equally valid.
//
// The eurosl name space is seeded on the first two only: "Inula hirta" for
// pentanema-hirtum (the target-space hit the spec wants ranked first) and
// "Inula britannica" for pentanema-britannica (an entry that exists but does
// NOT match the query). inula-hirta-var deliberately has no entry at all, so
// the same fixture also exercises RequireTargetSpace.
func seedInulaHomonyms(t *testing.T, db *DB) {
	t.Helper()
	ctx := context.Background()

	bv := domain.BackboneVersion{ID: "wcvp", Version: "v1", IngestedAt: "2026-09-14T00:00:00Z", ManifestSHA: "x"}
	ingestVia(t, db, bv, func(tx output.IngestTx) {
		hirtum := species("n-pentanema-hirtum", "Pentanema hirtum")
		hirtaL := domain.Name{ID: "n-inula-hirta-l", Canonical: "Inula hirta", Authorship: "L.", Rank: domain.RankSpecies}
		britannica := species("n-pentanema-britannica", "Pentanema britannica")
		hirtaPollich := domain.Name{ID: "n-inula-hirta-pollich", Canonical: "Inula hirta", Authorship: "Pollich", Rank: domain.RankSpecies, NomStatus: illegitimateHomonymStatus}
		variety := domain.Name{ID: "n-inula-hirta-var-hirtella", Canonical: "Inula hirta var. hirtella", Rank: domain.RankVariety}

		for _, n := range []domain.Name{hirtum, hirtaL, britannica, hirtaPollich, variety} {
			mustTx(t, tx.UpsertName(n))
		}

		for _, c := range []struct {
			id       string
			accepted domain.Name
			synonym  *domain.Name
		}{
			{"wcvp:concept:pentanema-hirtum", hirtum, &hirtaL},
			{"wcvp:concept:pentanema-britannica", britannica, &hirtaPollich},
			{"wcvp:concept:inula-hirta-var", variety, nil},
		} {
			// The concept's rank mirrors its accepted name's, as it does in
			// every ingest: hard-coding RankSpecies here would label the
			// variety a species and quietly mislead a future Ranks test.
			concept := domain.Concept{ID: c.id, BackboneID: "wcvp", AcceptedName: c.accepted, Rank: c.accepted.Rank, Status: domain.StatusAccepted}
			mustTx(t, tx.UpsertConcept(concept))
			mustTx(t, tx.LinkName(concept.ID, c.accepted.ID, "accepted", nil))
			if c.synonym != nil {
				mustTx(t, tx.LinkName(concept.ID, c.synonym.ID, "synonym", nil))
			}
		}
	})

	tx, err := db.BeginIngest(ctx, domain.BackboneVersion{ID: "eurosl-src", Version: "v1", IngestedAt: "2026-09-14T00:00:00Z", ManifestSHA: "y"})
	mustTx(t, err)
	mustTx(t, tx.UpsertNameSpace(domain.NameSpaceMeta{ID: "eurosl", Version: "v1", ManifestSHA: "y", Redistribution: domain.RedistributionUnknown}))
	mustTx(t, tx.AddNameSpaceEntry("wcvp:concept:pentanema-hirtum", domain.NameSpaceEntry{
		Space: "eurosl", ExtID: "e-1", Name: "Inula hirta", Status: "accepted",
	}))
	mustTx(t, tx.AddNameSpaceEntry("wcvp:concept:pentanema-britannica", domain.NameSpaceEntry{
		Space: "eurosl", ExtID: "e-2", Name: "Inula britannica", Status: "accepted",
	}))
	mustTx(t, tx.Commit())
}

// inulaHirtaQuery is the one query every seedInulaHomonyms-based test asks:
// the spec's motivating input, typed in full.
const inulaHirtaQuery = "Inula hirta"

// suggestByID runs Suggest for inulaHirtaQuery and indexes the result by
// concept id, so an assertion can state both "this concept is present with
// these signals" and "this concept is absent" without re-walking the slice
// each time.
func suggestByID(t *testing.T, db *DB, opts output.SuggestOpts) map[string]domain.SuggestItem {
	t.Helper()
	items, err := db.Suggest(context.Background(), inulaHirtaQuery, opts)
	if err != nil {
		t.Fatalf("Suggest(%q, %+v): unexpected error: %v", inulaHirtaQuery, opts, err)
	}
	byID := make(map[string]domain.SuggestItem, len(items))
	for _, it := range items {
		byID[it.ConceptID] = it
	}
	return byID
}

// TestSuggest_RequireTargetSpaceDropsConceptsWithoutEntry pins the filter
// half of the spec's first decision: with RequireTargetSpace AND a
// TargetSpace, a concept with no entry in that space disappears while one
// with an entry stays. The two control cases (either half alone) pin that
// the filter is inert without BOTH — a caller who set only one of them must
// see the un-filtered page, not a silently emptied one.
func TestSuggest_RequireTargetSpaceDropsConceptsWithoutEntry(t *testing.T) {
	db := openTestDB(t)
	seedInulaHomonyms(t, db)

	const withEntry = "wcvp:concept:pentanema-hirtum"
	const withoutEntry = "wcvp:concept:inula-hirta-var"

	filtered := suggestByID(t, db, output.SuggestOpts{Limit: 10, TargetSpace: "eurosl", RequireTargetSpace: true})
	if _, ok := filtered[withEntry]; !ok {
		t.Errorf("concept %q missing from the require_target_space result, want it kept (it has a eurosl entry)", withEntry)
	}
	if it, ok := filtered[withoutEntry]; ok {
		t.Errorf("concept %q = %+v survived require_target_space, want it dropped (no eurosl entry)", withoutEntry, it)
	}

	spaceOnly := suggestByID(t, db, output.SuggestOpts{Limit: 10, TargetSpace: "eurosl"})
	if _, ok := spaceOnly[withoutEntry]; !ok {
		t.Errorf("concept %q dropped for target_space alone, want it kept (enrichment only, no filter)", withoutEntry)
	}

	requireOnly := suggestByID(t, db, output.SuggestOpts{Limit: 10, RequireTargetSpace: true})
	if _, ok := requireOnly[withoutEntry]; !ok {
		t.Errorf("concept %q dropped for require_target_space WITHOUT a target_space, want it kept (there is no space to require an entry in)", withoutEntry)
	}
}

// TestSuggest_ExactHitIsSetOnlyForFullNameEquality pins the spec's first new
// ranking signal: ExactHit means a name of the concept EQUALS the
// canonicalized query, not merely starts with it.
//
// The "merely a prefix" concept carries "Inula hirta var. hirtella" rather
// than the spec's illustrative "Inula hirtella": the latter matches neither
// the FTS5 prefix token ("hirta"* does not reach "hirtella") nor the
// name_start filter, so it would never be a candidate for this query at all
// and the assertion below would be vacuously true no matter what the code
// did.
func TestSuggest_ExactHitIsSetOnlyForFullNameEquality(t *testing.T) {
	db := openTestDB(t)
	seedInulaHomonyms(t, db)

	got := suggestByID(t, db, output.SuggestOpts{Limit: 10})

	exact, ok := got["wcvp:concept:pentanema-hirtum"]
	if !ok {
		t.Fatalf("Suggest did not return the concept carrying the exact name (got %v)", conceptIDsList(itemsOf(got)))
	}
	if !exact.ExactHit {
		t.Errorf("concept %q: ExactHit = false, want true (carries the name %q verbatim)", exact.ConceptID, "Inula hirta")
	}

	longer, ok := got["wcvp:concept:inula-hirta-var"]
	if !ok {
		t.Fatalf("Suggest did not return the prefix-only concept (got %v)", conceptIDsList(itemsOf(got)))
	}
	if longer.ExactHit {
		t.Errorf("concept %q: ExactHit = true, want false (its only name %q merely STARTS with the query)", longer.ConceptID, "Inula hirta var. hirtella")
	}
	if !longer.PrefixHit {
		t.Errorf("concept %q: PrefixHit = false, want true (its name starts with the query)", longer.ConceptID)
	}
}

// TestSuggest_MatchedNameCarriesAuthorshipOfTheHit pins the spec's fifth
// decision: the result names the name that TRIGGERED the hit, with its
// authorship and role — the only thing that tells "Inula hirta L." apart
// from "Inula hirta Pollich" — and not the concept's own accepted name,
// which is identical in neither case.
func TestSuggest_MatchedNameCarriesAuthorshipOfTheHit(t *testing.T) {
	db := openTestDB(t)
	seedInulaHomonyms(t, db)

	got := suggestByID(t, db, output.SuggestOpts{Limit: 10})

	cases := []struct {
		conceptID string
		want      domain.MatchedName
	}{
		{"wcvp:concept:pentanema-hirtum", domain.MatchedName{Canonical: "Inula hirta", Authorship: "L.", Role: "synonym", NomStatusJudgement: domain.JudgementAbsent}},
		{"wcvp:concept:pentanema-britannica", domain.MatchedName{Canonical: "Inula hirta", Authorship: "Pollich", Role: "synonym", NomStatus: illegitimateHomonymNormalized, NomStatusJudgement: domain.JudgementDisqualifying}},
		{"wcvp:concept:inula-hirta-var", domain.MatchedName{Canonical: "Inula hirta var. hirtella", Authorship: "", Role: "accepted", NomStatusJudgement: domain.JudgementAbsent}},
	}
	for _, tc := range cases {
		it, ok := got[tc.conceptID]
		if !ok {
			t.Errorf("Suggest did not return concept %q", tc.conceptID)
			continue
		}
		if it.MatchedName != tc.want {
			t.Errorf("concept %q: MatchedName = %+v, want %+v", tc.conceptID, it.MatchedName, tc.want)
		}
	}
}

// TestSuggest_TargetSpaceHitOnlyForTheSpaceNameMatchingTheQuery pins the
// spec's second new ranking signal: it is the concept's name IN THE
// REQUESTED SPACE that has to match the query, not the name that produced
// the hit. Both concepts below matched via a WCVP synonym spelled "Inula
// hirta", but only pentanema-hirtum is called that in eurosl — britannica is
// "Inula britannica" there. Without a requested space the signal is false
// for every item (there is nothing for it to prefer).
func TestSuggest_TargetSpaceHitOnlyForTheSpaceNameMatchingTheQuery(t *testing.T) {
	db := openTestDB(t)
	seedInulaHomonyms(t, db)

	withSpace := suggestByID(t, db, output.SuggestOpts{Limit: 10, TargetSpace: "eurosl"})
	if it := withSpace["wcvp:concept:pentanema-hirtum"]; !it.TargetSpaceHit {
		t.Errorf("concept %q: TargetSpaceHit = false, want true (its eurosl name IS the query)", it.ConceptID)
	}
	if it := withSpace["wcvp:concept:pentanema-britannica"]; it.TargetSpaceHit {
		t.Errorf("concept %q: TargetSpaceHit = true, want false (its eurosl name is %q)", it.ConceptID, it.TargetSpaceName)
	}

	noSpace := suggestByID(t, db, output.SuggestOpts{Limit: 10})
	for id, it := range noSpace {
		if it.TargetSpaceHit {
			t.Errorf("concept %q: TargetSpaceHit = true without a requested target space, want false", id)
		}
	}
}

// TestSuggest_MatchedNameIsDeterministicBetweenSameCanonicalAuthors pins the
// tie-break the first three ORDER BY keys of matchedNameQuery cannot make:
// TWO names of ONE concept, same role, same canonical, differing only in
// authorship — which is what a homonym IS, and what 5,784 (concept_id, role,
// canonical_fold) groups of the production index look like. Nothing but
// authorship (then name id) separates them, so without those keys the answer
// is rowid order and flips with any plan or build change, while the console
// column "Treffer-Name" claims to name THE author of the hit.
//
// The name ids are chosen so the WRONG answer comes first without the fix:
// the query reaches concept_name through its (concept_id, name_id) covering
// index, so the un-fixed ORDER BY (stable up to name id) hands back
// "…-1" = "Sm.", while authorship ASC must pick "A.Gray". Verified: with the
// last two ORDER BY keys removed, this test fails. Naming them by author
// instead would have made "a…gray" sort first by id too and the test would
// have passed by luck on the broken code.
func TestSuggest_MatchedNameIsDeterministicBetweenSameCanonicalAuthors(t *testing.T) {
	db := openTestDB(t)
	const conceptID = "wcvp:concept:pentanema-duplicatum"

	bv := domain.BackboneVersion{ID: "wcvp", Version: "v1", IngestedAt: "2026-09-14T00:00:00Z", ManifestSHA: "x"}
	ingestVia(t, db, bv, func(tx output.IngestTx) {
		accepted := species("n-pentanema-duplicatum", "Pentanema duplicatum")
		later := domain.Name{ID: "n-inula-duplicata-1", Canonical: "Inula duplicata", Authorship: "Sm.", Rank: domain.RankSpecies}
		earlier := domain.Name{ID: "n-inula-duplicata-2", Canonical: "Inula duplicata", Authorship: "A.Gray", Rank: domain.RankSpecies}
		for _, n := range []domain.Name{accepted, later, earlier} {
			mustTx(t, tx.UpsertName(n))
		}
		c := domain.Concept{ID: conceptID, BackboneID: "wcvp", AcceptedName: accepted, Rank: domain.RankSpecies, Status: domain.StatusAccepted}
		mustTx(t, tx.UpsertConcept(c))
		mustTx(t, tx.LinkName(c.ID, accepted.ID, "accepted", nil))
		mustTx(t, tx.LinkName(c.ID, later.ID, "synonym", nil))
		mustTx(t, tx.LinkName(c.ID, earlier.ID, "synonym", nil))
	})

	want := domain.MatchedName{Canonical: "Inula duplicata", Authorship: "A.Gray", Role: "synonym", NomStatusJudgement: domain.JudgementAbsent}
	for run := 1; run <= 3; run++ {
		items, err := db.Suggest(context.Background(), "Inula duplicata", output.SuggestOpts{Limit: 10})
		if err != nil {
			t.Fatalf("Suggest (run %d): unexpected error: %v", run, err)
		}
		found := false
		for _, it := range items {
			if it.ConceptID != conceptID {
				continue
			}
			found = true
			if it.MatchedName != want {
				t.Errorf("run %d: MatchedName = %+v, want %+v (authorship ASC decides between two same-canonical synonyms)", run, it.MatchedName, want)
			}
		}
		if !found {
			t.Fatalf("run %d: Suggest returned no item for %q", run, conceptID)
		}
	}
}

// TestSuggest_AggregateQueryFallsBackToNominateSpaceName pins the fix for the
// self-contradiction the whole-branch review found: with
// require_target_space the endpoint keeps ONLY concepts that have an entry in
// the space (the filter tests EXISTS, i.e. any entry) — and then answered
// every one of those rows with an EMPTY target_space_name, because
// domain.ResolveTargetSpace hands back "" for an aggregate query when the
// space carries no is_aggregate entry. The console badges that as "kein Name
// … lässt sich dort nicht benennen" for a concept the filter kept precisely
// because it HAS a name there. Measured on the production index:
// q="Alyssum montanum agg."&target_space=eurosl&require_target_space=true
// returned 5 rows, all nameless, although eurosl calls
// wcvp:concept:2632304 "Alyssum montanum" (status=accepted).
//
// The second half is the ranking: TargetSpaceHit is derived from that name,
// so it was false for the WHOLE page — the spec's decisive criterion was dead
// for every aggregate query, which is the common case, not the rare one
// (eurosl 251 aggregate entries against ~125k entries overall).
//
// This fixture deliberately has NO Aggregate entry, the majority case that
// TestSuggest_TargetSpaceHitSurvivesDifferentAggregateMarkerSpelling (which
// seeds one) leaves open.
func TestSuggest_AggregateQueryFallsBackToNominateSpaceName(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	const conceptID = "wcvp:concept:alyssum-montanum"

	bv := domain.BackboneVersion{ID: "wcvp", Version: "v1", IngestedAt: "2026-09-15T00:00:00Z", ManifestSHA: "x"}
	ingestVia(t, db, bv, func(tx output.IngestTx) {
		accepted := species("n-alyssum-montanum", "Alyssum montanum")
		mustTx(t, tx.UpsertName(accepted))
		c := domain.Concept{ID: conceptID, BackboneID: "wcvp", AcceptedName: accepted, Rank: domain.RankSpecies, Status: domain.StatusAccepted}
		mustTx(t, tx.UpsertConcept(c))
		mustTx(t, tx.LinkName(c.ID, accepted.ID, "accepted", nil))
	})

	tx, err := db.BeginIngest(ctx, domain.BackboneVersion{ID: "eurosl-src", Version: "v1", IngestedAt: "2026-09-15T00:00:00Z", ManifestSHA: "y"})
	mustTx(t, err)
	mustTx(t, tx.UpsertNameSpace(domain.NameSpaceMeta{ID: "eurosl", Version: "v1", ManifestSHA: "y", Redistribution: domain.RedistributionUnknown}))
	// The space knows the taxon, but only under its NOMINATE spelling — no
	// aggregate entry anywhere, exactly as eurosl holds most taxa.
	mustTx(t, tx.AddNameSpaceEntry(conceptID, domain.NameSpaceEntry{
		Space: "eurosl", ExtID: "e-1", Name: "Alyssum montanum", Status: "accepted",
	}))
	mustTx(t, tx.Commit())

	opts := output.SuggestOpts{Limit: 10, TargetSpace: "eurosl", RequireTargetSpace: true}
	items, err := db.Suggest(ctx, "Alyssum montanum agg.", opts)
	if err != nil {
		t.Fatalf("Suggest: unexpected error: %v", err)
	}

	found := false
	for _, it := range items {
		if it.ConceptID != conceptID {
			continue
		}
		found = true
		if it.TargetSpaceName != "Alyssum montanum" {
			t.Errorf("TargetSpaceName = %q, want %q — require_target_space kept this concept BECAUSE it has an entry, so the answer must name it", it.TargetSpaceName, "Alyssum montanum")
		}
		if !it.TargetSpaceHit {
			t.Error("TargetSpaceHit = false, want true (the space's name for this concept IS the queried name)")
		}
	}
	if !found {
		t.Fatalf("Suggest returned no item for %q (require_target_space kept %d items)", conceptID, len(items))
	}
}

// TestSuggest_TargetSpaceHitSurvivesDifferentAggregateMarkerSpelling pins
// that TargetSpaceHit is decided by the NAME, not by which aggregate marker
// a name space happens to spell. The spaces genuinely disagree — measured on
// the production index, eurosl and floraveg write "… aggr." while germansl
// writes "… agg." — so comparing the raw canonical forms made every
// aggregate query against eurosl miss, and with it the spec's decisive
// ranking criterion, for the whole page (ResolveTargetSpace answers "" for
// concepts without an aggregate entry on an aggregate query). Both sides go
// through StripAggregateMarkers, exactly as ftsPrefixToken and
// nameStartFilter already do.
func TestSuggest_TargetSpaceHitSurvivesDifferentAggregateMarkerSpelling(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	const conceptID = "wcvp:concept:achillea-millefolium"

	bv := domain.BackboneVersion{ID: "wcvp", Version: "v1", IngestedAt: "2026-09-14T00:00:00Z", ManifestSHA: "x"}
	ingestVia(t, db, bv, func(tx output.IngestTx) {
		accepted := species("n-achillea-millefolium", "Achillea millefolium")
		mustTx(t, tx.UpsertName(accepted))
		c := domain.Concept{ID: conceptID, BackboneID: "wcvp", AcceptedName: accepted, Rank: domain.RankSpecies, Status: domain.StatusAccepted}
		mustTx(t, tx.UpsertConcept(c))
		mustTx(t, tx.LinkName(c.ID, accepted.ID, "accepted", nil))
	})

	tx, err := db.BeginIngest(ctx, domain.BackboneVersion{ID: "eurosl-src", Version: "v1", IngestedAt: "2026-09-14T00:00:00Z", ManifestSHA: "y"})
	mustTx(t, err)
	mustTx(t, tx.UpsertNameSpace(domain.NameSpaceMeta{ID: "eurosl", Version: "v1", ManifestSHA: "y", Redistribution: domain.RedistributionUnknown}))
	// eurosl's own spelling: "aggr.", while the caller below types "agg.".
	mustTx(t, tx.AddNameSpaceEntry(conceptID, domain.NameSpaceEntry{
		Space: "eurosl", ExtID: "e-1", Name: "Achillea millefolium aggr.", Status: "accepted", Aggregate: true,
	}))
	mustTx(t, tx.Commit())

	items, err := db.Suggest(ctx, "Achillea millefolium agg.", output.SuggestOpts{Limit: 10, TargetSpace: "eurosl"})
	if err != nil {
		t.Fatalf("Suggest: unexpected error: %v", err)
	}
	found := false
	for _, it := range items {
		if it.ConceptID != conceptID {
			continue
		}
		found = true
		if it.TargetSpaceName != "Achillea millefolium aggr." {
			t.Fatalf("TargetSpaceName = %q, want eurosl's own aggregate spelling — fixture no longer exercises the marker mismatch", it.TargetSpaceName)
		}
		if !it.TargetSpaceHit {
			t.Errorf("TargetSpaceHit = false for query %q against space name %q, want true (the marker spelling must not decide)", "Achillea millefolium agg.", it.TargetSpaceName)
		}
	}
	if !found {
		t.Fatalf("Suggest returned no item for %q", conceptID)
	}
}

// illegitimateHomonymStatus is WCVP's raw nom_status cell for "Inula hirta
// Pollich", copied verbatim from the production index (leading ", " and all —
// 99,111 of 99,252 populated cells start that way, which is exactly what
// domain.NormalizeNomStatus strips). illegitimateHomonymNormalized is what
// that function makes of it, and therefore what MatchedName.NomStatus must
// carry: the adapter stores the NORMALIZED form, never the raw cell.
const (
	illegitimateHomonymStatus     = ", nom. illeg. homonym. post."
	illegitimateHomonymNormalized = "nom. illeg. homonym. post."
)

// TestSuggest_MatchedNameCarriesNomStatusJudgement pins that the adapter does
// not merely COPY nom_status through but classifies it via
// domain.ClassifyNomStatus — the single rule table, shared with the synonym
// endpoint — and derives SuggestItem.MatchedNameDisqualified from that verdict.
// "Inula hirta Pollich" is the reported case: a later illegitimate homonym of
// Pentanema britannica that ranked as an equal of the valid "Inula hirta L."
// because nothing in the suggest path had ever looked at its status.
func TestSuggest_MatchedNameCarriesNomStatusJudgement(t *testing.T) {
	db := openTestDB(t)
	seedInulaHomonyms(t, db)

	got := suggestByID(t, db, output.SuggestOpts{Limit: 10})

	it, ok := got["wcvp:concept:pentanema-britannica"]
	if !ok {
		t.Fatalf("Suggest did not return the concept carrying the illegitimate homonym (got %v)", conceptIDsList(itemsOf(got)))
	}
	if it.MatchedName.NomStatus != illegitimateHomonymNormalized {
		t.Errorf("MatchedName.NomStatus = %q, want %q (normalized, not the raw cell %q)", it.MatchedName.NomStatus, illegitimateHomonymNormalized, illegitimateHomonymStatus)
	}
	if it.MatchedName.NomStatusJudgement != domain.JudgementDisqualifying {
		t.Errorf("MatchedName.NomStatusJudgement = %q, want %q", it.MatchedName.NomStatusJudgement, domain.JudgementDisqualifying)
	}
	if !it.MatchedNameDisqualified {
		t.Errorf("MatchedNameDisqualified = false, want true (derived from the disqualifying judgement)")
	}
}

// TestSuggest_MatchedNameWithoutNomStatusIsAbsentNotDisqualified pins the
// distinction the domain layer makes deliberately: nothing recorded is
// JudgementAbsent, NOT a clean bill of health and NOT a defect. "Inula hirta
// L." — the valid name, which WCVP leaves status-free — must therefore keep
// MatchedNameDisqualified false, and its empty NomStatus must come with an
// explicit verdict rather than the zero value of the judgement type.
func TestSuggest_MatchedNameWithoutNomStatusIsAbsentNotDisqualified(t *testing.T) {
	db := openTestDB(t)
	seedInulaHomonyms(t, db)

	got := suggestByID(t, db, output.SuggestOpts{Limit: 10})

	it, ok := got["wcvp:concept:pentanema-hirtum"]
	if !ok {
		t.Fatalf("Suggest did not return the concept carrying the valid name (got %v)", conceptIDsList(itemsOf(got)))
	}
	if it.MatchedName.NomStatus != "" {
		t.Errorf("MatchedName.NomStatus = %q, want %q (WCVP records nothing for this name)", it.MatchedName.NomStatus, "")
	}
	if it.MatchedName.NomStatusJudgement != domain.JudgementAbsent {
		t.Errorf("MatchedName.NomStatusJudgement = %q, want %q (absent is a verdict, not a missing one)", it.MatchedName.NomStatusJudgement, domain.JudgementAbsent)
	}
	if it.MatchedNameDisqualified {
		t.Errorf("MatchedNameDisqualified = true, want false (no status recorded is not a defect)")
	}
}

// TestSuggest_LegitimateNameWinsTheMatchedNameSelection pins the spec's fifth
// decision where it actually bites: ONE concept carrying TWO names that both
// satisfy the query, one of them disqualified. The console shows a single
// "Treffer-Name", so the choice is not cosmetic — naming the illegitimate one
// tells the operator the concept was reached through a name they must not use.
//
// The authorships are chosen so the WRONG answer wins without the fix: the
// remaining tie-breaks are authorship then id, and "A.Auct." sorts before
// "Z.Auct." — so a selection that ignores the judgement picks the
// illegitimate name. With the fix, the disqualified first row is REPLACED by
// the valid later one.
func TestSuggest_LegitimateNameWinsTheMatchedNameSelection(t *testing.T) {
	db := openTestDB(t)
	const conceptID = "wcvp:concept:pentanema-selectum"

	bv := domain.BackboneVersion{ID: "wcvp", Version: "v1", IngestedAt: "2026-09-15T00:00:00Z", ManifestSHA: "x"}
	ingestVia(t, db, bv, func(tx output.IngestTx) {
		accepted := species("n-pentanema-selectum", "Pentanema selectum")
		illegitimate := domain.Name{ID: "n-inula-selecta-1", Canonical: "Inula selecta", Authorship: "A.Auct.", Rank: domain.RankSpecies, NomStatus: illegitimateHomonymStatus}
		valid := domain.Name{ID: "n-inula-selecta-2", Canonical: "Inula selecta", Authorship: "Z.Auct.", Rank: domain.RankSpecies}
		for _, n := range []domain.Name{accepted, illegitimate, valid} {
			mustTx(t, tx.UpsertName(n))
		}
		c := domain.Concept{ID: conceptID, BackboneID: "wcvp", AcceptedName: accepted, Rank: domain.RankSpecies, Status: domain.StatusAccepted}
		mustTx(t, tx.UpsertConcept(c))
		mustTx(t, tx.LinkName(c.ID, accepted.ID, "accepted", nil))
		mustTx(t, tx.LinkName(c.ID, illegitimate.ID, "synonym", nil))
		mustTx(t, tx.LinkName(c.ID, valid.ID, "synonym", nil))
	})

	it := suggestOne(t, db, "Inula selecta", conceptID)
	want := domain.MatchedName{Canonical: "Inula selecta", Authorship: "Z.Auct.", Role: "synonym", NomStatusJudgement: domain.JudgementAbsent}
	if it.MatchedName != want {
		t.Errorf("MatchedName = %+v, want %+v (the valid name wins over the illegitimate one despite sorting later)", it.MatchedName, want)
	}
	if it.MatchedNameDisqualified {
		t.Errorf("MatchedNameDisqualified = true, want false (the SELECTED name is valid)")
	}
}

// TestSuggest_ExactDisqualifiedNameBeatsMerePrefixName pins the BOUNDARY the
// disqualification step must not cross. The step sits BELOW exact equality, so
// it may only break ties among names of the SAME exactness — never let a
// merely-prefix name displace an exact one.
//
// The first version of this code got that backwards and skipped disqualified
// rows outright, which on this fixture answered with the variety: the caller
// typed a name IN FULL and was shown a longer, different name, while ExactHit
// still reported true — an answer contradicting itself. And since the shown
// name then looked clean, MatchedNameDisqualified went false and the whole new
// ranking step stopped firing for the concept, i.e. the reported bug survived
// untouched. Measured on the production index, 3,810 concepts carry such a
// pair (always "species + infraspecific taxon"); this fixture is one of them,
// wcvp:concept:100405.
//
// Every other test in this file uses fixtures where BOTH candidate names equal
// the query, so none of them can see this case — which is exactly why the bug
// passed a green suite (and a mutation run with "Not covered: 0": a MISSING
// condition is not a mutation target).
func TestSuggest_ExactDisqualifiedNameBeatsMerePrefixName(t *testing.T) {
	db := openTestDB(t)
	const conceptID = "wcvp:concept:houstonia-angustifolia"

	bv := domain.BackboneVersion{ID: "wcvp", Version: "v1", IngestedAt: "2026-09-15T00:00:00Z", ManifestSHA: "x"}
	ingestVia(t, db, bv, func(tx output.IngestTx) {
		// The valid name is the merely-longer one, the illegitimate name the
		// one the caller actually typed — the constellation that makes the
		// two keys disagree.
		variety := domain.Name{ID: "n-houstonia-var", Canonical: "Houstonia angustifolia var. rigidiuscula", Authorship: "A.Gray", Rank: domain.RankVariety}
		illegitimate := domain.Name{ID: "n-houstonia-pursh", Canonical: "Houstonia angustifolia", Authorship: "Pursh", Rank: domain.RankSpecies, NomStatus: illegitimateHomonymStatus}
		for _, n := range []domain.Name{variety, illegitimate} {
			mustTx(t, tx.UpsertName(n))
		}
		c := domain.Concept{ID: conceptID, BackboneID: "wcvp", AcceptedName: variety, Rank: domain.RankVariety, Status: domain.StatusAccepted}
		mustTx(t, tx.UpsertConcept(c))
		mustTx(t, tx.LinkName(c.ID, variety.ID, "accepted", nil))
		mustTx(t, tx.LinkName(c.ID, illegitimate.ID, "synonym", nil))
	})

	it := suggestOne(t, db, "Houstonia angustifolia", conceptID)
	want := domain.MatchedName{
		Canonical: "Houstonia angustifolia", Authorship: "Pursh", Role: "synonym",
		NomStatus: illegitimateHomonymNormalized, NomStatusJudgement: domain.JudgementDisqualifying,
	}
	if it.MatchedName != want {
		t.Errorf("MatchedName = %+v, want %+v (the EXACT name wins even though it is disqualified — the step ranks below exactness)", it.MatchedName, want)
	}
	if !it.MatchedNameDisqualified {
		t.Errorf("MatchedNameDisqualified = false, want true (the selected name IS illegitimate; hiding that disables the ranking step)")
	}
	if !it.ExactHit {
		t.Fatalf("ExactHit = false — fixture no longer exercises the exact/prefix boundary")
	}
}

// TestSuggest_ValidExactNameWinsOverBothItsRivals combines the two halves the
// previous two tests pin separately, because the rule decides over a SEQUENCE
// of rows and two-candidate fixtures cannot show that the two keys compose:
// one concept, THREE matching names spanning BOTH exactness classes.
//
// The row order makes both keys act: the disqualified exact name comes first
// (authorship A before B) and must yield to the valid exact name (same class,
// the step applies), which must then survive the valid prefix name (different
// class, the step must not reach across).
//
// HONEST SCOPE — measured, not assumed: this test does NOT catch a missing
// exactness guard. Once a valid row exists in the exact class, the buggy
// "skip every disqualified row" rule lands on the same name, because it stops
// replacing as soon as the incumbent is valid; verified by removing the guard,
// this test stays green. The guard is pinned by
// TestSuggest_ExactDisqualifiedNameBeatsMerePrefixName, whose exact class is
// disqualified THROUGHOUT — which is what forces the broken rule across the
// class boundary. What this test adds is the composition the two-candidate
// fixtures cannot show: the step firing inside a class and the class boundary
// holding, in one row sequence. A rule without the step at all picks A.Auct.
// and fails here.
func TestSuggest_ValidExactNameWinsOverBothItsRivals(t *testing.T) {
	db := openTestDB(t)
	const conceptID = "wcvp:concept:pentanema-triplex"

	bv := domain.BackboneVersion{ID: "wcvp", Version: "v1", IngestedAt: "2026-09-15T00:00:00Z", ManifestSHA: "x"}
	ingestVia(t, db, bv, func(tx output.IngestTx) {
		variety := domain.Name{ID: "n-inula-triplex-var", Canonical: "Inula triplex var. minor", Authorship: "Hook.", Rank: domain.RankVariety}
		illegitimate := domain.Name{ID: "n-inula-triplex-a", Canonical: "Inula triplex", Authorship: "A.Auct.", Rank: domain.RankSpecies, NomStatus: illegitimateHomonymStatus}
		valid := domain.Name{ID: "n-inula-triplex-b", Canonical: "Inula triplex", Authorship: "B.Auct.", Rank: domain.RankSpecies}
		for _, n := range []domain.Name{variety, illegitimate, valid} {
			mustTx(t, tx.UpsertName(n))
		}
		c := domain.Concept{ID: conceptID, BackboneID: "wcvp", AcceptedName: variety, Rank: domain.RankVariety, Status: domain.StatusAccepted}
		mustTx(t, tx.UpsertConcept(c))
		mustTx(t, tx.LinkName(c.ID, variety.ID, "accepted", nil))
		mustTx(t, tx.LinkName(c.ID, illegitimate.ID, "synonym", nil))
		mustTx(t, tx.LinkName(c.ID, valid.ID, "synonym", nil))
	})

	it := suggestOne(t, db, "Inula triplex", conceptID)
	want := domain.MatchedName{Canonical: "Inula triplex", Authorship: "B.Auct.", Role: "synonym", NomStatusJudgement: domain.JudgementAbsent}
	if it.MatchedName != want {
		t.Errorf("MatchedName = %+v, want %+v (valid beats disqualified WITHIN the exact class, and the exact class beats the prefix one)", it.MatchedName, want)
	}
	if it.MatchedNameDisqualified {
		t.Errorf("MatchedNameDisqualified = true, want false")
	}
}

// TestSuggest_ConservedNameIsNotDemotedByHavingAStatus guards the trap the
// plan names explicitly: "carries a nom_status" is NOT "is disqualified".
// ", nom. cons." is a CONSERVED name — the strongest possible statement that
// the name is to be used — and the 1,237 names carrying it must not be pushed
// behind a status-free name the way an illegitimate one is.
//
// The fixture makes the two rules disagree: the conserved name is the
// concept's ACCEPTED name (the higher-precedence role), the status-free name
// a synonym that sorts earlier by canonical/authorship only if the role key is
// overridden. A coarse has_nom_status sort key applied as if it were the
// verdict — the shortcut this test exists to forbid — demotes the conserved
// accepted name and picks the synonym.
func TestSuggest_ConservedNameIsNotDemotedByHavingAStatus(t *testing.T) {
	db := openTestDB(t)
	const conceptID = "wcvp:concept:inula-conservata"

	bv := domain.BackboneVersion{ID: "wcvp", Version: "v1", IngestedAt: "2026-09-15T00:00:00Z", ManifestSHA: "x"}
	ingestVia(t, db, bv, func(tx output.IngestTx) {
		conserved := domain.Name{ID: "n-inula-conservata-acc", Canonical: "Inula conservata", Authorship: "L.", Rank: domain.RankSpecies, NomStatus: ", nom. cons."}
		plain := domain.Name{ID: "n-inula-conservata-syn", Canonical: "Inula conservata", Authorship: "A.Auct.", Rank: domain.RankSpecies}
		for _, n := range []domain.Name{conserved, plain} {
			mustTx(t, tx.UpsertName(n))
		}
		c := domain.Concept{ID: conceptID, BackboneID: "wcvp", AcceptedName: conserved, Rank: domain.RankSpecies, Status: domain.StatusAccepted}
		mustTx(t, tx.UpsertConcept(c))
		mustTx(t, tx.LinkName(c.ID, conserved.ID, "accepted", nil))
		mustTx(t, tx.LinkName(c.ID, plain.ID, "synonym", nil))
	})

	it := suggestOne(t, db, "Inula conservata", conceptID)
	want := domain.MatchedName{Canonical: "Inula conservata", Authorship: "L.", Role: "accepted", NomStatus: "nom. cons.", NomStatusJudgement: domain.JudgementAcceptable}
	if it.MatchedName != want {
		t.Errorf("MatchedName = %+v, want %+v (a conserved name keeps its accepted-before-synonym precedence)", it.MatchedName, want)
	}
	if it.MatchedNameDisqualified {
		t.Errorf("MatchedNameDisqualified = true, want false (nom. cons. is acceptable, not a defect)")
	}
}

// TestSuggest_UnclassifiedStatusIsNotTreatedAsDisqualified guards the SECOND
// judgement that is easily mistaken for a defect, and the more dangerous one:
// JudgementUnclassified. ", sensu auct." (1,117 names) and "fossil name" (274)
// are cases where the rule table WITHHOLDS a verdict — taxonomic uncertainty
// about which plant the name is applied to, not a flaw in how the name was
// published. A derivation written as "anything not provably clean is
// suspicious" would demote them, and unlike the acceptable case nothing in the
// domain suite currently catches that; see this task's report.
//
// Same two-name shape as the conserved test, so it pins the selection as well
// as the flag: the uncertain name is the ACCEPTED one and must keep that
// precedence over the status-free synonym.
func TestSuggest_UnclassifiedStatusIsNotTreatedAsDisqualified(t *testing.T) {
	db := openTestDB(t)
	const conceptID = "wcvp:concept:inula-incerta"

	bv := domain.BackboneVersion{ID: "wcvp", Version: "v1", IngestedAt: "2026-09-15T00:00:00Z", ManifestSHA: "x"}
	ingestVia(t, db, bv, func(tx output.IngestTx) {
		uncertain := domain.Name{ID: "n-inula-incerta-acc", Canonical: "Inula incerta", Authorship: "L.", Rank: domain.RankSpecies, NomStatus: ", sensu auct."}
		plain := domain.Name{ID: "n-inula-incerta-syn", Canonical: "Inula incerta", Authorship: "A.Auct.", Rank: domain.RankSpecies}
		for _, n := range []domain.Name{uncertain, plain} {
			mustTx(t, tx.UpsertName(n))
		}
		c := domain.Concept{ID: conceptID, BackboneID: "wcvp", AcceptedName: uncertain, Rank: domain.RankSpecies, Status: domain.StatusAccepted}
		mustTx(t, tx.UpsertConcept(c))
		mustTx(t, tx.LinkName(c.ID, uncertain.ID, "accepted", nil))
		mustTx(t, tx.LinkName(c.ID, plain.ID, "synonym", nil))
	})

	it := suggestOne(t, db, "Inula incerta", conceptID)
	want := domain.MatchedName{Canonical: "Inula incerta", Authorship: "L.", Role: "accepted", NomStatus: "sensu auct.", NomStatusJudgement: domain.JudgementUnclassified}
	if it.MatchedName != want {
		t.Errorf("MatchedName = %+v, want %+v (an unjudged status keeps its accepted-before-synonym precedence)", it.MatchedName, want)
	}
	if it.MatchedNameDisqualified {
		t.Errorf("MatchedNameDisqualified = true, want false (withheld verdict is uncertainty, not a nomenclatural defect)")
	}
}

// suggestOne runs Suggest for q and returns the single item for conceptID,
// failing the test if the concept is missing — the shape the per-fixture tests
// above need and suggestByID (fixed to inulaHirtaQuery) cannot give them.
func suggestOne(t *testing.T, db *DB, q, conceptID string) domain.SuggestItem {
	t.Helper()
	items, err := db.Suggest(context.Background(), q, output.SuggestOpts{Limit: 10})
	if err != nil {
		t.Fatalf("Suggest(%q): unexpected error: %v", q, err)
	}
	for _, it := range items {
		if it.ConceptID == conceptID {
			return it
		}
	}
	t.Fatalf("Suggest(%q) returned no item for %q (got %v)", q, conceptID, conceptIDsList(items))
	return domain.SuggestItem{}
}

func itemsOf(byID map[string]domain.SuggestItem) []domain.SuggestItem {
	out := make([]domain.SuggestItem, 0, len(byID))
	for _, it := range byID {
		out = append(out, it)
	}
	return out
}

func conceptIDsList(items []domain.SuggestItem) []string {
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = it.ConceptID
	}
	return out
}

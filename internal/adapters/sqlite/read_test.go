package sqlite

import (
	"context"
	"errors"
	"os"
	"sort"
	"testing"

	"github.com/jobrunner/hostus/internal/domain"
	"github.com/jobrunner/hostus/internal/ports/output"
)

const (
	corynephorusID = "c-corynephorus-canescens"
	jacobaeaID     = "c-jacobaea-vulgaris"
)

// openSeededDB opens an in-memory database (via the shared openTestDB
// helper from db_internal_test.go) and applies testdata/seed.sql, giving
// read tests two real concepts (with synonyms, xrefs, and distribution) to
// query against — no mocks, per SP0's TDD discipline.
func openSeededDB(t *testing.T) *DB {
	t.Helper()
	db := openTestDB(t)

	seed, err := os.ReadFile("testdata/seed.sql")
	if err != nil {
		t.Fatalf("reading testdata/seed.sql: %v", err)
	}
	if _, err := db.sql.Exec(string(seed)); err != nil {
		t.Fatalf("applying testdata/seed.sql: %v", err)
	}
	return db
}

func TestConcept_ReturnsAcceptedNameSynonymsXrefsAndDistribution(t *testing.T) {
	db := openSeededDB(t)

	concept, synonyms, xrefs, dists, err := db.Concept(context.Background(), corynephorusID)
	if err != nil {
		t.Fatalf("Concept(%q): unexpected error: %v", corynephorusID, err)
	}

	t.Run("concept fields", func(t *testing.T) {
		assertConceptFields(t, concept)
	})
	t.Run("synonyms", func(t *testing.T) {
		assertCorynephorusSynonyms(t, synonyms)
	})
	t.Run("xrefs", func(t *testing.T) {
		assertCorynephorusXrefs(t, xrefs)
	})
	t.Run("distribution", func(t *testing.T) {
		assertCorynephorusDistribution(t, dists)
	})
}

func assertConceptFields(t *testing.T, concept *domain.Concept) {
	t.Helper()
	if concept.ID != corynephorusID {
		t.Errorf("concept.ID = %q, want %q", concept.ID, corynephorusID)
	}
	if concept.BackboneID != "wcvp" {
		t.Errorf("concept.BackboneID = %q, want %q", concept.BackboneID, "wcvp")
	}
	if concept.BackboneVersion != "2026-06-15" {
		t.Errorf("concept.BackboneVersion = %q, want %q", concept.BackboneVersion, "2026-06-15")
	}
	if concept.Rank != domain.RankSpecies {
		t.Errorf("concept.Rank = %q, want %q", concept.Rank, domain.RankSpecies)
	}
	if concept.Status != domain.StatusAccepted {
		t.Errorf("concept.Status = %q, want %q", concept.Status, domain.StatusAccepted)
	}
	if concept.AcceptedName.ID != "n-corynephorus-canescens" {
		t.Errorf("concept.AcceptedName.ID = %q, want %q", concept.AcceptedName.ID, "n-corynephorus-canescens")
	}
	if concept.AcceptedName.Canonical != "corynephorus canescens" {
		t.Errorf("concept.AcceptedName.Canonical = %q, want %q", concept.AcceptedName.Canonical, "corynephorus canescens")
	}
	if concept.AcceptedName.Authorship != "(L.) P.Beauv." {
		t.Errorf("concept.AcceptedName.Authorship = %q, want %q", concept.AcceptedName.Authorship, "(L.) P.Beauv.")
	}
}

func assertCorynephorusSynonyms(t *testing.T, synonyms []output.SynonymName) {
	t.Helper()
	wantSynonyms := []string{"n-aira-canescens", "n-weingaertneria-canescens"}
	gotSynonyms := synonymIDs(synonyms)
	if !equalStrings(gotSynonyms, wantSynonyms) {
		t.Errorf("synonym ids = %v, want %v", gotSynonyms, wantSynonyms)
	}
	for _, n := range synonyms {
		if n.Rank != domain.RankSpecies {
			t.Errorf("synonym %q rank = %q, want %q", n.ID, n.Rank, domain.RankSpecies)
		}
	}
}

func assertCorynephorusXrefs(t *testing.T, xrefs []domain.Xref) {
	t.Helper()
	wantXrefs := map[string]string{"powo": "396681-1", "colxr": "YQW8"}
	if len(xrefs) != len(wantXrefs) {
		t.Fatalf("xrefs = %v, want %d entries matching %v", xrefs, len(wantXrefs), wantXrefs)
	}
	for _, x := range xrefs {
		if wantXrefs[x.Authority] != x.ExtID {
			t.Errorf("xref %+v not among expected %v", x, wantXrefs)
		}
	}
}

func assertCorynephorusDistribution(t *testing.T, dists []domain.Distribution) {
	t.Helper()
	wantDists := map[string]bool{"GER": true, "FRA": true}
	if len(dists) != len(wantDists) {
		t.Fatalf("distribution = %v, want %d entries matching %v", dists, len(wantDists), wantDists)
	}
	for _, d := range dists {
		if d.AreaScheme != "wgsrpd_l3" {
			t.Errorf("distribution %+v area_scheme = %q, want wgsrpd_l3", d, d.AreaScheme)
		}
		if !wantDists[d.AreaCode] {
			t.Errorf("distribution %+v not among expected area codes %v", d, wantDists)
		}
	}
}

func TestConcept_NoDistributionRowsReturnsEmptySlice(t *testing.T) {
	db := openSeededDB(t)

	_, synonyms, xrefs, dists, err := db.Concept(context.Background(), jacobaeaID)
	if err != nil {
		t.Fatalf("Concept(%q): unexpected error: %v", jacobaeaID, err)
	}
	if len(dists) != 0 {
		t.Errorf("distribution = %v, want empty (Jacobaea vulgaris has none seeded)", dists)
	}
	if got := synonymIDs(synonyms); !equalStrings(got, []string{"n-senecio-jacobaea"}) {
		t.Errorf("synonym ids = %v, want [n-senecio-jacobaea]", got)
	}
	if len(xrefs) != 1 || xrefs[0].Authority != "powo" || xrefs[0].ExtID != "226649-1" {
		t.Errorf("xrefs = %v, want [{powo 226649-1}]", xrefs)
	}
}

func TestConcept_UnknownIDReturnsErrNotFound(t *testing.T) {
	db := openSeededDB(t)

	concept, synonyms, xrefs, dists, err := db.Concept(context.Background(), "nope")
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("Concept(%q) error = %v, want errors.Is(err, domain.ErrNotFound)", "nope", err)
	}
	if concept != nil || synonyms != nil || xrefs != nil || dists != nil {
		t.Fatalf("Concept(%q) on error: want all-nil results, got %v %v %v %v", "nope", concept, synonyms, xrefs, dists)
	}
}

func TestConceptByXref_ResolvesConcept(t *testing.T) {
	db := openSeededDB(t)

	concept, err := db.ConceptByXref(context.Background(), "powo", "396681-1")
	if err != nil {
		t.Fatalf(`ConceptByXref("powo", "396681-1"): unexpected error: %v`, err)
	}
	if concept.ID != corynephorusID {
		t.Errorf("concept.ID = %q, want %q", concept.ID, corynephorusID)
	}
}

func TestConceptByXref_SecondAuthorityAlsoResolves(t *testing.T) {
	db := openSeededDB(t)

	concept, err := db.ConceptByXref(context.Background(), "colxr", "YQW8")
	if err != nil {
		t.Fatalf(`ConceptByXref("colxr", "YQW8"): unexpected error: %v`, err)
	}
	if concept.ID != corynephorusID {
		t.Errorf("concept.ID = %q, want %q", concept.ID, corynephorusID)
	}
}

func TestConceptByXref_UnknownExtIDReturnsErrNotFound(t *testing.T) {
	db := openSeededDB(t)

	concept, err := db.ConceptByXref(context.Background(), "powo", "nope")
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf(`ConceptByXref("powo", "nope") error = %v, want errors.Is(err, domain.ErrNotFound)`, err)
	}
	if concept != nil {
		t.Errorf(`ConceptByXref("powo", "nope") concept = %v, want nil`, concept)
	}
}

func TestConceptByXref_UnknownAuthorityReturnsErrNotFound(t *testing.T) {
	db := openSeededDB(t)

	// Same ext_id as a real xref, but under an authority that never
	// recorded it — must not match on ext_id alone.
	_, err := db.ConceptByXref(context.Background(), "wikidata", "396681-1")
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf(`ConceptByXref("wikidata", "396681-1") error = %v, want errors.Is(err, domain.ErrNotFound)`, err)
	}
}

func TestConceptIDsByXref_ResolvesOnlyKnownExtIDsForAuthority(t *testing.T) {
	db := openSeededDB(t)

	got, err := db.ConceptIDsByXref(context.Background(), "powo", []string{"396681-1", "226649-1", "nope"})
	if err != nil {
		t.Fatalf("ConceptIDsByXref: unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len(ConceptIDsByXref) = %d, want 2 (the unknown id must be absent, not zero-valued)", len(got))
	}
	if got["396681-1"] != corynephorusID {
		t.Errorf(`got["396681-1"] = %q, want %q`, got["396681-1"], corynephorusID)
	}
	if got["226649-1"] != jacobaeaID {
		t.Errorf(`got["226649-1"] = %q, want %q`, got["226649-1"], jacobaeaID)
	}
	if _, ok := got["nope"]; ok {
		t.Errorf(`got["nope"] present, want it absent from the result map`)
	}
}

func TestConceptIDsByXref_WrongAuthorityDoesNotMatchOnExtIDAlone(t *testing.T) {
	db := openSeededDB(t)

	got, err := db.ConceptIDsByXref(context.Background(), "wikidata", []string{"396681-1"})
	if err != nil {
		t.Fatalf("ConceptIDsByXref: unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("ConceptIDsByXref(wikidata, [396681-1]) = %v, want empty (that ext_id was never recorded under wikidata)", got)
	}
}

func TestConceptIDsByXref_EmptyExtIDsReturnsEmptyNotError(t *testing.T) {
	db := openSeededDB(t)

	got, err := db.ConceptIDsByXref(context.Background(), "powo", nil)
	if err != nil {
		t.Fatalf("ConceptIDsByXref: unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("ConceptIDsByXref(powo, nil) = %v, want empty", got)
	}
}

func TestMatchExact_AcceptedNameReturnsAcceptedCandidate(t *testing.T) {
	db := openSeededDB(t)

	got, err := db.MatchExact(context.Background(), "corynephorus canescens")
	if err != nil {
		t.Fatalf("MatchExact: unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("MatchExact(%q) = %v, want exactly 1 candidate", "corynephorus canescens", got)
	}
	c := got[0]
	if c.Role != "accepted" {
		t.Errorf("candidate.Role = %q, want %q", c.Role, "accepted")
	}
	if c.MatchedName.ID != "n-corynephorus-canescens" {
		t.Errorf("candidate.MatchedName.ID = %q, want %q", c.MatchedName.ID, "n-corynephorus-canescens")
	}
	if c.Concept.ID != corynephorusID {
		t.Errorf("candidate.Concept.ID = %q, want %q", c.Concept.ID, corynephorusID)
	}
	if c.Concept.AcceptedName.ID != "n-corynephorus-canescens" {
		t.Errorf("candidate.Concept.AcceptedName.ID = %q, want %q", c.Concept.AcceptedName.ID, "n-corynephorus-canescens")
	}
}

func TestMatchExact_SynonymReturnsSynonymCandidateCarryingAcceptedConcept(t *testing.T) {
	db := openSeededDB(t)

	got, err := db.MatchExact(context.Background(), "weingaertneria canescens")
	if err != nil {
		t.Fatalf("MatchExact: unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("MatchExact(%q) = %v, want exactly 1 candidate", "weingaertneria canescens", got)
	}
	c := got[0]
	if c.Role != "synonym" {
		t.Errorf("candidate.Role = %q, want %q", c.Role, "synonym")
	}
	if c.MatchedName.ID != "n-weingaertneria-canescens" {
		t.Errorf("candidate.MatchedName.ID = %q, want %q", c.MatchedName.ID, "n-weingaertneria-canescens")
	}
	// The candidate must carry the ACCEPTED concept (Corynephorus), not a
	// concept keyed on the synonym itself — concept_name has no row where
	// a synonym name id doubles as a concept id, so this also pins the
	// join going through concept_name -> taxon_concept correctly.
	if c.Concept.ID != corynephorusID {
		t.Errorf("candidate.Concept.ID = %q, want %q (the accepted concept)", c.Concept.ID, corynephorusID)
	}
	if c.Concept.AcceptedName.Canonical != "corynephorus canescens" {
		t.Errorf("candidate.Concept.AcceptedName.Canonical = %q, want %q", c.Concept.AcceptedName.Canonical, "corynephorus canescens")
	}
}

// TestMatchExact_FoldsDiacriticsAndMixedCase is the regression test for the
// bug the fold-column fix addresses: the seeded Weingaertneria synonym's
// stored `canonical` deliberately carries diacritics and non-ASCII casing
// ("Wéingaertneria canéscens" — see testdata/seed.sql), so this only
// passes if matching goes through canonical_fold rather than SQLite's
// ASCII-only LOWER() on canonical directly. Both a plain-ASCII query and a
// diacritic-bearing query must resolve to the same candidate.
func TestMatchExact_FoldsDiacriticsAndMixedCase(t *testing.T) {
	db := openSeededDB(t)

	for _, query := range []string{
		"Weingaertneria canescens", // plain ASCII, mixed case
		"wéingaertneria canéscens", // diacritic-bearing, lower case
		"WÉINGAERTNERIA CANÉSCENS", // diacritic-bearing, upper case
	} {
		t.Run(query, func(t *testing.T) {
			got, err := db.MatchExact(context.Background(), query)
			if err != nil {
				t.Fatalf("MatchExact(%q): unexpected error: %v", query, err)
			}
			if len(got) != 1 {
				t.Fatalf("MatchExact(%q) = %v, want exactly 1 candidate", query, got)
			}
			if got[0].Role != "synonym" || got[0].MatchedName.ID != "n-weingaertneria-canescens" || got[0].Concept.ID != corynephorusID {
				t.Errorf("MatchExact(%q) candidate = %+v, want role=synonym, matched=n-weingaertneria-canescens, concept=%q", query, got[0], corynephorusID)
			}
		})
	}
}

func TestMatchExact_BasionymSynonymAlsoMatches(t *testing.T) {
	db := openSeededDB(t)

	got, err := db.MatchExact(context.Background(), "aira canescens")
	if err != nil {
		t.Fatalf("MatchExact: unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("MatchExact(%q) = %v, want exactly 1 candidate", "aira canescens", got)
	}
	if got[0].Role != "synonym" || got[0].Concept.ID != corynephorusID {
		t.Errorf("candidate = %+v, want role=synonym, concept=%q", got[0], corynephorusID)
	}
}

func TestMatchExact_IsCaseAndWhitespaceInsensitive(t *testing.T) {
	db := openSeededDB(t)

	got, err := db.MatchExact(context.Background(), "  Corynephorus   Canescens ")
	if err != nil {
		t.Fatalf("MatchExact: unexpected error: %v", err)
	}
	if len(got) != 1 || got[0].Concept.ID != corynephorusID {
		t.Errorf("MatchExact with mixed case/whitespace = %v, want 1 candidate for %q", got, corynephorusID)
	}
}

func TestMatchExact_NoMatchReturnsEmptySlice(t *testing.T) {
	db := openSeededDB(t)

	got, err := db.MatchExact(context.Background(), "nothing")
	if err != nil {
		t.Fatalf("MatchExact: unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("MatchExact(%q) = %v, want empty", "nothing", got)
	}
}

func TestMatchExact_DoesNotConfuseDistinctEpithets(t *testing.T) {
	db := openSeededDB(t)

	// Regression guard for domain.ClassifyMatch's documented rule: congeneric
	// names differing only in epithet never match ("silene otites" vs
	// "silene otitis"). Jacobaea vulgaris must not surface when querying a
	// different (unseeded) epithet in the same genus.
	got, err := db.MatchExact(context.Background(), "jacobaea alpina")
	if err != nil {
		t.Fatalf("MatchExact: unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("MatchExact(%q) = %v, want empty", "jacobaea alpina", got)
	}
}

func TestMatchFuzzyCandidates_ReturnsNearMissForTypo(t *testing.T) {
	db := openSeededDB(t)

	got, err := db.MatchFuzzyCandidates(context.Background(), "festuca ovina", 10, "", "")
	if err != nil {
		t.Fatalf("MatchFuzzyCandidates: unexpected error: %v", err)
	}
	ids := make(map[string]bool, len(got))
	for _, c := range got {
		ids[c.MatchedName.ID] = true
	}
	if !ids["n-festuca-ovona"] || !ids["n-festuca-ovena"] {
		t.Fatalf("MatchFuzzyCandidates(%q) ids = %v, want both near-miss names present", "festuca ovina", ids)
	}
}

func TestMatchFuzzyCandidates_ExcludesUnrelatedNames(t *testing.T) {
	db := openSeededDB(t)

	// "abies alba" shares neither first letter nor length-window with
	// "festuca ovina" — the prefilter must exclude it even though it's a
	// perfectly real seeded name, proving the query doesn't fall back to a
	// full scan.
	got, err := db.MatchFuzzyCandidates(context.Background(), "festuca ovina", 10, "", "")
	if err != nil {
		t.Fatalf("MatchFuzzyCandidates: unexpected error: %v", err)
	}
	for _, c := range got {
		if c.MatchedName.ID == "n-abies-alba" {
			t.Fatalf("MatchFuzzyCandidates(%q) = %v, want n-abies-alba excluded by the prefilter", "festuca ovina", got)
		}
	}
}

func TestMatchFuzzyCandidates_RespectsLimit(t *testing.T) {
	db := openSeededDB(t)

	got, err := db.MatchFuzzyCandidates(context.Background(), "festuca ovina", 1, "", "")
	if err != nil {
		t.Fatalf("MatchFuzzyCandidates: unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("MatchFuzzyCandidates(%q, limit=1) = %d candidates, want exactly 1", "festuca ovina", len(got))
	}
}

// TestMatchFuzzyCandidates_OrdersByClosestLengthFirst pins the fix-round-1
// gap: fuzzyCandidateNameIDs had no ORDER BY before its LIMIT, so when the
// prefilter matched more rows than limit, SQLite could return an arbitrary
// subset — potentially truncating away the closest (best) match before
// domain.Similarity ever scored it. The fixture admits three candidates at
// different length-diffs from "festuca ovina" (n-festuca-ovona/-ovena at
// diff 0, n-festuca-ovinaxy at diff 2); with limit=1, the closest (diff-0)
// row must always win, deterministically, never the diff-2 one.
func TestMatchFuzzyCandidates_OrdersByClosestLengthFirst(t *testing.T) {
	db := openSeededDB(t)

	got, err := db.MatchFuzzyCandidates(context.Background(), "festuca ovina", 1, "", "")
	if err != nil {
		t.Fatalf("MatchFuzzyCandidates: unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("MatchFuzzyCandidates(%q, limit=1) = %d candidates, want exactly 1", "festuca ovina", len(got))
	}
	id := got[0].MatchedName.ID
	if id != "n-festuca-ovona" && id != "n-festuca-ovena" {
		t.Errorf("MatchFuzzyCandidates(%q, limit=1) returned %q, want one of the length-diff-0 names (n-festuca-ovona/n-festuca-ovena), not the farther n-festuca-ovinaxy", "festuca ovina", id)
	}
}

func TestMatchFuzzyCandidates_ZeroLimitUsesDefault(t *testing.T) {
	db := openSeededDB(t)

	// limit <= 0 must fall back to the adapter's default cap (not silently
	// become a SQL LIMIT 0, which would return nothing at all).
	got, err := db.MatchFuzzyCandidates(context.Background(), "festuca ovina", 0, "", "")
	if err != nil {
		t.Fatalf("MatchFuzzyCandidates: unexpected error: %v", err)
	}
	if len(got) == 0 {
		t.Fatalf("MatchFuzzyCandidates(%q, limit=0) = empty, want the default cap to apply (near-misses present)", "festuca ovina")
	}
}

func TestMatchFuzzyCandidates_NoNearMissReturnsEmpty(t *testing.T) {
	db := openSeededDB(t)

	got, err := db.MatchFuzzyCandidates(context.Background(), "zzznonexistent", 10, "", "")
	if err != nil {
		t.Fatalf("MatchFuzzyCandidates: unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("MatchFuzzyCandidates(%q) = %v, want empty", "zzznonexistent", got)
	}
}

func synonymIDs(names []output.SynonymName) []string {
	ids := make([]string, 0, len(names))
	for _, n := range names {
		ids = append(ids, n.ID)
	}
	sort.Strings(ids)
	return ids
}

// equalStrings reports whether a and b contain the same strings, ignoring
// order (both inputs are already-sorted-by-caller id lists, but this
// compares by count rather than by index to sidestep any assumption about
// matching order).
func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	counts := make(map[string]int, len(a))
	for _, s := range a {
		counts[s]++
	}
	for _, s := range b {
		counts[s]--
	}
	for _, n := range counts {
		if n != 0 {
			return false
		}
	}
	return true
}

// TestGlobEscape pins the literal-pattern contract of globEscape: GLOB's
// three metacharacters must come back wrapped in a single-character bracket
// set (GLOB has no backslash escape), everything else untouched. ']' is
// only special INSIDE a bracket set, so it passes through unchanged.
func TestGlobEscape(t *testing.T) {
	cases := map[string]string{
		"f": "f",
		"*": "[*]",
		"?": "[?]",
		"[": "[[]",
		"]": "]",
		"ä": "ä",
		"":  "",
	}
	for in, want := range cases {
		if got := globEscape(in); got != want {
			t.Errorf("globEscape(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestMatchFuzzyCandidates_GlobMetacharacterFirstRuneIsLiteral pins the
// prefilter against a query whose first rune is a GLOB metacharacter.
// domain.Canonicalize keeps punctuation, so a caller-supplied "*" or "?"
// reaches fuzzyCandidateNameIDs verbatim; unescaped, "*"+"*" is a pattern
// matching EVERY name, turning the prefilter into a no-op full scan that
// then returns candidates for a query nothing should match. Escaped, the
// rune is literal — no seeded name starts with "*" or "?", so the result is
// empty.
func TestMatchFuzzyCandidates_GlobMetacharacterFirstRuneIsLiteral(t *testing.T) {
	db := openSeededDB(t)

	// Same length as "festuca ovina", so only the first rune decides.
	for _, q := range []string{"*estuca ovina", "?estuca ovina"} {
		got, err := db.MatchFuzzyCandidates(context.Background(), q, 10, "", "")
		if err != nil {
			t.Fatalf("MatchFuzzyCandidates(%q): unexpected error: %v", q, err)
		}
		if len(got) != 0 {
			t.Errorf("MatchFuzzyCandidates(%q) = %d candidates, want 0: the first rune must filter LITERALLY, not as a GLOB wildcard", q, len(got))
		}
	}
}

// TestMatchFuzzyCandidates_BracketFirstRuneStillFiltersNormally pins the
// other metacharacter: an unescaped "[" opens a bracket set that the
// appended "*" never closes, and an unterminated set matches nothing —
// silently disabling fuzzy matching. Escaped, "[" is a literal the seeded
// name below really starts with, so the near-miss is found.
func TestMatchFuzzyCandidates_BracketFirstRuneStillFiltersNormally(t *testing.T) {
	db := openSeededDB(t)
	ctx := context.Background()

	tx, err := db.BeginIngest(ctx, domain.BackboneVersion{ID: "bracket", Version: "v1"})
	if err != nil {
		t.Fatalf("BeginIngest: unexpected error: %v", err)
	}
	name := domain.Name{ID: "n-bracket", Canonical: "[bracketus testicus", Rank: domain.RankSpecies}
	concept := domain.Concept{ID: "c-bracket", BackboneID: "bracket", AcceptedName: name, Rank: domain.RankSpecies, Status: domain.StatusAccepted}
	if err := tx.UpsertName(name); err != nil {
		t.Fatalf("UpsertName: unexpected error: %v", err)
	}
	if err := tx.UpsertConcept(concept); err != nil {
		t.Fatalf("UpsertConcept: unexpected error: %v", err)
	}
	if err := tx.LinkName(concept.ID, name.ID, "accepted", nil); err != nil {
		t.Fatalf("LinkName: unexpected error: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: unexpected error: %v", err)
	}

	got, err := db.MatchFuzzyCandidates(ctx, "[bracketus testicos", 10, "", "")
	if err != nil {
		t.Fatalf("MatchFuzzyCandidates: unexpected error: %v", err)
	}
	found := false
	for _, c := range got {
		if c.MatchedName.ID == "n-bracket" {
			found = true
		}
	}
	if !found {
		t.Errorf("MatchFuzzyCandidates = %v, want n-bracket: an unterminated bracket set must not silently match nothing", got)
	}
}

// ingestOtherRankConcept writes one OTHER-ranked accepted concept (WCVP's
// "proles") plus one OTHER-ranked synonym under it, via the real
// IngestTx — not seed.sql, since this is a self-contained fixture rather
// than a shared one — so the fix-round-1 rank_verbatim scanning tests
// below have something real to read back through Concept/MatchExact.
func ingestOtherRankConcept(t *testing.T, db *DB) {
	t.Helper()
	ctx := context.Background()
	tx, err := db.BeginIngest(ctx, domain.BackboneVersion{ID: "wcvp", Version: "v1", IngestedAt: "2026-07-31T00:00:00Z", ManifestSHA: "x"})
	if err != nil {
		t.Fatalf("BeginIngest: unexpected error: %v", err)
	}

	accepted := domain.Name{ID: "n-other-accepted", Canonical: "Paeonia corallina proles ovatifolia", Authorship: "Rouy & Foucaud", Rank: domain.RankOther, RankVerbatim: "proles"}
	synonym := domain.Name{ID: "n-other-synonym", Canonical: "Paeonia mascula proles ovatifolia", Rank: domain.RankOther, RankVerbatim: "lusus"}
	concept := domain.Concept{ID: "c-other", BackboneID: "wcvp", AcceptedName: accepted, Rank: domain.RankOther, RankVerbatim: "proles", Status: domain.StatusAccepted}

	if err := tx.UpsertName(accepted); err != nil {
		t.Fatalf("UpsertName(accepted): unexpected error: %v", err)
	}
	if err := tx.UpsertName(synonym); err != nil {
		t.Fatalf("UpsertName(synonym): unexpected error: %v", err)
	}
	if err := tx.UpsertConcept(concept); err != nil {
		t.Fatalf("UpsertConcept: unexpected error: %v", err)
	}
	if err := tx.LinkName(concept.ID, accepted.ID, "accepted", nil); err != nil {
		t.Fatalf("LinkName(accepted): unexpected error: %v", err)
	}
	if err := tx.LinkName(concept.ID, synonym.ID, "synonym", nil); err != nil {
		t.Fatalf("LinkName(synonym): unexpected error: %v", err)
	}
	if err := tx.Finalize(); err != nil {
		t.Fatalf("Finalize: unexpected error: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: unexpected error: %v", err)
	}
}

// TestConcept_OtherRank_ScansAcceptedNameAndSynonymRankVerbatim is the
// fix-round-1 regression for scanConcept's accepted-name branch and
// scanName's synonym branch: Concept must populate RankVerbatim on BOTH
// the concept itself (already covered by
// TestExportBundle_CarriesRankVerbatimThrough in bundle_test.go) AND its
// embedded AcceptedName, and conceptSynonyms/scanName must do the same for
// an OTHER-ranked synonym — none of which the shared seed.sql fixture (all
// SPECIES) exercises.
func TestConcept_OtherRank_ScansAcceptedNameAndSynonymRankVerbatim(t *testing.T) {
	db := openTestDB(t)
	ingestOtherRankConcept(t, db)

	concept, synonyms, _, _, err := db.Concept(context.Background(), "c-other")
	if err != nil {
		t.Fatalf("Concept(c-other): unexpected error: %v", err)
	}
	if concept.AcceptedName.Rank != domain.RankOther {
		t.Errorf("concept.AcceptedName.Rank = %q, want %q", concept.AcceptedName.Rank, domain.RankOther)
	}
	if concept.AcceptedName.RankVerbatim != "proles" {
		t.Errorf("concept.AcceptedName.RankVerbatim = %q, want %q", concept.AcceptedName.RankVerbatim, "proles")
	}

	if len(synonyms) != 1 {
		t.Fatalf("len(synonyms) = %d, want 1", len(synonyms))
	}
	if synonyms[0].Rank != domain.RankOther {
		t.Errorf("synonym.Rank = %q, want %q", synonyms[0].Rank, domain.RankOther)
	}
	if synonyms[0].RankVerbatim != "lusus" {
		t.Errorf("synonym.RankVerbatim = %q, want %q", synonyms[0].RankVerbatim, "lusus")
	}
}

// TestMatchExact_OtherRank_ScansRankVerbatimOnMatchedConceptAndAcceptedName
// is the fix-round-1 regression for scanMatchCandidateRows: querying
// MatchExact for an OTHER-ranked ACCEPTED name (so matched == accepted ==
// concept's own rank, all three RankOther branches in one row) must
// populate RankVerbatim on the matched name, the concept, and its embedded
// accepted name alike.
func TestMatchExact_OtherRank_ScansRankVerbatimOnMatchedConceptAndAcceptedName(t *testing.T) {
	db := openTestDB(t)
	ingestOtherRankConcept(t, db)

	got, err := db.MatchExact(context.Background(), "Paeonia corallina proles ovatifolia")
	if err != nil {
		t.Fatalf("MatchExact: unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(MatchExact) = %d, want 1", len(got))
	}
	candidate := got[0]

	if candidate.MatchedName.Rank != domain.RankOther {
		t.Errorf("MatchedName.Rank = %q, want %q", candidate.MatchedName.Rank, domain.RankOther)
	}
	if candidate.MatchedName.RankVerbatim != "proles" {
		t.Errorf("MatchedName.RankVerbatim = %q, want %q", candidate.MatchedName.RankVerbatim, "proles")
	}
	if candidate.Concept.RankVerbatim != "proles" {
		t.Errorf("Concept.RankVerbatim = %q, want %q", candidate.Concept.RankVerbatim, "proles")
	}
	if candidate.Concept.AcceptedName.RankVerbatim != "proles" {
		t.Errorf("Concept.AcceptedName.RankVerbatim = %q, want %q", candidate.Concept.AcceptedName.RankVerbatim, "proles")
	}
}

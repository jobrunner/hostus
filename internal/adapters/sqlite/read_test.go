package sqlite

import (
	"context"
	"errors"
	"os"
	"sort"
	"testing"

	"github.com/jobrunner/hostus/internal/domain"
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

func assertCorynephorusSynonyms(t *testing.T, synonyms []domain.Name) {
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

func synonymIDs(names []domain.Name) []string {
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

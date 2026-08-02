package sqlite

import (
	"context"
	"errors"
	"testing"

	"github.com/jobrunner/hostus/internal/domain"
)

const uc5ConceptID = "c-uc5-corynephorus"

// findCandidate returns the candidate with the given name id, failing the
// test if it is absent.
func findCandidate(t *testing.T, items []domain.SynonymCandidate, nameID string) domain.SynonymCandidate {
	t.Helper()
	for _, c := range items {
		if c.NameID == nameID {
			return c
		}
	}
	t.Fatalf("no synonym candidate with name id %q among %d candidates", nameID, len(items))
	return domain.SynonymCandidate{}
}

func TestSynonymCandidates_CarriesNomStatusHomotypicAndRank(t *testing.T) {
	db := openSeededDB(t)

	items, err := db.SynonymCandidates(context.Background(), uc5ConceptID)
	if err != nil {
		t.Fatalf("SynonymCandidates(%q): unexpected error: %v", uc5ConceptID, err)
	}
	if len(items) != 10 {
		t.Fatalf("got %d candidates, want 10 (see testdata/seed.sql's c-uc5-corynephorus block)", len(items))
	}

	incanescens := findCandidate(t, items, "n-uc5-corynephorus-incanescens")
	if incanescens.NomStatus != ", nom. illeg. superfl." {
		t.Errorf("incanescens NomStatus = %q, want the verbatim %q", incanescens.NomStatus, ", nom. illeg. superfl.")
	}
	if incanescens.Canonical != "uc5 corynephorus incanescens" || incanescens.Authorship != "Bubani" {
		t.Errorf("incanescens name = %q %q, want %q %q", incanescens.Canonical, incanescens.Authorship, "uc5 corynephorus incanescens", "Bubani")
	}
	if incanescens.ConceptID != uc5ConceptID {
		t.Errorf("incanescens ConceptID = %q, want %q", incanescens.ConceptID, uc5ConceptID)
	}

	pallidus := findCandidate(t, items, "n-uc5-f-pallidus")
	if pallidus.Rank != domain.RankForm {
		t.Errorf("f. pallidus Rank = %q, want %q", pallidus.Rank, domain.RankForm)
	}
	if pallidus.NomStatus != "" {
		t.Errorf("f. pallidus NomStatus = %q, want empty (no status recorded)", pallidus.NomStatus)
	}

	homotypic := findCandidate(t, items, "n-uc5-avena-canescens")
	if homotypic.Homotypic == nil || !*homotypic.Homotypic {
		t.Errorf("avena canescens Homotypic = %v, want a pointer to true", homotypic.Homotypic)
	}
	unknown := findCandidate(t, items, "n-uc5-aira-breviculmis")
	if unknown.Homotypic != nil {
		t.Errorf("aira breviculmis Homotypic = %v (points to %v), want nil — a NULL homotypic is UNKNOWN, never heterotypic", unknown.Homotypic, *unknown.Homotypic)
	}
}

// TestSynonymCandidates_PopulatesIsBasionym is the test the port's doc
// comment promises. n-uc5-aira-canescens IS c-uc5-corynephorus' accepted
// name's basionym_id; every other synonym of that concept is not. An
// adapter that hardcoded IsBasionym to false (or to true) fails here — and
// nothing in internal/domain could have caught it, since RankSynonyms
// consumes the flag rather than deriving it.
func TestSynonymCandidates_PopulatesIsBasionym(t *testing.T) {
	db := openSeededDB(t)

	items, err := db.SynonymCandidates(context.Background(), uc5ConceptID)
	if err != nil {
		t.Fatalf("SynonymCandidates(%q): unexpected error: %v", uc5ConceptID, err)
	}

	var basionyms []string
	for _, c := range items {
		if c.IsBasionym {
			basionyms = append(basionyms, c.NameID)
		}
	}
	if len(basionyms) != 1 || basionyms[0] != "n-uc5-aira-canescens" {
		t.Fatalf("IsBasionym is set on %v, want exactly [n-uc5-aira-canescens]", basionyms)
	}
}

// TestSynonymCandidates_ScansHeterotypicZero pins the tri-state scan on the
// one fixture row storing homotypic = 0. See testdata/seed.sql's
// c-uc5-heterotypic block: the measured index contains no such row, so only
// a fixture can prove the adapter does not collapse 0 onto NULL or true.
func TestSynonymCandidates_ScansHeterotypicZero(t *testing.T) {
	db := openSeededDB(t)

	items, err := db.SynonymCandidates(context.Background(), "c-uc5-heterotypic")
	if err != nil {
		t.Fatalf("SynonymCandidates: unexpected error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("got %d candidates, want 1", len(items))
	}
	if items[0].Homotypic == nil {
		t.Fatalf("Homotypic = nil, want a pointer to false (stored homotypic = 0)")
	}
	if *items[0].Homotypic {
		t.Errorf("Homotypic points to true, want false (stored homotypic = 0)")
	}
	if got := domain.TypificationOf(items[0].Homotypic); got != domain.TypificationHeterotypic {
		t.Errorf("TypificationOf = %q, want %q", got, domain.TypificationHeterotypic)
	}
}

func TestSynonymCandidates_OrderedByNameID(t *testing.T) {
	db := openSeededDB(t)

	items, err := db.SynonymCandidates(context.Background(), uc5ConceptID)
	if err != nil {
		t.Fatalf("SynonymCandidates: unexpected error: %v", err)
	}
	for i := 1; i < len(items); i++ {
		if items[i-1].NameID >= items[i].NameID {
			t.Fatalf("candidates not ordered by name id: %q before %q", items[i-1].NameID, items[i].NameID)
		}
	}
}

func TestSynonymCandidates_UnknownConceptIsNotFound(t *testing.T) {
	db := openSeededDB(t)

	_, err := db.SynonymCandidates(context.Background(), "c-does-not-exist")
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("SynonymCandidates(unknown) error = %v, want domain.ErrNotFound", err)
	}
}

func TestSynonymCandidates_KnownConceptWithoutSynonymsIsEmptyNotError(t *testing.T) {
	db := openSeededDB(t)

	items, err := db.SynonymCandidates(context.Background(), "c-uc5-genus")
	if err != nil {
		t.Fatalf("SynonymCandidates(c-uc5-genus): unexpected error: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("got %d candidates, want 0", len(items))
	}
}

// TestConcept_SynonymNameShapeUnchanged guards the constraint that adding
// SynonymCandidates must not perturb /v1/concept/{id}: Concept() still
// returns output.SynonymName, still without nom_status-derived fields, and
// still for the same concepts.
func TestConcept_SynonymNameShapeUnchanged(t *testing.T) {
	db := openSeededDB(t)

	concept, synonyms, xrefs, _, err := db.Concept(context.Background(), uc5ConceptID)
	if err != nil {
		t.Fatalf("Concept(%q): unexpected error: %v", uc5ConceptID, err)
	}
	if concept.ID != uc5ConceptID || len(xrefs) != 0 {
		t.Fatalf("Concept(%q) = %v with %d xrefs, want the concept itself and none", uc5ConceptID, concept, len(xrefs))
	}
	if len(synonyms) != 10 {
		t.Fatalf("Concept returned %d synonyms, want 10", len(synonyms))
	}
	for _, s := range synonyms {
		if s.ID == "n-uc5-avena-canescens" && (s.Homotypic == nil || !*s.Homotypic) {
			t.Errorf("Concept synonym %q Homotypic = %v, want a pointer to true", s.ID, s.Homotypic)
		}
	}
}

package app

import (
	"testing"

	"github.com/jobrunner/hostus/internal/adapters/namelist"
)

// TestClassificationFor_WalksParentChainToFamilyOrderClass pins the
// traversal Task 4 adds: a species row's Family/Order/Class come from
// walking ParentID upward through the SAME source's row set (species rows
// and the higher-rank Fall-B rows alike) until each rank is found once.
func TestClassificationFor_WalksParentChainToFamilyOrderClass(t *testing.T) {
	byID := map[string]namelist.Row{
		"cls-1": {SourceID: "cls-1", Taxon: "Magnoliopsida", Rank: "Class"},
		"ord-1": {SourceID: "ord-1", Taxon: "Caryophyllales", Rank: "Order", ParentID: "cls-1"},
		"fam-1": {SourceID: "fam-1", Taxon: "Chenopodiaceae", Rank: "Family", ParentID: "ord-1"},
		"gen-1": {SourceID: "gen-1", Taxon: "Salsola", Rank: "Genus", ParentID: "fam-1"},
	}
	species := namelist.Row{SourceID: "sp-1", Taxon: "Salsola kali", Rank: "Species", ParentID: "gen-1"}

	family, order, class := classificationFor(species, byID)
	if family != "Chenopodiaceae" {
		t.Errorf("family = %q, want %q", family, "Chenopodiaceae")
	}
	if order != "Caryophyllales" {
		t.Errorf("order = %q, want %q", order, "Caryophyllales")
	}
	if class != "Magnoliopsida" {
		t.Errorf("class = %q, want %q", class, "Magnoliopsida")
	}
}

// TestClassificationFor_WalksParentChainWithGermanSLRankCodes is the
// fix-round-1 regression test: GermanSL's real TaxonRank values are short
// codes ("FAM", "ORD", "KLA"), not WCVP's full words ("Family", "Order",
// "Class"). Before domain.ParseRankLenient learned GermanSL's rank-code
// table, classificationFor never recognized these ancestors at all —
// every GermanSL species silently got empty Family/OrderName/ClassName.
func TestClassificationFor_WalksParentChainWithGermanSLRankCodes(t *testing.T) {
	byID := map[string]namelist.Row{
		"kla-1": {SourceID: "kla-1", Taxon: "Magnoliopsida", Rank: "KLA"},
		"ord-1": {SourceID: "ord-1", Taxon: "Caryophyllales", Rank: "ORD", ParentID: "kla-1"},
		"fam-1": {SourceID: "fam-1", Taxon: "Chenopodiaceae", Rank: "FAM", ParentID: "ord-1"},
		"gat-1": {SourceID: "gat-1", Taxon: "Salsola", Rank: "GAT", ParentID: "fam-1"},
	}
	species := namelist.Row{SourceID: "spe-1", Taxon: "Salsola kali", Rank: "SPE", ParentID: "gat-1"}

	family, order, class := classificationFor(species, byID)
	if family != "Chenopodiaceae" {
		t.Errorf("family = %q, want %q", family, "Chenopodiaceae")
	}
	if order != "Caryophyllales" {
		t.Errorf("order = %q, want %q", order, "Caryophyllales")
	}
	if class != "Magnoliopsida" {
		t.Errorf("class = %q, want %q", class, "Magnoliopsida")
	}
}

// TestClassificationFor_MissingParentReturnsWhateverWasFound pins the
// no-guess rule: a chain that runs out of parents before reaching every
// rank returns empty strings for the ranks it never found, rather than
// erroring or fabricating a value.
func TestClassificationFor_MissingParentReturnsWhateverWasFound(t *testing.T) {
	byID := map[string]namelist.Row{
		"fam-1": {SourceID: "fam-1", Taxon: "Chenopodiaceae", Rank: "Family"},
		"gen-1": {SourceID: "gen-1", Taxon: "Salsola", Rank: "Genus", ParentID: "fam-1"},
	}
	species := namelist.Row{SourceID: "sp-1", Taxon: "Salsola kali", Rank: "Species", ParentID: "gen-1"}

	family, order, class := classificationFor(species, byID)
	if family != "Chenopodiaceae" {
		t.Errorf("family = %q, want %q", family, "Chenopodiaceae")
	}
	if order != "" {
		t.Errorf("order = %q, want empty (no Order ancestor in the chain)", order)
	}
	if class != "" {
		t.Errorf("class = %q, want empty (no Class ancestor in the chain)", class)
	}
}

// TestClassificationFor_DanglingParentIDStopsCleanly pins the defensive
// posture: a ParentID that points at a row absent from byID (malformed bulk
// pipeline data) stops the walk instead of panicking on a missing map entry.
func TestClassificationFor_DanglingParentIDStopsCleanly(t *testing.T) {
	species := namelist.Row{SourceID: "sp-1", Taxon: "Salsola kali", Rank: "Species", ParentID: "does-not-exist"}

	family, order, class := classificationFor(species, map[string]namelist.Row{})
	if family != "" || order != "" || class != "" {
		t.Errorf("classificationFor(dangling parent) = (%q,%q,%q), want all empty", family, order, class)
	}
}

// TestClassificationFor_CyclicParentChainTerminates pins the maxHops guard:
// a cyclic ParentID chain (malformed source data) must terminate rather than
// loop forever.
func TestClassificationFor_CyclicParentChainTerminates(t *testing.T) {
	byID := map[string]namelist.Row{
		"a": {SourceID: "a", Taxon: "A", Rank: "Genus", ParentID: "b"},
		"b": {SourceID: "b", Taxon: "B", Rank: "Genus", ParentID: "a"},
	}
	species := namelist.Row{SourceID: "sp-1", Taxon: "Salsola kali", Rank: "Species", ParentID: "a"}

	// classificationFor is synchronous; the test itself terminating (rather
	// than hanging) is the primary assertion that maxHops bounded the cycle.
	family, order, class := classificationFor(species, byID)
	if family != "" || order != "" || class != "" {
		t.Errorf("classificationFor(cyclic chain, no Family/Order/Class ranks) = (%q,%q,%q), want all empty", family, order, class)
	}
}

// TestNameSpaceRowSourceRows_CarriesClassificationAndVernacular exercises
// the bridge end to end: Rows() builds the byID map from the WHOLE dataset
// (species rows and higher-rank rows alike) and fills each species row's
// application.NameRow with its resolved Family/Order/Class plus the
// pass-through VernacularDE.
func TestNameSpaceRowSourceRows_CarriesClassificationAndVernacular(t *testing.T) {
	ds := &namelist.Dataset{
		Rows: []namelist.Row{
			{SourceID: "cls-1", Taxon: "Magnoliopsida", Rank: "Class"},
			{SourceID: "ord-1", Taxon: "Caryophyllales", Rank: "Order", ParentID: "cls-1"},
			{SourceID: "fam-1", Taxon: "Chenopodiaceae", Rank: "Family", ParentID: "ord-1"},
			{SourceID: "sp-1", Taxon: "Salsola kali", Rank: "Species", Status: "accepted", ParentID: "fam-1", VernacularDE: "Kali-Salzkraut"},
		},
	}
	src := nameSpaceRowSource{ds: ds}

	rows := src.Rows()
	if len(rows) != 4 {
		t.Fatalf("Rows() = %d rows, want 4", len(rows))
	}
	found := false
	for _, r := range rows {
		if r.SourceID != "sp-1" {
			continue
		}
		found = true
		if r.Family != "Chenopodiaceae" || r.OrderName != "Caryophyllales" || r.ClassName != "Magnoliopsida" {
			t.Errorf("species classification = %+v, want Family=Chenopodiaceae OrderName=Caryophyllales ClassName=Magnoliopsida", r)
		}
		if r.VernacularDE != "Kali-Salzkraut" {
			t.Errorf("species.VernacularDE = %q, want %q", r.VernacularDE, "Kali-Salzkraut")
		}
	}
	if !found {
		t.Fatal("species row (sp-1) not found in Rows() output")
	}
}

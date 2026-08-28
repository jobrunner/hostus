package domain_test

import (
	"testing"

	"github.com/jobrunner/hostus/internal/domain"
)

// TestParseRankLenient_EuroSLGoldenVocabulary pins ParseRankLenient's
// mapping for EVERY distinct raw TaxonRank value actually measured in the
// pinned EuroSL artifact (pipelines/eurosl/eurosl.summary.txt: 139,039
// rows, 29 distinct rank spellings — see
// docs/research/2026-08-27-drift-check.md). GermanSL's raw short-code
// vocabulary is already golden-tested in taxon_test.go
// (TestParseRankLenient_GermanSLRankCodes +
// TestParseRankLenient_GermanSLDeliberatelyUnmappedCodesStayOther); this
// function is EuroSL's counterpart, not a duplicate of either — no two
// tests in this package assert the same raw-value -> Rank mapping twice.
//
// IMPORTANT — this table is NOT "the mapping a reader would expect from
// each spelling's English meaning" in the abstract; it pins
// ParseRankLenient's REAL, measured behavior. Seven raw EuroSL spellings
// (Species Aggregate, Unranked (infraspecific), Unranked (infrageneric),
// Coll. species, Grex (infraspec.), Subsection bot., Division) are mapped
// via euroSLRankAliases (internal/domain/taxon.go) — EuroSL's own literal,
// space/punctuation-bearing column text does not match canonicalRanks'
// underscore-joined enum-style spelling (e.g. "SPECIES_AGGREGATE" vs.
// "Species Aggregate"), so a dedicated alias table (the EuroSL counterpart
// of germanSLRankCodes for GermanSL) bridges that gap, the same way
// nothotaxonRanks and germanSLRankCodes do for their own vocabularies. See
// the Task 13 follow-up report for the full writeup.
func TestParseRankLenient_EuroSLGoldenVocabulary(t *testing.T) {
	golden := map[string]domain.Rank{
		"Species":    domain.RankSpecies,
		"Subspecies": domain.RankSubspecies,
		"Variety":    domain.RankVariety,
		"Genus":      domain.RankGenus,
		"Form":       domain.RankForm,
		"Family":     domain.RankFamily,
		// "Unranked (infraspecific)" (296 rows) maps via euroSLRankAliases
		// to RankUnrankedInfraspecific.
		"Unranked (infraspecific)": domain.RankUnrankedInfraspecific,
		// "Species Aggregate" (287 rows — exactly the pinned aggregate
		// count from docs/research/2026-08-27-drift-check.md) maps via
		// euroSLRankAliases to RankSpeciesAggregate.
		"Species Aggregate": domain.RankSpeciesAggregate,
		"Subvariety":        domain.RankSubvariety,
		"Section":           domain.RankSection,
		// "Coll. species" (155 rows) maps via euroSLRankAliases to
		// RankCollSpecies.
		"Coll. species": domain.RankCollSpecies,
		"Order":         domain.RankOrder,
		"Tribe":         domain.RankTribe,
		"Subgenus":      domain.RankSubgenus,
		"Subform":       domain.RankSubform,
		"Proles":        domain.RankProles,
		"Subfamily":     domain.RankSubfamily,
		"Subclass":      domain.RankSubclass,
		"Race":          domain.RankRace,
		// "Unranked (infrageneric)" (19 rows) maps via euroSLRankAliases to
		// RankUnrankedInfrageneric.
		"Unranked (infrageneric)": domain.RankUnrankedInfrageneric,
		"Class":                   domain.RankClass,
		// "Grex (infraspec.)" (15 rows) maps via euroSLRankAliases to
		// RankGrex.
		"Grex (infraspec.)": domain.RankGrex,
		"Superorder":        domain.RankSuperorder,
		// "Subsection bot." (8 rows) maps via euroSLRankAliases to
		// RankSubsection.
		"Subsection bot.": domain.RankSubsection,
		"Subdivision":     domain.RankSubdivision,
		"Phylum":          domain.RankPhylum,
		// "Division" (2 rows, EuroSL's own synonym spelling for Phylum)
		// maps via euroSLRankAliases to RankPhylum — the biological
		// synonymy is honored explicitly there, not via canonicalRanks.
		"Division": domain.RankPhylum,
		"Convar":   domain.RankConvar,
		"Root":     domain.RankRoot,
		// Deliberately, correctly unmapped: a domain/bookkeeping node in
		// EuroSL's tree, not a real taxonomic rank (see Task 1's design
		// notes) — this one row is expected to stay RankOther.
		"Suprageneric Taxon": domain.RankOther,
	}
	// NOTE: pipelines/eurosl/eurosl.summary.txt's "ranks=" histogram
	// actually lists 30 distinct raw values, not the 29 this task's brief
	// (and its Task 3 predecessor) assumed — see the Task 13 report.
	if len(golden) != 30 {
		t.Fatalf("golden vocabulary has %d entries, want 30 (pipelines/eurosl/eurosl.summary.txt distinct rank count)", len(golden))
	}
	for raw, want := range golden {
		t.Run(raw, func(t *testing.T) {
			got, verbatim := domain.ParseRankLenient(raw)
			if got != want {
				t.Errorf("ParseRankLenient(%q) = %q, want %q", raw, got, want)
			}
			if verbatim != raw {
				t.Errorf("ParseRankLenient(%q) verbatim = %q, want %q", raw, verbatim, raw)
			}
		})
	}
}

// TestParseRankLenient_TrulyUnknownValueFallsBackToOther pins the contract
// a brand-new, never-before-seen raw rank spelling (i.e. one appearing in
// neither the EuroSL nor the GermanSL golden vocabularies above/in
// taxon_test.go) must still degrade gracefully to RankOther rather than
// being silently guessed at or erroring the ingest.
func TestParseRankLenient_TrulyUnknownValueFallsBackToOther(t *testing.T) {
	got, verbatim := domain.ParseRankLenient("ein-nie-gesehener-rang")
	if got != domain.RankOther {
		t.Errorf("ParseRankLenient(unbekannt) = %q, want %q", got, domain.RankOther)
	}
	if verbatim != "ein-nie-gesehener-rang" {
		t.Errorf("verbatim = %q, want %q", verbatim, "ein-nie-gesehener-rang")
	}
}

// TestParseRankLenient_GermanSLRootCode closes the one gap in GermanSL's
// golden coverage: pipelines/germansl/germansl.summary.txt's "ranks="
// histogram lists 26 distinct raw codes (not the 27 an earlier plan draft
// assumed), 25 of which are already golden-tested in taxon_test.go
// (TestParseRankLenient_GermanSLRankCodes's 23 measured codes — it also
// lists the unmeasured "CL2" for completeness — plus
// TestParseRankLenient_GermanSLDeliberatelyUnmappedCodesStayOther's UAB/
// AG3). The 26th, "ROOT", is deliberately NOT added to
// TestParseRankLenient_GermanSLRankCodes: that test's table also asserts
// ParseRank (the strict parser) must REJECT every code in it, but "ROOT"
// is GermanSL's raw spelling AND the canonical Rank spelling at once, so
// ParseRank("ROOT") correctly succeeds — asserting the opposite there
// would itself be wrong. It gets its own minimal case here instead.
func TestParseRankLenient_GermanSLRootCode(t *testing.T) {
	rank, verbatim := domain.ParseRankLenient("ROOT")
	if rank != domain.RankRoot {
		t.Errorf("ParseRankLenient(%q) = %q, want %q", "ROOT", rank, domain.RankRoot)
	}
	if verbatim != "ROOT" {
		t.Errorf("ParseRankLenient(%q) verbatim = %q, want %q", "ROOT", verbatim, "ROOT")
	}
}

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
// each spelling's English meaning". It pins ParseRankLenient's REAL,
// measured behavior as of this task, including seven raw EuroSL spellings
// that this task found silently degrade to RankOther even though a
// matching canonical constant (RankSpeciesAggregate,
// RankUnrankedInfraspecific, RankUnrankedInfrageneric, RankCollSpecies,
// RankGrex, RankSubsection, RankPhylum) already exists — see the "KNOWN
// DEFECT" comments below and the Task 13 report for the full writeup.
// canonicalRanks (internal/domain/taxon.go) only recognizes the
// underscore-joined enum-style spelling of each of these ranks (e.g.
// "SPECIES_AGGREGATE"), not EuroSL's literal, space/punctuation-bearing
// column text (e.g. "Species Aggregate"), and no EuroSL-specific alias
// table (the equivalent of germanSLRankCodes for GermanSL) exists yet to
// bridge that gap. A future fix must update this golden list deliberately,
// not let it silently regress back to RankOther expectations.
func TestParseRankLenient_EuroSLGoldenVocabulary(t *testing.T) {
	golden := map[string]domain.Rank{
		"Species":    domain.RankSpecies,
		"Subspecies": domain.RankSubspecies,
		"Variety":    domain.RankVariety,
		"Genus":      domain.RankGenus,
		"Form":       domain.RankForm,
		"Family":     domain.RankFamily,
		// KNOWN DEFECT: EuroSL's literal "Unranked (infraspecific)" (296
		// rows) is not recognized and degrades to RankOther instead of
		// RankUnrankedInfrageneric's sibling constant RankUnrankedInfraspecific.
		"Unranked (infraspecific)": domain.RankOther,
		// KNOWN DEFECT: EuroSL's literal "Species Aggregate" (287 rows —
		// exactly the pinned aggregate count from
		// docs/research/2026-08-27-drift-check.md) is not recognized and
		// degrades to RankOther instead of RankSpeciesAggregate. As
		// measured, the EuroSL ingest path cannot currently rely on Rank
		// alone to detect any of these 287 aggregate rows.
		"Species Aggregate": domain.RankOther,
		"Subvariety":        domain.RankSubvariety,
		"Section":           domain.RankSection,
		// KNOWN DEFECT: "Coll. species" (155 rows) degrades to RankOther
		// instead of RankCollSpecies.
		"Coll. species": domain.RankOther,
		"Order":         domain.RankOrder,
		"Tribe":         domain.RankTribe,
		"Subgenus":      domain.RankSubgenus,
		"Subform":       domain.RankSubform,
		"Proles":        domain.RankProles,
		"Subfamily":     domain.RankSubfamily,
		"Subclass":      domain.RankSubclass,
		"Race":          domain.RankRace,
		// KNOWN DEFECT: "Unranked (infrageneric)" (19 rows) degrades to
		// RankOther instead of RankUnrankedInfrageneric.
		"Unranked (infrageneric)": domain.RankOther,
		"Class":                   domain.RankClass,
		// KNOWN DEFECT: "Grex (infraspec.)" (15 rows) degrades to
		// RankOther instead of RankGrex.
		"Grex (infraspec.)": domain.RankOther,
		"Superorder":        domain.RankSuperorder,
		// KNOWN DEFECT: "Subsection bot." (8 rows) degrades to RankOther
		// instead of RankSubsection.
		"Subsection bot.": domain.RankOther,
		"Subdivision":     domain.RankSubdivision,
		"Phylum":          domain.RankPhylum,
		// KNOWN DEFECT: "Division" (2 rows, EuroSL's own synonym spelling
		// for Phylum) degrades to RankOther. Nothing in canonicalRanks
		// aliases "DIVISION" to RankPhylum, so the biological synonymy is
		// NOT honored by the current parser, despite an earlier plan
		// assumption that it already was.
		"Division": domain.RankOther,
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

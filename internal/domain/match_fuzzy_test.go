package domain_test

import (
	"testing"

	"github.com/jobrunner/hostus/internal/domain"
)

// TestFuzzyResolves_RejectsSharedEpithetAcrossUnrelatedGenera pins the
// measured precision failure of whole-string similarity alone: an epithet is
// the LONGER half of a binomial, so an identical epithet can carry two
// unrelated genera over FuzzyThreshold. Measured against a real index
// (docs/research/fuzzy-prefilter.md): 19 of 62 ESy names above the threshold
// were wrong this way, all of them non-vascular genera WCVP does not carry.
func TestFuzzyResolves_RejectsSharedEpithetAcrossUnrelatedGenera(t *testing.T) {
	t.Parallel()
	// Each pair is above FuzzyThreshold as a whole string — that is the
	// point; the guard has to look at the genus separately.
	pairs := [][2]string{
		{"sphagnum platyphyllum", "solanum platyphyllum"},
		{"cladonia fimbriata", "caladenia fimbriata"},
		{"kurzia pauciflora", "kunzea pauciflora"},
		{"thuidium tamariscinum", "thesium tamariscinum"},
		{"climacium dendroides", "limonium dendroides"},
	}
	for _, p := range pairs {
		if whole := domain.Similarity(p[0], p[1]); whole < domain.FuzzyThreshold {
			t.Fatalf("test setup: Similarity(%q, %q) = %v, want >= %v so the guard is what decides",
				p[0], p[1], whole, domain.FuzzyThreshold)
		}
		if domain.FuzzyResolves(p[0], p[1]) {
			t.Errorf("FuzzyResolves(%q, %q) = true, want false (unrelated genera sharing an epithet)", p[0], p[1])
		}
	}
}

// TestFuzzyResolves_AcceptsMisspelledGenus is the other side of the same
// guard: a genuine spelling mistake shifts the genus by at most a letter, so
// these must keep resolving. All measured live cases.
func TestFuzzyResolves_AcceptsMisspelledGenus(t *testing.T) {
	t.Parallel()
	pairs := [][2]string{
		{"cochleria amana", "cochlearia amana"},
		{"dorystoechas hastata", "dorystaechas hastata"},
		{"bellidiastrum michelii", "bellidastrum michelii"},
		{"corynephorus canascens", "corynephorus canescens"},
	}
	for _, p := range pairs {
		if !domain.FuzzyResolves(p[0], p[1]) {
			t.Errorf("FuzzyResolves(%q, %q) = false, want true (a misspelled genus is still the same genus)", p[0], p[1])
		}
	}
}

// TestFuzzyResolves_SkipsLeadingHybridMarker guards a measured true positive
// the guard would otherwise kill: the two spellings of the marker share no
// character, so comparing "x" against "×" as if it were the genus scores 0.
func TestFuzzyResolves_SkipsLeadingHybridMarker(t *testing.T) {
	t.Parallel()
	if !domain.FuzzyResolves("x ammocalamagrostis baltica", "× ammocalamagrostis baltica") {
		t.Error("FuzzyResolves(x ammocalamagrostis baltica, × ammocalamagrostis baltica) = false, want true")
	}
	if !domain.FuzzyResolves("abies borisii-regis", "abies × borisii-regis") {
		t.Error("FuzzyResolves(abies borisii-regis, abies × borisii-regis) = false, want true")
	}
}

// TestFuzzyResolves_SingleWordQueryFallsBackToWholeString: with no epithet
// there is nothing to guard against, so the guard must not reject what
// Similarity accepts.
func TestFuzzyResolves_SingleWordQueryFallsBackToWholeString(t *testing.T) {
	t.Parallel()
	if !domain.FuzzyResolves("astragalis", "astragalus") {
		t.Error("FuzzyResolves(astragalis, astragalus) = false, want true (genus-only query)")
	}
}

// TestFuzzyResolves_StillRequiresTheWholeStringThreshold: the guard is an
// ADDITIONAL condition, never a replacement. A genus rename keeps the epithet
// identical, so the genus check alone would be the wrong question — and the
// whole string (0.792 for the case from issue #67) has to keep deciding.
func TestFuzzyResolves_StillRequiresTheWholeStringThreshold(t *testing.T) {
	t.Parallel()
	if domain.FuzzyResolves("astracantha diphtherites", "astragalus diphtherites") {
		t.Error("FuzzyResolves(astracantha diphtherites, astragalus diphtherites) = true, want false (below the whole-string threshold)")
	}
	if domain.FuzzyResolves("festuca ovina", "festuca rubra") {
		t.Error("FuzzyResolves(festuca ovina, festuca rubra) = true, want false (same genus, different species)")
	}
}

// TestFuzzyResolves_RejectsARankMarkerMismatch pins a false-positive class the
// prefilter fix newly exposed, measured end-to-end against a real index: five
// ESy rows naming a SECTION ("Taraxacum sect. Alpina") resolved onto a
// SPECIES whose epithet merely resembles the abbreviation ("Taraxacum
// sectum", 0.875). The genus is identical, so the genus guard cannot see it.
//
// A rank abbreviation is not an epithet, and a name carrying one denotes a
// different kind of thing than a name without one — however close the two
// strings happen to be.
func TestFuzzyResolves_RejectsARankMarkerMismatch(t *testing.T) {
	t.Parallel()
	// The author-splitting step has already taken the capitalized section
	// name off, which is how this reaches the scorer this short.
	if domain.FuzzyResolves("taraxacum sect.", "taraxacum sectum") {
		t.Error("FuzzyResolves(taraxacum sect., taraxacum sectum) = true, want false (a rank marker is not an epithet)")
	}
	if domain.FuzzyResolves("taraxacum sectum", "taraxacum sect.") {
		t.Error("FuzzyResolves is not symmetric on the rank-marker guard; both directions must refuse")
	}
}

// TestFuzzyResolves_AcceptsARankMarkerOnBothSides: the guard is about a
// MISMATCH, not about markers as such. Two infraspecific names that both
// carry the marker are exactly the case fuzzy matching is for — measured
// live: "Astracantha parnassi subsp. calabricus" -> "... subsp. calabrica".
func TestFuzzyResolves_AcceptsARankMarkerOnBothSides(t *testing.T) {
	t.Parallel()
	if !domain.FuzzyResolves("astracantha parnassi subsp. calabricus", "astracantha parnassi subsp. calabrica") {
		t.Error("FuzzyResolves(... subsp. calabricus, ... subsp. calabrica) = false, want true")
	}
}

// TestFuzzyResolves_WholeStringThresholdIsInclusive: FuzzyThreshold's contract
// is inclusive (see its doc comment and the application-level
// TestMatchNames_FuzzyThresholdIsInclusiveAtExactBoundary), so a pair landing
// EXACTLY on it has to resolve. Without a case sitting precisely on the
// boundary, "< threshold" and "<= threshold" are indistinguishable — the
// mutation run showed exactly that survivor.
//
// The pair is engineered, not botanical: 20 runes, 3 substitutions, identical
// genus -> whole-string 0.850000 exactly, genus 1.0.
func TestFuzzyResolves_WholeStringThresholdIsInclusive(t *testing.T) {
	t.Parallel()
	const a, b = "festuca abcdefghijkl", "festuca abcdefghixyz"
	if got := domain.Similarity(a, b); got != domain.FuzzyThreshold {
		t.Fatalf("test setup: Similarity = %v, want exactly %v", got, domain.FuzzyThreshold)
	}
	if !domain.FuzzyResolves(a, b) {
		t.Error("FuzzyResolves at exactly FuzzyThreshold = false, want true (the threshold is inclusive)")
	}
}

// TestFuzzyResolves_GenusThresholdIsInclusive is the same boundary for the
// genus condition: a genus landing exactly on the threshold must still pass,
// or the guard would reject a case the measurement says it should keep.
//
// Engineered likewise: a 20-rune genus with 3 substitutions -> genus 0.850000
// exactly, while the whole string stays comfortably above at 0.869565.
func TestFuzzyResolves_GenusThresholdIsInclusive(t *testing.T) {
	t.Parallel()
	const a, b = "abcdefghijklmnopqrst aa", "abcdefghijklmnopqxyz aa"
	if got := domain.Similarity("abcdefghijklmnopqrst", "abcdefghijklmnopqxyz"); got != domain.FuzzyThreshold {
		t.Fatalf("test setup: genus Similarity = %v, want exactly %v", got, domain.FuzzyThreshold)
	}
	if got := domain.Similarity(a, b); got < domain.FuzzyThreshold {
		t.Fatalf("test setup: whole-string Similarity = %v, must stay above the threshold so only the genus condition is under test", got)
	}
	if !domain.FuzzyResolves(a, b) {
		t.Error("FuzzyResolves with the genus at exactly FuzzyThreshold = false, want true")
	}
}

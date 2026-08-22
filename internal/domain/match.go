package domain

import "strings"

// MatchType classifies how a query name matched a candidate name.
type MatchType string

const (
	// MatchExact: canonical name matches and the query carried no author
	// to disambiguate against.
	MatchExact MatchType = "exact"
	// MatchExactAuthor: canonical name and author both match.
	MatchExactAuthor MatchType = "exact_author"
	// MatchAggregateAlias: the candidate is an aggregate concept covering
	// the query (assigned by callers; not produced by ClassifyMatch).
	MatchAggregateAlias MatchType = "aggregate_alias"
	// MatchAggregateNominate: the query named an AGGREGATE, the index holds
	// no aggregate taxon for it, and the answer is the NOMINATE taxon under
	// it. Deliberately its own type rather than MatchExact: the answer is
	// narrower than the question — an aggregate covers more than the
	// nominate species — and a consumer that could not tell the two apart
	// would carry that narrowing into its own data unmarked.
	//
	// It is also not MatchAggregateAlias: there the index really does carry
	// the aggregate, so nothing was narrowed. The distinction is the whole
	// point of having two types.
	MatchAggregateNominate MatchType = "aggregate_nominate"
	// MatchFuzzy: no exact/exact_author/aggregate candidate was found, but a
	// candidate's canonical is similar enough (see Similarity,
	// FuzzyThreshold) to surface for review. Never produced by
	// ClassifyMatch — assigned by the application layer once fuzzy scoring
	// clears the threshold.
	MatchFuzzy MatchType = "fuzzy"
)

// FuzzyThreshold is the minimum Similarity score for a fuzzy candidate to be
// considered plausible enough to surface as a MatchFuzzy result (always
// with RequiresReview, per spec §B.2) rather than left UNRESOLVABLE.
//
// Tuned against the two reference cases spec §B.2 names explicitly: a
// single-letter epithet typo across otherwise-identical names ("silene
// otitis" vs the correctly-spelled "silene otites") scores ~0.923 and must
// match; two genuinely different congeneric species ("festuca ovina" vs
// "festuca rubra", sharing only the genus and word count) score ~0.692 and
// must not. 0.85 sits strictly between the two with headroom on both sides.
const FuzzyThreshold = 0.85

// Similarity returns a's and b's normalized Levenshtein similarity, in
// [0,1]: 1 - distance/maxLen, where distance is the Levenshtein edit
// distance (in runes) and maxLen is the longer input's rune length. 1.0
// means identical. a and b are expected to already be canonicalized by the
// caller (e.g. via Canonicalize) — Similarity does no normalization of its
// own and is symmetric: Similarity(a, b) == Similarity(b, a).
//
// Both inputs empty -> 1.0 (zero edits needed to turn "" into ""); exactly
// one empty -> 0.0 (every rune of the non-empty side would need inserting,
// i.e. distance == maxLen).
func Similarity(a, b string) float64 {
	ra, rb := []rune(a), []rune(b)
	if len(ra) == 0 && len(rb) == 0 {
		return 1.0
	}
	if len(ra) == 0 || len(rb) == 0 {
		return 0.0
	}
	maxLen := len(ra)
	// len(rb) > maxLen is a genuinely equivalent mutant at
	// CONDITIONALS_BOUNDARY (>=): when len(rb) == maxLen exactly, the
	// reassignment below sets maxLen to the value it already holds, so no
	// test can observe a difference at that boundary (same reasoning as
	// the documented equivalents in internal/application/suggest.go).
	if len(rb) > maxLen {
		maxLen = len(rb)
	}
	return 1.0 - float64(levenshteinDistance(ra, rb))/float64(maxLen)
}

// levenshteinDistance computes the classic Levenshtein edit distance (in
// runes) between a and b via the standard O(len(a)*len(b))-time,
// O(min(len(a),len(b)))-space dynamic-programming table: two rolling rows
// instead of a full matrix, since only the previous row is ever needed.
func levenshteinDistance(a, b []rune) int {
	// Iterate over the shorter side so the rolling rows stay as small as
	// possible. len(a) > len(b), at CONDITIONALS_BOUNDARY (>=) AND
	// CONDITIONALS_NEGATION (<), are both genuinely equivalent mutants
	// here: the swap only ever changes which slice is labeled "a" vs "b"
	// for the DP table's internal bookkeeping (row size, iteration order)
	// — Levenshtein distance is symmetric in its two inputs regardless of
	// that labeling, so the computed distance is identical whichever way
	// (or whether) the swap fires, at every input length, not just at the
	// len(a)==len(b) boundary. Only performance (rolling-row width)
	// depends on it, which no correctness-observing test can assert on.
	if len(a) > len(b) {
		a, b = b, a
	}
	prev := make([]int, len(a)+1)
	curr := make([]int, len(a)+1)
	for i := range prev {
		prev[i] = i
	}
	for j := 1; j <= len(b); j++ {
		curr[0] = j
		for i := 1; i <= len(a); i++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			deletion := prev[i] + 1
			insertion := curr[i-1] + 1
			substitution := prev[i-1] + cost
			m := deletion
			// insertion < m and substitution < m are each a genuinely
			// equivalent mutant at CONDITIONALS_BOUNDARY (<=): when the
			// right-hand value ties the current m exactly, reassigning m
			// to it is a no-op (same value), so no test can observe a
			// difference at that tie — the classic min-selection idiom's
			// boundary case (same reasoning as the documented equivalents
			// in internal/application/suggest.go). CONDITIONALS_NEGATION
			// (>=) on either line is NOT equivalent, though: it flips which
			// values trigger reassignment far more broadly (any
			// insertion/substitution not strictly less, not just those
			// tied at the boundary), which is a real, observable
			// mis-computation — see TestSimilarity's unequal-length cases.
			if insertion < m {
				m = insertion
			}
			if substitution < m {
				m = substitution
			}
			curr[i] = m
		}
		prev, curr = curr, prev
	}
	return prev[len(a)]
}

// ClassifyMatch decides whether a query (queryCanon, queryAuthor) matches a
// candidate (candCanon, candAuthor), per §B.2:
//   - if the canonical names differ, there is no match (ok=false), even for
//     congeneric names differing only in epithet (e.g. "silene otites" vs
//     "silene otitis" never match);
//   - if canonicals match and both authors are non-empty and equal ->
//     MatchExactAuthor;
//   - if canonicals match and the query supplied no author -> MatchExact;
//   - otherwise (canonicals match but authors are present and differ) ->
//     no match.
//
// All four inputs are expected to already be normalized (via Canonicalize /
// NormalizeAuthor) by the caller; ClassifyMatch itself does no
// normalization.
func ClassifyMatch(queryCanon, queryAuthor, candCanon, candAuthor string) (MatchType, bool) {
	if queryCanon != candCanon {
		return "", false
	}
	if queryAuthor == "" {
		return MatchExact, true
	}
	if queryAuthor == candAuthor {
		return MatchExactAuthor, true
	}
	return "", false
}

// FuzzyResolves reports whether candCanon is close enough to queryCanon to be
// RESOLVED to — as opposed to merely listed as a near miss. Both inputs are
// expected to be canonicalized already (Similarity does no normalization).
//
// Two conditions, both required:
//
//  1. the whole strings are within FuzzyThreshold, as before; and
//  2. their GENUS tokens are too.
//
// Condition 2 exists because condition 1 alone is measurably not enough. An
// epithet is the LONGER half of a binomial, so an identical epithet can drag
// two entirely unrelated genera over the threshold: "sphagnum platyphyllum"
// vs "solanum platyphyllum" scores 0.857. Measured against a real index
// (docs/research/fuzzy-prefilter.md), 19 of the 62 ESy names that cleared
// the threshold were wrong in exactly this way — 30.6 % — and every one of
// them was a bryophyte or lichen genus that the vascular-plant backbone does
// not carry at all, matched onto an unrelated flowering plant. Resolving
// those is worse for a consumer than returning nothing.
//
// The genus check uses the SAME FuzzyThreshold rather than a second, separate
// knob: 0.85 is where the measurement puts the split (a misspelled genus
// stays at 0.900-1.000 — "cochleria"/"cochlearia" = 0.900 — while a confused
// one falls to 0.625-0.778), and an independently tunable second threshold
// would be a number nothing measured.
//
// It is a guard on RESOLUTION only. The application layer still lists
// guard-rejected candidates as near misses: "sphagnum platyphyllum ->
// solanum platyphyllum" is useful evidence for whoever curates the row, as
// long as it is not handed back as an answer.
//
// Known residue, not fixable by any string measure: a genus that differs from
// an unrelated one in a single letter passes ("buellia punctata" vs "ruellia
// punctata", genus 0.857) — 1 of the measured 19.
func FuzzyResolves(queryCanon, candCanon string) bool {
	if Similarity(queryCanon, candCanon) < FuzzyThreshold {
		return false
	}
	if hasRankMarker(queryCanon) != hasRankMarker(candCanon) {
		return false
	}
	return Similarity(genusToken(queryCanon), genusToken(candCanon)) >= FuzzyThreshold
}

// The infrageneric markers below are not decoration. Measured end-to-end
// against a real index, five ESy rows naming a section ("Taraxacum sect.
// Alpina", whose capitalized section name the author-splitting step takes off,
// leaving "taraxacum sect.") resolved onto the SPECIES "Taraxacum sectum" at
// 0.875 — same genus, so the genus guard cannot see it, and two edits apart,
// so the whole-string threshold cannot either.
// infragenericMarkers are the rank abbreviations ABOVE species level. They
// are kept apart from normalize.go's infraspecificMarkers rather than merged
// into it because AutonymBase reads that set to decide what an autonym is,
// and a section is not an infraspecific rank — widening it there would change
// what AutonymBase collapses.
var infragenericMarkers = map[string]bool{
	"sect.":    true,
	"subsect.": true,
	"ser.":     true,
	"subser.":  true,
	"subg.":    true,
	"subgen.":  true,
}

// hasRankMarker reports whether a canonicalized name carries a rank
// abbreviation. FuzzyResolves compares the two sides' answers rather than
// either one alone: a marker on both sides is the ordinary infraspecific case
// fuzzy matching exists for ("... subsp. calabricus" -> "... subsp.
// calabrica"), and it is only the MISMATCH that says the two names denote
// different kinds of thing.
func hasRankMarker(canon string) bool {
	for _, f := range strings.Fields(canon) {
		// Composed from the two sets rather than a third hand-written list:
		// a marker added to one place and forgotten here would silently make
		// this guard blind to it.
		if infraspecificMarkers[f] || infragenericMarkers[f] {
			return true
		}
	}
	return false
}

// genusToken returns the token a canonicalized name's genus lives in: the
// first one, skipping a leading hybrid marker.
//
// Skipping the marker is not a taxonomic judgement (NormalizeHybridMarker
// deliberately refuses to read a LEADING standalone "x" as a nothogenus
// marker, and this does not change that) — it only decides which WORDS get
// compared against each other. It is required for correctness: the two
// spellings of the marker share no character, so comparing "x" against "×"
// as though it were the genus scores 0.0 and would reject a measured true
// positive ("x ammocalamagrostis baltica" -> "× ammocalamagrostis baltica",
// whole-string 0.963). No genus is one character long, so nothing else can
// be swallowed by this.
//
// An empty name yields "", which Similarity handles (both empty -> 1.0, one
// empty -> 0.0); a name that is NOTHING but a marker likewise yields "".
func genusToken(canon string) string {
	for _, f := range strings.Fields(canon) {
		if f == hybridMarker || f == "x" {
			continue
		}
		return f
	}
	return ""
}

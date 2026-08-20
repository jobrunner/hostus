package domain

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

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
)

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

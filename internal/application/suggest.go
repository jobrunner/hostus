package application

import (
	"context"
	"errors"
	"strings"

	"github.com/jobrunner/hostus/internal/domain"
	"github.com/jobrunner/hostus/internal/ports/output"
)

// ErrEmptyQuery is returned by Suggest when the request's Q is empty or
// whitespace-only. Handlers map it to the INVALID_QUERY error code.
var ErrEmptyQuery = errors.New("application: empty query")

// defaultSuggestLimit and maxSuggestLimit bound SuggestRequest.Limit: a
// value <= 0 falls back to defaultSuggestLimit, and any value above
// maxSuggestLimit is capped there — protecting the repo's fetch budget
// (and the eventual JSON response) from an unbounded caller-supplied limit.
const (
	defaultSuggestLimit = 10
	maxSuggestLimit     = 50
)

// SuggestRequest is one autosuggest query.
type SuggestRequest struct {
	Q     string
	Area  string
	Ranks []domain.Rank
	Limit int
}

// SuggestResponse is the ranked, truncated result of Suggest, plus the
// backbone versions the caller can attribute the results to.
type SuggestResponse struct {
	BackboneVersions map[string]string
	Results          []domain.SuggestItem
}

// Suggest resolves req against repo: it validates req.Q, calls
// repo.Suggest with an effective limit (defaulted/capped from req.Limit)
// so the adapter's own fetch budget isn't truncated, ranks the (unranked)
// results via domain.RankSuggestions, truncates to the effective limit, and
// attaches the repo's BackboneVersions.
func Suggest(ctx context.Context, repo output.Repository, req SuggestRequest) (SuggestResponse, error) {
	if strings.TrimSpace(req.Q) == "" {
		return SuggestResponse{}, ErrEmptyQuery
	}

	limit := effectiveLimit(req.Limit)

	items, err := repo.Suggest(ctx, req.Q, output.SuggestOpts{
		Area:  req.Area,
		Ranks: req.Ranks,
		Limit: limit,
	})
	if err != nil {
		return SuggestResponse{}, err
	}

	ranked := domain.RankSuggestions(items)
	// len(ranked) > limit is a genuinely equivalent mutant at
	// CONDITIONALS_BOUNDARY (>=): when len(ranked) == limit exactly,
	// ranked[:limit] reproduces the same slice content either branch
	// takes, so no test can observe a difference at that boundary (see
	// the analogous, already-documented case in
	// internal/adapters/sqlite/suggest_internal_test.go's fetchBudget
	// table; this task's report records the same finding for Suggest).
	if len(ranked) > limit {
		ranked = ranked[:limit]
	}

	versions, err := repo.BackboneVersions(ctx)
	if err != nil {
		return SuggestResponse{}, err
	}
	backboneVersions := make(map[string]string, len(versions))
	for _, v := range versions {
		backboneVersions[v.ID] = v.Version
	}

	return SuggestResponse{BackboneVersions: backboneVersions, Results: ranked}, nil
}

// effectiveLimit applies Suggest's default/cap policy to a caller-supplied
// limit: <= 0 defaults to defaultSuggestLimit, and anything above
// maxSuggestLimit is capped there.
func effectiveLimit(limit int) int {
	if limit <= 0 {
		return defaultSuggestLimit
	}
	// limit > maxSuggestLimit is likewise a genuinely equivalent mutant at
	// CONDITIONALS_BOUNDARY (>=): when limit == maxSuggestLimit exactly,
	// both branches return maxSuggestLimit, so no output-based test can
	// distinguish them at that boundary (same reasoning as the truncate
	// step above and the sqlite fetchBudget precedent).
	if limit > maxSuggestLimit {
		return maxSuggestLimit
	}
	return limit
}

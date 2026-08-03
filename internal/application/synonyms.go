package application

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jobrunner/hostus/internal/domain"
	"github.com/jobrunner/hostus/internal/ports/output"
)

// Relevance modes for GET /v1/concept/{id}/synonyms.
//
// RelevanceAll is the DEFAULT, deliberately. UC5's filter is a strong one —
// on the measured index it withholds 20 of Corynephorus canescens' 26
// synonyms — and a filter that strong must be asked for, never applied to a
// caller who did not ask. Three reasons, in descending order of weight:
//
//  1. This endpoint must not become the only door that quietly hides data.
//     The unfiltered list has to be reachable, and "reachable" is worth
//     more as the default than as an opt-out somebody has to know exists.
//  2. /v1/concept/{id} already serves the same concept's synonyms
//     UNFILTERED. Two endpoints answering "the synonyms of X" with
//     different row counts, one of them silently, is the kind of
//     discrepancy that reads as a bug in whichever one the client happened
//     to try second.
//  3. The publication rules rest on a nom_status vocabulary that is
//     populated on 6,85 % of names and has a 1.225-value unclassified tail
//     (domain.ClassifyNomStatus). Defaulting to a filter built on that
//     would put hostus' least-certain judgement on its busiest path.
//
// Both modes return the SAME per-synonym reasoning and the SAME exclusion
// summary; relevance decides only whether the withheld entries appear in
// the list.
const (
	// RelevanceAll returns every synonym, each still carrying its
	// publication judgement and reason.
	RelevanceAll = "all"
	// RelevancePublication returns only the synonyms UC5 considers
	// publishable, in UC5 order.
	RelevancePublication = "publication"
)

// PublicationRankSpecies is the only value the `rank` parameter accepts
// today: "I am publishing at species level", which UC5 answers with
// domain.RanksBelowSpecies() (VARIETY, SUBVARIETY, FORM, SUBFORM).
//
// A syntactically valid taxon rank that is NOT this one (say GENUS) is
// refused rather than accepted, because hostus has no UC5-sanctioned
// exclusion set for it: accepting it would return an unfiltered-by-rank
// list to a caller who believes they asked for a filter. A named refusal is
// the only honest answer until a use case defines the set.
const PublicationRankSpecies = "species"

// MaxSynonymLimit bounds the `max` parameter. It is set ABOVE the measured
// per-concept maximum (1.127 synonyms on one concept; 4,09 on average) so a
// caller can always express "all of them" through it, while an absurd value
// is refused before anything is allocated for it. 0 (and an absent
// parameter) mean "no truncation", which is not the same as 0 rows.
const MaxSynonymLimit = 2000

// Errors GET /v1/concept/{id}/synonyms distinguishes. All three map onto
// INVALID_QUERY and all three name the offending value, since a 400 that
// does not say WHICH value it rejected is barely better than a 500.
var (
	// ErrInvalidRelevance reports an unknown `relevance` value.
	ErrInvalidRelevance = errors.New("application: unknown relevance")
	// ErrInvalidPublicationRank reports a `rank` value that is not a
	// supported publication level.
	ErrInvalidPublicationRank = errors.New("application: unsupported publication rank")
	// ErrInvalidMax reports a `max` outside [0, MaxSynonymLimit].
	ErrInvalidMax = errors.New("application: max out of range")
)

// SynonymsRequest is one GET /v1/concept/{id}/synonyms call.
type SynonymsRequest struct {
	ConceptID string
	// Relevance is RelevanceAll or RelevancePublication; "" means
	// RelevanceAll (see the constants' doc comment for why that is the
	// default).
	Relevance string
	// PublicationRank is the level the caller publishes at, which decides
	// UC5's rank exclusions. "" means no rank exclusion at all — a caller
	// publishing a full infraspecific treatment.
	PublicationRank string
	// Max caps the RETURNED list. 0 means no truncation. Truncation always
	// happens AFTER ranking, so `max=3` returns the three best synonyms,
	// never three arbitrary ones.
	Max int
}

// SynonymsResult is the ranked, optionally filtered synonym list plus the
// audit trail that makes the filtering visible.
type SynonymsResult struct {
	ConceptID string
	// Relevance and PublicationRank are echoed back RESOLVED (never the
	// caller's raw string), so a response says which mode produced it.
	Relevance       string
	PublicationRank string
	// Synonyms is the answer: every candidate under RelevanceAll, only the
	// publishable ones under RelevancePublication, in domain.RankSynonyms
	// order, truncated to Max.
	Synonyms []domain.SynonymRelevance
	// Summary always covers ALL of the concept's synonyms, never just the
	// returned ones. That is the whole point: a caller who receives three
	// rows can see that 26 existed and read off which rule removed each of
	// the other 23.
	Summary domain.SynonymExclusionSummary
	// Truncated is how many entries Max removed from the list this mode
	// would otherwise have returned. It is reported separately from the
	// exclusion summary because a truncated synonym was not judged
	// irrelevant — it just did not fit.
	Truncated int
}

// Synonyms serves UC5: "the problem is filtering, not acquisition".
//
// It reads the concept's synonym candidates, runs the Task 2 relevance
// model over ALL of them, summarizes the outcome over ALL of them, and only
// then decides what to hand back. The order of those steps is the contract:
//
//  1. rank first — Max truncates the ranked list, never the input, so the
//     three synonyms a publication gets are the three best ones;
//  2. summarize over the full set — the summary counts what the concept
//     HAS, not what the response CARRIES, so a filter that dropped 20 of 26
//     says so;
//  3. filter last, and only when asked (see RelevanceAll).
//
// An unknown concept id surfaces domain.ErrNotFound (from the repository).
// A known concept with no synonyms is an empty list and a zeroed summary,
// never an error.
func Synonyms(ctx context.Context, repo output.Repository, req SynonymsRequest) (SynonymsResult, error) {
	relevance, err := parseRelevance(req.Relevance)
	if err != nil {
		return SynonymsResult{}, err
	}
	excludeRanks, publicationRank, err := parsePublicationRank(req.PublicationRank)
	if err != nil {
		return SynonymsResult{}, err
	}
	if req.Max < 0 || req.Max > MaxSynonymLimit {
		return SynonymsResult{}, fmt.Errorf("%w: %d is not in [0, %d]", ErrInvalidMax, req.Max, MaxSynonymLimit)
	}

	candidates, err := repo.SynonymCandidates(ctx, req.ConceptID)
	if err != nil {
		return SynonymsResult{}, err
	}

	ranked := domain.RankSynonyms(candidates, domain.SynonymOptions{ExcludeRanks: excludeRanks})
	res := SynonymsResult{
		ConceptID:       req.ConceptID,
		Relevance:       relevance,
		PublicationRank: publicationRank,
		Summary:         domain.SummarizeSynonyms(ranked),
	}

	list := ranked
	if relevance == RelevancePublication {
		list = publishable(ranked)
	}
	// `len(list) > req.Max` is a genuinely equivalent mutant at
	// CONDITIONALS_BOUNDARY (>=): at len(list) == req.Max the body would set
	// Truncated to 0 and reslice to the same length, i.e. do nothing. No
	// test can observe the difference (same rationale as the loop bound in
	// internal/adapters/sqlite/read.go's Classification).
	if req.Max > 0 && len(list) > req.Max {
		res.Truncated = len(list) - req.Max
		list = list[:req.Max]
	}
	res.Synonyms = list
	return res, nil
}

// publishable copies the publishable prefix of a ranked list. It relies on
// domain.RankSynonyms' primary sort key (publishable first) only for the
// ORDER of the result, not for finding the boundary — it filters every
// element explicitly, so a future change to that sort key degrades this
// into a slower correct answer rather than a silently truncated one.
func publishable(ranked []domain.SynonymRelevance) []domain.SynonymRelevance {
	out := make([]domain.SynonymRelevance, 0, len(ranked))
	for _, r := range ranked {
		if r.Publishable {
			out = append(out, r)
		}
	}
	return out
}

// parseRelevance resolves the `relevance` parameter. "" is RelevanceAll.
func parseRelevance(raw string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "":
		return RelevanceAll, nil
	case RelevanceAll:
		return RelevanceAll, nil
	case RelevancePublication:
		return RelevancePublication, nil
	default:
		return "", fmt.Errorf("%w %q (supported: %s, %s)", ErrInvalidRelevance, raw, RelevanceAll, RelevancePublication)
	}
}

// parsePublicationRank resolves the `rank` parameter into UC5's rank
// exclusion set, returning the resolved level alongside it. "" is no
// exclusion at all. See PublicationRankSpecies for why a valid-but-
// unsupported rank is refused instead of quietly ignored.
func parsePublicationRank(raw string) ([]domain.Rank, string, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "":
		return nil, "", nil
	case PublicationRankSpecies:
		return domain.RanksBelowSpecies(), PublicationRankSpecies, nil
	default:
		return nil, "", fmt.Errorf("%w %q (supported: %s)", ErrInvalidPublicationRank, raw, PublicationRankSpecies)
	}
}

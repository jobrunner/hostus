package application

import (
	"context"
	"strings"

	"github.com/jobrunner/hostus/internal/domain"
	"github.com/jobrunner/hostus/internal/ports/output"
)

// Confidence values assigned per match type, per spec §B.2.
const (
	confidenceExactAuthor    = 0.99
	confidenceAggregateAlias = 0.95
	confidenceExact          = 0.90
)

// Notes attached to results that need a human's attention.
const (
	noteAggregateResolved   = "Aggregat, keine Kleinartauflösung"
	noteAggregateUnresolved = "Aggregat ohne aufgelöstes Sammelart-Konzept"
	noteUnresolvable        = "Kein eindeutiger Treffer, keine Fuzzy-Auflösung in dieser SP"
	noteAmbiguous           = "Mehrdeutiger Treffer: mehrere Konzepte mit gleicher Übereinstimmungsstärke, manuelle Prüfung nötig"
)

// aggregateSuffixes are the trailing tokens (case-insensitive, after
// Fields-splitting) marking a verbatim name as an aggregate/collective
// species ("Sammelart") rather than a single microspecies, per §B.2.
var aggregateSuffixes = map[string]bool{
	"agg.":  true,
	"aggr.": true,
	"s.l.":  true,
}

// MatchRequest is one verbatim name to resolve, identified by a
// caller-supplied ID that is echoed back on the corresponding MatchResult.
type MatchRequest struct {
	ID       string
	Verbatim string
}

// MatchResult is the outcome of resolving one MatchRequest. A zero
// MatchType (with RequiresReview true and ConceptID empty) means the name
// could not be resolved (UNRESOLVABLE per §B.2) — this SP does not attempt
// fuzzy matching, so a near-miss is reported for review rather than guessed.
type MatchResult struct {
	ID             string
	MatchType      domain.MatchType
	Confidence     float64
	ConceptID      string
	Candidates     []string
	RequiresReview bool
	Note           string
}

// MatchNames resolves every req against repo, in order, per §B.2:
//
//  1. Split Verbatim into (canonical, author) via splitVerbatim.
//  2. If canonical ends in an aggregate marker (agg./aggr./s.l.), the
//     result is MatchAggregateAlias: resolved to whatever concept
//     repo.MatchExact finds for the (full, marker-included) canonical, or
//     UNRESOLVABLE if none does. No microspecies resolution is attempted.
//  3. Otherwise repo.MatchExact(Canonicalize(canonical)) is classified
//     candidate-by-candidate via domain.ClassifyMatch, preferring
//     exact_author over exact. Zero classified matches -> UNRESOLVABLE.
//
// Fuzzy matching is explicitly out of scope for this SP: a name that only
// near-matches a stored canonical (e.g. a misspelled epithet) is never
// silently resolved — it comes back UNRESOLVABLE for manual review.
func MatchNames(ctx context.Context, repo output.Repository, reqs []MatchRequest) ([]MatchResult, error) {
	results := make([]MatchResult, 0, len(reqs))
	for _, req := range reqs {
		res, err := matchOne(ctx, repo, req)
		if err != nil {
			return nil, err
		}
		results = append(results, res)
	}
	return results, nil
}

func matchOne(ctx context.Context, repo output.Repository, req MatchRequest) (MatchResult, error) {
	canonical, author := splitVerbatim(req.Verbatim)

	if isAggregate(canonical) {
		return matchAggregate(ctx, repo, req, canonical)
	}

	queryCanon := domain.Canonicalize(canonical)
	queryAuthor := domain.NormalizeAuthor(author)

	candidates, err := repo.MatchExact(ctx, queryCanon)
	if err != nil {
		return MatchResult{}, err
	}
	return classify(req, queryCanon, queryAuthor, candidates), nil
}

// matchAggregate resolves an aggregate/collective-species query: the
// aggregate's own canonical (marker included, e.g. "Festuca ovina agg.")
// is looked up verbatim; whichever concept repo.MatchExact finds for it is
// the answer, since there is no microspecies resolution in this SP.
func matchAggregate(ctx context.Context, repo output.Repository, req MatchRequest, canonical string) (MatchResult, error) {
	candidates, err := repo.MatchExact(ctx, domain.Canonicalize(canonical))
	if err != nil {
		return MatchResult{}, err
	}
	if len(candidates) == 0 {
		return MatchResult{
			ID:             req.ID,
			RequiresReview: true,
			Note:           noteAggregateUnresolved,
		}, nil
	}
	return MatchResult{
		ID:         req.ID,
		MatchType:  domain.MatchAggregateAlias,
		Confidence: confidenceAggregateAlias,
		ConceptID:  candidates[0].Concept.ID,
		Note:       noteAggregateResolved,
	}, nil
}

// classifiedHit is one candidate that classified as a match, carrying just
// enough to detect ambiguity (does the winning strength resolve to more
// than one distinct concept?) and to report it (the matched name).
type classifiedHit struct {
	conceptID string
	name      string
}

// classify runs domain.ClassifyMatch against every candidate, preferring
// exact_author matches over (weaker, author-less-query) exact matches: the
// winning strength is exact_author if any candidate classified that way,
// else exact, else neither (UNRESOLVABLE).
//
// Zero classified matches yields an UNRESOLVABLE result carrying the
// candidate names that were seen (if any), so a reviewer has something to
// look at.
//
// Two or more candidates classifying at the SAME winning strength but
// resolving to DIFFERENT concept IDs are a genuine ambiguity (e.g. a
// homonym: two distinct accepted concepts sharing one canonical+author) —
// silently picking the first would hide that from the caller. That result
// sets RequiresReview, lists every tied candidate's name in Candidates,
// and leaves ConceptID/MatchType empty rather than guessing. Multiple
// candidates at the winning strength that all resolve to the SAME concept
// (e.g. a synonym and its accepted name both classifying exact_author) are
// NOT ambiguous — they still resolve normally to that one concept.
func classify(req MatchRequest, queryCanon, queryAuthor string, candidates []output.MatchCandidate) MatchResult {
	var (
		names              []string
		exactAuthorMatches []classifiedHit
		exactMatches       []classifiedHit
	)
	for _, c := range candidates {
		names = append(names, c.MatchedName.Canonical)
		candCanon := domain.Canonicalize(c.MatchedName.Canonical)
		candAuthor := domain.NormalizeAuthor(c.MatchedName.Authorship)
		mt, ok := domain.ClassifyMatch(queryCanon, queryAuthor, candCanon, candAuthor)
		if !ok {
			continue
		}
		hit := classifiedHit{conceptID: c.Concept.ID, name: c.MatchedName.Canonical}
		switch mt {
		case domain.MatchExactAuthor:
			exactAuthorMatches = append(exactAuthorMatches, hit)
		case domain.MatchExact:
			exactMatches = append(exactMatches, hit)
		case domain.MatchAggregateAlias:
			// ClassifyMatch never produces this (it is assigned by
			// matchAggregate, a separate code path) — unreachable here.
		}
	}

	bestType := domain.MatchExactAuthor
	winners := exactAuthorMatches
	if len(winners) == 0 {
		bestType = domain.MatchExact
		winners = exactMatches
	}

	if len(winners) == 0 {
		return MatchResult{
			ID:             req.ID,
			RequiresReview: true,
			Note:           noteUnresolvable,
			Candidates:     names,
		}
	}

	distinctConcepts := make(map[string]bool, len(winners))
	for _, w := range winners {
		distinctConcepts[w.conceptID] = true
	}
	if len(distinctConcepts) > 1 {
		tiedNames := make([]string, 0, len(winners))
		for _, w := range winners {
			tiedNames = append(tiedNames, w.name)
		}
		return MatchResult{
			ID:             req.ID,
			RequiresReview: true,
			Note:           noteAmbiguous,
			Candidates:     tiedNames,
		}
	}

	conf := confidenceExact
	if bestType == domain.MatchExactAuthor {
		conf = confidenceExactAuthor
	}
	return MatchResult{
		ID:         req.ID,
		MatchType:  bestType,
		Confidence: conf,
		ConceptID:  winners[0].conceptID,
	}
}

// isAggregate reports whether canonical's last whitespace-separated token
// is an aggregate/collective-species marker (agg./aggr./s.l.), matched
// case-insensitively.
func isAggregate(canonical string) bool {
	fields := strings.Fields(canonical)
	if len(fields) == 0 {
		return false
	}
	return aggregateSuffixes[strings.ToLower(fields[len(fields)-1])]
}

// splitVerbatim splits a verbatim scientific-name string into its canonical
// name and its author citation. The first field (the genus) always starts
// the canonical part; subsequent fields keep joining the canonical for as
// long as they start with a lowercase letter (species/infraspecific
// epithets, rank markers like "subsp."/"var.", or an aggregate marker like
// "agg."/"s.l."). The first field that starts with '(' or an uppercase
// letter starts the author citation; everything from there to the end of
// the string is the (untouched) author.
//
// This is a purposefully small heuristic, not a full nomenclatural parser:
// it does not recognize hybrid markers, "sensu"/"auct." qualifiers, or
// author names that happen to start with a lowercase particle (e.g. "de
// Candolle" abbreviated oddly). Those are out of scope for this SP.
func splitVerbatim(verbatim string) (canonical, author string) {
	fields := strings.Fields(verbatim)
	if len(fields) == 0 {
		return "", ""
	}

	end := 1
	for end < len(fields) {
		r := []rune(fields[end])
		if r[0] == '(' || (r[0] >= 'A' && r[0] <= 'Z') {
			break
		}
		end++
	}
	return strings.Join(fields[:end], " "), strings.Join(fields[end:], " ")
}

package application

import (
	"context"
	"strings"

	"github.com/jobrunner/hostus/internal/domain"
	"github.com/jobrunner/hostus/internal/ports/output"
)

// Confidence values assigned per match type, per spec §B.2. A MatchFuzzy
// result has no fixed confidence tier here: its Confidence IS the winning
// domain.Similarity score (see matchFuzzy) — fuzzy confidence is inherently
// graded, not a fixed constant like the other match types.
const (
	confidenceExactAuthor    = 0.99
	confidenceAggregateAlias = 0.95
	confidenceExact          = 0.90
)

// fuzzyCandidateLimit bounds how many repo.MatchFuzzyCandidates rows
// matchFuzzy scores per query. The prefilter (same first letter + a bounded
// length window, see the sqlite adapter) already narrows candidates to a
// small set for any real near-miss; this cap just keeps the Similarity
// scoring cost bounded against a pathological backbone.
const fuzzyCandidateLimit = 20

// Notes attached to results that need a human's attention.
const (
	noteAggregateResolved   = "Aggregat, keine Kleinartauflösung"
	noteAggregateUnresolved = "Aggregat ohne aufgelöstes Sammelart-Konzept"
	noteUnresolvable        = "Kein eindeutiger Treffer, keine Fuzzy-Auflösung in dieser SP"
	noteAmbiguous           = "Mehrdeutiger Treffer: mehrere Konzepte mit gleicher Übereinstimmungsstärke, manuelle Prüfung nötig"
	noteFuzzy               = "Fuzzy-Treffer: Ähnlichkeit über Schwellenwert, manuelle Prüfung erforderlich"
	noteFuzzyAmbiguous      = "Mehrdeutiger Fuzzy-Treffer: mehrere Konzepte mit gleicher Ähnlichkeit, manuelle Prüfung nötig"
	// noteAggregatePrefix is prepended to whatever matchFuzzy's Note already
	// says (noteFuzzy or noteFuzzyAmbiguous) when a fuzzy hit resolves an
	// aggregate/collective-species query — see matchAggregate's fuzzy
	// fallback. It's the mechanism that keeps the result's aggregate nature
	// signaled even though MatchType becomes domain.MatchFuzzy rather than
	// domain.MatchAggregateAlias (MatchResult has no separate "is this an
	// aggregate" field; Note is the only carrier).
	noteAggregatePrefix = "Aggregat: "
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
//     repo.MatchExact finds for the (full, marker-included) canonical. No
//     microspecies resolution is attempted. If repo.MatchExact finds
//     nothing, matchAggregate itself falls through to a fuzzy attempt
//     (step 4's matchFuzzy) against that same marker-included canonical,
//     before giving up as UNRESOLVABLE — a typo'd aggregate name gets the
//     same fuzzy chance as a typo'd plain species name.
//  3. Otherwise repo.MatchExact(Canonicalize(canonical)) is classified
//     candidate-by-candidate via domain.ClassifyMatch, preferring
//     exact_author over exact.
//  4. If step 3 found nothing to classify (the plain-UNRESOLVABLE case —
//     NOT the ambiguous-tie case, which is already a resolved-but-uncertain
//     outcome), matchFuzzy tries a fuzzy resolution over
//     repo.MatchFuzzyCandidates, scored by domain.Similarity against
//     domain.FuzzyThreshold. A fuzzy hit ALWAYS sets RequiresReview (spec
//     §B.2 — this is not optional), whether it resolves to one concept or
//     is itself ambiguous across tied concepts. Nothing clearing the
//     threshold -> UNRESOLVABLE, unchanged from before fuzzy existed. Per
//     spec §B.2's own wording ("wenn exact/exact_author/aggregate nichts
//     liefert"), fuzzy is the catch-all for exact, exact_author, AND
//     aggregate all coming up empty — not just the first two.
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
	res, unresolved := classify(req, queryCanon, queryAuthor, candidates)
	if !unresolved {
		return res, nil
	}
	if len(candidates) > 0 {
		// The canonical DID match a real candidate exactly — it just failed
		// author verification (queryAuthor differs from every candidate's
		// author). That is a deliberate rejection, not "no match found":
		// falling through to fuzzy here would trivially "fuzzy-match" the
		// query back onto that very same canonical (similarity 1.0) and
		// silently launder the author check away. Fuzzy only ever applies
		// when MatchExact found nothing to classify in the first place.
		return res, nil
	}

	fuzzy, err := matchFuzzy(ctx, repo, req, queryCanon)
	if err != nil {
		return MatchResult{}, err
	}
	if fuzzy != nil {
		return *fuzzy, nil
	}
	return res, nil
}

// matchFuzzy attempts a fuzzy resolution of req once matchOne has ruled out
// every other outcome: classify's plain-UNRESOLVABLE case (not its
// ambiguous-tie case — a tie is not a matching failure), AND MatchExact
// returning zero candidates in the first place (not a canonical-matched
// author-mismatch — see matchOne's own guard, which never calls this
// function when repo.MatchExact found a canonical match at all). It scores
// every
// repo.MatchFuzzyCandidates(ctx, queryCanon, ...) result with
// domain.Similarity against queryCanon:
//
//   - no candidate reaches domain.FuzzyThreshold -> nil, so the caller
//     falls back to classify's original UNRESOLVABLE result.
//   - exactly one distinct concept ties for the best score -> a resolved
//     MatchType: domain.MatchFuzzy result. Confidence is that best score
//     (fuzzy confidence is graded, not a fixed tier); RequiresReview is
//     ALWAYS true per spec §B.2, with no exception for how close the score
//     is to 1.0; Candidates lists every name tied at the best score.
//   - two or more candidates at the best score resolve to DIFFERENT
//     concepts -> ambiguous: no ConceptID/MatchType, RequiresReview true,
//     Candidates lists the tied names — silently picking one would hide a
//     genuine ambiguity from the caller, same principle as classify's own
//     ambiguity handling.
func matchFuzzy(ctx context.Context, repo output.Repository, req MatchRequest, queryCanon string) (*MatchResult, error) {
	candidates, err := repo.MatchFuzzyCandidates(ctx, queryCanon, fuzzyCandidateLimit)
	if err != nil {
		return nil, err
	}

	best := 0.0
	scores := make([]float64, len(candidates))
	for i, c := range candidates {
		s := domain.Similarity(queryCanon, domain.Canonicalize(c.MatchedName.Canonical))
		scores[i] = s
		// s > best is a genuinely equivalent mutant at CONDITIONALS_BOUNDARY
		// (>=): when s == best exactly, reassigning best to it is a no-op
		// (same value) — the same max-selection-idiom equivalence already
		// documented on domain.Similarity's maxLen computation.
		if s > best {
			best = s
		}
	}
	// best < domain.FuzzyThreshold is NOT a boundary equivalent: a
	// similarity landing EXACTLY on FuzzyThreshold must still resolve (the
	// threshold is inclusive, per FuzzyThreshold's doc comment and the spec
	// examples it's tuned against) — see
	// TestMatchNames_FuzzyThresholdIsInclusiveAtExactBoundary, engineered to
	// land exactly on 0.85.
	if best < domain.FuzzyThreshold {
		return nil, nil
	}

	var winners []classifiedHit
	for i, c := range candidates {
		if scores[i] == best {
			winners = append(winners, classifiedHit{conceptID: c.Concept.ID, name: c.MatchedName.Canonical})
		}
	}

	distinctConcepts := make(map[string]bool, len(winners))
	for _, w := range winners {
		distinctConcepts[w.conceptID] = true
	}
	names := make([]string, 0, len(winners))
	for _, w := range winners {
		names = append(names, w.name)
	}

	if len(distinctConcepts) > 1 {
		return &MatchResult{
			ID:             req.ID,
			RequiresReview: true,
			Note:           noteFuzzyAmbiguous,
			Candidates:     names,
		}, nil
	}

	return &MatchResult{
		ID:             req.ID,
		MatchType:      domain.MatchFuzzy,
		Confidence:     best,
		ConceptID:      winners[0].conceptID,
		RequiresReview: true,
		Note:           noteFuzzy,
		Candidates:     names,
	}, nil
}

// matchAggregate resolves an aggregate/collective-species query: the
// aggregate's own canonical (marker included, e.g. "Festuca ovina agg.")
// is looked up verbatim; whichever concept repo.MatchExact finds for it is
// the answer, since there is no microspecies resolution in this SP.
//
// If repo.MatchExact finds nothing for that exact (marker-included)
// canonical, this falls through to matchFuzzy against the SAME canonical —
// per spec §B.2, fuzzy is the catch-all once exact/exact_author/aggregate
// all come up empty, and an aggregate name is no exception (a typo'd
// "Festuca ovinaa agg." deserves the same fuzzy chance as a typo'd plain
// species name). When that fuzzy attempt DOES resolve something,
// noteAggregatePrefix is prepended to whatever Note matchFuzzy produced
// (noteFuzzy for a clean resolution, noteFuzzyAmbiguous for a tied one) so
// the result still visibly conveys "this was an aggregate query" even
// though MatchType is domain.MatchFuzzy rather than
// domain.MatchAggregateAlias — RequiresReview is already unconditionally
// true from matchFuzzy either way. Only when matchFuzzy also finds nothing
// (nil) does this fall back to the plain noteAggregateUnresolved
// UNRESOLVABLE result, unchanged from before fuzzy existed.
//
// When repo.MatchExact DOES return candidates, this applies the same
// distinct-concept guard classify and matchFuzzy apply: several candidates
// resolving to the SAME concept (e.g. an aggregate's accepted name and a
// synonym of it) still resolve normally, but candidates resolving to two or
// more DISTINCT concepts are a genuine ambiguity — no ConceptID/MatchType
// is invented, RequiresReview is set and every candidate name is listed.
// Picking candidates[0] there would silently answer a question hostus
// cannot answer, which is precisely what the other two paths refuse to do.
func matchAggregate(ctx context.Context, repo output.Repository, req MatchRequest, canonical string) (MatchResult, error) {
	queryCanon := domain.Canonicalize(canonical)
	candidates, err := repo.MatchExact(ctx, queryCanon)
	if err != nil {
		return MatchResult{}, err
	}
	if len(candidates) == 0 {
		fuzzy, err := matchFuzzy(ctx, repo, req, queryCanon)
		if err != nil {
			return MatchResult{}, err
		}
		if fuzzy != nil {
			fuzzy.Note = noteAggregatePrefix + fuzzy.Note
			return *fuzzy, nil
		}
		return MatchResult{
			ID:             req.ID,
			RequiresReview: true,
			Note:           noteAggregateUnresolved,
		}, nil
	}
	distinctConcepts := make(map[string]bool, len(candidates))
	names := make([]string, 0, len(candidates))
	for _, c := range candidates {
		distinctConcepts[c.Concept.ID] = true
		names = append(names, c.MatchedName.Canonical)
	}
	if len(distinctConcepts) > 1 {
		return MatchResult{
			ID:             req.ID,
			RequiresReview: true,
			Note:           noteAmbiguous,
			Candidates:     names,
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
// The returned bool is true ONLY for the plain-UNRESOLVABLE case (zero
// candidates classified at all) — it is false both when a result resolved
// normally AND when it resolved to an ambiguous tie, since a tie is not a
// matching failure that fuzzy matching should get a second attempt at.
// matchOne uses this to decide whether to fall through to matchFuzzy.
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
func classify(req MatchRequest, queryCanon, queryAuthor string, candidates []output.MatchCandidate) (MatchResult, bool) {
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
		case domain.MatchAggregateAlias, domain.MatchFuzzy:
			// ClassifyMatch never produces either of these — they are
			// assigned by separate code paths (matchAggregate, matchFuzzy)
			// — unreachable here.
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
		}, true
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
		}, false
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
	}, false
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

package application

import (
	"context"
	"errors"
	"sort"
	"strings"

	"github.com/jobrunner/hostus/internal/domain"
	"github.com/jobrunner/hostus/internal/ports/output"
)

// ErrUnknownTargetSpace is returned by MatchInSpace when the requested target
// space is not an ingested name space. The HTTP adapter renders it as a 400
// INVALID_QUERY that names the offending space; it is never silently ignored,
// since a caller asking for a space hostus does not have is asking a question
// hostus cannot answer, not a question with an empty answer.
var ErrUnknownTargetSpace = errors.New("unknown target space")

// ErrUnknownBackbone and ErrUnknownSec are returned by MatchInSpace when an
// entry_backbone / entry_sec resolution filter names a backbone / sec.
// reference that is not ingested. The HTTP adapter renders each as a 400
// INVALID_QUERY naming the offending value.
var (
	ErrUnknownBackbone = errors.New("unknown entry_backbone")
	ErrUnknownSec      = errors.New("unknown entry_sec")
)

// MatchFilter narrows verbatim resolution to a single backbone and/or sec.
// reference space, so a name shared across the multi-backbone index (WCVP +
// CDM's ~119 sec spaces) resolves to one concept instead of an ambiguous tie.
// The zero value is no filter: resolution is then byte-for-byte the unfiltered
// match. Backbone and Sec compose with AND; Sec alone implies a sec-bearing
// concept (WCVP candidates, whose SecReference is empty, are then dropped).
type MatchFilter struct {
	Backbone string
	Sec      string
}

func (f MatchFilter) empty() bool { return f.Backbone == "" && f.Sec == "" }

// apply drops the candidates that do not match the filter. A zero filter
// returns the slice unchanged (the byte-identical unfiltered path).
func (f MatchFilter) apply(cands []output.MatchCandidate) []output.MatchCandidate {
	if f.empty() {
		return cands
	}
	kept := make([]output.MatchCandidate, 0, len(cands))
	for _, c := range cands {
		if f.Backbone != "" && c.Concept.BackboneID != f.Backbone {
			continue
		}
		if f.Sec != "" && c.Concept.SecReference != f.Sec {
			continue
		}
		kept = append(kept, c)
	}
	return kept
}

// validateFilter checks an entry_backbone/entry_sec filter against the ingested
// backbones and sec. references BEFORE any name is resolved, so an unknown
// value never does partial work. A zero filter is always valid.
// validateBackbone reports ErrUnknownBackbone unless backbone names an
// ingested backbone. An empty backbone is "no filter" and always valid.
// Shared by the match entry_backbone filter and Suggest's, so both reject the
// same values with the same error.
func validateBackbone(ctx context.Context, repo output.Repository, backbone string) error {
	if backbone == "" {
		return nil
	}
	bvs, err := repo.BackboneVersions(ctx)
	if err != nil {
		return err
	}
	for _, b := range bvs {
		if b.ID == backbone {
			return nil
		}
	}
	return ErrUnknownBackbone
}

// validateTargetSpace reports ErrUnknownTargetSpace unless space names an
// ingested name space. An empty space is "no space requested" and always
// valid. Shared by MatchInSpace and Suggest so both reject the same values
// with the same error.
func validateTargetSpace(ctx context.Context, repo output.Repository, space string) error {
	if space == "" {
		return nil
	}
	spaces, err := repo.NameSpaces(ctx)
	if err != nil {
		return err
	}
	for _, s := range spaces {
		if s.ID == space {
			return nil
		}
	}
	return ErrUnknownTargetSpace
}

func validateFilter(ctx context.Context, repo output.Repository, filter MatchFilter) error {
	if err := validateBackbone(ctx, repo, filter.Backbone); err != nil {
		return err
	}
	if filter.Sec != "" {
		_, err := repo.SecReferenceByID(ctx, filter.Sec)
		if errors.Is(err, domain.ErrNotFound) {
			return ErrUnknownSec
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// Confidence values assigned per match type, per spec §B.2. A MatchFuzzy
// result has no fixed confidence tier here: its Confidence IS the winning
// domain.Similarity score (see matchFuzzy) — fuzzy confidence is inherently
// graded, not a fixed constant like the other match types.
const (
	confidenceExactAuthor    = 0.99
	confidenceAggregateAlias = 0.95
	confidenceExact          = 0.90
	// Below the exact tier on purpose: the concept is certain, the ANSWER is
	// not what was asked. An aggregate query answered with the nominate taxon
	// is a knowingly coarser reply, and the number has to say so.
	//
	// It must nevertheless stay ABOVE domain.FuzzyThreshold (0.85). A fuzzy
	// result's confidence IS its similarity score, so it is always >= that
	// threshold AND always carries RequiresReview — an unreviewed guess. This
	// type is a certain concept. Ranking it below fuzzy would invert the
	// ordering the numbers exist to express: a caller auto-accepting at, say,
	// 0.8 would take the guesses and reject the certainties. The first draft
	// used 0.75 and did exactly that.
	confidenceAggregateNominate = 0.88
)

// fuzzyCandidateCap bounds how many repo.MatchFuzzyCandidates rows matchFuzzy
// scores per query — as a load guard only, never as a selection rule.
//
// The old value was 20, and the sentence that justified it ("the prefilter
// already narrows candidates to a small set") was measurably false: on a real
// index the prefilter's pool was ~41.000 rows, so 20 was not a safety margin
// but the actual selection, made in alphabetical order. Recall was 0.0 % in
// every error class (docs/research/fuzzy-prefilter.md).
//
// 0 hands the decision to the repository, which is where the measurement of
// what its own prefilter actually returns lives — and it means this constant
// cannot silently overrule a fixed prefilter from the outside, which is
// exactly what it did before. Scoring the resulting pool is cheap: the
// largest one measured was 8.081 rows, and 8.081 Levenshtein comparisons do
// not register next to the query that produced them.
const fuzzyCandidateCap = 0

// fuzzyReviewFloor is the similarity below which a candidate is not worth
// showing a reviewer at all. Between it and domain.FuzzyThreshold a name is
// not resolved but IS handed back for curation, instead of being scored and
// then thrown away (issue #67, class 3).
//
// Measured against that issue's own examples, which is what fixes the number:
// the genus-synonymy group — a segregate genus the backbone does not use —
// lands at 0.792 ("Astracantha diphtherites" -> "Astragalus diphtherites") and
// 0.750 ("... parnassi"), so a floor above 0.70 would drop exactly the class
// worth reviewing. Below it lies noise: 0.545 for "Arctostaphylos alpinus" ->
// "Arctous alpina" and 0.318 for "Bellidiastrum michelii" -> "Aster
// bellidiastrum". Those two need synonymy knowledge, not string distance —
// lowering the floor to reach them would bury the real near misses in
// unrelated names rather than find them.
const fuzzyReviewFloor = 0.70

// Notes attached to results that need a human's attention.
const (
	noteAggregateResolved   = "Aggregat, keine Kleinartauflösung"
	noteAggregateUnresolved = "Aggregat ohne aufgelöstes Sammelart-Konzept"
	noteAggregateNominate   = "Aggregat: keine Sammelart im Index, aufgelöst auf das Nominal-Konzept — deckt weniger ab als die Anfrage"
	noteUnresolvable        = "Kein eindeutiger Treffer, keine Fuzzy-Auflösung in dieser SP"
	noteNearMiss            = "Nicht aufgelöst: kein Treffer über der Ähnlichkeitsschwelle. Die gelisteten Kandidaten sind die nächstliegenden Namen im Index, zur manuellen Prüfung — KEINE Auflösung"
	// noteGenusMismatch replaces noteNearMiss when a candidate DID clear the
	// similarity threshold and was refused because its genus is a different
	// one. Saying "kein Treffer über der Ähnlichkeitsschwelle" there would be
	// untrue, and a note that contradicts the candidate list next to it
	// teaches the reader to ignore notes.
	noteGenusMismatch = "Nicht aufgelöst: der nächstliegende Name ist ähnlich genug, gehört aber zu einer anderen Gattung (häufig ein geteiltes Epitheton bei nicht verwandten Pflanzen) — zur manuellen Prüfung, KEINE Auflösung"
	// noteRankMarkerMismatch is the second refusal reason. Kept apart from
	// noteGenusMismatch because for these the genus is usually IDENTICAL
	// ("Taraxacum sect." vs "Taraxacum sectum"), so blaming the genus would be
	// factually wrong in front of the candidate it names.
	noteRankMarkerMismatch = "Nicht aufgelöst: der nächstliegende Name ist ähnlich genug, benennt aber einen anderen Rang — auf einer Seite steht ein Rangkürzel (z. B. sect., subsp.), auf der anderen nicht — zur manuellen Prüfung, KEINE Auflösung"
	noteAmbiguous          = "Mehrdeutiger Treffer: mehrere Konzepte mit gleicher Übereinstimmungsstärke, manuelle Prüfung nötig"
	noteFuzzy              = "Fuzzy-Treffer: Ähnlichkeit über Schwellenwert, manuelle Prüfung erforderlich"
	noteFuzzyAmbiguous     = "Mehrdeutiger Fuzzy-Treffer: mehrere Konzepte mit gleicher Ähnlichkeit, manuelle Prüfung nötig"
	// noteAggregatePrefix is prepended to whatever matchFuzzy's Note already
	// says (noteFuzzy or noteFuzzyAmbiguous) when a fuzzy hit resolves an
	// aggregate/collective-species query — see matchAggregate's fuzzy
	// fallback. It's the mechanism that keeps the result's aggregate nature
	// signaled even though MatchType becomes domain.MatchFuzzy rather than
	// domain.MatchAggregateAlias (MatchResult has no separate "is this an
	// aggregate" field; Note is the only carrier).
	noteAggregatePrefix = "Aggregat: "
)

// MatchRequest is one verbatim name to resolve, identified by a
// caller-supplied ID that is echoed back on the corresponding MatchResult.
type MatchRequest struct {
	ID       string
	Verbatim string
}

// MatchResult is the outcome of resolving one MatchRequest. A zero MatchType
// (with RequiresReview true and ConceptID empty) means the name could not be
// resolved (UNRESOLVABLE per §B.2).
//
// Such a result can still carry Candidates, and they mean different things
// depending on how it arose: names that matched the canonical but failed author
// verification, the several names of an ambiguous tie, or — below the
// similarity threshold — the nearest neighbors found for curation
// (nearMissNames). None of the three is a resolution.
type MatchResult struct {
	ID             string
	MatchType      domain.MatchType
	Confidence     float64
	ConceptID      string
	Candidates     []string
	RequiresReview bool
	Note           string

	// TargetSpaceName and AggregatePolicy are populated only by MatchInSpace
	// (UC4), never by MatchNames — both stay zero on the plain match path so
	// that path's result is byte-for-byte what it always was. TargetSpaceName
	// is the ESy-compatible spelling the target space uses for ConceptID;
	// AggregatePolicy is the tri-state from domain.ResolveTargetSpace (empty
	// for a plain species). See MatchInSpace.
	TargetSpaceName string
	AggregatePolicy domain.AggregatePolicy

	// Classification and AggregateResolution are populated by
	// matchNamesFiltered (Task 10), so BOTH MatchNames and MatchInSpace carry
	// them — unlike TargetSpaceName/AggregatePolicy above, these are not
	// UC4-only.
	//
	// Classification is always set once ConceptID resolves (Family/OrderName/
	// ClassName from Repository.Concept, possibly all-empty if the backbone
	// never carried them).
	//
	// AggregateResolution is nil unless the query is an aggregate/collective
	// name (domain.IsAggregateName) or the resolved concept itself carries a
	// collective rank (isCollectiveRank) — nil rather than a zero value so
	// its JSON absence is unambiguous.
	Classification      domain.Classification
	AggregateResolution *domain.AggregateResolution
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
	return matchNamesFiltered(ctx, repo, reqs, MatchFilter{})
}

// matchNamesFiltered is MatchNames with an optional resolution filter applied
// to every entry. A zero filter makes it byte-for-byte MatchNames.
func matchNamesFiltered(ctx context.Context, repo output.Repository, reqs []MatchRequest, filter MatchFilter) ([]MatchResult, error) {
	// Loaded only when the filter is empty: preferGenuineClaimants is never
	// invoked on the filtered path (every call site below is itself gated by
	// filter.empty()), so a caller pinning entry_backbone/entry_sec costs no
	// extra DB round trip for a set it will never consult. nativeSpaces stays
	// nil then, which preferGenuineClaimants/preferGenuineClaimantsPerSpelling
	// already tolerate (len(nil map) == 0) — but that code path is simply
	// never reached with a non-empty filter.
	var nativeSpaces map[string]bool
	if filter.empty() {
		var err error
		nativeSpaces, err = nativeSpaceSet(ctx, repo)
		if err != nil {
			return nil, err
		}
	}
	results := make([]MatchResult, 0, len(reqs))
	for _, req := range reqs {
		res, err := matchOne(ctx, repo, req, filter, nativeSpaces)
		if err != nil {
			return nil, err
		}
		if res.ConceptID != "" {
			concept, _, _, _, err := repo.Concept(ctx, res.ConceptID)
			if err != nil {
				return nil, err
			}
			res.Classification = domain.Classification{Family: concept.Family, OrderName: concept.OrderName, ClassName: concept.ClassName}
			if domain.IsAggregateName(req.Verbatim) || isCollectiveRank(concept.Rank) {
				canonical, _ := splitVerbatim(req.Verbatim)
				res.AggregateResolution, err = buildAggregateResolution(ctx, repo, canonical, res.ConceptID)
				if err != nil {
					return nil, err
				}
			}
		}
		results = append(results, res)
	}
	return results, nil
}

// nativeAggregateNameSpaces is the fixed, ordered list of Task 10's
// aggregate-resolution name spaces — deliberately NOT
// Repository.NameSpaces(ctx), which reads the ingested name_space table and
// never carries "wcvp" (a backbone, not a name space in the SP9 sense).
// wcvp is included anyway, always with AggregatePolicy "" (absent)/no
// concept id/0 members, since WCVP structurally carries no native aggregate
// concepts — see domain.AggregateResolution's doc comment.
var nativeAggregateNameSpaces = []string{"eurosl", "germansl", "wcvp"}

// collectiveRanks are the Fall-B collective/aggregate ranks (design spec
// §6): a concept resolved to one of these is itself a collective, even when
// the QUERY string carried no aggregate marker at all — e.g. a verbatim
// "Taraxacum sect. Palustria" resolving straight to a SECTION concept.
var collectiveRanks = map[domain.Rank]bool{
	domain.RankSpeciesAggregate: true,
	domain.RankGenusAggregate:   true,
	domain.RankSection:          true,
	domain.RankSubsection:       true,
	domain.RankSubgenus:         true,
}

// isCollectiveRank reports whether r is one of collectiveRanks.
func isCollectiveRank(r domain.Rank) bool {
	return collectiveRanks[r]
}

// aggregateResolutionRanks is collectiveRanks as the []domain.Rank slice
// Repository.AggregateConcepts takes.
//
// KNOWN ASYMMETRY (final-review Minor-11 finding): this list (5 ranks) is
// wider than concept_agreement.go's aggregateRanks (2 ranks:
// SPECIES_AGGREGATE/GENUS_AGGREGATE only). resolveAggregateOption below uses
// THIS list, so aggregate_resolution.options[]'s per-name-space known/
// unresolvable lookup works for all 5 collective ranks. But
// buildAggregateResolution's Agreement field is populated by
// Repository.ConceptAgreement, which only ever has rows for the 2 ranks
// ComputeConceptAgreement (concept_agreement.go) actually compares — so a
// SECTION/SUBSECTION/SUBGENUS match's aggregate_resolution.agreement stays
// permanently empty in the live /v1/match path, even though its
// options[].status resolves correctly. Widening ComputeConceptAgreement to
// all 5 ranks (so the two lists agree) is a separate, larger follow-up, not
// fixed in this round — see openapi.yaml's
// AggregateResolution.agreement description for the client-facing version
// of this note.
var aggregateResolutionRanks = []domain.Rank{
	domain.RankSpeciesAggregate, domain.RankGenusAggregate,
	domain.RankSection, domain.RankSubsection, domain.RankSubgenus,
}

// aggregateMatchKey is the ONE name-matching key both call sites in this
// file use to decide whether two aggregate/collective-rank spellings name
// the same taxon, regardless of aggregate-marker SPELLING (agg./aggr./
// s.l./...) — resolveAggregateOption applies it to a repo.AggregateConcepts
// summary's stored canonical, buildAggregateResolution applies it to the
// query's split verbatim, and a single shared function is what guarantees
// the two can never independently drift onto different orders (they used
// to: see git history / task-10-report.md's fix-round-1 entry).
//
// It canonicalizes BEFORE stripping the marker — the ORDER deliberately
// differs from Task 7's application.ComputeConceptAgreement
// (concept_agreement.go's indexAggregatesByMatchKey strips first, then
// canonicalizes). That order is harmless for BACKBONE-ingested canonical
// names, which by taxonomic convention always spell markers lowercase, but
// domain.StripAggregateMarkers itself does a case-SENSITIVE map lookup
// against lowercase-only marker spellings (its own doc comment: "peels the
// trailing aggregate markers off an ALREADY-CANONICALIZED name") — stripping
// BEFORE canonicalizing silently fails to strip a marker typed in
// non-lowercase case. Verified directly:
// StripAggregateMarkers("Festuca ovina AGGR.") returns the string
// UNCHANGED (marker still attached), while
// StripAggregateMarkers(Canonicalize("Festuca ovina AGGR.")) correctly
// yields "festuca ovina". The query side here processes RAW user-typed
// verbatim (via splitVerbatim), which carries no lowercase guarantee —
// canonicalizing first is what keeps a query like "Salsola kali AGG."
// resolving to the same key as a stored "Salsola kali agg.", consistent
// with domain.IsAggregateName/this package's own isAggregate, which already
// canonicalize before checking for a marker. See
// TestAggregateMatchKey_MixedCaseMarkerRegression.
func aggregateMatchKey(canonical string) string {
	return domain.StripAggregateMarkers(domain.Canonicalize(canonical))
}

// resolveAggregateOption is buildAggregateResolution's per-name-space
// lookup: ns's native aggregate concepts (Repository.AggregateConcepts),
// name-matched against prefix (an aggregateMatchKey value) by the same key
// application.ComputeConceptAgreement's own name-matching logic follows in
// intent (see aggregateMatchKey's doc comment for the order it deliberately
// does NOT share with concept_agreement.go). wcvp always gets the absent/
// zero option unconditionally — it structurally carries no native aggregate
// concepts, so there is nothing to look up.
func resolveAggregateOption(ctx context.Context, repo output.Repository, ns, prefix string) (domain.AggregateResolutionOption, error) {
	opt := domain.AggregateResolutionOption{NameSpace: ns}
	if ns == "wcvp" {
		return opt, nil
	}
	summaries, err := repo.AggregateConcepts(ctx, ns, aggregateResolutionRanks)
	if err != nil {
		return domain.AggregateResolutionOption{}, err
	}
	found := ""
	for _, s := range summaries {
		if aggregateMatchKey(s.Canonical) == prefix {
			found = s.ConceptID
			break
		}
	}
	if found == "" {
		opt.Status = domain.AggregatePolicyUnresolvable
		return opt, nil
	}
	opt.Status = domain.AggregatePolicyKnown
	opt.AggregateConceptID = found
	members, err := repo.AggregateMembers(ctx, found)
	if err != nil {
		return domain.AggregateResolutionOption{}, err
	}
	opt.MemberCount = len(members)
	return opt, nil
}

// buildAggregateResolution builds the per-match AggregateResolution for an
// aggregate/collective-rank result (Task 10): for eurosl and germansl, it
// looks up that space's native aggregate concepts (Repository.
// AggregateConcepts, the concept_aggregate-table mechanism — a DIFFERENT,
// newer mechanism than domain.ResolveTargetSpace/NameSpaceEntry.Aggregate's
// Fall-A alias, which MatchInSpace uses unchanged) and matches them against
// canonical by aggregateMatchKey — the SAME function resolveAggregateOption
// applies to each candidate's stored canonical, so the two sides can never
// drift onto different normalization orders. wcvp always gets the absent/
// zero option, since it structurally carries no native aggregate concepts.
//
// RequestedNameSpace/its own Status/MemberCount reflect the space of
// resolvedConceptID itself (its "<space>:concept:" id prefix); Agreement is
// populated only when BOTH eurosl and germansl resolved Known, via
// Repository.ConceptAgreement keyed on eurosl's aggregate concept id.
func buildAggregateResolution(ctx context.Context, repo output.Repository, canonical, resolvedConceptID string) (*domain.AggregateResolution, error) {
	prefix := aggregateMatchKey(canonical)

	options := make(map[string]domain.AggregateResolutionOption, len(nativeAggregateNameSpaces))
	for _, ns := range nativeAggregateNameSpaces {
		opt, err := resolveAggregateOption(ctx, repo, ns, prefix)
		if err != nil {
			return nil, err
		}
		options[ns] = opt
	}

	res := &domain.AggregateResolution{
		RequestedNameSpace: strings.SplitN(resolvedConceptID, ":", 2)[0],
	}
	res.Options = make([]domain.AggregateResolutionOption, len(nativeAggregateNameSpaces))
	for i, ns := range nativeAggregateNameSpaces {
		opt := options[ns]
		res.Options[i] = opt
		if ns == res.RequestedNameSpace {
			res.Status = opt.Status
			res.MemberCount = opt.MemberCount
		}
	}

	eurosl, germansl := options["eurosl"], options["germansl"]
	if eurosl.Status == domain.AggregatePolicyKnown && germansl.Status == domain.AggregatePolicyKnown {
		pair, err := repo.ConceptAgreement(ctx, eurosl.AggregateConceptID)
		if err != nil {
			return nil, err
		}
		if pair != nil {
			res.Agreement = pair.Agreement
		}
	}
	return res, nil
}

// MatchInSpace resolves reqs exactly as MatchNames and then, for the ingested
// target space `space`, annotates each result with the space's ESy-compatible
// spelling (MatchResult.TargetSpaceName) and its AggregatePolicy — the
// buildable half of UC4.
//
// space == "" is the opt-out: MatchInSpace then IS MatchNames, and no UC4 field
// is touched (UC3 and UC6 share this endpoint and must see the unchanged
// shape). A non-empty space that is not an ingested name space returns
// ErrUnknownTargetSpace — validated up front, before any matching, so an
// unknown space never does partial work.
//
// The policy is derived from the SAME aggregate predicate the matcher itself
// uses (isAggregate over the split canonical) and the concept's entries in the
// target space, via domain.ResolveTargetSpace — there is no second notion of
// "is this an aggregate" here. Results without a resolved ConceptID (an
// UNRESOLVABLE match) carry no name-space annotation, since there is no concept
// to look one up for.
func MatchInSpace(ctx context.Context, repo output.Repository, reqs []MatchRequest, space string, filter MatchFilter) ([]MatchResult, error) {
	if err := validateFilter(ctx, repo, filter); err != nil {
		return nil, err
	}
	if space == "" {
		return matchNamesFiltered(ctx, repo, reqs, filter)
	}
	if err := validateTargetSpace(ctx, repo, space); err != nil {
		return nil, err
	}

	results, err := matchNamesFiltered(ctx, repo, reqs, filter)
	if err != nil {
		return nil, err
	}
	for i := range results {
		if results[i].ConceptID == "" {
			continue
		}
		entries, err := repo.NameSpaceEntries(ctx, results[i].ConceptID, []string{space})
		if err != nil {
			return nil, err
		}
		canonical, _ := splitVerbatim(reqs[i].Verbatim)
		name, policy := domain.ResolveTargetSpace(isAggregate(canonical), entries)
		results[i].TargetSpaceName = name
		results[i].AggregatePolicy = policy
	}
	return results, nil
}

func matchOne(ctx context.Context, repo output.Repository, req MatchRequest, filter MatchFilter, nativeSpaces map[string]bool) (MatchResult, error) {
	canonical, author := splitVerbatim(req.Verbatim)

	if isAggregate(canonical) {
		return matchAggregate(ctx, repo, req, canonical, filter, nativeSpaces)
	}

	queryCanon := domain.Canonicalize(canonical)
	queryAuthor := domain.NormalizeAuthor(author)

	candidates, err := repo.MatchExact(ctx, queryCanon)
	if err != nil {
		return MatchResult{}, err
	}
	candidates = filter.apply(candidates)
	if filter.empty() {
		// Same two-tier claimant preference the ingest crosswalk applies
		// (preferGenuineClaimants, spec 2026-09-01 Entscheidung 1+2): a
		// sec.-space or Fall-B-native concept is not a second claimant. With
		// entry_backbone/entry_sec set, the caller has already pinned the
		// space — no preference runs then, so the filtered path stays
		// byte-identical to what it was before this fix.
		candidates = preferGenuineClaimants(candidates, nativeSpaces)
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

	fuzzy, err := matchFuzzy(ctx, repo, req, queryCanon, filter, nativeSpaces)
	if err != nil {
		return MatchResult{}, err
	}
	if fuzzy != nil {
		return *fuzzy, nil
	}
	return res, nil
}

// unresolvedNote picks the note for an unresolved result that still carries
// candidates. Which one applies is decided by the NEAREST candidate that
// similarity alone would have resolved — that is the name the note is talking
// about — and by why the guard refused it:
//
//   - a different genus (the shared-epithet case), or
//   - a rank abbreviation on only one side (a section is not a species), or
//   - nothing reached the threshold at all.
//
// It reads the verdicts the scoring loop already produced rather than
// re-deriving a reason, because a note that contradicts the candidate printed
// next to it teaches the reader to ignore notes — which is exactly what
// happened when this function guessed "genus" for every refusal.
func unresolvedNote(scores []float64, verdicts []domain.FuzzyVerdict) string {
	// The caller builds both slices in lockstep; the guard is here because the
	// alternative failure mode is an index-out-of-range panic in a handler.
	if len(scores) != len(verdicts) {
		return noteNearMiss
	}
	best, reason := 0.0, domain.FuzzyBelowThreshold
	for i, s := range scores {
		if s < domain.FuzzyThreshold || s <= best {
			continue
		}
		best, reason = s, verdicts[i]
	}
	switch reason {
	case domain.FuzzyGenusMismatch:
		return noteGenusMismatch
	case domain.FuzzyRankMarkerMismatch:
		return noteRankMarkerMismatch
	case domain.FuzzyBelowThreshold:
		// Nothing cleared the threshold, so no guard ever ran.
		return noteNearMiss
	case domain.FuzzyResolvable:
		// Unreachable: this function is only called on the unresolved path, so
		// a resolvable candidate would have been resolved. Listed rather than
		// folded into a default so a NEW verdict cannot slip through silently
		// — the exhaustive linter is the point.
		return noteNearMiss
	}
	return noteNearMiss
}

// nearMissNames returns the candidate names worth a reviewer's time — those at
// or above fuzzyReviewFloor — best score first, de-duplicated.
//
// Best-first because a curator reads the top of the list; de-duplicated
// because one canonical can reach the query through several concepts (the same
// name in WCVP and in a dozen CDM floras), and repeating it would push the
// genuinely different suggestions off the end.
func nearMissNames(candidates []output.MatchCandidate, scores []float64) []string {
	// The caller builds both slices in lockstep, so this cannot differ today.
	// It is a guard rather than a comment because the alternative failure mode
	// is an index-out-of-range panic inside a request handler.
	if len(scores) != len(candidates) {
		return nil
	}
	type scored struct {
		name  string
		score float64
	}
	best := make(map[string]float64, len(candidates))
	for i, c := range candidates {
		if scores[i] < fuzzyReviewFloor {
			continue
		}
		name := c.MatchedName.Canonical
		if prev, seen := best[name]; !seen || scores[i] > prev {
			best[name] = scores[i]
		}
	}
	out := make([]scored, 0, len(best))
	for name, score := range best {
		out = append(out, scored{name: name, score: score})
	}
	// Name as the tiebreaker keeps the order deterministic — map iteration is
	// not, and a list that reshuffles between identical requests is a poor
	// thing to hand a reviewer.
	sort.Slice(out, func(i, j int) bool {
		if out[i].score != out[j].score {
			return out[i].score > out[j].score
		}
		return out[i].name < out[j].name
	})
	names := make([]string, len(out))
	for i, s := range out {
		names[i] = s.name
	}
	return names
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
func matchFuzzy(ctx context.Context, repo output.Repository, req MatchRequest, queryCanon string, filter MatchFilter, nativeSpaces map[string]bool) (*MatchResult, error) {
	candidates, err := repo.MatchFuzzyCandidates(ctx, queryCanon, fuzzyCandidateCap, filter.Backbone, filter.Sec)
	if err != nil {
		return nil, err
	}
	// The repo already restricts to backbone/sec BEFORE its LIMIT (so the
	// wanted space's near-miss is not truncated away); apply is a defensive
	// no-op on a correct repo, and still filters a fake repo that ignores the
	// hints.
	candidates = filter.apply(candidates)
	if filter.empty() {
		// The PER-SPELLING variant, not the plain one matchOne/matchAggregate
		// use: this pool is heterogeneous (many DIFFERENT names within the
		// prefilter's prefix+length window), and applying preferGenuineClaimants
		// directly across it silently dropped every sec-/native-only candidate
		// pool-wide as soon as ANY unrelated backbone name shared the pool —
		// see preferGenuineClaimantsPerSpelling's doc comment for the measured
		// regression this fixes. Applied BEFORE scoring so near-miss lists and
		// winner ties see the narrowed candidate base.
		candidates = preferGenuineClaimantsPerSpelling(candidates, nativeSpaces)
	}

	best := 0.0
	scores := make([]float64, len(candidates))
	// resolves[i] is whether candidate i may be RESOLVED to, not merely
	// listed. It is a stricter question than scores[i] >= FuzzyThreshold:
	// domain.FuzzyResolves also requires the genus to agree, because the
	// epithet-keyed prefilter route admits candidates that share an epithet
	// with the query and nothing else. See domain.FuzzyResolves for the
	// measured 30,6 % of false positives that produces without it.
	resolves := make([]bool, len(candidates))
	// verdicts[i] is why candidate i was refused, so the note can name the
	// actual reason instead of guessing one — a note reading "different genus"
	// next to a candidate whose genus is identical is worse than no note.
	verdicts := make([]domain.FuzzyVerdict, len(candidates))
	for i, c := range candidates {
		cand := domain.Canonicalize(c.MatchedName.Canonical)
		s := domain.Similarity(queryCanon, cand)
		scores[i] = s
		// Below the threshold the verdict is settled, and asking
		// domain.FuzzyClassify would recompute this very Similarity — cheap at
		// the old candidate budget of 20, no longer so now that the whole
		// prefilter pool is scored.
		if s < domain.FuzzyThreshold {
			verdicts[i] = domain.FuzzyBelowThreshold
			continue
		}
		verdicts[i] = domain.FuzzyClassify(queryCanon, cand)
		resolves[i] = verdicts[i] == domain.FuzzyResolvable
		if !resolves[i] {
			continue
		}
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
		// Nothing resolves — but the candidates were just scored, and throwing
		// them away is what made a whole row disappear over a genus rename
		// (issue #67, class 3). Hand back the plausible ones for curation,
		// WITHOUT a ConceptID or MatchType: this is an unresolved result that
		// carries evidence, not a weaker kind of match.
		if near := nearMissNames(candidates, scores); len(near) > 0 {
			return &MatchResult{
				ID:             req.ID,
				RequiresReview: true,
				Note:           unresolvedNote(scores, verdicts),
				Candidates:     near,
			}, nil
		}
		return nil, nil
	}

	var winners []classifiedHit
	for i, c := range candidates {
		// resolves[i] as well as the score: a guard-rejected candidate can
		// tie the winning score (an unrelated genus sharing an epithet scores
		// whatever it scores), and letting it in here would hand back the
		// very concept the guard just refused — or turn a clean hit into a
		// false ambiguity.
		if resolves[i] && scores[i] == best {
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
// UNRESOLVABLE result.
//
// matchFuzzy can now also return an UNRESOLVED near-miss result (candidates
// for curation, no ConceptID — see nearMissNames), and this path prefixes
// that note too, giving "Aggregat: <near-miss note>" with candidates where it
// previously said noteAggregateUnresolved with none.
//
// In practice that state is unreachable for the ordinary case, and the reason
// is worth stating so nobody expects otherwise: matchFuzzy is called with the
// MARKER-INCLUDED canonical, and the repository's prefilter admits candidates
// only within fuzzyCandidateLengthWindow (3) runes of it. " aggr." adds six.
// So a bare near neighbor — the realistic shape, since backbones carry
// unmarked names — is always outside the window. Only a neighbor that carries
// a marker of its own can land there. Pinned by
// TestMatchNames_AggregateNearMissStaysUnresolvedInPractice.
//
// When repo.MatchExact DOES return candidates, this applies the same
// distinct-concept guard classify and matchFuzzy apply: several candidates
// resolving to the SAME concept (e.g. an aggregate's accepted name and a
// synonym of it) still resolve normally, but candidates resolving to two or
// more DISTINCT concepts are a genuine ambiguity — no ConceptID/MatchType
// is invented, RequiresReview is set and every candidate name is listed.
// Picking candidates[0] there would silently answer a question hostus
// cannot answer, which is precisely what the other two paths refuse to do.
func matchAggregate(ctx context.Context, repo output.Repository, req MatchRequest, canonical string, filter MatchFilter, nativeSpaces map[string]bool) (MatchResult, error) {
	queryCanon := domain.Canonicalize(canonical)
	candidates, err := repo.MatchExact(ctx, queryCanon)
	if err != nil {
		return MatchResult{}, err
	}
	candidates = filter.apply(candidates)
	if filter.empty() {
		// Same claimant preference as matchOne — see that call site's comment.
		candidates = preferGenuineClaimants(candidates, nativeSpaces)
	}
	if len(candidates) == 0 {
		// No aggregate taxon for the marked name. Before giving up, try the
		// name WITHOUT the marker: a data set writing "X aggr." against a
		// backbone that carries only X used to lose the whole row (issue #67,
		// 96 names), even though X itself resolves exactly. The nominate
		// concept plus a type saying it is coarser beats no answer at all.
		nominate, err := matchAggregateNominate(ctx, repo, req, queryCanon, filter, nativeSpaces)
		if err != nil {
			return MatchResult{}, err
		}
		if nominate != nil {
			return *nominate, nil
		}
		fuzzy, err := matchFuzzy(ctx, repo, req, queryCanon, filter, nativeSpaces)
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

// matchAggregateNominate retries an aggregate query against SHORTER spellings
// of the same name, returning nil when none yields a resolved concept so the
// caller can carry on to fuzzy and finally to UNRESOLVABLE.
//
// domain.AggregateBases peels the markers one LAYER at a time, and the layers
// are tried in that order because a name can carry several ("X aggr. s. l.")
// and a backbone may well hold the aggregate under the shorter marker. So an
// intermediate base can still BE an aggregate name ("X aggr."), and only the
// last one is the bare nominate — which is why the result type is decided per
// base rather than fixed up front:
//
//   - base still aggregate-marked and found -> MatchAggregateAlias. The index
//     really does carry the aggregate; nothing was narrowed, and claiming
//     otherwise would be a lie about the index's contents.
//   - bare nominate found -> MatchAggregateNominate, the narrowing this
//     function exists to make visible.
//
// Resolution goes through classify, not a hand-rolled "exactly one candidate"
// test, so it inherits the same tie-break as a plain query (accepted name beats
// homotypic synonym) instead of growing a second, subtly different notion of
// ambiguity. classify signals "no candidates at all" through its second return
// value but reports an ambiguous TIE as a normal result with an empty
// ConceptID — so a resolved concept is the condition to check. Stamping a
// match type onto an ambiguous tie would produce the worst possible answer: a
// confident-looking type and confidence with no concept behind them.
func matchAggregateNominate(ctx context.Context, repo output.Repository, req MatchRequest, queryCanon string, filter MatchFilter, nativeSpaces map[string]bool) (*MatchResult, error) {
	for _, base := range domain.AggregateBases(queryCanon) {
		if base == "" {
			continue
		}
		candidates, err := repo.MatchExact(ctx, base)
		if err != nil {
			return nil, err
		}
		candidates = filter.apply(candidates)
		if filter.empty() {
			// Same claimant preference as matchOne — see that call site's
			// comment.
			candidates = preferGenuineClaimants(candidates, nativeSpaces)
		}
		if len(candidates) == 0 {
			continue
		}
		res, noCandidates := classify(req, base, "", candidates)
		if noCandidates || res.ConceptID == "" {
			continue
		}
		if domain.IsAggregateName(base) {
			res.MatchType = domain.MatchAggregateAlias
			res.Confidence = confidenceAggregateAlias
			res.Note = noteAggregateResolved
			return &res, nil
		}
		res.MatchType = domain.MatchAggregateNominate
		res.Confidence = confidenceAggregateNominate
		res.Note = noteAggregateNominate
		return &res, nil
	}
	return nil, nil
}

// classifiedHit is one candidate that classified as a match, carrying just
// enough to detect ambiguity (does the winning strength resolve to more
// than one distinct concept?) and to report it (the matched name).
type classifiedHit struct {
	conceptID string
	name      string
	role      string // accepted|synonym (from the exact-match candidate)
	homotypic *bool  // for a synonym match: homotypic link (nil = unknown)
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
		hit := classifiedHit{conceptID: c.Concept.ID, name: c.MatchedName.Canonical, role: c.Role, homotypic: c.Homotypic}
		switch mt {
		case domain.MatchExactAuthor:
			exactAuthorMatches = append(exactAuthorMatches, hit)
		case domain.MatchExact:
			exactMatches = append(exactMatches, hit)
		case domain.MatchAggregateAlias, domain.MatchAggregateNominate, domain.MatchFuzzy:
			// ClassifyMatch never produces any of these — they are assigned
			// by separate code paths (matchAggregate,
			// matchAggregateNominate, matchFuzzy) — unreachable here.
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
		// Tie-break before reporting an ambiguous tie: prefer the single concept
		// (if any) for which the queried name is the genuine name-bearer — see
		// genuineBearerWinner.
		if cid, ok := genuineBearerWinner(winners); ok {
			conf := confidenceExact
			if bestType == domain.MatchExactAuthor {
				conf = confidenceExactAuthor
			}
			return MatchResult{
				ID:         req.ID,
				MatchType:  bestType,
				Confidence: conf,
				ConceptID:  cid,
			}, false
		}
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

// roleAccepted is the concept_name.role value for a concept's accepted name.
const roleAccepted = "accepted"

// genuineBearerWinner breaks a match tie by nomenclatural type, or reports
// that the tie stands. Called from two places: classify below (this file,
// squarely on the serving path — every /v1/match exact-tie goes through it)
// and crosswalk.go's resolveTraitName under policyResolveGenuineBearer (no
// current caller passes that policy — see its own doc comment).
//
// The two claims a candidate can make are TIERED, not interchangeable:
//
//  1. the queried name is that concept's ACCEPTED name — the strongest claim,
//     since that concept is what the name denotes today;
//  2. it is a HOMOTYPIC synonym there — same nomenclatural type (a
//     recombination/basionym), but the concept has moved the name aside.
//
// A tier decides as soon as it holds ANY candidate: exactly one concept -> that
// concept, several -> the tie stands. A weaker tier never rescues an ambiguous
// stronger one, because answering two competing accepted names with some third
// concept's synonym would be choosing rather than resolving.
//
// So "Inula hirta" resolves to Pentanema hirtum via tier 2 (homotypic there,
// heterotypic under P. britannica, accepted in neither), while several CDM
// floras all accepting "Inula hirta" stay ambiguous at tier 1.
//
// Scope worth knowing: the candidates are whatever the caller's filter left in
// play — narrowed, when the filter is empty, by preferGenuineClaimants
// BEFORE this function ever sees them (spec 2026-09-01 Entscheidung 1+2), so
// a sec.-space or Fall-B-native concept sharing the name with a genuine
// backbone concept has already been dropped and never reaches this tie-break
// at all. What remains here and legitimately ties is two GENUINE claimants:
// e.g. two distinct backbones (or two sec. spaces with no backbone
// candidate) each holding the name as accepted — measured on the real index
// pre-fix, that was 983 of the 10260 names tier 1 decides within WCVP alone.
func genuineBearerWinner(winners []classifiedHit) (string, bool) {
	// The two claims are TIERED, not equivalent. Treating them as one set was
	// measured wrong (issue #67): "Beckmannia eruciformis" is the accepted name
	// of wcvp:concept:399185 and a homotypic synonym under 424915, which made
	// two bearers and left a decidable case unresolvable.
	//
	// Being a concept's ACCEPTED name is the stronger claim: that concept is
	// what the name denotes today. A homotypic synonym shares the type but the
	// concept has moved the name aside, so it only decides when no candidate
	// holds the name as accepted — which is the Inula hirta case the tier below
	// exists for (homotypic under Pentanema hirtum, heterotypic under
	// P. britannica, accepted in neither).
	tiers := []func(classifiedHit) bool{
		func(w classifiedHit) bool { return w.role == roleAccepted },
		func(w classifiedHit) bool { return w.homotypic != nil && *w.homotypic },
	}
	for _, qualifies := range tiers {
		id, present := soleConcept(winners, qualifies)
		if !present {
			continue // nothing at this tier — the next one may decide
		}
		// Present but not sole: the strongest claim in play is ambiguous, and a
		// weaker tier must NOT rescue it. Two floras both accepting the name is
		// a real ambiguity; answering with some third concept's homotypic
		// synonym would be picking, not resolving.
		return id, id != ""
	}
	return "", false
}

// soleConcept reports whether any winner qualifies (present) and, if exactly
// one CONCEPT does, which. present with an empty id means "several qualified"
// — the caller must treat that as ambiguous rather than looking further.
func soleConcept(winners []classifiedHit, qualifies func(classifiedHit) bool) (id string, present bool) {
	for _, w := range winners {
		if !qualifies(w) {
			continue
		}
		if present && id != w.conceptID {
			return "", true
		}
		id, present = w.conceptID, true
	}
	return id, present
}

// isAggregate reports whether canonical names an aggregate/collective species.
//
// It delegates to domain.IsAggregateName rather than testing the last token
// against its own marker list, which is what it used to do. That local list
// knew only single-token markers, so a LAYERED name ("X aggr. s. l." — 60 EIVE
// taxa are spelled that way) was not recognized as an aggregate at all and
// never reached the aggregate path. Two independently maintained notions of
// "is this an aggregate" is also exactly the duplicated-mapper defect this
// codebase has been bitten by before; the marker set belongs in one place.
func isAggregate(canonical string) bool {
	return domain.IsAggregateName(canonical)
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

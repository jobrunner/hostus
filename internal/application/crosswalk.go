package application

import (
	"context"
	"sort"

	"github.com/jobrunner/hostus/internal/domain"
	"github.com/jobrunner/hostus/internal/ports/output"
)

// unmatchedSampleCap bounds every crosswalk report's *Sample fields (e.g.
// XrefIngestReport.UnmatchedSample, NameSpaceIngestReport.UnmatchedSample) so
// a huge source never balloons the report — the point is to show a reviewer
// WHICH names/taxa were lost, not to enumerate every one.
const unmatchedSampleCap = 20

// RuleCount is one normalisation rule's contribution to a crosswalk: how
// many rows and how many distinct TAXON names it resolved that the plain
// exact key would have lost. Flagged mirrors
// domain.NormalizationRule.Flagged so a reader of the report does not have
// to know which rules are judgement calls.
type RuleCount struct {
	Rule    domain.NormalizationRule
	Rows    int
	Taxa    int
	Flagged bool
}

// traitResolution is one canonical name's crosswalk outcome, decided in a
// crosswalk's resolve phase and consumed in its write phase. Despite the
// name (a holdover from the trait-vocabulary crosswalk this type was
// introduced for, since removed — see CHANGELOG "Traits-Subsystem
// entfernt"), it is the shared per-name outcome every crosswalk that walks
// domain.NameCandidates uses, including the name-space crosswalk
// (resolveNameSpaceNames in namespace_ingest.go).
type traitResolution struct {
	conceptID string
	matched   bool
	ambiguous bool
	// tieBroken records that this outcome came from genuineBearerWinner
	// rather than from a single-candidate key, so a caller can report it.
	tieBroken bool
	// rule is the normalisation rule whose key produced this outcome (both
	// for matched and for ambiguous). domain.RuleExact means the plain
	// Canonicalize key answered — the pre-normalisation behavior.
	rule domain.NormalizationRule
}

// crosswalkPolicy selects how resolveTraitName decides when a candidate key
// answers with several distinct concepts. It is a parameter rather than one
// behavior baked in because resolveTraitName has multiple callers with
// different evidence behind them, and one of them inherited a change meant
// for another without anyone noticing.
type crosswalkPolicy int

const (
	// policyPreferBackbone drops sec.-space-only candidates whenever a
	// backbone concept holds the same name (preferBackboneConcepts), but
	// still refuses to guess among whatever remains — it does NOT run
	// genuineBearerWinner. Measured on a full real ingest (2026-08-31,
	// rows/matched/ambiguous — see resolveNameSpaceNames' call site history
	// for the exact before/after-fix figures) when name-space ingest still
	// used this policy: a sec.-space reference concept (e.g. one of CDM's
	// ~18 Standardliste sec. spaces) is an attribution detail of a SEPARATE
	// concept source, not a second genuine claimant on a eurosl/germansl
	// spelling, so it must not count toward "this name is ambiguous" once a
	// real backbone (WCVP) concept already carries it — this filter
	// recovered essentially the same match rate as dropping CDM's
	// concept_sources entry entirely (eurosl 113733 vs 113607 matched,
	// germansl 10436 vs 10393) while keeping CDM — and /v1/translate — in
	// the database. Name-space ingest has since moved to
	// policyResolveAcceptedBearer (spec 2026-09-04), which applies the same
	// preferGenuineClaimants filter AND breaks the accepted-bearer tie —
	// so this policy currently has no production caller, exactly like
	// policyResolveGenuineBearer below, and stays available as the
	// filter-only rollback target (and for any future crosswalk that wants
	// the narrowing without a tie-break). Unlike policyResolveGenuineBearer/
	// policyResolveAcceptedBearer, this stays SILENT (no tieBroken, no
	// report counter) precisely because it resolves nothing on its own.
	policyPreferBackbone crosswalkPolicy = iota
	// policyResolveGenuineBearer prefers backbone concepts over sec.-space
	// ones AND resolves a remaining homonym to the concept that genuinely
	// bears the name (genuineBearerWinner). Introduced for the
	// trait-vocabulary crosswalk (see docs/research/reality-check.md and the
	// CHANGELOG entry for the measured recovery), which has since been
	// removed and transferred to situs — no current caller passes this
	// value, but the policy stays available for any future crosswalk that
	// needs the same tie-break.
	policyResolveGenuineBearer
	// policyResolveAcceptedBearer prefers backbone concepts over sec.-space
	// ones (the same preferGenuineClaimants filter as policyResolveGenuineBearer)
	// and resolves a remaining homonym with acceptedBearerWinner — tier 1
	// of genuineBearerWinner ALONE, deliberately without tier 2 (homotypic
	// synonym bearer). Used by the name-space crosswalk (resolveNameSpaceNames
	// in namespace_ingest.go): tier 1 is the nomenclaturally grounded case —
	// a later homonym is illegitimate BECAUSE it duplicates a legitimate
	// name's spelling — measured 2026-09-01 on the real eurosl index:
	// Illegitimate rows collide with a foreign accepted canonical at 17.3%
	// vs 0.16% for ordinary synonyms (4716 folds this tier decides). Tier 2
	// is not yet measured for name spaces and stays serving-path-only (spec
	// 2026-09-04, decision 1) — do not add it here without a measurement of
	// its own, the same way tier 1 got one.
	policyResolveAcceptedBearer
)

// resolveTraitName walks domain.NameCandidates' deterministic ladder for
// the already canonicalized name canon and returns the FIRST candidate key
// the index answers at all, classified into exactly one of: matched to a
// single concept id, matched=false (no key answered), or ambiguous=true.
//
// What counts as ambiguous depends on policy. The base rule, under all three
// policies this package defines: a key answering with two or more distinct
// concepts is ambiguous and no concept is picked. Candidates are always
// narrowed first by preferGenuineClaimants — tier 1 (preferBackboneConcepts)
// drops sec.-space-only candidates whenever a backbone concept shares the
// name, and tier 2 then drops name-space-NATIVE candidates (nativeSpaces)
// whenever a genuine backbone concept remains (spec 2026-09-01 B2,
// preferGenuineClaimants' doc comment). The policies differ only in what
// happens to a tie that filter leaves behind:
//
//   - policyPreferBackbone never breaks it — whatever remains still follows
//     the base rule, so this outcome never carries tieBroken.
//   - policyResolveGenuineBearer breaks a remaining tie with
//     genuineBearerWinner's full two tiers (accepted bearer, then homotypic
//     synonym bearer) — only a key with no single genuine bearer stays
//     ambiguous, and the outcome then carries tieBroken.
//   - policyResolveAcceptedBearer breaks a remaining tie with
//     acceptedBearerWinner — genuineBearerWinner's tier 1 ALONE, no tier 2
//     (spec 2026-09-04, decision 1) — so a key with no single ACCEPTED
//     bearer stays ambiguous even when a single homotypic bearer exists.
//
// (resolveHomonymTie is where this three-way dispatch actually lives.)
//
// Two properties matter and are worth stating explicitly:
//
//   - The ladder's first key is always domain.Canonicalize(canon) itself,
//     so a name that resolved (or was ambiguous) before name normalisation
//     existed resolves identically now. Normalisation can only ever act on
//     names the exact key left with ZERO candidates — it cannot re-route an
//     existing hit onto a different concept.
//   - An AMBIGUOUS key stops the walk rather than letting a later,
//     semantically looser rule "rescue" it. A key that answers with several
//     concepts and no single bearer is a genuine ambiguity about which taxon the source
//     meant; continuing down the ladder until some rule happens to produce
//     a single-concept key would be guessing, which is precisely what the
//     rest of this crosswalk refuses to do.
func resolveTraitName(ctx context.Context, repo output.Repository, canon string, policy crosswalkPolicy, nativeSpaces map[string]bool) (traitResolution, error) {
	for _, cand := range domain.NameCandidates(canon) {
		candidates, err := repo.MatchExact(ctx, cand.Key)
		if err != nil {
			return traitResolution{}, err
		}
		// Every policy this package defines narrows the claimant set the same
		// way; the policies differ only in the tie-break that runs afterward
		// (resolveHomonymTie) — so this call is unconditional, not gated on
		// policy (a policy-gated condition covering all three values would be
		// tautological and mutation-blind: no test could ever observe it
		// change).
		candidates = preferGenuineClaimants(candidates, nativeSpaces)
		if len(candidates) == 0 {
			continue
		}
		distinct := make(map[string]bool, len(candidates))
		for _, c := range candidates {
			distinct[c.Concept.ID] = true
		}
		if len(distinct) > 1 {
			// Several concepts hold this spelling — but "several concepts" is
			// not the same as "undecidable". resolveHomonymTie applies
			// whichever tiered tie-break policy selects, or reports the tie
			// stands.
			if id, ok := resolveHomonymTie(candidates, policy); ok {
				return traitResolution{conceptID: id, matched: true, tieBroken: true, rule: cand.Rule}, nil
			}
			return traitResolution{ambiguous: true, rule: cand.Rule}, nil
		}
		return traitResolution{conceptID: candidates[0].Concept.ID, matched: true, rule: cand.Rule}, nil
	}
	return traitResolution{}, nil
}

// resolveHomonymTie applies policy's tie-break to candidates, which several
// distinct concepts hold: policyResolveGenuineBearer runs genuineBearerWinner
// (accepted bearer first, homotypic bearer second — the serving path's
// full two-tier rule, issue #67 class 2); policyResolveAcceptedBearer runs
// acceptedBearerWinner (tier 1 alone, no tier 2 — spec 2026-09-04, decision
// 1); any other policy (policyPreferBackbone) never breaks a tie. ok is
// false — the tie stands — whenever the selected tie-break decides nothing.
func resolveHomonymTie(candidates []output.MatchCandidate, policy crosswalkPolicy) (string, bool) {
	switch policy {
	case policyResolveGenuineBearer:
		return genuineBearerWinner(traitBearers(candidates))
	case policyResolveAcceptedBearer:
		return acceptedBearerWinner(traitBearers(candidates))
	// policyPreferBackbone is spelled out explicitly, not left to fall into
	// default, so a future fourth policy cannot land here silently — adding
	// one without a matching case here should be a visible gap (exhaustive
	// lint won't catch a plain default), not an accidental "never breaks a
	// tie" behavior nobody decided.
	case policyPreferBackbone:
		return "", false
	default:
		return "", false
	}
}

// preferBackboneConcepts drops candidates living inside a sec. reference space
// whenever a backbone candidate exists for the same name.
//
// A value written under policyResolveGenuineBearer is only reachable from
// the concept id it was written to, so attributing it to a sec. concept
// would make it invisible to every consumer holding a backbone id. The
// fallback is deliberate rather than a blanket filter: a name that ONLY a
// concept source carries keeps resolving exactly as before (single candidate
// -> matched, several -> ambiguous).
func preferBackboneConcepts(candidates []output.MatchCandidate) []output.MatchCandidate {
	backbone := make([]output.MatchCandidate, 0, len(candidates))
	for _, c := range candidates {
		if c.Concept.SecReference == "" {
			backbone = append(backbone, c)
		}
	}
	if len(backbone) == 0 {
		return candidates
	}
	return backbone
}

// preferGenuineClaimants is the shared two-tier claimant preference both the
// ingest crosswalk (resolveTraitName) and the serving path (match.go) apply
// before counting distinct concepts:
//
//	tier 1: preferBackboneConcepts — sec.-space candidates are dropped when a
//	        non-sec candidate holds the name (PR #94, see that function).
//	tier 2: name-space-NATIVE candidates (Fall B concepts eurosl/germansl
//	        write for ranks above SPECIES, nativespace_ingest.go) are dropped
//	        when a genuine taxonomic-backbone concept remains. A native
//	        concept carries SecReference == "" and is therefore invisible to
//	        tier 1 — measured on a full real ingest (2026-09-01, spec B2):
//	        2866 GENUS + 319 FAMILY folds held both a WCVP and a native
//	        concept, costing germansl ~544 genus entries purely by ingest
//	        order. nativeSpaces is the set of name_space ids (Repository.
//	        NameSpaces) — floraveg is in it and harmless (it writes no
//	        native concepts, so no candidate ever carries its id).
//
// Both tiers share the same fallback: a name that ONLY the filtered class
// carries keeps its candidates (single native -> matched, several -> the
// base ambiguity rule), so bryophyte genera existing solely as native
// concepts (Abietinella) keep resolving.
func preferGenuineClaimants(candidates []output.MatchCandidate, nativeSpaces map[string]bool) []output.MatchCandidate {
	candidates = preferBackboneConcepts(candidates)
	if len(nativeSpaces) == 0 {
		return candidates
	}
	genuine := make([]output.MatchCandidate, 0, len(candidates))
	for _, c := range candidates {
		if !nativeSpaces[c.Concept.BackboneID] {
			genuine = append(genuine, c)
		}
	}
	if len(genuine) == 0 {
		return candidates
	}
	return genuine
}

// preferGenuineClaimantsPerSpelling applies preferGenuineClaimants WITHIN
// each spelling group of a heterogeneous candidate pool (grouped by
// domain.Canonicalize(MatchedName.Canonical)), preserving the pool's
// original order.
//
// preferGenuineClaimants itself is defined for candidates sharing ONE
// spelling — "two claimants on the same name". Applying it directly across a
// fuzzy prefilter pool of DIFFERENT names (match.go's matchFuzzy) silently
// dropped every sec-/native-only name from the pool as soon as ANY unrelated
// backbone name shared the pool: measured, a query "Abietinela" against a
// pool holding the native-only concept "Abietinella" plus the unrelated WCVP
// name "Abies alba" (both matching the prefilter's prefix+length window)
// resolved to nothing, because tier 1 saw "Abies alba" (no SecReference) and
// dropped every sec./native candidate pool-wide — including "Abietinella",
// which has no accepted backbone spelling at all. That killed both fuzzy
// resolution and the curator-facing near-miss list for exactly the names the
// fallback invariant (preferGenuineClaimants' own doc comment) exists to
// protect. Grouping by spelling first — so the preference only ever competes
// candidates that actually name the same taxon — fixes that without
// weakening the preference where it does apply (see
// TestMatchFuzzy_PreferenceAppliesPerSpellingNotAcrossThePool).
func preferGenuineClaimantsPerSpelling(candidates []output.MatchCandidate, nativeSpaces map[string]bool) []output.MatchCandidate {
	if len(candidates) == 0 {
		return candidates
	}
	groups := make(map[string][]output.MatchCandidate, len(candidates))
	order := make([]string, 0, len(candidates))
	for _, c := range candidates {
		key := domain.Canonicalize(c.MatchedName.Canonical)
		if _, seen := groups[key]; !seen {
			order = append(order, key)
		}
		groups[key] = append(groups[key], c)
	}
	// survive is a multiset (candidate value -> remaining count), not a plain
	// set: two structurally identical candidates in the pool must both
	// survive if the group-local filter keeps both. output.MatchCandidate
	// has no slice/map fields, so it is a valid map key.
	survive := make(map[output.MatchCandidate]int, len(candidates))
	for _, key := range order {
		for _, c := range preferGenuineClaimants(groups[key], nativeSpaces) {
			survive[c]++
		}
	}
	out := make([]output.MatchCandidate, 0, len(candidates))
	for _, c := range candidates {
		if survive[c] > 0 {
			survive[c]--
			out = append(out, c)
		}
	}
	return out
}

// nativeSpaceSet loads the name-space id set preferGenuineClaimants consults,
// exactly once per crosswalk run / match batch — never per name.
func nativeSpaceSet(ctx context.Context, repo output.Repository) (map[string]bool, error) {
	spaces, err := repo.NameSpaces(ctx)
	if err != nil {
		return nil, err
	}
	set := make(map[string]bool, len(spaces))
	for _, s := range spaces {
		set[s.ID] = true
	}
	return set, nil
}

// traitBearers adapts repository candidates to what genuineBearerWinner reads.
// Only the fields the tiers actually consult are carried over; the name is
// passed through because a classifiedHit without one would be a half-built
// value waiting to confuse the next reader of that type.
func traitBearers(candidates []output.MatchCandidate) []classifiedHit {
	hits := make([]classifiedHit, 0, len(candidates))
	for _, c := range candidates {
		hits = append(hits, classifiedHit{
			conceptID: c.Concept.ID,
			name:      c.MatchedName.Canonical,
			role:      c.Role,
			homotypic: c.Homotypic,
		})
	}
	return hits
}

// ruleCounts folds a per-rule row/taxon tally into the sorted []RuleCount
// every crosswalk report's Normalized field carries (e.g.
// NameSpaceIngestReport.Normalized). Shared across crosswalks because it IS
// the same breakdown of the same domain.NameCandidates ladder — a second
// copy could drift in sort order or in which rules it credits.
//
// Returns nil (not an empty slice) when no rule fired, so a source resolving
// purely on exact keys reports an absent breakdown rather than an empty one.
func ruleCounts(ruleRows map[domain.NormalizationRule]int, ruleTaxa map[domain.NormalizationRule]map[string]bool) []RuleCount {
	if len(ruleRows) == 0 {
		return nil
	}
	out := make([]RuleCount, 0, len(ruleRows))
	for rule, n := range ruleRows {
		out = append(out, RuleCount{Rule: rule, Rows: n, Taxa: len(ruleTaxa[rule]), Flagged: rule.Flagged()})
	}
	// out[i].Rule <= out[j].Rule is a genuinely equivalent mutant at
	// CONDITIONALS_BOUNDARY: out carries exactly one entry per DISTINCT
	// rule (it is built from a map keyed by rule), so two different indices
	// never hold equal Rules and sort.Slice never calls less(i, i) — the
	// boundary the mutant moves is unreachable, and no test can observe it.
	// Same provable-equivalence class as sortedSample's documented cap
	// boundary below.
	sort.Slice(out, func(i, j int) bool { return out[i].Rule < out[j].Rule })
	return out
}

// sortedSample returns a deterministic (sorted), bounded (at most
// unmatchedSampleCap) sample of set's keys, so a report's sample field
// never varies across runs and never grows unbounded for a large lossy
// source. Every caller in this package shares the same cap (there is
// exactly one sample-size policy), so it is not a parameter.
func sortedSample(set map[string]bool) []string {
	if len(set) == 0 {
		return nil
	}
	all := make([]string, 0, len(set))
	for k := range set {
		all = append(all, k)
	}
	sort.Strings(all)
	// len(all) >= unmatchedSampleCap is a genuinely equivalent mutant at
	// CONDITIONALS_BOUNDARY: at len(all) == unmatchedSampleCap exactly,
	// all[:unmatchedSampleCap] IS all, so both branches produce the
	// identical slice and no test can observe the difference (same
	// provable-equivalence class as the two documented boundaries in
	// suggest.go).
	if len(all) > unmatchedSampleCap {
		all = all[:unmatchedSampleCap]
	}
	return all
}

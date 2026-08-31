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
	// genuineBearerWinner. Used by name-space ingest (see
	// resolveNameSpaceNames in namespace_ingest.go): a sec.-space reference
	// concept (e.g. one of CDM's ~18 Standardliste sec. spaces) is an
	// attribution detail of a SEPARATE concept source, not a second genuine
	// claimant on a eurosl/germansl spelling, so it must not count toward
	// "this name is ambiguous" once a real backbone (WCVP) concept already
	// carries it. Measured on a full real ingest (2026-08-31, rows/matched/
	// ambiguous — see resolveNameSpaceNames' call site for the exact
	// before/after-fix figures): loading CDM alongside eurosl/germansl in
	// the same database, this filter recovers essentially the same match
	// rate as dropping CDM's concept_sources entry entirely (eurosl
	// 113733 vs 113607 matched, germansl 10436 vs 10393) while keeping CDM
	// — and /v1/translate — in the database. CDM was the dominant source of
	// "ambiguous" here, not genuine eurosl/germansl homonymy. Unlike
	// policyResolveGenuineBearer, this stays SILENT (no tieBroken, no report
	// counter) precisely because it resolves nothing on its own — it only
	// narrows the candidate set before the normal single-candidate/ambiguous
	// decision runs, so it needs no new report field and carries none of the
	// "was this measured for name spaces" risk the tie-break's own doc
	// comment warns about.
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
)

// resolveTraitName walks domain.NameCandidates' deterministic ladder for
// the already canonicalized name canon and returns the FIRST candidate key
// the index answers at all, classified into exactly one of: matched to a
// single concept id, matched=false (no key answered), or ambiguous=true.
//
// What counts as ambiguous depends on policy. The base rule, under both
// policies this package defines: a key answering with two or more distinct
// concepts is ambiguous and no concept is picked. Under policyPreferBackbone,
// sec.-space-only candidates are dropped first (preferBackboneConcepts)
// whenever a backbone concept shares the name, but whatever remains still
// follows that base rule — no tie-break runs, so this outcome never carries
// tieBroken. Under policyResolveGenuineBearer the SAME preferBackboneConcepts
// filter runs, and additionally a remaining tie is broken by
// genuineBearerWinner — only a key with no single genuine bearer is
// ambiguous, and the outcome then carries tieBroken so the caller can report
// it.
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
func resolveTraitName(ctx context.Context, repo output.Repository, canon string, policy crosswalkPolicy) (traitResolution, error) {
	for _, cand := range domain.NameCandidates(canon) {
		candidates, err := repo.MatchExact(ctx, cand.Key)
		if err != nil {
			return traitResolution{}, err
		}
		if policy == policyResolveGenuineBearer || policy == policyPreferBackbone {
			candidates = preferBackboneConcepts(candidates)
		}
		if len(candidates) == 0 {
			continue
		}
		distinct := make(map[string]bool, len(candidates))
		for _, c := range candidates {
			distinct[c.Concept.ID] = true
		}
		if len(distinct) > 1 {
			// Several concepts hold this spelling — but "several concepts" is
			// not the same as "undecidable". genuineBearerWinner is the
			// tiered rule the serving path already applies to exactly this
			// case (issue #67 class 2) — accepted bearer first, homotypic
			// bearer second, tie stands otherwise.
			if id, ok := genuineBearerWinner(traitBearers(candidates)); ok && policy == policyResolveGenuineBearer {
				return traitResolution{conceptID: id, matched: true, tieBroken: true, rule: cand.Rule}, nil
			}
			return traitResolution{ambiguous: true, rule: cand.Rule}, nil
		}
		return traitResolution{conceptID: candidates[0].Concept.ID, matched: true, rule: cand.Rule}, nil
	}
	return traitResolution{}, nil
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

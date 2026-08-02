package domain

import (
	"fmt"
	"strings"
)

// Relation is a typed taxon-concept relation in the Berendsohn sense: an
// assertion about how the CIRCUMSCRIPTION of one concept relates to that of
// another, across two different sec. reference spaces (UC6, SP5).
//
// The vocabulary below is MEASURED, not assumed. hostus' schema originally
// carried five values (congruent|includes|included_in|overlaps|disjoint) as
// an SP1 assumption. A full crawl of the CDM rl_standardliste
// (pipelines/cdm/cdm.summary.txt: 26.346 relations) corrected that
// assumption in both directions:
//
//   - "disjoint" NEVER occurs and has been dropped;
//   - "included_in" never occurs IN THE DATA either, but is retained as the
//     documented inverse of "includes" (see Inverse) — CDM only ever emits
//     the "Includes" direction, and hostus stores that direction verbatim
//     rather than synthesizing the mirror row (see the directionality note
//     in application.IngestCDM);
//   - three values the five did not have DO occur: "pro parte",
//     "misapplied" and the genuinely uncertain ⊂⊃⊕; and
//   - "Not Congruent to" occurs exactly once in 26.346 rows — invisible in
//     Task 1's 500-concept sample, found only because the pipeline carries
//     the raw vocabulary through. That single row is precisely why
//     ParseRelation must fail loudly rather than default.
type Relation string

const (
	// RelationCongruent: the two circumscriptions are the same set (≜).
	// ~85% of the measured relations.
	RelationCongruent Relation = "congruent"
	// RelationNotCongruent: the two circumscriptions are asserted NOT to be
	// the same set, without saying how they differ (CDM "Not Congruent to").
	RelationNotCongruent Relation = "not_congruent"
	// RelationIncludes: the from-concept's circumscription is a proper
	// superset of the to-concept's (⊃).
	RelationIncludes Relation = "includes"
	// RelationIncludedIn is the inverse of RelationIncludes (⊂). CDM never
	// emits it; it exists so query-time inversion has a name (see Inverse).
	RelationIncludedIn Relation = "included_in"
	// RelationOverlaps: the two circumscriptions share members, and each
	// holds members the other does not (⊕).
	RelationOverlaps Relation = "overlaps"
	// RelationUncertain is CDM's "Included in or Includes or Overlaps"
	// (⊂⊃⊕): the source asserts the two circumscriptions are related but
	// explicitly does NOT say which of the three ways. It is deliberately
	// its own value and is NEVER collapsed onto RelationOverlaps — that
	// collapse would silently upgrade an uncertain assertion into a definite
	// one, and /translate has to present the uncertainty honestly.
	RelationUncertain Relation = "includes_or_included_in_or_overlaps"
	// RelationProParte: the from-name applies only partly to the to-concept
	// (CDM "is pro parte synonym for"). CDM flags these conceptRelationship
	// = true, so hostus treats them as concept relations.
	RelationProParte Relation = "pro_parte"
	// RelationMisapplied: the from-name has been MISAPPLIED to the
	// to-concept (CDM "is misapplied name for", conceptRelationship =
	// false). This is a statement about name usage, not a set relation
	// between circumscriptions — see IsConceptRelation.
	RelationMisapplied Relation = "misapplied"
)

// rawRelations maps the lower-cased raw CDM relation spellings onto their
// Relation. Kept separate from the canonical spellings in ParseRelation's
// switch so it is obvious which side is the source vocabulary and which is
// hostus'.
var rawRelations = map[string]Relation{
	"congruent to":                        RelationCongruent,
	"not congruent to":                    RelationNotCongruent,
	"includes":                            RelationIncludes,
	"included in":                         RelationIncludedIn,
	"overlaps":                            RelationOverlaps,
	"included in or includes or overlaps": RelationUncertain,
	"is pro parte synonym for":            RelationProParte,
	"is misapplied name for":              RelationMisapplied,
}

// ParseRelation maps a relation spelling — either a raw CDM
// relationship-type label or one of Relation's own canonical values —
// onto a Relation, case-insensitively and ignoring surrounding whitespace.
//
// It is the STRICT parser and has no lenient sibling, deliberately. This is
// the ParseRank lesson applied: SP1's ParseRank knew six ranks, WCVP had 34,
// and the full ingest aborted 5,4 s in on "proles". The fix there was a
// lenient parser plus a verbatim column, because a rank is descriptive
// metadata. A relation is not: coercing an unmapped relation type onto a
// neighboring value would fabricate a scientific claim, and defaulting or
// dropping it would hide one. So an unmapped value returns an error NAMING
// the offending value, and IngestCDM counts and samples such rows rather
// than guessing.
func ParseRelation(s string) (Relation, error) {
	if r, ok := rawRelations[strings.ToLower(strings.TrimSpace(s))]; ok {
		return r, nil
	}
	switch Relation(strings.ToLower(strings.TrimSpace(s))) {
	case RelationCongruent:
		return RelationCongruent, nil
	case RelationNotCongruent:
		return RelationNotCongruent, nil
	case RelationIncludes:
		return RelationIncludes, nil
	case RelationIncludedIn:
		return RelationIncludedIn, nil
	case RelationOverlaps:
		return RelationOverlaps, nil
	case RelationUncertain:
		return RelationUncertain, nil
	case RelationProParte:
		return RelationProParte, nil
	case RelationMisapplied:
		return RelationMisapplied, nil
	default:
		return "", fmt.Errorf("domain: unknown concept relation type %q", s)
	}
}

// IsConceptRelation reports whether r is a genuine Berendsohn CONCEPT
// relation — an assertion about how two circumscriptions relate — as opposed
// to an assertion about name usage.
//
// Only RelationMisapplied is not: "Pinus abies L. 1753 is a misapplied name
// for Abies alba Mill. sec. Wisskirchen & Haeupler 1998" says nothing about
// how two circumscriptions overlap; it says a name was used wrongly. CDM
// agrees and flags exactly those rows conceptRelationship = false. Keeping
// them in concept_relation would make /translate silently mix two different
// kinds of claim under one column, so IngestCDM drops them (counted and
// sampled, never silent).
//
// RelationProParte is the near-miss that shows this is a real distinction
// and not just "is it a synonym relation": pro-parte rows carry
// conceptRelationship = true in CDM, because "A applies partly to B" IS a
// (partial) statement about circumscriptions.
func (r Relation) IsConceptRelation() bool {
	return r != RelationMisapplied
}

// IsEquality reports whether r licenses treating the two concepts as the
// SAME taxon.
//
// Exactly one relation does: RelationCongruent. Everything else — including
// RelationIncludes/RelationIncludedIn (a proper super-/subset), the
// deliberately undecided RelationUncertain (⊂⊃⊕), RelationOverlaps,
// RelationNotCongruent and the two directed name assertions — asserts
// something weaker than or different from identity, and /translate must
// never render it in a way a consumer could read as "same taxon" (SP5
// Task 4's first non-negotiable). This predicate exists so that rule lives
// in the domain, in ONE place, instead of being re-derived at every
// rendering site.
func (r Relation) IsEquality() bool {
	return r == RelationCongruent
}

// Inverse returns the relation that holds in the opposite direction, and
// whether one exists at all.
//
// congruent/not_congruent/overlaps/uncertain are symmetric (their own
// inverse). includes and included_in are each other's inverse. pro_parte
// and misapplied have NO inverse: they are directed assertions about the
// FROM-side name, and mirroring them would invent a claim the source never
// made.
//
// hostus stores relations in the direction the source states them and never
// materializes the mirror row (see application.IngestCDM); this method is
// what lets a query-time consumer traverse an edge backwards without a
// second stored row.
func (r Relation) Inverse() (Relation, bool) {
	switch r {
	case RelationCongruent, RelationNotCongruent, RelationOverlaps, RelationUncertain:
		return r, true
	case RelationIncludes:
		return RelationIncludedIn, true
	case RelationIncludedIn:
		return RelationIncludes, true
	case RelationProParte, RelationMisapplied:
		return "", false
	default:
		return "", false
	}
}

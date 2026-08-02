package domain

import (
	"fmt"
	"sort"
	"strings"
)

// NomStatusJudgement is the publication verdict for one raw WCVP
// `nom_status` cell (UC5, SP6).
//
// The vocabulary this classifies is MEASURED, not assumed (SP6 Task 1):
// `nom_status` is populated on 99.252 of 1.448.984 names (6,85 %), 92.492 of
// them on synonym-role names, across 1.304 DISTINCT values. The top 20 cover
// 95,8 %; 1.225 values have fewer than 10 hits. That shape rules out both a
// closed enum and fail-loud parsing — fail-loud would abort on the tail
// constantly. It also rules out equality matching: the UC5 worked example,
// Corynephorus incanescens Bubani (wcvp:name:405842), carries
// ", nom. illeg. superfl." and NOT the ", nom. superfl." the source doc
// assumes. Exact ", nom. superfl." hits 1.716 names; containment on
// "superfl" hits 12.502. Matching is therefore token containment over a
// normalized cell, never equality.
type NomStatusJudgement string

const (
	// JudgementAbsent: the source recorded no status at all (1.349.732 of
	// 1.448.984 names). Deliberately NOT the same as JudgementAcceptable —
	// "nothing was written down" is not "checked and found clean" — but it
	// is the only judgement that leaves a synonym publishable, because
	// withholding 93 % of the corpus on an absence would defeat the
	// endpoint.
	JudgementAbsent NomStatusJudgement = "absent"
	// JudgementAcceptable: the cell asserts the name IS nomenclaturally
	// sound (nom. cons., orth. cons., nom. altern., legitimate homonym).
	JudgementAcceptable NomStatusJudgement = "acceptable"
	// JudgementDisqualifying: the cell records a nomenclatural defect. Such
	// a name is excluded from the publication list outright, not
	// down-ranked — a nom. nud. does not belong in a publication at any
	// position.
	JudgementDisqualifying NomStatusJudgement = "disqualifying"
	// JudgementUnclassified: the source DID record something and no rule
	// covers it — the 1.225-value long tail, the 141 values that are not
	// statuses at all (citation fragments like "[Cusc.: 184]", free text
	// like `published as "mutatio nova"`), the five values that need a
	// botanical decision (see BotanicalOpenItems), and pending proposals
	// (nom. cons. prop. / nom. rej. prop.). It is NOT publishable: unlike
	// JudgementAbsent, something was written down, so the conservative
	// reading is that it may matter. The raw value travels with the verdict
	// and the count is summarized (SummarizeSynonyms), so every exclusion
	// is auditable and any tail value that turns out to matter can be
	// promoted to a rule later.
	JudgementUnclassified NomStatusJudgement = "unclassified"
)

// NomStatusRule is one token-containment rule. Names is the number of names
// in the measured 1.448.984-name WCVP corpus whose `nom_status` contains
// Fragment (case-insensitive substring, SP6 Task 1 / re-measured for this
// task). No rule exists for a token nobody observed.
type NomStatusRule struct {
	// Fragment is the measured token: the normalized (lower-case,
	// whitespace-collapsed) substring searched for in the normalized cell.
	// It is called Fragment rather than Token only because gosec's G101
	// reads a struct field named "token" as a hardcoded credential.
	Fragment string
	// Judgement is the verdict this rule contributes.
	Judgement NomStatusJudgement
	// Names is the measured containment count for Fragment.
	Names int
	// OpenItem marks the values that need a BOTANICAL, not a technical,
	// decision (Task 1 §5.3). They are surfaced, never guessed.
	OpenItem bool
	// Note records what the token means and why it lands where it does.
	Note string
}

// nomStatusUncertainty is the ONE rule that overrides everything, including a
// disqualifying token: a literal question mark means the SOURCE ITSELF is
// unsure, and hostus does not resolve an uncertainty WCVP declined to
// resolve. It is deliberately generic rather than tied to one value.
//
// Measured: 13 names carry "?" in nom_status, across exactly three values —
// ", not validly publ.?" (8), ", an nom. valid.?" (4) and
// ", nom. superfl. ?" (1). An earlier revision guarded only the first of
// them, which classified ", nom. superfl. ?" as disqualifying via the bare
// "superfl" rule: the identical epistemic situation, the opposite verdict.
// One name's worth of blast radius, but the principle has to hold, so the
// marker is now matched wherever it appears.
//
// Unlike a guard the marker is NOT masked out: the other rules still run and
// still appear in Matched, so the reason shows what the cell would have said
// had the source been sure. Only the verdict is overridden — always towards
// unclassified, i.e. withheld, never towards publishable.
var nomStatusUncertainty = NomStatusRule{
	Fragment: "?", Judgement: JudgementUnclassified, Names: 13, OpenItem: true,
	Note: "literal question mark: the source itself is unsure; covers ', not validly publ.?' (8), ', an nom. valid.?' (4), ', nom. superfl. ?' (1)",
}

// nomStatusGuards run BEFORE nomStatusRules and their match is MASKED OUT of
// the cell, so a broader token cannot fire on text a narrower rule has
// already claimed. Two kinds live here:
//
//   - botanical open items (OpenItem) — see BotanicalOpenItems; and
//   - pending nomenclatural PROPOSALS, where the decision has not been
//     taken: "nom. cons. prop." would read as conserved, "nom. rej. prop."
//     as rejected.
//
// A guard never overrides a defect found elsewhere in the same cell — see
// ClassifyNomStatus's precedence. The uncertainty marker above is the sole
// exception, and it is not a guard.
var nomStatusGuards = []NomStatusRule{
	{Fragment: "sensu auct.", Judgement: JudgementUnclassified, Names: 1117, OpenItem: true,
		Note: "a misapplication, not a nomenclatural defect — whether UC5 excludes it is a botanical decision"},
	{Fragment: "tentatively listed as a synonym", Judgement: JudgementUnclassified, Names: 290, OpenItem: true,
		Note: "taxonomic uncertainty, not a publication question"},
	{Fragment: "fossil name", Judgement: JudgementUnclassified, Names: 274, OpenItem: true,
		Note: "says nothing about nomenclatural validity"},
	{Fragment: "isonym", Judgement: JudgementUnclassified, Names: 13, OpenItem: true,
		Note: "duplicate publication of the same name; relevance to a publication filter is a judgement call"},

	{Fragment: "nom. cons. prop.", Judgement: JudgementUnclassified, Names: 33,
		Note: "conservation PROPOSED, not decided — must not read as nom. cons."},
	{Fragment: "nom. utique rej. prop.", Judgement: JudgementUnclassified, Names: 14,
		Note: "utter rejection PROPOSED, not decided"},
	{Fragment: "nom. rej. prop.", Judgement: JudgementUnclassified, Names: 48,
		Note: "rejection PROPOSED, not decided — must not read as nom. rej."},
}

// nomStatusRules are evaluated over the guard-masked cell. Order inside the
// slice is presentation order only (descending measured reach); precedence
// is decided by Judgement in ClassifyNomStatus, not by position.
//
// Deliberately NOT rules, although they look tempting:
//   - bare "homonym" (36.517) — ", legitimate homonym." (12) is explicitly
//     legitimate; the illegitimate cases are already covered by "illeg"
//     (49.705) and "later homonym" (60).
//   - bare "orth." (2.219) — ", orth. cons." (11) and ", nom. & orth. cons."
//     (7) are conserved spellings, the opposite verdict from "orth. var.".
//   - bare "descr." (1.411) — ", descr. ampl." (2) is an amplified
//     description, not a defect.
//
// The clearest PROMOTABLE candidates in the unclassified tail are
// ", typ. cons." (4) and ", nom. & typ. cons." (2): they assert a CONSERVED
// TYPE and contain neither "nom. cons." nor "type", so they fall through to
// unclassified and are withheld. Withheld is the safe direction and one
// table row promotes them once a botanist confirms the reading.
var nomStatusRules = []NomStatusRule{
	{Fragment: "illeg", Judgement: JudgementDisqualifying, Names: 49705,
		Note: "illegitimate; covers nom. illeg. homonym. post. (36.424), nom. illeg. superfl. (10.768), nom. illeg. (2.405)"},
	{Fragment: "not validly publ", Judgement: JudgementDisqualifying, Names: 18623,
		Note: "not validly published; also covers basionym/genus/species-level variants"},
	{Fragment: "superfl", Judgement: JudgementDisqualifying, Names: 12502,
		Note: "superfluous when published; the UC5 worked example's token"},
	{Fragment: "nom. nud.", Judgement: JudgementDisqualifying, Names: 9222,
		Note: "nomen nudum — published without a description"},
	{Fragment: "pro syn", Judgement: JudgementDisqualifying, Names: 6224,
		Note: "published as a synonym, hence not validly published"},
	{Fragment: "orth. var.", Judgement: JudgementDisqualifying, Names: 2196,
		Note: "orthographic variant — a misspelling, not a name to publish"},
	{Fragment: "opus utique", Judgement: JudgementDisqualifying, Names: 1640,
		Note: "published in a suppressed work (opus utique oppr. 1.528 / rej. 111)"},
	{Fragment: "basionym", Judgement: JudgementDisqualifying, Names: 1438,
		Note: "defective or missing basionym reference — an invalid combination (all 1.438 cells are defect statements)"},
	{Fragment: "latin descr", Judgement: JudgementDisqualifying, Names: 1344,
		Note: "no Latin description; merges the measured spellings without a/latin/Latin"},
	{Fragment: "type", Judgement: JudgementDisqualifying, Names: 1099,
		Note: "type-citation defect; every one of the 1.099 cells containing 'type' states a defect (verified by grouping all distinct values)"},
	{Fragment: "nom. rej", Judgement: JudgementDisqualifying, Names: 894,
		Note: "rejected; merges nom. rej. (831) and nom. rejic. (10) — the '. prop.' proposals are masked by a guard"},
	{Fragment: "contrary to art", Judgement: JudgementDisqualifying, Names: 432,
		Note: "published contrary to a named ICN/ICBN article"},
	{Fragment: "nom. provis", Judgement: JudgementDisqualifying, Names: 363,
		Note: "provisional name — not validly published"},
	{Fragment: "nom. subnud", Judgement: JudgementDisqualifying, Names: 238,
		Note: "insufficiently described"},
	{Fragment: "comb. not", Judgement: JudgementDisqualifying, Names: 201,
		Note: "combination not made / not validly published"},
	{Fragment: "sphalm", Judgement: JudgementDisqualifying, Names: 199,
		Note: "sphalmate — a printing error, not a name"},
	{Fragment: "nom. utique rej", Judgement: JudgementDisqualifying, Names: 151,
		Note: "utterly rejected; the '. prop.' proposals are masked by a guard"},
	{Fragment: "not effectively publ", Judgement: JudgementDisqualifying, Names: 66,
		Note: "not effectively published; merges publ./published spellings"},
	{Fragment: "describing the collection", Judgement: JudgementDisqualifying, Names: 61,
		Note: "describes the collection, not the taxon — invalid"},
	{Fragment: "later homonym", Judgement: JudgementDisqualifying, Names: 60,
		Note: "later homonym; illegitimate under Art. 53"},
	{Fragment: "combination not", Judgement: JudgementDisqualifying, Names: 37,
		Note: "spelled-out variant of comb. not made."},
	{Fragment: "without diagnostic descr", Judgement: JudgementDisqualifying, Names: 17,
		Note: "no diagnostic description"},
	{Fragment: "sine descr. lat.", Judgement: JudgementDisqualifying, Names: 15,
		Note: "Latin spelling of 'without a Latin descr.'"},

	{Fragment: "nom. cons.", Judgement: JudgementAcceptable, Names: 1237,
		Note: "conserved name — explicitly legitimate ('. prop.' proposals masked by a guard)"},
	{Fragment: "nom. altern.", Judgement: JudgementAcceptable, Names: 103,
		Note: "alternative name, validly published"},
	{Fragment: "nom. alt.", Judgement: JudgementAcceptable, Names: 36,
		Note: "short spelling of nom. altern."},
	{Fragment: "legitimate homonym", Judgement: JudgementAcceptable, Names: 12,
		Note: "explicitly legitimate; the reason bare 'homonym' is not a rule"},
	{Fragment: "orth. cons.", Judgement: JudgementAcceptable, Names: 11,
		Note: "conserved spelling; also covers nom. & orth. cons. (7)"},
}

// NomStatusRules returns the complete rule table — the uncertainty marker,
// then the guards, then the containment rules — as a copy, so callers (and
// the docs generator) can render the token/count/note table without reaching
// into package state.
func NomStatusRules() []NomStatusRule {
	out := make([]NomStatusRule, len(nomStatusTable))
	copy(out, nomStatusTable)
	return out
}

// nomStatusTable is the concatenated rule table in presentation order. It
// exists so NomStatusRules can hand out a copy without recomputing the
// concatenation on every call.
var nomStatusTable = append(
	append([]NomStatusRule{nomStatusUncertainty}, nomStatusGuards...),
	nomStatusRules...,
)

// BotanicalOpenItems returns the values whose UC5 treatment is a BOTANICAL,
// not a technical, decision (Task 1 §5.3). They are classified
// JudgementUnclassified on purpose: hostus surfaces them instead of guessing
// a verdict a botanist has not given.
func BotanicalOpenItems() []NomStatusRule {
	out := []NomStatusRule{nomStatusUncertainty}
	for _, r := range nomStatusGuards {
		if r.OpenItem {
			out = append(out, r)
		}
	}
	return out
}

// NormalizeNomStatus prepares a raw WCVP `nom_status` cell for token
// matching: lower-case, whitespace collapsed to single spaces (16 names
// carry ", contrary to  Art. 39.1. (ICN, 2012)." with a double space), and
// the leading comma/space stripped — 99.111 of 99.252 populated cells begin
// with ", " because WCVP concatenates a citation fragment with the status.
// The cell is kept whole: 684 cells carry several statuses, but splitting on
// "," is wrong because commas occur INSIDE single statuses.
func NormalizeNomStatus(raw string) string {
	s := strings.Join(strings.Fields(strings.ToLower(raw)), " ")
	return strings.TrimLeft(s, ", ")
}

// NomStatusVerdict is the classification of one raw `nom_status` cell,
// carrying the raw value and every rule that fired so a consumer sees the
// reason and not just the verdict.
type NomStatusVerdict struct {
	Raw        string
	Normalized string
	Judgement  NomStatusJudgement
	Matched    []NomStatusRule
}

// Reason renders the verdict as one human-readable sentence.
func (v NomStatusVerdict) Reason() string {
	if v.Judgement == JudgementAbsent {
		return "no nom_status recorded (not the same as verified clean)"
	}
	if len(v.Matched) == 0 {
		return fmt.Sprintf("nom_status %q matches no known token", v.Raw)
	}
	tokens := make([]string, 0, len(v.Matched))
	for _, r := range v.Matched {
		tokens = append(tokens, fmt.Sprintf("%q", r.Fragment))
	}
	return fmt.Sprintf("nom_status %q: %s (%s)", v.Raw, v.Judgement, strings.Join(tokens, ", "))
}

// ClassifyNomStatus classifies a raw `nom_status` cell by token containment.
//
// Precedence, in order:
//  1. the uncertainty marker "?" wins over everything — the source itself is
//     unsure, so hostus withholds rather than resolve it (nomStatusUncertainty);
//  2. any disqualifying token in the guard-masked cell — a recorded defect is
//     a defect even when the cell also asserts something sound
//     (", nom. cons., nom. illeg.") or carries an open-item token
//     (", nom. illeg., later homonym of a fossil name.");
//  3. otherwise a guard match (botanical open item or pending proposal)
//     yields JudgementUnclassified;
//  4. otherwise an acceptable token yields JudgementAcceptable;
//  5. otherwise JudgementUnclassified — the long tail, with no rule matched.
//
// An empty cell yields JudgementAbsent.
func ClassifyNomStatus(raw string) NomStatusVerdict {
	v := NomStatusVerdict{Raw: raw, Normalized: NormalizeNomStatus(raw)}
	if v.Normalized == "" {
		v.Judgement = JudgementAbsent
		return v
	}

	rest := v.Normalized
	uncertain := strings.Contains(rest, nomStatusUncertainty.Fragment)
	if uncertain {
		v.Matched = append(v.Matched, nomStatusUncertainty)
	}
	guarded := false
	for _, g := range nomStatusGuards {
		if strings.Contains(rest, g.Fragment) {
			v.Matched = append(v.Matched, g)
			rest = strings.ReplaceAll(rest, g.Fragment, " ")
			guarded = true
		}
	}

	var disqualified, acceptable []NomStatusRule
	for _, r := range nomStatusRules {
		if !strings.Contains(rest, r.Fragment) {
			continue
		}
		if r.Judgement == JudgementDisqualifying {
			disqualified = append(disqualified, r)
			continue
		}
		acceptable = append(acceptable, r)
	}
	v.Matched = append(v.Matched, disqualified...)
	v.Matched = append(v.Matched, acceptable...)

	// The two comparisons are HOISTED out of the switch on purpose. Go's
	// coverage model ends the enclosing basic block at a switch statement's
	// opening brace, so a condition written inside a `case` arm sits in no
	// counted block at all: `make mutation` reported CONDITIONALS_BOUNDARY
	// and CONDITIONALS_NEGATION mutants here as NOT COVERED, which no test
	// could have fixed. As plain assignments they are covered, mutated and
	// killed; the case arms are then bare identifiers, which carry no
	// mutants of their own.
	anyDisqualifying := len(disqualified) > 0
	anyAcceptable := len(acceptable) > 0

	switch {
	case uncertain:
		v.Judgement = JudgementUnclassified
	case anyDisqualifying:
		v.Judgement = JudgementDisqualifying
	case guarded:
		v.Judgement = JudgementUnclassified
	case anyAcceptable:
		v.Judgement = JudgementAcceptable
	default:
		v.Judgement = JudgementUnclassified
	}
	return v
}

// Typification says how a synonym's type relates to the accepted name's.
//
// It is a TRI-state because `concept_name.homotypic` is: true on 271.821
// rows (basionym-proven), false, and NULL on 692.941 rows. NULL means
// UNKNOWN — SP3 deliberately refused to guess, and /v1/concept omits the
// field rather than claiming false. Collapsing NULL onto heterotypic would
// silently demote most of the corpus on a fact nobody established.
type Typification string

const (
	// TypificationHomotypic: same type as the accepted name (homotypic = true).
	TypificationHomotypic Typification = "homotypic"
	// TypificationUnknown: the source did not establish the relation
	// (homotypic IS NULL). Ranks between the two known states and is
	// reported as its own value.
	TypificationUnknown Typification = "unknown"
	// TypificationHeterotypic: different type (homotypic = false).
	TypificationHeterotypic Typification = "heterotypic"
)

// TypificationOf maps the tri-state homotypic flag onto a Typification. A
// nil pointer is UNKNOWN, never heterotypic.
func TypificationOf(homotypic *bool) Typification {
	if homotypic == nil {
		return TypificationUnknown
	}
	if *homotypic {
		return TypificationHomotypic
	}
	return TypificationHeterotypic
}

// TypificationOrder is the UC5 rule-3 ordinal: known-homotypic (0) before
// unknown (1) before known-heterotypic (2).
func TypificationOrder(t Typification) int {
	switch t {
	case TypificationHomotypic:
		return 0
	case TypificationUnknown:
		return 1
	case TypificationHeterotypic:
		return 2
	default:
		return 3
	}
}

// SynonymCandidate is one synonym of an accepted concept, as the repository
// hands it to the relevance model. NomStatus is the VERBATIM WCVP cell
// (possibly empty); Homotypic is the tri-state flag, nil meaning unknown;
// IsBasionym is true when this name is the accepted name's basionym
// (429.172 names carry a basionym_id) — resolving that link is the
// repository's job, not the domain's.
type SynonymCandidate struct {
	NameID     string
	ConceptID  string
	Canonical  string
	Authorship string
	Rank       Rank
	// RankVerbatim is the original source "taxonrank" spelling when Rank is
	// RankOther, empty otherwise — same contract as Name.RankVerbatim. It is
	// carried (not used) by the relevance model: no UC5 rule reads it, but
	// 3.731 of the 6.409 OTHER-ranked synonym rows have one (`proles`,
	// `lusus`, `microgène`, `Convariety`, `grex`), none of them is excluded
	// by RanksBelowSpecies, and a consumer that only saw "OTHER" could not
	// tell what it had been handed.
	RankVerbatim string
	NomStatus    string
	Homotypic    *bool
	IsBasionym   bool
}

// SynonymExclusion names the rule that withheld a synonym from the
// publication list. Empty means the synonym is publishable.
type SynonymExclusion string

const (
	// ExclusionNone: not excluded.
	ExclusionNone SynonymExclusion = ""
	// ExclusionNomStatus: UC5 rule 1 — a recorded nomenclatural defect.
	ExclusionNomStatus SynonymExclusion = "nom_status"
	// ExclusionUnclassifiedStatus: UC5 rule 1, long tail — a status was
	// recorded and no rule covers it. Withheld rather than published, with
	// the raw value kept visible.
	ExclusionUnclassifiedStatus SynonymExclusion = "unclassified_nom_status"
	// ExclusionRank: UC5 rule 2 — the caller publishes above this rank.
	ExclusionRank SynonymExclusion = "rank"
)

// SynonymOptions carries the caller-controlled part of the decision. Rank
// exclusion is NOT hard-wired: a caller publishing at species level passes
// RanksBelowSpecies(), a caller publishing a full infraspecific treatment
// passes nothing.
type SynonymOptions struct {
	ExcludeRanks []Rank
}

// RanksBelowSpecies returns the ranks UC5 names for a species-level
// publication: VARIETY, SUBVARIETY, FORM, SUBFORM. The nothotaxon ranks are
// deliberately absent — UC5 does not name them and hostus does not invent a
// rule the use case has not asked for.
func RanksBelowSpecies() []Rank {
	return []Rank{RankVariety, RankSubvariety, RankForm, RankSubform}
}

// SynonymRelevance is the decision for one synonym, carrying WHY: the full
// nom_status verdict (raw value plus every rule that fired), which of the
// three typification states applied, and — when withheld — which rule
// excluded it.
type SynonymRelevance struct {
	Candidate    SynonymCandidate
	Publishable  bool
	Status       NomStatusVerdict
	Typification Typification
	Exclusion    SynonymExclusion
	Reason       string
}

// RankSynonyms judges and orders synonyms for a publication list (UC5). It
// returns EVERY candidate — excluded ones included, so Task 3 can render an
// exclusion summary — with the publishable ones first.
//
// Judgement, in the source doc's priority:
//  1. exclude on a recorded nomenclatural defect (ClassifyNomStatus); an
//     unclassified status is also withheld, under its own reason;
//  2. exclude on rank, only for the ranks the caller listed;
//  3. homotypic before unknown before heterotypic (see Typification);
//  4. the basionym leads its typification block;
//  5. NameID as the final tiebreaker.
//
// PRECONDITION: NameIDs must be UNIQUE within items. Under that precondition
// the sort key is a TOTAL order and any permutation of the same input yields
// byte-identical output. It is not merely stable — see
// TestRankSynonyms_TotalOrder, which permutes exhaustively. Duplicate NameIDs
// break the guarantee: the comparator then reports the pair as neither less
// nor greater and sort.SliceStable falls back to input order, so a shuffled
// input can come back shuffled. The precondition is free to satisfy —
// concept_name's primary key is (concept_id, name_id), and the measured
// corpus contains zero duplicate pairs — but it is a precondition, not an
// invariant this package can enforce.
//
// RankSynonyms is pure and does not mutate its input.
func RankSynonyms(items []SynonymCandidate, opts SynonymOptions) []SynonymRelevance {
	excluded := make(map[Rank]bool, len(opts.ExcludeRanks))
	for _, r := range opts.ExcludeRanks {
		excluded[r] = true
	}

	out := make([]SynonymRelevance, 0, len(items))
	for _, c := range items {
		out = append(out, judgeSynonym(c, excluded))
	}

	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Publishable != b.Publishable {
			return a.Publishable
		}
		if ao, bo := TypificationOrder(a.Typification), TypificationOrder(b.Typification); ao != bo {
			return ao < bo
		}
		if a.Candidate.IsBasionym != b.Candidate.IsBasionym {
			return a.Candidate.IsBasionym
		}
		return a.Candidate.NameID < b.Candidate.NameID
	})

	return out
}

func judgeSynonym(c SynonymCandidate, excludedRanks map[Rank]bool) SynonymRelevance {
	rel := SynonymRelevance{
		Candidate:    c,
		Status:       ClassifyNomStatus(c.NomStatus),
		Typification: TypificationOf(c.Homotypic),
	}

	// Hoisted for the same coverage reason as ClassifyNomStatus' switch.
	disqualifying := rel.Status.Judgement == JudgementDisqualifying
	unclassified := rel.Status.Judgement == JudgementUnclassified

	switch {
	case disqualifying:
		rel.Exclusion = ExclusionNomStatus
		rel.Reason = rel.Status.Reason()
	case unclassified:
		rel.Exclusion = ExclusionUnclassifiedStatus
		rel.Reason = "withheld for review: " + rel.Status.Reason()
	case excludedRanks[c.Rank]:
		rel.Exclusion = ExclusionRank
		rel.Reason = fmt.Sprintf("rank %s excluded by the caller", c.Rank)
	default:
		rel.Publishable = true
		rel.Reason = fmt.Sprintf("%s, %s", rel.Typification, rel.Status.Reason())
	}

	return rel
}

// SynonymExclusionSummary is the auditable counterpart to the ranked list:
// how many synonyms were withheld, by which rule, and which raw nom_status
// values hostus could not classify. Task 3 renders it into the response so
// an unclassified value is visible rather than silently dropped.
type SynonymExclusionSummary struct {
	Total       int
	Publishable int
	// Absent counts the publishable synonyms whose nom_status was EMPTY.
	// "Nothing was recorded" is not "checked and found clean", so the audit
	// trail has to say how much of a publication list rests on an absence
	// rather than on a soundness assertion.
	Absent int
	// Excluded is always non-nil, even when nothing was excluded, so Task 3
	// can increment into it without a nil-map panic.
	Excluded             map[SynonymExclusion]int
	UnclassifiedStatuses []string
}

// SummarizeSynonyms counts a ranked list by exclusion reason and collects
// the distinct raw nom_status values that fell through to unclassified,
// sorted for determinism.
func SummarizeSynonyms(rel []SynonymRelevance) SynonymExclusionSummary {
	sum := SynonymExclusionSummary{Total: len(rel), Excluded: map[SynonymExclusion]int{}}
	seen := map[string]bool{}
	for _, r := range rel {
		if r.Publishable {
			sum.Publishable++
			if r.Status.Judgement == JudgementAbsent {
				sum.Absent++
			}
			continue
		}
		sum.Excluded[r.Exclusion]++
		if r.Exclusion == ExclusionUnclassifiedStatus && !seen[r.Candidate.NomStatus] {
			seen[r.Candidate.NomStatus] = true
			sum.UnclassifiedStatuses = append(sum.UnclassifiedStatuses, r.Candidate.NomStatus)
		}
	}
	sort.Strings(sum.UnclassifiedStatuses)
	return sum
}

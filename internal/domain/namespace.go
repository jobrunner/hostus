package domain

import "strings"

// Name spaces (SP9, UC4).
//
// A NAME SPACE is a checklist that contributes NAMES but no taxonomy: it has
// no synonymy graph, no parent chain and no external ids hostus could join
// on — only a flat list of the spellings that space uses, each with the
// space's own stable id. FloraVeg.EU's list (the namespace the ESy expert
// system's rules are written against) is the first one; the same canonical
// CSV contract covers GermanSL, EuroSL and Euro+Med (pipelines/README.md,
// "Canonical CSV contract (name lists)").
//
// A name space is therefore deliberately NOT a backbone and NOT a trait
// vocabulary:
//
//   - It must not land in backbone_version. Those rows are served verbatim
//     as the backbone_versions provenance block of every /v1/suggest and
//     /v1/match response and they gate /health/ready — a name list that
//     contributes zero concepts would tell clients it is a backbone.
//   - It contributes no taxon_concept rows at all. Its entries ATTACH to
//     concepts an existing backbone already holds, via the SP3 name
//     crosswalk (NameCandidates/Canonicalize), which is lossy by
//     construction and whose loss is reported rather than absorbed.
//
// What it buys is the thing UC4 is missing: given a concept, the spelling
// the target space uses for it (an ESy-compatible name), and whether that
// spelling denotes an AGGREGATE rather than a single taxon.

// NameSpaceMeta is one ingested name space's provenance row — the name-space
// counterpart of BackboneVersion/XrefSourceMeta. IngestedAt is stamped by
// the repository adapter, not by the caller, exactly as it is for those two.
type NameSpaceMeta struct {
	// ID is the manifest-pinned space id, e.g. "floraveg". It is also the
	// value NameSpaceEntry.Space carries and the one a /v1/match
	// target_space parameter will name.
	ID string
	// Version pins the harvest/edition, never "latest" (e.g. FloraVeg's
	// sheet date "2023-01-03").
	Version   string
	License   string
	SourceURL string
	// ManifestSHA binds this ingest to the exact manifest revision that was
	// validated, like BackboneVersion.ManifestSHA.
	ManifestSHA string
	// Redistribution gates ExportBundle, never local ingest. FloraVeg's is
	// "unknown" — no license statement is findable (pipelines/README.md).
	Redistribution Redistribution
}

// NameSpaceEntry is one name-space spelling attached to a hostus concept:
// the space's own id and name string, whether that name denotes an
// aggregate, and how the crosswalk reached the concept.
//
// One concept can legitimately carry SEVERAL entries from the same space.
// FloraVeg spells Festuca ovina three ways under three SeqIDs — "Festuca
// ovina" (5647), "Festuca ovina aggr." (5648) and "Festuca ovina s. l."
// (5649) — and all three crosswalk onto the same WCVP concept, because WCVP
// carries no aggregate-marked names at all (see
// NormalizationRule.Flagged). Collapsing them would throw away exactly the
// distinction UC4's aggregate_policy has to make, so the entry is keyed by
// (Space, ExtID) and every spelling is kept.
type NameSpaceEntry struct {
	Space string
	ExtID string
	// Name is the space's spelling, stored VERBATIM — this is the string a
	// caller asking for a target space gets back, so it must not be folded
	// to the canonical match key.
	Name string
	// Aggregate reports whether Name denotes a collective species
	// ("Sammelart") rather than a single taxon — see IsAggregateName.
	Aggregate bool
	// Status is the space's OWN nomenclatural status for this spelling,
	// verbatim from the source list ("accepted", "synonym",
	// "synonymobjective", ...). Empty means the ingest predates this field —
	// see AcceptedInSpace for why that is not the same as "not accepted".
	//
	// It is what makes a target-space name determinate. A space maps many of
	// its names onto ONE backbone concept (measured on the real index: 45% of
	// concepts with a eurosl entry carry 2 to 391 spellings), so without a
	// status any pick among them is arbitrary — and an arbitrary synonym
	// presented as "the name in that space" is worse than none.
	Status string
	// Resolution records HOW the crosswalk reached the concept: empty for an
	// exact canonical match, else the NormalizationRule that was needed.
	// Absence is information here: empty means "no normalisation was
	// needed", never "unknown".
	Resolution string
	// Rank is the space's OWN rank for this spelling, normalised through
	// ParseRankLenient at ingest (the source vocabularies disagree: Euro+Med
	// writes "Subspecies", GermanSL "SSP", WCVP "subsp.").
	//
	// It is what keeps a target-space name rank-determinate. Status alone
	// cannot: a space accepts a species AND its subspecies, so several
	// entries attached to one concept are equally accepted and the pick among
	// them fell to ext_id order — which answered a plain Bromus erectus with
	// "Bromopsis erecta subsp. permixta". The zero value means the entry
	// predates this field; see ResolveTargetSpace for what that falls back
	// to.
	Rank Rank
	// AcceptedName is the name the space itself files this spelling under — its
	// own accepted_taxon column, a NAME string rather than an id, and empty
	// when the entry IS the accepted one.
	//
	// It is what makes a target-space name determinate when status and rank
	// have both run out: a concept can carry several entries the space accepts,
	// at the same rank, belonging to DIFFERENT taxa (measured: 1.362 concepts
	// on the real eurosl index). Ranking that pool cannot pick the right one —
	// but the space usually spells the source concept itself and says, right
	// here, which of its taxa that spelling belongs to.
	AcceptedName string
}

// AggregatePolicy is UC4's tri-state answer to "can coverage assigned to an
// aggregate be resolved in the target space?". It is deliberately NOT a
// boolean — the three states are genuinely distinct (same reasoning as SP6's
// absent-vs-unclassified):
//
//   - AggregatePolicyKnown: the target space carries the aggregate as a taxon
//     of its own, so an ESy-compatible aggregate name exists for it.
//   - AggregatePolicyUnresolvable: the query IS an aggregate but the target
//     space knows only microspecies under it, no aggregate taxon. Per the
//     source document this means "not decidable", NOT "not met" — and coverage
//     must not be distributed onto the microspecies.
//   - the ZERO value (empty string): no aggregate is involved at all (a plain
//     species). Emitting Known here would drain the field of meaning, so a
//     plain species carries no policy.
type AggregatePolicy string

const (
	AggregatePolicyKnown        AggregatePolicy = "known"
	AggregatePolicyUnresolvable AggregatePolicy = "unresolvable"
)

// NameSpaceStatusAccepted is the status value a source uses for its own
// accepted name. Synonym statuses vary by source ("synonym",
// "synonymobjective", ...), so acceptance is tested positively rather than by
// listing everything it is not.
const NameSpaceStatusAccepted = "accepted"

// ResolutionSourceSynonymyClosure is the marker
// IngestNameSpace's post-resolve source-synonymy closure pass
// (application.closeSynonymyGroups, spec 2026-09-13 decision 3) appends onto
// NameSpaceEntry.Resolution for a row it attached — never the whole
// Resolution string by itself (it composes as "<rule>+source_synonymy_closure"
// when a normalisation rule also fired), which is why pickSpelling below
// tests it with strings.Contains rather than equality. Exported from domain,
// not kept as an application-private literal, so pickSpelling — which lives
// in domain because it decides ResolveTargetSpace's ordering, not the
// ingest's — has one shared source of truth for the string instead of a
// second copy that could drift from the one application actually writes.
const ResolutionSourceSynonymyClosure = "source_synonymy_closure"

// AcceptedInSpace reports whether e is the space's accepted spelling.
//
// An entry ingested before Status existed reports false — deliberately, so a
// stale index falls back to the old arbitrary-but-harmless behavior instead
// of claiming a synonym is accepted. Re-ingest is what fixes it.
func (e NameSpaceEntry) AcceptedInSpace() bool {
	return e.Status == NameSpaceStatusAccepted
}

// TargetSpaceChoice is the entry ResolveTargetSpace chose to report, not just
// its name: Habitatus needs the source's own identity for that spelling to
// tell whether it got the space's ACCEPTED name or a synonym (see Status) and
// to carry a stable external key forward (see ExtID) — the name alone answers
// neither question.
//
// Name == "" means "no entry" exactly as it did when ResolveTargetSpace
// returned a bare string; ExtID/Status are then also empty and carry no
// separate meaning.
type TargetSpaceChoice struct {
	// Name is the ESy-compatible spelling the target space uses — the same
	// value ResolveTargetSpace used to return on its own.
	Name string
	// ExtID is the chosen entry's OWN id in the target space
	// (NameSpaceEntry.ExtID) — for "eurosl" this is the Euro+Med
	// TaxonUsageID, a stable key external resources (e.g. EuroVeg.eu) can be
	// joined on. It is the source's id, not a hostus concept id.
	ExtID string
	// Status is the chosen entry's own nomenclatural status, verbatim from
	// the source (NameSpaceEntry.Status: "accepted", "synonym",
	// "synonymobjective", ...). This is how a caller distinguishes "the
	// space's accepted name" from a synonym fallback (see pickSpelling) —
	// the name string alone cannot carry that distinction.
	Status string
}

// TargetSpaceQuery is what ResolveTargetSpace knows about the concept being
// translated, as opposed to the candidate spellings it chooses among. It is a
// struct rather than a pair of parameters because the two fields answer
// unrelated questions and read as nothing at a call site — ResolveTargetSpace
// (true, false, entries) says neither which flag is which nor what either
// means.
type TargetSpaceQuery struct {
	// IsAggregate says whether the verbatim the caller matched carried an
	// aggregate marker (see IsAggregateName). It selects UC4's aggregate
	// branch, which is a different question from rank congruence: an
	// aggregate query wants the collective spelling even though its rank
	// matches no plain species.
	IsAggregate bool
	// SourceRank is the rank of the concept being translated, normalised to
	// hostus' own vocabulary. The zero value — and RankOther, which
	// ParseRankLenient returns for the empty string — both mean "unknown"
	// and stand the congruence rule down rather than filtering every
	// candidate away.
	SourceRank Rank
	// SourceName is the accepted canonical name of the concept being
	// translated. It is the key to the strongest evidence available: if the
	// space spells this name itself, that entry says — in the space's own
	// data — which of its taxa the concept belongs to. Empty stands the
	// anchor down and leaves the ranked preferences to decide.
	SourceName string
}

// ResolveTargetSpace decides, for one matched concept, the ESy-compatible name
// the target space uses and the AggregatePolicy that applies. query describes
// the concept being translated (see TargetSpaceQuery); entries are that
// concept's spellings in the target space (see NameSpaceEntry), already
// ordered by the repository.
//
// The rules mirror AggregatePolicy's three states exactly:
//
//   - query.IsAggregate and an aggregate-marked entry exists -> that spelling +
//     Known.
//   - query.IsAggregate and NO aggregate-marked entry exists -> "" + Unresolvable.
//     No name is handed back: offering the microspecies spelling here is
//     precisely the false "not met" the source document warns against.
//   - otherwise -> the nominate (non-aggregate) spelling if any, else the
//     first spelling, else ""; policy is the zero value (absent).
//
// Within either branch the source's OWN spelling is looked for first (see
// anchoredSpelling); only when the space does not spell it, or files it under
// a taxon whose accepted entry is not attached here, do the ranked
// preferences decide — accepted over synonym, then rank-congruent over not.
func ResolveTargetSpace(query TargetSpaceQuery, entries []NameSpaceEntry) (TargetSpaceChoice, AggregatePolicy) {
	if query.IsAggregate {
		if e, ok := pickWithAnchor(entries, true, query); ok {
			return TargetSpaceChoice{Name: e.Name, ExtID: e.ExtID, Status: e.Status}, AggregatePolicyKnown
		}
		return TargetSpaceChoice{}, AggregatePolicyUnresolvable
	}
	if e, ok := pickWithAnchor(entries, false, query); ok {
		return TargetSpaceChoice{Name: e.Name, ExtID: e.ExtID, Status: e.Status}, ""
	}
	if len(entries) > 0 {
		e := entries[0]
		return TargetSpaceChoice{Name: e.Name, ExtID: e.ExtID, Status: e.Status}, ""
	}
	return TargetSpaceChoice{}, ""
}

// pickWithAnchor is ResolveTargetSpace's choice for one aggregate branch: the
// anchored answer when the space's own data settles it, else the ranked one.
func pickWithAnchor(entries []NameSpaceEntry, aggregate bool, query TargetSpaceQuery) (NameSpaceEntry, bool) {
	if e, ok := anchoredSpelling(entries, aggregate, query.SourceName); ok {
		return e, true
	}
	return pickSpelling(entries, aggregate, query.SourceRank)
}

// anchoredSpelling answers from the target space's own statement about the
// source concept's spelling, rather than by ranking the candidate pool.
//
// Why it has to come first: status and rank both narrow the pool, but neither
// SEPARATES entries that are equally accepted at the same rank and belong to
// different taxa of that space. Measured on the real eurosl index
// (2026-10-07), 1.362 concepts carry such a pool; the WCVP Bromus erectus
// concept carries thirty eurosl entries, eight of them accepted. Ranking them
// picked "Bromopsis erecta subsp. permixta" before the rank fix and
// "Bromopsis zangezura" after it — both merely the lowest ext_id of their
// tier, neither the right taxon.
//
// The space itself knows the answer. Euro+Med spells "Bromus erectus" and
// files it as a synonym of Bromopsis erecta, so the chain is:
//
//	source concept's name -> the space's entry of that spelling
//	                      -> that entry's own accepted_taxon
//	                      -> the accepted entry carrying that name
//
// Two deliberate refusals. A spelling the space accepts outright ends the
// chain there (no hop). And when the hop finds no attached entry — the space
// files the name under a taxon this concept did not crosswalk onto — the
// anchor reports nothing instead of handing back the SYNONYM's spelling:
// a name the space explicitly does not accept is not "the name in that
// space", and the ranked fallback at least answers with one it does.
//
// aggregate is honored so the anchor never reaches across UC4's split; the
// comparison runs through Canonicalize, like every other name comparison here.
//
// EVERY row carrying the spelling is inspected, never just the first. Entries
// are keyed by (Space, ExtID), so a space may hold one spelling twice under
// different ids, and they arrive in ext_id order — stopping early would let
// that order decide, which is the kind of arbitrary pick the anchor exists to
// replace. Worse, a first row whose hop dangles would suppress a later row
// whose hop resolves. The three outcomes:
//
//   - any row the space accepts outright -> that row (strongest, no hop);
//   - otherwise the hops that resolve, but only if they AGREE — two rows of
//     the same spelling pointing at different accepted taxa is the space
//     telling us two incompatible things, and choosing between them by ext_id
//     would dress an arbitrary pick as source evidence;
//   - otherwise nothing, leaving the ranked preferences to answer.
//
// Measured honestly: no concept on the real eurosl/floraveg/germansl indexes
// carries a repeated spelling today (2026-10-07), so this is precaution rather
// than a fix for observed data. It costs one pass and removes an input order
// from a decision that order was never evidence for.
func anchoredSpelling(entries []NameSpaceEntry, aggregate bool, sourceName string) (NameSpaceEntry, bool) {
	wanted := Canonicalize(sourceName)
	if wanted == "" {
		return NameSpaceEntry{}, false
	}
	var hopped NameSpaceEntry
	found := false
	for _, e := range entries {
		if e.Aggregate != aggregate || Canonicalize(e.Name) != wanted {
			continue
		}
		if e.AcceptedInSpace() {
			return e, true
		}
		target, ok := acceptedEntryNamed(entries, aggregate, e.AcceptedName)
		if !ok {
			continue
		}
		if found && target.ExtID != hopped.ExtID {
			return NameSpaceEntry{}, false
		}
		hopped, found = target, true
	}
	return hopped, found
}

// acceptedEntryNamed finds the entry the space accepts under name, which is
// the far end of anchoredSpelling's synonymy hop. It insists on
// AcceptedInSpace: the hop exists to reach the space's accepted spelling, and
// landing on a second synonym would answer the question with the thing it was
// asked to resolve.
func acceptedEntryNamed(entries []NameSpaceEntry, aggregate bool, name string) (NameSpaceEntry, bool) {
	wanted := Canonicalize(name)
	if wanted == "" {
		return NameSpaceEntry{}, false
	}
	for _, e := range entries {
		if e.Aggregate == aggregate && e.AcceptedInSpace() && Canonicalize(e.Name) == wanted {
			return e, true
		}
	}
	return NameSpaceEntry{}, false
}

// pickSpelling returns the entry to report among those matching aggregate,
// preferring the space's ACCEPTED spelling.
//
// The preference is the whole point: a space maps many names onto one concept,
// so without it the answer is whichever row the store happened to return
// first. Falling back to the first match when no entry is marked accepted
// keeps an index ingested before Status was recorded working exactly as
// before, rather than reporting nothing.
//
// Among several accepted entries, a DIRECT one — its Resolution does not
// carry ResolutionSourceSynonymyClosure — outranks a CLOSED one (attached by
// application.closeSynonymyGroups' post-resolve pass, spec 2026-09-13
// decision 3), even when the closed entry's ext_id would otherwise sort
// first. This is a real defect the closure pass exposed rather than caused:
// a concept can carry two or more accepted-in-space entries already (2.732
// such concepts measured on the real index BEFORE the closure pass; it adds
// more), and the prior plain ext_id order let a closure-attached entry — one
// concept_name link inferred from its OWN group, not from THIS row's
// spelling — outrank an entry the crosswalk matched onto this concept
// DIRECTLY by name. Direct name evidence must win; within either tier the
// existing ext_id order (the order entries already arrives in) is
// unchanged. See whole-branch review 2026-09-13, I3.
//
// RANK CONGRUENCE (see rankCongruence) subdivides each of those tiers, below
// the accepted/direct distinctions rather than above them. It has to be
// below: a space's accepted spelling is a nomenclatural fact, while
// congruence only separates candidates the earlier rules left tied — which
// is exactly the Bromus erectus case, where Euro+Med accepts the species AND
// nine of its subspecies and ext_id order decided among them.
func pickSpelling(entries []NameSpaceEntry, aggregate bool, sourceRank Rank) (NameSpaceEntry, bool) {
	var best NameSpaceEntry
	bestTier, found := 0, false
	for _, e := range entries {
		if e.Aggregate != aggregate {
			continue
		}
		tier := spellingTier(e, rankCongruence(sourceRank, e.Rank))
		if !found || tier < bestTier {
			best, bestTier, found = e, tier, true
		}
	}
	return best, found
}

// spellingTier scores one candidate spelling: lower wins, and ties keep the
// order entries arrived in (pickSpelling only replaces on a STRICTLY better
// tier).
//
// Two independent preferences, applied in this order: the nomenclatural one
// (accepted over not, and within accepted, directly matched over
// closure-attached) and then the rank one. The rank preference is three-valued
// rather than a flag — see rankCongruence for why an unknown rank must NOT tie
// with a known-matching one — so each nomenclatural class spans three
// consecutive scores and the arithmetic keeps that readable instead of
// enumerating nine constants.
func spellingTier(e NameSpaceEntry, congruence rankCongruenceClass) int {
	const (
		classAcceptedDirect = iota
		classAcceptedClosed
		classOther
	)
	class := classOther
	if e.AcceptedInSpace() {
		class = classAcceptedDirect
		if strings.Contains(e.Resolution, ResolutionSourceSynonymyClosure) {
			class = classAcceptedClosed
		}
	}
	return class*3 + int(congruence)
}

// rankCongruenceClass orders candidates by what their rank says about them:
// evidence that matches, no evidence, evidence that contradicts. The values
// are used as an offset by spellingTier, so the order of the constants IS the
// preference.
type rankCongruenceClass int

const (
	rankCongruenceMatches rankCongruenceClass = iota
	rankCongruenceUnknown
	rankCongruenceConflicts
)

// rankCongruence reports how an entry's rank relates to the rank of the
// concept being translated.
//
// Compatibility is decided in CLASSES, never by equality. Two checklists
// disagreeing on whether a taxon is a subspecies or a variety is a taxonomic
// opinion, not an error, and demanding equality would reject the only sensible
// spelling on offer. What must not happen is a species resolving to one of its
// own subspecies — a different taxon, silently narrower than what the caller
// matched.
//
// An unknown rank on the ENTRY — the zero value, or RankOther, which
// ParseRankLenient returns for the empty string and for the long tail of
// exotic ranks — is its own class, deliberately NOT folded into "matches".
// Reporting it as a match reads like standing the rule down, but does
// something else: it puts an entry carrying no evidence in the same preferred
// tier as one whose rank is known to fit, so ext_id order decides between them
// again. Mixed pools are a real state — a legacy row has no rank until
// re-ingest, and only one space may have been re-ingested — and in one of
// those the rank-less row would beat a known-fitting one just by sorting
// first. It still outranks a known mismatch, so a wholly legacy pool keeps
// answering exactly as it did.
//
// An unknown SOURCE rank is different: there is nothing to be congruent with,
// so every entry reports Unknown and none is preferred on rank grounds.
// Separating them would present the absence of a comparison basis as a
// judgement.
func rankCongruence(source, entry Rank) rankCongruenceClass {
	sc := rankClassOf(source)
	if sc == rankClassUnknown {
		return rankCongruenceUnknown
	}
	ec := rankClassOf(entry)
	switch {
	case ec == rankClassUnknown:
		return rankCongruenceUnknown
	case sc == ec:
		return rankCongruenceMatches
	default:
		return rankCongruenceConflicts
	}
}

// rankClass is the coarse grouping rankCongruence compares in: everything
// finer than the species, the species itself, and everything above it.
type rankClass int

const (
	rankClassUnknown rankClass = iota
	rankClassInfraspecific
	rankClassSpecies
	rankClassSupraspecific
)

// rankClasses maps every Rank with a settled position relative to the species
// to its class. A lookup table rather than a switch for the same reason
// canonicalRanks is one — and because an exhaustive switch over Rank would
// have to name all ~45 constants to satisfy the linter, which buys nothing
// here: a rank absent from these tables is exactly the "unknown" case
// rankClassOf already has an answer for.
//
// The collective ranks (SPECIES_AGGREGATE, COLL_SPECIES) count as
// species-level: an aggregate stands in for a species, never for a subspecies
// of one. Which spelling an aggregate QUERY gets is a separate question,
// answered by ResolveTargetSpace's aggregate branch before this ever runs.
//
// RankOther and the empty Rank are deliberately in NEITHER table: both mean
// the rank is not determinable, and a lookup miss is how rankClassOf reports
// that.
var speciesRanks = map[Rank]struct{}{
	RankSpecies: {}, RankSpeciesAggregate: {}, RankCollSpecies: {},
}

var infraspecificRanks = map[Rank]struct{}{
	RankSubspecies: {}, RankVariety: {}, RankSubvariety: {},
	RankForm: {}, RankSubform: {},
	RankNothosubspecies: {}, RankNothovariety: {}, RankNothoform: {},
	RankSubspeciesGroup: {}, RankProles: {}, RankRace: {}, RankConvar: {},
	RankGrex: {}, RankUnrankedInfraspecific: {},
}

var supraspecificRanks = map[Rank]struct{}{
	RankGenus: {}, RankGenusAggregate: {}, RankSubgenus: {},
	RankSection: {}, RankSubsection: {}, RankSeries: {},
	RankUnrankedInfrageneric: {},
	RankFamily:               {}, RankSubfamily: {}, RankTribe: {},
	RankOrder: {}, RankSuperorder: {}, RankClass: {}, RankSubclass: {},
	RankPhylum: {}, RankSubdivision: {}, RankInformalClade: {}, RankRoot: {},
}

func rankClassOf(r Rank) rankClass {
	if _, ok := speciesRanks[r]; ok {
		return rankClassSpecies
	}
	if _, ok := infraspecificRanks[r]; ok {
		return rankClassInfraspecific
	}
	if _, ok := supraspecificRanks[r]; ok {
		return rankClassSupraspecific
	}
	return rankClassUnknown
}

// IsAggregateName reports whether a verbatim name denotes an AGGREGATE — a
// collective species wider than a single taxon ("Festuca ovina aggr.",
// "Festuca ovina s. l.").
//
// It is deliberately a thin predicate over AggregateBases rather than its own
// marker list: AggregateBases already owns the measured set of trailing
// markers (and the reasoning for what is excluded from it — "s. str."
// NARROWS and is not an aggregate marker), and a second, independently
// maintained list would be exactly the duplicated-mapper defect class this
// milestone has already hit twice. A name carries an aggregate marker
// precisely when peeling markers off it yields at least one base.
func IsAggregateName(name string) bool {
	return len(AggregateBases(Canonicalize(name))) > 0
}

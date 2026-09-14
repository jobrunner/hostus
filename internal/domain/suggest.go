package domain

import "sort"

// SuggestItem is a single autosuggest candidate, combining the taxon
// identity (ConceptID/Canonical/Display/VernacularDE/Rank/Status) with the
// area and relevance signals used to rank it against other candidates.
//
// Score is the raw SQLite FTS5 bm25() value for the match: bm25 is a
// distance-like relevance measure where LOWER is MORE relevant (the
// opposite sign convention from cosine similarity or most "score" fields).
// RankSuggestions therefore sorts ascending on Score, and callers must not
// flip that sign when constructing SuggestItem.
type SuggestItem struct {
	ConceptID    string
	Canonical    string
	Display      string
	VernacularDE string
	Rank         Rank
	Status       Status
	InArea       bool
	// ExactHit is true when the concept carries a name whose canonicalized
	// form EQUALS the canonicalized query (not merely starts with it). It is
	// the match_mode-independent signal PrefixHit can't be once match_mode
	// widens beyond name_start: a caller who typed the full name wants no
	// longer match ranked ahead of it.
	ExactHit bool
	// TargetSpaceHit is true when a SuggestOpts.TargetSpace was requested
	// AND this concept's name in that space (see TargetSpaceName) matches
	// the query (exact, else prefix). Without a requested target_space it
	// is false for every item and therefore has no effect on ordering —
	// there is nothing for it to prefer. It is the decisive criterion for
	// the "Inula hirta" homonym: Inula hirta L. (a WCVP synonym of
	// Pentanema hirtum) IS the eurosl name, while Inula hirta Pollich (a
	// synonym of Pentanema britannica) matches only in WCVP — britannica's
	// eurosl name is "Inula britannica". Both concepts are taxonomically
	// correct hits; this only decides which one serves the caller's
	// declared target space first.
	TargetSpaceHit bool
	PrefixHit      bool
	Score          float64
	// Aggregate is true when this concept was reached via an AGGREGATE
	// name-space alias (e.g. FloraVeg's "Achillea millefolium aggr."), so a
	// client can badge the hit as an aggregate. It is MAX(is_aggregate) over
	// the matched names for the concept: a query that matched only the
	// concept's own (non-aggregate) name leaves it false.
	Aggregate bool
	// SecReference is the concept's sec. reference space id, or "" for a
	// concept with none (WCVP). Its presence lets a caller tell two same-name
	// CDM concepts apart (SP5); the HTTP layer resolves the title from it.
	SecReference string
	// TargetSpaceName is this concept's spelling in the requested
	// SuggestOpts.TargetSpace, or "" when no space was requested OR the
	// concept has no entry in it. The two are not distinguished on purpose:
	// either way there is no name to offer, and a caller that asked for a
	// space knows which case it is in.
	//
	// It answers "can I use this concept downstream in that space?" while
	// choosing, rather than one concept at a time afterwards.
	TargetSpaceName string
	// MatchedName is the name that triggered this item's hit — the source
	// of the "Inula hirta L." vs. "Inula hirta Pollich" distinction that
	// Canonical/Display (the concept's accepted name) cannot show for a
	// homonym-driven synonym match. When several of the concept's names
	// match, the same precedence as RankSuggestions applies: exact before
	// prefix, accepted before synonym.
	MatchedName MatchedName
}

// MatchedName identifies which of a concept's names caused a suggest hit,
// distinct from the concept's own accepted name (SuggestItem.Canonical).
// Canonical is empty when the triggering name could not be determined by
// the caller constructing SuggestItem.
type MatchedName struct {
	Canonical  string
	Authorship string
	// Role is "accepted" or "synonym".
	Role string
}

// rankOrder assigns the ordinal used by RankOrderPriority/RankSuggestions
// priority step 6 (see RankSuggestions' doc comment): species before
// subspecies before variety before form, with
// FAMILY and GENUS ranked ahead of all of those (broader ranks first). The
// nothotaxon (hybrid) ranks are placed directly after their non-hybrid
// counterpart (nothosubsp. after subspecies, nothovar. after subvariety,
// nothof. after subform) — WCVP doesn't define a relative order between a
// rank and its nothotaxon sibling, so this is a deliberate, documented
// choice rather than a measured requirement.
//
// The 26 EuroSL/GermanSL ranks (Task 1's extended canonical rank set) are
// interleaved by taxonomic generality rather than appended at the end:
//   - ROOT..ORDER sit ahead of FAMILY (more general than family);
//   - SUBFAMILY/TRIBE sit between FAMILY and GENUS;
//   - the collective/infrageneric ranks (SUBGENUS, SECTION, SUBSECTION,
//     SERIES, SPECIES_AGGREGATE, GENUS_AGGREGATE) sit between GENUS and
//     SPECIES — they are taxonomically broader than a species but narrower
//     than a genus;
//   - the remaining infraspecific ranks (COLL_SPECIES, SUBSPECIES_GROUP,
//     PROLES, RACE, CONVAR, GREX, UNRANKED_INFRAGENERIC,
//     UNRANKED_INFRASPECIFIC) sit after the existing infraspecies chain
//     (SUBFORM/NOTHOFORM), since they are all finer-or-equal-to subspecies
//     but the source data gives no sharper relative order among them.
//
// RankOther (and any other unrecognized Rank) is placed last, after all of
// the above, so it never masquerades as a real rank or intrudes elsewhere
// in the ordering.
var rankOrder = map[Rank]int{
	RankRoot:          0,
	RankPhylum:        1,
	RankSubdivision:   2,
	RankInformalClade: 3,
	RankClass:         4,
	RankSubclass:      5,
	RankSuperorder:    6,
	RankOrder:         7,

	RankFamily:    8,
	RankSubfamily: 9,
	RankTribe:     10,

	RankGenus:            11,
	RankSubgenus:         12,
	RankSection:          13,
	RankSubsection:       14,
	RankSeries:           15,
	RankSpeciesAggregate: 16,
	RankGenusAggregate:   17,

	RankSpecies:         18,
	RankSubspecies:      19,
	RankNothosubspecies: 20,
	RankVariety:         21,
	RankSubvariety:      22,
	RankNothovariety:    23,
	RankForm:            24,
	RankSubform:         25,
	RankNothoform:       26,

	RankCollSpecies:           27,
	RankSubspeciesGroup:       28,
	RankProles:                29,
	RankRace:                  30,
	RankConvar:                31,
	RankGrex:                  32,
	RankUnrankedInfrageneric:  33,
	RankUnrankedInfraspecific: 34,
}

const unknownRankOrder = 35

// RankOrderPriority returns the ordinal used to compare Ranks for suggest
// ranking (RankSuggestions' priority step 6): the general-to-specific
// ordering documented on rankOrder above, with RankOther/any unrecognized
// Rank sorting after all of them (unknownRankOrder).
func RankOrderPriority(r Rank) int {
	if order, ok := rankOrder[r]; ok {
		return order
	}
	return unknownRankOrder
}

// RankSuggestions returns a new, stably-sorted copy of items ordered by the
// autosuggest priority (originally §B.1, extended by the 2026-09-14
// suggest-filter-und-ranking spec with the two leading criteria below),
// highest priority first:
//
//  1. ExactHit true before false — the concept carries a name whose
//     canonicalized form equals the canonicalized query, not merely a
//     prefix of it
//  2. TargetSpaceHit true before false — only meaningful when a
//     SuggestOpts.TargetSpace was requested; without one it is false for
//     every item and this key is a no-op
//  3. PrefixHit true before false
//  4. InArea true before false
//  5. Status == StatusAccepted before any other status
//  6. lower RankOrderPriority first (broader/simpler ranks before finer ones)
//  7. Score ascending (bm25: lower Score means more relevant — see
//     SuggestItem's doc comment on the sign convention)
//
// Items that compare equal on every key above keep their relative input
// order (sort.SliceStable). RankSuggestions is pure: it does not mutate
// its input slice.
func RankSuggestions(items []SuggestItem) []SuggestItem {
	out := make([]SuggestItem, len(items))
	copy(out, items)

	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]

		if a.ExactHit != b.ExactHit {
			return a.ExactHit
		}
		if a.TargetSpaceHit != b.TargetSpaceHit {
			return a.TargetSpaceHit
		}
		if a.PrefixHit != b.PrefixHit {
			return a.PrefixHit
		}
		if a.InArea != b.InArea {
			return a.InArea
		}
		aAccepted := a.Status == StatusAccepted
		bAccepted := b.Status == StatusAccepted
		if aAccepted != bAccepted {
			return aAccepted
		}
		if ao, bo := RankOrderPriority(a.Rank), RankOrderPriority(b.Rank); ao != bo {
			return ao < bo
		}
		return a.Score < b.Score
	})

	return out
}

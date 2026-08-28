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
	PrefixHit    bool
	Score        float64
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
}

// rankOrder assigns the ordinal used by RankOrderPriority/RankSuggestions priority
// step 4: species before subspecies before variety before form, with
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
// ranking (§B.1 step 4): the general-to-specific ordering documented on
// rankOrder above, with RankOther/any unrecognized Rank sorting after all
// of them (unknownRankOrder).
func RankOrderPriority(r Rank) int {
	if order, ok := rankOrder[r]; ok {
		return order
	}
	return unknownRankOrder
}

// RankSuggestions returns a new, stably-sorted copy of items ordered by the
// §B.1 autosuggest priority, highest priority first:
//
//  1. PrefixHit true before false
//  2. InArea true before false
//  3. Status == StatusAccepted before any other status
//  4. lower RankOrderPriority first (broader/simpler ranks before finer ones)
//  5. Score ascending (bm25: lower Score means more relevant — see
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

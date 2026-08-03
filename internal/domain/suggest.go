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
}

// rankOrder assigns the ordinal used by RankOrder/RankSuggestions priority
// step 4: species before subspecies before variety before form, with
// FAMILY and GENUS ranked ahead of all of those (broader ranks first).
// Unknown ranks are placed last, after FORM, so an unrecognized Rank never
// masquerades as FAMILY (ordinal 0) or intrudes elsewhere in the ordering.
var rankOrder = map[Rank]int{
	RankFamily:     0,
	RankGenus:      1,
	RankSpecies:    2,
	RankSubspecies: 3,
	RankVariety:    4,
	RankForm:       5,
}

const unknownRankOrder = 6

// RankOrder returns the ordinal used to compare Ranks for suggest ranking
// (§B.1 step 4): FAMILY(0) < GENUS(1) < SPECIES(2) < SUBSPECIES(3) <
// VARIETY(4) < FORM(5). An unrecognized Rank returns an ordinal after FORM.
func RankOrder(r Rank) int {
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
//  4. lower RankOrder first (broader/simpler ranks before finer ones)
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
		if ao, bo := RankOrder(a.Rank), RankOrder(b.Rank); ao != bo {
			return ao < bo
		}
		return a.Score < b.Score
	})

	return out
}

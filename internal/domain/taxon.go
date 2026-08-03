package domain

import (
	"fmt"
	"strings"
)

// Rank is a taxonomic rank, spelled per the WCVP "taxonrank" column.
type Rank string

const (
	RankFamily     Rank = "FAMILY"
	RankGenus      Rank = "GENUS"
	RankSpecies    Rank = "SPECIES"
	RankSubspecies Rank = "SUBSPECIES"
	RankVariety    Rank = "VARIETY"
	RankForm       Rank = "FORM"
)

// ParseRank maps a WCVP "taxonrank" spelling (case-insensitive) to a Rank.
// Unknown or empty input returns an error.
func ParseRank(s string) (Rank, error) {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "FAMILY":
		return RankFamily, nil
	case "GENUS":
		return RankGenus, nil
	case "SPECIES":
		return RankSpecies, nil
	case "SUBSPECIES":
		return RankSubspecies, nil
	case "VARIETY":
		return RankVariety, nil
	case "FORM":
		return RankForm, nil
	default:
		return "", fmt.Errorf("domain: unknown taxon rank %q", s)
	}
}

// Status is the taxonomic status of a Concept (accepted, synonym, ...).
type Status string

const (
	StatusAccepted Status = "ACCEPTED"
	StatusSynonym  Status = "SYNONYM"
	StatusUnplaced Status = "UNPLACED"
	StatusUnknown  Status = "UNKNOWN"
)

// ParseStatus maps a WCVP/GBIF status spelling (case-insensitive) to a
// Status. Unrecognized input maps to StatusUnknown rather than failing,
// since status is descriptive metadata, not a validity gate.
func ParseStatus(s string) Status {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "ACCEPTED":
		return StatusAccepted
	case "SYNONYM":
		return StatusSynonym
	case "UNPLACED":
		return StatusUnplaced
	default:
		return StatusUnknown
	}
}

// Name is a scientific name record: a canonical name plus its authorship
// and bibliographic/nomenclatural metadata.
type Name struct {
	ID          string
	Canonical   string
	Authorship  string
	Rank        Rank
	IPNIID      string
	PublishedIn string
	NomStatus   string
	BasionymID  string
}

// Concept is a taxonomic concept: an accepted name plus its placement and
// the secundum (sec.) reference that scopes the circumscription.
type Concept struct {
	ID              string
	BackboneID      string
	BackboneVersion string
	AcceptedName    Name
	Rank            Rank
	ParentID        string
	SecReference    string
	Status          Status
}

// Xref is a cross-reference to a name or concept in an external authority
// (e.g. GBIF backbone, IPNI).
type Xref struct {
	Authority string
	ExtID     string
}

// Distribution is a single area assignment for a Concept, keyed by the
// area-coding scheme in use (e.g. WGSRPD level 3).
type Distribution struct {
	AreaScheme string
	AreaCode   string
}

// Canonicalize normalizes a scientific name (or name fragment) into a
// comparison key: it trims leading/trailing whitespace, collapses internal
// whitespace runs to a single space, lower-cases, and strips diacritics.
// This mirrors the SQLite FTS5 "unicode61 remove_diacritics 2" tokenizer so
// keys computed here line up with the ones the database computes at index
// time. It does NOT strip punctuation or hyphens, and it does NOT touch
// distinct letters/words — "otites" and "otitis" remain distinct.
func Canonicalize(name string) string {
	fields := strings.Fields(name)
	joined := strings.ToLower(strings.Join(fields, " "))

	var b strings.Builder
	b.Grow(len(joined))
	for _, r := range joined {
		if plain, ok := diacriticFold[r]; ok {
			b.WriteRune(plain)
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// diacriticFold maps common Latin letters carrying diacritics (as used in
// botanical author names and place names) to their base ASCII letter. This
// is a fixed table rather than a full Unicode-normalization dependency,
// keeping internal/domain free of third-party imports per CLAUDE.md's
// "Allowed Libraries Only" constraint. It covers the Latin-1 Supplement and
// the common Latin Extended-A range; it is not a complete Unicode
// decomposition.
//
// It deliberately EXCLUDES 'ß', 'ł', 'ø', 'đ': these are not base+combining-
// mark decompositions in Unicode (there is no plain "s"/"l"/"o"/"d" +
// diacritic to strip), so SQLite's FTS5 `unicode61 remove_diacritics 2`
// tokenizer — used to index fts_name (see adapters/sqlite/schema.sql) — does
// NOT fold them either (verified empirically; see
// internal/adapters/sqlite/fts_parity_test.go). Folding them here anyway
// would make Canonicalize's comparison keys diverge from the ones the FTS5
// index computes at query time, breaking exact-match lookups keyed by
// canonical name. Confirmed via the same probe: every OTHER letter in this
// table (a/e/i/u/y/n/c/z/r/t/g families and 'ď') does fold under unicode61
// remove_diacritics=2, matching this table's coverage.
var diacriticFold = map[rune]rune{
	'à': 'a', 'á': 'a', 'â': 'a', 'ã': 'a', 'ä': 'a', 'å': 'a', 'ā': 'a', 'ă': 'a', 'ą': 'a',
	'è': 'e', 'é': 'e', 'ê': 'e', 'ë': 'e', 'ē': 'e', 'ĕ': 'e', 'ė': 'e', 'ę': 'e', 'ě': 'e',
	'ì': 'i', 'í': 'i', 'î': 'i', 'ï': 'i', 'ĩ': 'i', 'ī': 'i', 'ĭ': 'i', 'į': 'i',
	'ò': 'o', 'ó': 'o', 'ô': 'o', 'õ': 'o', 'ö': 'o', 'ō': 'o', 'ŏ': 'o', 'ő': 'o',
	'ù': 'u', 'ú': 'u', 'û': 'u', 'ü': 'u', 'ũ': 'u', 'ū': 'u', 'ŭ': 'u', 'ů': 'u', 'ű': 'u', 'ų': 'u',
	'ý': 'y', 'ÿ': 'y',
	'ñ': 'n', 'ń': 'n', 'ņ': 'n', 'ň': 'n',
	'ç': 'c', 'ć': 'c', 'ĉ': 'c', 'ċ': 'c', 'č': 'c',
	'ś': 's', 'ŝ': 's', 'ş': 's', 'š': 's',
	'ź': 'z', 'ż': 'z', 'ž': 'z',
	'ĺ': 'l', 'ļ': 'l', 'ľ': 'l',
	'ř': 'r', 'ŕ': 'r', 'ŗ': 'r',
	'ď': 'd',
	'ť': 't', 'ţ': 't',
	'ğ': 'g', 'ģ': 'g',
}

// NormalizeAuthor normalizes a botanical author-citation string for
// comparison purposes. It:
//   - trims leading/trailing whitespace and collapses internal whitespace
//     runs to a single space,
//   - removes a space directly before a '.' (e.g. "L ." -> "L."),
//   - normalizes '&' to be surrounded by exactly one space on each side,
//     regardless of the spacing (or lack of it) around it in the input
//     (e.g. "L.&Beauv." -> "L. & Beauv.").
//
// It does NOT insert missing separators otherwise (e.g. "(L.)P.Beauv." is
// left as-is — it does not become "(L.) P.Beauv."), does NOT expand
// abbreviations (e.g. it does not turn "L." into "Linnaeus"), does NOT fix
// misspellings, and does NOT reorder multiple authors. Comparisons of
// NormalizeAuthor output are still case-sensitive on purpose: author
// citations carry meaningful capitalization.
func NormalizeAuthor(a string) string {
	s := strings.Join(strings.Fields(a), " ")
	if s == "" {
		return s
	}
	s = strings.ReplaceAll(s, " .", ".")
	return normalizeAmpersandSpacing(s)
}

func normalizeAmpersandSpacing(s string) string {
	parts := strings.Split(s, "&")
	if len(parts) == 1 {
		return s
	}
	for i, p := range parts {
		parts[i] = strings.TrimSpace(p)
	}
	return strings.Join(parts, " & ")
}

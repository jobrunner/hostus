package domain

import (
	"fmt"
	"strings"
)

// TraitDim is an ecological indicator dimension, spanning both the EIVE/
// Tichý Ellenberg-type indicator dimensions (moisture, nutrients, reaction,
// light, temperature, salinity) and the Midolo disturbance-indicator
// dimensions. Not every TraitDim is meaningful for every TraitVocab — see
// ScaleFor.
type TraitDim string

const (
	// DimM is moisture (EIVE, Tichý).
	DimM TraitDim = "M"
	// DimN is nutrients (EIVE, Tichý).
	DimN TraitDim = "N"
	// DimR is reaction / soil pH (EIVE, Tichý).
	DimR TraitDim = "R"
	// DimL is light (EIVE, Tichý).
	DimL TraitDim = "L"
	// DimT is temperature (EIVE, Tichý).
	DimT TraitDim = "T"
	// DimS is salinity. Tichý-only: EIVE has no salinity dimension.
	DimS TraitDim = "S"

	// DimDisturbanceSeverity is the Midolo whole-community/herb-layer
	// disturbance severity indicator.
	DimDisturbanceSeverity TraitDim = "disturbance_severity"
	// DimDisturbanceFrequency is the Midolo disturbance frequency indicator.
	DimDisturbanceFrequency TraitDim = "disturbance_frequency"
	// DimMowingFrequency is a Midolo derived disturbance indicator.
	DimMowingFrequency TraitDim = "mowing_frequency"
	// DimGrazingPressure is a Midolo derived disturbance indicator.
	DimGrazingPressure TraitDim = "grazing_pressure"
	// DimSoilDisturbance is a Midolo derived disturbance indicator.
	DimSoilDisturbance TraitDim = "soil_disturbance"
)

// ParseTraitDim maps a trait dimension spelling (case-insensitive, leading/
// trailing whitespace ignored) to a TraitDim. Unknown or empty input
// returns an error.
func ParseTraitDim(s string) (TraitDim, error) {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "M":
		return DimM, nil
	case "N":
		return DimN, nil
	case "R":
		return DimR, nil
	case "L":
		return DimL, nil
	case "T":
		return DimT, nil
	case "S":
		return DimS, nil
	case "DISTURBANCE_SEVERITY":
		return DimDisturbanceSeverity, nil
	case "DISTURBANCE_FREQUENCY":
		return DimDisturbanceFrequency, nil
	case "MOWING_FREQUENCY":
		return DimMowingFrequency, nil
	case "GRAZING_PRESSURE":
		return DimGrazingPressure, nil
	case "SOIL_DISTURBANCE":
		return DimSoilDisturbance, nil
	default:
		return "", fmt.Errorf("domain: unknown trait dimension %q", s)
	}
}

// TraitVocab identifies one of the three trait vocabularies hostus 2.0
// ingests: EIVE 1.0, Tichý et al. 2023, and Midolo et al. 2023. See
// poc/P06-findings.md for the format/scale details each constant refers to.
type TraitVocab string

const (
	// VocabEIVE is EIVE 1.0: dims M/N/R/L/T, uniform continuous 0-10,
	// with per-dimension niche width and source-system count.
	VocabEIVE TraitVocab = "eive"
	// VocabTichy is Tichý et al. 2023: dims L/T/M/R/N plus Salinity, on
	// varying Ellenberg-compatible (not normalized) scales.
	VocabTichy TraitVocab = "tichy2023"
	// VocabMidolo is Midolo et al. 2023: disturbance-indicator dims,
	// continuous with no fixed min/max scale.
	VocabMidolo TraitVocab = "midolo2023"
)

// ParseTraitVocab maps a vocabulary spelling (case-insensitive, leading/
// trailing whitespace ignored) to a TraitVocab. Unknown or empty input
// returns an error.
func ParseTraitVocab(s string) (TraitVocab, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case string(VocabEIVE):
		return VocabEIVE, nil
	case string(VocabTichy):
		return VocabTichy, nil
	case string(VocabMidolo):
		return VocabMidolo, nil
	default:
		return "", fmt.Errorf("domain: unknown trait vocabulary %q", s)
	}
}

// TraitValue is one indicator value for one taxon in one dimension of one
// vocabulary.
//
// NicheWidth and NSystems are pointers on purpose: EIVE's niche-width
// (".nw3") and source-system count (".n") columns do not exist in Tichý or
// Midolo (poc/P06-findings.md). A nil pointer means "this vocabulary does
// not provide this datum"; a non-nil pointer to 0.0/0 means the vocabulary
// provided it and the value happens to be zero. Callers must never treat
// these as interchangeable — collapsing nil into 0 would silently invent
// data EIVE-only entries have but Tichý/Midolo entries do not.
type TraitValue struct {
	Vocab        TraitVocab
	VocabVersion string
	Dim          TraitDim
	Value        float64
	NicheWidth   *float64
	NSystems     *int
}

// TraitSet groups all TraitValue entries hostus holds for one taxon in one
// vocabulary.
//
// Taxonomy records the namespace the values were harmonized against
// ("euromed-aligned" for EIVE, "floraveg-eunis-aligned" for Tichý/Midolo).
// PoC P10 found these namespaces genuinely diverge, so callers must key
// joins/merges on Taxonomy and never combine TraitSets across different
// Taxonomy values as if they described the same taxon concept.
type TraitSet struct {
	Vocab        TraitVocab
	VocabVersion string
	Taxonomy     string
	Values       []TraitValue
}

// ScaleFor reports the value range and normalization state of one
// (vocabulary, dimension) combination, so callers can tell whether two
// TraitValue.Value numbers are even comparable. It is the guard against
// silently treating a EIVE value of 4.2 as equivalent to a Tichý value of
// 4.2: they are on different scales even though the field type is the same
// float64.
//
//   - EIVE M/N/R/L/T:                       (0, 10, true)  — uniform, normalized.
//   - Tichý L/T/M/R/N:                      (1, 9, false)  — classic Ellenberg-
//     compatible range, NOT normalized.
//   - Tichý S (Salinity):                   (-3, 3, false) — a documented
//     default range (Salinity is a signed scale centered on 0;
//     poc/P06-findings.md observed roughly -0.02 to 0 in the sampled sheet
//     but did not exhaustively confirm the full range). The authoritative
//     range for any given value is the one the ingest pipeline records
//     from the source data; this is only a fallback default, deliberately
//     distinct from Midolo's (0,0) "no fixed scale" sentinel below.
//   - Midolo disturbance dims:              (0, 0, false) — continuous,
//     unbounded, no fixed scale. (0, 0, false) is the sentinel for "no
//     fixed scale", not "the value is confined to exactly zero".
//   - Any other (vocab, dim) combination     that is not meaningful for that
//     (e.g. Midolo+M, EIVE+S, EIVE+a          vocabulary (the vocabulary simply
//     disturbance dim):                      does not define that dimension) —
//     the same (0, 0, false) "no fixed scale" sentinel, handled explicitly
//     rather than falling through to a zero value by accident.
func ScaleFor(v TraitVocab, d TraitDim) (min, max float64, normalized bool) {
	switch v {
	case VocabEIVE:
		switch d {
		case DimM, DimN, DimR, DimL, DimT:
			return 0, 10, true
		case DimS, DimDisturbanceSeverity, DimDisturbanceFrequency, DimMowingFrequency, DimGrazingPressure, DimSoilDisturbance:
			// EIVE defines none of these dimensions.
			return 0, 0, false
		}
	case VocabTichy:
		switch d {
		case DimL, DimT, DimM, DimR, DimN:
			return 1, 9, false
		case DimS:
			// Documented fallback default; see the ScaleFor doc comment
			// above for why -3/3 and why this is not the (0,0) sentinel.
			return -3, 3, false
		case DimDisturbanceSeverity, DimDisturbanceFrequency, DimMowingFrequency, DimGrazingPressure, DimSoilDisturbance:
			// Tichý defines none of the Midolo disturbance dimensions.
			return 0, 0, false
		}
	case VocabMidolo:
		switch d {
		case DimDisturbanceSeverity, DimDisturbanceFrequency, DimMowingFrequency, DimGrazingPressure, DimSoilDisturbance:
			return 0, 0, false
		case DimM, DimN, DimR, DimL, DimT, DimS:
			// Midolo defines none of the EIVE/Tichý indicator dimensions.
			return 0, 0, false
		}
	}
	return 0, 0, false
}

package domain

import "testing"

func TestParseTraitDim(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		want    TraitDim
		wantErr bool
	}{
		{"moisture", "M", DimM, false},
		{"nutrients", "N", DimN, false},
		{"reaction", "R", DimR, false},
		{"light", "L", DimL, false},
		{"temperature", "T", DimT, false},
		{"salinity", "S", DimS, false},
		{"disturbance severity", "disturbance_severity", DimDisturbanceSeverity, false},
		{"disturbance frequency", "disturbance_frequency", DimDisturbanceFrequency, false},
		{"mowing frequency", "mowing_frequency", DimMowingFrequency, false},
		{"grazing pressure", "grazing_pressure", DimGrazingPressure, false},
		{"soil disturbance", "soil_disturbance", DimSoilDisturbance, false},
		{"lowercase m", "m", DimM, false},
		{"mixed case disturbance", "Disturbance_Severity", DimDisturbanceSeverity, false},
		{"whitespace padded", "  L  ", DimL, false},
		{"unknown", "Q", "", true},
		{"empty", "", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseTraitDim(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("ParseTraitDim(%q) = %q, nil; want error", tc.in, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseTraitDim(%q) unexpected error: %v", tc.in, err)
			}
			if got != tc.want {
				t.Fatalf("ParseTraitDim(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestParseTraitVocab(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		want    TraitVocab
		wantErr bool
	}{
		{"eive", "eive", VocabEIVE, false},
		{"tichy", "tichy2023", VocabTichy, false},
		{"midolo", "midolo2023", VocabMidolo, false},
		{"uppercase eive", "EIVE", VocabEIVE, false},
		{"mixed case tichy", "Tichy2023", VocabTichy, false},
		{"whitespace padded", "  midolo2023  ", VocabMidolo, false},
		{"unknown", "gbif", "", true},
		{"empty", "", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseTraitVocab(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("ParseTraitVocab(%q) = %q, nil; want error", tc.in, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseTraitVocab(%q) unexpected error: %v", tc.in, err)
			}
			if got != tc.want {
				t.Fatalf("ParseTraitVocab(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestScaleFor_EIVE(t *testing.T) {
	for _, d := range []TraitDim{DimM, DimN, DimR, DimL, DimT} {
		min, max, normalized := ScaleFor(VocabEIVE, d)
		if min != 0 || max != 10 || !normalized {
			t.Fatalf("ScaleFor(EIVE, %q) = (%v,%v,%v), want (0,10,true)", d, min, max, normalized)
		}
	}
}

func TestScaleFor_Tichy(t *testing.T) {
	for _, d := range []TraitDim{DimL, DimT, DimM, DimR, DimN} {
		min, max, normalized := ScaleFor(VocabTichy, d)
		if min != 1 || max != 9 || normalized {
			t.Fatalf("ScaleFor(Tichy, %q) = (%v,%v,%v), want (1,9,false)", d, min, max, normalized)
		}
	}
}

func TestScaleFor_TichySalinity(t *testing.T) {
	// PoC P6 only observed a narrow sample range (roughly -0.02 to 0) for
	// Tichý Salinity and did not confirm the full range, so ScaleFor must
	// NOT invent a numeric range here — it reports the same (0,0,false)
	// "no fixed scale established" sentinel used for Midolo's genuinely
	// unbounded dims. The real range is recorded by the T2 ingest pipeline
	// from the actual data, not hardcoded in this function.
	min, max, normalized := ScaleFor(VocabTichy, DimS)
	if min != 0 || max != 0 || normalized {
		t.Fatalf("ScaleFor(Tichy, S) = (%v,%v,%v), want (0,0,false)", min, max, normalized)
	}
}

func TestScaleFor_Midolo(t *testing.T) {
	for _, d := range []TraitDim{
		DimDisturbanceSeverity, DimDisturbanceFrequency,
		DimMowingFrequency, DimGrazingPressure, DimSoilDisturbance,
	} {
		min, max, normalized := ScaleFor(VocabMidolo, d)
		if min != 0 || max != 0 || normalized {
			t.Fatalf("ScaleFor(Midolo, %q) = (%v,%v,%v), want (0,0,false)", d, min, max, normalized)
		}
	}
}

// EIVE and Tichý must never report the same scale for the same logical
// dimension: this is the guard against silently comparing a 4.2 EIVE value
// with a 4.2 Tichý value as if they meant the same thing.
func TestScaleFor_EIVEAndTichyDiffer(t *testing.T) {
	for _, d := range []TraitDim{DimM, DimN, DimR, DimL, DimT} {
		eMin, eMax, eNorm := ScaleFor(VocabEIVE, d)
		tMin, tMax, tNorm := ScaleFor(VocabTichy, d)
		if eMin == tMin && eMax == tMax && eNorm == tNorm {
			t.Fatalf("ScaleFor(EIVE, %q) and ScaleFor(Tichy, %q) must differ, both got (%v,%v,%v)", d, d, eMin, eMax, eNorm)
		}
	}
}

func TestScaleFor_UnknownCombination(t *testing.T) {
	// An unknown (vocab, dim) pairing (e.g. Midolo has no M/N/R/L/T dims,
	// EIVE has no disturbance dims) must be handled explicitly: it returns
	// the same "no fixed scale" sentinel as Midolo's genuine continuous
	// dims, not a silently wrong number.
	min, max, normalized := ScaleFor(VocabMidolo, DimM)
	if min != 0 || max != 0 || normalized {
		t.Fatalf("ScaleFor(Midolo, M) = (%v,%v,%v), want (0,0,false)", min, max, normalized)
	}

	min, max, normalized = ScaleFor(VocabEIVE, DimDisturbanceSeverity)
	if min != 0 || max != 0 || normalized {
		t.Fatalf("ScaleFor(EIVE, DisturbanceSeverity) = (%v,%v,%v), want (0,0,false)", min, max, normalized)
	}

	min, max, normalized = ScaleFor(VocabEIVE, DimS)
	if min != 0 || max != 0 || normalized {
		t.Fatalf("ScaleFor(EIVE, S) = (%v,%v,%v), want (0,0,false) — EIVE has no salinity dimension", min, max, normalized)
	}
}

// TraitValue's NicheWidth/NSystems are pointers so that "the vocabulary
// does not provide this datum" (nil) is distinguishable from "the value is
// zero" (pointer to 0.0 / 0).
func TestTraitValue_NilPointersDistinguishFromZero(t *testing.T) {
	zero := 0.0
	zeroN := 0

	withZero := TraitValue{Vocab: VocabEIVE, Dim: DimM, Value: 5, NicheWidth: &zero, NSystems: &zeroN}
	withNil := TraitValue{Vocab: VocabEIVE, Dim: DimM, Value: 5}

	if withZero.NicheWidth == nil {
		t.Fatalf("withZero.NicheWidth is nil, want pointer to 0.0")
	}
	if *withZero.NicheWidth != 0 {
		t.Fatalf("withZero.NicheWidth = %v, want 0.0", *withZero.NicheWidth)
	}
	if withNil.NicheWidth != nil {
		t.Fatalf("withNil.NicheWidth = %v, want nil", withNil.NicheWidth)
	}
	if withZero.NSystems == nil {
		t.Fatalf("withZero.NSystems is nil, want pointer to 0")
	}
	if *withZero.NSystems != 0 {
		t.Fatalf("withZero.NSystems = %v, want 0", *withZero.NSystems)
	}
	if withNil.NSystems != nil {
		t.Fatalf("withNil.NSystems = %v, want nil", withNil.NSystems)
	}
}

func TestTraitSet_Fields(t *testing.T) {
	nw := 3.5
	n := 12
	ts := TraitSet{
		Vocab:        VocabEIVE,
		VocabVersion: "1.0",
		Taxonomy:     "euromed-aligned",
		Values: []TraitValue{
			{Vocab: VocabEIVE, VocabVersion: "1.0", Dim: DimM, Value: 4.2, NicheWidth: &nw, NSystems: &n},
		},
	}
	if ts.Taxonomy != "euromed-aligned" {
		t.Fatalf("Taxonomy = %q, want euromed-aligned", ts.Taxonomy)
	}
	if len(ts.Values) != 1 || ts.Values[0].Dim != DimM {
		t.Fatalf("Values = %+v, want single DimM entry", ts.Values)
	}
}

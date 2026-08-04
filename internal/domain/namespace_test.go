package domain_test

import (
	"testing"

	"github.com/jobrunner/hostus/internal/domain"
)

// TestIsAggregateName covers the three spellings FloraVeg's own list
// actually contains (305 "… aggr.", 3 "… s. l.", 0 "… s. str.") plus the
// negative cases the predicate must NOT claim.
func TestIsAggregateName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want bool
	}{
		{name: "bare binomial is not an aggregate", in: "Festuca ovina", want: false},
		{name: "aggr. marker", in: "Festuca ovina aggr.", want: true},
		{name: "spaced sensu lato", in: "Festuca ovina s. l.", want: true},
		{name: "agg. marker", in: "Acer opalus agg.", want: true},
		{name: "unspaced s.l.", in: "Alchemilla vulgaris s.l.", want: true},
		// s. str. NARROWS rather than widens — AggregateBases excludes it on
		// purpose, and this predicate must inherit that decision rather than
		// re-decide it.
		{name: "sensu stricto is not an aggregate", in: "Festuca ovina s. str.", want: false},
		{name: "infraspecific name is not an aggregate", in: "Acer opalus subsp. obtusatum", want: false},
		{name: "empty name", in: "", want: false},
		{name: "whitespace only", in: "   ", want: false},
		// A bare marker with nothing in front of it must not be read as an
		// aggregate: AggregateBases refuses to strip down to nothing.
		{name: "bare marker alone", in: "aggr.", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := domain.IsAggregateName(tt.in); got != tt.want {
				t.Errorf("IsAggregateName(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

// TestIsAggregateName_CanonicalizesFirst pins that the predicate folds its
// input rather than requiring a pre-canonicalized name: the CSV carries the
// source's own casing and spacing, and a caller must not have to remember to
// fold first.
func TestIsAggregateName_CanonicalizesFirst(t *testing.T) {
	t.Parallel()

	for _, in := range []string{"FESTUCA OVINA AGGR.", "Festuca   ovina   aggr.", "  Festuca ovina aggr.  "} {
		if !domain.IsAggregateName(in) {
			t.Errorf("IsAggregateName(%q) = false, want true", in)
		}
	}
}

package application

import "testing"

func TestSplitVerbatim(t *testing.T) {
	cases := []struct {
		verbatim      string
		wantCanonical string
		wantAuthor    string
	}{
		// no author at all.
		{"Genus species", "Genus species", ""},
		// a single-token trailing author starting with an uppercase letter.
		{"Senecio jacobaea L.", "Senecio jacobaea", "L."},
		// a parenthesized basionym author followed by the combining author.
		{"Genus species (Author) Author", "Genus species", "(Author) Author"},
		// the real fixture's exact form: parenthesized author glued to the
		// combining author with no space, and a two-word combining author.
		{"Corynephorus canescens (L.) P.Beauv.", "Corynephorus canescens", "(L.) P.Beauv."},
		// an aggregate marker is a lowercase trailing token, not an author:
		// it must stay part of the canonical.
		{"Genus species agg.", "Genus species agg.", ""},
		// a rank marker ("subsp.") is also lowercase and stays canonical,
		// even though the epithet after it plus the following author looks
		// similar in shape.
		{"Jacobaea vulgaris subsp. dunensis (Dumort.) Pelser & Meijden", "Jacobaea vulgaris subsp. dunensis", "(Dumort.) Pelser & Meijden"},
		// empty input.
		{"", "", ""},
		// bare genus, no epithet, no author.
		{"Genus", "Genus", ""},
		// author starting with 'A', the low boundary of the uppercase range.
		{"Genus species Answer", "Genus species", "Answer"},
		// author starting with 'Z', the high boundary of the uppercase range.
		{"Genus species Zephyr", "Genus species", "Zephyr"},
	}
	for _, c := range cases {
		gotCanonical, gotAuthor := splitVerbatim(c.verbatim)
		if gotCanonical != c.wantCanonical || gotAuthor != c.wantAuthor {
			t.Errorf("splitVerbatim(%q) = (%q, %q), want (%q, %q)", c.verbatim, gotCanonical, gotAuthor, c.wantCanonical, c.wantAuthor)
		}
	}
}

func TestIsAggregate(t *testing.T) {
	cases := []struct {
		canonical string
		want      bool
	}{
		{"Festuca ovina agg.", true},
		{"Festuca ovina aggr.", true},
		{"Festuca ovina s.l.", true},
		{"Festuca ovina", false},
		{"", false},
		// case-insensitive last token.
		{"Festuca ovina AGG.", true},
	}
	for _, c := range cases {
		if got := isAggregate(c.canonical); got != c.want {
			t.Errorf("isAggregate(%q) = %v, want %v", c.canonical, got, c.want)
		}
	}
}

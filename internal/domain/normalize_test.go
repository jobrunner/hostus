package domain_test

import (
	"testing"

	"github.com/jobrunner/hostus/internal/domain"
)

// Every input in this file is a REAL name that the full-data trait
// crosswalk failed to resolve against the complete WCVP, taken verbatim
// from poc/measure/out/unmatched-{eive,tichy,midolo}.txt (see
// docs/research/reality-check.md, M2'). The expected outputs are the
// canonical_fold spellings those same names DO have in the ingested WCVP
// index — verified against poc/measure/out/m1real.sqlite, not invented.

func TestNormalizeHybridMarker(t *testing.T) {
	tests := []struct {
		name  string
		in    string
		want  string
		want2 bool
	}{
		{
			// EIVE unmatched: "Acer ×coriaceum"; WCVP holds
			// "acer × coriaceum".
			name: "attached multiplication sign on a species epithet",
			in:   "acer ×coriaceum", want: "acer × coriaceum", want2: true,
		},
		{
			// EIVE unmatched: "Aconogonon ×fennicum".
			name: "attached multiplication sign, second real EIVE miss",
			in:   "aconogonon ×fennicum", want: "aconogonon × fennicum", want2: true,
		},
		{
			// Midolo unmatched: "Crocosmia x crocosmiiflora" — ASCII x as a
			// standalone token; WCVP holds "crocosmia × crocosmiiflora".
			name: "standalone ASCII x as a species-level marker",
			in:   "crocosmia x crocosmiiflora", want: "crocosmia × crocosmiiflora", want2: true,
		},
		{
			// Midolo unmatched: "Medicago x varia".
			name: "standalone ASCII x, second real Midolo miss",
			in:   "medicago x varia", want: "medicago × varia", want2: true,
		},
		{
			name: "attached multiplication sign on a nothogenus",
			in:   "×aegilotriticum", want: "× aegilotriticum", want2: true,
		},
		{
			name: "already spaced marker is left alone",
			in:   "abies × borisii-regis", want: "abies × borisii-regis", want2: false,
		},
		{
			name: "plain binomial is untouched",
			in:   "festuca ovina", want: "festuca ovina", want2: false,
		},
		{
			// The dangerous case: a lowercase x that is a legitimate letter
			// of the epithet, not a marker.
			name: "x-initial epithet is not a marker",
			in:   "rosa xanthina", want: "rosa xanthina", want2: false,
		},
		{
			name: "x-initial genus is not a marker",
			in:   "xanthium strumarium", want: "xanthium strumarium", want2: false,
		},
		{
			// A leading standalone ASCII "x" cannot be a genus-level marker
			// in a canonicalized name without also being indistinguishable
			// from a (nonexistent) one-letter genus; it is only ever a
			// marker AFTER the first token.
			name: "leading standalone ASCII x is not treated as a marker",
			in:   "x crocosmiiflora", want: "x crocosmiiflora", want2: false,
		},
		{
			// EIVE unmatched: "Abies alba × nordmanniana" — a binary hybrid
			// FORMULA. Spacing is already correct; nothing to normalise.
			name: "binary hybrid formula keeps its spacing",
			in:   "abies alba × nordmanniana", want: "abies alba × nordmanniana", want2: false,
		},
		{
			name: "empty input",
			in:   "", want: "", want2: false,
		},
		{
			// A bare marker token carries no epithet to attach to.
			name: "bare multiplication sign",
			in:   "×", want: "×", want2: false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, changed := domain.NormalizeHybridMarker(tc.in)
			if got != tc.want || changed != tc.want2 {
				t.Errorf("NormalizeHybridMarker(%q) = (%q, %v), want (%q, %v)", tc.in, got, changed, tc.want, tc.want2)
			}
		})
	}
}

func TestDropHybridMarker(t *testing.T) {
	tests := []struct {
		name  string
		in    string
		want  string
		want2 bool
	}{
		{
			// EIVE unmatched: "Anacamptis ×albertii"; WCVP holds the name
			// WITHOUT a marker, as "anacamptis albertii".
			name: "marker dropped recovers a name WCVP does not mark",
			in:   "anacamptis × albertii", want: "anacamptis albertii", want2: true,
		},
		{
			// EIVE unmatched: "Centaurea ×pouzinii".
			name: "marker dropped, second real EIVE miss",
			in:   "centaurea × pouzinii", want: "centaurea pouzinii", want2: true,
		},
		{
			name: "no marker, nothing to drop",
			in:   "festuca ovina", want: "festuca ovina", want2: false,
		},
		{
			name: "attached marker is not a standalone token and is not dropped",
			in:   "acer ×coriaceum", want: "acer ×coriaceum", want2: false,
		},
		{
			name: "dropping every marker of a formula",
			in:   "abies alba × nordmanniana", want: "abies alba nordmanniana", want2: true,
		},
		{
			name: "a name that is nothing but a marker keeps it",
			in:   "×", want: "×", want2: false,
		},
		{
			name: "empty input",
			in:   "", want: "", want2: false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, changed := domain.DropHybridMarker(tc.in)
			if got != tc.want || changed != tc.want2 {
				t.Errorf("DropHybridMarker(%q) = (%q, %v), want (%q, %v)", tc.in, got, changed, tc.want, tc.want2)
			}
		})
	}
}

func TestAddHybridMarker(t *testing.T) {
	tests := []struct {
		name  string
		in    string
		want  string
		want2 bool
	}{
		{
			// Tichý AND Midolo unmatched: "Abies borisii-regis"; WCVP holds
			// "abies × borisii-regis" (documented in reality-check M2.4).
			name: "unmarked nothospecies gains the marker",
			in:   "abies borisii-regis", want: "abies × borisii-regis", want2: true,
		},
		{
			// EIVE/Tichý/Midolo unmatched: "Amelanchier lamarckii"; WCVP
			// holds "amelanchier × lamarckii".
			name: "unmarked nothospecies, second real miss across all three vocabularies",
			in:   "amelanchier lamarckii", want: "amelanchier × lamarckii", want2: true,
		},
		{
			name: "already marked, nothing to add",
			in:   "abies × borisii-regis", want: "abies × borisii-regis", want2: false,
		},
		{
			name: "single token is not a binomial",
			in:   "abies", want: "abies", want2: false,
		},
		{
			// Only binomials get the hypothesis: an infraspecific name has
			// no measured case and adding a marker there would multiply
			// candidates without evidence.
			name: "infraspecific name is left alone",
			in:   "festuca ovina subsp. ovina", want: "festuca ovina subsp. ovina", want2: false,
		},
		{
			name: "empty input",
			in:   "", want: "", want2: false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, changed := domain.AddHybridMarker(tc.in)
			if got != tc.want || changed != tc.want2 {
				t.Errorf("AddHybridMarker(%q) = (%q, %v), want (%q, %v)", tc.in, got, changed, tc.want, tc.want2)
			}
		})
	}
}

func TestAggregateBases(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{
			// EIVE unmatched: "Acer opalus aggr.".
			name: "single aggregate marker",
			in:   "acer opalus aggr.", want: []string{"acer opalus"},
		},
		{
			// EIVE unmatched: "Alchemilla vulgaris aggr.".
			name: "single aggregate marker, second real EIVE miss",
			in:   "alchemilla vulgaris aggr.", want: []string{"alchemilla vulgaris"},
		},
		{
			// EIVE unmatched: "Agrostis capillaris aggr. s. l." — stacked
			// markers; 60 EIVE names carry this exact shape.
			name: "stacked aggregate markers strip one layer at a time",
			in:   "agrostis capillaris aggr. s. l.", want: []string{"agrostis capillaris aggr.", "agrostis capillaris"},
		},
		{
			// Tichý unmatched: "Festuca ovina s. l.".
			name: "spaced sensu lato is one marker, not two tokens",
			in:   "festuca ovina s. l.", want: []string{"festuca ovina"},
		},
		{
			name: "spaced sensu stricto",
			in:   "festuca ovina s. str.", want: []string{"festuca ovina"},
		},
		{
			// Boundary: the shortest name a SPACED sensu marker can be
			// stripped from is three tokens — one name token plus "s." plus
			// the qualifier.
			name: "genus plus spaced sensu lato is exactly the three-token boundary",
			in:   "festuca s. l.", want: []string{"festuca"},
		},
		{
			// Boundary: the shortest name a single-token marker can be
			// stripped from is two tokens.
			name: "genus plus aggregate marker is exactly the two-token boundary",
			in:   "festuca aggr.", want: []string{"festuca"},
		},
		{
			// "L." is Linnaeus, not "lato": a bare trailing "l." is only a
			// sensu marker when preceded by "s.".
			name: "bare trailing l. is not a sensu marker",
			in:   "festuca ovina l.", want: nil,
		},
		{
			name: "unspaced sensu lato",
			in:   "festuca ovina s.l.", want: []string{"festuca ovina"},
		},
		{
			name: "agg. spelling",
			in:   "festuca ovina agg.", want: []string{"festuca ovina"},
		},
		{
			name: "no marker",
			in:   "festuca ovina", want: nil,
		},
		{
			// Never strip a name down to nothing.
			name: "marker-only name is not stripped away",
			in:   "aggr.", want: nil,
		},
		{
			name: "empty input",
			in:   "", want: nil,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := domain.AggregateBases(tc.in)
			if len(got) != len(tc.want) {
				t.Fatalf("AggregateBases(%q) = %q, want %q", tc.in, got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("AggregateBases(%q) = %q, want %q", tc.in, got, tc.want)
				}
			}
		})
	}
}

func TestAutonymBase(t *testing.T) {
	tests := []struct {
		name  string
		in    string
		want  string
		want2 bool
	}{
		{
			// EIVE unmatched: "Acer obtusatum subsp. obtusatum"; WCVP holds
			// only the species, "acer obtusatum".
			name: "subspecies autonym",
			in:   "acer obtusatum subsp. obtusatum", want: "acer obtusatum", want2: true,
		},
		{
			// EIVE unmatched: "Aconitum lycoctonum subsp. lycoctonum".
			name: "subspecies autonym, second real EIVE miss",
			in:   "aconitum lycoctonum subsp. lycoctonum", want: "aconitum lycoctonum", want2: true,
		},
		{
			name: "variety autonym",
			in:   "festuca ovina var. ovina", want: "festuca ovina", want2: true,
		},
		{
			name: "form autonym",
			in:   "festuca ovina f. ovina", want: "festuca ovina", want2: true,
		},
		{
			name: "ssp. spelling",
			in:   "festuca ovina ssp. ovina", want: "festuca ovina", want2: true,
		},
		{
			// NOT an autonym: the infraspecific epithet differs from the
			// species epithet, so this is a genuinely different taxon that
			// must never be collapsed onto the species.
			name: "non-nominate subspecies is not an autonym",
			in:   "allium circinatum subsp. peloponnesiacum", want: "allium circinatum subsp. peloponnesiacum", want2: false,
		},
		{
			name: "species without infraspecific part",
			in:   "festuca ovina", want: "festuca ovina", want2: false,
		},
		{
			name: "unknown rank marker is not an autonym",
			in:   "festuca ovina prol. ovina", want: "festuca ovina prol. ovina", want2: false,
		},
		{
			// Deeper infraspecific chains ("x y subsp. y var. y") are not
			// reduced: there is no measured case and the reduction target
			// would be ambiguous.
			name: "double infraspecific chain is left alone",
			in:   "festuca ovina subsp. ovina var. ovina", want: "festuca ovina subsp. ovina var. ovina", want2: false,
		},
		{
			name: "empty input",
			in:   "", want: "", want2: false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := domain.AutonymBase(tc.in)
			if got != tc.want || ok != tc.want2 {
				t.Errorf("AutonymBase(%q) = (%q, %v), want (%q, %v)", tc.in, got, ok, tc.want, tc.want2)
			}
		})
	}
}

func TestGenitiveVariant(t *testing.T) {
	tests := []struct {
		name  string
		in    string
		want  string
		want2 bool
	}{
		{
			// EIVE/Tichý/Midolo unmatched: "Cardamine plumierii"; WCVP holds
			// "cardamine plumieri".
			name: "double-i genitive reduced to single i",
			in:   "cardamine plumierii", want: "cardamine plumieri", want2: true,
		},
		{
			// EIVE/Tichý/Midolo unmatched: "Cota triumfettii"; WCVP holds
			// "cota triumfetti".
			name: "double-i genitive, second real miss across all three vocabularies",
			in:   "cota triumfettii", want: "cota triumfetti", want2: true,
		},
		{
			// Midolo unmatched: "Plantago cornutii"; WCVP: "plantago cornuti".
			name: "double-i genitive, real Midolo miss",
			in:   "plantago cornutii", want: "plantago cornuti", want2: true,
		},
		{
			// EIVE/Tichý unmatched: "Polygala edmundi"; WCVP holds
			// "polygala edmundii" — the same rule, other direction.
			name: "single-i genitive extended to double i",
			in:   "polygala edmundi", want: "polygala edmundii", want2: true,
		},
		{
			// EIVE unmatched: "Crocus biflorus subsp. adamii"; WCVP:
			// "crocus biflorus subsp. adami". The rule applies to the LAST
			// epithet, infraspecific included.
			name: "infraspecific epithet",
			in:   "crocus biflorus subsp. adamii", want: "crocus biflorus subsp. adami", want2: true,
		},
		{
			// Boundary: genitiveStemMin is inclusive — a three-rune stem is
			// short but still a stem, and IS rewritten.
			name: "three-rune stem is exactly the shortest accepted stem",
			in:   "genus abci", want: "genus abcii", want2: true,
		},
		{
			name: "two-rune stem is one rune too short",
			in:   "genus abi", want: "genus abi", want2: false,
		},
		{
			name: "epithet not ending in i",
			in:   "festuca ovina", want: "festuca ovina", want2: false,
		},
		{
			// A three-i tail is not the Art. 60.8 -ii/-i alternation.
			name: "triple i is not the genitive alternation",
			in:   "genus abciii", want: "genus abciii", want2: false,
		},
		{
			// Too short to be a personal-name genitive; "i"/"ii" alone is
			// not an epithet stem.
			name: "bare i is not an epithet",
			in:   "genus i", want: "genus i", want2: false,
		},
		{
			name: "bare ii is not an epithet",
			in:   "genus ii", want: "genus ii", want2: false,
		},
		{
			name: "rank marker token is never rewritten",
			in:   "festuca ovina subsp.", want: "festuca ovina subsp.", want2: false,
		},
		{
			name: "empty input",
			in:   "", want: "", want2: false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := domain.GenitiveVariant(tc.in)
			if got != tc.want || ok != tc.want2 {
				t.Errorf("GenitiveVariant(%q) = (%q, %v), want (%q, %v)", tc.in, got, ok, tc.want, tc.want2)
			}
		})
	}
}

func TestNormalizationRuleFlagged(t *testing.T) {
	flagged := []domain.NormalizationRule{
		domain.RuleAggregateToNominate,
		domain.RuleAutonym,
	}
	unflagged := []domain.NormalizationRule{
		domain.RuleExact,
		domain.RuleHybridSpacing,
		domain.RuleHybridMarkerDropped,
		domain.RuleHybridMarkerAdded,
		domain.RuleAggregate,
		domain.RuleOrthographyGenitive,
	}
	for _, r := range flagged {
		if !r.Flagged() {
			t.Errorf("rule %q: Flagged() = false, want true — it is a botanical judgement call and must be reported", r)
		}
	}
	for _, r := range unflagged {
		if r.Flagged() {
			t.Errorf("rule %q: Flagged() = true, want false — it is a pure spelling normalisation, not a judgement call", r)
		}
	}
	if domain.NormalizationRule("no such rule").Flagged() {
		t.Error("unknown rule: Flagged() = true, want false")
	}
}

func TestNameCandidates(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []domain.NameCandidate
	}{
		{
			name: "plain binomial yields the exact key plus the hybrid hypothesis",
			in:   "festuca ovina",
			want: []domain.NameCandidate{
				{Key: "festuca ovina", Rule: domain.RuleExact},
				{Key: "festuca × ovina", Rule: domain.RuleHybridMarkerAdded},
			},
		},
		{
			// EIVE unmatched: "Acer ×coriaceum".
			name: "attached hybrid marker",
			in:   "Acer ×coriaceum",
			want: []domain.NameCandidate{
				{Key: "acer ×coriaceum", Rule: domain.RuleExact},
				{Key: "acer × coriaceum", Rule: domain.RuleHybridSpacing},
				{Key: "acer coriaceum", Rule: domain.RuleHybridMarkerDropped},
			},
		},
		{
			// EIVE unmatched: "Acer opalus aggr.". The aggregate CONCEPT is
			// tried before the nominate species — the fallback must never
			// pre-empt a real aggregate.
			name: "aggregate tries the aggregate concept before the nominate species",
			in:   "Acer opalus aggr.",
			want: []domain.NameCandidate{
				{Key: "acer opalus aggr.", Rule: domain.RuleExact},
				{Key: "acer opalus", Rule: domain.RuleAggregateToNominate},
			},
		},
		{
			// EIVE unmatched: "Agrostis capillaris aggr. s. l." — the
			// intermediate, still-aggregate-marked strips are RuleAggregate
			// (a real aggregate concept), only the bare species is the
			// flagged fallback.
			name: "stacked aggregate markers: only the bare species is the flagged fallback",
			in:   "Agrostis capillaris aggr. s. l.",
			want: []domain.NameCandidate{
				{Key: "agrostis capillaris aggr. s. l.", Rule: domain.RuleExact},
				{Key: "agrostis capillaris aggr.", Rule: domain.RuleAggregate},
				{Key: "agrostis capillaris", Rule: domain.RuleAggregateToNominate},
			},
		},
		{
			// EIVE unmatched: "Acer obtusatum subsp. obtusatum".
			name: "autonym",
			in:   "Acer obtusatum subsp. obtusatum",
			want: []domain.NameCandidate{
				{Key: "acer obtusatum subsp. obtusatum", Rule: domain.RuleExact},
				{Key: "acer obtusatum", Rule: domain.RuleAutonym},
			},
		},
		{
			// Tichý/Midolo unmatched: "Cardamine plumierii".
			name: "genitive orthography",
			in:   "Cardamine plumierii",
			want: []domain.NameCandidate{
				{Key: "cardamine plumierii", Rule: domain.RuleExact},
				{Key: "cardamine × plumierii", Rule: domain.RuleHybridMarkerAdded},
				{Key: "cardamine plumieri", Rule: domain.RuleOrthographyGenitive},
			},
		},
		{
			// Midolo unmatched: "Crocosmia x crocosmiiflora".
			name: "ASCII x marker normalised then dropped",
			in:   "Crocosmia x crocosmiiflora",
			want: []domain.NameCandidate{
				{Key: "crocosmia x crocosmiiflora", Rule: domain.RuleExact},
				{Key: "crocosmia × crocosmiiflora", Rule: domain.RuleHybridSpacing},
				{Key: "crocosmia crocosmiiflora", Rule: domain.RuleHybridMarkerDropped},
			},
		},
		{
			// EIVE unmatched: "Abies alba × nordmanniana" — a binary hybrid
			// FORMULA. hostus has no deterministic way to reach a
			// nothospecies concept from a formula; the marker-dropped form
			// is the only candidate, and it will not resolve either.
			name: "binary hybrid formula gets no invented candidate",
			in:   "Abies alba × nordmanniana",
			want: []domain.NameCandidate{
				{Key: "abies alba × nordmanniana", Rule: domain.RuleExact},
				{Key: "abies alba nordmanniana", Rule: domain.RuleHybridMarkerDropped},
			},
		},
		{
			name: "empty input yields nothing",
			in:   "   ",
			want: nil,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := domain.NameCandidates(tc.in)
			if len(got) != len(tc.want) {
				t.Fatalf("NameCandidates(%q) = %+v, want %+v", tc.in, got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("NameCandidates(%q) = %+v, want %+v", tc.in, got, tc.want)
				}
			}
		})
	}
}

func TestNameCandidatesAlwaysStartsWithTheExactKey(t *testing.T) {
	// The ladder must never displace the pre-normalisation behavior: the
	// first candidate is always Canonicalize(verbatim) itself, so a name
	// that resolved before still resolves the same way.
	for _, in := range []string{
		"Festuca ovina",
		"Acer ×coriaceum",
		"Acer opalus aggr.",
		"Acer obtusatum subsp. obtusatum",
		"Cardamine plumierii",
		"Abies alba × nordmanniana",
	} {
		got := domain.NameCandidates(in)
		if len(got) == 0 {
			t.Fatalf("NameCandidates(%q) returned nothing", in)
		}
		if got[0].Key != domain.Canonicalize(in) || got[0].Rule != domain.RuleExact {
			t.Errorf("NameCandidates(%q)[0] = %+v, want {Key: %q, Rule: exact}", in, got[0], domain.Canonicalize(in))
		}
	}
}

func TestNameCandidatesAreDeduplicated(t *testing.T) {
	// A rule that reproduces an earlier candidate must not add a duplicate
	// lookup: the ladder is executed as repository queries, and a duplicate
	// key would both cost a query and risk reporting a weaker rule for a
	// key an earlier, stronger rule already covered.
	for _, in := range []string{
		"Festuca ovina",
		"Acer ×coriaceum",
		"Agrostis capillaris aggr. s. l.",
		"Crocosmia x crocosmiiflora",
	} {
		seen := map[string]bool{}
		for _, c := range domain.NameCandidates(in) {
			if seen[c.Key] {
				t.Errorf("NameCandidates(%q) repeats key %q", in, c.Key)
			}
			seen[c.Key] = true
		}
	}
}

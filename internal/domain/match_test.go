package domain_test

import (
	"testing"

	"github.com/jobrunner/hostus/internal/domain"
)

func TestClassifyMatch(t *testing.T) {
	cases := []struct {
		qc, qa, cc, ca string
		want           domain.MatchType
		ok             bool
	}{
		// both canonical and author match -> exact_author.
		{"corynephorus canescens", "(l.) p.beauv.", "corynephorus canescens", "(l.) p.beauv.", domain.MatchExactAuthor, true},
		// canonical matches, query has no author -> exact.
		{"corynephorus canescens", "", "corynephorus canescens", "(l.) p.beauv.", domain.MatchExact, true},
		// canonical differs (congeneric species, epithet mismatch) -> no match.
		{"silene otites", "", "silene otitis", "", "", false},
		// canonical matches but query author differs from candidate author -> no match.
		{"corynephorus canescens", "(mill.) p.beauv.", "corynephorus canescens", "(l.) p.beauv.", "", false},
		// both canonical and author empty candidate but query has author -> no match (author mismatch).
		{"corynephorus canescens", "(l.) p.beauv.", "corynephorus canescens", "", "", false},
		// both canonical and author empty -> exact (query has no author, canonical matches).
		{"corynephorus canescens", "", "corynephorus canescens", "", domain.MatchExact, true},
	}
	for _, c := range cases {
		got, ok := domain.ClassifyMatch(c.qc, c.qa, c.cc, c.ca)
		if ok != c.ok || (ok && got != c.want) {
			t.Fatalf("ClassifyMatch(%q,%q,%q,%q): got %q,%v want %q,%v", c.qc, c.qa, c.cc, c.ca, got, ok, c.want, c.ok)
		}
	}
}

func TestSimilarity(t *testing.T) {
	cases := []struct {
		name    string
		a, b    string
		want    float64
		epsilon float64
	}{
		{"identical strings score 1.0", "silene otites", "silene otites", 1.0, 0},
		{"single-letter epithet typo clears FuzzyThreshold", "silene otitis", "silene otites", 0.923, 0.01},
		{"two distinct congeners fall well short of FuzzyThreshold", "festuca ovina", "festuca rubra", 0.692, 0.01},
		{"both empty is defined as identical (1.0), not 0", "", "", 1.0, 0},
		{"one empty, one not scores 0.0", "", "festuca ovina", 0.0, 0},
		{"one empty, one not scores 0.0 (args swapped)", "festuca ovina", "", 0.0, 0},
		// Unequal-length inputs, exercising the maxLen-selection and the
		// insertion-driven DP cells that every same-length case above never
		// touches: distance("a","ab") is exactly 1 (a single insertion), and
		// the correct maxLen is 2 (the LONGER side) -> 1 - 1/2 = 0.5. Get
		// either the max-selection or the insertion branch wrong and this
		// value changes (see match.go's mutation-testing notes on
		// levenshteinDistance).
		{"unequal lengths: single insertion, shorter first", "a", "ab", 0.5, 0.001},
		{"unequal lengths: single insertion, longer first", "ab", "a", 0.5, 0.001},
		{"unequal lengths: two insertions", "a", "abc", 1.0 / 3.0, 0.001},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := domain.Similarity(c.a, c.b)
			if diff := got - c.want; diff < -c.epsilon || diff > c.epsilon {
				t.Errorf("Similarity(%q, %q) = %v, want %v (+/- %v)", c.a, c.b, got, c.want, c.epsilon)
			}
		})
	}
}

func TestSimilarity_IsSymmetric(t *testing.T) {
	pairs := [][2]string{
		{"silene otitis", "silene otites"},
		{"festuca ovina", "festuca rubra"},
		{"corynephorus canescens", "corynephorus canescens"},
		{"", "abc"},
		{"", ""},
		{"a", "ab"},
	}
	for _, p := range pairs {
		ab := domain.Similarity(p[0], p[1])
		ba := domain.Similarity(p[1], p[0])
		if ab != ba {
			t.Errorf("Similarity(%q,%q)=%v != Similarity(%q,%q)=%v, want symmetric", p[0], p[1], ab, p[1], p[0], ba)
		}
	}
}

func TestSimilarity_ThresholdBoundary(t *testing.T) {
	if got := domain.Similarity("silene otitis", "silene otites"); got < domain.FuzzyThreshold {
		t.Errorf("Similarity(silene otitis, silene otites) = %v, want >= FuzzyThreshold (%v)", got, domain.FuzzyThreshold)
	}
	if got := domain.Similarity("festuca ovina", "festuca rubra"); got >= domain.FuzzyThreshold {
		t.Errorf("Similarity(festuca ovina, festuca rubra) = %v, want < FuzzyThreshold (%v)", got, domain.FuzzyThreshold)
	}
}

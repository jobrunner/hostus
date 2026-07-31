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

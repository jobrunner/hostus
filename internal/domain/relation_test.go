package domain_test

import (
	"strings"
	"testing"

	"github.com/jobrunner/hostus/internal/domain"
)

func TestParseRelationMapsMeasuredCDMVocabulary(t *testing.T) {
	// The raw spellings below are the COMPLETE relation vocabulary measured
	// over all 26.346 relations of the CDM rl_standardliste crawl
	// (pipelines/cdm/cdm.summary.txt, Task 2). Every one of them must map;
	// anything else must fail loudly (see the test below).
	cases := []struct {
		raw  string
		want domain.Relation
	}{
		{"Congruent to", domain.RelationCongruent},
		{"Not Congruent to", domain.RelationNotCongruent},
		{"Includes", domain.RelationIncludes},
		{"Overlaps", domain.RelationOverlaps},
		{"Included in or Includes or Overlaps", domain.RelationUncertain},
		{"is pro parte synonym for", domain.RelationProParte},
		{"is misapplied name for", domain.RelationMisapplied},
	}
	for _, tc := range cases {
		got, err := domain.ParseRelation(tc.raw)
		if err != nil {
			t.Fatalf("ParseRelation(%q): unexpected error: %v", tc.raw, err)
		}
		if got != tc.want {
			t.Errorf("ParseRelation(%q) = %q, want %q", tc.raw, got, tc.want)
		}
	}
}

func TestParseRelationRoundTripsItsOwnSpellings(t *testing.T) {
	// Everything ParseRelation emits must be re-parseable, because the
	// canonical value is what lands in concept_relation.relation and is read
	// back out again.
	for _, r := range []domain.Relation{
		domain.RelationCongruent,
		domain.RelationNotCongruent,
		domain.RelationIncludes,
		domain.RelationIncludedIn,
		domain.RelationOverlaps,
		domain.RelationUncertain,
		domain.RelationProParte,
		domain.RelationMisapplied,
	} {
		got, err := domain.ParseRelation(string(r))
		if err != nil {
			t.Fatalf("ParseRelation(%q): unexpected error: %v", r, err)
		}
		if got != r {
			t.Errorf("ParseRelation(%q) = %q, want %q", r, got, r)
		}
	}
}

func TestParseRelationIsCaseAndWhitespaceInsensitive(t *testing.T) {
	got, err := domain.ParseRelation("  CONGRUENT TO  ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != domain.RelationCongruent {
		t.Errorf("got %q, want %q", got, domain.RelationCongruent)
	}
}

func TestParseRelationFailsLoudlyWithTheOffendingValue(t *testing.T) {
	// The ParseRank lesson: an unmapped value must never be coerced,
	// defaulted or dropped, and the error must name what it choked on so the
	// operator can extend the mapping.
	for _, raw := range []string{"", "  ", "Sister of", "disjoint", "excludes"} {
		_, err := domain.ParseRelation(raw)
		if err == nil {
			t.Fatalf("ParseRelation(%q): want error, got nil", raw)
		}
		if !strings.Contains(err.Error(), raw) && strings.TrimSpace(raw) != "" {
			t.Errorf("ParseRelation(%q): error %q does not contain the offending value", raw, err)
		}
	}
}

func TestParseRelationRejectsIncludedInOrIncludesOrOverlapsCollapse(t *testing.T) {
	// ⊂⊃⊕ is a genuinely UNCERTAIN relation. Task 1 refused to collapse it
	// onto "overlaps" and the reviewer confirmed that call: /translate has to
	// present it honestly, so it must stay distinguishable.
	got, err := domain.ParseRelation("Included in or Includes or Overlaps")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == domain.RelationOverlaps {
		t.Fatal("⊂⊃⊕ must NOT collapse onto overlaps")
	}
	if got == domain.RelationIncludes || got == domain.RelationIncludedIn {
		t.Fatal("⊂⊃⊕ must NOT collapse onto includes/included_in")
	}
}

func TestRelationIsConceptRelation(t *testing.T) {
	cases := map[domain.Relation]bool{
		domain.RelationCongruent:    true,
		domain.RelationNotCongruent: true,
		domain.RelationIncludes:     true,
		domain.RelationIncludedIn:   true,
		domain.RelationOverlaps:     true,
		domain.RelationUncertain:    true,
		domain.RelationProParte:     true,
		// A misapplied name is a statement about NAME USAGE, not a set
		// relation between two circumscriptions.
		domain.RelationMisapplied: false,
	}
	for r, want := range cases {
		if got := r.IsConceptRelation(); got != want {
			t.Errorf("%q.IsConceptRelation() = %v, want %v", r, got, want)
		}
	}
}

func TestRelationInverse(t *testing.T) {
	cases := map[domain.Relation]domain.Relation{
		domain.RelationCongruent:    domain.RelationCongruent,
		domain.RelationNotCongruent: domain.RelationNotCongruent,
		domain.RelationOverlaps:     domain.RelationOverlaps,
		domain.RelationUncertain:    domain.RelationUncertain,
		domain.RelationIncludes:     domain.RelationIncludedIn,
		domain.RelationIncludedIn:   domain.RelationIncludes,
	}
	for r, want := range cases {
		got, ok := r.Inverse()
		if !ok {
			t.Fatalf("%q.Inverse(): want ok, got !ok", r)
		}
		if got != want {
			t.Errorf("%q.Inverse() = %q, want %q", r, got, want)
		}
	}
	// pro parte and misapplied are directed ASSERTIONS about a name, not
	// symmetric set relations — inverting them would invent a claim the
	// source never made.
	for _, r := range []domain.Relation{domain.RelationProParte, domain.RelationMisapplied} {
		if _, ok := r.Inverse(); ok {
			t.Errorf("%q.Inverse(): want !ok (no meaningful inverse)", r)
		}
	}
}

// TestRelationIsEqualityOnlyCongruent pins SP5 Task 4's first
// non-negotiable at the level it belongs to: a /translate answer may be
// presented as "the same taxon" for exactly one relation. If a future
// relation value is added and someone wants it to count as identity, this
// test is the place that has to be argued with.
func TestRelationIsEqualityOnlyCongruent(t *testing.T) {
	if !domain.RelationCongruent.IsEquality() {
		t.Errorf("congruent.IsEquality() = false, want true")
	}
	notEquality := []domain.Relation{
		domain.RelationNotCongruent,
		domain.RelationIncludes,
		domain.RelationIncludedIn,
		domain.RelationOverlaps,
		domain.RelationUncertain,
		domain.RelationProParte,
		domain.RelationMisapplied,
	}
	for _, r := range notEquality {
		if r.IsEquality() {
			t.Errorf("%q.IsEquality() = true, want false — only congruent may read as identity", r)
		}
	}
}

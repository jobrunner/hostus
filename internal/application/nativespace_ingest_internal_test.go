package application

import (
	"testing"

	"github.com/jobrunner/hostus/internal/domain"
)

// TestRankVerbatimFor_OnlyRankOtherKeepsVerbatim pins the documented
// invariant (internal/domain/taxon.go: domain.Name.RankVerbatim /
// domain.Concept.RankVerbatim are populated ONLY for domain.RankOther)
// directly at the source, independent of whether IngestNativeSpace's write
// path can currently reach RankOther (it cannot: qualifiesAsFallBConcept
// excludes RankOther from nativeConceptRankOrder, so no row of that rank is
// ever written as its own Fall-B concept — see this file's other tests).
// This is the fix-round-1 regression test for the reviewed finding.
func TestRankVerbatimFor_OnlyRankOtherKeepsVerbatim(t *testing.T) {
	cases := []struct {
		name     string
		rank     domain.Rank
		verbatim string
		want     string
	}{
		{"ordinary rank (Family) drops verbatim", domain.RankFamily, "Family", ""},
		{"ordinary rank (SpeciesAggregate) drops verbatim", domain.RankSpeciesAggregate, "SPECIES_AGGREGATE", ""},
		{"RankOther keeps verbatim", domain.RankOther, "totally-unknown-rank", "totally-unknown-rank"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := rankVerbatimFor(c.rank, c.verbatim)
			if got != c.want {
				t.Errorf("rankVerbatimFor(%q, %q) = %q, want %q", c.rank, c.verbatim, got, c.want)
			}
		})
	}
}

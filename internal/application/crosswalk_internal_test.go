package application

import (
	"testing"

	"github.com/jobrunner/hostus/internal/domain"
	"github.com/jobrunner/hostus/internal/ports/output"
)

// TestPreferGenuineClaimants pins the two-tier claimant preference (Spec
// B2, Stufe 2): tier 1 (preferBackboneConcepts) drops sec.-space-only
// candidates first, tier 2 then drops name-space-NATIVE candidates
// (Fall-B concepts, SecReference == "" but BackboneID in nativeSpaces)
// whenever a genuine backbone candidate remains — both tiers falling back
// to the unfiltered set whenever their own filter would empty it.
func TestPreferGenuineClaimants(t *testing.T) {
	// backboneSpaceID/nativeSpaceID/secSpaceID are deliberately synthetic,
	// NOT the real "wcvp"/"eurosl": this file lives in package application
	// itself (internal test), so goconst correlates its literals with
	// match.go's/translate.go's real occurrences of those same ids — reusing
	// them here would tip that pre-existing, unrelated count over
	// golangci-lint's goconst threshold and flag code this task must not
	// touch.
	const (
		backboneSpaceID = "fixture-backbone"
		nativeSpaceID   = "fixture-native"
		secSpaceID      = "fixture-sec"
	)
	nativeSpaces := map[string]bool{nativeSpaceID: true}

	backboneCandidate := output.MatchCandidate{Concept: domain.Concept{ID: backboneSpaceID + ":concept:g1", BackboneID: backboneSpaceID}}
	nativeCandidate := output.MatchCandidate{Concept: domain.Concept{ID: nativeSpaceID + ":concept:e1", BackboneID: nativeSpaceID}}
	secCandidate := output.MatchCandidate{Concept: domain.Concept{ID: secSpaceID + ":concept:x", BackboneID: secSpaceID, SecReference: secSpaceID + "-sec-1"}}

	cases := []struct {
		name         string
		candidates   []output.MatchCandidate
		nativeSpaces map[string]bool
		want         []output.MatchCandidate
	}{
		{
			name:         "backbone and native -> only backbone survives",
			candidates:   []output.MatchCandidate{backboneCandidate, nativeCandidate},
			nativeSpaces: nativeSpaces,
			want:         []output.MatchCandidate{backboneCandidate},
		},
		{
			name:         "only native -> fallback keeps it unchanged",
			candidates:   []output.MatchCandidate{nativeCandidate},
			nativeSpaces: nativeSpaces,
			want:         []output.MatchCandidate{nativeCandidate},
		},
		{
			name:         "sec and native -> tier 1 resolves it first, native alone remains",
			candidates:   []output.MatchCandidate{secCandidate, nativeCandidate},
			nativeSpaces: nativeSpaces,
			want:         []output.MatchCandidate{nativeCandidate},
		},
		{
			name:         "empty nativeSpaces set -> identical to preferBackboneConcepts",
			candidates:   []output.MatchCandidate{backboneCandidate, nativeCandidate},
			nativeSpaces: map[string]bool{},
			want:         []output.MatchCandidate{backboneCandidate, nativeCandidate},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := preferGenuineClaimants(c.candidates, c.nativeSpaces)
			if len(got) != len(c.want) {
				t.Fatalf("preferGenuineClaimants() = %+v, want %+v", got, c.want)
			}
			for i := range got {
				if got[i].Concept.ID != c.want[i].Concept.ID {
					t.Errorf("preferGenuineClaimants()[%d].Concept.ID = %q, want %q", i, got[i].Concept.ID, c.want[i].Concept.ID)
				}
			}
		})
	}
}

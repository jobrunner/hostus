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
	// otherBackboneCandidate is a SECOND genuine (non-sec, non-native)
	// backbone concept — distinct from backboneCandidate, sharing no
	// BackboneID/space with either it or nativeSpaceID.
	otherBackboneCandidate := output.MatchCandidate{Concept: domain.Concept{ID: "fixture-other-backbone:concept:g2", BackboneID: "fixture-other-backbone"}}
	nativeCandidate := output.MatchCandidate{Concept: domain.Concept{ID: nativeSpaceID + ":concept:e1", BackboneID: nativeSpaceID}}
	secCandidate := output.MatchCandidate{Concept: domain.Concept{ID: secSpaceID + ":concept:x", BackboneID: secSpaceID, SecReference: secSpaceID + "-sec-1"}}
	// secAndNativeCandidate carries BOTH traits at once: its BackboneID is a
	// native space AND it has a SecReference set (a shape that should not
	// occur in practice — a native concept never carries SecReference — but
	// preferGenuineClaimants must still resolve it correctly: tier 1 already
	// drops it via SecReference, before tier 2's native check ever runs).
	secAndNativeCandidate := output.MatchCandidate{Concept: domain.Concept{ID: nativeSpaceID + ":concept:secnative", BackboneID: nativeSpaceID, SecReference: secSpaceID + "-sec-2"}}

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
		{
			// M4: two GENUINE backbone candidates (neither sec-bearing nor
			// native) must stay ambiguous — the preference only ever narrows
			// a sec./native shadow away, never picks between two real
			// claimants.
			name:         "two genuine backbone candidates -> both kept, tie stands",
			candidates:   []output.MatchCandidate{backboneCandidate, otherBackboneCandidate},
			nativeSpaces: nativeSpaces,
			want:         []output.MatchCandidate{backboneCandidate, otherBackboneCandidate},
		},
		{
			// M4: nil is matchNamesFiltered's actual value on the filtered
			// path (see matchNamesFiltered's lazy load) — must behave
			// exactly like the already-covered empty-map case, not panic or
			// diverge.
			name:         "nil nativeSpaces map -> identical to preferBackboneConcepts",
			candidates:   []output.MatchCandidate{backboneCandidate, nativeCandidate},
			nativeSpaces: nil,
			want:         []output.MatchCandidate{backboneCandidate, nativeCandidate},
		},
		{
			// M4: a candidate that is simultaneously sec-bearing AND native
			// is dropped by tier 1 already (SecReference != ""), regardless
			// of also being native — tier 2 never gets a say.
			name:         "candidate both sec-bearing and native -> dropped by tier 1 already",
			candidates:   []output.MatchCandidate{backboneCandidate, secAndNativeCandidate},
			nativeSpaces: nativeSpaces,
			want:         []output.MatchCandidate{backboneCandidate},
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

// TestResolutionWithTieBreak pins resolutionWithTieBreak's rendering: the
// base rule (empty for domain.RuleExact) with the tie-break marker appended
// only when tieBroken is set — the SQL-greppable audit trail spec 2026-09-04
// requires (resolution LIKE '%accepted_bearer_tiebreak%').
func TestResolutionWithTieBreak(t *testing.T) {
	cases := []struct {
		name      string
		rule      domain.NormalizationRule
		tieBroken bool
		want      string
	}{
		{"exact ohne tie-break", domain.RuleExact, false, ""},
		{"exact mit tie-break", domain.RuleExact, true, "accepted_bearer_tiebreak"},
		{"rule ohne tie-break", domain.RuleHybridSpacing, false, string(domain.RuleHybridSpacing)},
		{"rule mit tie-break", domain.RuleHybridSpacing, true, string(domain.RuleHybridSpacing) + "+accepted_bearer_tiebreak"},
	}
	for _, c := range cases {
		if got := resolutionWithTieBreak(c.rule, c.tieBroken); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

// TestResolveHomonymTie pins resolveTraitName's policy dispatch directly at
// resolveHomonymTie, since after switching resolveNameSpaceNames to
// policyResolveAcceptedBearer no test or production path exercises the
// function with policyPreferBackbone or policyResolveGenuineBearer any
// more — those branches (and the default case) would otherwise be
// mutation-uncovered as a direct consequence of this task.
//
// The tier-2 case is the boundary that actually distinguishes
// policyResolveAcceptedBearer from policyResolveGenuineBearer: with no
// accepted bearer but one homotypic-synonym bearer, tier 2 decides under
// policyResolveGenuineBearer (the serving path's full rule) but must NOT
// decide under policyResolveAcceptedBearer (spec 2026-09-04, decision 1 —
// tier 2 stays out of the name-space crosswalk).
func TestResolveHomonymTie(t *testing.T) {
	tru := true
	oneAcceptedBearer := []output.MatchCandidate{
		{Concept: domain.Concept{ID: "c-accepted"}, Role: roleAccepted},
		{Concept: domain.Concept{ID: "c-synonym"}, Role: "synonym"},
	}
	onlyHomotypicBearer := []output.MatchCandidate{
		{Concept: domain.Concept{ID: "c-heterotypic"}, Role: "synonym"},
		{Concept: domain.Concept{ID: "c-homotypic"}, Role: "synonym", Homotypic: &tru},
	}

	cases := []struct {
		name       string
		candidates []output.MatchCandidate
		policy     crosswalkPolicy
		wantID     string
		wantOK     bool
	}{
		{"acceptedBearer policy, sole accepted bearer -> wins", oneAcceptedBearer, policyResolveAcceptedBearer, "c-accepted", true},
		{"preferBackbone policy, sole accepted bearer -> tie stands", oneAcceptedBearer, policyPreferBackbone, "", false},
		{"genuineBearer policy, sole accepted bearer -> wins via tier 1", oneAcceptedBearer, policyResolveGenuineBearer, "c-accepted", true},
		{"genuineBearer policy, only homotypic bearer -> wins via tier 2", onlyHomotypicBearer, policyResolveGenuineBearer, "c-homotypic", true},
		{"acceptedBearer policy, only homotypic bearer -> tie stands (no tier 2 here)", onlyHomotypicBearer, policyResolveAcceptedBearer, "", false},
		{"preferBackbone policy, only homotypic bearer -> tie stands", onlyHomotypicBearer, policyPreferBackbone, "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			id, ok := resolveHomonymTie(c.candidates, c.policy)
			if id != c.wantID || ok != c.wantOK {
				t.Errorf("resolveHomonymTie() = (%q, %v), want (%q, %v)", id, ok, c.wantID, c.wantOK)
			}
		})
	}
}

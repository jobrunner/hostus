package sqlite

import (
	"testing"

	"github.com/jobrunner/hostus/internal/domain"
)

// TestDedupeRestrictedSourcesByID_KeepsMoreSevereRedistribution pins the
// CONDITIONALS_NEGATION mutant at dedupeRestrictedSourcesByID's `>` — the
// exact "two-version dedup test" its own doc comment already describes but
// never had: two records sharing one ID, in BOTH orders, asserting on the
// SURVIVING Redistribution value, not just the surviving id.
func TestDedupeRestrictedSourcesByID_KeepsMoreSevereRedistribution(t *testing.T) {
	unknownFirst := dedupeRestrictedSourcesByID([]restrictedSource{
		{ID: "floraveg", Redistribution: string(domain.RedistributionUnknown)},
		{ID: "floraveg", Redistribution: string(domain.RedistributionRestricted)},
	})
	if len(unknownFirst) != 1 || unknownFirst[0].Redistribution != string(domain.RedistributionRestricted) {
		t.Errorf("dedupeRestrictedSourcesByID(unknown, restricted) = %+v, want [restricted] to survive", unknownFirst)
	}

	restrictedFirst := dedupeRestrictedSourcesByID([]restrictedSource{
		{ID: "floraveg", Redistribution: string(domain.RedistributionRestricted)},
		{ID: "floraveg", Redistribution: string(domain.RedistributionUnknown)},
	})
	if len(restrictedFirst) != 1 || restrictedFirst[0].Redistribution != string(domain.RedistributionRestricted) {
		t.Errorf("dedupeRestrictedSourcesByID(restricted, unknown) = %+v, want [restricted] to survive", restrictedFirst)
	}
}

// TestDedupeRestrictedSourcesByID_PreservesFirstSeenOrderAcrossIDs pins the
// `order` slice: distinct ids keep the order they were first seen in,
// unaffected by dedup — including the Redistribution value the dedup step
// itself decided should survive for the repeated id (Copilot review, PR #92:
// the message implied this but the assertion didn't check it).
func TestDedupeRestrictedSourcesByID_PreservesFirstSeenOrderAcrossIDs(t *testing.T) {
	got := dedupeRestrictedSourcesByID([]restrictedSource{
		{ID: "b", Redistribution: string(domain.RedistributionUnknown)},
		{ID: "a", Redistribution: string(domain.RedistributionUnknown)},
		{ID: "b", Redistribution: string(domain.RedistributionRestricted)},
	})
	if len(got) != 2 || got[0].ID != "b" || got[0].Redistribution != string(domain.RedistributionRestricted) || got[1].ID != "a" {
		t.Errorf("dedupeRestrictedSourcesByID order = %+v, want [b(restricted) a]", got)
	}
}

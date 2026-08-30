package sqlite

import "testing"

// TestDedupeRestrictedSourcesByID_KeepsMoreSevereRedistribution pins the
// CONDITIONALS_NEGATION mutant at dedupeRestrictedSourcesByID's `>` — the
// exact "two-version dedup test" its own doc comment already describes but
// never had: two records sharing one ID, in BOTH orders, asserting on the
// SURVIVING Redistribution value, not just the surviving id.
func TestDedupeRestrictedSourcesByID_KeepsMoreSevereRedistribution(t *testing.T) {
	unknownFirst := dedupeRestrictedSourcesByID([]restrictedSource{
		{ID: "floraveg", Redistribution: "unknown"},
		{ID: "floraveg", Redistribution: "restricted"},
	})
	if len(unknownFirst) != 1 || unknownFirst[0].Redistribution != "restricted" {
		t.Errorf("dedupeRestrictedSourcesByID(unknown, restricted) = %+v, want [restricted] to survive", unknownFirst)
	}

	restrictedFirst := dedupeRestrictedSourcesByID([]restrictedSource{
		{ID: "floraveg", Redistribution: "restricted"},
		{ID: "floraveg", Redistribution: "unknown"},
	})
	if len(restrictedFirst) != 1 || restrictedFirst[0].Redistribution != "restricted" {
		t.Errorf("dedupeRestrictedSourcesByID(restricted, unknown) = %+v, want [restricted] to survive", restrictedFirst)
	}
}

// TestDedupeRestrictedSourcesByID_PreservesFirstSeenOrderAcrossIDs pins the
// `order` slice: distinct ids keep the order they were first seen in,
// unaffected by dedup.
func TestDedupeRestrictedSourcesByID_PreservesFirstSeenOrderAcrossIDs(t *testing.T) {
	got := dedupeRestrictedSourcesByID([]restrictedSource{
		{ID: "b", Redistribution: "unknown"},
		{ID: "a", Redistribution: "unknown"},
		{ID: "b", Redistribution: "restricted"},
	})
	if len(got) != 2 || got[0].ID != "b" || got[1].ID != "a" {
		t.Errorf("dedupeRestrictedSourcesByID order = %+v, want [b(restricted) a]", got)
	}
}

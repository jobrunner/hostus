package application

import (
	"testing"

	"github.com/jobrunner/hostus/internal/domain"
	"github.com/jobrunner/hostus/internal/ports/output"
)

// TestClassify_HomotypicSynonymBreaksTie pins the match tie-break: when a
// verbatim name is a synonym under several concepts, the one where it is a
// HOMOTYPIC synonym (same nomenclatural type — the genuine name-bearer, e.g.
// Inula hirta L. ≡ Pentanema hirtum) wins over heterotypic/unknown links,
// instead of an ambiguous tie. Combined with entry_backbone=wcvp (which drops
// the CDM sec. concepts), this resolves "Inula hirta" to Pentanema hirtum.
func TestClassify_HomotypicSynonymBreaksTie(t *testing.T) {
	tru := true
	cands := []output.MatchCandidate{
		// "Inula hirta" as a heterotypic/unknown synonym (misapplication):
		{Concept: domain.Concept{ID: "wcvp:concept:britannica"}, MatchedName: domain.Name{Canonical: "Inula hirta"}, Role: "synonym"},
		// "Inula hirta" as the homotypic synonym (genuine name-bearer):
		{Concept: domain.Concept{ID: "wcvp:concept:hirtum"}, MatchedName: domain.Name{Canonical: "Inula hirta"}, Role: "synonym", Homotypic: &tru},
	}
	res, unresolved := classify(MatchRequest{ID: "1", Verbatim: "Inula hirta"}, domain.Canonicalize("Inula hirta"), "", cands)
	if unresolved {
		t.Fatal("classify returned unresolved; want resolved via the homotypic tie-break")
	}
	if res.ConceptID != "wcvp:concept:hirtum" {
		t.Errorf("ConceptID=%q, want wcvp:concept:hirtum (the homotypic synonym wins the tie)", res.ConceptID)
	}
	if res.RequiresReview {
		t.Errorf("RequiresReview=true; a single homotypic winner is a confident resolution")
	}
}

// TestClassify_TwoGenuineBearersStayAmbiguous pins the guard: the tie-break
// resolves ONLY when exactly one tied concept is the genuine name-bearer.
// Two concepts that both carry the name as their ACCEPTED name (the CDM flora
// case, when not scoped by entry_backbone) remain a legitimate ambiguity.
func TestClassify_TwoGenuineBearersStayAmbiguous(t *testing.T) {
	cands := []output.MatchCandidate{
		{Concept: domain.Concept{ID: "cdm:concept:a"}, MatchedName: domain.Name{Canonical: "Inula hirta"}, Role: "accepted"},
		{Concept: domain.Concept{ID: "cdm:concept:b"}, MatchedName: domain.Name{Canonical: "Inula hirta"}, Role: "accepted"},
	}
	res, _ := classify(MatchRequest{ID: "1", Verbatim: "Inula hirta"}, domain.Canonicalize("Inula hirta"), "", cands)
	if res.ConceptID != "" || !res.RequiresReview {
		t.Errorf("two accepted-name concepts must stay ambiguous; got ConceptID=%q RequiresReview=%v", res.ConceptID, res.RequiresReview)
	}
}

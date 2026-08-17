package sqlite

import (
	"context"
	"testing"

	"github.com/jobrunner/hostus/internal/domain"
	"github.com/jobrunner/hostus/internal/ports/output"
)

// TestMatchExact_ReturnsHomotypic pins that MatchExact carries the
// concept_name.homotypic flag through on each candidate, so the application
// tie-break can prefer a homotypic synonym (the genuine name-bearer).
func TestMatchExact_ReturnsHomotypic(t *testing.T) {
	db := openTestDB(t)
	tru := true
	bv := domain.BackboneVersion{ID: "wcvp", Version: "v1", IngestedAt: "2026-08-18T00:00:00Z", ManifestSHA: "x"}
	ingestVia(t, db, bv, func(tx output.IngestTx) {
		acc := species("n-hirtum", "Pentanema hirtum")
		syn := species("n-inula", "Inula hirta")
		mustTx(t, tx.UpsertName(acc))
		mustTx(t, tx.UpsertName(syn))
		c := domain.Concept{ID: "wcvp:concept:hirtum", BackboneID: "wcvp", AcceptedName: acc, Rank: domain.RankSpecies, Status: domain.StatusAccepted}
		mustTx(t, tx.UpsertConcept(c))
		mustTx(t, tx.LinkName(c.ID, acc.ID, "accepted", nil))
		mustTx(t, tx.LinkName(c.ID, syn.ID, "synonym", &tru)) // homotypic
	})

	cands, err := db.MatchExact(context.Background(), "Inula hirta")
	mustTx(t, err)
	var got *bool
	found := false
	for _, c := range cands {
		if c.MatchedName.Canonical == "Inula hirta" && c.Role == "synonym" {
			got, found = c.Homotypic, true
		}
	}
	if !found {
		t.Fatalf("MatchExact returned no synonym candidate for Inula hirta (got %d candidates)", len(cands))
	}
	if got == nil || !*got {
		t.Errorf("Homotypic=%v, want true (the synonym link was ingested homotypic)", got)
	}
}

package sqlite_test

import (
	"context"
	"testing"

	"github.com/jobrunner/hostus/internal/application"
	"github.com/jobrunner/hostus/internal/domain"
	"github.com/jobrunner/hostus/internal/ports/output"
)

// TestSuggest_SecReferenceReflectsSecBearingConcept: a Suggest hit carries the
// concept's sec. reference id (empty for a WCVP concept, set for a CDM one), so
// the HTTP layer can render a `sec` field that tells same-name results apart.
func TestSuggest_SecReferenceReflectsSecBearingConcept(t *testing.T) {
	ctx := context.Background()
	db := ingestWCVPFixture(t)

	concepts := []application.CDMConceptRow{{
		ConceptUUID: "secplant", ScientificName: "Cdmsecplant unica", Authorship: "L.",
		Rank: "Species", Status: "Accepted", SecUUID: "sec-uno", SecTitle: "Flora Uno",
	}}
	if _, err := application.IngestCDM(ctx, db, concepts, nil,
		domain.BackboneVersion{ID: "cdm", Version: "v1", Redistribution: domain.RedistributionUnknown}); err != nil {
		t.Fatalf("IngestCDM: %v", err)
	}

	cdm, err := db.Suggest(ctx, "Cdmsecplant", output.SuggestOpts{Limit: 10})
	if err != nil {
		t.Fatalf("Suggest(cdm): %v", err)
	}
	c, ok := conceptIDs(cdm)["cdm:concept:secplant"]
	if !ok {
		t.Fatalf("Suggest did not find the CDM concept; got %+v", cdm)
	}
	if c.SecReference != "sec-uno" {
		t.Errorf("CDM SecReference = %q, want %q", c.SecReference, "sec-uno")
	}

	wcvp, err := db.Suggest(ctx, "coryn", output.SuggestOpts{Limit: 10})
	if err != nil {
		t.Fatalf("Suggest(wcvp): %v", err)
	}
	w, ok := conceptIDs(wcvp)["wcvp:concept:405825"]
	if !ok {
		t.Fatalf("Suggest did not find the WCVP concept; got %+v", wcvp)
	}
	if w.SecReference != "" {
		t.Errorf("WCVP SecReference = %q, want empty", w.SecReference)
	}
}

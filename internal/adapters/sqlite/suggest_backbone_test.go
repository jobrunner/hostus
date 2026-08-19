package sqlite_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/jobrunner/hostus/internal/application"
	"github.com/jobrunner/hostus/internal/domain"
	"github.com/jobrunner/hostus/internal/ports/output"
)

// TestSuggest_BackboneRestrictsToOneBackbone: the Backbone option keeps only
// concepts of that backbone, so a caller can ask for the WCVP view instead of
// the same name repeated once per CDM sec. reference.
func TestSuggest_BackboneRestrictsToOneBackbone(t *testing.T) {
	ctx := context.Background()
	db := ingestWCVPFixture(t)

	concepts := []application.CDMConceptRow{{
		ConceptUUID: "coryn-cdm", ScientificName: "Corynefake unica", Authorship: "L.",
		Rank: "Species", Status: "Accepted", SecUUID: "sec-uno", SecTitle: "Flora Uno",
	}}
	if _, err := application.IngestCDM(ctx, db, concepts, nil,
		domain.BackboneVersion{ID: "cdm", Version: "v1", Redistribution: domain.RedistributionUnknown}); err != nil {
		t.Fatalf("IngestCDM: %v", err)
	}

	// Unfiltered, both backbones answer the same prefix.
	all, err := db.Suggest(ctx, "coryn", output.SuggestOpts{Limit: 50})
	if err != nil {
		t.Fatalf("Suggest (unfiltered): %v", err)
	}
	if _, ok := conceptIDs(all)["cdm:concept:coryn-cdm"]; !ok {
		t.Fatalf("unfiltered Suggest did not return the CDM concept; got %+v", all)
	}

	got, err := db.Suggest(ctx, "coryn", output.SuggestOpts{Limit: 50, Backbone: "wcvp"})
	if err != nil {
		t.Fatalf("Suggest (backbone=wcvp): %v", err)
	}
	if len(got) == 0 {
		t.Fatal("Suggest(backbone=wcvp) = empty, want the WCVP Corynephorus concepts")
	}
	for _, it := range got {
		if !strings.HasPrefix(it.ConceptID, "wcvp:") {
			t.Errorf("concept %q leaked into a backbone=wcvp result", it.ConceptID)
		}
	}
	if _, ok := conceptIDs(got)["wcvp:concept:405825"]; !ok {
		t.Errorf("Suggest(backbone=wcvp) = %+v, want the Corynephorus canescens concept", got)
	}
}

// TestSuggest_BackboneFiltersBeforeTheLimit is the regression this option
// exists for: on the real index a query like "Inula"+area=GER returns ~19 CDM
// concepts (the same name once per German flora) and a single WCVP one, so a
// caller filtering client-side after the fact keeps almost nothing. Filtering
// must therefore happen in the query, ahead of the limit — here many CDM
// concepts outnumber the fetch budget, and a WCVP result must still come back.
func TestSuggest_BackboneFiltersBeforeTheLimit(t *testing.T) {
	ctx := context.Background()
	db := ingestWCVPFixture(t)

	const noise = 40 // > the adapter's fetch budget for a small limit
	concepts := make([]application.CDMConceptRow, 0, noise)
	for i := range noise {
		concepts = append(concepts, application.CDMConceptRow{
			ConceptUUID:    fmt.Sprintf("coryn-noise-%02d", i),
			ScientificName: fmt.Sprintf("Corynefake specimen%02d", i),
			Authorship:     "L.",
			Rank:           "Species",
			Status:         "Accepted",
			SecUUID:        fmt.Sprintf("sec-%02d", i),
			SecTitle:       fmt.Sprintf("Flora %02d", i),
		})
	}
	if _, err := application.IngestCDM(ctx, db, concepts, nil,
		domain.BackboneVersion{ID: "cdm", Version: "v1", Redistribution: domain.RedistributionUnknown}); err != nil {
		t.Fatalf("IngestCDM: %v", err)
	}

	got, err := db.Suggest(ctx, "coryn", output.SuggestOpts{Limit: 2, Backbone: "wcvp"})
	if err != nil {
		t.Fatalf("Suggest: %v", err)
	}
	if len(got) == 0 {
		t.Fatal("Suggest(limit=2, backbone=wcvp) = empty; the filter ran after the limit, so the CDM noise crowded WCVP out")
	}
	for _, it := range got {
		if !strings.HasPrefix(it.ConceptID, "wcvp:") {
			t.Errorf("concept %q leaked into a backbone=wcvp result", it.ConceptID)
		}
	}
}

// TestSuggest_EmptyBackboneKeepsEveryBackbone pins that the option is opt-in:
// omitting it must not change the existing behavior.
func TestSuggest_EmptyBackboneKeepsEveryBackbone(t *testing.T) {
	ctx := context.Background()
	db := ingestWCVPFixture(t)

	concepts := []application.CDMConceptRow{{
		ConceptUUID: "coryn-cdm", ScientificName: "Corynefake unica", Authorship: "L.",
		Rank: "Species", Status: "Accepted", SecUUID: "sec-uno", SecTitle: "Flora Uno",
	}}
	if _, err := application.IngestCDM(ctx, db, concepts, nil,
		domain.BackboneVersion{ID: "cdm", Version: "v1", Redistribution: domain.RedistributionUnknown}); err != nil {
		t.Fatalf("IngestCDM: %v", err)
	}

	got, err := db.Suggest(ctx, "coryn", output.SuggestOpts{Limit: 50, Backbone: ""})
	if err != nil {
		t.Fatalf("Suggest: %v", err)
	}
	byID := conceptIDs(got)
	if _, ok := byID["cdm:concept:coryn-cdm"]; !ok {
		t.Errorf("empty Backbone dropped the CDM concept; got %+v", got)
	}
	if _, ok := byID["wcvp:concept:405825"]; !ok {
		t.Errorf("empty Backbone dropped the WCVP concept; got %+v", got)
	}
}

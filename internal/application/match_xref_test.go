package application_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jobrunner/hostus/internal/adapters/sqlite"
	"github.com/jobrunner/hostus/internal/application"
	"github.com/jobrunner/hostus/internal/domain"
)

// xrefTestBackbone is the backbone id seedXrefConcept registers its concept
// under — deliberately distinct from "wcvp" (which seededMatchRepo already
// registers) so TestMatchNames_XrefRespectsBackboneFilter can filter to
// "wcvp" and see the xref concept fall outside it.
const xrefTestBackbone = "test-xref"

// xrefTestAuthority/xrefTestExtID are the foreign id seedXrefConcept attaches
// to its concept — standing in for the PlantNet workflow's POWO id.
const (
	xrefTestAuthority = "powo"
	xrefTestExtID     = "77178414-1"
)

// seedXrefConcept ingests one accepted concept under xrefTestBackbone and
// attaches it a powo xref (xrefTestAuthority/xrefTestExtID) directly through
// the Repository port — the same seeding pattern as seedSileneOtites, plus
// the AddXref call the ingest crosswalk itself makes (internal/application/
// ingest.go's real POWO-xref write). Returns the concept id.
func seedXrefConcept(t *testing.T, repo *sqlite.DB) string {
	t.Helper()
	ctx := context.Background()
	tx, err := repo.BeginIngest(ctx, domain.BackboneVersion{ID: xrefTestBackbone, Version: "v1"})
	if err != nil {
		t.Fatalf("BeginIngest: unexpected error: %v", err)
	}
	name := domain.Name{ID: xrefTestBackbone + ":name:1", Canonical: "Xrefia testica", Authorship: "L.", Rank: domain.RankSpecies}
	concept := domain.Concept{ID: xrefTestBackbone + ":concept:1", BackboneID: xrefTestBackbone, AcceptedName: name, Rank: domain.RankSpecies, Status: domain.StatusAccepted}
	if err := tx.UpsertName(name); err != nil {
		t.Fatalf("UpsertName: unexpected error: %v", err)
	}
	if err := tx.UpsertConcept(concept); err != nil {
		t.Fatalf("UpsertConcept: unexpected error: %v", err)
	}
	if err := tx.LinkName(concept.ID, name.ID, "accepted", nil); err != nil {
		t.Fatalf("LinkName: unexpected error: %v", err)
	}
	if err := tx.AddXref(concept.ID, domain.Xref{Authority: xrefTestAuthority, ExtID: xrefTestExtID}, ""); err != nil {
		t.Fatalf("AddXref: unexpected error: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: unexpected error: %v", err)
	}
	return concept.ID
}

// seedSecReference registers id as a known sec. reference space with no
// concept attached — just enough for validateFilter (MatchFilter.Sec) to
// accept it, for tests that only need a REGISTERED id, not a concept that
// carries it (seedSameNameAcrossSecs is for the latter).
func seedSecReference(t *testing.T, repo *sqlite.DB, id string) string {
	t.Helper()
	ctx := context.Background()
	tx, err := repo.BeginIngest(ctx, domain.BackboneVersion{ID: "test-secref-only", Version: "v1"})
	if err != nil {
		t.Fatalf("BeginIngest: %v", err)
	}
	if err := tx.UpsertSecReference(domain.SecReference{ID: id, Title: "Flora " + id}); err != nil {
		t.Fatalf("UpsertSecReference(%s): %v", id, err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	return id
}

// addEurosl registers an "eurosl" name space (addFloraVeg's pattern, applied
// to the space TestMatchNames_XrefEntryResolvesByForeignID targets) and
// attaches the given per-concept entries.
func addEurosl(t *testing.T, repo *sqlite.DB, entries map[string][]domain.NameSpaceEntry) {
	t.Helper()
	ctx := context.Background()
	tx, err := repo.BeginTraitIngest(ctx)
	if err != nil {
		t.Fatalf("BeginTraitIngest: %v", err)
	}
	if err := tx.UpsertNameSpace(domain.NameSpaceMeta{
		ID: "eurosl", Version: "2026-01-01", ManifestSHA: "x",
		Redistribution: domain.RedistributionUnknown,
	}); err != nil {
		t.Fatalf("UpsertNameSpace: %v", err)
	}
	for conceptID, es := range entries {
		for _, e := range es {
			if err := tx.AddNameSpaceEntry(conceptID, e); err != nil {
				t.Fatalf("AddNameSpaceEntry(%s): %v", conceptID, err)
			}
		}
	}
	if err := tx.Finalize(); err != nil {
		t.Fatalf("Finalize: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}
}

// TestMatchNames_XrefEntryResolvesByForeignID pins the PlantNet workflow
// (spec 2026-09-13): a row carrying {"xref":{"authority":"powo","id":...}}
// resolves through Repository.ConceptByXref — no name ladder, no fuzzy —
// with match_type "xref" and confidence 1.0, and target_space enrichment
// works exactly as for verbatim rows.
func TestMatchNames_XrefEntryResolvesByForeignID(t *testing.T) {
	repo := seededMatchRepo(t)
	conceptID := seedXrefConcept(t, repo)
	addEurosl(t, repo, map[string][]domain.NameSpaceEntry{
		conceptID: {
			{Space: "eurosl", ExtID: "1", Name: "Xrefia testica", Aggregate: false},
		},
	})

	results, err := application.MatchInSpace(context.Background(), repo, []application.MatchRequest{
		{ID: "1", Xref: &application.XrefRef{Authority: xrefTestAuthority, ID: xrefTestExtID}},
	}, "eurosl", application.MatchFilter{})
	if err != nil {
		t.Fatalf("MatchInSpace: unexpected error: %v", err)
	}
	r := results[0]
	if r.MatchType != domain.MatchXref {
		t.Errorf("MatchType = %q, want %q", r.MatchType, domain.MatchXref)
	}
	if r.Confidence != 1.0 {
		t.Errorf("Confidence = %v, want 1.0", r.Confidence)
	}
	if r.ConceptID != conceptID {
		t.Errorf("ConceptID = %q, want %q", r.ConceptID, conceptID)
	}
	if r.TargetSpaceName != "Xrefia testica" {
		t.Errorf("TargetSpaceName = %q, want %q", r.TargetSpaceName, "Xrefia testica")
	}
}

// TestMatchNames_XrefUnknownIDIsUnresolvable: an unknown foreign id resolves
// to an unresolved result (no ConceptID/MatchType, RequiresReview true), and
// the batch carries on — the neighboring row with a valid verbatim still
// resolves normally.
func TestMatchNames_XrefUnknownIDIsUnresolvable(t *testing.T) {
	repo := seededMatchRepo(t)
	const corynephorusConceptID = "wcvp:concept:405825"

	results, err := application.MatchNames(context.Background(), repo, []application.MatchRequest{
		{ID: "1", Xref: &application.XrefRef{Authority: "powo", ID: "does-not-exist"}},
		{ID: "2", Verbatim: "Corynephorus canescens"},
	})
	if err != nil {
		t.Fatalf("MatchNames: unexpected error: %v", err)
	}
	r0 := results[0]
	if r0.ConceptID != "" {
		t.Errorf("results[0].ConceptID = %q, want empty", r0.ConceptID)
	}
	if r0.MatchType != "" {
		t.Errorf("results[0].MatchType = %q, want empty (unresolvable)", r0.MatchType)
	}
	if !r0.RequiresReview {
		t.Error("results[0].RequiresReview = false, want true")
	}
	if r0.Note == "" {
		t.Error("results[0].Note = empty, want an explanation")
	}
	r1 := results[1]
	if r1.ConceptID != corynephorusConceptID {
		t.Errorf("results[1].ConceptID = %q, want %q (batch must continue past the unresolvable xref row)", r1.ConceptID, corynephorusConceptID)
	}
}

// TestMatchNames_XrefRespectsBackboneFilter: the xref concept lives under
// xrefTestBackbone; filtering resolution to entry_backbone="wcvp" must
// refuse it, exactly as it refuses a verbatim candidate outside the filter.
func TestMatchNames_XrefRespectsBackboneFilter(t *testing.T) {
	repo := seededMatchRepo(t)
	seedXrefConcept(t, repo)

	results, err := application.MatchInSpace(context.Background(), repo, []application.MatchRequest{
		{ID: "1", Xref: &application.XrefRef{Authority: xrefTestAuthority, ID: xrefTestExtID}},
	}, "", application.MatchFilter{Backbone: "wcvp"})
	if err != nil {
		t.Fatalf("MatchInSpace: unexpected error: %v", err)
	}
	r := results[0]
	if r.ConceptID != "" {
		t.Errorf("ConceptID = %q, want empty (concept is outside the entry_backbone filter)", r.ConceptID)
	}
	if r.MatchType != "" {
		t.Errorf("MatchType = %q, want empty (unresolvable)", r.MatchType)
	}
	if !r.RequiresReview {
		t.Error("RequiresReview = false, want true")
	}
	if r.Note == "" {
		t.Error("Note = empty, want an explanation")
	}
}

// TestMatchNames_XrefRespectsSecFilter pins M7 (whole-branch review
// 2026-09-13): matchByXref's entry_sec check, the Sec counterpart of
// TestMatchNames_XrefRespectsBackboneFilter above. The WCVP fixture concept
// resolved by xref here is a genuine backbone concept — SecReference is
// empty, never any sec. reference space — so ANY non-empty entry_sec filter
// must refuse it exactly as the backbone filter does: unresolvable, with
// noteXrefFiltered (never a "wrong backbone" error — the id itself is
// valid, it just names a concept outside the requested sec. space).
func TestMatchNames_XrefRespectsSecFilter(t *testing.T) {
	repo := seededMatchRepo(t)
	const corynephorusPowoID = "396681-1"
	// A registered sec. reference space, so validateFilter accepts it —
	// the xref concept below carries none of it (it is a genuine WCVP
	// backbone concept, SecReference empty), which is exactly the mismatch
	// under test. No concept needs to carry this sec — the filter only
	// checks the ID is a known sec. reference.
	sec := seedSecReference(t, repo, "sec-corynephorus-filter-probe")

	results, err := application.MatchInSpace(context.Background(), repo, []application.MatchRequest{
		{ID: "1", Xref: &application.XrefRef{Authority: "powo", ID: corynephorusPowoID}},
	}, "", application.MatchFilter{Sec: sec})
	if err != nil {
		t.Fatalf("MatchInSpace: unexpected error: %v", err)
	}
	r := results[0]
	if r.ConceptID != "" {
		t.Errorf("ConceptID = %q, want empty (concept carries no sec. reference, so any entry_sec filter excludes it)", r.ConceptID)
	}
	if r.MatchType != "" {
		t.Errorf("MatchType = %q, want empty (unresolvable)", r.MatchType)
	}
	if !r.RequiresReview {
		t.Error("RequiresReview = false, want true")
	}
	const noteXrefFiltered = "Konzept liegt außerhalb des angeforderten Backbone-/Sec-Filters"
	if r.Note != noteXrefFiltered {
		t.Errorf("Note = %q, want %q", r.Note, noteXrefFiltered)
	}
}

// TestMatchNames_RowWithBothOrNeitherIsRejected: a row must set exactly one
// of Verbatim/Xref. Both set, or neither, is a request-shape error
// (application.ErrInvalidMatchRequest) naming the offending row id — the
// HTTP adapter maps it to 400 INVALID_QUERY.
func TestMatchNames_RowWithBothOrNeitherIsRejected(t *testing.T) {
	repo := seededMatchRepo(t)

	tests := []struct {
		name string
		req  application.MatchRequest
	}{
		{
			name: "both set",
			req:  application.MatchRequest{ID: "row-both", Verbatim: "Corynephorus canescens", Xref: &application.XrefRef{Authority: "powo", ID: "1"}},
		},
		{
			name: "neither set",
			req:  application.MatchRequest{ID: "row-neither"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := application.MatchNames(context.Background(), repo, []application.MatchRequest{tc.req})
			if !errors.Is(err, application.ErrInvalidMatchRequest) {
				t.Fatalf("MatchNames error = %v, want errors.Is(err, application.ErrInvalidMatchRequest)", err)
			}
			if !errorContains(err, tc.req.ID) {
				t.Errorf("error %q does not name the offending row id %q", err, tc.req.ID)
			}
		})
	}
}

// errorContains reports whether err's message contains substr — a tiny local
// helper so the row-id assertion above stays readable.
func errorContains(err error, substr string) bool {
	return err != nil && strings.Contains(err.Error(), substr)
}

package application_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/jobrunner/hostus/internal/adapters/sqlite"
	"github.com/jobrunner/hostus/internal/application"
	"github.com/jobrunner/hostus/internal/domain"
)

// seedSameNameAcrossBackbones ingests TWO accepted concepts that share the
// exact same canonical+author but live in DIFFERENT backbones — the
// multi-backbone ambiguity the sec filter exists to resolve (WCVP + CDM in
// production). Without a filter a query for that name is ambiguous across both;
// entry_backbone must narrow it to one.
func seedSameNameAcrossBackbones(t *testing.T, repo *sqlite.DB) (conceptA, conceptB, backboneA, backboneB string) {
	t.Helper()
	ctx := context.Background()
	mk := func(bb string) string {
		tx, err := repo.BeginIngest(ctx, domain.BackboneVersion{ID: bb, Version: "v1"})
		if err != nil {
			t.Fatalf("BeginIngest(%s): %v", bb, err)
		}
		name := domain.Name{ID: bb + ":name:x", Canonical: "Filtrandum testicum", Authorship: "L.", Rank: domain.RankSpecies}
		concept := domain.Concept{ID: bb + ":concept:x", BackboneID: bb, AcceptedName: name, Rank: domain.RankSpecies, Status: domain.StatusAccepted}
		if err := tx.UpsertName(name); err != nil {
			t.Fatalf("UpsertName(%s): %v", bb, err)
		}
		if err := tx.UpsertConcept(concept); err != nil {
			t.Fatalf("UpsertConcept(%s): %v", bb, err)
		}
		if err := tx.LinkName(concept.ID, name.ID, "accepted", nil); err != nil {
			t.Fatalf("LinkName(%s): %v", bb, err)
		}
		if err := tx.Commit(); err != nil {
			t.Fatalf("Commit(%s): %v", bb, err)
		}
		return concept.ID
	}
	backboneA, backboneB = "test-bb-a", "test-bb-b"
	return mk(backboneA), mk(backboneB), backboneA, backboneB
}

// seedSameNameAcrossSecs ingests two accepted concepts with the same
// canonical+author in ONE backbone but under two different sec. reference
// spaces (the CDM shape: one name, many sec. concepts). entry_sec must narrow
// a query to exactly one sec space.
func seedSameNameAcrossSecs(t *testing.T, repo *sqlite.DB) (secA, secB string) {
	t.Helper()
	ctx := context.Background()
	tx, err := repo.BeginIngest(ctx, domain.BackboneVersion{ID: "test-secbb", Version: "v1"})
	if err != nil {
		t.Fatalf("BeginIngest: %v", err)
	}
	secA, secB = "sec-alpha", "sec-beta"
	for _, s := range []string{secA, secB} {
		if err := tx.UpsertSecReference(domain.SecReference{ID: s, Title: "Flora " + s}); err != nil {
			t.Fatalf("UpsertSecReference(%s): %v", s, err)
		}
	}
	for i, s := range []string{secA, secB} {
		name := domain.Name{ID: "test-secbb:name:" + s, Canonical: "Secfiltrandum testicum", Authorship: "L.", Rank: domain.RankSpecies}
		concept := domain.Concept{ID: "test-secbb:concept:" + s, BackboneID: "test-secbb", AcceptedName: name, Rank: domain.RankSpecies, Status: domain.StatusAccepted, SecReference: s}
		if err := tx.UpsertName(name); err != nil {
			t.Fatalf("UpsertName[%d]: %v", i, err)
		}
		if err := tx.UpsertConcept(concept); err != nil {
			t.Fatalf("UpsertConcept[%d]: %v", i, err)
		}
		if err := tx.LinkName(concept.ID, name.ID, "accepted", nil); err != nil {
			t.Fatalf("LinkName[%d]: %v", i, err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	return secA, secB
}

// TestMatchFilter_EmptyFilterEqualsMatchNames pins the invariant: with no
// filter, MatchInSpace's result is byte-for-byte MatchNames'.
func TestMatchFilter_EmptyFilterEqualsMatchNames(t *testing.T) {
	repo := seededMatchRepo(t)
	reqs := []application.MatchRequest{{ID: "1", Verbatim: "Corynephorus canescens"}}

	filtered, err := application.MatchInSpace(context.Background(), repo, reqs, "", application.MatchFilter{})
	if err != nil {
		t.Fatalf("MatchInSpace(empty filter): %v", err)
	}
	plain, err := application.MatchNames(context.Background(), repo, reqs)
	if err != nil {
		t.Fatalf("MatchNames: %v", err)
	}
	if !reflect.DeepEqual(filtered[0], plain[0]) {
		t.Errorf("empty filter = %+v, want identical to MatchNames = %+v", filtered[0], plain[0])
	}
}

// TestMatchFilter_BackboneDisambiguates: a name present in two backbones is
// ambiguous without a filter and resolves to exactly one with entry_backbone.
func TestMatchFilter_BackboneDisambiguates(t *testing.T) {
	repo := seededMatchRepo(t)
	conceptA, _, backboneA, _ := seedSameNameAcrossBackbones(t, repo)
	reqs := []application.MatchRequest{{ID: "1", Verbatim: "Filtrandum testicum L."}}

	amb, err := application.MatchInSpace(context.Background(), repo, reqs, "", application.MatchFilter{})
	if err != nil {
		t.Fatalf("no filter: %v", err)
	}
	if amb[0].ConceptID != "" || amb[0].MatchType != "" {
		t.Fatalf("no filter = %+v, want ambiguous (empty ConceptID/MatchType)", amb[0])
	}

	one, err := application.MatchInSpace(context.Background(), repo, reqs, "", application.MatchFilter{Backbone: backboneA})
	if err != nil {
		t.Fatalf("entry_backbone: %v", err)
	}
	if one[0].ConceptID != conceptA || one[0].MatchType != domain.MatchExactAuthor {
		t.Errorf("entry_backbone=%s = %+v, want ConceptID=%s exact_author", backboneA, one[0], conceptA)
	}
}

// TestMatchFilter_SecDisambiguates: a name present under two sec spaces
// resolves to one with entry_sec.
func TestMatchFilter_SecDisambiguates(t *testing.T) {
	repo := seededMatchRepo(t)
	secA, _ := seedSameNameAcrossSecs(t, repo)
	reqs := []application.MatchRequest{{ID: "1", Verbatim: "Secfiltrandum testicum L."}}

	one, err := application.MatchInSpace(context.Background(), repo, reqs, "", application.MatchFilter{Sec: secA})
	if err != nil {
		t.Fatalf("entry_sec: %v", err)
	}
	if one[0].ConceptID != "test-secbb:concept:"+secA || one[0].MatchType != domain.MatchExactAuthor {
		t.Errorf("entry_sec=%s = %+v, want ConceptID=test-secbb:concept:%s exact_author", secA, one[0], secA)
	}
}

// TestMatchFilter_UnknownBackboneAndSecAreRejected pins the validation: an
// un-ingested backbone or sec is a named error (HTTP renders 400), checked
// before any matching.
func TestMatchFilter_UnknownBackboneAndSecAreRejected(t *testing.T) {
	repo := seededMatchRepo(t)
	reqs := []application.MatchRequest{{ID: "1", Verbatim: "Corynephorus canescens"}}

	if _, err := application.MatchInSpace(context.Background(), repo, reqs, "", application.MatchFilter{Backbone: "nope"}); !errors.Is(err, application.ErrUnknownBackbone) {
		t.Errorf("entry_backbone=nope err = %v, want ErrUnknownBackbone", err)
	}
	if _, err := application.MatchInSpace(context.Background(), repo, reqs, "", application.MatchFilter{Sec: "nope"}); !errors.Is(err, application.ErrUnknownSec) {
		t.Errorf("entry_sec=nope err = %v, want ErrUnknownSec", err)
	}
}

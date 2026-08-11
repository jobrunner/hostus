package sqlite_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/jobrunner/hostus/internal/adapters/sqlite"
	"github.com/jobrunner/hostus/internal/domain"
)

// TestMatchFuzzyCandidates_SecFilterAppliedBeforeLimit is the regression test
// for the review finding: the fuzzy prefilter caps at `limit` rows, and if the
// backbone/sec filter were applied only AFTER that cap, out-of-space
// same-length near-misses could fill every slot and truncate away the wanted
// space's genuine near-miss — a false UNRESOLVABLE in the very multi-sec case
// the filter exists to serve. With the filter pushed into the prefilter query,
// the wanted candidate survives.
func TestMatchFuzzyCandidates_SecFilterAppliedBeforeLimit(t *testing.T) {
	ctx := context.Background()
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	tx, err := db.BeginIngest(ctx, domain.BackboneVersion{ID: "cdm-fuzz", Version: "v1"})
	if err != nil {
		t.Fatalf("BeginIngest: %v", err)
	}
	for _, s := range []string{"s1", "s2"} {
		if err := tx.UpsertSecReference(domain.SecReference{ID: s, Title: "Flora " + s}); err != nil {
			t.Fatalf("UpsertSecReference(%s): %v", s, err)
		}
	}
	add := func(id, canonical, sec string) {
		name := domain.Name{ID: id + ":name", Canonical: canonical, Rank: domain.RankSpecies}
		concept := domain.Concept{ID: id, BackboneID: "cdm-fuzz", AcceptedName: name, Rank: domain.RankSpecies, Status: domain.StatusAccepted, SecReference: sec}
		if err := tx.UpsertName(name); err != nil {
			t.Fatalf("UpsertName(%s): %v", id, err)
		}
		if err := tx.UpsertConcept(concept); err != nil {
			t.Fatalf("UpsertConcept(%s): %v", id, err)
		}
		if err := tx.LinkName(concept.ID, name.ID, "accepted", nil); err != nil {
			t.Fatalf("LinkName(%s): %v", id, err)
		}
	}
	// 25 out-of-space distractors (sec s2), all 9-char, first rune 'p', and
	// sorting BEFORE the target's canonical ("poa a..." < "pxa a...") — more
	// than the default cap of 20, so they would fill it entirely.
	for i := 0; i < 25; i++ {
		add(fmt.Sprintf("distract-%02d", i), fmt.Sprintf("Poa aaaa%c", 'a'+i), "s2")
	}
	// The one genuine near-miss in the wanted space s1.
	const target = "target-s1"
	add(target, "Pxa annua", "s1")
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	// Query near "pxa annuo" restricted to s1: without the pre-limit filter the
	// 25 s2 distractors would crowd out the target; with it, the target is kept.
	got, err := db.MatchFuzzyCandidates(ctx, "pxa annuo", 20, "", "s1")
	if err != nil {
		t.Fatalf("MatchFuzzyCandidates: %v", err)
	}
	found := false
	for _, c := range got {
		if c.Concept.ID == target {
			found = true
		}
		if c.Concept.SecReference != "s1" {
			t.Errorf("candidate %s is in sec %q, want only s1", c.Concept.ID, c.Concept.SecReference)
		}
	}
	if !found {
		t.Fatalf("MatchFuzzyCandidates(sec=s1) did not return the s1 target; got %d candidates, all out-of-space distractors would mean the pre-limit filter regressed", len(got))
	}
}

package sqlite

import (
	"context"
	"testing"

	"github.com/jobrunner/hostus/internal/domain"
	"github.com/jobrunner/hostus/internal/ports/output"
)

// TestSuggest_TargetSpaceResolvesLikeTranslate is the regression for a defect
// the domain fix alone did NOT reach: /v1/suggest loads a concept's spellings
// through targetSpaceQuery rather than through NameSpaceEntries, and that
// query selected only name/aggregate/status. Rank, AcceptedName and Resolution
// arrived empty, so domain.ResolveTargetSpace ran with its evidence removed —
// rank congruence and the synonymy anchor both stood down, and a
// closure-attached entry passed as direct.
//
// Measured on the real index while /v1/translate already answered correctly:
//
//	/v1/translate → Bromopsis erecta
//	/v1/suggest   → Bromopsis erecta subsp. permixta
//
// That split is the whole point of this test. /v1/suggest is the endpoint
// expertus' species field calls, so the originally reported defect survived
// untouched behind a fix verified only through translate.
func TestSuggest_TargetSpaceResolvesLikeTranslate(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	seedThreePrefixConcepts(t, db)

	tx, err := db.BeginIngest(ctx, domain.BackboneVersion{ID: "eurosl-src", Version: "v1", IngestedAt: "2026-08-19T00:00:00Z", ManifestSHA: "x"})
	mustTx(t, err)
	mustTx(t, tx.UpsertNameSpace(domain.NameSpaceMeta{
		ID: "eurosl", Version: "2024-11-03", ManifestSHA: "x", Redistribution: domain.RedistributionUnknown,
	}))
	// The real Bromus erectus constellation, reduced and renamed onto the
	// seeded concept: several entries the space accepts, of which the lowest
	// ext_id is a subspecies — so ext_id order alone answers wrong, status
	// alone cannot separate them, and only the synonym entry naming the
	// source concept settles it.
	for _, e := range []domain.NameSpaceEntry{
		{
			Space: "eurosl", ExtID: "a-subsp", Name: "Zzq acceptum subsp. permixtum",
			Status: "accepted", Rank: domain.RankSubspecies,
		},
		{
			Space: "eurosl", ExtID: "b-other", Name: "Zzq alienum",
			Status: "accepted", Rank: domain.RankSpecies,
		},
		{
			Space: "eurosl", ExtID: "c-anchor", Name: "Zzqaaa aaa",
			Status: "synonymobjective", Rank: domain.RankSpecies,
			AcceptedName: "Zzq acceptum",
		},
		{
			Space: "eurosl", ExtID: "d-accepted", Name: "Zzq acceptum",
			Status: "accepted", Rank: domain.RankSpecies,
		},
	} {
		mustTx(t, tx.AddNameSpaceEntry("wcvp:concept:zzq-a", e))
	}
	mustTx(t, tx.Commit())

	got, err := db.Suggest(ctx, "Zzqaaa", output.SuggestOpts{Limit: 20, TargetSpace: "eurosl"})
	mustTx(t, err)

	found := false
	for _, it := range got {
		if it.ConceptID != "wcvp:concept:zzq-a" {
			continue
		}
		found = true
		if it.TargetSpaceName != "Zzq acceptum" {
			t.Errorf("TargetSpaceName = %q, want %q — suggest must resolve through the same evidence translate does",
				it.TargetSpaceName, "Zzq acceptum")
		}
	}
	if !found {
		t.Fatal("the seeded concept did not come back from Suggest at all")
	}
}

// TestSuggest_TargetSpacePrefersDirectOverClosureAttached pins the third field
// the suggest query dropped. Resolution carries the marker
// application.closeSynonymyGroups appends to entries it attached by inference
// rather than by this row's own spelling, and pickSpelling ranks a DIRECT
// accepted entry above a closure-attached one. With Resolution unscanned every
// entry looked direct, so that preference — a fix of its own (whole-branch
// review 2026-09-13, I3) — was silently inert on this path.
func TestSuggest_TargetSpacePrefersDirectOverClosureAttached(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	seedThreePrefixConcepts(t, db)

	tx, err := db.BeginIngest(ctx, domain.BackboneVersion{ID: "eurosl-src", Version: "v1", IngestedAt: "2026-08-19T00:00:00Z", ManifestSHA: "x"})
	mustTx(t, err)
	mustTx(t, tx.UpsertNameSpace(domain.NameSpaceMeta{
		ID: "eurosl", Version: "2024-11-03", ManifestSHA: "x", Redistribution: domain.RedistributionUnknown,
	}))
	for _, e := range []domain.NameSpaceEntry{
		{
			Space: "eurosl", ExtID: "a-closed", Name: "Zzq per inferentiam",
			Status: "accepted", Resolution: domain.ResolutionSourceSynonymyClosure,
		},
		{
			Space: "eurosl", ExtID: "b-direct", Name: "Zzq directum",
			Status: "accepted",
		},
	} {
		mustTx(t, tx.AddNameSpaceEntry("wcvp:concept:zzq-a", e))
	}
	mustTx(t, tx.Commit())

	got, err := db.Suggest(ctx, "Zzqaaa", output.SuggestOpts{Limit: 20, TargetSpace: "eurosl"})
	mustTx(t, err)

	for _, it := range got {
		if it.ConceptID != "wcvp:concept:zzq-a" {
			continue
		}
		if it.TargetSpaceName != "Zzq directum" {
			t.Errorf("TargetSpaceName = %q, want the DIRECTLY matched entry over the closure-attached one", it.TargetSpaceName)
		}
	}
}

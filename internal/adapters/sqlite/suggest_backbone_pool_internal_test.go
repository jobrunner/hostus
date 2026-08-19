package sqlite

import (
	"context"
	"strings"
	"testing"

	"github.com/jobrunner/hostus/internal/domain"
	"github.com/jobrunner/hostus/internal/ports/output"
)

// seedMajorityWCVPMinorityCDM writes four WCVP concepts and one CDM concept,
// all matching the prefix "Zzq". Only the CDM one is distributed in area "ZZZ",
// so it is both the minority backbone AND the in-area hit.
func seedMajorityWCVPMinorityCDM(t *testing.T, db *DB) {
	t.Helper()
	wcvp := domain.BackboneVersion{ID: "wcvp", Version: "v1", IngestedAt: "2026-08-19T00:00:00Z", ManifestSHA: "x"}
	ingestVia(t, db, wcvp, func(tx output.IngestTx) {
		for _, s := range []struct{ id, canonical string }{
			{"wcvp:concept:zzq-a", "Zzqaaa aaa"},
			{"wcvp:concept:zzq-b", "Zzqbbb bbb"},
			{"wcvp:concept:zzq-c", "Zzqccc ccc"},
			{"wcvp:concept:zzq-d", "Zzqddd ddd"},
		} {
			n := species("n-"+s.id, s.canonical)
			mustTx(t, tx.UpsertName(n))
			c := domain.Concept{ID: s.id, BackboneID: "wcvp", AcceptedName: n, Rank: domain.RankSpecies, Status: domain.StatusAccepted}
			mustTx(t, tx.UpsertConcept(c))
			mustTx(t, tx.LinkName(c.ID, n.ID, "accepted", nil))
		}
	})

	cdm := domain.BackboneVersion{ID: "cdm", Version: "v1", IngestedAt: "2026-08-19T00:00:00Z", ManifestSHA: "y"}
	ingestVia(t, db, cdm, func(tx output.IngestTx) {
		n := species("n-cdm-zzq", "Zzqeee eee")
		mustTx(t, tx.UpsertName(n))
		c := domain.Concept{ID: "cdm:concept:zzq-e", BackboneID: "cdm", AcceptedName: n, Rank: domain.RankSpecies, Status: domain.StatusAccepted}
		mustTx(t, tx.UpsertConcept(c))
		mustTx(t, tx.LinkName(c.ID, n.ID, "accepted", nil))
		mustTx(t, tx.AddDistribution(c.ID, domain.Distribution{AreaScheme: "wgsrpd_l3", AreaCode: "ZZZ"}))
	})
}

// TestSuggest_BackboneKeepsInAreaBeyondPool is the recall guarantee that
// matters for a MINORITY backbone.
//
// The bm25 pool is filled with no backbone awareness, so a majority backbone
// can crowd a minority one out of it: on the real index the prefix "ca" matches
// 4345 CDM names but only 292 reach a 5000-row pool, because WCVP takes 4708
// slots. For plain relevance ranking that is the pool's documented and accepted
// trade-off — the 292 kept ARE that backbone's most relevant, which is more
// than a result page needs (verified on the real index: entry_backbone=cdm&q=ca
// returns a full, sensible page).
//
// It is NOT acceptable for in_area, which outranks bm25: an in-area concept
// dropped by relevance would vanish from page 1 entirely. The in-area union
// recovers those regardless of backbone, and the backbone predicate then keeps
// the requested one. This test pins that combination with a pool too small to
// hold the match set.
func TestSuggest_BackboneKeepsInAreaBeyondPool(t *testing.T) {
	db := openTestDB(t)
	seedMajorityWCVPMinorityCDM(t, db)
	ctx := context.Background()
	mustTx(t, db.BuildDistributionClosure(ctx))

	orig := suggestMatchPool
	t.Cleanup(func() { suggestMatchPool = orig })

	// Smaller than the five matching names, so the pool must drop some — and
	// the four WCVP rows were indexed first.
	suggestMatchPool = 1

	items, err := db.Suggest(ctx, "Zzq", output.SuggestOpts{Limit: 20, Area: "ZZZ", Backbone: "cdm"})
	mustTx(t, err)

	found := false
	for _, it := range items {
		if !strings.HasPrefix(it.ConceptID, "cdm:") {
			t.Errorf("concept %q leaked into a Backbone=cdm result", it.ConceptID)
		}
		if it.ConceptID == "cdm:concept:zzq-e" {
			found = true
			if !it.InArea {
				t.Error("recovered concept has InArea=false, want true")
			}
		}
	}
	if !found {
		t.Errorf("Suggest(pool=1, area=ZZZ, backbone=cdm) = %+v, want the in-area CDM concept recovered via the in-area union", items)
	}
}

// TestSuggest_BackbonePoolStillBoundsWork pins that the backbone filter does
// not disable the pool cap: the cap is what keeps a broad prefix from ranking
// and grouping the whole index (the 502 this pool exists to prevent).
func TestSuggest_BackbonePoolStillBoundsWork(t *testing.T) {
	db := openTestDB(t)
	seedMajorityWCVPMinorityCDM(t, db)
	ctx := context.Background()

	orig := suggestMatchPool
	t.Cleanup(func() { suggestMatchPool = orig })

	suggestMatchPool = 2
	items, err := db.Suggest(ctx, "Zzq", output.SuggestOpts{Limit: 20, Backbone: "wcvp"})
	mustTx(t, err)
	if len(items) > 2 {
		t.Errorf("pool=2, backbone=wcvp: got %d concepts, want at most the pool cap of 2", len(items))
	}
}

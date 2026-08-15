package sqlite

import (
	"context"
	"testing"

	"github.com/jobrunner/hostus/internal/domain"
	"github.com/jobrunner/hostus/internal/ports/output"
)

// seedThreePrefixConcepts writes three WCVP concepts whose accepted names all
// share the rare prefix "Zzq" (so a "Zzq" prefix query matches exactly them,
// three distinct fts_name rows -> three groups).
func seedThreePrefixConcepts(t *testing.T, db *DB) {
	bv := domain.BackboneVersion{ID: "wcvp", Version: "v1", IngestedAt: "2026-08-15T00:00:00Z", ManifestSHA: "x"}
	ingestVia(t, db, bv, func(tx output.IngestTx) {
		for _, s := range []struct{ id, canonical string }{
			{"wcvp:concept:zzq-a", "Zzqaaa aaa"},
			{"wcvp:concept:zzq-b", "Zzqbbb bbb"},
			{"wcvp:concept:zzq-c", "Zzqccc ccc"},
		} {
			n := species("n-"+s.id, s.canonical)
			mustTx(t, tx.UpsertName(n))
			c := domain.Concept{ID: s.id, BackboneID: "wcvp", AcceptedName: n, Rank: domain.RankSpecies, Status: domain.StatusAccepted}
			mustTx(t, tx.UpsertConcept(c))
			mustTx(t, tx.LinkName(c.ID, n.ID, "accepted", nil))
		}
	})
}

func suggestCount(t *testing.T, db *DB, q string) int {
	t.Helper()
	items, err := db.Suggest(context.Background(), q, output.SuggestOpts{Limit: 20})
	mustTx(t, err)
	return len(items)
}

// seedTwoInAreaOneOut writes three WCVP concepts matching "Zzq": two with own
// distribution in area "ZZZ", one with none. All three share the prefix so a
// "Zzq" query matches all; only A and B are in ZZZ.
func seedTwoInAreaOneOut(t *testing.T, db *DB) {
	bv := domain.BackboneVersion{ID: "wcvp", Version: "v1", IngestedAt: "2026-08-15T00:00:00Z", ManifestSHA: "x"}
	ingestVia(t, db, bv, func(tx output.IngestTx) {
		for _, s := range []struct {
			id, canonical string
			inArea        bool
		}{
			{"wcvp:concept:zzq-a", "Zzqaaa aaa", true},
			{"wcvp:concept:zzq-b", "Zzqbbb bbb", true},
			{"wcvp:concept:zzq-c", "Zzqccc ccc", false},
		} {
			n := species("n-"+s.id, s.canonical)
			mustTx(t, tx.UpsertName(n))
			c := domain.Concept{ID: s.id, BackboneID: "wcvp", AcceptedName: n, Rank: domain.RankSpecies, Status: domain.StatusAccepted}
			mustTx(t, tx.UpsertConcept(c))
			mustTx(t, tx.LinkName(c.ID, n.ID, "accepted", nil))
			if s.inArea {
				mustTx(t, tx.AddDistribution(c.ID, domain.Distribution{AreaScheme: "wgsrpd_l3", AreaCode: "ZZZ"}))
			}
		}
	})
}

func suggestInAreaCount(t *testing.T, db *DB, q, area string) int {
	t.Helper()
	items, err := db.Suggest(context.Background(), q, output.SuggestOpts{Area: area, Limit: 20})
	mustTx(t, err)
	n := 0
	for _, it := range items {
		if it.InArea {
			n++
		}
	}
	return n
}

// TestSuggest_MatchPoolKeepsInAreaBeyondPool pins the recall guarantee: even
// when the bm25 pool is far smaller than the number of matches, every in-area
// concept must still surface (in_area is the primary rank key, so an in-area
// hit dropped by a bm25-only cap would silently vanish from page 1 — the sparse
// -area regression the review caught). With pool=1 the two in-area concepts A
// and B must BOTH come back via the in-area union; a bm25-only cap would return
// at most one concept total.
func TestSuggest_MatchPoolKeepsInAreaBeyondPool(t *testing.T) {
	db := openTestDB(t)
	seedTwoInAreaOneOut(t, db)

	orig := suggestMatchPool
	t.Cleanup(func() { suggestMatchPool = orig })

	suggestMatchPool = 1
	if n := suggestInAreaCount(t, db, "Zzq", "ZZZ"); n != 2 {
		t.Errorf("pool=1, area=ZZZ: got %d in-area concepts, want both (2) recovered via the in-area union", n)
	}
}

// TestSuggest_MatchPoolBoundsBroadPrefix pins the perf guard: the matches CTE
// keeps at most suggestMatchPool FTS rows, so a prefix matching more names than
// the pool yields at most that many concepts. With a pool large enough to hold
// all matches the result is unbounded (all three), proving the cap — not some
// other filter — is what limits the small-pool case.
func TestSuggest_MatchPoolBoundsBroadPrefix(t *testing.T) {
	db := openTestDB(t)
	seedThreePrefixConcepts(t, db)

	orig := suggestMatchPool
	t.Cleanup(func() { suggestMatchPool = orig })

	suggestMatchPool = 5000
	if n := suggestCount(t, db, "Zzq"); n != 3 {
		t.Fatalf("pool=5000: Suggest(\"Zzq\") returned %d concepts, want all 3", n)
	}

	suggestMatchPool = 2
	if n := suggestCount(t, db, "Zzq"); n != 2 {
		t.Errorf("pool=2: Suggest(\"Zzq\") returned %d concepts, want the pool cap of 2", n)
	}
}

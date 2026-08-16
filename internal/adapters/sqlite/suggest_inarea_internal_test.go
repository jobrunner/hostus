package sqlite

import (
	"context"
	"testing"

	"github.com/jobrunner/hostus/internal/domain"
	"github.com/jobrunner/hostus/internal/ports/output"
)

func mustTx(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

// ingestVia runs one backbone's writes in a single ingest tx and finalizes it.
func ingestVia(t *testing.T, db *DB, bv domain.BackboneVersion, build func(tx output.IngestTx)) {
	t.Helper()
	tx, err := db.BeginIngest(context.Background(), bv)
	mustTx(t, err)
	build(tx)
	mustTx(t, tx.Finalize())
	mustTx(t, tx.Commit())
}

func species(id, canonical string) domain.Name {
	return domain.Name{ID: id, Canonical: canonical, Rank: domain.RankSpecies}
}

// seedWCVPInulaHirta writes an accepted WCVP "Pentanema hirtum" (distributed in
// GER) with "Inula hirta" as a synonym pointing at it.
func seedWCVPInulaHirta(t *testing.T, db *DB) {
	bv := domain.BackboneVersion{ID: "wcvp", Version: "v1", IngestedAt: "2026-08-14T00:00:00Z", ManifestSHA: "x"}
	ingestVia(t, db, bv, func(tx output.IngestTx) {
		accepted := species("n-pentanema-hirtum", "Pentanema hirtum")
		synonym := species("n-inula-hirta-wcvp", "Inula hirta")
		mustTx(t, tx.UpsertName(accepted))
		mustTx(t, tx.UpsertName(synonym))
		c := domain.Concept{ID: "wcvp:concept:pentanema-hirtum", BackboneID: "wcvp", AcceptedName: accepted, Rank: domain.RankSpecies, Status: domain.StatusAccepted}
		mustTx(t, tx.UpsertConcept(c))
		mustTx(t, tx.LinkName(c.ID, accepted.ID, "accepted", nil))
		mustTx(t, tx.LinkName(c.ID, synonym.ID, "synonym", nil))
		mustTx(t, tx.AddDistribution(c.ID, domain.Distribution{AreaScheme: "wgsrpd_l3", AreaCode: "GER"}))
	})
}

// seedCDMInulaHirta writes CDM "Inula hirta" sec. concepts plus a control:
//   - inula-hirta: no distribution of its own -> in_area decided by WCVP name.
//   - inula-hirta-fra: has its OWN distribution (FRA, not GER) -> the NOT EXISTS
//     guard must keep it out of GER even though the WCVP name twin IS in GER.
//   - zzz-nowcvp: a name WCVP does not carry -> no positive evidence.
func seedCDMInulaHirta(t *testing.T, db *DB) {
	bv := domain.BackboneVersion{ID: "cdm", Version: "v1", IngestedAt: "2026-08-14T00:00:00Z", ManifestSHA: "x"}
	ingestVia(t, db, bv, func(tx output.IngestTx) {
		mustTx(t, tx.UpsertSecReference(domain.SecReference{ID: "sec-x", Title: "Fl. X"}))
		want := []domain.Concept{
			{ID: "cdm:concept:inula-hirta", AcceptedName: species("n-inula-hirta-cdm", "Inula hirta")},
			{ID: "cdm:concept:inula-hirta-fra", AcceptedName: species("n-inula-hirta-fra", "Inula hirta")},
			{ID: "cdm:concept:zzz-nowcvp", AcceptedName: species("n-zzz-nowcvp", "Zzz nowcvp")},
		}
		for _, c := range want {
			mustTx(t, tx.UpsertName(c.AcceptedName))
			c.BackboneID, c.Rank, c.Status, c.SecReference = "cdm", domain.RankSpecies, domain.StatusAccepted, "sec-x"
			mustTx(t, tx.UpsertConcept(c))
			mustTx(t, tx.LinkName(c.ID, c.AcceptedName.ID, "accepted", nil))
		}
		mustTx(t, tx.AddDistribution("cdm:concept:inula-hirta-fra", domain.Distribution{AreaScheme: "wgsrpd_l3", AreaCode: "FRA"}))
	})
}

func suggestInArea(t *testing.T, db *DB, q, conceptID string) bool {
	t.Helper()
	items, err := db.Suggest(context.Background(), q, output.SuggestOpts{Area: "GER", Limit: 20})
	mustTx(t, err)
	for _, it := range items {
		if it.ConceptID == conceptID {
			return it.InArea
		}
	}
	t.Fatalf("Suggest(%q) returned no item for %q (got %d items)", q, conceptID, len(items))
	return false
}

// TestSuggest_InArea_DerivedFromWCVPNameForSecConcept pins the presence-only
// semantics: a CDM sec. concept carries no distribution of its own, so in_area
// is decided by whether the SAME name is in the area on a WCVP concept
// (accepted or synonym). "Inula hirta" is a WCVP synonym of the GER-distributed
// "Pentanema hirtum", so both the WCVP hit AND the CDM "Inula hirta" concept
// report in_area=true; a CDM concept whose name WCVP does not place in the area
// reports false ("keine Angabe" in the UI, never a false "nein").
func TestSuggest_InArea_DerivedFromWCVPNameForSecConcept(t *testing.T) {
	db := openTestDB(t)
	seedWCVPInulaHirta(t, db)
	seedCDMInulaHirta(t, db)
	mustTx(t, db.BuildDistributionClosure(context.Background()))

	if !suggestInArea(t, db, "Inula hirta", "wcvp:concept:pentanema-hirtum") {
		t.Error("WCVP Pentanema hirtum: in_area=false, want true (own GER distribution)")
	}
	if !suggestInArea(t, db, "Inula hirta", "cdm:concept:inula-hirta") {
		t.Error("CDM Inula hirta: in_area=false, want true (via WCVP name)")
	}
	if suggestInArea(t, db, "Inula hirta", "cdm:concept:inula-hirta-fra") {
		t.Error("CDM Inula hirta (own FRA distribution): in_area=true for GER, want false — own distribution guards against the WCVP-name fallback")
	}
	if suggestInArea(t, db, "Zzz nowcvp", "cdm:concept:zzz-nowcvp") {
		t.Error("CDM Zzz nowcvp: in_area=true, want false (no WCVP twin -> keine Angabe)")
	}
}

// seedSparseCDMTwin writes a WCVP "Foo bar" with own distribution in the
// sparse area "ZZ" plus a CDM "Foo bar" sec. concept with no distribution of
// its own — mirrors seedTwoInAreaOneOut but for the CDM name-fallback (now
// closure "name" origin) case.
func seedSparseCDMTwin(t *testing.T, db *DB) {
	bv := domain.BackboneVersion{ID: "wcvp", Version: "v1", IngestedAt: "2026-08-16T00:00:00Z", ManifestSHA: "x"}
	ingestVia(t, db, bv, func(tx output.IngestTx) {
		n := species("n-wcvp-foo-bar", "Foo bar")
		mustTx(t, tx.UpsertName(n))
		c := domain.Concept{ID: "wcvp:concept:foo-bar", BackboneID: "wcvp", AcceptedName: n, Rank: domain.RankSpecies, Status: domain.StatusAccepted}
		mustTx(t, tx.UpsertConcept(c))
		mustTx(t, tx.LinkName(c.ID, n.ID, "accepted", nil))
		mustTx(t, tx.AddDistribution(c.ID, domain.Distribution{AreaScheme: "wgsrpd_l3", AreaCode: "ZZ"}))
	})

	bvCDM := domain.BackboneVersion{ID: "cdm", Version: "v1", IngestedAt: "2026-08-16T00:00:00Z", ManifestSHA: "x"}
	ingestVia(t, db, bvCDM, func(tx output.IngestTx) {
		mustTx(t, tx.UpsertSecReference(domain.SecReference{ID: "sec-foo", Title: "Fl. Foo"}))
		n := species("n-cdm-foo-bar", "Foo bar")
		mustTx(t, tx.UpsertName(n))
		c := domain.Concept{ID: "cdm:concept:foo-bar", BackboneID: "cdm", AcceptedName: n, Rank: domain.RankSpecies, Status: domain.StatusAccepted, SecReference: "sec-foo"}
		mustTx(t, tx.UpsertConcept(c))
		mustTx(t, tx.LinkName(c.ID, n.ID, "accepted", nil))
	})
}

// TestSuggest_InAreaCDMTwinInSparseArea pins the closure recall union for a
// CDM concept: with no own distribution, its in-area evidence comes ONLY from
// distribution_effective's "name" origin row (its WCVP name twin's area). Even
// under a pool of 1 (poor bm25 rank), the in-area union must still surface it
// — the old own-distribution-only in_area_rows union could not, because a CDM
// concept never has a row in `distribution` to be recovered by.
func TestSuggest_InAreaCDMTwinInSparseArea(t *testing.T) {
	db := openTestDB(t)
	seedSparseCDMTwin(t, db)
	mustTx(t, db.BuildDistributionClosure(context.Background()))

	orig := suggestMatchPool
	t.Cleanup(func() { suggestMatchPool = orig })
	suggestMatchPool = 1

	items, err := db.Suggest(context.Background(), "Foo", output.SuggestOpts{Area: "ZZ", Limit: 20})
	mustTx(t, err)
	for _, it := range items {
		if it.ConceptID == "cdm:concept:foo-bar" {
			if !it.InArea {
				t.Error("CDM Foo bar in ZZ via twin: in_area=false under pool=1 — closure recall union broken")
			}
			return
		}
	}
	t.Fatalf("Suggest(%q) returned no item for %q (got %d items)", "Foo", "cdm:concept:foo-bar", len(items))
}

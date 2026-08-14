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

// seedCDMInulaHirta writes a CDM "Inula hirta" sec. concept (no distribution)
// plus a control concept whose name WCVP does not carry.
func seedCDMInulaHirta(t *testing.T, db *DB) {
	bv := domain.BackboneVersion{ID: "cdm", Version: "v1", IngestedAt: "2026-08-14T00:00:00Z", ManifestSHA: "x"}
	ingestVia(t, db, bv, func(tx output.IngestTx) {
		mustTx(t, tx.UpsertSecReference(domain.SecReference{ID: "sec-x", Title: "Fl. X"}))
		want := []domain.Concept{
			{ID: "cdm:concept:inula-hirta", AcceptedName: species("n-inula-hirta-cdm", "Inula hirta")},
			{ID: "cdm:concept:zzz-nowcvp", AcceptedName: species("n-zzz-nowcvp", "Zzz nowcvp")},
		}
		for _, c := range want {
			mustTx(t, tx.UpsertName(c.AcceptedName))
			c.BackboneID, c.Rank, c.Status, c.SecReference = "cdm", domain.RankSpecies, domain.StatusAccepted, "sec-x"
			mustTx(t, tx.UpsertConcept(c))
			mustTx(t, tx.LinkName(c.ID, c.AcceptedName.ID, "accepted", nil))
		}
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

	if !suggestInArea(t, db, "Inula hirta", "wcvp:concept:pentanema-hirtum") {
		t.Error("WCVP Pentanema hirtum: in_area=false, want true (own GER distribution)")
	}
	if !suggestInArea(t, db, "Inula hirta", "cdm:concept:inula-hirta") {
		t.Error("CDM Inula hirta: in_area=false, want true (via WCVP name)")
	}
	if suggestInArea(t, db, "Zzz nowcvp", "cdm:concept:zzz-nowcvp") {
		t.Error("CDM Zzz nowcvp: in_area=true, want false (no WCVP twin -> keine Angabe)")
	}
}

package sqlite

import (
	"context"
	"strings"
	"testing"

	"github.com/jobrunner/hostus/internal/domain"
	"github.com/jobrunner/hostus/internal/ports/output"
)

// openSeededSuggestDB seeds two backbones ("wcvp", "cdm") with an accepted
// "Inula hirta" species concept each, both indexed into fts_name via
// Finalize (see ingestVia) — enough for buildSuggestQuery's join order to be
// planned realistically, without needing the real, large index.
func openSeededSuggestDB(t *testing.T) *DB {
	t.Helper()
	db := openTestDB(t)

	wcvp := domain.BackboneVersion{ID: "wcvp", Version: "v1", IngestedAt: "2026-09-05T00:00:00Z", ManifestSHA: "x"}
	ingestVia(t, db, wcvp, func(tx output.IngestTx) {
		n := species("n-wcvp-inula-hirta", "Inula hirta")
		mustTx(t, tx.UpsertName(n))
		c := domain.Concept{ID: "wcvp:concept:inula-hirta", BackboneID: "wcvp", AcceptedName: n, Rank: domain.RankSpecies, Status: domain.StatusAccepted}
		mustTx(t, tx.UpsertConcept(c))
		mustTx(t, tx.LinkName(c.ID, n.ID, "accepted", nil))
	})

	cdm := domain.BackboneVersion{ID: "cdm", Version: "v1", IngestedAt: "2026-09-05T00:00:00Z", ManifestSHA: "y"}
	ingestVia(t, db, cdm, func(tx output.IngestTx) {
		n := species("n-cdm-inula-hirta", "Inula hirta")
		mustTx(t, tx.UpsertName(n))
		c := domain.Concept{ID: "cdm:concept:inula-hirta", BackboneID: "cdm", AcceptedName: n, Rank: domain.RankSpecies, Status: domain.StatusAccepted}
		mustTx(t, tx.UpsertConcept(c))
		mustTx(t, tx.LinkName(c.ID, n.ID, "accepted", nil))
	})

	return db
}

// explainPlan runs EXPLAIN QUERY PLAN against query/args and joins every
// plan row's "detail" column with "\n", so a single strings.Contains check
// can scan the whole plan regardless of how many rows SQLite reports.
func explainPlan(t *testing.T, db *DB, query string, args []any) string {
	t.Helper()
	rows, err := db.sql.QueryContext(context.Background(), "EXPLAIN QUERY PLAN "+query, args...)
	if err != nil {
		t.Fatalf("EXPLAIN QUERY PLAN: unexpected error: %v", err)
	}
	defer func() { _ = rows.Close() }()

	var details []string
	for rows.Next() {
		var id, parent, notused int
		var detail string
		if err := rows.Scan(&id, &parent, &notused, &detail); err != nil {
			t.Fatalf("scanning EXPLAIN QUERY PLAN row: %v", err)
		}
		details = append(details, detail)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterating EXPLAIN QUERY PLAN rows: %v", err)
	}
	return strings.Join(details, "\n")
}

// TestSuggestQueryPlanDoesNotScanBackboneIndex pins the fix for the
// Synology 502 (spec 2026-09-05): with entry_backbone set, SQLite flipped
// the join order onto idx_taxon_concept_backbone_id — scanning EVERY
// concept of the backbone (440k for wcvp) and running the correlated
// name_start EXISTS per concept: measured 6.96s vs 0.0023s for identical
// results. The unary + on the term disables index use for it and restores
// the FTS-driven order. Plan choice in SQLite (without ANALYZE) is
// structural, so the assertion holds on a small seeded DB too — proven by
// the control assertion below, which shows the UNFIXED form picking the
// index on this very database (without it, this test could go blind if a
// future SQLite version changed planning).
func TestSuggestQueryPlanDoesNotScanBackboneIndex(t *testing.T) {
	db := openSeededSuggestDB(t)

	opts := output.SuggestOpts{Limit: 10, Backbone: "wcvp"}
	query, args, ok := buildSuggestQuery("inula hirta", opts)
	if !ok {
		t.Fatal("buildSuggestQuery returned ok=false for a valid query")
	}
	plan := explainPlan(t, db, query, args)
	if strings.Contains(plan, "idx_taxon_concept_backbone_id") {
		t.Errorf("suggest plan drives from idx_taxon_concept_backbone_id again (backbone scan, the Synology-502 shape):\n%s", plan)
	}

	// KONTROLLE: die Vor-Fix-Form MUSS den Index wählen, sonst ist die
	// Assertion oben nicht beweiskräftig.
	unfixed := strings.Replace(query, "+tc.backbone_id", "tc.backbone_id", 1)
	if unfixed == query {
		t.Fatal("fixed query does not contain +tc.backbone_id — fix missing or renamed")
	}
	controlPlan := explainPlan(t, db, unfixed, args)
	if !strings.Contains(controlPlan, "idx_taxon_concept_backbone_id") {
		t.Fatalf("control (unfixed) plan does not use the backbone index — the assertion above proves nothing on this database:\n%s", controlPlan)
	}
}

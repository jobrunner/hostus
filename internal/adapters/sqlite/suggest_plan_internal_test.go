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
	// Positive counterpart to the absence check above (symmetry with
	// TestAttachTargetSpaceNamesQueryPlanDoesNotScanSpace): "SCAN m" is
	// SQLite's stable EQP wording for driving off the materialized `matches`
	// CTE (the FTS5 match set), i.e. the join order this fix restores.
	if !strings.Contains(plan, "SCAN m") {
		t.Errorf("suggest plan does not drive from the FTS-matched `matches` CTE (want a \"SCAN m\" row):\n%s", plan)
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

// TestAttachTargetSpaceNamesQueryPlanDoesNotScanSpace pins the SECOND
// instance of the same planner trap (spec 2026-09-05, fix round 1): the
// Synology-form gate ("< 50ms") still failed with target_space=eurosl
// because attachTargetSpaceNames' query drove from
// sqlite_autoindex_name_space_entry_1 (name_space_entry's PRIMARY KEY
// (space, ext_id)) on `space = ?`, scanning every row of the requested
// space (116k for eurosl) instead of the handful of concept ids already
// selected by Suggest via idx_name_space_entry_concept_id — measured 0.455s
// vs 0.001s for an identical 5-row result. See the backboneFilter doc
// comment in suggest.go for the full mechanism; targetSpaceQuery's own doc
// comment states the measured values for this second instance.
func TestAttachTargetSpaceNamesQueryPlanDoesNotScanSpace(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	bv := domain.BackboneVersion{ID: "wcvp", Version: "v1", IngestedAt: "2026-09-05T00:00:00Z", ManifestSHA: "x"}
	ingestVia(t, db, bv, func(tx output.IngestTx) {
		n := species("n-wcvp-inula-hirta", "Inula hirta")
		mustTx(t, tx.UpsertName(n))
		c := domain.Concept{ID: "wcvp:concept:inula-hirta", BackboneID: "wcvp", AcceptedName: n, Rank: domain.RankSpecies, Status: domain.StatusAccepted}
		mustTx(t, tx.UpsertConcept(c))
		mustTx(t, tx.LinkName(c.ID, n.ID, "accepted", nil))
	})

	tx, err := db.BeginIngest(ctx, domain.BackboneVersion{ID: "eurosl-src", Version: "v1", IngestedAt: "2026-09-05T00:00:00Z", ManifestSHA: "y"})
	mustTx(t, err)
	mustTx(t, tx.UpsertNameSpace(domain.NameSpaceMeta{ID: "eurosl", Version: "v1", ManifestSHA: "y", Redistribution: domain.RedistributionUnknown}))
	mustTx(t, tx.AddNameSpaceEntry("wcvp:concept:inula-hirta", domain.NameSpaceEntry{
		Space: "eurosl", ExtID: "e-1", Name: "Inula hirta", Status: "accepted",
	}))
	mustTx(t, tx.Commit())

	idsJSON, err := marshalIDs([]string{"wcvp:concept:inula-hirta"})
	if err != nil {
		t.Fatalf("marshalIDs: unexpected error: %v", err)
	}
	args := []any{"eurosl", idsJSON}

	plan := explainPlan(t, db, targetSpaceQuery, args)
	if strings.Contains(plan, "sqlite_autoindex_name_space_entry_1") {
		t.Errorf("target-space plan drives from the (space, ext_id) primary-key autoindex again (space scan, the fix-round-1 shape):\n%s", plan)
	}
	if !strings.Contains(plan, "idx_name_space_entry_concept_id") {
		t.Errorf("target-space plan does not use idx_name_space_entry_concept_id:\n%s", plan)
	}

	// KONTROLLE: die Vor-Fix-Form MUSS den PK-Autoindex wählen, sonst ist die
	// Assertion oben nicht beweiskräftig.
	unfixed := strings.Replace(targetSpaceQuery, "+space", "space", 1)
	if unfixed == targetSpaceQuery {
		t.Fatal("fixed query does not contain +space — fix missing or renamed")
	}
	controlPlan := explainPlan(t, db, unfixed, args)
	if !strings.Contains(controlPlan, "sqlite_autoindex_name_space_entry_1") {
		t.Fatalf("control (unfixed) plan does not use the primary-key autoindex — the assertion above proves nothing on this database:\n%s", controlPlan)
	}
}

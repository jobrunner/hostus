package sqlite

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildDistributionClosure(t *testing.T) {
	db := openTestDB(t)
	// WCVP: accepted "Pentanema hirtum" in GER, synonym "Inula hirta".
	seedWCVPInulaHirta(t, db)
	// CDM: "Inula hirta" (no own dist, twin via name) + "Zzz nowcvp" (no twin).
	seedCDMInulaHirta(t, db)
	if err := db.BuildDistributionClosure(context.Background()); err != nil {
		t.Fatal(err)
	}
	// WCVP own-distribution concept -> 'own' row for GER.
	if got := effRows(t, db, "wcvp:concept:pentanema-hirtum"); got != "GER:own" {
		t.Errorf("wcvp pentanema: got %q, want GER:own", got)
	}
	// CDM concept with a WCVP twin in GER -> 'name' row for GER.
	if got := effRows(t, db, "cdm:concept:inula-hirta"); got != "GER:name" {
		t.Errorf("cdm inula-hirta: got %q, want GER:name", got)
	}
	// CDM concept whose name WCVP does not carry -> no rows.
	if got := effRows(t, db, "cdm:concept:zzz-nowcvp"); got != "" {
		t.Errorf("cdm zzz-nowcvp: got %q, want none", got)
	}
}

// TestOpenDoesNotBuildClosure pins the serve-startup fix: Open must NEVER build
// distribution_effective. `hostus serve` opens the DB before it binds its
// listener, so a heavy build here blocks (and can OOM-kill) the container before
// it ever listens or logs — the reverse proxy then sees no upstream. The closure
// is a build artifact, rebuilt only at ingest via BuildDistributionClosure. A DB
// opened with an empty closure must stay empty (in_area falls back to false)
// rather than trigger a build on Open.
func TestOpenDoesNotBuildClosure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "noheal.sqlite")
	db := openTestDBAt(t, path)
	seedWCVPInulaHirta(t, db)
	seedCDMInulaHirta(t, db)
	// Simulate a DB whose closure was never built (pre-C2 or a bundle):
	// distribution present, distribution_effective empty.
	if _, err := db.sql.ExecContext(context.Background(), `DELETE FROM distribution_effective`); err != nil {
		t.Fatal(err)
	}
	mustTx(t, db.Close())

	// Re-open: must NOT build the closure (would block/OOM serve startup).
	db2, err := Open(path)
	mustTx(t, err)
	t.Cleanup(func() { _ = db2.Close() })
	var n int
	mustTx(t, db2.sql.QueryRowContext(context.Background(),
		`SELECT count(*) FROM distribution_effective`).Scan(&n))
	if n != 0 {
		t.Errorf("Open built %d distribution_effective rows; want 0 (Open must not build the closure)", n)
	}
	// And the build path still works when called explicitly (ingest path).
	mustTx(t, db2.BuildDistributionClosure(context.Background()))
	if got := effRows(t, db2, "cdm:concept:inula-hirta"); got != "GER:name" {
		t.Errorf("after explicit BuildDistributionClosure: got %q, want GER:name", got)
	}
}

// effRows returns "CODE:origin,CODE:origin" (sorted) for a concept's
// distribution_effective rows, "" if none.
func effRows(t *testing.T, db *DB, conceptID string) string {
	t.Helper()
	rows, err := db.sql.QueryContext(context.Background(),
		`SELECT area_code, origin FROM distribution_effective
		 WHERE concept_id = ? ORDER BY area_scheme, area_code`, conceptID)
	mustTx(t, err)
	defer func() { _ = rows.Close() }()
	var parts []string
	for rows.Next() {
		var code, origin string
		mustTx(t, rows.Scan(&code, &origin))
		parts = append(parts, code+":"+origin)
	}
	mustTx(t, rows.Err())
	return strings.Join(parts, ",")
}

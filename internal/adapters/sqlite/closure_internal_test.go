package sqlite

import (
	"context"
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

package sqlite

import (
	"context"
	"fmt"
	"testing"

	"github.com/jobrunner/hostus/internal/domain"
)

// TestFTSUnicode61Parity_MatchesCanonicalize is the golden parity test
// carried forward from Task 1's review: domain.Canonicalize hand-rolls a
// diacritic-fold table (to stay free of third-party Unicode-normalization
// deps), and it must agree with what SQLite's real
// `unicode61 remove_diacritics 2` tokenizer folds at index time — the exact
// tokenizer fts_name uses (schema.sql) — or comparison keys computed in Go
// and keys computed by the database will diverge.
//
// For each accented/base rune pair below, it seeds the REAL fts_name table
// (via the documented fts_name_map rowid mapping) with a token containing
// the accented rune, queries it with the plain-ASCII base rune, and
// compares "did FTS5 fold this?" against "does domain.Canonicalize fold
// this?". They must always agree.
func TestFTSUnicode61Parity_MatchesCanonicalize(t *testing.T) {
	// A representative sample spanning the fully-decomposable Latin
	// diacritics domain.Canonicalize DOES fold (ä ö ü é ç), plus the four
	// non-decomposable letters carried forward from the Task 1 review that
	// it deliberately does NOT fold (ß ł ø đ).
	pairs := []struct {
		accented, base rune
	}{
		{'ä', 'a'},
		{'ö', 'o'},
		{'ü', 'u'},
		{'é', 'e'},
		{'ç', 'c'},
		{'ß', 's'},
		{'ł', 'l'},
		{'ø', 'o'},
		{'đ', 'd'},
	}

	db := openTestDB(t)
	conceptID := seedProbeConcept(t, db)

	for i, p := range pairs {
		accentedToken := fmt.Sprintf("x%cx%d", p.accented, i) // suffix keeps tokens from colliding across pairs sharing a base letter
		rowID := insertFTSName(t, db, conceptID, accentedToken)

		queryToken := fmt.Sprintf("x%cx%d", p.base, i)
		ftsFolds := ftsNameMatches(t, db, queryToken, rowID)
		domainFolds := domain.Canonicalize(string(p.accented)) == string(p.base)

		if ftsFolds != domainFolds {
			t.Errorf("rune %q: unicode61 remove_diacritics=2 folds=%v, domain.Canonicalize folds=%v (want equal) — reconcile diacriticFold table",
				p.accented, ftsFolds, domainFolds)
		}
	}
}

// seedProbeConcept inserts a minimal, real taxon_concept row (via the
// ordinary ingest path) so fts_name_map's FK on concept_id has something
// valid to reference, and returns its id.
func seedProbeConcept(t *testing.T, db *DB) string {
	t.Helper()
	ctx := context.Background()
	tx, err := db.BeginIngest(ctx, domain.BackboneVersion{ID: "probe", Version: "v1", IngestedAt: "2026-07-31T00:00:00Z", ManifestSHA: "x"})
	if err != nil {
		t.Fatalf("BeginIngest: %v", err)
	}
	n := domain.Name{ID: "probe-name", Canonical: "probe", Rank: domain.RankSpecies}
	if err := tx.UpsertName(n); err != nil {
		t.Fatalf("UpsertName: %v", err)
	}
	c := domain.Concept{ID: "probe-concept", BackboneID: "probe", AcceptedName: n, Rank: domain.RankSpecies, Status: domain.StatusAccepted}
	if err := tx.UpsertConcept(c); err != nil {
		t.Fatalf("UpsertConcept: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	return c.ID
}

// insertFTSName seeds one row into fts_name via the fts_name_map rowid
// mapping documented in schema.sql and returns the assigned rowid.
func insertFTSName(t *testing.T, db *DB, conceptID, canonical string) int64 {
	t.Helper()
	res, err := db.sql.Exec(`INSERT INTO fts_name_map (concept_id) VALUES (?)`, conceptID)
	if err != nil {
		t.Fatalf("inserting fts_name_map row: %v", err)
	}
	rowID, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("reading last insert id: %v", err)
	}
	if _, err := db.sql.Exec(`INSERT INTO fts_name (rowid, canonical) VALUES (?, ?)`, rowID, canonical); err != nil {
		t.Fatalf("inserting fts_name row: %v", err)
	}
	return rowID
}

// ftsNameMatches reports whether querying fts_name for query returns the
// row with the given rowid.
func ftsNameMatches(t *testing.T, db *DB, query string, rowID int64) bool {
	t.Helper()
	var n int
	err := db.sql.QueryRow(`SELECT count(*) FROM fts_name WHERE fts_name MATCH ? AND rowid = ?`, query, rowID).Scan(&n)
	if err != nil {
		t.Fatalf("querying fts_name MATCH %q: %v", query, err)
	}
	return n > 0
}

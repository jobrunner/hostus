// Package main is a throwaway PoC probe (P1) verifying that
// modernc.org/sqlite (pure-Go SQLite driver) supports FTS5 with
// prefix queries and bm25 ranking. See ../P01-findings.md for the
// verdict. NOT part of the hostus service module.
package main

import (
	"database/sql"
	"fmt"
	"log"

	_ "modernc.org/sqlite"
)

func must(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

func run(db *sql.DB, label, query string) {
	fmt.Printf("\n-- %s --\nSQL: %s\n", label, query)
	rows, err := db.Query(query)
	if err != nil {
		fmt.Printf("ERROR: %v\n", err)
		return
	}
	defer rows.Close()

	cols, _ := rows.Columns()
	fmt.Println("columns:", cols)
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		must(rows.Scan(ptrs...))
		fmt.Println(vals...)
	}
	must(rows.Err())
}

func main() {
	db, err := sql.Open("sqlite", ":memory:")
	must(err)
	defer db.Close()

	// 1. Create FTS5 virtual table with unicode61 tokenizer + diacritics folding.
	_, err = db.Exec(`CREATE VIRTUAL TABLE t USING fts5(
		canonical,
		tokenize = 'unicode61 remove_diacritics 2'
	)`)
	if err != nil {
		fmt.Println("FTS5 VIRTUAL TABLE CREATE FAILED:", err)
		fmt.Println("=> FTS5 is likely NOT compiled into this modernc.org/sqlite build.")
		return
	}
	fmt.Println("FTS5 virtual table created successfully.")

	// 2. Seed data.
	seed := []string{
		"Corynephorus canescens",
		"Corynephorus divaricatus",
		"Festuca ovina",
		"Silene otites",
		// Extra row with a diacritic to test remove_diacritics.
		"Cortaderia selloana",    // decoy, no diacritic but similar prefix
		"Ceratophyllum démersum", // fictitious diacritic form to test folding
	}
	stmt, err := db.Prepare(`INSERT INTO t (canonical) VALUES (?)`)
	must(err)
	for _, s := range seed {
		_, err = stmt.Exec(s)
		must(err)
	}
	must(stmt.Close())

	// 3. Prefix query using implicit `rank` column (bm25 by default).
	run(db, "prefix match 'coryn*' ordered by implicit rank",
		`SELECT canonical, rank FROM t WHERE t MATCH 'coryn*' ORDER BY rank`)

	// 4. Prefix query using explicit bm25(t) function.
	run(db, "prefix match 'coryn*' ordered by explicit bm25(t)",
		`SELECT canonical, bm25(t) AS score FROM t WHERE t MATCH 'coryn*' ORDER BY score`)

	// 5. Full match sanity check (non-prefix).
	run(db, "exact token match 'ovina'",
		`SELECT canonical, rank FROM t WHERE t MATCH 'ovina' ORDER BY rank`)

	// 6. remove_diacritics test: search "demersum" (no accent) and expect it
	// to match the seeded "démersum" (with accent) row.
	run(db, "remove_diacritics: search 'demersum' (no accent) against accented row",
		`SELECT canonical, rank FROM t WHERE t MATCH 'demersum' ORDER BY rank`)

	// 7. remove_diacritics with prefix: "demer*"
	run(db, "remove_diacritics + prefix: search 'demer*'",
		`SELECT canonical, rank FROM t WHERE t MATCH 'demer*' ORDER BY rank`)

	fmt.Println("\nDone.")
}

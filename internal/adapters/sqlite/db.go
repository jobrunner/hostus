package sqlite

import (
	"context"
	"database/sql"
	_ "embed"
	"errors"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite" // pure-Go SQLite driver (CGO-free, FTS5 built in) — see ADR-0010

	"github.com/jobrunner/hostus/internal/domain"
	"github.com/jobrunner/hostus/internal/ports/output"
)

//go:embed schema.sql
var schemaSQL string

// DB is a modernc.org/sqlite-backed output.Repository.
type DB struct {
	sql *sql.DB
}

var _ output.Repository = (*DB)(nil)

// OpenPool opens path with a pool of up to maxConns connections. Open's
// single-connection contract exists for WRITERS and for ":memory:" — the
// serve path is read-only on a WAL database, which is exactly the
// concurrent-reader case WAL supports, and one connection serialized every
// request behind whichever query happened to run (measured 2026-09-05:
// keystroke bursts on /v1/suggest queued on the single connection until the
// reverse proxy answered 502; spec 2026-09-05-serve-read-pool-loadshed.md).
// The per-connection pragmas (_journal_mode, _busy_timeout) come from the
// DSN and therefore apply to every pooled connection.
//
// ":memory:" always uses ONE connection regardless of maxConns — each
// physical connection gets its own private, empty in-memory database, so a
// second connection would see a database missing every table the first one
// just created. maxConns < 1 falls back to 1.
//
// The single-connection case (":memory:", or any WRITING caller via Open)
// also matters for `PRAGMA foreign_keys=ON` (set by schema.sql, below):
// enforcement is per-connection in SQLite, not database-wide, so with a
// multi-connection pool only the connection that happened to run the
// schema would have it on, and any other pooled connection would silently
// accept FK-violating writes. Neither concern applies to a genuine
// read-only pool: no pooled connection ever writes, so there is nothing for
// FK enforcement to guard and no schema-application race to lose — but the
// guard above still pins ":memory:" to one connection regardless of
// caller, since a read-only pool against an ephemeral, never-ingested
// database is not a real use case this codebase has.
//
// Operational note: if an ingest is running concurrently against the same
// file, multiple open read cursors from this pool can delay ingest's WAL
// checkpoint (the -wal file grows until every reader releases its
// snapshot) — stop serve, or lower maxConns, for the duration of a long
// ingest against a live serve database.
func OpenPool(path string, maxConns int) (*DB, error) {
	if path == ":memory:" || maxConns < 1 {
		maxConns = 1
	}
	// journal_mode=WAL lets a concurrent READER (e.g. serve's Suggest/Match
	// queries, opened via its own *DB) proceed while an ingest writer holds
	// a write transaction open — the default rollback-journal mode instead
	// locks the whole database for the duration of a write, blocking every
	// reader until it commits. busy_timeout bounds how long any lock wait
	// (a reader momentarily blocked by a writer's checkpoint, or the
	// reverse) may block before giving up with SQLITE_BUSY, rather than
	// failing immediately. Both are set via the modernc.org/sqlite DSN
	// shorthand so they apply at connection-open time, before schema.sql
	// runs any DDL below.
	dsn := path + "?_journal_mode=WAL&_busy_timeout=5000"
	sqlDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("sqlite: open %q: %w", path, err)
	}
	sqlDB.SetMaxOpenConns(maxConns)
	// database/sql's idle-pool default is 2: without raising it to match,
	// a pool sized above 2 would open connections 3+ under a burst and then
	// CLOSE them the moment they go idle, only to physically reopen (and
	// reapply the DSN's per-connection pragmas) on the next burst — exactly
	// the connection churn under keystroke-burst load a read pool exists to
	// avoid. For maxConns==1 this is behaviorally neutral (an idle limit of
	// 1 does not evict the one connection the pool ever holds), so it does
	// not change Open's single-connection contract.
	sqlDB.SetMaxIdleConns(maxConns)
	if _, err := sqlDB.ExecContext(context.Background(), schemaSQL); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("sqlite: applying schema: %w", err)
	}
	if err := migrateXrefSourceColumn(context.Background(), sqlDB); err != nil {
		_ = sqlDB.Close()
		return nil, err
	}
	if err := migrateConceptRelationPK(context.Background(), sqlDB); err != nil {
		_ = sqlDB.Close()
		return nil, err
	}
	if err := migrateNameSpaceEntryStatus(context.Background(), sqlDB); err != nil {
		_ = sqlDB.Close()
		return nil, err
	}
	if err := migrateTaxonConceptClassification(context.Background(), sqlDB); err != nil {
		_ = sqlDB.Close()
		return nil, err
	}
	if err := verifySchemaColumns(context.Background(), sqlDB); err != nil {
		_ = sqlDB.Close()
		return nil, err
	}
	// NOTE: Open must NOT build distribution_effective here. `hostus serve`
	// calls Open on startup BEFORE binding the HTTP listener (see
	// app.openRepo), so any heavy work here blocks — and, for the closure's
	// multi-million-row join+insert, can OOM-kill — the container before it
	// ever listens or logs a line (the reverse proxy then sees no upstream).
	// The closure is a build artifact: it is (re)built at ingest time by
	// application/app.Ingest via BuildDistributionClosure, never on the serve
	// path. A DB opened without a closure simply reports in_area=false until
	// it is re-ingested (fail-safe), which is vastly preferable to a serve
	// that will not start.
	return &DB{sql: sqlDB}, nil
}

// Open opens path pinned to exactly one physical connection — the contract
// every WRITING caller (ingest, bundle, export) and every ":memory:" test
// relies on. See OpenPool for the read-pool variant the serve path uses.
func Open(path string) (*DB, error) { return OpenPool(path, 1) }

// verifySchemaColumns fails Open if any table in the just-opened database is
// missing a column the current embedded schema declares for it. It exists
// because the schema is applied with CREATE TABLE IF NOT EXISTS, which adds a
// missing TABLE to an old database but never a missing COLUMN to a table that
// already exists — so an index built by an older hostus silently lacks columns
// later releases added (e.g. trait_value.resolution, added in SP3), and the
// first query selecting one fails at RUNTIME with an opaque 500 while
// /health/ready still reports 200. The two ad-hoc migrations above heal the
// specific columns they know about; this catches every other drift, present
// and future, and turns it into a loud, named STARTUP failure — a missing
// column is a startup problem, not a per-request one.
//
// The expected shape is read from the embedded schema itself, applied to a
// throwaway in-memory database, so there is no second hand-maintained column
// list to drift out of sync: whatever schema.sql declares today is exactly
// what is required. The check is one-directional — every expected column must
// be present, but extra columns are fine — so an older binary opening a NEWER
// database (more columns than it knows) still works.
//
// It compares column NAMES only, not types/nullability/PK shape: the drift it
// targets is a column an older build never created at all (the SP3
// trait_value.resolution case), which is a name-level absence. A legacy column
// of the wrong type is out of scope and not detected.
func verifySchemaColumns(ctx context.Context, sqlDB *sql.DB) error {
	expected, tables, err := expectedSchemaColumns(ctx)
	if err != nil {
		return err
	}

	var problems []string
	for _, table := range tables {
		actualCols, err := tableColumns(ctx, sqlDB, table)
		if err != nil {
			return err
		}
		actual := make(map[string]bool, len(actualCols))
		for _, c := range actualCols {
			actual[c] = true
		}
		var missing []string
		for _, col := range expected[table] {
			if !actual[col] {
				missing = append(missing, col)
			}
		}
		if len(missing) > 0 {
			problems = append(problems, fmt.Sprintf("%s (missing %s)", table, strings.Join(missing, ", ")))
		}
	}
	if len(problems) > 0 {
		return fmt.Errorf("sqlite: database schema is out of date — %s; re-ingest with the current hostus into a FRESH database file (`hostus ingest` reopens this same path and hits this check again), or add the column(s) in place by hand (a value absent from a legacy row is correct as NULL)", strings.Join(problems, "; "))
	}
	return nil
}

// expectedSchemaColumns applies the embedded schema to a throwaway in-memory
// database and reads back, per user table, the columns it declares. It returns
// the columns keyed by table plus the table names in a deterministic
// (name-sorted) order, so verifySchemaColumns reports drift the same way every
// run.
//
// "User table" excludes both the fts_name virtual table (its row is
// `CREATE VIRTUAL TABLE ...`) and the FTS5 shadow tables it spawns (their
// sqlite_master.sql quotes the table name: `CREATE TABLE 'fts_name_data'(...)`)
// via the `sql LIKE 'CREATE TABLE ' || name || '%'` filter, which only the
// hand-written `CREATE TABLE <bareName> (` statements match. fts_name_map is a
// real table of ours and is (correctly) included.
func expectedSchemaColumns(ctx context.Context) (map[string][]string, []string, error) {
	ref, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		return nil, nil, fmt.Errorf("sqlite: opening schema reference database: %w", err)
	}
	// One connection so the applied schema is visible to the reads below —
	// :memory: is per-connection (see Open's pool comment).
	ref.SetMaxOpenConns(1)
	defer func() { _ = ref.Close() }()
	if _, err := ref.ExecContext(ctx, schemaSQL); err != nil {
		return nil, nil, fmt.Errorf("sqlite: applying schema to reference database: %w", err)
	}

	rows, err := ref.QueryContext(ctx, `
		SELECT name FROM sqlite_master
		WHERE type = 'table' AND sql LIKE 'CREATE TABLE ' || name || '%'
		ORDER BY name`)
	if err != nil {
		return nil, nil, fmt.Errorf("sqlite: listing reference schema tables: %w", err)
	}
	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			_ = rows.Close()
			return nil, nil, fmt.Errorf("sqlite: scanning reference schema table: %w", err)
		}
		tables = append(tables, name)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, nil, fmt.Errorf("sqlite: iterating reference schema tables: %w", err)
	}
	_ = rows.Close()

	expected := make(map[string][]string, len(tables))
	for _, table := range tables {
		cols, err := tableColumns(ctx, ref, table)
		if err != nil {
			return nil, nil, err
		}
		expected[table] = cols
	}
	return expected, tables, nil
}

// tableColumns returns the column names of table in db, in schema (cid) order.
// The table name comes from the embedded schema (never user input), so it is
// interpolated into the pragma call directly.
func tableColumns(ctx context.Context, db *sql.DB, table string) ([]string, error) {
	rows, err := db.QueryContext(ctx, fmt.Sprintf(`SELECT name FROM pragma_table_info('%s')`, table))
	if err != nil {
		return nil, fmt.Errorf("sqlite: reading columns of %q: %w", table, err)
	}
	defer func() { _ = rows.Close() }()

	var cols []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("sqlite: scanning column of %q: %w", table, err)
		}
		cols = append(cols, name)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: iterating columns of %q: %w", table, err)
	}
	return cols, nil
}

// conceptRelationRebuildTable is the scratch name the concept_relation
// primary-key rebuild builds into before renaming. It is referenced in three
// places (recovery, drop-before-create, and the rebuild itself), so it is
// spelled once.
const conceptRelationRebuildTable = "concept_relation_sp5"

// migrateConceptRelationPK widens concept_relation's primary key from
// (from_concept, to_concept, source) to (from_concept, to_concept, relation,
// source) on a database created before SP5. schema.sql cannot do it: its
// CREATE TABLE is IF NOT EXISTS, so an existing table keeps its old key, and
// SQLite cannot alter a primary key in place — the table has to be rebuilt.
//
// Without the widening, two DIFFERENT relation types between the same pair
// of concepts from the same source collide, and AddConceptRelation's
// INSERT OR REPLACE silently keeps only the last one. CDM emits exactly that
// shape (a congruent and a misapplied edge on the same pair).
//
// Rows are carried across rather than dropped, even though concept_relation
// has never been written to before SP5 (schema.sql called it "created here
// but unused"): a migration that quietly discards data is a worse habit than
// one that copies four columns it will usually find empty. Running against a
// fresh database is a no-op.
//
// The rebuild runs INSIDE ONE TRANSACTION. SQLite's DDL is transactional, so
// this is what makes the migration all-or-nothing — and it has to be, because
// both windows of a non-transactional version are reachable and neither is
// recoverable on its own:
//
//   - crashing between the INSERT and the DROP leaves the scratch table
//     behind, and every later Open then fails on "table
//     concept_relation_sp5 already exists" — an unopenable database;
//   - crashing between the DROP and the RENAME leaves NO concept_relation at
//     all, so schema.sql's CREATE TABLE IF NOT EXISTS recreates it empty with
//     the new key, the migration check reports "already migrated", and the
//     rows sit orphaned in the scratch table — silent data loss, the exact
//     outcome this function exists to prevent.
//
// PRAGMA foreign_keys is toggled OUTSIDE the transaction, which is required:
// SQLite silently ignores the pragma while a transaction is open.
func migrateConceptRelationPK(ctx context.Context, sqlDB *sql.DB) error {
	// Foreign keys off for BOTH the recovery and the rebuild: each moves rows
	// between two tables that reference taxon_concept, and enforcement
	// mid-flight would judge an intermediate state rather than the result.
	// Recovery in particular must be inside this window — with enforcement on,
	// a scratch row whose end no longer resolves makes the INSERT fail, and
	// since recovery runs on every Open the database would be permanently
	// unopenable with an opaque driver error: precisely the terminal outcome
	// this whole migration exists to eliminate. Instead both paths finish and
	// then answer to checkConceptRelationForeignKeys, which names the problem.
	//
	// The pragma is toggled OUTSIDE any transaction, which is required:
	// SQLite silently ignores it while one is open. Restored on every path,
	// which matters because Open pins the pool to one connection and the
	// setting is per-connection.
	if _, err := sqlDB.ExecContext(ctx, `PRAGMA foreign_keys = OFF`); err != nil {
		return fmt.Errorf("sqlite: disabling foreign keys for concept_relation migration: %w", err)
	}
	migrateErr := migrateConceptRelationUnenforced(ctx, sqlDB)
	if _, err := sqlDB.ExecContext(ctx, `PRAGMA foreign_keys = ON`); err != nil {
		// errors.Join, not a bare return: if both fail, the migration error
		// is the informative one and must not be dropped for the pragma's.
		return errors.Join(migrateErr,
			fmt.Errorf("sqlite: re-enabling foreign keys after concept_relation migration: %w", err))
	}
	return migrateErr
}

// migrateConceptRelationUnenforced is the body of migrateConceptRelationPK,
// running with foreign-key enforcement already off: first recover from any
// interrupted earlier attempt, then rebuild if the key still needs widening.
func migrateConceptRelationUnenforced(ctx context.Context, sqlDB *sql.DB) error {
	if err := recoverInterruptedConceptRelationRebuild(ctx, sqlDB); err != nil {
		return err
	}
	migrated, err := conceptRelationHasRelationInPK(ctx, sqlDB)
	if err != nil {
		return err
	}
	if migrated {
		return nil
	}
	return rebuildConceptRelation(ctx, sqlDB)
}

// sqlTx is the subset of *sql.Tx / *sql.Conn the migration helpers need, so
// the same code can run inside either. It exists because the migration's
// transactions are opened with an explicit BEGIN IMMEDIATE on a dedicated
// connection rather than via sql.DB.BeginTx (see withImmediateTx).
type sqlTx interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// withImmediateTx runs fn inside a BEGIN IMMEDIATE transaction on a dedicated
// connection, committing if fn returns nil and rolling back otherwise.
//
// BEGIN IMMEDIATE rather than sql.DB.BeginTx's plain BEGIN, because both
// callers READ (pragma_table_info, sqlite_master) and then WRITE. Under WAL a
// plain BEGIN takes its read snapshot at the first statement; if a second
// process commits in between, the upgrade to a write lock fails with
// SQLITE_BUSY_SNAPSHOT, which busy_timeout does NOT retry — the transaction
// simply dies. Nothing is corrupted, but Open would fail with a confusing
// busy error instead of the "sees the finished work and does nothing" outcome
// the re-check below is written to produce. IMMEDIATE takes the write lock up
// front, so the loser waits (busy_timeout applies) and then reads a snapshot
// that already includes the winner's commit.
func withImmediateTx(ctx context.Context, sqlDB *sql.DB, what string, fn func(tx sqlTx) error) error {
	conn, err := sqlDB.Conn(ctx)
	if err != nil {
		return fmt.Errorf("sqlite: %s: acquiring connection: %w", what, err)
	}
	defer func() { _ = conn.Close() }()

	if _, err := conn.ExecContext(ctx, `BEGIN IMMEDIATE`); err != nil {
		return fmt.Errorf("sqlite: %s: beginning transaction: %w", what, err)
	}
	if err := fn(conn); err != nil {
		_, _ = conn.ExecContext(ctx, `ROLLBACK`)
		return err
	}
	if _, err := conn.ExecContext(ctx, `COMMIT`); err != nil {
		_, _ = conn.ExecContext(ctx, `ROLLBACK`)
		return fmt.Errorf("sqlite: %s: committing: %w", what, err)
	}
	return nil
}

// rebuildConceptRelation performs the rebuild itself, in one transaction.
func rebuildConceptRelation(ctx context.Context, sqlDB *sql.DB) error {
	return withImmediateTx(ctx, sqlDB, "migrating concept_relation primary key", func(tx sqlTx) error {
		// Re-check the condition INSIDE the transaction. Open pins its own
		// pool to one connection, but nothing stops a second PROCESS from
		// opening the same file concurrently; whichever transaction gets
		// there second must see the finished work and do nothing rather than
		// rebuild a table that is already correct.
		var pk int
		err := tx.QueryRowContext(ctx, `SELECT pk FROM pragma_table_info('concept_relation') WHERE name = 'relation'`).Scan(&pk)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("sqlite: re-checking concept_relation primary key: %w", err)
		}
		if pk > 0 {
			return nil
		}

		stmts := []string{
			// Defensive: a scratch table left by a pre-transaction migration
			// is normally cleared by
			// recoverInterruptedConceptRelationRebuild, but CREATE must never
			// be the statement that discovers one.
			`DROP TABLE IF EXISTS ` + conceptRelationRebuildTable,
			`CREATE TABLE ` + conceptRelationRebuildTable + ` (
				from_concept  TEXT NOT NULL REFERENCES taxon_concept(id),
				to_concept    TEXT NOT NULL REFERENCES taxon_concept(id),
				relation      TEXT NOT NULL,
				source        TEXT,
				PRIMARY KEY (from_concept, to_concept, relation, source)
			)`,
			`INSERT OR REPLACE INTO ` + conceptRelationRebuildTable + ` (from_concept, to_concept, relation, source)
				SELECT from_concept, to_concept, relation, source FROM concept_relation`,
			`DROP TABLE concept_relation`,
			`ALTER TABLE ` + conceptRelationRebuildTable + ` RENAME TO concept_relation`,
			`CREATE INDEX IF NOT EXISTS idx_concept_relation_to_concept ON concept_relation(to_concept)`,
		}
		for _, stmt := range stmts {
			if _, err := tx.ExecContext(ctx, stmt); err != nil {
				return fmt.Errorf("sqlite: migrating concept_relation primary key: %w", err)
			}
		}

		// Foreign keys were off for the rebuild, so nothing checked the
		// copied rows. A legacy row whose end no longer resolves must be
		// reported, not carried across as a dangling edge that the next
		// enforced write would trip over somewhere far from here.
		return checkConceptRelationForeignKeys(ctx, tx)
	})
}

// checkConceptRelationForeignKeys runs PRAGMA foreign_key_check over the
// rebuilt table and fails if any row has an unresolvable end. The pragma is
// an explicit checker and works regardless of whether enforcement is on.
func checkConceptRelationForeignKeys(ctx context.Context, tx sqlTx) error {
	rows, err := tx.QueryContext(ctx, `PRAGMA foreign_key_check('concept_relation')`)
	if err != nil {
		return fmt.Errorf("sqlite: checking concept_relation foreign keys after migration: %w", err)
	}
	defer func() { _ = rows.Close() }()

	violations := 0
	for rows.Next() {
		violations++
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("sqlite: checking concept_relation foreign keys after migration: %w", err)
	}
	if violations > 0 {
		return fmt.Errorf("sqlite: refusing to migrate concept_relation: %d row(s) reference a taxon_concept that does not exist", violations)
	}
	return nil
}

// recoverInterruptedConceptRelationRebuild cleans up after a rebuild
// interrupted by the PRE-TRANSACTION version of this migration. The current
// version cannot produce either state, but a database written by a hostus
// build that shipped the earlier one can already be in it, and Open must not
// be the thing that makes such a database unusable.
//
// Both crash windows leave the same artifact — a leftover scratch table — and
// one recovery handles both. Its rows are folded back into concept_relation
// with INSERT OR IGNORE and the scratch table is dropped:
//
//   - crash before the DROP: concept_relation still holds every one of those
//     rows, so the insert is a no-op and the normal migration then runs;
//   - crash after the DROP: concept_relation was recreated empty by
//     schema.sql, so the insert is what restores the data.
//
// INSERT OR IGNORE rather than OR REPLACE: the live table is authoritative
// where the two disagree.
func recoverInterruptedConceptRelationRebuild(ctx context.Context, sqlDB *sql.DB) error {
	var name string
	err := sqlDB.QueryRowContext(ctx,
		`SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?`, conceptRelationRebuildTable).Scan(&name)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("sqlite: looking for an interrupted concept_relation migration: %w", err)
	}

	return withImmediateTx(ctx, sqlDB, "recovering interrupted concept_relation migration", func(tx sqlTx) error {
		for _, stmt := range []string{
			`INSERT OR IGNORE INTO concept_relation (from_concept, to_concept, relation, source)
				SELECT from_concept, to_concept, relation, source FROM ` + conceptRelationRebuildTable,
			`DROP TABLE ` + conceptRelationRebuildTable,
		} {
			if _, err := tx.ExecContext(ctx, stmt); err != nil {
				return fmt.Errorf("sqlite: recovering interrupted concept_relation migration: %w", err)
			}
		}
		// The fold-back ran with enforcement off (see migrateConceptRelationPK),
		// so a scratch row whose end no longer resolves would otherwise be
		// carried across unchecked. Reported here with the same named error the
		// rebuild produces, instead of an opaque constraint failure — and, since
		// the whole thing is one transaction, a refusal leaves the database
		// exactly as it was rather than half-recovered.
		return checkConceptRelationForeignKeys(ctx, tx)
	})
}

// conceptRelationHasRelationInPK reports whether concept_relation's `relation`
// column is already part of the table's primary key (pragma_table_info's `pk`
// is the 1-based position within the key, 0 for a non-key column).
func conceptRelationHasRelationInPK(ctx context.Context, sqlDB *sql.DB) (bool, error) {
	var pk int
	err := sqlDB.QueryRowContext(ctx, `SELECT pk FROM pragma_table_info('concept_relation') WHERE name = 'relation'`).Scan(&pk)
	if errors.Is(err, sql.ErrNoRows) {
		// No such column/table: schema.sql has just created the current
		// shape, or this database predates the table entirely. Either way
		// there is nothing to rebuild.
		return true, nil
	}
	if err != nil {
		return false, fmt.Errorf("sqlite: inspecting concept_relation primary key: %w", err)
	}
	return pk > 0, nil
}

// migrateXrefSourceColumn adds xref.source to a database created before that
// column existed (SP1–SP3). schema.sql alone cannot do this: its CREATE TABLE
// is IF NOT EXISTS, so an already-existing xref table is left exactly as it
// was, and SQLite has no "ADD COLUMN IF NOT EXISTS". The column is nullable
// with no default, which is the one ALTER TABLE form SQLite can apply
// in-place to a table carrying a REFERENCES clause — so an existing index
// keeps every xref row it had, with source NULL (correctly meaning "not
// attributable to any ingested xref source"; see schema.sql). Running against
// a fresh database is a no-op, since schema.sql already created the column.
// addColumnIfMissing runs ALTER TABLE ... ADD COLUMN unless the column is
// already there. CREATE TABLE IF NOT EXISTS adds a missing TABLE to an old
// database but never a missing COLUMN, and verifySchemaColumns turns any such
// gap into a startup failure — so every column added after a release needs one
// of these to keep existing indexes openable.
func addColumnIfMissing(ctx context.Context, sqlDB *sql.DB, table, column, definition string) error {
	rows, err := sqlDB.QueryContext(ctx,
		`SELECT 1 FROM pragma_table_info(?) WHERE name = ?`, table, column)
	if err != nil {
		return fmt.Errorf("sqlite: checking for %s.%s column: %w", table, column, err)
	}
	present := rows.Next()
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return fmt.Errorf("sqlite: checking for %s.%s column: %w", table, column, err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("sqlite: checking for %s.%s column: %w", table, column, err)
	}
	if present {
		return nil
	}
	// Table, column and definition are compile-time constants from the call
	// sites below, never caller input — ALTER TABLE takes no placeholders for
	// identifiers, so this is the only way to express it.
	if _, err := sqlDB.ExecContext(ctx, "ALTER TABLE "+table+" ADD COLUMN "+column+" "+definition); err != nil {
		return fmt.Errorf("sqlite: adding %s.%s column: %w", table, column, err)
	}
	return nil
}

func migrateXrefSourceColumn(ctx context.Context, sqlDB *sql.DB) error {
	return addColumnIfMissing(ctx, sqlDB, "xref", "source", "TEXT REFERENCES xref_source(id)")
}

// migrateNameSpaceEntryStatus adds name_space_entry.status to an index built
// before it existed. Without it verifySchemaColumns would refuse to open such
// a database at all, since the embedded schema now declares the column and
// CREATE TABLE IF NOT EXISTS never adds a column to an existing table.
//
// Existing rows keep ” — "status not recorded" — which ResolveTargetSpace
// treats as "fall back to the previous behavior", not as "not accepted". Only
// a re-ingest fills it, and only then do target-space names become
// determinate.
func migrateNameSpaceEntryStatus(ctx context.Context, sqlDB *sql.DB) error {
	return addColumnIfMissing(ctx, sqlDB, "name_space_entry", "status", "TEXT NOT NULL DEFAULT ''")
}

// migrateTaxonConceptClassification adds taxon_concept.family/order_name/
// class_name to an index built before they existed. Without it
// verifySchemaColumns would refuse to open such a database at all, since the
// embedded schema now declares the columns and CREATE TABLE IF NOT EXISTS
// never adds a column to an existing table.
//
// Existing rows keep NULL — "classification not recorded" — which mirrors
// the schema's own NULL-means-unknown rule for these columns (never guessed).
// Only a re-ingest fills them.
func migrateTaxonConceptClassification(ctx context.Context, sqlDB *sql.DB) error {
	if err := addColumnIfMissing(ctx, sqlDB, "taxon_concept", "family", "TEXT"); err != nil {
		return fmt.Errorf("sqlite: migrating taxon_concept.family: %w", err)
	}
	if err := addColumnIfMissing(ctx, sqlDB, "taxon_concept", "order_name", "TEXT"); err != nil {
		return fmt.Errorf("sqlite: migrating taxon_concept.order_name: %w", err)
	}
	if err := addColumnIfMissing(ctx, sqlDB, "taxon_concept", "class_name", "TEXT"); err != nil {
		return fmt.Errorf("sqlite: migrating taxon_concept.class_name: %w", err)
	}
	return nil
}

// Close releases the underlying database handle.
func (db *DB) Close() error {
	return db.sql.Close()
}

// MaxOpenConns reports the connection pool size this *DB was opened with
// (db.sql.Stats().MaxOpenConnections). It exists ONLY as a test seam for
// asserting the app package's serve-side OpenPool wiring end to end — the
// output.Repository port stays intentionally narrow and does not declare
// this method; callers reach it via a type assertion to *sqlite.DB in a
// test, never through the port.
func (db *DB) MaxOpenConns() int {
	return db.sql.Stats().MaxOpenConnections
}

// BackboneVersions lists every ingested backbone artifact.
func (db *DB) BackboneVersions(ctx context.Context) ([]domain.BackboneVersion, error) {
	rows, err := db.sql.QueryContext(ctx, `
		SELECT id, version, license, source_url, ingested_at, manifest_sha, redistribution
		FROM backbone_version
		ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("sqlite: querying backbone_version: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []domain.BackboneVersion
	for rows.Next() {
		var bv domain.BackboneVersion
		var license, sourceURL sql.NullString
		var redistribution string
		if err := rows.Scan(&bv.ID, &bv.Version, &license, &sourceURL, &bv.IngestedAt, &bv.ManifestSHA, &redistribution); err != nil {
			return nil, fmt.Errorf("sqlite: scanning backbone_version row: %w", err)
		}
		bv.License = license.String
		bv.SourceURL = sourceURL.String
		bv.Redistribution = domain.Redistribution(redistribution)
		out = append(out, bv)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: iterating backbone_version rows: %w", err)
	}
	return out, nil
}

// BeginIngest records bv into backbone_version and starts a transaction
// scoping every write of one backbone import, so a failed/partial ingest
// never leaves the index half-written. Callers must Commit or Rollback the
// returned IngestTx.
//
// PRAGMA defer_foreign_keys=ON pushes every FK check in this transaction
// from per-statement to commit time. Real native-space source data is NOT
// reliably parent-before-child ordered — measured on the real GermanSL
// canonical CSV, 6,567 of 15,083 parent_id references (43.5%) point at a
// row that appears LATER in the file — so writeNativeRow's sequential
// insert-with-parent_id-already-set would otherwise fail immediately on a
// forward reference, even though the referenced parent DOES get written
// later in this same transaction. The pragma is session-scoped and SQLite
// resets it to OFF automatically at commit/rollback, so it never leaks into
// a later transaction on this connection.
func (db *DB) BeginIngest(ctx context.Context, bv domain.BackboneVersion) (output.IngestTx, error) {
	tx, err := db.sql.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("sqlite: beginning ingest transaction: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `PRAGMA defer_foreign_keys = ON`); err != nil {
		_ = tx.Rollback()
		return nil, fmt.Errorf("sqlite: enabling defer_foreign_keys: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT OR REPLACE INTO backbone_version (id, version, license, source_url, ingested_at, manifest_sha, redistribution)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		bv.ID, bv.Version, bv.License, bv.SourceURL, bv.IngestedAt, bv.ManifestSHA, string(bv.Redistribution),
	); err != nil {
		_ = tx.Rollback()
		return nil, fmt.Errorf("sqlite: recording backbone_version %q: %w", bv.ID, err)
	}
	return &ingestTx{ctx: ctx, tx: tx, backboneID: bv.ID}, nil
}

// BeginTraitIngest starts a transaction for one trait-vocabulary import
// WITHOUT touching backbone_version — see output.Repository.BeginTraitIngest
// for why a trait vocabulary must never be recorded as a backbone. The
// returned IngestTx carries an empty backboneID, which makes Finalize a
// no-op: its query filters on tc.backbone_id, and no taxon_concept belongs
// to the empty backbone, so a trait ingest never (re)writes the FTS index it
// did not contribute names to.
func (db *DB) BeginTraitIngest(ctx context.Context) (output.IngestTx, error) {
	tx, err := db.sql.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("sqlite: beginning trait ingest transaction: %w", err)
	}
	return &ingestTx{ctx: ctx, tx: tx, backboneID: ""}, nil
}

// ingestTx implements output.IngestTx over a single *sql.Tx.
type ingestTx struct {
	ctx        context.Context
	tx         *sql.Tx
	backboneID string
}

var _ output.IngestTx = (*ingestTx)(nil)

// nullableFK converts an optional foreign-key value into a driver value
// that SQLite treats as NULL when empty, rather than as the literal empty
// string. This matters for nullable FK columns (name.basionym_id,
// taxon_concept.parent_id): SQLite only exempts NULL child keys from FK
// checking, so writing "" instead of NULL would fail the constraint unless
// a row with id="" happens to exist.
func nullableFK(id string) any {
	if id == "" {
		return nil
	}
	return id
}

// nullString converts an optional (non-FK) string column value into a
// driver value that SQLite stores as NULL when empty, rather than as a
// literal empty string — used for name.rank_verbatim/
// taxon_concept.rank_verbatim, which are empty exactly when the rank is
// canonical (nothing to preserve) and set only for RankOther rows, and for
// name.published_in/name.nom_status, whose schema contract is likewise
// "NULL when the source recorded nothing" (schema.sql).
func nullString(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// conceptExists reports whether id is a known taxon_concept, so a reader can
// distinguish "concept exists but has none of what was asked for" (empty
// slice, nil error) from "concept unknown" (domain.ErrNotFound).
func (db *DB) conceptExists(ctx context.Context, id string) (bool, error) {
	var one int
	err := db.sql.QueryRowContext(ctx, `SELECT 1 FROM taxon_concept WHERE id = ?`, id).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func (t *ingestTx) UpsertName(n domain.Name) error {
	_, err := t.tx.ExecContext(t.ctx, `
		INSERT OR REPLACE INTO name (id, canonical, canonical_fold, authorship, rank, ipni_id, published_in, nom_status, basionym_id, rank_verbatim)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		n.ID, n.Canonical, domain.Canonicalize(n.Canonical), n.Authorship, string(n.Rank), n.IPNIID, nullString(n.PublishedIn), nullString(n.NomStatus), nullableFK(n.BasionymID), nullString(n.RankVerbatim),
	)
	if err != nil {
		return fmt.Errorf("sqlite: upserting name %q: %w", n.ID, err)
	}
	return nil
}

func (t *ingestTx) UpsertConcept(c domain.Concept) error {
	_, err := t.tx.ExecContext(t.ctx, `
		INSERT OR REPLACE INTO taxon_concept (id, backbone_id, accepted_name, rank, parent_id, sec_reference, status, rank_verbatim)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		c.ID, c.BackboneID, c.AcceptedName.ID, string(c.Rank), nullableFK(c.ParentID), c.SecReference, string(c.Status), nullString(c.RankVerbatim),
	)
	if err != nil {
		return fmt.Errorf("sqlite: upserting concept %q: %w", c.ID, err)
	}
	return nil
}

func (t *ingestTx) LinkName(conceptID, nameID, role string, homotypic *bool) error {
	var h any
	if homotypic != nil {
		if *homotypic {
			h = 1
		} else {
			h = 0
		}
	}
	_, err := t.tx.ExecContext(t.ctx, `
		INSERT OR REPLACE INTO concept_name (concept_id, name_id, role, homotypic)
		VALUES (?, ?, ?, ?)`,
		conceptID, nameID, role, h,
	)
	if err != nil {
		return fmt.Errorf("sqlite: linking name %q to concept %q: %w", nameID, conceptID, err)
	}
	return nil
}

// AddXref writes one xref row for conceptID, attributed to the xref_source
// named by source. source is "" (stored as SQL NULL via nullableFK, since it
// is a nullable FK onto xref_source) for xrefs the backbone ingest derives
// from a taxon row — see schema.sql's note on why those are deliberately
// unattributed.
func (t *ingestTx) AddXref(conceptID string, x domain.Xref, source string) error {
	_, err := t.tx.ExecContext(t.ctx, `
		INSERT OR REPLACE INTO xref (concept_id, authority, ext_id, source)
		VALUES (?, ?, ?, ?)`,
		conceptID, x.Authority, x.ExtID, nullableFK(source),
	)
	if err != nil {
		return fmt.Errorf("sqlite: adding xref %s:%s for concept %q: %w", x.Authority, x.ExtID, conceptID, err)
	}
	return nil
}

func (t *ingestTx) AddDistribution(conceptID string, d domain.Distribution) error {
	_, err := t.tx.ExecContext(t.ctx, `
		INSERT OR REPLACE INTO distribution (concept_id, area_scheme, area_code)
		VALUES (?, ?, ?)`,
		conceptID, d.AreaScheme, d.AreaCode,
	)
	if err != nil {
		return fmt.Errorf("sqlite: adding distribution %s/%s for concept %q: %w", d.AreaScheme, d.AreaCode, conceptID, err)
	}
	return nil
}

// nameCanonicalPair is one (concept_id, name.canonical) row collected by
// Finalize before it starts writing fts_name/fts_name_map, so the write
// loop never runs concurrently with an open *sql.Rows on the same
// transaction.
type nameCanonicalPair struct {
	conceptID string
	canonical string
}

// Finalize builds the FTS5 autosuggest index for every name this
// transaction's backbone has linked to a concept — both the accepted name
// and every synonym, via concept_name — so a prefix search on a synonym's
// canonical resolves back to its accepted concept (fts_name_map only
// stores concept_id, not which specific name matched, so Suggest always
// reports the accepted name's own canonical/rank/status; indexing the
// synonym text here is what makes it findABLE at all).
//
// Known limitation: fts_name is a contentless FTS5 table (content=”,
// schema.sql), and contentless tables reject plain DELETE ("cannot DELETE
// from contentless fts5 table") unless the table opts into
// contentless_delete=1, which schema.sql does not set. Finalize therefore
// cannot clean up a backbone's previously-indexed rows before re-adding
// them, so re-running Ingest for the same backbone_id (BeginIngest itself
// is documented as INSERT OR REPLACE, a supported operation) will append a
// second set of fts_name/fts_name_map rows alongside the first rather than
// replacing them. This does not affect Suggest's correctness — it
// GROUP BYs on tc.id, so duplicate index entries for the same concept
// simply collapse back into one result — only the index's on-disk size
// under repeated re-ingestion of the same backbone.
func (t *ingestTx) Finalize() error {
	rows, err := t.tx.QueryContext(t.ctx, `
		SELECT cn.concept_id, n.canonical
		FROM concept_name cn
		JOIN name n ON n.id = cn.name_id
		JOIN taxon_concept tc ON tc.id = cn.concept_id
		WHERE tc.backbone_id = ?`, t.backboneID)
	if err != nil {
		return fmt.Errorf("sqlite: querying concept_name for FTS indexing (backbone %q): %w", t.backboneID, err)
	}
	var pairs []nameCanonicalPair
	for rows.Next() {
		var p nameCanonicalPair
		if err := rows.Scan(&p.conceptID, &p.canonical); err != nil {
			_ = rows.Close()
			return fmt.Errorf("sqlite: scanning concept_name row for FTS indexing (backbone %q): %w", t.backboneID, err)
		}
		pairs = append(pairs, p)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return fmt.Errorf("sqlite: iterating concept_name rows for FTS indexing (backbone %q): %w", t.backboneID, err)
	}
	_ = rows.Close()

	for _, p := range pairs {
		res, err := t.tx.ExecContext(t.ctx, `INSERT INTO fts_name_map (concept_id) VALUES (?)`, p.conceptID)
		if err != nil {
			return fmt.Errorf("sqlite: inserting fts_name_map for concept %q: %w", p.conceptID, err)
		}
		rowID, err := res.LastInsertId()
		if err != nil {
			return fmt.Errorf("sqlite: reading fts_name_map rowid for concept %q: %w", p.conceptID, err)
		}
		// vernacular_de is left empty: SP1 does not ingest vernacular
		// names yet (see the task brief); a later ingest can populate it
		// once that data source exists.
		if _, err := t.tx.ExecContext(t.ctx, `INSERT INTO fts_name (rowid, canonical, vernacular_de) VALUES (?, ?, '')`, rowID, p.canonical); err != nil {
			return fmt.Errorf("sqlite: inserting fts_name for concept %q: %w", p.conceptID, err)
		}
	}
	return nil
}

// UpsertSecReference records one sec. reference space (SP5). The title is
// written verbatim; there is no normalisation, because a citation IS its
// spelling.
func (t *ingestTx) UpsertSecReference(s domain.SecReference) error {
	_, err := t.tx.ExecContext(t.ctx, `
		INSERT OR REPLACE INTO sec_reference (id, title)
		VALUES (?, ?)`,
		s.ID, s.Title,
	)
	if err != nil {
		return fmt.Errorf("sqlite: upserting sec reference %q: %w", s.ID, err)
	}
	return nil
}

// AddConceptRelation writes one typed concept relation. Both ends are FKs
// onto taxon_concept, so this will fail (rather than write a dangling edge)
// if either concept is missing — application.IngestCDM resolves both ends
// before the transaction is opened precisely so that never happens.
//
// The row is written in the direction given; the inverse is never
// synthesized (see domain.Relation.Inverse).
func (t *ingestTx) AddConceptRelation(fromID, toID string, rel domain.Relation, source string) error {
	_, err := t.tx.ExecContext(t.ctx, `
		INSERT OR REPLACE INTO concept_relation (from_concept, to_concept, relation, source)
		VALUES (?, ?, ?, ?)`,
		fromID, toID, string(rel), source,
	)
	if err != nil {
		return fmt.Errorf("sqlite: adding concept relation %s -> %s (%s): %w", fromID, toID, rel, err)
	}
	return nil
}

// UpsertXrefSource records one xref-source provenance row. ingested_at is
// stamped with the current time here rather than taken from meta: provenance/
// timing metadata is orthogonal to the domain-level fields callers construct.
func (t *ingestTx) UpsertXrefSource(meta domain.XrefSourceMeta) error {
	_, err := t.tx.ExecContext(t.ctx, `
		INSERT OR REPLACE INTO xref_source (id, version, license, source_url, ingested_at, manifest_sha, redistribution)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		meta.ID, meta.Version, meta.License, meta.SourceURL, time.Now().UTC().Format(time.RFC3339), meta.ManifestSHA, string(meta.Redistribution),
	)
	if err != nil {
		return fmt.Errorf("sqlite: upserting xref source %s/%s: %w", meta.ID, meta.Version, err)
	}
	return nil
}

func (t *ingestTx) Commit() error {
	if err := t.tx.Commit(); err != nil {
		return fmt.Errorf("sqlite: committing ingest transaction: %w", err)
	}
	return nil
}

func (t *ingestTx) Rollback() error {
	if err := t.tx.Rollback(); err != nil {
		return fmt.Errorf("sqlite: rolling back ingest transaction: %w", err)
	}
	return nil
}

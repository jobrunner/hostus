package sqlite

import (
	"context"
	"database/sql"
	_ "embed"
	"errors"
	"fmt"
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

// Open opens (or creates) the SQLite database at path and applies the
// embedded schema (idempotent — every DDL statement is IF NOT EXISTS).
// path may be ":memory:" for an ephemeral, in-process database, as used by
// tests. Foreign-key enforcement is turned on for the returned connection.
func Open(path string) (*DB, error) {
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
	// SP1 is single-writer, so pin the pool to exactly one physical
	// connection. This matters for two reasons beyond avoiding write
	// contention: `PRAGMA foreign_keys=ON` (set by schema.sql, below) is
	// per-connection in SQLite, not a database-wide setting — with a
	// multi-connection pool, only the connection that happened to run the
	// schema would have FK enforcement on, and any other pooled connection
	// would silently accept FK-violating writes. And for path=":memory:",
	// each physical connection gets its OWN private, empty in-memory
	// database — a second connection would see a database missing every
	// table the first one just created. Capping the pool at one connection
	// makes both FK enforcement and :memory: state deterministic.
	sqlDB.SetMaxOpenConns(1)
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
	return &DB{sql: sqlDB}, nil
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
func migrateXrefSourceColumn(ctx context.Context, sqlDB *sql.DB) error {
	rows, err := sqlDB.QueryContext(ctx, `SELECT 1 FROM pragma_table_info('xref') WHERE name = 'source'`)
	if err != nil {
		return fmt.Errorf("sqlite: checking for xref.source column: %w", err)
	}
	present := rows.Next()
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return fmt.Errorf("sqlite: checking for xref.source column: %w", err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("sqlite: checking for xref.source column: %w", err)
	}
	if present {
		return nil
	}
	if _, err := sqlDB.ExecContext(ctx, `ALTER TABLE xref ADD COLUMN source TEXT REFERENCES xref_source(id)`); err != nil {
		return fmt.Errorf("sqlite: adding xref.source column: %w", err)
	}
	return nil
}

// Close releases the underlying database handle.
func (db *DB) Close() error {
	return db.sql.Close()
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
func (db *DB) BeginIngest(ctx context.Context, bv domain.BackboneVersion) (output.IngestTx, error) {
	tx, err := db.sql.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("sqlite: beginning ingest transaction: %w", err)
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
// canonical (nothing to preserve) and set only for RankOther rows.
func nullString(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func (t *ingestTx) UpsertName(n domain.Name) error {
	_, err := t.tx.ExecContext(t.ctx, `
		INSERT OR REPLACE INTO name (id, canonical, canonical_fold, authorship, rank, ipni_id, published_in, nom_status, basionym_id, rank_verbatim)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		n.ID, n.Canonical, domain.Canonicalize(n.Canonical), n.Authorship, string(n.Rank), n.IPNIID, n.PublishedIn, n.NomStatus, nullableFK(n.BasionymID), nullString(n.RankVerbatim),
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

// nullableFloat converts an optional *float64 into a driver value SQLite
// treats as NULL when the pointer is nil. This is the write-side of the nil
// vs 0.0 distinction domain.TraitValue documents: a nil NicheWidth must
// become SQL NULL, never a literal 0.0.
func nullableFloat(f *float64) any {
	if f == nil {
		return nil
	}
	return *f
}

// nullableInt converts an optional *int into a driver value SQLite treats
// as NULL when the pointer is nil (the int counterpart of nullableFloat,
// for TraitValue.NSystems).
func nullableInt(i *int) any {
	if i == nil {
		return nil
	}
	return *i
}

// AddTraitValue writes one trait_value row for conceptID. tv.NicheWidth and
// tv.NSystems are written as SQL NULL when nil, per the pointer semantics
// domain.TraitValue documents — never coerced to 0/0.0. tv.Resolution
// follows the same "absence is information" rule via nullString: an empty
// Resolution (an exact canonical match) is stored as NULL, not as ”.
func (t *ingestTx) AddTraitValue(conceptID string, tv domain.TraitValue) error {
	_, err := t.tx.ExecContext(t.ctx, `
		INSERT OR REPLACE INTO trait_value (concept_id, vocab, vocab_version, dim, value, niche_width, n_systems, resolution)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		conceptID, string(tv.Vocab), tv.VocabVersion, string(tv.Dim), tv.Value, nullableFloat(tv.NicheWidth), nullableInt(tv.NSystems), nullString(tv.Resolution),
	)
	if err != nil {
		return fmt.Errorf("sqlite: adding trait value %s/%s/%s for concept %q: %w", tv.Vocab, tv.VocabVersion, tv.Dim, conceptID, err)
	}
	return nil
}

// UpsertTraitVocabulary records one (vocab, version) metadata row. ingested_at
// is stamped with the current time here rather than taken from meta (which
// carries no such field) — this mirrors ExportBundle's bundle_meta pattern,
// where provenance/timing metadata is orthogonal to the domain-level fields
// callers construct.
func (t *ingestTx) UpsertTraitVocabulary(meta domain.TraitVocabMeta) error {
	_, err := t.tx.ExecContext(t.ctx, `
		INSERT OR REPLACE INTO trait_vocabulary (vocab, version, taxonomy, license, source_url, ingested_at, redistribution)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		string(meta.Vocab), meta.Version, meta.Taxonomy, meta.License, meta.SourceURL, time.Now().UTC().Format(time.RFC3339), string(meta.Redistribution),
	)
	if err != nil {
		return fmt.Errorf("sqlite: upserting trait vocabulary %s/%s: %w", meta.Vocab, meta.Version, err)
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

// UpsertXrefSource records one xref-source provenance row, the xref
// counterpart of UpsertTraitVocabulary. ingested_at is stamped with the
// current time here for the same reason it is there: provenance/timing
// metadata is orthogonal to the domain-level fields callers construct.
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

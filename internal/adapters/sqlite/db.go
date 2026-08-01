package sqlite

import (
	"context"
	"database/sql"
	_ "embed"
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
	return &DB{sql: sqlDB}, nil
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

func (t *ingestTx) AddXref(conceptID string, x domain.Xref) error {
	_, err := t.tx.ExecContext(t.ctx, `
		INSERT OR REPLACE INTO xref (concept_id, authority, ext_id)
		VALUES (?, ?, ?)`,
		conceptID, x.Authority, x.ExtID,
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

package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// BundleOpts configures ExportBundle.
type BundleOpts struct {
	// Area restricts the bundle to concepts whose distribution intersects
	// this WGSRPD level-3 area code (or one of areaCodes' convenience
	// aliases, e.g. "DE" — the same resolution Suggest's Area option uses).
	// Empty means no filter: every concept in src is copied.
	Area string
	// SnapshotVersion identifies which offline snapshot this bundle was
	// cut from (e.g. "v1"). Recorded verbatim into bundle_meta.
	SnapshotVersion string
	// Now supplies the bundle_meta.created_at timestamp. Defaults to
	// time.Now when nil; tests inject a fixed clock so created_at is
	// deterministic.
	Now func() time.Time
}

// BundleReport summarizes one ExportBundle call.
type BundleReport struct {
	Concepts int
	Names    int
	Areas    int
	Path     string
}

// ExportBundle creates a new, standalone SQLite database at out (same
// embedded schema as Open) containing only the taxonomy BundleOpts.Area
// selects out of src — or all of it, if Area is empty: the matching
// taxon_concept rows, their names/concept_name/xref/distribution/vernacular,
// the backbone_version rows they belong to, and a bundle_meta provenance
// row. The bundle's fts_name/fts_name_map index is rebuilt from the copied
// rows (by reusing ingestTx.Finalize against the bundle connection, not by
// copying fts_name's own rows — fts_name is a contentless FTS5 table and
// cannot be populated via a plain row copy), so the returned file is
// independently queryable via Open + Suggest, not just direct table reads.
func ExportBundle(ctx context.Context, src *DB, out string, opts BundleOpts) (BundleReport, error) {
	conceptIDs, err := scopeConceptIDs(ctx, src, opts.Area)
	if err != nil {
		return BundleReport{}, err
	}

	bundle, err := Open(out)
	if err != nil {
		return BundleReport{}, fmt.Errorf("sqlite: bundle: creating %q: %w", out, err)
	}
	defer func() { _ = bundle.Close() }()

	report, err := populateBundle(ctx, src, bundle, conceptIDs, opts)
	if err != nil {
		return BundleReport{}, err
	}
	report.Path = out
	return report, nil
}

// scopeConceptIDs resolves BundleOpts.Area into the set of taxon_concept
// ids ExportBundle copies: every concept id when area is blank, or every
// concept id with at least one distribution row in one of area's resolved
// WGSRPD level-3 codes (via the same areaCodes alias table Suggest uses)
// otherwise.
func scopeConceptIDs(ctx context.Context, src *DB, area string) ([]string, error) {
	var (
		rows *sql.Rows
		err  error
	)
	if strings.TrimSpace(area) == "" {
		rows, err = src.sql.QueryContext(ctx, `SELECT id FROM taxon_concept ORDER BY id`)
	} else {
		codes := areaCodes(area)
		rows, err = src.sql.QueryContext(ctx, scopeByAreaQuery(len(codes)), idArgs(codes)...)
	}
	if err != nil {
		return nil, fmt.Errorf("sqlite: bundle: resolving concept scope for area %q: %w", area, err)
	}
	defer func() { _ = rows.Close() }()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("sqlite: bundle: scanning concept scope row: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: bundle: iterating concept scope rows: %w", err)
	}
	return ids, nil
}

// scopeByAreaQuery builds the query scopeConceptIDs runs to find every
// concept with a distribution row in one of n area codes. It is its own
// function (taking only an int, not the raw area string) purely so the
// query text it concatenates never has a string-typed parameter in scope —
// n distinct "?" placeholders, never interpolated area text.
func scopeByAreaQuery(n int) string {
	return `
		SELECT DISTINCT tc.id
		FROM taxon_concept tc
		JOIN distribution d ON d.concept_id = tc.id
		WHERE d.area_scheme = 'wgsrpd_l3' AND d.area_code IN (` + placeholdersFor(n) + `)
		ORDER BY tc.id`
}

// placeholdersFor returns n comma-joined "?" placeholders for a SQL IN
// clause.
func placeholdersFor(n int) string {
	ph := make([]string, n)
	for i := range ph {
		ph[i] = "?"
	}
	return strings.Join(ph, ",")
}

// idArgs adapts a []string of ids into the []any driver.Value args
// database/sql's *Context methods take.
func idArgs(ids []string) []any {
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	return args
}

// populateBundle copies every row scoped by conceptIDs from src into
// bundle, rebuilds the FTS index, and writes bundle_meta, in FK-safe
// order: backbone_version, then name (both referenced by taxon_concept),
// then taxon_concept itself, then the concept_id-keyed tables.
func populateBundle(ctx context.Context, src, bundle *DB, conceptIDs []string, opts BundleOpts) (BundleReport, error) {
	var report BundleReport
	if len(conceptIDs) == 0 {
		if err := insertBundleMeta(ctx, bundle, opts, ""); err != nil {
			return report, err
		}
		return report, nil
	}

	placeholders := placeholdersFor(len(conceptIDs))
	args := idArgs(conceptIDs)

	backboneIDs, manifestSHA, err := copyBackboneVersions(ctx, src, bundle, len(conceptIDs), args)
	if err != nil {
		return report, err
	}

	names, err := copyRows(ctx, src, bundle,
		`SELECT DISTINCT n.id, n.canonical, n.canonical_fold, n.authorship, n.rank, n.ipni_id, n.published_in, n.nom_status, n.basionym_id
		 FROM name n
		 JOIN concept_name cn ON cn.name_id = n.id
		 WHERE cn.concept_id IN (`+placeholders+`)`, args,
		`INSERT INTO name (id, canonical, canonical_fold, authorship, rank, ipni_id, published_in, nom_status, basionym_id) VALUES (?,?,?,?,?,?,?,?,?)`)
	if err != nil {
		return report, err
	}
	report.Names = names

	concepts, err := copyRows(ctx, src, bundle,
		`SELECT id, backbone_id, accepted_name, rank, parent_id, sec_reference, status FROM taxon_concept WHERE id IN (`+placeholders+`)`, args,
		`INSERT INTO taxon_concept (id, backbone_id, accepted_name, rank, parent_id, sec_reference, status) VALUES (?,?,?,?,?,?,?)`)
	if err != nil {
		return report, err
	}
	report.Concepts = concepts

	if err := copyConceptScopedTables(ctx, src, bundle, placeholders, args); err != nil {
		return report, err
	}

	for _, backboneID := range backboneIDs {
		if err := rebuildFTS(ctx, bundle, backboneID); err != nil {
			return report, err
		}
	}

	areas, err := countDistinctAreas(ctx, bundle)
	if err != nil {
		return report, err
	}
	report.Areas = areas

	if err := insertBundleMeta(ctx, bundle, opts, manifestSHA); err != nil {
		return report, err
	}
	return report, nil
}

// copyConceptScopedTables copies every remaining concept_id-keyed table
// (concept_name, xref, distribution, vernacular, trait_value) scoped by
// placeholders/args, plus trait_vocabulary in full (metadata, not
// concept-scoped — the offline field app needs the Taxonomy/license/source
// provenance for every trait_value row copied above). Split out of
// populateBundle purely to keep that function's cyclomatic complexity down;
// it carries no logic of its own beyond sequencing copyRows calls.
func copyConceptScopedTables(ctx context.Context, src, bundle *DB, placeholders string, args []any) error {
	if _, err := copyRows(ctx, src, bundle,
		`SELECT concept_id, name_id, role, homotypic FROM concept_name WHERE concept_id IN (`+placeholders+`)`, args,
		`INSERT INTO concept_name (concept_id, name_id, role, homotypic) VALUES (?,?,?,?)`); err != nil {
		return err
	}

	if _, err := copyRows(ctx, src, bundle,
		`SELECT concept_id, authority, ext_id FROM xref WHERE concept_id IN (`+placeholders+`)`, args,
		`INSERT INTO xref (concept_id, authority, ext_id) VALUES (?,?,?)`); err != nil {
		return err
	}

	if _, err := copyRows(ctx, src, bundle,
		`SELECT concept_id, area_scheme, area_code FROM distribution WHERE concept_id IN (`+placeholders+`)`, args,
		`INSERT INTO distribution (concept_id, area_scheme, area_code) VALUES (?,?,?)`); err != nil {
		return err
	}

	if _, err := copyRows(ctx, src, bundle,
		`SELECT concept_id, lang, name, preferred FROM vernacular WHERE concept_id IN (`+placeholders+`)`, args,
		`INSERT INTO vernacular (concept_id, lang, name, preferred) VALUES (?,?,?,?)`); err != nil {
		return err
	}

	if _, err := copyRows(ctx, src, bundle,
		`SELECT concept_id, vocab, vocab_version, dim, value, niche_width, n_systems FROM trait_value WHERE concept_id IN (`+placeholders+`)`, args,
		`INSERT INTO trait_value (concept_id, vocab, vocab_version, dim, value, niche_width, n_systems) VALUES (?,?,?,?,?,?,?)`); err != nil {
		return err
	}

	if _, err := copyRows(ctx, src, bundle,
		`SELECT vocab, version, taxonomy, license, source_url, ingested_at FROM trait_vocabulary`, nil,
		`INSERT INTO trait_vocabulary (vocab, version, taxonomy, license, source_url, ingested_at) VALUES (?,?,?,?,?,?)`); err != nil {
		return err
	}

	return nil
}

// backboneVersionScopeQuery builds the query copyBackboneVersions runs to
// find every backbone_version referenced by n concepts in scope. It is its
// own function (taking only an int, not a raw string) purely so the query
// text it concatenates never has a string-typed parameter in scope — n
// distinct "?" placeholders, never interpolated data.
func backboneVersionScopeQuery(n int) string {
	return `
		SELECT DISTINCT bv.id, bv.version, bv.license, bv.source_url, bv.ingested_at, bv.manifest_sha
		FROM backbone_version bv
		JOIN taxon_concept tc ON tc.backbone_id = bv.id
		WHERE tc.id IN (` + placeholdersFor(n) + `)
		ORDER BY bv.id`
}

// copyBackboneVersions copies every backbone_version row referenced by one
// of the n concepts in scope (bound via args) into bundle, returning the
// distinct backbone ids copied (so populateBundle knows which backbones to
// rebuildFTS for) and the manifest_sha of the last row copied
// (bundle_meta.source_manifest_sha — in practice a single ingest run
// stamps every backbone_version row with the same manifest_sha, so which
// row "wins" does not matter).
func copyBackboneVersions(ctx context.Context, src, bundle *DB, n int, args []any) ([]string, string, error) {
	rows, err := src.sql.QueryContext(ctx, backboneVersionScopeQuery(n), args...)
	if err != nil {
		return nil, "", fmt.Errorf("sqlite: bundle: querying backbone_version scope: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var (
		ids         []string
		manifestSHA string
	)
	for rows.Next() {
		var id, version, ingestedAt, sha string
		var license, sourceURL sql.NullString
		if err := rows.Scan(&id, &version, &license, &sourceURL, &ingestedAt, &sha); err != nil {
			return nil, "", fmt.Errorf("sqlite: bundle: scanning backbone_version row: %w", err)
		}
		if _, err := bundle.sql.ExecContext(ctx, `
			INSERT INTO backbone_version (id, version, license, source_url, ingested_at, manifest_sha)
			VALUES (?, ?, ?, ?, ?, ?)`,
			id, version, license, sourceURL, ingestedAt, sha,
		); err != nil {
			return nil, "", fmt.Errorf("sqlite: bundle: inserting backbone_version %q: %w", id, err)
		}
		ids = append(ids, id)
		manifestSHA = sha
	}
	if err := rows.Err(); err != nil {
		return nil, "", fmt.Errorf("sqlite: bundle: iterating backbone_version rows: %w", err)
	}
	return ids, manifestSHA, nil
}

// copyRows executes query (with args) against src and, for each resulting
// row, executes insertSQL against bundle with the exact same column values
// — query's SELECT list and insertSQL's column list must therefore be
// written in the same order. It returns the number of rows copied. This
// generic row-shuttle (rather than a hand-scanned struct per table) is
// possible because ExportBundle only ever copies a row byte-for-byte, never
// transforms it.
func copyRows(ctx context.Context, src, bundle *DB, query string, args []any, insertSQL string) (int, error) {
	rows, err := src.sql.QueryContext(ctx, query, args...)
	if err != nil {
		return 0, fmt.Errorf("sqlite: bundle: querying %q: %w", query, err)
	}
	defer func() { _ = rows.Close() }()

	cols, err := rows.Columns()
	if err != nil {
		return 0, fmt.Errorf("sqlite: bundle: reading columns for %q: %w", query, err)
	}

	n := 0
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return 0, fmt.Errorf("sqlite: bundle: scanning row for %q: %w", query, err)
		}
		if _, err := bundle.sql.ExecContext(ctx, insertSQL, vals...); err != nil {
			return 0, fmt.Errorf("sqlite: bundle: inserting row via %q: %w", insertSQL, err)
		}
		n++
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("sqlite: bundle: iterating rows for %q: %w", query, err)
	}
	return n, nil
}

// rebuildFTS (re)builds bundle's fts_name/fts_name_map rows for backboneID
// by running the exact same ingestTx.Finalize logic BeginIngest's own
// transaction uses — the copied taxon_concept/concept_name/name rows are
// all Finalize needs, so a bundle's FTS index is byte-for-byte the same
// population logic as a live ingest, not a reimplementation of it.
func rebuildFTS(ctx context.Context, bundle *DB, backboneID string) error {
	tx, err := bundle.sql.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("sqlite: bundle: beginning FTS rebuild for backbone %q: %w", backboneID, err)
	}
	it := &ingestTx{ctx: ctx, tx: tx, backboneID: backboneID}
	if err := it.Finalize(); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("sqlite: bundle: rebuilding FTS for backbone %q: %w", backboneID, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("sqlite: bundle: committing FTS rebuild for backbone %q: %w", backboneID, err)
	}
	return nil
}

// countDistinctAreas reports BundleReport.Areas: the number of distinct
// area_code values across every distribution row the bundle now holds.
func countDistinctAreas(ctx context.Context, bundle *DB) (int, error) {
	var n int
	if err := bundle.sql.QueryRowContext(ctx, `SELECT COUNT(DISTINCT area_code) FROM distribution`).Scan(&n); err != nil {
		return 0, fmt.Errorf("sqlite: bundle: counting distribution areas: %w", err)
	}
	return n, nil
}

// insertBundleMeta writes the bundle's single provenance row. createdAt
// comes from opts.Now (defaulting to time.Now) rather than a direct
// time.Now() call here, so tests can inject a fixed clock and assert an
// exact, deterministic timestamp.
func insertBundleMeta(ctx context.Context, bundle *DB, opts BundleOpts, manifestSHA string) error {
	now := opts.Now
	if now == nil {
		now = time.Now
	}
	createdAt := now().UTC().Format(time.RFC3339)

	if _, err := bundle.sql.ExecContext(ctx, `
		INSERT INTO bundle_meta (snapshot_version, area, created_at, source_manifest_sha)
		VALUES (?, ?, ?, ?)`,
		opts.SnapshotVersion, opts.Area, createdAt, manifestSHA,
	); err != nil {
		return fmt.Errorf("sqlite: bundle: inserting bundle_meta: %w", err)
	}
	return nil
}

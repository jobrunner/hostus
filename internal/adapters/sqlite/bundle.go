package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jobrunner/hostus/internal/domain"
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
	// AllowRestricted opts out of the redistribution gate (see
	// findRestrictedSources): without it, ExportBundle refuses to export
	// when any contributing backbone/trait-vocabulary source is not
	// domain.RedistributionAllowed. With it, the export succeeds AND the
	// offending sources are recorded in bundle_meta.restricted_sources, so
	// a bundle can never silently carry unclearable data. Surfaced as
	// "hostus bundle --force-include-restricted".
	AllowRestricted bool
}

// BundleReport summarizes one ExportBundle call.
type BundleReport struct {
	Concepts int
	Names    int
	Areas    int
	Path     string
}

// restrictedSource is one non-domain.RedistributionAllowed backbone or
// trait-vocabulary source that contributed data (a taxon_concept, or a
// trait_value row) to a bundle's export scope.
type restrictedSource struct {
	ID             string
	Redistribution string
}

// formatRestrictedSources renders rs for ExportBundle's refusal error,
// naming both the offending source and its redistribution value (e.g.
// "germansl (redistribution=unknown)") — never just an id, since the whole
// point of the message is to tell the operator WHY the export was refused.
func formatRestrictedSources(rs []restrictedSource) string {
	parts := make([]string, len(rs))
	for i, r := range rs {
		parts[i] = fmt.Sprintf("%s (redistribution=%s)", r.ID, r.Redistribution)
	}
	return strings.Join(parts, ", ")
}

// restrictedSourceIDs renders rs into bundle_meta.restricted_sources' stable,
// deterministic representation: a comma-joined, sorted list of ids only (no
// redistribution values — the value is already recoverable from the
// original source database's backbone_version/trait_vocabulary rows).
func restrictedSourceIDs(rs []restrictedSource) string {
	if len(rs) == 0 {
		return ""
	}
	ids := make([]string, len(rs))
	for i, r := range rs {
		ids[i] = r.ID
	}
	sort.Strings(ids)
	return strings.Join(ids, ",")
}

// findRestrictedSources reports every backbone or trait-vocabulary source
// that contributes data to conceptIDs' scope (a taxon_concept belonging to
// it, or a trait_value row on one of conceptIDs) and whose redistribution
// is not domain.RedistributionAllowed, sorted by id for a deterministic
// result. An empty conceptIDs (nothing in scope) trivially contributes no
// sources.
func findRestrictedSources(ctx context.Context, src *DB, conceptIDs []string) ([]restrictedSource, error) {
	if len(conceptIDs) == 0 {
		return nil, nil
	}
	placeholders := placeholdersFor(len(conceptIDs))
	args := idArgs(conceptIDs)

	var out []restrictedSource

	backbones, err := queryNonAllowedSources(ctx, src, `
		SELECT DISTINCT bv.id, bv.redistribution
		FROM backbone_version bv
		JOIN taxon_concept tc ON tc.backbone_id = bv.id
		WHERE tc.id IN (`+placeholders+`)`, args)
	if err != nil {
		return nil, fmt.Errorf("sqlite: bundle: checking backbone redistribution: %w", err)
	}
	out = append(out, backbones...)

	vocabs, err := queryNonAllowedSources(ctx, src, `
		SELECT DISTINCT tv.vocab, tv.redistribution
		FROM trait_vocabulary tv
		JOIN trait_value v ON v.vocab = tv.vocab AND v.vocab_version = tv.version
		WHERE v.concept_id IN (`+placeholders+`)`, args)
	if err != nil {
		return nil, fmt.Errorf("sqlite: bundle: checking trait vocabulary redistribution: %w", err)
	}
	out = append(out, vocabs...)

	out = dedupeRestrictedSourcesByID(out)

	// out[i].ID < out[j].ID vs. <=: a provable-equivalence-class boundary,
	// same as sortedSample's len(all) > cap in traits_ingest.go. This is
	// true ONLY because of the dedupeRestrictedSourcesByID call directly
	// above: trait_vocabulary's primary key is (vocab, version), not vocab
	// alone, and IngestTraits never deletes an older version's row when a
	// vocabulary is re-ingested at a new version — so the raw `vocabs`
	// query above CAN legitimately return the same vocab id twice (once
	// per version) with two different redistribution values, and the
	// backbone query's ids could in principle collide with a vocab id too.
	// dedupeRestrictedSourcesByID collapses all of that to one entry per
	// id first, so by the time this sort runs, every element of out is
	// guaranteed distinct by construction — no two ever compare equal, so
	// <= would only produce a different result from < for equal keys,
	// which cannot occur here.
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// dedupeRestrictedSourcesByID collapses in to exactly one restrictedSource
// per distinct ID, keeping the MOST SEVERE Redistribution value observed
// for that id (restricted outranks unknown). This is necessary — not just
// tidying — because trait_vocabulary's primary key is (vocab, version):
// re-ingesting a vocabulary at a new version leaves the prior version's row
// (and its trait_value rows) in place, so a bundle's scope can genuinely
// include trait_value rows from TWO versions of the same vocab id with two
// different redistribution values (e.g. eive 1.0 unknown, eive 2.0
// restricted). Without this dedup, findRestrictedSources' caller-visible
// list would name that id twice — undermining the gate's core promise that
// an offending source is named, and recorded into
// bundle_meta.restricted_sources, EXACTLY once.
func dedupeRestrictedSourcesByID(in []restrictedSource) []restrictedSource {
	bySeverity := func(r string) int {
		switch domain.Redistribution(r) {
		case domain.RedistributionRestricted:
			return 2
		case domain.RedistributionUnknown:
			return 1
		case domain.RedistributionAllowed:
			// Never actually reaches here: in is only ever populated by
			// queryNonAllowedSources, which already filters out
			// RedistributionAllowed rows. Kept as an explicit case (not
			// folded into default) so the exhaustive linter enforces this
			// switch covers every domain.Redistribution value, the same
			// convention domain.ScaleFor's per-vocabulary switches use.
			return 0
		default: // any value outside the three known constants (should not occur past ParseRedistribution)
			return 0
		}
	}

	byID := make(map[string]restrictedSource, len(in))
	var order []string
	for _, r := range in {
		existing, ok := byID[r.ID]
		if !ok {
			order = append(order, r.ID)
			byID[r.ID] = r
			continue
		}
		// > vs >=: a provable-equivalence-class boundary at the tie case
		// (severities equal). bySeverity is a total, injective map over
		// exactly the three known domain.Redistribution values, so
		// bySeverity(r.Redistribution) == bySeverity(existing.Redistribution)
		// implies the two Redistribution strings are themselves equal —
		// there is no pair of DIFFERENT known values that share a
		// severity. So at the tie, replacing existing with r (>=) or
		// keeping existing (>) produces the same observable
		// Redistribution string either way; only the strict > vs <=
		// direction (CONDITIONALS_NEGATION) changes behavior, which the
		// two-version dedup test below pins by asserting on the SURVIVING
		// value, not just the surviving id.
		if bySeverity(r.Redistribution) > bySeverity(existing.Redistribution) {
			byID[r.ID] = r
		}
	}

	out := make([]restrictedSource, 0, len(order))
	for _, id := range order {
		out = append(out, byID[id])
	}
	return out
}

// queryNonAllowedSources runs query (with args) against src, expecting two
// columns (id, redistribution), and returns every row whose redistribution
// is not domain.RedistributionAllowed — the shared scan loop
// findRestrictedSources' two queries (backbone_version, trait_vocabulary)
// both use.
func queryNonAllowedSources(ctx context.Context, src *DB, query string, args []any) ([]restrictedSource, error) {
	rows, err := src.sql.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("sqlite: bundle: querying %q: %w", query, err)
	}
	defer func() { _ = rows.Close() }()

	var out []restrictedSource
	for rows.Next() {
		var id, redistribution string
		if err := rows.Scan(&id, &redistribution); err != nil {
			return nil, fmt.Errorf("sqlite: bundle: scanning %q: %w", query, err)
		}
		if redistribution != string(domain.RedistributionAllowed) {
			out = append(out, restrictedSource{ID: id, Redistribution: redistribution})
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: bundle: iterating %q: %w", query, err)
	}
	return out, nil
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
//
// Before copying anything, ExportBundle checks findRestrictedSources: by
// default (opts.AllowRestricted == false), a bundle whose scope includes
// data from any source that is not domain.RedistributionAllowed is refused
// outright, naming the offending source(s) and their redistribution value
// — local ingest of those sources is never affected, only export. With
// opts.AllowRestricted, the export proceeds AND the offending source ids
// are recorded into bundle_meta.restricted_sources (restrictedSourceIDs),
// so a bundle can never silently carry unclearable data.
func ExportBundle(ctx context.Context, src *DB, out string, opts BundleOpts) (BundleReport, error) {
	conceptIDs, err := scopeConceptIDs(ctx, src, opts.Area)
	if err != nil {
		return BundleReport{}, err
	}

	restricted, err := findRestrictedSources(ctx, src, conceptIDs)
	if err != nil {
		return BundleReport{}, err
	}
	if len(restricted) > 0 && !opts.AllowRestricted {
		return BundleReport{}, fmt.Errorf("sqlite: bundle: refusing to export: source(s) not cleared for redistribution: %s (use --force-include-restricted to override)", formatRestrictedSources(restricted))
	}

	bundle, err := Open(out)
	if err != nil {
		return BundleReport{}, fmt.Errorf("sqlite: bundle: creating %q: %w", out, err)
	}
	defer func() { _ = bundle.Close() }()

	report, err := populateBundle(ctx, src, bundle, conceptIDs, opts, restrictedSourceIDs(restricted))
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
// bundle, rebuilds the FTS index, and writes bundle_meta (including
// restrictedSources — see ExportBundle's doc comment — which was already
// computed and, if non-empty, cleared for override BEFORE any copying
// started), in FK-safe order: backbone_version, then name (both
// referenced by taxon_concept), then taxon_concept itself, then the
// concept_id-keyed tables.
func populateBundle(ctx context.Context, src, bundle *DB, conceptIDs []string, opts BundleOpts, restrictedSources string) (BundleReport, error) {
	var report BundleReport
	if len(conceptIDs) == 0 {
		if err := insertBundleMeta(ctx, bundle, opts, "", restrictedSources); err != nil {
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

	// idCol/refCol pin the 0-based index of each row's own id / its
	// self-referencing FK in the SELECT lists below — see
	// copySelfReferencingRows' doc comment for why both columns are copied
	// in two sub-passes rather than one.
	const (
		nameIDCol       = 0
		nameBasionymCol = 8
	)
	names, err := copySelfReferencingRows(ctx, src, bundle,
		`SELECT DISTINCT n.id, n.canonical, n.canonical_fold, n.authorship, n.rank, n.ipni_id, n.published_in, n.nom_status, n.basionym_id
		 FROM name n
		 JOIN concept_name cn ON cn.name_id = n.id
		 WHERE cn.concept_id IN (`+placeholders+`)`, args,
		`INSERT INTO name (id, canonical, canonical_fold, authorship, rank, ipni_id, published_in, nom_status, basionym_id) VALUES (?,?,?,?,?,?,?,?,?)`,
		nameIDCol, nameBasionymCol, `UPDATE name SET basionym_id = ? WHERE id = ?`)
	if err != nil {
		return report, err
	}
	report.Names = names

	const (
		conceptIDCol       = 0
		conceptParentIDCol = 4
	)
	concepts, err := copySelfReferencingRows(ctx, src, bundle,
		`SELECT id, backbone_id, accepted_name, rank, parent_id, sec_reference, status FROM taxon_concept WHERE id IN (`+placeholders+`)`, args,
		`INSERT INTO taxon_concept (id, backbone_id, accepted_name, rank, parent_id, sec_reference, status) VALUES (?,?,?,?,?,?,?)`,
		conceptIDCol, conceptParentIDCol, `UPDATE taxon_concept SET parent_id = ? WHERE id = ?`)
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

	if err := insertBundleMeta(ctx, bundle, opts, manifestSHA, restrictedSources); err != nil {
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
	if err := copyRows(ctx, src, bundle,
		`SELECT concept_id, name_id, role, homotypic FROM concept_name WHERE concept_id IN (`+placeholders+`)`, args,
		`INSERT INTO concept_name (concept_id, name_id, role, homotypic) VALUES (?,?,?,?)`); err != nil {
		return err
	}

	if err := copyRows(ctx, src, bundle,
		`SELECT concept_id, authority, ext_id FROM xref WHERE concept_id IN (`+placeholders+`)`, args,
		`INSERT INTO xref (concept_id, authority, ext_id) VALUES (?,?,?)`); err != nil {
		return err
	}

	if err := copyRows(ctx, src, bundle,
		`SELECT concept_id, area_scheme, area_code FROM distribution WHERE concept_id IN (`+placeholders+`)`, args,
		`INSERT INTO distribution (concept_id, area_scheme, area_code) VALUES (?,?,?)`); err != nil {
		return err
	}

	if err := copyRows(ctx, src, bundle,
		`SELECT concept_id, lang, name, preferred FROM vernacular WHERE concept_id IN (`+placeholders+`)`, args,
		`INSERT INTO vernacular (concept_id, lang, name, preferred) VALUES (?,?,?,?)`); err != nil {
		return err
	}

	if err := copyRows(ctx, src, bundle,
		`SELECT concept_id, vocab, vocab_version, dim, value, niche_width, n_systems FROM trait_value WHERE concept_id IN (`+placeholders+`)`, args,
		`INSERT INTO trait_value (concept_id, vocab, vocab_version, dim, value, niche_width, n_systems) VALUES (?,?,?,?,?,?,?)`); err != nil {
		return err
	}

	if err := copyRows(ctx, src, bundle,
		`SELECT vocab, version, taxonomy, license, source_url, ingested_at, redistribution FROM trait_vocabulary`, nil,
		`INSERT INTO trait_vocabulary (vocab, version, taxonomy, license, source_url, ingested_at, redistribution) VALUES (?,?,?,?,?,?,?)`); err != nil {
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
		SELECT DISTINCT bv.id, bv.version, bv.license, bv.source_url, bv.ingested_at, bv.manifest_sha, bv.redistribution
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
		var id, version, ingestedAt, sha, redistribution string
		var license, sourceURL sql.NullString
		if err := rows.Scan(&id, &version, &license, &sourceURL, &ingestedAt, &sha, &redistribution); err != nil {
			return nil, "", fmt.Errorf("sqlite: bundle: scanning backbone_version row: %w", err)
		}
		if _, err := bundle.sql.ExecContext(ctx, `
			INSERT INTO backbone_version (id, version, license, source_url, ingested_at, manifest_sha, redistribution)
			VALUES (?, ?, ?, ?, ?, ?, ?)`,
			id, version, license, sourceURL, ingestedAt, sha, redistribution,
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

// copySelfReferencingRows is copyRows for a table whose own rows carry a
// NULLABLE, SELF-referencing FK (name.basionym_id, taxon_concept.parent_id)
// — i.e. a reference that may point at ANOTHER row this very call is about
// to copy. Two problems that plain copyRows cannot handle follow from that:
//
//  1. Forward references: query has no defined row order, so a row whose
//     reference points at a row copied LATER in this same call would fail
//     bundle's (immediate) FK check — the target genuinely doesn't exist
//     YET at that point, even though it's about to. Fixed the same way
//     internal/application/ingest.go's own two-sub-pass handles the
//     identical problem for the live ingest: every row is inserted with
//     refCol forced NULL first (so no insert ever forward-references
//     anything), then a second sub-pass UPDATEs refCol in, once every row
//     is guaranteed to already exist.
//  2. Out-of-scope references: an area-scoped export can include a row
//     whose reference points OUTSIDE the copied set entirely (e.g. a
//     concept whose parent has no distribution row in the requested area,
//     so it was never selected for the bundle at all — the SP2
//     forward-note this closes). The referenced row is genuinely absent
//     from the bundle, so the only FK-safe options are dropping the
//     referencing row (losing data the caller DID ask to export) or
//     nulling just the out-of-scope reference; sub-pass 2 does the latter,
//     leaving refCol NULL (as sub-pass 1 already left it) for any
//     reference whose target isn't among the ids THIS call actually copied.
//
// idCol/refCol are 0-based indices into query's SELECT list (== insertSQL's
// column order, per copyRows' own contract). updateSQL is the bundle-side
// statement sub-pass 2 uses to link the reference back in — a literal
// "UPDATE <table> SET <refColumn> = ? WHERE <idColumn> = ?" the caller
// writes out at each call site (never built by string concatenation here),
// taking (ref, id) as its two positional parameters in that order.
func copySelfReferencingRows(ctx context.Context, src, bundle *DB, query string, args []any, insertSQL string, idCol, refCol int, updateSQL string) (int, error) {
	buffered, err := fetchRowsForCopy(ctx, src, query, args)
	if err != nil {
		return 0, err
	}

	// The in-scope set for sub-pass 2 is exactly the ids THIS call is
	// copying — computed from the buffered rows themselves, not passed in,
	// since for name.basionym_id that set (every name linked to a concept
	// in the export scope) isn't known any earlier than this.
	inScope := make(map[string]bool, len(buffered))
	refs := make([]any, len(buffered))
	for i, vals := range buffered {
		if id, ok := vals[idCol].(string); ok {
			inScope[id] = true
		}
		refs[i] = vals[refCol]
	}

	n, err := insertRowsWithColumnNulled(ctx, bundle, insertSQL, buffered, refCol)
	if err != nil {
		return 0, err
	}
	if err := linkInScopeReferences(ctx, bundle, updateSQL, buffered, refs, idCol, inScope); err != nil {
		return 0, err
	}
	return n, nil
}

// fetchRowsForCopy runs query (with args) against src and buffers every
// resulting row as a []any (one per SELECT column), for callers that need
// the FULL result set in memory before deciding how to write it — unlike
// copyRows, which can stream straight from cursor to insert.
func fetchRowsForCopy(ctx context.Context, src *DB, query string, args []any) ([][]any, error) {
	rows, err := src.sql.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("sqlite: bundle: querying %q: %w", query, err)
	}
	defer func() { _ = rows.Close() }()

	cols, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("sqlite: bundle: reading columns for %q: %w", query, err)
	}

	var buffered [][]any
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, fmt.Errorf("sqlite: bundle: scanning row for %q: %w", query, err)
		}
		buffered = append(buffered, vals)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: bundle: iterating rows for %q: %w", query, err)
	}
	return buffered, nil
}

// insertRowsWithColumnNulled inserts every buffered row via insertSQL,
// forcing column col to NULL first — copySelfReferencingRows' sub-pass 1
// (see its doc comment for why the self-reference must never be written on
// the first insert).
func insertRowsWithColumnNulled(ctx context.Context, bundle *DB, insertSQL string, buffered [][]any, col int) (int, error) {
	n := 0
	for _, vals := range buffered {
		vals[col] = nil
		if _, err := bundle.sql.ExecContext(ctx, insertSQL, vals...); err != nil {
			return 0, fmt.Errorf("sqlite: bundle: inserting row via %q: %w", insertSQL, err)
		}
		n++
	}
	return n, nil
}

// linkInScopeReferences is copySelfReferencingRows' sub-pass 2: for every
// buffered row whose original reference (refs[i], captured before sub-pass
// 1 nulled it) names an id inScope, it fills the reference back in via
// updateSQL(ref, id). A reference that's empty or points outside inScope is
// left as sub-pass 1 already wrote it: NULL.
func linkInScopeReferences(ctx context.Context, bundle *DB, updateSQL string, buffered [][]any, refs []any, idCol int, inScope map[string]bool) error {
	for i, vals := range buffered {
		ref, ok := refs[i].(string)
		if !ok || ref == "" || !inScope[ref] {
			continue
		}
		if _, err := bundle.sql.ExecContext(ctx, updateSQL, ref, vals[idCol]); err != nil {
			return fmt.Errorf("sqlite: bundle: linking self-reference for %v: %w", vals[idCol], err)
		}
	}
	return nil
}

// copyRows executes query (with args) against src and, for each resulting
// row, executes insertSQL against bundle with the exact same column values
// — query's SELECT list and insertSQL's column list must therefore be
// written in the same order. This generic row-shuttle (rather than a
// hand-scanned struct per table) is possible because ExportBundle only
// ever copies a row byte-for-byte, never transforms it. Unlike
// copySelfReferencingRows, no caller needs a row count back, so this
// returns only error.
func copyRows(ctx context.Context, src, bundle *DB, query string, args []any, insertSQL string) error {
	rows, err := src.sql.QueryContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("sqlite: bundle: querying %q: %w", query, err)
	}
	defer func() { _ = rows.Close() }()

	cols, err := rows.Columns()
	if err != nil {
		return fmt.Errorf("sqlite: bundle: reading columns for %q: %w", query, err)
	}

	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return fmt.Errorf("sqlite: bundle: scanning row for %q: %w", query, err)
		}
		if _, err := bundle.sql.ExecContext(ctx, insertSQL, vals...); err != nil {
			return fmt.Errorf("sqlite: bundle: inserting row via %q: %w", insertSQL, err)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("sqlite: bundle: iterating rows for %q: %w", query, err)
	}
	return nil
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
// exact, deterministic timestamp. restrictedSources is ExportBundle's
// pre-computed restrictedSourceIDs result — non-empty only when
// opts.AllowRestricted overrode a gate refusal — recorded verbatim so a
// bundle can never silently carry unclearable data (see BundleOpts.AllowRestricted).
func insertBundleMeta(ctx context.Context, bundle *DB, opts BundleOpts, manifestSHA, restrictedSources string) error {
	now := opts.Now
	if now == nil {
		now = time.Now
	}
	createdAt := now().UTC().Format(time.RFC3339)

	if _, err := bundle.sql.ExecContext(ctx, `
		INSERT INTO bundle_meta (snapshot_version, area, created_at, source_manifest_sha, restricted_sources)
		VALUES (?, ?, ?, ?, ?)`,
		opts.SnapshotVersion, opts.Area, createdAt, manifestSHA, restrictedSources,
	); err != nil {
		return fmt.Errorf("sqlite: bundle: inserting bundle_meta: %w", err)
	}
	return nil
}

package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jobrunner/hostus/internal/domain"
)

// BundleOpts configures ExportBundle.
type BundleOpts struct {
	// Area restricts the bundle to concepts whose distribution intersects
	// one of a comma-separated list of WGSRPD level-3 area codes (or one
	// of areaCodes' convenience aliases, e.g. "DE" — the same resolution
	// Suggest's Area option uses), e.g. "DE,AT,CH" for a Mitteleuropa
	// bundle. A single value (no comma) keeps working exactly as before.
	// Empty means no filter: every concept in src is copied. See
	// resolveAreaCodes for the exact comma/alias resolution.
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
	// when any contributing backbone/trait-vocabulary/xref source is not
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

// restrictedSource is one non-domain.RedistributionAllowed backbone,
// trait-vocabulary or xref source that contributed data (a taxon_concept, a
// trait_value row, or a source-attributed xref row) to a bundle's export
// scope.
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
// original source database's backbone_version/trait_vocabulary/xref_source
// rows).
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

// marshalIDs JSON-encodes ids for binding as ONE parameter via
// `IN (SELECT value FROM json_each(?))`, rather than building a
// "?,?,?..." placeholder list whose length scales with len(ids): the
// query text stays a fixed literal regardless of how many ids there are,
// which is what makes ExportBundle scope-independent (see
// findRestrictedSources', copyBackboneVersions', copySelfReferencingRows'
// and copyConceptScopedTables' doc comments) — an un-scoped export binds
// all 440k+ concept ids through this exact path instead of hitting
// SQLite's SQLITE_MAX_VARIABLE_NUMBER (see docs/research/reality-check.md
// M5.1). MatchFuzzyCandidates (read.go) established this same pattern for
// its own id list first.
func marshalIDs(ids []string) (string, error) {
	b, err := json.Marshal(ids)
	if err != nil {
		return "", fmt.Errorf("sqlite: bundle: encoding id list: %w", err)
	}
	return string(b), nil
}

// findRestrictedSources reports every backbone, trait-vocabulary or
// xref source that contributes data to conceptIDs' scope (a taxon_concept
// belonging to it, a trait_value row on one of conceptIDs, or an xref row on
// one of conceptIDs attributed to it via xref.source) and whose
// redistribution is not domain.RedistributionAllowed, sorted by id for a
// deterministic result. An empty conceptIDs (nothing in scope) trivially
// contributes no sources.
//
// The xref query deliberately joins on xref.source, so it covers exactly the
// rows an xref-source ingest wrote; xrefs the backbone ingest derived from a
// taxon row carry source NULL and are already gated by the backbone query
// above (see schema.sql's note on xref.source).
func findRestrictedSources(ctx context.Context, src *DB, conceptIDs []string) ([]restrictedSource, error) {
	if len(conceptIDs) == 0 {
		return nil, nil
	}
	idsJSON, err := marshalIDs(conceptIDs)
	if err != nil {
		return nil, err
	}

	var out []restrictedSource

	backbones, err := queryNonAllowedSources(ctx, src, `
		SELECT DISTINCT bv.id, bv.redistribution
		FROM backbone_version bv
		JOIN taxon_concept tc ON tc.backbone_id = bv.id
		WHERE tc.id IN (SELECT value FROM json_each(?))`, []any{idsJSON})
	if err != nil {
		return nil, fmt.Errorf("sqlite: bundle: checking backbone redistribution: %w", err)
	}
	out = append(out, backbones...)

	vocabs, err := queryNonAllowedSources(ctx, src, `
		SELECT DISTINCT tv.vocab, tv.redistribution
		FROM trait_vocabulary tv
		JOIN trait_value v ON v.vocab = tv.vocab AND v.vocab_version = tv.version
		WHERE v.concept_id IN (SELECT value FROM json_each(?))`, []any{idsJSON})
	if err != nil {
		return nil, fmt.Errorf("sqlite: bundle: checking trait vocabulary redistribution: %w", err)
	}
	out = append(out, vocabs...)

	xrefSources, err := queryNonAllowedSources(ctx, src, `
		SELECT DISTINCT xs.id, xs.redistribution
		FROM xref_source xs
		JOIN xref x ON x.source = xs.id
		WHERE x.concept_id IN (SELECT value FROM json_each(?))`, []any{idsJSON})
	if err != nil {
		return nil, fmt.Errorf("sqlite: bundle: checking xref source redistribution: %w", err)
	}
	out = append(out, xrefSources...)

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
// findRestrictedSources' three queries (backbone_version, trait_vocabulary,
// xref_source) all use.
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
// data (concepts, trait values or cross-references) from any source that is
// not domain.RedistributionAllowed is refused
// outright, naming the offending source(s) and their redistribution value
// — local ingest of those sources is never affected, only export. With
// opts.AllowRestricted, the export proceeds AND the offending source ids
// are recorded into bundle_meta.restricted_sources (restrictedSourceIDs),
// so a bundle can never silently carry unclearable data.
func ExportBundle(ctx context.Context, src *DB, out string, opts BundleOpts) (BundleReport, error) {
	conceptIDs, areaScope, err := scopeConceptIDs(ctx, src, opts.Area)
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

	report, err := populateBundle(ctx, src, bundle, conceptIDs, areaScope, opts, restrictedSourceIDs(restricted))
	if err != nil {
		return BundleReport{}, err
	}
	report.Path = out
	return report, nil
}

// resolveAreaCodes turns a BundleOpts.Area value into the deduplicated set
// of WGSRPD level-3 codes ExportBundle scopes to: area is split on commas
// (so "DE,AT,CH" resolves each part independently, e.g. via areaCodes'
// alias table — "DE" alone expands to wgsrpdGermanyL3 — and unions the
// results), blank parts are skipped, and an all-blank/empty area returns
// nil, the existing "no filter" convention. A single value with no comma
// (the pre-multi-area form) behaves exactly as before: it is just a
// one-element split. Sorted so the result (and its json_each encoding) is
// deterministic regardless of the order --area listed its parts in.
func resolveAreaCodes(area string) []string {
	seen := make(map[string]bool)
	var out []string
	for _, part := range strings.Split(area, ",") {
		for _, code := range areaCodes(part) {
			if !seen[code] {
				seen[code] = true
				out = append(out, code)
			}
		}
	}
	sort.Strings(out)
	return out
}

// scopeConceptIDs resolves BundleOpts.Area into the set of taxon_concept
// ids ExportBundle copies (every concept id when area is blank, or every
// concept id with at least one distribution row in one of area's resolved
// WGSRPD level-3 codes otherwise) AND the resolved code set itself, which
// populateBundle/copyDistribution also needs to scope the distribution
// table (see copyDistribution's doc comment) — computing it here, once,
// keeps scopeByAreaQuery and copyDistribution's filter provably in sync:
// both use exactly the same resolveAreaCodes(area) result.
func scopeConceptIDs(ctx context.Context, src *DB, area string) ([]string, []string, error) {
	codes := resolveAreaCodes(area)

	var (
		rows *sql.Rows
		err  error
	)
	if len(codes) == 0 {
		rows, err = src.sql.QueryContext(ctx, `SELECT id FROM taxon_concept ORDER BY id`)
	} else {
		codesJSON, jerr := marshalIDs(codes)
		if jerr != nil {
			return nil, nil, jerr
		}
		rows, err = src.sql.QueryContext(ctx, scopeByAreaQuery, codesJSON)
	}
	if err != nil {
		return nil, nil, fmt.Errorf("sqlite: bundle: resolving concept scope for area %q: %w", area, err)
	}
	defer func() { _ = rows.Close() }()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, nil, fmt.Errorf("sqlite: bundle: scanning concept scope row: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("sqlite: bundle: iterating concept scope rows: %w", err)
	}
	return ids, codes, nil
}

// scopeByAreaQuery finds every concept with a distribution row in one of
// the (any number of) area codes bound via json_each — see
// resolveAreaCodes for how BundleOpts.Area's comma-separated value becomes
// that code list, and marshalIDs' doc comment for why the query text is a
// fixed literal regardless of how many codes there are.
const scopeByAreaQuery = `
	SELECT DISTINCT tc.id
	FROM taxon_concept tc
	JOIN distribution d ON d.concept_id = tc.id
	WHERE d.area_scheme = 'wgsrpd_l3' AND d.area_code IN (SELECT value FROM json_each(?))
	ORDER BY tc.id`

// placeholdersFor returns n comma-joined "?" placeholders for a SQL IN
// clause. Still used by traits.go (a small, caller-bounded vocab list);
// bundle.go's own concept/area-code id lists moved to marshalIDs'
// json_each pattern precisely because they are NOT caller-bounded (an
// un-scoped export's conceptIDs can be 440k+ — see marshalIDs' doc
// comment).
func placeholdersFor(n int) string {
	ph := make([]string, n)
	for i := range ph {
		ph[i] = "?"
	}
	return strings.Join(ph, ",")
}

// populateBundle copies every row scoped by conceptIDs from src into
// bundle, rebuilds the FTS index, and writes bundle_meta (including
// restrictedSources — see ExportBundle's doc comment — which was already
// computed and, if non-empty, cleared for override BEFORE any copying
// started), in FK-safe order: backbone_version, then name (both
// referenced by taxon_concept), then taxon_concept itself, then the
// concept_id-keyed tables. areaScope is scopeConceptIDs' resolved code set
// (nil for a whole-DB export) — copyConceptScopedTables uses it to scope
// the distribution copy to just the requested areas (see
// copyDistribution's doc comment).
func populateBundle(ctx context.Context, src, bundle *DB, conceptIDs, areaScope []string, opts BundleOpts, restrictedSources string) (BundleReport, error) {
	var report BundleReport
	if len(conceptIDs) == 0 {
		if err := insertBundleMeta(ctx, bundle, opts, "", restrictedSources); err != nil {
			return report, err
		}
		return report, nil
	}

	idsJSON, err := marshalIDs(conceptIDs)
	if err != nil {
		return report, err
	}

	backboneIDs, manifestSHA, err := copyBackboneVersions(ctx, src, bundle, idsJSON)
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
		`SELECT DISTINCT n.id, n.canonical, n.canonical_fold, n.authorship, n.rank, n.ipni_id, n.published_in, n.nom_status, n.basionym_id, n.rank_verbatim
		 FROM name n
		 JOIN concept_name cn ON cn.name_id = n.id
		 WHERE cn.concept_id IN (SELECT value FROM json_each(?))`, []any{idsJSON},
		`INSERT INTO name (id, canonical, canonical_fold, authorship, rank, ipni_id, published_in, nom_status, basionym_id, rank_verbatim) VALUES (?,?,?,?,?,?,?,?,?,?)`,
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
		`SELECT id, backbone_id, accepted_name, rank, parent_id, sec_reference, status, rank_verbatim FROM taxon_concept WHERE id IN (SELECT value FROM json_each(?))`, []any{idsJSON},
		`INSERT INTO taxon_concept (id, backbone_id, accepted_name, rank, parent_id, sec_reference, status, rank_verbatim) VALUES (?,?,?,?,?,?,?,?)`,
		conceptIDCol, conceptParentIDCol, `UPDATE taxon_concept SET parent_id = ? WHERE id = ?`)
	if err != nil {
		return report, err
	}
	report.Concepts = concepts

	if err := copyConceptScopedTables(ctx, src, bundle, idsJSON, areaScope); err != nil {
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
// idsJSON, plus xref_source and trait_vocabulary in full (metadata, not concept-scoped —
// the offline field app needs the Taxonomy/license/source provenance for
// every trait_value row copied above). Split out of populateBundle purely
// to keep that function's cyclomatic complexity down; it carries no logic
// of its own beyond sequencing copyRows calls (copyDistribution is the one
// exception — see its own doc comment for the area-scoping it does).
func copyConceptScopedTables(ctx context.Context, src, bundle *DB, idsJSON string, areaScope []string) error {
	if err := copyRows(ctx, src, bundle,
		`SELECT concept_id, name_id, role, homotypic FROM concept_name WHERE concept_id IN (SELECT value FROM json_each(?))`, []any{idsJSON},
		`INSERT INTO concept_name (concept_id, name_id, role, homotypic) VALUES (?,?,?,?)`); err != nil {
		return err
	}

	// xref_source before xref: xref.source is an FK onto it, and the bundle
	// connection enforces foreign keys. Copied in full (metadata, not
	// concept-scoped) for the same reason trait_vocabulary is — the bundle
	// must carry the license/version/manifest_sha provenance of every xref
	// row it holds.
	if err := copyRows(ctx, src, bundle,
		`SELECT id, version, license, source_url, ingested_at, manifest_sha, redistribution FROM xref_source`, nil,
		`INSERT INTO xref_source (id, version, license, source_url, ingested_at, manifest_sha, redistribution) VALUES (?,?,?,?,?,?,?)`); err != nil {
		return err
	}

	if err := copyRows(ctx, src, bundle,
		`SELECT concept_id, authority, ext_id, source FROM xref WHERE concept_id IN (SELECT value FROM json_each(?))`, []any{idsJSON},
		`INSERT INTO xref (concept_id, authority, ext_id, source) VALUES (?,?,?,?)`); err != nil {
		return err
	}

	if err := copyDistribution(ctx, src, bundle, idsJSON, areaScope); err != nil {
		return err
	}

	if err := copyRows(ctx, src, bundle,
		`SELECT concept_id, lang, name, preferred FROM vernacular WHERE concept_id IN (SELECT value FROM json_each(?))`, []any{idsJSON},
		`INSERT INTO vernacular (concept_id, lang, name, preferred) VALUES (?,?,?,?)`); err != nil {
		return err
	}

	if err := copyRows(ctx, src, bundle,
		`SELECT concept_id, vocab, vocab_version, dim, value, niche_width, n_systems, resolution FROM trait_value WHERE concept_id IN (SELECT value FROM json_each(?))`, []any{idsJSON},
		`INSERT INTO trait_value (concept_id, vocab, vocab_version, dim, value, niche_width, n_systems, resolution) VALUES (?,?,?,?,?,?,?,?)`); err != nil {
		return err
	}

	if err := copyRows(ctx, src, bundle,
		`SELECT vocab, version, taxonomy, license, source_url, ingested_at, redistribution FROM trait_vocabulary`, nil,
		`INSERT INTO trait_vocabulary (vocab, version, taxonomy, license, source_url, ingested_at, redistribution) VALUES (?,?,?,?,?,?,?)`); err != nil {
		return err
	}

	// sec_reference SCOPED to the reference spaces the copied concepts
	// actually name — deliberately NOT copied in full the way xref_source and
	// trait_vocabulary above are.
	//
	// The difference is what the rows contain. xref_source and
	// trait_vocabulary hold provenance ABOUT a source (id, version, license,
	// source url); sec_reference.title holds harvested CONTENT — the citation
	// strings lifted out of the source itself. Copying all of them unscoped
	// would ship 18 citations from a redistribution=unknown source into an
	// area-scoped bundle whose gate never fired, because CDM concepts carry
	// no distribution rows and so fall out of scopeConceptIDs entirely: no
	// refusal, and nothing recorded in bundle_meta.restricted_sources either.
	// Scoping closes that, and it closes it structurally — a bundle can only
	// carry a citation for a concept it also carries.
	//
	// Note this applies to a WHOLE-DATABASE export too, not just an
	// area-scoped one: a sec_reference row that no concept names is dropped
	// there as well. That is a deliberate consequence of scoping structurally
	// rather than by export mode — an orphan citation is by definition
	// unreachable from any concept in the bundle, so nothing can miss it.
	// Unreachable in practice today, since the CDM ingest only ever writes a
	// sec_reference it is about to attach to a concept.
	//
	// The analogous unscoped copy of xref_source/trait_vocabulary is left
	// as-is: it is the same SHAPE of leak but only of source metadata, those
	// tables predate this task, and narrowing them is a change to SP3/SP4
	// behavior that belongs with its own measurement rather than smuggled in
	// here.
	if err := copyRows(ctx, src, bundle,
		`SELECT id, title FROM sec_reference
		 WHERE id IN (
			SELECT DISTINCT sec_reference FROM taxon_concept
			WHERE id IN (SELECT value FROM json_each(?))
			  AND sec_reference IS NOT NULL AND sec_reference <> ''
		 )`, []any{idsJSON},
		`INSERT INTO sec_reference (id, title) VALUES (?,?)`); err != nil {
		return err
	}

	// concept_relation is scoped by BOTH ends, not one: the column is a
	// foreign key on either side, and the bundle connection enforces foreign
	// keys, so copying an edge whose partner concept is out of scope would
	// fail the insert. An area-scoped bundle therefore carries only the
	// relations wholly inside its scope — which is also the honest answer,
	// since half an edge asserts nothing.
	return copyRows(ctx, src, bundle,
		`SELECT from_concept, to_concept, relation, source FROM concept_relation
		 WHERE from_concept IN (SELECT value FROM json_each(?))
		   AND to_concept IN (SELECT value FROM json_each(?))`, []any{idsJSON, idsJSON},
		`INSERT INTO concept_relation (from_concept, to_concept, relation, source) VALUES (?,?,?,?)`)
}

// copyDistribution copies distribution rows for the concepts named by
// idsJSON into bundle. This is the measured, deliberate size reduction
// from docs/research/reality-check.md M5.2: a GER-scoped export used to
// copy every one of a concept's distribution rows — its FULL global
// range across all 369 WGSRPD-L3 areas WCVP records, not just "occurs in
// GER" — accounting for ~39% of that bundle's 108.9 MB (distribution +
// its unique index, measured via `SELECT name, SUM(pgsize) FROM dbstat`).
// A field bundle scoped to a region has no documented use case that needs
// a concept's occurrence in areas OUTSIDE that region, so when areaScope
// is non-empty (an area-scoped export), only the wgsrpd_l3 rows matching
// one of areaScope are copied — a bundle's distribution table then answers
// exactly "does this concept occur in the requested area(s)", not "what is
// this concept's whole-world range". A whole-DB export (areaScope empty)
// is unaffected: every distribution row is still copied, because
// "everything" genuinely means everything there too.
func copyDistribution(ctx context.Context, src, bundle *DB, idsJSON string, areaScope []string) error {
	if len(areaScope) == 0 {
		return copyRows(ctx, src, bundle,
			`SELECT concept_id, area_scheme, area_code FROM distribution WHERE concept_id IN (SELECT value FROM json_each(?))`, []any{idsJSON},
			`INSERT INTO distribution (concept_id, area_scheme, area_code) VALUES (?,?,?)`)
	}

	areaScopeJSON, err := marshalIDs(areaScope)
	if err != nil {
		return err
	}
	return copyRows(ctx, src, bundle,
		`SELECT concept_id, area_scheme, area_code FROM distribution
		 WHERE concept_id IN (SELECT value FROM json_each(?))
		   AND area_scheme = 'wgsrpd_l3' AND area_code IN (SELECT value FROM json_each(?))`,
		[]any{idsJSON, areaScopeJSON},
		`INSERT INTO distribution (concept_id, area_scheme, area_code) VALUES (?,?,?)`)
}

// backboneVersionScopeQuery finds every backbone_version referenced by the
// concept ids bound via json_each (see marshalIDs' doc comment for why).
const backboneVersionScopeQuery = `
	SELECT DISTINCT bv.id, bv.version, bv.license, bv.source_url, bv.ingested_at, bv.manifest_sha, bv.redistribution
	FROM backbone_version bv
	JOIN taxon_concept tc ON tc.backbone_id = bv.id
	WHERE tc.id IN (SELECT value FROM json_each(?))
	ORDER BY bv.id`

// copyBackboneVersions copies every backbone_version row referenced by one
// of the concepts named by idsJSON into bundle, returning the distinct
// backbone ids copied (so populateBundle knows which backbones to
// rebuildFTS for) and the manifest_sha of the last row copied
// (bundle_meta.source_manifest_sha — in practice a single ingest run
// stamps every backbone_version row with the same manifest_sha, so which
// row "wins" does not matter).
func copyBackboneVersions(ctx context.Context, src, bundle *DB, idsJSON string) ([]string, string, error) {
	rows, err := src.sql.QueryContext(ctx, backboneVersionScopeQuery, idsJSON)
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

package application

import (
	"context"
	"fmt"
	"sort"

	"github.com/jobrunner/hostus/internal/domain"
	"github.com/jobrunner/hostus/internal/ports/output"
)

// XrefRow is the minimal shape of one cross-reference CSV row IngestXrefs
// needs: an external authority's id (Authority/ExtID) for the concept
// identified by (JoinAuthority, JoinID) — the existing xref row hostus
// already holds (e.g. JoinAuthority="powo", JoinID="396681-1"). A concrete
// xref reader's row type (xref.Row) is adapted into this DTO by the caller,
// the same RowSource-bridge pattern TraitRowSource/RowSource use so
// application never imports internal/adapters/xref directly (depguard).
type XrefRow struct {
	JoinAuthority string
	JoinID        string
	Authority     string
	ExtID         string
}

// XrefRowSource streams one xref source's rows for IngestXrefs.
type XrefRowSource interface {
	Rows() []XrefRow
}

// XrefIngestReport summarizes one xref source's ID-based join run.
// Matched+Unmatched+Conflicting always sums to Rows.
type XrefIngestReport struct {
	Source      string
	Rows        int
	Matched     int
	Unmatched   int
	Conflicting int
	// PerAuthority counts, per external authority, how many DISTINCT
	// concepts gained at least one xref for that authority — this is the
	// coverage number the SP4 measurement reports (e.g. PerAuthority["inat"]
	// is UC2's reachability ceiling), not a row count.
	PerAuthority map[string]int
	// UnmatchedSample is a bounded, deterministic sample of the join keys
	// (formatted "join_authority:join_id") whose join_id has no existing
	// xref row at all, so nothing was written for that row.
	UnmatchedSample []string
	// ConflictSample is a bounded, deterministic sample of the external
	// keys (formatted "authority:ext_id") that were claimed by two or more
	// DISTINCT concepts across the source's rows — see the conflict-rule
	// doc comment on IngestXrefs. Every row sharing such a key is skipped
	// entirely (never guessed which concept it belonged to).
	ConflictSample []string
	// MultiPerAuthority counts, per authority, how many concepts
	// legitimately received MORE THAN ONE distinct ext_id for that same
	// authority (e.g. two Wikidata items both carrying the same IPNI id).
	// This is NOT a conflict: every one of those ext_ids is written, since
	// xref's PK is (authority, ext_id) and distinct ext_ids never collide.
	// It is reported so the phenomenon is visible rather than silently
	// folded into PerAuthority's per-concept count.
	MultiPerAuthority map[string]int
	// MultiSample is a bounded, deterministic sample of the concept ids
	// exhibiting MultiPerAuthority (formatted "authority:concept_id").
	MultiSample []string
	// Redistribution is this source's manifest-pinned redistribution value
	// (see domain.Redistribution), surfaced for "hostus ingest" to print a
	// notice for anything that is not "allowed". Local ingest itself is
	// never gated by it — the act it gates is EXPORT: it is persisted onto
	// the source's xref_source row, and ExportBundle refuses to copy this
	// source's xrefs into a bundle unless it is "allowed" (see
	// domain.XrefSourceMeta.Redistribution and findRestrictedSources in
	// internal/adapters/sqlite).
	Redistribution string
}

// xrefJoinKey identifies the bridge concept a row's external id is claimed
// for: hostus' own (join_authority, join_id) xref lookup key.
type xrefJoinKey struct {
	joinAuthority string
	joinID        string
}

// xrefExtKey identifies one external authority's id — the (authority,
// ext_id) pair that is xref's actual primary key, and therefore the unit
// IngestXrefs' conflict detection groups by.
type xrefExtKey struct {
	authority string
	extID     string
}

// IngestXrefs resolves every row src provides against repo's EXISTING xref
// table (an ID-based join, never a name crosswalk) and writes the new
// cross-references src carries, one per non-conflicting row.
//
// Resolution is two-phase, exactly like IngestNameSpace/IngestCDM, and for
// the identical reason: the sqlite adapter runs with SetMaxOpenConns(1), so
// a repository read issued while the ingest transaction is open would
// deadlock. Phase 1 resolves every row's join key with no ingest
// transaction open; phase 2 opens one transaction and only writes, never
// reads the repository again.
//
// Phase 1 resolves two things, both as pre-transaction reads: each row's
// join key (resolveXrefJoinKeys) and the concept that ALREADY owns each of
// this source's external keys in the database, if any (resolveXrefExtOwners
// — see the cross-run conflict case below).
//
// Conflict handling is the crux of this ingest, and two genuinely different
// situations must be told apart (both counted, both sampled, never silently
// resolved into one outcome):
//
//  1. The SAME (authority, ext_id) — xref's actual primary key — claimed by
//     TWO OR MORE DISTINCT concepts. That happens either WITHIN this source
//     (two different join_ids carrying the identical external id: a genuine
//     upstream data conflict) or ACROSS runs (the key is already in the xref
//     table pointing at another concept — a previous run, an older CSV, or a
//     second source). Both are detected, because detectXrefConflicts folds
//     the pre-existing owner resolved in phase 1 into the same grouping. The
//     safe default is SKIP-AND-REPORT: every row sharing that (authority,
//     ext_id) is counted as Conflicting and nothing is written for it,
//     rather than guessing which concept it belongs to or letting whichever
//     row is written last silently overwrite (AddXref is an INSERT OR
//     REPLACE keyed on exactly this pair). A key whose pre-existing owner is
//     the SAME concept this run resolves it to is not a conflict, so
//     re-ingesting an unchanged source stays idempotent.
//  2. ONE concept legitimately receiving SEVERAL distinct ext_ids for the
//     SAME authority (e.g. two Wikidata items both carrying the same IPNI
//     id, so the same concept picks up two Wikidata QIDs). This is not a
//     conflict at all — xref's PK is (authority, ext_id), so two different
//     ext_ids never collide — and every one of those rows IS written
//     (Matched). It is additionally tallied in
//     XrefIngestReport.MultiPerAuthority purely so the phenomenon is
//     visible, not because it needed a decision.
//
// Because join_id resolution only ever walks existing xref rows —
// themselves only ever written for a taxon_concept row, which
// internal/application/ingest.go creates only for accepted rows (see
// upsertAcceptedConcept) — an xref written by this function can never land
// on a synonym-only row: there is no such row to resolve to in the first
// place. This is a structural invariant of the ID-based join, not something
// this function re-derives.
func IngestXrefs(ctx context.Context, repo output.Repository, src XrefRowSource, meta domain.XrefSourceMeta) (XrefIngestReport, error) {
	report := XrefIngestReport{Source: meta.ID, Redistribution: string(meta.Redistribution)}
	rows := src.Rows()
	report.Rows = len(rows)

	resolved, err := resolveXrefJoinKeys(ctx, repo, rows)
	if err != nil {
		return report, fmt.Errorf("application: resolving xref source %q: %w", meta.ID, err)
	}
	owners, err := resolveXrefExtOwners(ctx, repo, rows)
	if err != nil {
		return report, fmt.Errorf("application: resolving existing xref owners for source %q: %w", meta.ID, err)
	}
	conflicted := detectXrefConflicts(rows, resolved, owners)

	tx, err := repo.BeginTraitIngest(ctx)
	if err != nil {
		return report, fmt.Errorf("application: starting xref ingest for source %q: %w", meta.ID, err)
	}
	if err := tx.UpsertXrefSource(meta); err != nil {
		_ = tx.Rollback()
		return report, fmt.Errorf("application: recording xref source %q: %w", meta.ID, err)
	}

	tally := newXrefTally()
	for _, row := range rows {
		joinKey := xrefJoinKey{joinAuthority: row.JoinAuthority, joinID: row.JoinID}
		conceptID, ok := resolved[joinKey]
		if !ok {
			report.Unmatched++
			tally.countUnmatched(joinKey)
			continue
		}
		extKey := xrefExtKey{authority: row.Authority, extID: row.ExtID}
		if conflicted[extKey] {
			report.Conflicting++
			tally.countConflict(extKey)
			continue
		}
		if err := tx.AddXref(conceptID, domain.Xref{Authority: row.Authority, ExtID: row.ExtID}, meta.ID); err != nil {
			_ = tx.Rollback()
			return report, fmt.Errorf("application: writing xref %s:%s for concept %q, source %q: %w", row.Authority, row.ExtID, conceptID, meta.ID, err)
		}
		report.Matched++
		tally.countMatched(conceptID, row.Authority, row.ExtID)
	}

	if err := tx.Finalize(); err != nil {
		_ = tx.Rollback()
		return report, fmt.Errorf("application: finalizing xref ingest for source %q: %w", meta.ID, err)
	}
	if err := tx.Commit(); err != nil {
		return report, fmt.Errorf("application: committing xref ingest for source %q: %w", meta.ID, err)
	}

	tally.report(&report)
	return report, nil
}

// resolveXrefJoinKeys is the first half of IngestXrefs' phase 1: it maps
// every DISTINCT (join_authority, join_id) occurring in rows to its concept
// id. It must be called with no ingest transaction open — see IngestXrefs'
// doc comment.
func resolveXrefJoinKeys(ctx context.Context, repo output.Repository, rows []XrefRow) (map[xrefJoinKey]string, error) {
	return resolveConceptsByAuthority(ctx, repo, rows,
		func(r XrefRow) (string, string) { return r.JoinAuthority, r.JoinID },
		func(authority, id string) xrefJoinKey {
			return xrefJoinKey{joinAuthority: authority, joinID: id}
		})
}

// resolveXrefExtOwners is the second half of IngestXrefs' phase 1: it maps
// each DISTINCT external key (authority, ext_id) this source writes to the
// concept that ALREADY owns it in the repository, for the keys that are
// already present. Without it, conflict detection would be intra-source
// only: a key written by an earlier run (or an older CSV, or another
// source) pointing at a different concept would be silently repointed by
// AddXref's INSERT OR REPLACE, uncounted.
//
// It is the very same lookup resolveXrefJoinKeys performs, just keyed on
// this source's OWN (authority, ext_id) pairs instead of its join keys —
// hence the shared resolveConceptsByAuthority below. Like
// resolveXrefJoinKeys it must be called with no ingest transaction open (see
// IngestXrefs' doc comment); it is deliberately a separate pre-transaction
// read rather than a lookup inside the write loop.
func resolveXrefExtOwners(ctx context.Context, repo output.Repository, rows []XrefRow) (map[xrefExtKey]string, error) {
	return resolveConceptsByAuthority(ctx, repo, rows,
		func(r XrefRow) (string, string) { return r.Authority, r.ExtID },
		func(authority, id string) xrefExtKey {
			return xrefExtKey{authority: authority, extID: id}
		})
}

// resolveConceptsByAuthority is the shared batching engine behind both
// phase-1 lookups: pairOf extracts one (authority, id) pair per row, and the
// DISTINCT ids per authority are resolved with ONE repo.ConceptIDsByXref
// call each (in practice a handful of calls for 1.7 M rows) rather than one
// query per row. keyOf turns each resolved pair back into the caller's own
// key type. Only ids that actually have an xref row appear in the result,
// exactly as ConceptIDsByXref documents.
func resolveConceptsByAuthority[K comparable](
	ctx context.Context,
	repo output.Repository,
	rows []XrefRow,
	pairOf func(XrefRow) (string, string),
	keyOf func(authority, id string) K,
) (map[K]string, error) {
	distinctByAuthority := make(map[string]map[string]bool)
	for _, row := range rows {
		authority, id := pairOf(row)
		ids, ok := distinctByAuthority[authority]
		if !ok {
			ids = map[string]bool{}
			distinctByAuthority[authority] = ids
		}
		ids[id] = true
	}

	resolved := make(map[K]string)
	for authority, idSet := range distinctByAuthority {
		ids := make([]string, 0, len(idSet))
		for id := range idSet {
			ids = append(ids, id)
		}
		matched, err := repo.ConceptIDsByXref(ctx, authority, ids)
		if err != nil {
			return nil, fmt.Errorf("authority %q: %w", authority, err)
		}
		for id, conceptID := range matched {
			resolved[keyOf(authority, id)] = conceptID
		}
	}
	return resolved, nil
}

// detectXrefConflicts groups rows by their EXTERNAL key (authority, ext_id —
// xref's actual primary key) and reports which of those keys are claimed by
// two or more DISTINCT concepts. A key's claimants are every concept THIS
// source's rows resolve it to, plus — seeded from owners
// (resolveXrefExtOwners) — the concept that already owns it in the
// repository, so a cross-run/cross-source disagreement is detected exactly
// like an intra-source one. Seeding cannot manufacture a false conflict: a
// pre-existing owner that equals the concept this run resolves to collapses
// into the same one-element set (re-ingesting an unchanged source is
// idempotent), and a key whose rows are all Unmatched is never written
// either way. Rows whose join_id did not resolve at all contribute no
// concept to their group (they are Unmatched, not part of any conflict).
func detectXrefConflicts(rows []XrefRow, resolved map[xrefJoinKey]string, owners map[xrefExtKey]string) map[xrefExtKey]bool {
	concepts := make(map[xrefExtKey]map[string]bool)
	for _, row := range rows {
		conceptID, ok := resolved[xrefJoinKey{joinAuthority: row.JoinAuthority, joinID: row.JoinID}]
		if !ok {
			continue
		}
		extKey := xrefExtKey{authority: row.Authority, extID: row.ExtID}
		if concepts[extKey] == nil {
			concepts[extKey] = map[string]bool{}
			if owner, owned := owners[extKey]; owned {
				concepts[extKey][owner] = true
			}
		}
		concepts[extKey][conceptID] = true
	}

	conflicted := make(map[xrefExtKey]bool)
	for extKey, ids := range concepts {
		if len(ids) > 1 {
			conflicted[extKey] = true
		}
	}
	return conflicted
}

// xrefTally accumulates the per-row bookkeeping IngestXrefs needs for the
// parts of XrefIngestReport that are not plain counters.
type xrefTally struct {
	unmatched map[string]bool
	conflicts map[string]bool
	// perAuthorityConcepts[authority] is the set of concept ids that
	// received at least one WRITTEN xref for authority — its size is
	// XrefIngestReport.PerAuthority[authority].
	perAuthorityConcepts map[string]map[string]bool
	// perConceptExtIDs[authority][conceptID] is the set of DISTINCT ext_ids
	// WRITTEN for (authority, conceptID) — a size > 1 is the case (b)
	// "multiple ids, same authority, one concept" phenomenon.
	perConceptExtIDs map[string]map[string]map[string]bool
}

func newXrefTally() *xrefTally {
	return &xrefTally{
		unmatched:            map[string]bool{},
		conflicts:            map[string]bool{},
		perAuthorityConcepts: map[string]map[string]bool{},
		perConceptExtIDs:     map[string]map[string]map[string]bool{},
	}
}

func (t *xrefTally) countUnmatched(key xrefJoinKey) {
	t.unmatched[fmt.Sprintf("%s:%s", key.joinAuthority, key.joinID)] = true
}

func (t *xrefTally) countConflict(key xrefExtKey) {
	t.conflicts[fmt.Sprintf("%s:%s", key.authority, key.extID)] = true
}

func (t *xrefTally) countMatched(conceptID, authority, extID string) {
	if t.perAuthorityConcepts[authority] == nil {
		t.perAuthorityConcepts[authority] = map[string]bool{}
	}
	t.perAuthorityConcepts[authority][conceptID] = true

	if t.perConceptExtIDs[authority] == nil {
		t.perConceptExtIDs[authority] = map[string]map[string]bool{}
	}
	if t.perConceptExtIDs[authority][conceptID] == nil {
		t.perConceptExtIDs[authority][conceptID] = map[string]bool{}
	}
	t.perConceptExtIDs[authority][conceptID][extID] = true
}

// report writes the accumulated samples and per-authority breakdowns onto
// r. Every map is walked in SORTED key order before being folded into a
// deterministic slice/sample, since Go map iteration order is randomized
// and this report is printed by "hostus ingest" and compared across runs.
func (t *xrefTally) report(r *XrefIngestReport) {
	r.UnmatchedSample = sortedSample(t.unmatched)
	r.ConflictSample = sortedSample(t.conflicts)

	if len(t.perAuthorityConcepts) > 0 {
		r.PerAuthority = make(map[string]int, len(t.perAuthorityConcepts))
		for authority, concepts := range t.perAuthorityConcepts {
			r.PerAuthority[authority] = len(concepts)
		}
	}

	multiSample := map[string]bool{}
	multiPerAuthority := map[string]int{}
	authorities := make([]string, 0, len(t.perConceptExtIDs))
	for authority := range t.perConceptExtIDs {
		authorities = append(authorities, authority)
	}
	sort.Strings(authorities)
	for _, authority := range authorities {
		conceptIDs := make([]string, 0, len(t.perConceptExtIDs[authority]))
		for conceptID := range t.perConceptExtIDs[authority] {
			conceptIDs = append(conceptIDs, conceptID)
		}
		sort.Strings(conceptIDs)
		for _, conceptID := range conceptIDs {
			if len(t.perConceptExtIDs[authority][conceptID]) > 1 {
				multiPerAuthority[authority]++
				multiSample[fmt.Sprintf("%s:%s", authority, conceptID)] = true
			}
		}
	}
	if len(multiPerAuthority) > 0 {
		r.MultiPerAuthority = multiPerAuthority
	}
	r.MultiSample = sortedSample(multiSample)
}

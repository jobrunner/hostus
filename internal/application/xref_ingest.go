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
	// notice for anything that is not "allowed" — local ingest itself is
	// never gated by it.
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
// Resolution is two-phase, exactly like IngestTraits, and for the identical
// reason (see that function's doc comment on the sqlite adapter's
// SetMaxOpenConns(1)): phase 1 resolves every row's join key with no ingest
// transaction open; phase 2 opens one transaction and only writes, never
// reads the repository again.
//
// Conflict handling is the crux of this ingest, and two genuinely different
// situations must be told apart (both counted, both sampled, never silently
// resolved into one outcome):
//
//  1. The SAME (authority, ext_id) — xref's actual primary key — claimed by
//     TWO OR MORE DISTINCT concepts (because two different join_ids in the
//     source happen to carry the identical external id: a genuine upstream
//     data conflict). The safe default is SKIP-AND-REPORT: every row
//     sharing that (authority, ext_id) is counted as Conflicting and
//     nothing is written for it, rather than guessing which concept it
//     belongs to or letting whichever row is written last silently
//     overwrite (AddXref is an INSERT OR REPLACE keyed on exactly this
//     pair).
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
	conflicted := detectXrefConflicts(rows, resolved)

	tx, err := repo.BeginTraitIngest(ctx)
	if err != nil {
		return report, fmt.Errorf("application: starting xref ingest for source %q: %w", meta.ID, err)
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
		if err := tx.AddXref(conceptID, domain.Xref{Authority: row.Authority, ExtID: row.ExtID}); err != nil {
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

// resolveXrefJoinKeys is IngestXrefs' phase 1: it maps every DISTINCT
// (join_authority, join_id) occurring in rows to its concept id, batching
// one repo.ConceptIDsByXref call per distinct join_authority (in practice
// just "powo") rather than one query per row. It must be called with no
// ingest transaction open — see IngestXrefs' doc comment.
func resolveXrefJoinKeys(ctx context.Context, repo output.Repository, rows []XrefRow) (map[xrefJoinKey]string, error) {
	distinctByAuthority := make(map[string]map[string]bool)
	for _, row := range rows {
		ids, ok := distinctByAuthority[row.JoinAuthority]
		if !ok {
			ids = map[string]bool{}
			distinctByAuthority[row.JoinAuthority] = ids
		}
		ids[row.JoinID] = true
	}

	resolved := make(map[xrefJoinKey]string)
	for joinAuthority, idSet := range distinctByAuthority {
		ids := make([]string, 0, len(idSet))
		for id := range idSet {
			ids = append(ids, id)
		}
		matched, err := repo.ConceptIDsByXref(ctx, joinAuthority, ids)
		if err != nil {
			return nil, fmt.Errorf("join authority %q: %w", joinAuthority, err)
		}
		for joinID, conceptID := range matched {
			resolved[xrefJoinKey{joinAuthority: joinAuthority, joinID: joinID}] = conceptID
		}
	}
	return resolved, nil
}

// detectXrefConflicts groups rows by their EXTERNAL key (authority, ext_id —
// xref's actual primary key) and reports which of those keys are claimed by
// two or more DISTINCT resolved concepts. Rows whose join_id did not
// resolve at all contribute no concept to their group (they are Unmatched,
// not part of any conflict).
func detectXrefConflicts(rows []XrefRow, resolved map[xrefJoinKey]string) map[xrefExtKey]bool {
	concepts := make(map[xrefExtKey]map[string]bool)
	for _, row := range rows {
		conceptID, ok := resolved[xrefJoinKey{joinAuthority: row.JoinAuthority, joinID: row.JoinID}]
		if !ok {
			continue
		}
		extKey := xrefExtKey{authority: row.Authority, extID: row.ExtID}
		if concepts[extKey] == nil {
			concepts[extKey] = map[string]bool{}
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

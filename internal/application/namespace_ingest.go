package application

import (
	"context"
	"fmt"

	"github.com/jobrunner/hostus/internal/domain"
	"github.com/jobrunner/hostus/internal/ports/output"
)

// NameRow is the minimal shape of one canonical name-list row
// IngestNameSpace needs: the space's own spelling and its stable id. A
// concrete reader's row type (namelist.Row) is adapted into this DTO by the
// caller — the same RowSource-bridge pattern TraitRowSource/XrefRowSource use
// so application never imports internal/adapters/namelist directly
// (depguard).
//
// Rank/AcceptedTaxon are deliberately NOT carried. A name space contributes
// names, not taxonomy: hostus never builds a concept, a parent chain or a
// synonymy edge out of one, so a field this use case cannot act on would be a
// field a reader of the DTO would reasonably expect it to act on.
//
// Status WAS excluded on the same grounds and is now carried, which is the
// measured decision that exclusion invited. It is not taxonomy here: it does
// not create an edge, it decides WHICH of a space's names to report. A space
// maps many of its own spellings onto one backbone concept — on the real index
// 45% of concepts holding a eurosl entry carry 2 to 391 of them, one concept
// 391 — so without a status every target-space name was whichever row came
// back first. Measured example: the Hyssopus concept answered "Hyssopus
// ruber", one of 23. Downstream that is not cosmetic; it is the wrong name.
type NameRow struct {
	Taxon    string
	SourceID string
	// Status is the space's own nomenclatural status, verbatim
	// ("accepted", "synonym", "synonymobjective", ...).
	Status string
	// Family, OrderName and ClassName carry the row's classification above
	// family, ALREADY RESOLVED by the caller (see internal/app/ingest.go's
	// classificationFor) by walking the source's own parent chain up to the
	// nearest FAMILY/ORDER/CLASS ancestor. Empty when that walk found none.
	// Mirrors domain.Concept's fields of the same name — writeNameSpaceRow
	// is what carries them across.
	Family    string
	OrderName string
	ClassName string
	// VernacularDE is the row's German common name, verbatim from the
	// source (currently only GermanSL emits one). Empty when the source
	// carries none.
	VernacularDE string
}

// NameRowSource streams one name space's rows for IngestNameSpace.
type NameRowSource interface {
	Rows() []NameRow
}

// NameSpaceIngestReport summarizes one name space's crosswalk run. Like the
// trait crosswalk it reports LOSS explicitly — Matched+Unmatched+Ambiguous
// always sums to Rows — because attaching a flat name list to an existing
// backbone by name is lossy by construction and the loss is the measurement.
type NameSpaceIngestReport struct {
	Space     string
	Rows      int
	Matched   int
	Unmatched int
	Ambiguous int
	// Aggregates counts the rows whose own spelling denotes a collective
	// species (domain.IsAggregateName), and AggregatesMatched how many of
	// those resolved onto a concept. Reported separately from the overall
	// rate on purpose: WCVP carries ZERO aggregate-marked names, so an
	// aggregate can only ever resolve through the FLAGGED
	// aggregate-to-nominate rule, and a headline match rate that hid that
	// would misrepresent exactly the entries UC4's aggregate_policy is
	// about.
	Aggregates        int
	AggregatesMatched int
	// Concepts is the number of DISTINCT concepts that gained at least one
	// entry — the coverage number, which is smaller than Matched whenever a
	// space spells one concept several ways (FloraVeg's three Festuca ovina
	// rows).
	Concepts int
	// ReaderErrors counts rows the reader rejected before this use case ever
	// saw them (short row, empty taxon, empty source_id). Surfaced here so
	// Rows + ReaderErrors accounts for every line of the artifact and no
	// loss stage is invisible.
	ReaderErrors int
	// DuplicateExtIDs counts rows skipped because another row of this same
	// run already claimed their (space, ext_id) — name_space_entry's primary
	// key. Zero for FloraVeg (SeqIDs are unique), but an INSERT OR REPLACE
	// on a duplicate would otherwise be silent overwrite, which is exactly
	// the class of loss this project counts rather than absorbs.
	DuplicateExtIDs int
	// UnmatchedSample, AmbiguousSample and DuplicateSample are bounded
	// (unmatchedSampleCap), deterministic (sorted, deduplicated) samples, so
	// a reviewer can see WHICH names were lost without dumping every one.
	UnmatchedSample []string
	AmbiguousSample []string
	DuplicateSample []string
	// Normalized breaks down, per deterministic normalisation rule, the
	// entries that were actually WRITTEN under that rule — identical
	// semantics to TraitIngestReport.Normalized, and it agrees with
	// "SELECT resolution, COUNT(*) FROM name_space_entry" on the resulting
	// database. Rules that did not fire are absent.
	Normalized []RuleCount
	// FlaggedSample samples the names that matched only through a FLAGGED
	// rule (aggregate-to-nominate, autonym) — the two judgement calls that
	// equate circumscriptions which are not identical.
	FlaggedSample []string
	// Redistribution is this space's manifest-pinned redistribution value.
	// Local ingest is never gated by it; EXPORT is (see ExportBundle).
	Redistribution string
}

// IngestNameSpace resolves every name src provides against repo's name index
// and attaches the ones that resolve to their concept as name_space_entry
// rows, then records meta as the space's provenance.
//
// It runs in two strictly separated phases — RESOLVE first, WRITE second:
// the sqlite adapter runs with SetMaxOpenConns(1), so a repository read
// issued while the ingest transaction is open blocks forever waiting for a
// second connection. That is a real deadlock in "hostus ingest", not a test
// artifact. Phase 1 resolves every DISTINCT canonical name with no
// transaction open; phase 2 opens one transaction and only writes.
//
// Resolution REUSES resolveTraitName (crosswalk.go) — the shared crosswalk
// ladder (domain.NameCandidates: exact key first, then hybrid/genitive
// spelling rewrites, then the two flagged circumscription judgements), with
// the same three outcomes and the same refusal to guess:
//
//  1. the first candidate key the index answers decides the outcome;
//  2. no key answered -> Unmatched, nothing written;
//  3. the answering key resolves to two or more DISTINCT concepts (after
//     dropping sec.-space-only candidates when a backbone concept shares the
//     name — see policyPreferBackbone) -> Ambiguous, skipped entirely. This
//     path passes policyPreferBackbone, so the policyResolveGenuineBearer
//     homonym tie-break does NOT apply here — see the call site for why
//     that is deliberate.
//
// It is deliberately not a second name-resolution path. SP3's crosswalk only
// reached 98,0 % after normalisation rules nobody predicted from the raw hit
// rate, and this milestone has twice found the same silent-loss defect class
// in duplicated mappers.
//
// Phase 2 uses repo.BeginTraitIngest, NOT repo.BeginIngest: a name space is
// not a taxonomic backbone and must never leave a row in backbone_version
// (see output.Repository.BeginTraitIngest). Its Finalize is a no-op here —
// name-space entries are not indexed for autosuggest, since /v1/suggest
// answers in hostus' own backbone namespace.
//
// The space metadata is recorded regardless of match outcome: a space that
// resolves nothing at all must still be visible as ingested, and must still
// be visible to the redistribution gate.
func IngestNameSpace(ctx context.Context, repo output.Repository, src NameRowSource, meta domain.NameSpaceMeta) (NameSpaceIngestReport, error) {
	report := NameSpaceIngestReport{Space: meta.ID, Redistribution: string(meta.Redistribution)}
	rows := src.Rows()
	report.Rows = len(rows)

	resolved, err := resolveNameSpaceNames(ctx, repo, rows)
	if err != nil {
		return report, fmt.Errorf("application: resolving names for name space %q: %w", meta.ID, err)
	}

	tx, err := repo.BeginTraitIngest(ctx)
	if err != nil {
		return report, fmt.Errorf("application: starting name space ingest for %q: %w", meta.ID, err)
	}
	if err := tx.UpsertNameSpace(meta); err != nil {
		_ = tx.Rollback()
		return report, fmt.Errorf("application: recording name space %q: %w", meta.ID, err)
	}

	tally := newNameSpaceTally()
	for _, row := range rows {
		if err := writeNameSpaceRow(tx, row, resolved, meta, &report, tally); err != nil {
			_ = tx.Rollback()
			return report, err
		}
	}

	if err := tx.Finalize(); err != nil {
		_ = tx.Rollback()
		return report, fmt.Errorf("application: finalizing name space ingest for %q: %w", meta.ID, err)
	}
	if err := tx.Commit(); err != nil {
		return report, fmt.Errorf("application: committing name space ingest for %q: %w", meta.ID, err)
	}

	tally.report(&report)
	return report, nil
}

// writeNameSpaceRow classifies and (if it resolved) writes ONE row, updating
// report and tally. Split out of IngestNameSpace's loop to keep that
// function's cognitive complexity within the linter's bound; it carries the
// per-row decision, nothing else.
//
// Order of the checks matters and is deliberate: the aggregate tally counts
// EVERY aggregate-spelled row including the unmatched ones (the denominator
// of "how many of the 308 aggregates resolve" is all 308, not just the ones
// that happened to work), while the duplicate check runs only for rows that
// would actually be written — an unmatched row writes nothing and so cannot
// collide with anything.
func writeNameSpaceRow(
	tx output.IngestTx,
	row NameRow,
	resolved map[string]traitResolution,
	meta domain.NameSpaceMeta,
	report *NameSpaceIngestReport,
	tally *nameSpaceTally,
) error {
	aggregate := domain.IsAggregateName(row.Taxon)
	if aggregate {
		report.Aggregates++
	}

	res := resolved[domain.Canonicalize(row.Taxon)]
	switch {
	case res.ambiguous:
		report.Ambiguous++
		tally.countAmbiguous(row.Taxon)
		return nil
	case !res.matched:
		report.Unmatched++
		tally.countUnmatched(row.Taxon)
		return nil
	}

	report.Matched++
	if aggregate {
		report.AggregatesMatched++
	}
	if tally.claimed(row.SourceID) {
		report.DuplicateExtIDs++
		tally.countDuplicate(row.SourceID)
		return nil
	}

	entry := domain.NameSpaceEntry{
		Space:      meta.ID,
		ExtID:      row.SourceID,
		Name:       row.Taxon,
		Aggregate:  aggregate,
		Resolution: resolutionFor(res.rule),
		// The source list already carries its own status; dropping it here was
		// what made a target-space name arbitrary for every concept a space
		// maps several of its names onto.
		Status: row.Status,
	}
	if err := tx.AddNameSpaceEntry(res.conceptID, entry); err != nil {
		return fmt.Errorf("application: writing name space entry %s:%s for concept %q: %w", meta.ID, row.SourceID, res.conceptID, err)
	}
	if row.Family != "" || row.OrderName != "" || row.ClassName != "" {
		if err := tx.UpsertClassification(res.conceptID, row.Family, row.OrderName, row.ClassName); err != nil {
			return fmt.Errorf("application: writing classification for concept %q: %w", res.conceptID, err)
		}
	}
	if row.VernacularDE != "" {
		if err := tx.AddVernacularName(res.conceptID, domain.VernacularName{Language: "de", Name: row.VernacularDE}); err != nil {
			return fmt.Errorf("application: writing vernacular name for concept %q: %w", res.conceptID, err)
		}
	}
	tally.countWritten(res.conceptID, row.SourceID, row.Taxon, res.rule)
	return nil
}

// resolutionFor renders the rule that resolved a name for storage. It stays
// EMPTY for domain.RuleExact, exactly as traitValueFor does: an exact
// canonical match needed no normalisation, and storing "exact" would turn the
// ordinary case into an assertion on every row. Non-empty means "this entry
// reached its concept through a rewrite".
func resolutionFor(rule domain.NormalizationRule) string {
	if rule == domain.RuleExact {
		return ""
	}
	return string(rule)
}

// resolveNameSpaceNames is IngestNameSpace's phase 1: it maps every DISTINCT
// canonical name occurring in rows to its outcome, querying repo exactly once
// per distinct name. It must be called with no ingest transaction open — see
// IngestNameSpace's doc comment.
//
// The per-name resolution is resolveTraitName, unchanged: this is the SP3
// crosswalk, not a copy of it. nativeSpaceSet is loaded ONCE here, before the
// loop, not per name — it never changes within one ingest run.
func resolveNameSpaceNames(ctx context.Context, repo output.Repository, rows []NameRow) (map[string]traitResolution, error) {
	nativeSpaces, err := nativeSpaceSet(ctx, repo)
	if err != nil {
		return nil, fmt.Errorf("application: loading name-space set: %w", err)
	}

	resolved := make(map[string]traitResolution)
	for _, row := range rows {
		canon := domain.Canonicalize(row.Taxon)
		if _, seen := resolved[canon]; seen {
			continue
		}
		// policyPreferBackbone, explicitly: the homonym TIE-BREAK the trait
		// crosswalk uses (genuineBearerWinner) is measured for trait
		// vocabularies, not for name spaces. Inheriting it here would be
		// invisible (this report has no tiebroken counter and the CLI prints
		// none) and it would let a space gain a SECOND entry for one concept,
		// which is what domain.ResolveTargetSpace picks a target-space
		// spelling from — so /v1/translate would start choosing between two
		// spellings on evidence nobody gathered. See
		// TestIngestNameSpace_HomonymStaysAmbiguousHere.
		//
		// The two-tier FILTER (preferGenuineClaimants) is a separate,
		// safe-by-construction step that DOES apply here, unlike the
		// tie-break. Tier 1: a sec.-reference concept (e.g. one of CDM's
		// ~18 Standardliste sec. spaces) is an attribution detail of a
		// concept SOURCE distinct from the eurosl/germansl name space being
		// crosswalked, not a second genuine claimant on that spelling.
		// Counting it as one inflates "ambiguous" on well-known species.
		// Measured on a real full ingest that also loads CDM (2026-08-31,
		// rows/matched/ambiguous):
		//   eurosl:   before 139039/102916/19402 -> after 139039/113733/8585
		//   germansl: before  26599/ 3370/ 9934   -> after  26599/10436/2868
		// — matching (within noise) an ingest with CDM dropped entirely
		// (eurosl 113607/8194, germansl 10393/2586), while keeping CDM (and
		// /v1/translate) in the database. Tier 2: a name-space-NATIVE
		// concept (Fall B, nativespace_ingest.go) is dropped the same way
		// whenever a genuine backbone concept remains — see
		// preferGenuineClaimants' doc comment (spec 2026-09-01 B2) for the
		// measured genus/family fold counts this recovers. Self-shadowing
		// note: on a re-ingest of the SAME name space, that space is already
		// in nativeSpaces (loaded above), so its own Fall-B concepts from the
		// prior run are correctly demoted too — no special case needed for
		// idempotent re-ingest.
		res, err := resolveTraitName(ctx, repo, canon, policyPreferBackbone, nativeSpaces)
		if err != nil {
			return nil, fmt.Errorf("name %q: %w", row.Taxon, err)
		}
		resolved[canon] = res
	}
	return resolved, nil
}

// nameSpaceTally accumulates the bookkeeping NameSpaceIngestReport needs
// beyond its plain counters: the three name samples, the set of distinct
// concepts covered, the claimed ext_ids, and the per-rule breakdown.
type nameSpaceTally struct {
	unmatched map[string]bool
	ambiguous map[string]bool
	duplicate map[string]bool
	flagged   map[string]bool
	concepts  map[string]bool
	extIDs    map[string]bool
	ruleRows  map[domain.NormalizationRule]int
	ruleTaxa  map[domain.NormalizationRule]map[string]bool
}

func newNameSpaceTally() *nameSpaceTally {
	return &nameSpaceTally{
		unmatched: map[string]bool{},
		ambiguous: map[string]bool{},
		duplicate: map[string]bool{},
		flagged:   map[string]bool{},
		concepts:  map[string]bool{},
		extIDs:    map[string]bool{},
		ruleRows:  map[domain.NormalizationRule]int{},
		ruleTaxa:  map[domain.NormalizationRule]map[string]bool{},
	}
}

func (t *nameSpaceTally) countUnmatched(name string)  { t.unmatched[name] = true }
func (t *nameSpaceTally) countAmbiguous(name string)  { t.ambiguous[name] = true }
func (t *nameSpaceTally) countDuplicate(extID string) { t.duplicate[extID] = true }

// claimed reports whether extID has already been written in this run — the
// (space, ext_id) primary key, checked before the write rather than after,
// so a collision is counted instead of silently replacing the earlier row.
func (t *nameSpaceTally) claimed(extID string) bool { return t.extIDs[extID] }

// countWritten records one WRITTEN entry: it claims the row's ext_id (so a
// later row carrying the same one is counted as a duplicate rather than
// silently replacing this one) and adds its concept to the coverage set.
// Rows resolved on the plain exact key are not credited to a rule:
// Normalized exists to show what normalisation ADDED, and an exact hit added
// nothing.
func (t *nameSpaceTally) countWritten(conceptID, extID, name string, rule domain.NormalizationRule) {
	t.extIDs[extID] = true
	t.concepts[conceptID] = true
	if rule == domain.RuleExact {
		return
	}
	t.ruleRows[rule]++
	if t.ruleTaxa[rule] == nil {
		t.ruleTaxa[rule] = map[string]bool{}
	}
	t.ruleTaxa[rule][name] = true
	if rule.Flagged() {
		t.flagged[name] = true
	}
}

// report writes the accumulated samples, concept count and per-rule counts
// onto r. Every slice is sorted, since these are printed by "hostus ingest"
// and compared across runs while Go map iteration order is randomized.
func (t *nameSpaceTally) report(r *NameSpaceIngestReport) {
	r.UnmatchedSample = sortedSample(t.unmatched)
	r.AmbiguousSample = sortedSample(t.ambiguous)
	r.DuplicateSample = sortedSample(t.duplicate)
	r.FlaggedSample = sortedSample(t.flagged)
	r.Concepts = len(t.concepts)
	r.Normalized = ruleCounts(t.ruleRows, t.ruleTaxa)
}

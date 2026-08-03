package application

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/jobrunner/hostus/internal/domain"
	"github.com/jobrunner/hostus/internal/ports/output"
)

// Backbone is the subset of a manifest backbone entry the ingest use case
// needs. It deliberately does not reference internal/adapters/manifest.
// Backbone: application may only import domain and ports, never adapters,
// so the caller (the composition root) maps manifest.Backbone into this
// type before calling Ingest.
type Backbone struct {
	ID        string
	Version   string
	License   string
	SourceURL string
	Path      string
	// Redistribution gates ExportBundle (see internal/adapters/sqlite); it
	// is trusted verbatim here, since the composition root already ran it
	// through the manifest's schema-validated enum before constructing this
	// DTO.
	Redistribution string
}

// Dataset is the subset of a parsed dataset.yaml manifest the ingest use
// case needs: the pinned backbones plus the checksum of the manifest that
// was validated, so every ingested backbone_version row can be bound back
// to the exact manifest revision that authorized it.
type Dataset struct {
	Backbones   []Backbone
	ManifestSHA string
}

// TaxonRow is the minimal, backbone-agnostic shape of one taxon record
// Ingest needs: an id, whether it is the accepted name for its group (and
// if not, which accepted id it belongs to), the name fields to upsert, and
// its POWO cross-reference id (empty if none). A WCVP wcvp.TaxonRow (or any
// other backbone reader's row type) is adapted into this DTO by the caller,
// which is how application avoids importing internal/adapters/wcvp
// directly (depguard).
type TaxonRow struct {
	TaxonID         string
	AcceptedTaxonID string // meaningless when Accepted is true
	Accepted        bool
	Canonical       string
	Authorship      string
	Rank            string
	Status          string
	POWOID          string
	// ParentTaxonID is the source row's raw parent taxon id (WCVP
	// parentnameusageid), or "" if none. It is resolved into
	// domain.Concept.ParentID only when the id it names is itself an
	// ACCEPTED row in this same ingest (see ingestState.acceptedTaxonIDs) —
	// a parent that never got its own concept has no taxon_concept row to
	// point at, so ParentID is left "" (NULL) rather than dangling.
	ParentTaxonID string
	// BasionymTaxonID is the source row's raw basionym/original-name-usage
	// id (WCVP originalnameusageid), or "" if none. It is resolved into
	// domain.Name.BasionymID only when the id it names is present ANYWHERE
	// in this ingest's taxa (accepted or synonym — see
	// ingestState.presentTaxonIDs), since every row (not just accepted
	// ones) gets its own Name.
	BasionymTaxonID string
}

// DistributionRow is one area assignment, joined to a TaxonRow by TaxonID.
type DistributionRow struct {
	TaxonID  string
	AreaCode string
}

// RowSource streams one backbone's rows for Ingest. The caller adapts a
// concrete backbone reader (e.g. wcvp.Read's *wcvp.Dataset) into this
// interface; application never imports the adapter that produced the rows.
type RowSource interface {
	Taxa() []TaxonRow
	Distributions() []DistributionRow
}

// BackboneReport summarizes one backbone's ingest.
type BackboneReport struct {
	ID       string
	Names    int
	Concepts int
	Synonyms int
	Orphaned int // synonym rows whose accepted target was never ingested (dangling reference in the source data)
	// OtherRanks counts taxon rows whose "taxonrank" column didn't match one
	// of domain's canonical Rank constants and so normalized to
	// domain.RankOther (see domain.ParseRankLenient) — this is what makes
	// an exotic rank spelling (e.g. WCVP's "proles") visible in the report
	// instead of silently swallowed, mirroring TraitIngestReport.Unmatched.
	OtherRanks int
	// OtherRankSample is a bounded (otherRankSampleCap), deterministic
	// sample of the verbatim rank spellings counted in OtherRanks, most
	// frequent first (ties broken alphabetically) — see sortedRankCounts.
	OtherRankSample []RankVerbatimCount
	// Redistribution is this backbone's manifest-pinned redistribution
	// value (see domain.Redistribution), surfaced here so "hostus ingest"
	// can print a notice for anything that is not "allowed" — the local
	// ingest itself is never gated by it.
	Redistribution string
}

// RankVerbatimCount is one entry in BackboneReport.OtherRankSample: a
// verbatim source "taxonrank" spelling (as returned by
// domain.ParseRankLenient) paired with how many taxon rows carried it.
type RankVerbatimCount struct {
	Verbatim string
	Count    int
}

// otherRankSampleCap bounds BackboneReport.OtherRankSample the same way
// unmatchedSampleCap bounds TraitIngestReport.UnmatchedSample (see
// traits_ingest.go): a real WCVP ingest can carry dozens of distinct exotic
// rank spellings (see docs/research/reality-check.md's measured
// inventory), and the report must stay readable rather than dumping every
// one of them.
const otherRankSampleCap = 20

// IngestReport summarizes an Ingest run across every backbone in the dataset.
type IngestReport struct {
	Backbones []BackboneReport
}

// Ingest imports every backbone listed in ds into repo. For each backbone
// it opens a RowSource via readerFor and writes it in one transaction
// (output.Repository.BeginIngest/IngestTx), in two passes:
//
//  1. Every taxon row's Name is upserted. Rows that are the accepted name
//     for their group (TaxonRow.Accepted) additionally get a Concept,
//     an "accepted" concept_name link, a powo xref (if POWOID is set),
//     and their own distribution rows.
//  2. Every non-accepted (synonym) row is linked to its accepted Concept
//     (resolved via AcceptedTaxonID), grouping synonyms under the accepted
//     taxon rather than giving them their own concept. Rows whose accepted
//     target was never ingested (a dangling reference in the source data)
//     are skipped and counted as Orphaned, not treated as fatal — this
//     mirrors the source backbone readers' tolerance of dirty real-world
//     data.
//
// Each backbone_version record ds.ManifestSHA binds the ingest to the exact
// manifest revision that was validated.
func Ingest(ctx context.Context, ds *Dataset, readerFor func(Backbone) (RowSource, error), repo output.Repository) (IngestReport, error) {
	var report IngestReport
	for _, b := range ds.Backbones {
		rs, err := readerFor(b)
		if err != nil {
			return report, fmt.Errorf("application: opening reader for backbone %q: %w", b.ID, err)
		}
		br, err := ingestBackbone(ctx, b, ds.ManifestSHA, rs, repo)
		if err != nil {
			return report, err
		}
		report.Backbones = append(report.Backbones, br)
	}
	return report, nil
}

// ingestState carries the per-backbone context both ingest passes need,
// so pass1AcceptedAndNames/pass2Synonyms can stay small, single-purpose
// functions instead of one long one.
type ingestState struct {
	backbone    Backbone
	tx          output.IngestTx
	distByTaxon map[string][]DistributionRow
	// accepted tracks which source taxonIDs are ACCEPTED rows (get their
	// own concept). It is fully resolved from taxa BEFORE either pass
	// writes anything (see acceptedTaxonIDs) — not built up progressively
	// during pass 1 — because ParentTaxonID may name a taxon that appears
	// LATER in taxa than the row referencing it; resolving parent linkage
	// during pass 1 needs to see the complete set regardless of source
	// order. Pass 2 also uses it to tell a real accepted-concept link from
	// a dangling reference in the source data.
	accepted map[string]bool
	// basionymOf maps a source taxonID to the ALREADY-RESOLVED basionym
	// name id (nameID(backbone, row.BasionymTaxonID)) that row's Name got
	// in pass 1, or "" if BasionymTaxonID was empty or unresolvable
	// (target not in present). Built once upfront (see basionymIDsByTaxon)
	// so pass 2's homotypic rule can look up both the synonym's own
	// basionym AND the accepted row's basionym without re-deriving ids.
	basionymOf map[string]string
	// namesByTaxon and conceptsByTaxon record each row's already-written
	// Name/Concept from pass 1's first sub-pass (still missing
	// BasionymID/ParentID), keyed by source taxonID, so the second
	// sub-pass (linkSelfReferences) can look them up, set the
	// now-safe-to-write linkage field, and re-issue the identical INSERT
	// OR REPLACE.
	namesByTaxon    map[string]domain.Name
	conceptsByTaxon map[string]domain.Concept
	// otherRankCounts tallies, per verbatim source rank spelling, how many
	// taxon rows normalized to domain.RankOther via
	// domain.ParseRankLenient — the raw material for
	// BackboneReport.OtherRanks/OtherRankSample (see
	// finalizeOtherRanksReport).
	otherRankCounts map[string]int
}

// acceptedTaxonIDs resolves the full set of ACCEPTED source taxonIDs from
// taxa, in memory, before any write happens — this is what makes parent_id
// resolution order-independent (a child's parent may appear later in taxa
// than the child itself).
func acceptedTaxonIDs(taxa []TaxonRow) map[string]bool {
	m := make(map[string]bool, len(taxa))
	for _, row := range taxa {
		if row.Accepted {
			m[row.TaxonID] = true
		}
	}
	return m
}

// presentTaxonIDs resolves the full set of source taxonIDs present in taxa
// (accepted or synonym) — every row gets a Name, so this is the membership
// test for basionym_id resolution (which may point at any row, not just
// accepted ones).
func presentTaxonIDs(taxa []TaxonRow) map[string]bool {
	m := make(map[string]bool, len(taxa))
	for _, row := range taxa {
		m[row.TaxonID] = true
	}
	return m
}

// basionymIDsByTaxon resolves, for every source taxonID, the already-derived
// basionym NAME id (or "" if unresolvable), so pass 1 (writing Name.BasionymID)
// and pass 2 (the homotypic rule) can both look it up without recomputing.
func basionymIDsByTaxon(b Backbone, taxa []TaxonRow, present map[string]bool) map[string]string {
	m := make(map[string]string, len(taxa))
	for _, row := range taxa {
		id := ""
		if row.BasionymTaxonID != "" && present[row.BasionymTaxonID] {
			id = nameID(b.ID, row.BasionymTaxonID)
		}
		m[row.TaxonID] = id
	}
	return m
}

func ingestBackbone(ctx context.Context, b Backbone, manifestSHA string, rs RowSource, repo output.Repository) (BackboneReport, error) {
	report := BackboneReport{ID: b.ID, Redistribution: b.Redistribution}

	tx, err := repo.BeginIngest(ctx, domain.BackboneVersion{
		ID:             b.ID,
		Version:        b.Version,
		License:        b.License,
		SourceURL:      b.SourceURL,
		IngestedAt:     time.Now().UTC().Format(time.RFC3339),
		ManifestSHA:    manifestSHA,
		Redistribution: domain.Redistribution(b.Redistribution),
	})
	if err != nil {
		return report, fmt.Errorf("application: starting ingest for backbone %q: %w", b.ID, err)
	}

	distByTaxon := make(map[string][]DistributionRow)
	for _, d := range rs.Distributions() {
		distByTaxon[d.TaxonID] = append(distByTaxon[d.TaxonID], d)
	}

	taxa := rs.Taxa()
	present := presentTaxonIDs(taxa)
	st := &ingestState{
		backbone:        b,
		tx:              tx,
		distByTaxon:     distByTaxon,
		accepted:        acceptedTaxonIDs(taxa),
		basionymOf:      basionymIDsByTaxon(b, taxa, present),
		otherRankCounts: make(map[string]int),
	}

	if err := st.pass1AcceptedAndNames(taxa, &report); err != nil {
		_ = tx.Rollback()
		return report, err
	}
	if err := st.pass2Synonyms(taxa, &report); err != nil {
		_ = tx.Rollback()
		return report, err
	}
	st.finalizeOtherRanksReport(&report)
	if err := tx.Finalize(); err != nil {
		_ = tx.Rollback()
		return report, fmt.Errorf("application: finalizing FTS index for backbone %q: %w", b.ID, err)
	}

	if err := tx.Commit(); err != nil {
		return report, fmt.Errorf("application: committing backbone %q: %w", b.ID, err)
	}
	return report, nil
}

// pass1AcceptedAndNames upserts every row's Name, and additionally upserts
// a Concept (plus its "accepted" link, powo xref, and own distribution)
// for every row that is the accepted name of its group. It runs in two
// SUB-passes rather than setting basionym_id/parent_id on the first write:
// a row's basionym or parent may be a taxonID that appears LATER in taxa
// than the row referencing it, and SQLite's (default, immediate) foreign-key
// enforcement would reject that forward reference — the target genuinely
// doesn't exist YET at that point in the transaction, even though it is
// guaranteed to exist by the time the whole ingest commits. Sub-pass 1a
// therefore writes every Name/Concept WITHOUT its self-referencing column;
// sub-pass 1b (linkSelfReferences) then re-writes just those rows whose
// linkage resolved to something non-empty, once every row's target
// definitely already exists.
func (st *ingestState) pass1AcceptedAndNames(taxa []TaxonRow, report *BackboneReport) error {
	b := st.backbone
	st.namesByTaxon = make(map[string]domain.Name, len(taxa))
	st.conceptsByTaxon = make(map[string]domain.Concept)

	for _, row := range taxa {
		// ParseRankLenient (never ParseRank/error here) is what makes the
		// ingest tolerant of WCVP's full rank vocabulary: an exotic
		// spelling degrades to domain.RankOther instead of aborting the
		// whole backbone (see docs/research/reality-check.md's M1.0 — the
		// defect this fixes). st.otherRankCounts tallies every occurrence
		// so the report can surface them (finalizeOtherRanksReport).
		rank, verbatim := domain.ParseRankLenient(row.Rank)
		if rank == domain.RankOther {
			st.otherRankCounts[verbatim]++
		}

		// POWOID IS the IPNI id (POWO mints its taxon ids in IPNI's own
		// namespace, e.g. "396681-1") — spec §A.1's nomenclatural anchor —
		// so every name's ipni_id is populated straight from it, not just
		// the accepted row's powo xref.
		name := domain.Name{
			ID:         nameID(b.ID, row.TaxonID),
			Canonical:  row.Canonical,
			Authorship: row.Authorship,
			Rank:       rank,
			IPNIID:     row.POWOID,
		}
		if rank == domain.RankOther {
			name.RankVerbatim = verbatim
		}
		if err := st.tx.UpsertName(name); err != nil {
			return fmt.Errorf("application: backbone %q: %w", b.ID, err)
		}
		report.Names++
		st.namesByTaxon[row.TaxonID] = name

		if !row.Accepted {
			continue
		}
		concept, err := st.upsertAcceptedConcept(row, name, rank)
		if err != nil {
			return err
		}
		report.Concepts++
		st.conceptsByTaxon[row.TaxonID] = concept
	}

	return st.linkSelfReferences(taxa)
}

func (st *ingestState) upsertAcceptedConcept(row TaxonRow, name domain.Name, rank domain.Rank) (domain.Concept, error) {
	b := st.backbone
	cID := conceptID(b.ID, row.TaxonID)
	concept := domain.Concept{
		ID:           cID,
		BackboneID:   b.ID,
		AcceptedName: name,
		Rank:         rank,
		Status:       domain.ParseStatus(row.Status),
		// RankVerbatim mirrors name.RankVerbatim: both were derived from
		// the same row by the same domain.ParseRankLenient call, so
		// they're always identical (see domain.Concept.RankVerbatim's
		// doc comment for why it's still its own field).
		RankVerbatim: name.RankVerbatim,
	}
	if err := st.tx.UpsertConcept(concept); err != nil {
		return domain.Concept{}, fmt.Errorf("application: backbone %q: %w", b.ID, err)
	}
	if err := st.tx.LinkName(cID, name.ID, "accepted", nil); err != nil {
		return domain.Concept{}, fmt.Errorf("application: backbone %q: %w", b.ID, err)
	}
	if row.POWOID != "" {
		// Source "" (SQL NULL): a powo id read straight off a backbone taxon
		// row is not attributable to any ingested xref source — the
		// backbone's own redistribution value already gates it (see
		// schema.sql's note on xref.source).
		if err := st.tx.AddXref(cID, domain.Xref{Authority: "powo", ExtID: row.POWOID}, ""); err != nil {
			return domain.Concept{}, fmt.Errorf("application: backbone %q: %w", b.ID, err)
		}
	}
	for _, d := range st.distByTaxon[row.TaxonID] {
		if err := st.tx.AddDistribution(cID, domain.Distribution{AreaScheme: "wgsrpd_l3", AreaCode: d.AreaCode}); err != nil {
			return domain.Concept{}, fmt.Errorf("application: backbone %q: %w", b.ID, err)
		}
	}
	return concept, nil
}

// linkSelfReferences is pass 1's second sub-pass (see pass1AcceptedAndNames'
// doc comment): it re-writes name.basionym_id (via UpsertName) and
// taxon_concept.parent_id (via UpsertConcept) — both plain INSERT OR
// REPLACE, so re-issuing them with the same id and the linkage field now
// filled in is safe — for every row whose linkage resolved to something
// non-empty, once sub-pass 1a has guaranteed every row's Name/Concept
// already exists.
func (st *ingestState) linkSelfReferences(taxa []TaxonRow) error {
	b := st.backbone
	for _, row := range taxa {
		if basionymID := st.basionymOf[row.TaxonID]; basionymID != "" {
			name := st.namesByTaxon[row.TaxonID]
			name.BasionymID = basionymID
			if err := st.tx.UpsertName(name); err != nil {
				return fmt.Errorf("application: backbone %q: linking basionym for taxon %q: %w", b.ID, row.TaxonID, err)
			}
		}

		if !row.Accepted || row.ParentTaxonID == "" || !st.accepted[row.ParentTaxonID] {
			continue
		}
		concept := st.conceptsByTaxon[row.TaxonID]
		concept.ParentID = conceptID(b.ID, row.ParentTaxonID)
		if err := st.tx.UpsertConcept(concept); err != nil {
			return fmt.Errorf("application: backbone %q: linking parent for taxon %q: %w", b.ID, row.TaxonID, err)
		}
	}
	return nil
}

// pass2Synonyms links every non-accepted row's Name to the Concept its
// AcceptedTaxonID resolves to — grouping synonyms under the accepted taxon
// rather than giving them a concept of their own. Rows whose accepted
// target was never ingested in pass 1 (a dangling reference in the source
// data) are skipped and counted, not treated as fatal.
func (st *ingestState) pass2Synonyms(taxa []TaxonRow, report *BackboneReport) error {
	b := st.backbone
	for _, row := range taxa {
		if row.Accepted {
			continue
		}
		if !st.accepted[row.AcceptedTaxonID] {
			report.Orphaned++
			continue
		}
		cID := conceptID(b.ID, row.AcceptedTaxonID)
		if err := st.tx.LinkName(cID, nameID(b.ID, row.TaxonID), "synonym", st.homotypic(row)); err != nil {
			return fmt.Errorf("application: backbone %q: %w", b.ID, err)
		}
		report.Synonyms++
	}
	return nil
}

// homotypic implements the conservative homotypic rule for a synonym row:
// it returns a pointer to true when the basionym linkage PROVES the synonym
// shares its accepted concept's nomenclatural type, and nil (unknown, never
// a pointer to false) otherwise — concept_name.homotypic is NULL unless
// provably true, since NULL means "unknown" and a literal false would
// falsely assert "heterotypic", which absent linkage data can never prove.
//
// The rule fires (returns true) when any of these hold, comparing resolved
// NAME ids (nameID(backbone, taxonID), never raw source taxonIDs):
//
//  1. the synonym's own basionym equals the accepted name's id — the
//     synonym is a recombination of the accepted name (e.g. "Bromus ovinus
//     (L.) Scop." whose originalnameusageid IS "Festuca ovina L.", the
//     accepted name itself);
//  2. the synonym's own basionym equals the accepted name's basionym
//     (both non-empty) — synonym and accepted name are two recombinations
//     of the same underlying basionym;
//  3. the synonym IS itself the accepted name's basionym (i.e. the
//     accepted name is a recombination of this synonym).
//
// Any row whose basionym linkage doesn't resolve (empty, or pointing at a
// taxonID absent from this ingest) contributes "" to the comparison, which
// never equals another empty/absent side — so an unresolvable case always
// falls through to nil rather than a coincidental true.
func (st *ingestState) homotypic(row TaxonRow) *bool {
	b := st.backbone
	synonymNameID := nameID(b.ID, row.TaxonID)
	synonymBasionymID := st.basionymOf[row.TaxonID]
	acceptedNameID := nameID(b.ID, row.AcceptedTaxonID)
	acceptedBasionymID := st.basionymOf[row.AcceptedTaxonID]

	proven := (synonymBasionymID != "" && synonymBasionymID == acceptedNameID) ||
		(synonymBasionymID != "" && acceptedBasionymID != "" && synonymBasionymID == acceptedBasionymID) ||
		(acceptedBasionymID != "" && synonymNameID == acceptedBasionymID)
	if !proven {
		return nil
	}
	t := true
	return &t
}

// finalizeOtherRanksReport copies st.otherRankCounts (accumulated during
// pass 1) into report.OtherRanks/OtherRankSample, once every row has been
// processed. Kept as its own step (rather than updating report inline as
// rows are counted) so the bounding/sorting only happens once, not on
// every row.
func (st *ingestState) finalizeOtherRanksReport(report *BackboneReport) {
	for _, n := range st.otherRankCounts {
		report.OtherRanks += n
	}
	report.OtherRankSample = sortedRankCounts(st.otherRankCounts, otherRankSampleCap)
}

// sortedRankCounts returns a deterministic, bounded (at most cap) sample of
// counts, ordered by Count descending (most frequent exotic rank first, so
// the report leads with what matters most) and, for equal counts, by
// Verbatim ascending — the same "sorted for determinism, capped for size"
// approach as traits_ingest.go's sortedSample.
func sortedRankCounts(counts map[string]int, cap int) []RankVerbatimCount {
	if len(counts) == 0 {
		return nil
	}
	all := make([]RankVerbatimCount, 0, len(counts))
	for v, n := range counts {
		all = append(all, RankVerbatimCount{Verbatim: v, Count: n})
	}
	sort.Slice(all, func(i, j int) bool {
		// Both comparisons below are guarded (explicitly or by
		// construction) against the equal case, so mutating > to >= or
		// < to <= at CONDITIONALS_BOUNDARY is a genuinely equivalent
		// mutant: neither branch can ever observe operands it considers
		// equal.
		if all[i].Count != all[j].Count {
			return all[i].Count > all[j].Count
		}
		// This branch only runs when Counts are equal; Verbatim strings
		// are never equal to each other here regardless, since all was
		// built from a map's keys (each key appears at most once) — so
		// all[i].Verbatim == all[j].Verbatim never happens for i != j.
		return all[i].Verbatim < all[j].Verbatim
	})
	// len(all) >= cap is a genuinely equivalent mutant at
	// CONDITIONALS_BOUNDARY: at len(all) == cap exactly, all[:cap] IS all,
	// so both branches produce the identical slice and no test can observe
	// the difference — the same documented-equivalence class as
	// traits_ingest.go's sortedSample.
	if len(all) > cap {
		all = all[:cap]
	}
	return all
}

// nameID and conceptID derive stable, deterministic ids from the backbone
// id plus the backbone's own row id (WCVP's "taxonid" / plant_name_id),
// namespaced by kind so a name and its concept never collide despite
// deriving from the same source row.
func nameID(backboneID, taxonID string) string {
	return backboneID + ":name:" + taxonID
}

func conceptID(backboneID, taxonID string) string {
	return backboneID + ":concept:" + taxonID
}

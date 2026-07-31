package application

import (
	"context"
	"fmt"
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
}

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
	// accepted tracks which source taxonIDs got their own concept in pass
	// 1, so pass 2 can tell a real accepted-concept link from a dangling
	// reference in the source data.
	accepted map[string]bool
}

func ingestBackbone(ctx context.Context, b Backbone, manifestSHA string, rs RowSource, repo output.Repository) (BackboneReport, error) {
	report := BackboneReport{ID: b.ID}

	tx, err := repo.BeginIngest(ctx, domain.BackboneVersion{
		ID:          b.ID,
		Version:     b.Version,
		License:     b.License,
		SourceURL:   b.SourceURL,
		IngestedAt:  time.Now().UTC().Format(time.RFC3339),
		ManifestSHA: manifestSHA,
	})
	if err != nil {
		return report, fmt.Errorf("application: starting ingest for backbone %q: %w", b.ID, err)
	}

	distByTaxon := make(map[string][]DistributionRow)
	for _, d := range rs.Distributions() {
		distByTaxon[d.TaxonID] = append(distByTaxon[d.TaxonID], d)
	}

	st := &ingestState{backbone: b, tx: tx, distByTaxon: distByTaxon, accepted: make(map[string]bool)}
	taxa := rs.Taxa()

	if err := st.pass1AcceptedAndNames(taxa, &report); err != nil {
		_ = tx.Rollback()
		return report, err
	}
	if err := st.pass2Synonyms(taxa, &report); err != nil {
		_ = tx.Rollback()
		return report, err
	}
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
// for every row that is the accepted name of its group.
func (st *ingestState) pass1AcceptedAndNames(taxa []TaxonRow, report *BackboneReport) error {
	b := st.backbone
	for _, row := range taxa {
		rank, err := domain.ParseRank(row.Rank)
		if err != nil {
			return fmt.Errorf("application: backbone %q, taxon %q: %w", b.ID, row.TaxonID, err)
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
		if err := st.tx.UpsertName(name); err != nil {
			return fmt.Errorf("application: backbone %q: %w", b.ID, err)
		}
		report.Names++

		if !row.Accepted {
			continue
		}
		if err := st.upsertAcceptedConcept(row, name, rank); err != nil {
			return err
		}
		report.Concepts++
		st.accepted[row.TaxonID] = true
	}
	return nil
}

func (st *ingestState) upsertAcceptedConcept(row TaxonRow, name domain.Name, rank domain.Rank) error {
	b := st.backbone
	cID := conceptID(b.ID, row.TaxonID)
	concept := domain.Concept{
		ID:           cID,
		BackboneID:   b.ID,
		AcceptedName: name,
		Rank:         rank,
		Status:       domain.ParseStatus(row.Status),
	}
	if err := st.tx.UpsertConcept(concept); err != nil {
		return fmt.Errorf("application: backbone %q: %w", b.ID, err)
	}
	if err := st.tx.LinkName(cID, name.ID, "accepted", nil); err != nil {
		return fmt.Errorf("application: backbone %q: %w", b.ID, err)
	}
	if row.POWOID != "" {
		if err := st.tx.AddXref(cID, domain.Xref{Authority: "powo", ExtID: row.POWOID}); err != nil {
			return fmt.Errorf("application: backbone %q: %w", b.ID, err)
		}
	}
	for _, d := range st.distByTaxon[row.TaxonID] {
		if err := st.tx.AddDistribution(cID, domain.Distribution{AreaScheme: "wgsrpd_l3", AreaCode: d.AreaCode}); err != nil {
			return fmt.Errorf("application: backbone %q: %w", b.ID, err)
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
		if err := st.tx.LinkName(cID, nameID(b.ID, row.TaxonID), "synonym", nil); err != nil {
			return fmt.Errorf("application: backbone %q: %w", b.ID, err)
		}
		report.Synonyms++
	}
	return nil
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

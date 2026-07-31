package app

import (
	"context"
	"fmt"

	"github.com/jobrunner/hostus/internal/adapters/manifest"
	"github.com/jobrunner/hostus/internal/adapters/sqlite"
	"github.com/jobrunner/hostus/internal/adapters/wcvp"
	"github.com/jobrunner/hostus/internal/application"
)

// LoadDataset parses and schema-validates the dataset.yaml manifest at
// path, then adapts it into the application.Dataset DTO application.Ingest
// expects. application never imports internal/adapters/manifest (depguard),
// so this mapping lives here in the composition root — the one place
// allowed to know about both.
func LoadDataset(path string) (*application.Dataset, error) {
	ds, err := manifest.Parse(path)
	if err != nil {
		return nil, err
	}
	backbones := make([]application.Backbone, 0, len(ds.Backbones))
	for _, b := range ds.Backbones {
		backbones = append(backbones, application.Backbone{
			ID:        b.ID,
			Version:   b.Version,
			License:   b.License,
			SourceURL: b.SourceURL,
			Path:      b.Path,
		})
	}
	return &application.Dataset{Backbones: backbones, ManifestSHA: ds.ManifestSHA}, nil
}

// wcvpRowSource adapts a *wcvp.Dataset (T4's reader output) into
// application.RowSource, so application never imports internal/adapters/wcvp
// directly (depguard). It mirrors the identical bridge duplicated in
// internal/application/ingest_test.go and internal/adapters/http/taxa_test.go
// for their own boundary-respecting test fixtures; this is the one copy
// production code actually calls.
type wcvpRowSource struct{ ds *wcvp.Dataset }

func (s wcvpRowSource) Taxa() []application.TaxonRow {
	out := make([]application.TaxonRow, 0, len(s.ds.Taxa))
	for _, t := range s.ds.Taxa {
		out = append(out, application.TaxonRow{
			TaxonID:         t.TaxonID,
			AcceptedTaxonID: t.AcceptedNameUsageID,
			Accepted:        t.IsAccepted(),
			Canonical:       t.Canonical,
			Authorship:      t.Authorship,
			Rank:            t.Rank,
			Status:          t.Status,
			POWOID:          t.POWOID(),
		})
	}
	return out
}

func (s wcvpRowSource) Distributions() []application.DistributionRow {
	out := make([]application.DistributionRow, 0, len(s.ds.Distributions))
	for _, d := range s.ds.Distributions {
		out = append(out, application.DistributionRow{TaxonID: d.CoreID, AreaCode: d.AreaCode()})
	}
	return out
}

// readerFor opens b's local directory as a WCVP DwC-A bundle and adapts it
// into an application.RowSource. SP1 only ships the WCVP reader (T4); every
// backbone entry in the manifest is read through it regardless of ID, the
// same way T5/T7's own tests exercise Ingest. A per-ID reader dispatch (COL
// XR, Euro+Med, FloraVeg.EU) is future work once those readers land.
func readerFor(b application.Backbone) (application.RowSource, error) {
	ds, err := wcvp.Read(b.Path)
	if err != nil {
		return nil, fmt.Errorf("app: reading backbone %q at %q: %w", b.ID, b.Path, err)
	}
	return wcvpRowSource{ds: ds}, nil
}

// Ingest parses and validates the manifest at manifestPath, opens (or
// creates) the SQLite database at dbPath, and runs application.Ingest
// against it end to end, returning the per-backbone report. It is the
// entry point "hostus ingest" calls.
func Ingest(ctx context.Context, manifestPath, dbPath string) (application.IngestReport, error) {
	ds, err := LoadDataset(manifestPath)
	if err != nil {
		return application.IngestReport{}, err
	}

	repo, err := sqlite.Open(dbPath)
	if err != nil {
		return application.IngestReport{}, fmt.Errorf("app: opening database %q: %w", dbPath, err)
	}
	defer func() { _ = repo.Close() }()

	return application.Ingest(ctx, ds, readerFor, repo)
}

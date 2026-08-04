package sqlite_test

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jobrunner/hostus/internal/adapters/sqlite"
	"github.com/jobrunner/hostus/internal/domain"
)

// addFloraVegNameSpace records one name_space row with
// Redistribution=unknown (FloraVeg's real, measured value — no license
// statement is findable, pipelines/README.md) plus one name_space_entry for
// conceptID, making "floraveg" a genuinely contributing, non-allowed source
// for the AUT-scoped bundle. The name-space counterpart of
// addRestrictedXrefSource/addUnknownEIVETraitValue.
func addFloraVegNameSpace(t *testing.T, src *sqlite.DB, conceptID string) {
	t.Helper()
	ctx := context.Background()
	tx, err := src.BeginTraitIngest(ctx)
	if err != nil {
		t.Fatalf("BeginTraitIngest: unexpected error: %v", err)
	}
	if err := tx.UpsertNameSpace(domain.NameSpaceMeta{
		ID: "floraveg", Version: "2023-01-03",
		SourceURL:   "https://files.ibot.cas.cz/cevs/downloads/floraveg/Life_form.xlsx",
		ManifestSHA: "cafebabe", Redistribution: domain.RedistributionUnknown,
	}); err != nil {
		t.Fatalf("UpsertNameSpace: unexpected error: %v", err)
	}
	if err := tx.AddNameSpaceEntry(conceptID, domain.NameSpaceEntry{
		Space: "floraveg", ExtID: "5648", Name: "Festuca ovina aggr.",
		Aggregate: true, Resolution: string(domain.RuleAggregateToNominate),
	}); err != nil {
		t.Fatalf("AddNameSpaceEntry: unexpected error: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: unexpected error: %v", err)
	}
}

// TestExportBundle_RefusesByDefaultWhenNameSpaceNotAllowed is the SP9
// counterpart of the SP4 xref-gate regression, and it is written because
// "the gate already covers three source kinds" is exactly the assumption
// that let a hole through last time: every task was locally correct and the
// gate still leaked, because a NEW kind of source simply was not in the
// query. FloraVeg's redistribution is "unknown", so a bundle whose scope
// carries any of its entries must be refused by default, naming the source
// and its value — and no file may be left behind.
func TestExportBundle_RefusesByDefaultWhenNameSpaceNotAllowed(t *testing.T) {
	ctx := context.Background()
	src := ingestWCVPFixture(t)
	const conceptID = "wcvp:concept:405825" // Corynephorus canescens, AUT scope
	addFloraVegNameSpace(t, src, conceptID)

	out := filepath.Join(t.TempDir(), "bundle-namespace-refused.sqlite")
	_, err := sqlite.ExportBundle(ctx, src, out, sqlite.BundleOpts{Area: "AUT", SnapshotVersion: "v1"})
	if err == nil {
		t.Fatal("ExportBundle: want an error when a contributing name space is not redistribution-allowed, got nil")
	}
	if !strings.Contains(err.Error(), "floraveg (redistribution=unknown)") {
		t.Errorf("ExportBundle error = %q, want it to name the offending name space and its value", err)
	}
	if _, statErr := os.Stat(out); statErr == nil {
		t.Errorf("ExportBundle refused, but %q was still created", out)
	}
}

// TestExportBundle_ForceIncludeRestrictedNameSpace_SucceedsAndRecordsSource is
// the opt-out half: --force-include-restricted must export, record
// "floraveg" in bundle_meta.restricted_sources, and carry both the space's
// provenance row and its entry — so the exported file itself still says
// where those names came from and that they were never cleared.
func TestExportBundle_ForceIncludeRestrictedNameSpace_SucceedsAndRecordsSource(t *testing.T) {
	ctx := context.Background()
	src := ingestWCVPFixture(t)
	const conceptID = "wcvp:concept:405825"
	addFloraVegNameSpace(t, src, conceptID)

	out := filepath.Join(t.TempDir(), "bundle-namespace-forced.sqlite")
	if _, err := sqlite.ExportBundle(ctx, src, out, sqlite.BundleOpts{
		Area: "AUT", SnapshotVersion: "v1", AllowRestricted: true,
	}); err != nil {
		t.Fatalf("ExportBundle(AllowRestricted): unexpected error: %v", err)
	}

	meta := readBundleMeta(t, out)
	if meta.RestrictedSources != "floraveg" {
		t.Errorf("bundle_meta.restricted_sources = %q, want %q", meta.RestrictedSources, "floraveg")
	}

	raw, err := sql.Open("sqlite", out)
	if err != nil {
		t.Fatalf("sql.Open(%q): unexpected error: %v", out, err)
	}
	defer func() { _ = raw.Close() }()

	var version, sha, redistribution string
	if err := raw.QueryRow(`SELECT version, manifest_sha, redistribution FROM name_space WHERE id = 'floraveg'`).
		Scan(&version, &sha, &redistribution); err != nil {
		t.Fatalf("reading name_space from the bundle: unexpected error: %v", err)
	}
	if version != "2023-01-03" || sha != "cafebabe" || redistribution != "unknown" {
		t.Errorf("bundle name_space row = (%q, %q, %q), want (%q, %q, %q)",
			version, sha, redistribution, "2023-01-03", "cafebabe", "unknown")
	}

	var name, resolution string
	var aggregate int
	if err := raw.QueryRow(`SELECT name, aggregate, resolution FROM name_space_entry WHERE space = 'floraveg' AND ext_id = '5648'`).
		Scan(&name, &aggregate, &resolution); err != nil {
		t.Fatalf("reading name_space_entry from the bundle: unexpected error: %v", err)
	}
	if name != "Festuca ovina aggr." || aggregate != 1 || resolution != string(domain.RuleAggregateToNominate) {
		t.Errorf("bundle name_space_entry = (%q, %d, %q), want (%q, 1, %q)",
			name, aggregate, resolution, "Festuca ovina aggr.", domain.RuleAggregateToNominate)
	}
}

// TestExportBundle_OutOfScopeNameSpaceEntryIsNotCopied pins that the
// name-space copy is scoped STRUCTURALLY, by concept, not by export mode —
// the same lesson sec_reference's scoping records. An entry attached to a
// concept outside the area scope must neither trip the gate (its concept is
// not in the bundle) nor leak its name into the file, and the space's
// provenance row must not be copied either when no entry of it survives.
func TestExportBundle_OutOfScopeNameSpaceEntryIsNotCopied(t *testing.T) {
	ctx := context.Background()
	src := ingestWCVPFixture(t)
	// The GENUS-rank Corynephorus has no distribution row at all in the
	// fixture, so it is outside every area scope — see
	// TestExportBundle_AreaFilter_ExcludesConceptsOutsideArea.
	addFloraVegNameSpace(t, src, "wcvp:concept:451295")

	out := filepath.Join(t.TempDir(), "bundle-namespace-out-of-scope.sqlite")
	if _, err := sqlite.ExportBundle(ctx, src, out, sqlite.BundleOpts{Area: "AUT", SnapshotVersion: "v1"}); err != nil {
		t.Fatalf("ExportBundle: unexpected error: %v — an out-of-scope name space must not trip the gate", err)
	}

	meta := readBundleMeta(t, out)
	if meta.RestrictedSources != "" {
		t.Errorf("bundle_meta.restricted_sources = %q, want empty", meta.RestrictedSources)
	}

	raw, err := sql.Open("sqlite", out)
	if err != nil {
		t.Fatalf("sql.Open(%q): unexpected error: %v", out, err)
	}
	defer func() { _ = raw.Close() }()

	var entries, spaces int
	if err := raw.QueryRow(`SELECT count(*) FROM name_space_entry`).Scan(&entries); err != nil {
		t.Fatalf("counting name_space_entry: unexpected error: %v", err)
	}
	if entries != 0 {
		t.Errorf("bundle carries %d name_space_entry row(s), want 0", entries)
	}
	if err := raw.QueryRow(`SELECT count(*) FROM name_space`).Scan(&spaces); err != nil {
		t.Fatalf("counting name_space: unexpected error: %v", err)
	}
	if spaces != 0 {
		t.Errorf("bundle carries %d name_space row(s) whose entries are all out of scope, want 0", spaces)
	}
}

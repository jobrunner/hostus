package manifest_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jobrunner/hostus/internal/adapters/manifest"
)

func TestParse_ValidManifest(t *testing.T) {
	ds, err := manifest.Parse("testdata/dataset-valid.yaml")
	if err != nil {
		t.Fatalf("Parse: unexpected error: %v", err)
	}

	if got, want := len(ds.Backbones), 2; got != want {
		t.Fatalf("len(Backbones) = %d, want %d", got, want)
	}
	wcvp := ds.Backbones[0]
	if wcvp.ID != "wcvp" {
		t.Errorf("Backbones[0].ID = %q, want %q", wcvp.ID, "wcvp")
	}
	if wcvp.Version != "2026-06-15" {
		t.Errorf("Backbones[0].Version = %q, want %q", wcvp.Version, "2026-06-15")
	}
	if wcvp.License != "CC-BY-4.0" {
		t.Errorf("Backbones[0].License = %q, want %q", wcvp.License, "CC-BY-4.0")
	}
	wantPath := filepath.Join("testdata", "..", "..", "wcvp", "testdata", "wcvp-sample")
	if wcvp.Path != wantPath {
		t.Errorf("Backbones[0].Path = %q, want %q (relative paths resolved against the manifest's directory)", wcvp.Path, wantPath)
	}

	if len(ds.Raw) == 0 {
		t.Error("Raw = empty, want the manifest's raw bytes")
	}
	if ds.ManifestSHA == "" {
		t.Error("ManifestSHA = empty, want a non-empty checksum")
	}
	if len(ds.ManifestSHA) != 64 {
		t.Errorf("len(ManifestSHA) = %d, want 64 (hex-encoded SHA-256)", len(ds.ManifestSHA))
	}
}

func TestParse_ValidManifestTraitVocabularies(t *testing.T) {
	ds, err := manifest.Parse("testdata/dataset-valid.yaml")
	if err != nil {
		t.Fatalf("Parse: unexpected error: %v", err)
	}

	if got, want := len(ds.TraitVocabularies), 2; got != want {
		t.Fatalf("len(TraitVocabularies) = %d, want %d", got, want)
	}
	eive := ds.TraitVocabularies[0]
	if eive.ID != "eive" {
		t.Errorf("TraitVocabularies[0].ID = %q, want %q", eive.ID, "eive")
	}
	if eive.Version != "1.0" {
		t.Errorf("TraitVocabularies[0].Version = %q, want %q", eive.Version, "1.0")
	}
	if eive.Taxonomy != "euromed-via-eurosl" {
		t.Errorf("TraitVocabularies[0].Taxonomy = %q, want %q", eive.Taxonomy, "euromed-via-eurosl")
	}
	if eive.License != "CC-BY-4.0" {
		t.Errorf("TraitVocabularies[0].License = %q, want %q", eive.License, "CC-BY-4.0")
	}
	if eive.SourceURL != "https://example.org/eive" {
		t.Errorf("TraitVocabularies[0].SourceURL = %q, want %q", eive.SourceURL, "https://example.org/eive")
	}
	wantTraitPath := filepath.Join("testdata", "..", "..", "traits", "testdata", "eive-sample.csv")
	if eive.Path != wantTraitPath {
		t.Errorf("TraitVocabularies[0].Path = %q, want %q (relative paths resolved against the manifest's directory)", eive.Path, wantTraitPath)
	}
}

func TestParse_ValidManifestXrefSources(t *testing.T) {
	ds, err := manifest.Parse("testdata/dataset-valid.yaml")
	if err != nil {
		t.Fatalf("Parse: unexpected error: %v", err)
	}

	if got, want := len(ds.XrefSources), 1; got != want {
		t.Fatalf("len(XrefSources) = %d, want %d", got, want)
	}
	wikidata := ds.XrefSources[0]
	if wikidata.ID != "wikidata" {
		t.Errorf("XrefSources[0].ID = %q, want %q", wikidata.ID, "wikidata")
	}
	if wikidata.Version != "2026-08-02" {
		t.Errorf("XrefSources[0].Version = %q, want %q", wikidata.Version, "2026-08-02")
	}
	if wikidata.License != "CC0" {
		t.Errorf("XrefSources[0].License = %q, want %q", wikidata.License, "CC0")
	}
	if wikidata.SourceURL != "https://query.wikidata.org/sparql" {
		t.Errorf("XrefSources[0].SourceURL = %q, want %q", wikidata.SourceURL, "https://query.wikidata.org/sparql")
	}
	if wikidata.Redistribution != "allowed" {
		t.Errorf("XrefSources[0].Redistribution = %q, want %q", wikidata.Redistribution, "allowed")
	}
	wantXrefPath := filepath.Join("testdata", "..", "..", "xref", "testdata", "wikidata-sample.csv")
	if wikidata.Path != wantXrefPath {
		t.Errorf("XrefSources[0].Path = %q, want %q (relative paths resolved against the manifest's directory)", wikidata.Path, wantXrefPath)
	}
}

func TestParse_RejectsXrefSourceMissingRequiredField(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dataset.yaml")
	content := "backbones:\n" +
		"  - id: wcvp\n" +
		"    version: \"1.0\"\n" +
		"    path: /tmp/x\n" +
		"    redistribution: allowed\n" +
		"xref_sources:\n" +
		"  - id: wikidata\n" +
		"    version: \"1.0\"\n" +
		"    path: /tmp/x.csv\n" +
		"    redistribution: allowed\n" // missing license/source
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	if _, err := manifest.Parse(path); err == nil {
		t.Fatal("Parse: expected error for xref_sources entry missing required license/source, got nil")
	}
}

func TestParse_ValidExampleManifest(t *testing.T) {
	ds, err := manifest.Parse("../../../dataset.example.yaml")
	if err != nil {
		t.Fatalf("Parse(dataset.example.yaml): unexpected error: %v", err)
	}
	if got, want := len(ds.Backbones), 4; got != want {
		t.Fatalf("len(Backbones) = %d, want %d", got, want)
	}
	if got, want := len(ds.TraitVocabularies), 3; got != want {
		t.Fatalf("len(TraitVocabularies) = %d, want %d", got, want)
	}
	if got, want := len(ds.XrefSources), 1; got != want {
		t.Fatalf("len(XrefSources) = %d, want %d", got, want)
	}
	if ds.XrefSources[0].ID != "wikidata" {
		t.Errorf("XrefSources[0].ID = %q, want %q", ds.XrefSources[0].ID, "wikidata")
	}
	ids := make(map[string]bool, len(ds.Backbones))
	for _, b := range ds.Backbones {
		ids[b.ID] = true
		if b.Version == "" {
			t.Errorf("backbone %q has empty Version", b.ID)
		}
		if b.Path == "" {
			t.Errorf("backbone %q has empty Path", b.ID)
		}
	}
	for _, want := range []string{"wcvp", "colxr", "euromed", "floraveg"} {
		if !ids[want] {
			t.Errorf("Backbones missing %q", want)
		}
	}
}

func TestParse_ManifestSHAIsStableAndContentAddressed(t *testing.T) {
	a, err := manifest.Parse("testdata/dataset-valid.yaml")
	if err != nil {
		t.Fatalf("Parse: unexpected error: %v", err)
	}
	b, err := manifest.Parse("testdata/dataset-valid.yaml")
	if err != nil {
		t.Fatalf("Parse: unexpected error: %v", err)
	}
	if a.ManifestSHA != b.ManifestSHA {
		t.Errorf("ManifestSHA differs across identical parses: %q vs %q", a.ManifestSHA, b.ManifestSHA)
	}

	other, err := manifest.Parse("testdata/dataset-unknown-key.yaml")
	if err == nil && other.ManifestSHA == a.ManifestSHA {
		t.Error("ManifestSHA collided across two different manifest files")
	}
}

func TestParse_RejectsUnknownTopLevelKey(t *testing.T) {
	_, err := manifest.Parse("testdata/dataset-unknown-key.yaml")
	if err == nil {
		t.Fatal("Parse: expected error for an unknown top-level key, got nil")
	}
	if !strings.Contains(err.Error(), "schema validation") {
		t.Errorf("Parse error = %q, want it to mention schema validation (additionalProperties rejects the unknown key)", err)
	}
}

func TestParse_RejectsMissingRequiredField(t *testing.T) {
	_, err := manifest.Parse("testdata/dataset-missing-required.yaml")
	if err == nil {
		t.Fatal("Parse: expected error for a missing required field, got nil")
	}
	if !strings.Contains(err.Error(), "schema validation") {
		t.Errorf("Parse error = %q, want it to mention schema validation (required rejects the missing field)", err)
	}
}

func TestParse_RejectsMissingRedistribution(t *testing.T) {
	_, err := manifest.Parse("testdata/dataset-missing-redistribution.yaml")
	if err == nil {
		t.Fatal("Parse: expected error for a backbone missing redistribution, got nil")
	}
	if !strings.Contains(err.Error(), "schema validation") {
		t.Errorf("Parse error = %q, want it to mention schema validation (required rejects the missing field)", err)
	}
}

func TestParse_RejectsInvalidRedistributionValue(t *testing.T) {
	_, err := manifest.Parse("testdata/dataset-invalid-redistribution.yaml")
	if err == nil {
		t.Fatal("Parse: expected error for an invalid redistribution value, got nil")
	}
	if !strings.Contains(err.Error(), "schema validation") {
		t.Errorf("Parse error = %q, want it to mention schema validation (enum rejects the invalid value)", err)
	}
}

func TestParse_MissingAndInvalidRedistributionAreDistinctErrors(t *testing.T) {
	_, missingErr := manifest.Parse("testdata/dataset-missing-redistribution.yaml")
	_, invalidErr := manifest.Parse("testdata/dataset-invalid-redistribution.yaml")
	if missingErr == nil || invalidErr == nil {
		t.Fatal("expected both invalid manifests to fail Parse")
	}
	if missingErr.Error() == invalidErr.Error() {
		t.Errorf("missing-redistribution and invalid-redistribution errors are identical (%q); want distinct failure messages", missingErr)
	}
}

func TestParse_ValidManifestParsesRedistribution(t *testing.T) {
	ds, err := manifest.Parse("testdata/dataset-valid.yaml")
	if err != nil {
		t.Fatalf("Parse: unexpected error: %v", err)
	}
	for _, b := range ds.Backbones {
		if b.Redistribution != "allowed" {
			t.Errorf("Backbone %q Redistribution = %q, want %q", b.ID, b.Redistribution, "allowed")
		}
	}
	for _, tv := range ds.TraitVocabularies {
		if tv.Redistribution != "allowed" {
			t.Errorf("TraitVocabulary %q Redistribution = %q, want %q", tv.ID, tv.Redistribution, "allowed")
		}
	}
}

func TestParse_RejectsTraitVocabularyMissingRequiredField(t *testing.T) {
	_, err := manifest.Parse("testdata/dataset-trait-vocab-missing-required.yaml")
	if err == nil {
		t.Fatal("Parse: expected error for a trait vocabulary entry missing license/source/path, got nil")
	}
	if !strings.Contains(err.Error(), "schema validation") {
		t.Errorf("Parse error = %q, want it to mention schema validation", err)
	}
}

func TestParse_UnknownKeyAndMissingRequiredAreDistinctErrors(t *testing.T) {
	_, unknownKeyErr := manifest.Parse("testdata/dataset-unknown-key.yaml")
	_, missingFieldErr := manifest.Parse("testdata/dataset-missing-required.yaml")
	if unknownKeyErr == nil || missingFieldErr == nil {
		t.Fatal("expected both invalid manifests to fail Parse")
	}
	if unknownKeyErr.Error() == missingFieldErr.Error() {
		t.Errorf("unknown-key and missing-required errors are identical (%q); want distinct failure messages", unknownKeyErr)
	}
}

func TestParse_MissingFile(t *testing.T) {
	if _, err := manifest.Parse("testdata/does-not-exist.yaml"); err == nil {
		t.Fatal("Parse: expected error for a missing file, got nil")
	}
}

func TestParse_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dataset.yaml")
	writeFile(t, path, "backbones: [this is not: valid yaml: :::")
	if _, err := manifest.Parse(path); err == nil {
		t.Fatal("Parse: expected error for malformed YAML, got nil")
	}
}

func TestParse_AbsolutePathLeftUntouched(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dataset.yaml")
	writeFile(t, path, `backbones:
  - id: wcvp
    version: "2026-06-15"
    path: /already/absolute
    redistribution: allowed
`)
	ds, err := manifest.Parse(path)
	if err != nil {
		t.Fatalf("Parse: unexpected error: %v", err)
	}
	if got, want := ds.Backbones[0].Path, "/already/absolute"; got != want {
		t.Errorf("Backbones[0].Path = %q, want %q (already-absolute paths must not be rejoined)", got, want)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}
}

func TestParseConceptSourceResolvesBothPathsAndRequiresRedistribution(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dataset.yaml")
	if err := os.WriteFile(path, []byte(`backbones:
  - id: wcvp
    version: "2026-06-15"
    path: ./backbones/wcvp
    redistribution: allowed
concept_sources:
  - id: cdm
    version: "2026-08-02"
    source: https://api.cybertaxonomy.org/rl_standardliste
    concepts: ./cdm/concepts.csv
    relations: ./cdm/relations.csv
    redistribution: unknown
`), 0o600); err != nil {
		t.Fatalf("writing manifest: %v", err)
	}
	ds, err := manifest.Parse(path)
	if err != nil {
		t.Fatalf("Parse: unexpected error: %v", err)
	}
	if len(ds.ConceptSources) != 1 {
		t.Fatalf("got %d concept sources, want 1", len(ds.ConceptSources))
	}
	cs := ds.ConceptSources[0]
	if cs.Concepts != filepath.Join(dir, "cdm/concepts.csv") {
		t.Errorf("concepts path = %q, not resolved against the manifest dir", cs.Concepts)
	}
	if cs.Relations != filepath.Join(dir, "cdm/relations.csv") {
		t.Errorf("relations path = %q, not resolved against the manifest dir", cs.Relations)
	}
	if cs.Redistribution != "unknown" {
		t.Errorf("redistribution = %q, want unknown", cs.Redistribution)
	}
}

func TestParseConceptSourceRejectsMissingRedistribution(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dataset.yaml")
	if err := os.WriteFile(path, []byte(`backbones:
  - id: wcvp
    version: "2026-06-15"
    path: ./backbones/wcvp
    redistribution: allowed
concept_sources:
  - id: cdm
    version: "2026-08-02"
    source: https://api.cybertaxonomy.org/rl_standardliste
    concepts: ./cdm/concepts.csv
    relations: ./cdm/relations.csv
`), 0o600); err != nil {
		t.Fatalf("writing manifest: %v", err)
	}
	if _, err := manifest.Parse(path); err == nil {
		t.Fatal("a concept source without redistribution must fail schema validation")
	}
}

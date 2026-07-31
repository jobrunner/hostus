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

	if got, want := len(ds.TraitVocabularies), 2; got != want {
		t.Fatalf("len(TraitVocabularies) = %d, want %d", got, want)
	}
	if ds.TraitVocabularies[0].ID != "eive" {
		t.Errorf("TraitVocabularies[0].ID = %q, want %q", ds.TraitVocabularies[0].ID, "eive")
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

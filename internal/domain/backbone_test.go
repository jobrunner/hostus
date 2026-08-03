package domain_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/jobrunner/hostus/internal/domain"
)

func TestErrNotFound_WrapsWithErrorsIs(t *testing.T) {
	wrapped := fmt.Errorf("concept %q: %w", "abc", domain.ErrNotFound)
	if !errors.Is(wrapped, domain.ErrNotFound) {
		t.Fatalf("errors.Is(%v, domain.ErrNotFound) = false, want true", wrapped)
	}
}

func TestBackboneVersion_Fields(t *testing.T) {
	bv := domain.BackboneVersion{
		ID:          "wcvp",
		Version:     "2026-06-15",
		License:     "CC-BY-4.0",
		SourceURL:   "https://example.org/wcvp.zip",
		IngestedAt:  "2026-07-31T00:00:00Z",
		ManifestSHA: "deadbeef",
	}
	if bv.ID != "wcvp" || bv.Version != "2026-06-15" || bv.ManifestSHA != "deadbeef" {
		t.Fatalf("BackboneVersion fields did not round-trip: %+v", bv)
	}
}

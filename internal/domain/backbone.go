package domain

import "errors"

// ErrNotFound is returned by output.Repository lookups (e.g. Concept) when
// the requested entity does not exist. Adapters must wrap it with
// errors.Is-compatible errors (fmt.Errorf("...: %w", ErrNotFound)) rather
// than returning a fresh sentinel, so callers can rely on errors.Is(err,
// ErrNotFound) regardless of adapter.
var ErrNotFound = errors.New("domain: not found")

// BackboneVersion identifies one ingested taxonomic backbone artifact
// (e.g. WCVP) and its provenance, mirroring the backbone_version table
// (spec §4.3).
type BackboneVersion struct {
	ID          string // e.g. "wcvp"
	Version     string // e.g. "2026-06-15"; never "latest" (immutable identity)
	License     string
	SourceURL   string
	IngestedAt  string // RFC3339 timestamp
	ManifestSHA string // checksum of the validated manifest
}

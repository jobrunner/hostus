// Package manifest reads and validates dataset.yaml — the fixed-name
// manifest that pins every backbone/trait-vocabulary artifact hostus
// ingests (spec §D.2 / §4.4). Validation is double-gated: an embedded
// JSON Schema (2020-12) checks required fields and rejects unknown ones via
// additionalProperties:false, and the YAML decode itself runs with
// KnownFields(true) as a second, independent guard against typos that a
// looser schema might miss.
package manifest

import (
	"bytes"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/google/jsonschema-go/jsonschema"
	yaml "go.yaml.in/yaml/v3"
)

//go:embed dataset.schema.json
var schemaJSON []byte

// Backbone is one pinned backbone artifact entry (spec §D.2): an immutable
// version/license/source-URL identity plus the local filesystem path to its
// bundle (resolved to an absolute path relative to the manifest file by
// Parse, so callers never have to guess the base directory).
type Backbone struct {
	ID        string `yaml:"id" json:"id"`
	Version   string `yaml:"version" json:"version"`
	License   string `yaml:"license,omitempty" json:"license,omitempty"`
	SourceURL string `yaml:"source,omitempty" json:"source,omitempty"`
	Path      string `yaml:"path" json:"path"`
	Note      string `yaml:"note,omitempty" json:"note,omitempty"`
}

// TraitVocabulary is one pinned trait-vocabulary entry (spec §D.2): an
// immutable version/license/source-URL identity plus the local filesystem
// path to its canonical trait CSV (see internal/adapters/traits), resolved
// to an absolute path relative to the manifest file by Parse, exactly like
// Backbone.Path.
type TraitVocabulary struct {
	ID        string `yaml:"id" json:"id"`
	Version   string `yaml:"version" json:"version"`
	Taxonomy  string `yaml:"taxonomy" json:"taxonomy"`
	License   string `yaml:"license" json:"license"`
	SourceURL string `yaml:"source" json:"source"`
	Path      string `yaml:"path" json:"path"`
}

// Dataset is the parsed, validated contents of a dataset.yaml manifest.
type Dataset struct {
	Backbones         []Backbone        `yaml:"backbones" json:"backbones"`
	TraitVocabularies []TraitVocabulary `yaml:"trait_vocabularies,omitempty" json:"trait_vocabularies,omitempty"`

	// Raw holds the exact bytes read from disk, and ManifestSHA their
	// SHA-256 hex digest — so an ingest can record manifest_sha and bind
	// itself to the precise manifest revision that was validated.
	Raw         []byte
	ManifestSHA string
}

var resolvedSchema *jsonschema.Resolved

func init() {
	var schema jsonschema.Schema
	if err := json.Unmarshal(schemaJSON, &schema); err != nil {
		panic(fmt.Sprintf("manifest: embedded schema is invalid JSON: %v", err))
	}
	rs, err := schema.Resolve(nil)
	if err != nil {
		panic(fmt.Sprintf("manifest: embedded schema failed to resolve: %v", err))
	}
	resolvedSchema = rs
}

// Parse reads the dataset.yaml manifest at path, validates it against the
// embedded JSON Schema (2020-12; required fields, additionalProperties:
// false), and separately re-decodes it with KnownFields(true) as a second,
// independent unknown-field guard. Backbone.Path entries that are not
// already absolute are resolved relative to path's directory.
func Parse(path string) (*Dataset, error) {
	raw, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return nil, fmt.Errorf("manifest: reading %s: %w", path, err)
	}

	// Schema validation: decode loosely into a generic value first, since
	// [jsonschema.Resolved.Validate] expects a value shaped like the
	// result of unmarshaling JSON into `any` — a strict struct decode
	// would hide missing/extra keys behind zero values.
	var generic any
	if err := yaml.Unmarshal(raw, &generic); err != nil {
		return nil, fmt.Errorf("manifest: parsing %s: %w", path, err)
	}
	if err := resolvedSchema.Validate(generic); err != nil {
		return nil, fmt.Errorf("manifest: %s failed schema validation: %w", path, err)
	}

	// Strict decode: KnownFields(true) rejects any key the Dataset/Backbone/
	// TraitVocabulary structs don't declare, independent of the schema
	// check above.
	var ds Dataset
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)
	if err := dec.Decode(&ds); err != nil {
		return nil, fmt.Errorf("manifest: decoding %s: %w", path, err)
	}

	baseDir := filepath.Dir(path)
	for i, b := range ds.Backbones {
		if b.Path != "" && !filepath.IsAbs(b.Path) {
			ds.Backbones[i].Path = filepath.Join(baseDir, b.Path)
		}
	}
	for i, tv := range ds.TraitVocabularies {
		if tv.Path != "" && !filepath.IsAbs(tv.Path) {
			ds.TraitVocabularies[i].Path = filepath.Join(baseDir, tv.Path)
		}
	}

	sum := sha256.Sum256(raw)
	ds.Raw = raw
	ds.ManifestSHA = hex.EncodeToString(sum[:])

	return &ds, nil
}

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
	// Redistribution is required (schema-enforced): allowed|restricted|unknown.
	// See internal/domain.Redistribution — it gates ExportBundle, never
	// local ingest.
	Redistribution string `yaml:"redistribution" json:"redistribution"`
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
	// Redistribution is required (schema-enforced): allowed|restricted|unknown.
	// See internal/domain.Redistribution — it gates ExportBundle, never
	// local ingest.
	Redistribution string `yaml:"redistribution" json:"redistribution"`
}

// XrefSource is one pinned cross-reference source entry (SP4): an
// immutable version/license/source-URL identity plus the local filesystem
// path to its canonical xref CSV (see internal/adapters/xref), resolved to
// an absolute path relative to the manifest file by Parse, exactly like
// Backbone.Path/TraitVocabulary.Path.
type XrefSource struct {
	ID        string `yaml:"id" json:"id"`
	Version   string `yaml:"version" json:"version"`
	License   string `yaml:"license" json:"license"`
	SourceURL string `yaml:"source" json:"source"`
	Path      string `yaml:"path" json:"path"`
	// Redistribution is required (schema-enforced): allowed|restricted|unknown.
	// See internal/domain.Redistribution — it gates ExportBundle, never
	// local ingest. Wikidata itself is CC0 (allowed).
	Redistribution string `yaml:"redistribution" json:"redistribution"`
}

// ConceptSource is one pinned taxon-CONCEPT source entry (SP5): a source
// that contributes concepts scoped by a sec. reference space plus the typed
// relations between them (today: the CDM rl_standardliste harvest). It is
// deliberately not a Backbone entry even though its concepts land in
// taxon_concept under their own backbone_version row: it is pinned by TWO
// canonical CSVs rather than one directory, and conflating the two shapes
// would mean one of them carries a meaningless field.
//
// Concepts/Relations are resolved to absolute paths relative to the manifest
// file by Parse, exactly like Backbone.Path.
type ConceptSource struct {
	ID        string `yaml:"id" json:"id"`
	Version   string `yaml:"version" json:"version"`
	License   string `yaml:"license,omitempty" json:"license,omitempty"`
	SourceURL string `yaml:"source" json:"source"`
	Concepts  string `yaml:"concepts" json:"concepts"`
	Relations string `yaml:"relations" json:"relations"`
	Note      string `yaml:"note,omitempty" json:"note,omitempty"`
	// Redistribution is required (schema-enforced): allowed|restricted|unknown.
	// CDM is "unknown" — no license is findable anywhere on the portal, the
	// API or the payloads (pipelines/cdm/README.md).
	Redistribution string `yaml:"redistribution" json:"redistribution"`
}

// NameSpace is one pinned NAME-SPACE entry (SP9, UC4): a checklist that
// contributes names but no taxonomy, pinned by its canonical name-list CSV
// (see internal/adapters/namelist and pipelines/README.md's "Canonical CSV
// contract (name lists)"). Path is resolved to an absolute path relative to
// the manifest file by Parse, exactly like Backbone.Path.
//
// It is deliberately not a `backbones:` entry even though FloraVeg was
// listed as one before SP9: a backbone entry pins a DwC-A bundle DIRECTORY
// and produces taxon_concept rows, while a name space pins ONE canonical CSV
// and produces none — it attaches to concepts an existing backbone already
// holds. Leaving it under `backbones:` meant it was read by the WCVP DwC-A
// reader (internal/app.readerFor reads every backbone entry through it), so
// it could never actually be ingested.
//
// License/SourceURL are optional here, unlike TraitVocabulary: the name-list
// sources are exactly the ones with NO findable license (pipelines/README.md)
// — which is why Redistribution stays schema-required and gates ExportBundle.
type NameSpace struct {
	ID        string `yaml:"id" json:"id"`
	Version   string `yaml:"version" json:"version"`
	License   string `yaml:"license,omitempty" json:"license,omitempty"`
	SourceURL string `yaml:"source,omitempty" json:"source,omitempty"`
	Path      string `yaml:"path" json:"path"`
	Note      string `yaml:"note,omitempty" json:"note,omitempty"`
	// Redistribution is required (schema-enforced): allowed|restricted|unknown.
	// FloraVeg's is "unknown".
	Redistribution string `yaml:"redistribution" json:"redistribution"`
}

// Dataset is the parsed, validated contents of a dataset.yaml manifest.
type Dataset struct {
	Backbones         []Backbone        `yaml:"backbones" json:"backbones"`
	TraitVocabularies []TraitVocabulary `yaml:"trait_vocabularies,omitempty" json:"trait_vocabularies,omitempty"`
	XrefSources       []XrefSource      `yaml:"xref_sources,omitempty" json:"xref_sources,omitempty"`
	ConceptSources    []ConceptSource   `yaml:"concept_sources,omitempty" json:"concept_sources,omitempty"`
	NameSpaces        []NameSpace       `yaml:"name_spaces,omitempty" json:"name_spaces,omitempty"`

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

	resolvePaths(&ds, filepath.Dir(path))

	sum := sha256.Sum256(raw)
	ds.Raw = raw
	ds.ManifestSHA = hex.EncodeToString(sum[:])

	return &ds, nil
}

// resolvePaths rewrites every relative artifact path in ds to an absolute
// one, resolved against baseDir (the manifest file's own directory), so
// callers never have to guess what a relative path was relative to. Split out
// of Parse because Parse's cognitive complexity is dominated by validation,
// not by this bookkeeping.
func resolvePaths(ds *Dataset, baseDir string) {
	resolve := func(p string) string {
		if p == "" || filepath.IsAbs(p) {
			return p
		}
		return filepath.Join(baseDir, p)
	}
	for i := range ds.Backbones {
		ds.Backbones[i].Path = resolve(ds.Backbones[i].Path)
	}
	for i := range ds.TraitVocabularies {
		ds.TraitVocabularies[i].Path = resolve(ds.TraitVocabularies[i].Path)
	}
	for i := range ds.XrefSources {
		ds.XrefSources[i].Path = resolve(ds.XrefSources[i].Path)
	}
	for i := range ds.ConceptSources {
		ds.ConceptSources[i].Concepts = resolve(ds.ConceptSources[i].Concepts)
		ds.ConceptSources[i].Relations = resolve(ds.ConceptSources[i].Relations)
	}
	for i := range ds.NameSpaces {
		ds.NameSpaces[i].Path = resolve(ds.NameSpaces[i].Path)
	}
}

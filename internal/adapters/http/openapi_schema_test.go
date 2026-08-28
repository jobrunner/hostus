package httpx

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	yaml "go.yaml.in/yaml/v3"

	"github.com/jobrunner/hostus/internal/httperr"
)

// TestOpenAPISchemasMatchDTOs is the deterministic SCHEMA-CONTENT half of the
// OpenAPI contract (the routes<->spec half lives in openapi_contract_test.go).
// It reflects over every response/request DTO and asserts, recursively, that
// api/openapi/openapi.yaml's component schema for it agrees on: the set of JSON
// property names, which are required (Go `omitempty` <=> absent from the
// schema's `required`), the scalar type of each leaf (string/boolean/integer/
// number), array element types, map (additionalProperties) value types, and the
// $ref/inline-object structure of nested DTOs. So a field added, renamed, made
// (non-)optional, or retyped in a DTO without the matching spec edit fails CI —
// the hand-written spec can no longer drift from the wire shape in content, not
// just in path+method.
//
// Deliberately out of scope (documented in known-gaps): enum value lists,
// descriptions/examples, and format hints — this checks structure, not prose.
func TestOpenAPISchemasMatchDTOs(t *testing.T) {
	schemas := loadOpenAPISchemas(t)

	// Component schema name -> the Go DTO it documents. Every named schema must
	// appear here; a new schema without a mapping (or a mapping without a
	// schema) fails below, so this registry cannot silently fall behind.
	registry := map[string]reflect.Type{
		"BackboneRef":               reflect.TypeOf(backboneRefDTO{}),
		"Synonym":                   reflect.TypeOf(synonymDTO{}),
		"ClassificationEntry":       reflect.TypeOf(classificationDTO{}),
		"Distribution":              reflect.TypeOf(distributionDTO{}),
		"Concept":                   reflect.TypeOf(conceptDTO{}),
		"SynonymDetail":             reflect.TypeOf(synonymDetailDTO{}),
		"SynonymSummary":            reflect.TypeOf(synonymSummaryDTO{}),
		"SynonymsResponse":          reflect.TypeOf(synonymsResponseDTO{}),
		"TranslateRequest":          reflect.TypeOf(translateRequestDTO{}),
		"SecReference":              reflect.TypeOf(secReferenceDTO{}),
		"SecListResponse":           reflect.TypeOf(secListResponseDTO{}),
		"Area":                      reflect.TypeOf(areaDTO{}),
		"AreaListResponse":          reflect.TypeOf(areaListResponseDTO{}),
		"TranslateSource":           reflect.TypeOf(translateSourceDTO{}),
		"TranslateEntry":            reflect.TypeOf(translateEntryDTO{}),
		"RelationStatement":         reflect.TypeOf(relationStatementDTO{}),
		"TranslateCandidate":        reflect.TypeOf(translateCandidateDTO{}),
		"TranslateNameCandidate":    reflect.TypeOf(translateNameCandidateDTO{}),
		"TranslateResponse":         reflect.TypeOf(translateResponseDTO{}),
		"NameSpaceTranslation":      reflect.TypeOf(nameSpaceTranslationDTO{}),
		"ErrorResponse":             reflect.TypeOf(httperr.Response{}),
		"MatchNameRequest":          reflect.TypeOf(matchNameDTO{}),
		"MatchRequest":              reflect.TypeOf(matchRequestDTO{}),
		"MatchResult":               reflect.TypeOf(matchResultDTO{}),
		"MatchResponse":             reflect.TypeOf(matchResponseDTO{}),
		"SuggestItem":               reflect.TypeOf(suggestItemDTO{}),
		"SuggestResponse":           reflect.TypeOf(suggestResponseDTO{}),
		"Scale":                     reflect.TypeOf(scaleDTO{}),
		"TraitValue":                reflect.TypeOf(traitValueDTO{}),
		"TraitSet":                  reflect.TypeOf(traitSetDTO{}),
		"TraitsResponse":            reflect.TypeOf(traitsResponseDTO{}),
		"Backbone":                  reflect.TypeOf(backboneDTO{}),
		"BackboneListResponse":      reflect.TypeOf(backboneListResponseDTO{}),
		"Space":                     reflect.TypeOf(spaceDTO{}),
		"SpaceListResponse":         reflect.TypeOf(spaceListResponseDTO{}),
		"ClassificationInfo":        reflect.TypeOf(classificationInfoDTO{}),
		"VernacularNameEntry":       reflect.TypeOf(vernacularNameDTO{}),
		"AggregateMember":           reflect.TypeOf(aggregateMemberDTO{}),
		"AggregateMembership":       reflect.TypeOf(aggregateMembershipDTO{}),
		"AggregateResolution":       reflect.TypeOf(aggregateResolutionDTO{}),
		"AggregateResolutionOption": reflect.TypeOf(aggregateResolutionOptionDTO{}),
	}

	for name := range schemas {
		if _, ok := registry[name]; !ok {
			t.Errorf("component schema %q has no DTO in the test registry — map it (or it is undocumented drift)", name)
		}
	}
	for name, typ := range registry {
		s, ok := schemas[name]
		if !ok {
			t.Errorf("registry maps %q but api/openapi/openapi.yaml has no such component schema", name)
			continue
		}
		compareStructToSchema(t, name, typ, s, schemas)
	}
}

// jsonSchema is the subset of OpenAPI schema we compare against reflection.
type jsonSchema struct {
	Type                 string                 `yaml:"type"`
	Required             []string               `yaml:"required"`
	Properties           map[string]*jsonSchema `yaml:"properties"`
	Items                *jsonSchema            `yaml:"items"`
	AdditionalProperties *jsonSchema            `yaml:"additionalProperties"`
	Ref                  string                 `yaml:"$ref"`
	AllOf                []*jsonSchema          `yaml:"allOf"`
}

func loadOpenAPISchemas(t *testing.T) map[string]*jsonSchema {
	t.Helper()
	specPath := filepath.Join(findRepoRoot(t), "api", "openapi", "openapi.yaml")
	data, err := os.ReadFile(filepath.Clean(specPath))
	if err != nil {
		t.Fatalf("reading %s: %v", specPath, err)
	}
	var doc struct {
		Components struct {
			Schemas map[string]*jsonSchema `yaml:"schemas"`
		} `yaml:"components"`
	}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		t.Fatalf("parsing %s: %v", specPath, err)
	}
	if len(doc.Components.Schemas) == 0 {
		t.Fatalf("no component schemas parsed from %s", specPath)
	}
	return doc.Components.Schemas
}

// refName returns the component-schema name a property references, either
// directly ($ref) or via a single-element allOf (the idiom the spec uses to
// attach a description to a $ref), or "" if the property is not a reference.
func refName(s *jsonSchema) string {
	ref := s.Ref
	if ref == "" && len(s.AllOf) == 1 {
		ref = s.AllOf[0].Ref
	}
	if ref == "" {
		return ""
	}
	return ref[strings.LastIndex(ref, "/")+1:]
}

// compareStructToSchema asserts goType (a struct, or pointer to one) has
// exactly the JSON properties the object schema s declares, with matching
// required-ness, recursing into every property.
func compareStructToSchema(t *testing.T, path string, goType reflect.Type, s *jsonSchema, schemas map[string]*jsonSchema) {
	t.Helper()
	goType = deref(goType)
	if goType.Kind() != reflect.Struct {
		t.Errorf("%s: OpenAPI says object but the Go type is %s", path, goType.Kind())
		return
	}
	// An object schema must actually declare type: object (or leave it implicit
	// via properties). A schema mistyped as array/string while carrying
	// properties would otherwise be compared as a struct and pass.
	if s.Type != "" && s.Type != "object" {
		t.Errorf("%s: mapped to a struct but schema declares type %q, not object", path, s.Type)
		return
	}

	goFields, goRequired := jsonFields(goType)
	specProps := map[string]bool{}
	for name := range s.Properties {
		specProps[name] = true
	}

	// Property name sets must match exactly, both directions.
	for name := range goFields {
		if !specProps[name] {
			t.Errorf("%s: DTO field %q is not documented in the schema", path, name)
		}
	}
	for name := range specProps {
		if _, ok := goFields[name]; !ok {
			t.Errorf("%s: schema documents property %q that the DTO does not have", path, name)
		}
	}

	// Required sets must match exactly.
	specRequired := map[string]bool{}
	for _, r := range s.Required {
		specRequired[r] = true
	}
	if diff := setDiff(goRequired, specRequired); diff != "" {
		t.Errorf("%s: required mismatch — %s (Go omitempty <=> schema `required`)", path, diff)
	}

	for name, field := range goFields {
		prop := s.Properties[name]
		if prop == nil {
			continue // already reported as missing above
		}
		comparePropToType(t, path+"."+name, field.Type, prop, schemas)
	}
}

// comparePropToType compares one property schema against one Go field type,
// recursing through refs, arrays, maps and inline objects down to scalars.
func comparePropToType(t *testing.T, path string, goType reflect.Type, prop *jsonSchema, schemas map[string]*jsonSchema) {
	t.Helper()
	goType = deref(goType)

	if ref := refName(prop); ref != "" {
		target, ok := schemas[ref]
		if !ok {
			t.Errorf("%s: property $refs %q which is not a component schema", path, ref)
			return
		}
		compareStructToSchema(t, path+"->"+ref, goType, target, schemas)
		return
	}

	switch prop.Type {
	case "array":
		if goType.Kind() != reflect.Slice {
			t.Errorf("%s: schema is array but Go type is %s", path, goType.Kind())
			return
		}
		if prop.Items == nil {
			t.Errorf("%s: array schema has no items", path)
			return
		}
		comparePropToType(t, path+"[]", goType.Elem(), prop.Items, schemas)
	case "object":
		switch {
		case prop.AdditionalProperties != nil: // a map
			if goType.Kind() != reflect.Map {
				t.Errorf("%s: schema is object+additionalProperties (map) but Go type is %s", path, goType.Kind())
				return
			}
			comparePropToType(t, path+"{}", goType.Elem(), prop.AdditionalProperties, schemas)
		case prop.Properties != nil: // an inline nested object
			compareStructToSchema(t, path, goType, prop, schemas)
		default:
			t.Errorf("%s: object schema is neither a map nor has properties", path)
		}
	default: // scalar
		want := goScalarType(goType.Kind())
		if want == "" {
			t.Errorf("%s: Go type %s has no scalar OpenAPI mapping but schema type is %q", path, goType.Kind(), prop.Type)
			return
		}
		if want != prop.Type {
			t.Errorf("%s: type mismatch — Go %s maps to %q but schema says %q", path, goType.Kind(), want, prop.Type)
		}
	}
}

// jsonFields returns the DTO's JSON property name -> field, plus the set of
// names that are REQUIRED (no `omitempty`). Unexported fields and json:"-" are
// skipped.
func jsonFields(t reflect.Type) (map[string]reflect.StructField, map[string]bool) {
	fields := map[string]reflect.StructField{}
	required := map[string]bool{}
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if f.PkgPath != "" { // unexported
			continue
		}
		tag := f.Tag.Get("json")
		if tag == "-" {
			continue
		}
		parts := strings.Split(tag, ",")
		name := parts[0]
		if name == "" {
			name = f.Name
		}
		fields[name] = f
		omitempty := false
		for _, p := range parts[1:] {
			if p == "omitempty" {
				omitempty = true
			}
		}
		if !omitempty {
			required[name] = true
		}
	}
	return fields, required
}

func goScalarType(k reflect.Kind) string {
	// An if-chain, not a switch: a tagged `switch k` trips the exhaustive
	// linter (it would demand every reflect.Kind), and a tagless switch trips
	// staticcheck QF1002 (wants it tagged). The if-chain satisfies both for a
	// four-category mapping.
	if k == reflect.String {
		return "string"
	}
	if k == reflect.Bool {
		return "boolean"
	}
	if k >= reflect.Int && k <= reflect.Uint64 {
		return "integer" // reflect.Int..reflect.Uint64 are contiguous integer kinds
	}
	if k == reflect.Float32 || k == reflect.Float64 {
		return "number"
	}
	return ""
}

func deref(t reflect.Type) reflect.Type {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	return t
}

// setDiff renders the symmetric difference of two name sets, or "" if equal.
func setDiff(a, b map[string]bool) string {
	var onlyA, onlyB []string
	for k := range a {
		if !b[k] {
			onlyA = append(onlyA, k)
		}
	}
	for k := range b {
		if !a[k] {
			onlyB = append(onlyB, k)
		}
	}
	if len(onlyA) == 0 && len(onlyB) == 0 {
		return ""
	}
	sort.Strings(onlyA)
	sort.Strings(onlyB)
	return "required only in DTO: " + strings.Join(onlyA, ",") + " | required only in schema: " + strings.Join(onlyB, ",")
}

func findRepoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found walking up from the test directory")
		}
		dir = parent
	}
}

package taxonomy

import (
	"testing"

	"github.com/jobrunner/hostus/internal/gbif"
)

func TestMapAndGroup_EmptyResults(t *testing.T) {
	result := MapAndGroup([]gbif.Taxon{})
	if len(result) != 0 {
		t.Errorf("expected empty result, got %d items", len(result))
	}
}

func TestMapAndGroup_SingleAccepted(t *testing.T) {
	taxa := []gbif.Taxon{
		{
			Key:             123,
			ScientificName:  "Quercus robur L.",
			CanonicalName:   "Quercus robur",
			Rank:            "SPECIES",
			Family:          "Fagaceae",
			TaxonomicStatus: "ACCEPTED",
		},
	}

	result := MapAndGroup(taxa)

	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}

	if result[0].AcceptedKey != 123 {
		t.Errorf("expected AcceptedKey 123, got %d", result[0].AcceptedKey)
	}
	if result[0].AcceptedName != "Quercus robur" {
		t.Errorf("expected AcceptedName 'Quercus robur', got %s", result[0].AcceptedName)
	}
	if result[0].Family != "Fagaceae" {
		t.Errorf("expected Family 'Fagaceae', got %s", result[0].Family)
	}
	if len(result[0].Synonyms) != 0 {
		t.Errorf("expected no synonyms, got %d", len(result[0].Synonyms))
	}
}

func TestMapAndGroup_WithSynonyms(t *testing.T) {
	taxa := []gbif.Taxon{
		{
			Key:             100,
			ScientificName:  "Schoenoplectus lacustris",
			CanonicalName:   "Schoenoplectus lacustris",
			Rank:            "SPECIES",
			Family:          "Cyperaceae",
			TaxonomicStatus: "ACCEPTED",
		},
		{
			Key:             200,
			ScientificName:  "Scirpus lacustris",
			CanonicalName:   "Scirpus lacustris",
			Rank:            "SPECIES",
			Family:          "Cyperaceae",
			TaxonomicStatus: "SYNONYM",
			AcceptedKey:     100,
			Accepted:        "Schoenoplectus lacustris",
		},
	}

	result := MapAndGroup(taxa)

	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}

	if result[0].AcceptedKey != 100 {
		t.Errorf("expected AcceptedKey 100, got %d", result[0].AcceptedKey)
	}
	if len(result[0].Synonyms) != 1 {
		t.Fatalf("expected 1 synonym, got %d", len(result[0].Synonyms))
	}
	if result[0].Synonyms[0].Key != 200 {
		t.Errorf("expected synonym key 200, got %d", result[0].Synonyms[0].Key)
	}
	if result[0].Synonyms[0].Name != "Scirpus lacustris" {
		t.Errorf("expected synonym name 'Scirpus lacustris', got %s", result[0].Synonyms[0].Name)
	}
}

func TestMapAndGroup_SynonymWithoutAccepted(t *testing.T) {
	taxa := []gbif.Taxon{
		{
			Key:             200,
			ScientificName:  "Scirpus lacustris",
			CanonicalName:   "Scirpus lacustris",
			Rank:            "SPECIES",
			Family:          "Cyperaceae",
			TaxonomicStatus: "SYNONYM",
			AcceptedKey:     100,
			Accepted:        "Schoenoplectus lacustris",
		},
	}

	result := MapAndGroup(taxa)

	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}

	if result[0].AcceptedKey != 100 {
		t.Errorf("expected AcceptedKey 100, got %d", result[0].AcceptedKey)
	}
	if result[0].AcceptedName != "Schoenoplectus lacustris" {
		t.Errorf("expected AcceptedName from synonym's Accepted field, got %s", result[0].AcceptedName)
	}
}

func TestMapAndGroup_MultipleSynonymTypes(t *testing.T) {
	taxa := []gbif.Taxon{
		{
			Key:             100,
			CanonicalName:   "Accepted Taxon",
			TaxonomicStatus: "ACCEPTED",
		},
		{
			Key:             201,
			CanonicalName:   "Synonym 1",
			TaxonomicStatus: "SYNONYM",
			AcceptedKey:     100,
		},
		{
			Key:             202,
			CanonicalName:   "Synonym 2",
			TaxonomicStatus: "HETEROTYPIC_SYNONYM",
			AcceptedKey:     100,
		},
		{
			Key:             203,
			CanonicalName:   "Synonym 3",
			TaxonomicStatus: "HOMOTYPIC_SYNONYM",
			AcceptedKey:     100,
		},
	}

	result := MapAndGroup(taxa)

	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}

	if len(result[0].Synonyms) != 3 {
		t.Errorf("expected 3 synonyms, got %d", len(result[0].Synonyms))
	}
}

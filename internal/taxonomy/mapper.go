package taxonomy

import (
	"github.com/jobrunner/hostus/internal/gbif"
)

func MapAndGroup(results []gbif.Taxon) []TaxonSuggestion {
	if len(results) == 0 {
		return []TaxonSuggestion{}
	}

	acceptedTaxa := make(map[int]*gbif.Taxon)
	synonymsByAccepted := make(map[int][]Synonym)

	collectTaxa(results, acceptedTaxa, synonymsByAccepted)
	createPlaceholders(results, acceptedTaxa)

	return buildSuggestions(acceptedTaxa, synonymsByAccepted)
}

func collectTaxa(results []gbif.Taxon, accepted map[int]*gbif.Taxon, synonyms map[int][]Synonym) {
	for i := range results {
		t := &results[i]
		acceptedKey := t.GetAcceptedKey()

		if t.IsAccepted() {
			addAccepted(accepted, acceptedKey, t)
		} else if t.IsSynonym() {
			addSynonym(synonyms, acceptedKey, t)
		}
	}
}

func addAccepted(accepted map[int]*gbif.Taxon, key int, t *gbif.Taxon) {
	if _, exists := accepted[key]; !exists {
		accepted[key] = t
	}
}

func addSynonym(synonyms map[int][]Synonym, acceptedKey int, t *gbif.Taxon) {
	name := t.CanonicalName
	if name == "" {
		name = t.ScientificName
	}
	syn := Synonym{
		Key:    t.Key,
		Name:   name,
		Status: t.TaxonomicStatus,
	}
	synonyms[acceptedKey] = append(synonyms[acceptedKey], syn)
}

func createPlaceholders(results []gbif.Taxon, accepted map[int]*gbif.Taxon) {
	for i := range results {
		t := &results[i]
		if t.IsSynonym() && t.AcceptedKey > 0 {
			if _, exists := accepted[t.AcceptedKey]; !exists {
				accepted[t.AcceptedKey] = createPlaceholderTaxon(t)
			}
		}
	}
}

func createPlaceholderTaxon(t *gbif.Taxon) *gbif.Taxon {
	return &gbif.Taxon{
		Key:             t.AcceptedKey,
		ScientificName:  t.Accepted,
		CanonicalName:   t.Accepted,
		Rank:            t.Rank,
		Family:          t.Family,
		TaxonomicStatus: "ACCEPTED",
	}
}

func buildSuggestions(accepted map[int]*gbif.Taxon, synonyms map[int][]Synonym) []TaxonSuggestion {
	suggestions := make([]TaxonSuggestion, 0, len(accepted))
	for key, taxon := range accepted {
		suggestions = append(suggestions, createSuggestion(key, taxon, synonyms[key]))
	}
	return suggestions
}

func createSuggestion(key int, taxon *gbif.Taxon, synonyms []Synonym) TaxonSuggestion {
	name := taxon.CanonicalName
	if name == "" {
		name = taxon.ScientificName
	}
	return TaxonSuggestion{
		AcceptedKey:  key,
		AcceptedName: name,
		Rank:         taxon.Rank,
		Family:       taxon.Family,
		Synonyms:     synonyms,
	}
}

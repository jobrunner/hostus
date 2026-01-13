package taxonomy

import (
	"github.com/jobrunner/hostus/internal/gbif"
)

func MapAndGroup(results []gbif.Taxon) []TaxonSuggestion {
	if len(results) == 0 {
		return []TaxonSuggestion{}
	}

	// Group by accepted key
	acceptedTaxa := make(map[int]*gbif.Taxon)
	synonymsByAccepted := make(map[int][]Synonym)

	for i := range results {
		t := &results[i]
		acceptedKey := t.GetAcceptedKey()

		if t.IsAccepted() {
			if _, exists := acceptedTaxa[acceptedKey]; !exists {
				acceptedTaxa[acceptedKey] = t
			}
		} else if t.IsSynonym() {
			syn := Synonym{
				Key:    t.Key,
				Name:   t.CanonicalName,
				Status: t.TaxonomicStatus,
			}
			if syn.Name == "" {
				syn.Name = t.ScientificName
			}
			synonymsByAccepted[acceptedKey] = append(synonymsByAccepted[acceptedKey], syn)
		}
	}

	// For synonyms without accepted taxon in results, we may need to create a placeholder
	for i := range results {
		t := &results[i]
		if t.IsSynonym() && t.AcceptedKey > 0 {
			if _, exists := acceptedTaxa[t.AcceptedKey]; !exists {
				// Create placeholder from synonym's accepted info
				acceptedTaxa[t.AcceptedKey] = &gbif.Taxon{
					Key:             t.AcceptedKey,
					ScientificName:  t.Accepted,
					CanonicalName:   t.Accepted,
					Rank:            t.Rank,
					Family:          t.Family,
					TaxonomicStatus: "ACCEPTED",
				}
			}
		}
	}

	// Build result list
	suggestions := make([]TaxonSuggestion, 0, len(acceptedTaxa))
	for key, taxon := range acceptedTaxa {
		name := taxon.CanonicalName
		if name == "" {
			name = taxon.ScientificName
		}

		suggestion := TaxonSuggestion{
			AcceptedKey:  key,
			AcceptedName: name,
			Rank:         taxon.Rank,
			Family:       taxon.Family,
			Synonyms:     synonymsByAccepted[key],
		}
		suggestions = append(suggestions, suggestion)
	}

	return suggestions
}

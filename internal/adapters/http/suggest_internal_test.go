package httpx

import (
	"testing"

	"github.com/jobrunner/hostus/internal/application"
	"github.com/jobrunner/hostus/internal/domain"
)

// TestSuggestResponseToDTO_CarriesAggregate pins that the aggregate flag from
// domain.SuggestItem reaches the wire DTO, so the console can badge an
// aggregate hit.
func TestSuggestResponseToDTO_CarriesAggregate(t *testing.T) {
	resp := application.SuggestResponse{
		Results: []domain.SuggestItem{
			{ConceptID: "c1", Canonical: "Achillea millefolium", Rank: domain.RankSpecies, Status: domain.StatusAccepted, Aggregate: true},
			{ConceptID: "c2", Canonical: "Bellis perennis", Rank: domain.RankSpecies, Status: domain.StatusAccepted, Aggregate: false},
		},
	}
	dto := suggestResponseToDTO(resp)
	if len(dto.Results) != 2 {
		t.Fatalf("got %d results, want 2", len(dto.Results))
	}
	if !dto.Results[0].Aggregate {
		t.Error("Results[0].Aggregate = false, want true")
	}
	if dto.Results[1].Aggregate {
		t.Error("Results[1].Aggregate = true, want false")
	}
}

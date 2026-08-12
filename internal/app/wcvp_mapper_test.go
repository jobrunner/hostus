package app

import (
	"testing"

	"github.com/jobrunner/hostus/internal/adapters/wcvp"
	"github.com/jobrunner/hostus/internal/application"
)

// TestWCVPRowSource_Taxa_MapsEverySourceColumn guards the production
// wcvp.TaxonRow -> application.TaxonRow mapper (wcvpRowSource.Taxa) against
// the SILENT-LOSS class documented in docs/explanation/known-gaps.md: a
// source column that is read but then dropped by a mapping line, leaving the
// target column empty with nobody complaining. The whole-struct equality is
// deliberate — if any single mapping line is deleted, its field falls back to
// the zero value and this comparison fails, naming exactly which column was
// lost. Every application.TaxonRow field the mapper is responsible for is set
// to a distinct sentinel; POWOID and Accepted are derived (via
// TaxonRow.POWOID / IsAccepted), so they are exercised through their real
// derivation, not a raw copy.
func TestWCVPRowSource_Taxa_MapsEverySourceColumn(t *testing.T) {
	ds := &wcvp.Dataset{
		Taxa: []wcvp.TaxonRow{
			// A synonym row (AcceptedNameUsageID != TaxonID) so AcceptedTaxonID
			// carries its own distinct sentinel and Accepted is observably
			// false through the mapper.
			{
				TaxonID:             "taxon-id",
				AcceptedNameUsageID: "accepted-id",
				Canonical:           "canonical-name",
				Authorship:          "authorship-val",
				Rank:                "rank-val",
				Status:              "status-val",
				ParentNameUsageID:   "parent-id",
				OriginalNameUsageID: "basionym-id",
				PublishedIn:         "published-in-val",
				NomenclaturalStatus: "nom-status-val",
				DynamicProperties:   `{"powoid":"powoid-val"}`,
			},
			// A self-referential accepted row, so the Accepted=true branch of
			// the derivation is mapped too.
			{TaxonID: "acc", AcceptedNameUsageID: "acc"},
		},
	}

	got := wcvpRowSource{ds: ds}.Taxa()
	if len(got) != 2 {
		t.Fatalf("Taxa() returned %d rows, want 2", len(got))
	}

	want := application.TaxonRow{
		TaxonID:         "taxon-id",
		AcceptedTaxonID: "accepted-id",
		Accepted:        false,
		Canonical:       "canonical-name",
		Authorship:      "authorship-val",
		Rank:            "rank-val",
		Status:          "status-val",
		POWOID:          "powoid-val",
		ParentTaxonID:   "parent-id",
		BasionymTaxonID: "basionym-id",
		PublishedIn:     "published-in-val",
		NomStatus:       "nom-status-val",
	}
	if got[0] != want {
		t.Errorf("Taxa()[0] = %+v\nwant %+v\n(a mismatched/empty field means that source column was dropped by the mapper)", got[0], want)
	}

	if !got[1].Accepted {
		t.Errorf("Taxa()[1].Accepted = false, want true (AcceptedNameUsageID == TaxonID must map to Accepted via IsAccepted)")
	}
}

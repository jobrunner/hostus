package gbif

type SearchResponse struct {
	Offset  int     `json:"offset"`
	Limit   int     `json:"limit"`
	Count   int     `json:"count"`
	Results []Taxon `json:"results"`
}

type Taxon struct {
	Key             int    `json:"key"`
	NubKey          int    `json:"nubKey,omitempty"`
	NameKey         int    `json:"nameKey,omitempty"`
	TaxonID         string `json:"taxonID,omitempty"`
	Kingdom         string `json:"kingdom,omitempty"`
	Phylum          string `json:"phylum,omitempty"`
	Class           string `json:"class,omitempty"`
	Order           string `json:"order,omitempty"`
	Family          string `json:"family,omitempty"`
	Genus           string `json:"genus,omitempty"`
	Species         string `json:"species,omitempty"`
	KingdomKey      int    `json:"kingdomKey,omitempty"`
	PhylumKey       int    `json:"phylumKey,omitempty"`
	ClassKey        int    `json:"classKey,omitempty"`
	OrderKey        int    `json:"orderKey,omitempty"`
	FamilyKey       int    `json:"familyKey,omitempty"`
	GenusKey        int    `json:"genusKey,omitempty"`
	SpeciesKey      int    `json:"speciesKey,omitempty"`
	ScientificName  string `json:"scientificName"`
	CanonicalName   string `json:"canonicalName,omitempty"`
	Authorship      string `json:"authorship,omitempty"`
	AcceptedKey     int    `json:"acceptedKey,omitempty"`
	Accepted        string `json:"accepted,omitempty"`
	Rank            string `json:"rank"`
	TaxonomicStatus string `json:"taxonomicStatus"`
	Origin          string `json:"origin,omitempty"`
	NumDescendants  int    `json:"numDescendants,omitempty"`
	Parent          string `json:"parent,omitempty"`
	ParentKey       int    `json:"parentKey,omitempty"`
}

func (t *Taxon) IsAccepted() bool {
	return t.TaxonomicStatus == "ACCEPTED"
}

func (t *Taxon) IsSynonym() bool {
	return t.TaxonomicStatus == "SYNONYM" ||
		t.TaxonomicStatus == "HETEROTYPIC_SYNONYM" ||
		t.TaxonomicStatus == "HOMOTYPIC_SYNONYM" ||
		t.TaxonomicStatus == "PROPARTE_SYNONYM"
}

func (t *Taxon) GetAcceptedKey() int {
	if t.IsAccepted() {
		return t.Key
	}
	return t.AcceptedKey
}

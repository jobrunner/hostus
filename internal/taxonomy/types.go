package taxonomy

type TaxonSuggestion struct {
	AcceptedKey  int       `json:"acceptedKey"`
	AcceptedName string    `json:"acceptedName"`
	Rank         string    `json:"rank"`
	Family       string    `json:"family"`
	Synonyms     []Synonym `json:"synonyms,omitempty"`
}

type Synonym struct {
	Key    int    `json:"key"`
	Name   string `json:"name"`
	Status string `json:"status"`
}

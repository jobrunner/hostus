package domain

// SecReference is the bibliographic identity of a taxonomic reference space
// — the "sec." (secundum) part of "Abies alba Mill. sec. Wisskirchen &
// Haeupler 1998". It is what makes SP5 different from every milestone before
// it: SP1–SP4 treated a concept as one global truth keyed by name, whereas a
// sec. reference says WHOSE circumscription is meant, and two concepts that
// share a name but differ in their SecReference are deliberately distinct
// rows, never merged.
//
// ID is the stable identity (for CDM: the citation UUID) and is what
// taxon_concept.sec_reference stores; Title is the human-readable citation,
// kept in the sec_reference lookup table so a response can name the flora
// rather than a UUID.
type SecReference struct {
	ID    string
	Title string
}

// IsZero reports whether s carries no reference-space identity. A Title
// without an ID counts as zero: the id is what concepts key on, so a bare
// title cannot scope a circumscription.
func (s SecReference) IsZero() bool {
	return s.ID == ""
}

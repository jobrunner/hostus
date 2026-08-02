package domain

// XrefSourceMeta is the provenance record for one ingested cross-reference
// source (e.g. the Wikidata bridge-hub CSV): license/source metadata plus
// the redistribution gate, mirroring TraitVocabMeta's shape for the same
// purpose. Unlike a backbone or trait vocabulary, an xref source has no
// version-joined table of its own — xref rows carry no provenance column —
// so this exists purely for the ingest report and the
// "hostus ingest" redistribution notice.
type XrefSourceMeta struct {
	ID        string
	Version   string
	License   string
	SourceURL string
	// Redistribution gates whether this source's xrefs may be copied into
	// an exported bundle (see ExportBundle in internal/adapters/sqlite). It
	// never gates local ingest or reads.
	Redistribution Redistribution
}

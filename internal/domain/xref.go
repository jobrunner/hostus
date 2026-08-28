package domain

// XrefSourceMeta is the provenance record for one ingested cross-reference
// source (e.g. the Wikidata bridge-hub CSV): license/source metadata plus
// the redistribution gate, mirroring NameSpaceMeta's shape for the same
// purpose. It is persisted as one xref_source row per ingested source, and
// every xref row that source writes carries its id in xref.source — so an
// ingested database can answer both "which harvest are these xrefs from?"
// (Version + ManifestSHA) and "may they be redistributed?".
type XrefSourceMeta struct {
	ID        string
	Version   string
	License   string
	SourceURL string
	// ManifestSHA is the checksum of the validated manifest this source was
	// pinned by, recorded verbatim into xref_source.manifest_sha — the same
	// binding backbone_version.manifest_sha provides for a backbone.
	ManifestSHA string
	// Redistribution gates whether this source's xrefs may be copied into
	// an exported bundle: ExportBundle (internal/adapters/sqlite) joins
	// xref_source against the xref rows in scope and refuses the export
	// unless every contributing source is RedistributionAllowed (see
	// findRestrictedSources). It never gates local ingest or reads.
	Redistribution Redistribution
}

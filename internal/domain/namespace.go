package domain

// Name spaces (SP9, UC4).
//
// A NAME SPACE is a checklist that contributes NAMES but no taxonomy: it has
// no synonymy graph, no parent chain and no external ids hostus could join
// on — only a flat list of the spellings that space uses, each with the
// space's own stable id. FloraVeg.EU's list (the namespace the ESy expert
// system's rules are written against) is the first one; the same canonical
// CSV contract covers GermanSL, EuroSL and Euro+Med (pipelines/README.md,
// "Canonical CSV contract (name lists)").
//
// A name space is therefore deliberately NOT a backbone and NOT a trait
// vocabulary:
//
//   - It must not land in backbone_version. Those rows are served verbatim
//     as the backbone_versions provenance block of every /v1/suggest and
//     /v1/match response and they gate /health/ready — a name list that
//     contributes zero concepts would tell clients it is a backbone.
//   - It contributes no taxon_concept rows at all. Its entries ATTACH to
//     concepts an existing backbone already holds, via the SP3 name
//     crosswalk (NameCandidates/Canonicalize), which is lossy by
//     construction and whose loss is reported rather than absorbed.
//
// What it buys is the thing UC4 is missing: given a concept, the spelling
// the target space uses for it (an ESy-compatible name), and whether that
// spelling denotes an AGGREGATE rather than a single taxon.

// NameSpaceMeta is one ingested name space's provenance row — the name-space
// counterpart of BackboneVersion/TraitVocabMeta/XrefSourceMeta. IngestedAt is
// stamped by the repository adapter, not by the caller, exactly as it is for
// those three.
type NameSpaceMeta struct {
	// ID is the manifest-pinned space id, e.g. "floraveg". It is also the
	// value NameSpaceEntry.Space carries and the one a /v1/match
	// target_space parameter will name.
	ID string
	// Version pins the harvest/edition, never "latest" (e.g. FloraVeg's
	// sheet date "2023-01-03").
	Version   string
	License   string
	SourceURL string
	// ManifestSHA binds this ingest to the exact manifest revision that was
	// validated, like BackboneVersion.ManifestSHA.
	ManifestSHA string
	// Redistribution gates ExportBundle, never local ingest. FloraVeg's is
	// "unknown" — no license statement is findable (pipelines/README.md).
	Redistribution Redistribution
}

// NameSpaceEntry is one name-space spelling attached to a hostus concept:
// the space's own id and name string, whether that name denotes an
// aggregate, and how the crosswalk reached the concept.
//
// One concept can legitimately carry SEVERAL entries from the same space.
// FloraVeg spells Festuca ovina three ways under three SeqIDs — "Festuca
// ovina" (5647), "Festuca ovina aggr." (5648) and "Festuca ovina s. l."
// (5649) — and all three crosswalk onto the same WCVP concept, because WCVP
// carries no aggregate-marked names at all (see
// NormalizationRule.Flagged). Collapsing them would throw away exactly the
// distinction UC4's aggregate_policy has to make, so the entry is keyed by
// (Space, ExtID) and every spelling is kept.
type NameSpaceEntry struct {
	Space string
	ExtID string
	// Name is the space's spelling, stored VERBATIM — this is the string a
	// caller asking for a target space gets back, so it must not be folded
	// to the canonical match key.
	Name string
	// Aggregate reports whether Name denotes a collective species
	// ("Sammelart") rather than a single taxon — see IsAggregateName.
	Aggregate bool
	// Resolution records HOW the crosswalk reached the concept: empty for an
	// exact canonical match, else the NormalizationRule that was needed.
	// Same "absence is information" rule as TraitValue.Resolution: empty
	// means "no normalisation was needed", never "unknown".
	Resolution string
}

// IsAggregateName reports whether a verbatim name denotes an AGGREGATE — a
// collective species wider than a single taxon ("Festuca ovina aggr.",
// "Festuca ovina s. l.").
//
// It is deliberately a thin predicate over AggregateBases rather than its own
// marker list: AggregateBases already owns the measured set of trailing
// markers (and the reasoning for what is excluded from it — "s. str."
// NARROWS and is not an aggregate marker), and a second, independently
// maintained list would be exactly the duplicated-mapper defect class this
// milestone has already hit twice. A name carries an aggregate marker
// precisely when peeling markers off it yields at least one base.
func IsAggregateName(name string) bool {
	return len(AggregateBases(Canonicalize(name))) > 0
}

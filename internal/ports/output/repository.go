package output

import (
	"context"

	"github.com/jobrunner/hostus/internal/domain"
)

// Repository is the driven port through which the application reaches the
// local taxonomy index. Read methods surface domain.ErrNotFound (wrapped)
// for missing entities; ingest methods are exposed via BeginIngest/IngestTx
// so an adapter can batch a whole backbone import into one transaction.
type Repository interface {
	// Concept resolves a taxon_concept by id, returning its accepted
	// concept, its synonym names (each carrying its own homotypic linkage,
	// see SynonymName), its cross-references, and its distribution. Returns
	// domain.ErrNotFound (wrapped) if id is unknown.
	Concept(ctx context.Context, id string) (*domain.Concept, []SynonymName, []domain.Xref, []domain.Distribution, error)
	// SynonymCandidates returns conceptID's synonyms in the shape the UC5
	// relevance model consumes (domain.SynonymCandidate): the name itself
	// plus the three fields the decision needs and Concept()'s SynonymName
	// does not carry — the verbatim `nom_status` cell, the tri-state
	// homotypic flag, and IsBasionym.
	//
	// It is a SEPARATE method rather than a widening of Concept(): every
	// /v1/concept/{id} response would otherwise pay for the extra join, and
	// SynonymName's wire-visible shape is deliberately frozen.
	//
	// IsBasionym is the REPOSITORY's job (domain.SynonymCandidate's doc
	// comment): it is true when this synonym's name id equals the concept's
	// ACCEPTED name's basionym_id — i.e. the accepted name is a
	// recombination of this synonym. 429.172 names carry a basionym_id and
	// 113.642 synonym rows satisfy that equality in the measured index. An
	// implementation that left the flag false everywhere would silently turn
	// UC5 rule 4 (basionym leads its typification block) into a no-op that
	// nothing in the domain package can detect, so it is pinned by an
	// adapter test.
	//
	// Ordering is by name id, matching conceptSynonyms; ranking is the
	// application layer's job (domain.RankSynonyms). Returns
	// domain.ErrNotFound (wrapped) if conceptID is unknown; a known concept
	// with no synonyms returns an empty, non-error slice — callers must not
	// conflate the two.
	SynonymCandidates(ctx context.Context, conceptID string) ([]domain.SynonymCandidate, error)
	// Classification walks conceptID's taxon_concept.parent_id chain
	// upward, bounded to a small fixed depth (see the sqlite adapter's
	// maxClassificationDepth) so a cyclic or corrupt parent_id chain can
	// never hang. Returns the ancestor chain ROOT-FIRST — index 0 is the
	// topmost ancestor reached, and the last element is conceptID's
	// immediate parent; conceptID itself is never included. A concept with
	// no parent_id (or one whose chain terminates within the depth bound)
	// returns an empty, non-error slice. Returns domain.ErrNotFound
	// (wrapped) if conceptID is unknown.
	Classification(ctx context.Context, conceptID string) ([]domain.ClassificationEntry, error)
	// ConceptByXref resolves a taxon_concept via a cross-reference to an
	// external authority (e.g. authority="powo", extID="396681-1").
	ConceptByXref(ctx context.Context, authority, extID string) (*domain.Concept, error)
	// ConceptIDsByXref batch-resolves extIDs against xref for a single
	// authority, returning only the ones that matched: map[extID]conceptID.
	// An extID with no xref row for authority is simply absent from the
	// result — this is the ID-based join application.IngestXrefs' resolve
	// phase uses, sized for resolving hundreds of thousands of ids in one
	// call (see the sqlite adapter's doc comment on the json_each binding).
	ConceptIDsByXref(ctx context.Context, authority string, extIDs []string) (map[string]string, error)
	// ExistingConceptIDs reports which of ids already have a taxon_concept
	// row, as a set: an id with no row is simply absent from the result.
	// This is the pre-transaction read application.IngestCDM's phase 1 uses
	// to decide whether a concept_relation's two ends can be written at all
	// (concept_relation FKs BOTH ends to taxon_concept), sized to take the
	// whole id list in one call like ConceptIDsByXref.
	ExistingConceptIDs(ctx context.Context, ids []string) (map[string]bool, error)
	// SecReferences lists every ingested sec. reference space (the
	// bibliographic identity of a circumscription's reference frame),
	// ordered by id.
	SecReferences(ctx context.Context) ([]domain.SecReference, error)
	// Areas lists every distribution area that actually carries data (a
	// DISTINCT area_scheme/area_code from the distribution table), each with
	// its human-readable name where one was ingested (empty otherwise),
	// ordered by (scheme, code). Backs GET /v1/areas.
	Areas(ctx context.Context) ([]domain.Area, error)
	// SecReferenceByID resolves one sec. reference space by its id.
	// Returns domain.ErrNotFound (wrapped) if the id is unknown — which is
	// what lets /v1/translate tell a MISTYPED target space (404) apart from
	// a real space that simply has no relation into it (an explicit empty
	// answer). Conflating the two is the failure mode UC6 exists to avoid.
	SecReferenceByID(ctx context.Context, id string) (domain.SecReference, error)
	// ConceptRelationsInSec returns conceptID's own concept row together
	// with every stored concept_relation row that touches it — in EITHER
	// stored direction — whose OTHER end is a concept in the sec. reference
	// space targetSec. It is exactly one hop: no chaining, since a
	// transitive chain across relation types is not sound in general
	// (congruent∘includes is defensible, overlaps∘overlaps is not), and the
	// boundary is enforced here rather than left to the caller's
	// discipline.
	//
	// Rows are returned in the direction the source states them (hostus
	// never materializes the mirror row, see IngestTx.AddConceptRelation);
	// ConceptRelationEdge.Outgoing says which end conceptID was on, so the
	// application layer can decide whether to name domain.Relation.Inverse
	// and can report honestly when no inverse exists. A relation from
	// conceptID to itself is not a translation and is excluded.
	//
	// Returns domain.ErrNotFound (wrapped) if conceptID is unknown; a known
	// concept with no relation into targetSec returns a populated Source
	// and an empty Edges slice — callers must not conflate the two.
	ConceptRelationsInSec(ctx context.Context, conceptID, targetSec string) (ConceptRelations, error)
	// MatchExact returns every name (accepted or synonym) whose canonical
	// form equals canon, leaving classification (exact vs. exact_author,
	// etc.) to the application layer.
	MatchExact(ctx context.Context, canon string) ([]MatchCandidate, error)
	// MatchFuzzyCandidates returns up to limit names that are CHEAP TO FIND
	// near-misses of canon (an already domain.Canonicalize'd query) for the
	// application layer to score with domain.Similarity — it does not
	// itself compute or filter by similarity. Implementations must use a
	// prefilter (e.g. same first letter + a bounded canonical-length
	// window) so a fuzzy lookup never scans the whole name table; see the
	// sqlite adapter's doc comment for the concrete prefilter and its
	// recall trade-off (a real near-miss whose length or first letter
	// diverges enough from canon can be missed — deliberately, to keep this
	// cheap). limit <= 0 uses the adapter's default cap.
	//
	// backbone/sec (either or both "" for no restriction) narrow the prefilter
	// to one backbone / sec. reference space BEFORE the limit is applied, so a
	// SP5 resolution filter (MatchFilter) does not lose the target space's
	// genuine near-miss to the limit when many out-of-space same-length names
	// crowd the top-N. Applying the filter only after this call would truncate
	// the wanted candidate away in the very multi-sec case the filter serves.
	MatchFuzzyCandidates(ctx context.Context, canon string, limit int, backbone, sec string) ([]MatchCandidate, error)
	// BackboneVersions lists every ingested backbone artifact.
	BackboneVersions(ctx context.Context) ([]domain.BackboneVersion, error)

	// BuildDistributionClosure (re)builds the derived distribution_effective
	// table. Call once after ALL backbones (incl. CDM) are ingested — it
	// resolves CDM concepts' in_area name fallback against WCVP twins, which
	// must already be present.
	BuildDistributionClosure(ctx context.Context) error

	// Traits returns every domain.TraitSet hostus holds for conceptID,
	// grouped PER VOCABULARY — TraitSets are never merged across
	// vocabularies, since PoC P10 found their Taxonomy namespaces genuinely
	// diverge (see domain.TraitSet's doc comment). Each returned TraitSet
	// carries the VocabVersion/Taxonomy joined from trait_vocabulary, and
	// its Values are ordered by Dim for a deterministic result. vocabs
	// restricts which vocabularies are returned; nil or empty means every
	// vocabulary the concept has values in. Returns domain.ErrNotFound
	// (wrapped) if conceptID is unknown; a concept with no ingested trait
	// values (but which does exist) returns an empty, non-nil-error slice —
	// callers must not conflate the two.
	Traits(ctx context.Context, conceptID string, vocabs []domain.TraitVocab) ([]domain.TraitSet, error)
	// TraitVocabularies lists every ingested (vocab, version) metadata row,
	// for API/response provenance.
	TraitVocabularies(ctx context.Context) ([]domain.TraitVocabMeta, error)

	// NameSpaceEntries returns every name-space spelling attached to
	// conceptID (SP9/UC4 — e.g. the FloraVeg/ESy names for a WCVP concept),
	// ordered by (space, ext_id) for a deterministic result. spaces
	// restricts which name spaces are returned; nil or empty means every
	// ingested space.
	//
	// A concept can carry SEVERAL entries from the SAME space (FloraVeg
	// spells Festuca ovina three ways under three SeqIDs); they are all
	// returned, never collapsed — see domain.NameSpaceEntry.
	//
	// Returns domain.ErrNotFound (wrapped) if conceptID is unknown; a known
	// concept with no name-space entry returns an empty, non-error slice —
	// callers must not conflate the two.
	NameSpaceEntries(ctx context.Context, conceptID string, spaces []string) ([]domain.NameSpaceEntry, error)
	// NameSpaces lists every ingested name-space provenance row, ordered by
	// id, for API/response provenance and for "hostus ingest" to report what
	// a database actually holds.
	NameSpaces(ctx context.Context) ([]domain.NameSpaceMeta, error)

	// AggregateMembers returns the WCVP concept ids that aggregateConceptID
	// (a Fall-B native concept, rank SPECIES_AGGREGATE/GENUS_AGGREGATE)
	// includes, via concept_aggregate. An aggregate with no linked members
	// returns an empty, non-error slice.
	AggregateMembers(ctx context.Context, aggregateConceptID string) ([]string, error)
	// AggregatesByMember is the reverse of AggregateMembers: it returns
	// every Fall-B aggregate/collective concept id that lists
	// memberConceptID (a WCVP concept) among its concept_aggregate members,
	// via the table's member_concept_id index. A member linked into no
	// aggregate returns an empty, non-error slice. Used to resolve a
	// SPECIES concept's `aggregate_memberships[].aggregate_concept_id`
	// (spec §4) — a caller narrows the result to one name space by
	// matching its "<space>:concept:" id prefix (see
	// internal/application/nativespace_ingest.go's concept id scheme).
	AggregatesByMember(ctx context.Context, memberConceptID string) ([]string, error)

	// VernacularNames returns every vernacular-name row for conceptID (see
	// schema.sql's vernacular table), ordered by (lang, name) for a
	// deterministic result. A concept with no vernacular name returns an
	// empty, non-error slice.
	VernacularNames(ctx context.Context, conceptID string) ([]domain.VernacularName, error)

	// AggregateConcepts returns every taxon_concept in backboneID whose rank
	// is one of ranks — the native Fall-B aggregate/collective-species
	// concepts (Task 5/6) application.ComputeConceptAgreement pairs up
	// across name spaces. An empty result (no matching rows) is not an
	// error.
	AggregateConcepts(ctx context.Context, backboneID string, ranks []domain.Rank) ([]AggregateConceptSummary, error)

	// WriteConceptAgreement (re)writes the precomputed eurosl/germansl
	// aggregate comparison (schema.sql's concept_agreement table) for every
	// given pair, INSERT OR REPLACE per pair. Deliberately NOT part of
	// IngestTx: it runs once, after every backbone has been ingested, so
	// there is no FK-ordering risk requiring transactional batching with an
	// ingest.
	WriteConceptAgreement(ctx context.Context, pairs []domain.ConceptAgreementPair) error

	// Suggest returns FTS5 prefix-match candidates for q (an autosuggest
	// query fragment), scored but UNRANKED: the application layer runs
	// domain.RankSuggestions over the result and truncates to opts.Limit
	// itself. Suggest fetches a superset of at least opts.Limit rows
	// before any Ranks filtering so that filtering never starves the
	// caller of candidates that would otherwise have made the cut; the
	// returned slice's length is therefore not bounded by opts.Limit.
	//
	// q is matched by its domain.Canonicalize'd form as a left-anchored
	// FTS5 prefix query; a canonicalized q shorter than two runes
	// (including empty) returns an empty, non-error result — too short a
	// prefix is both a meaningless autosuggest signal and a pathologically
	// broad FTS5 MATCH. opts.Area == "" means "no area filter": InArea is
	// false on every returned item (an unknown area cannot be "in").
	Suggest(ctx context.Context, q string, opts SuggestOpts) ([]domain.SuggestItem, error)

	// BeginIngest starts an ingest transaction for the given backbone
	// version. Callers must Commit or Rollback the returned IngestTx.
	BeginIngest(ctx context.Context, bv domain.BackboneVersion) (IngestTx, error)

	// BeginTraitIngest starts an ingest transaction for a TRAIT
	// vocabulary. It is deliberately separate from BeginIngest: a trait
	// vocabulary is NOT a taxonomic backbone, so it must never appear in
	// backbone_version — those rows are served verbatim as the
	// backbone_versions provenance block of every /v1/suggest and /v1/match
	// response and gate /health/ready, and a synthetic "trait:<vocab>" row
	// there would tell clients a trait vocabulary is a backbone (and could
	// make a backbone-less database report ready). The returned IngestTx
	// therefore has no backbone: its Finalize is a no-op (there are no
	// concepts to index), and only AddTraitValue/UpsertTraitVocabulary are
	// meaningful on it. Callers must Commit or Rollback.
	BeginTraitIngest(ctx context.Context) (IngestTx, error)
}

// SuggestOpts configures Repository.Suggest.
type SuggestOpts struct {
	// Area is a WGSRPD level-3 area code (e.g. "GER"), or one of a small
	// set of documented convenience aliases (e.g. "DE"); see
	// internal/adapters/sqlite's areaCodes. Empty means no area filter.
	Area string
	// Ranks restricts results to the given domain.Rank values. Empty means
	// no rank filter (every rank is eligible).
	Ranks []domain.Rank
	// TargetSpace names a name space (e.g. "eurosl"); each result then
	// carries its spelling in that space as SuggestItem.TargetSpaceName.
	// Empty means no space is resolved and the field stays empty.
	TargetSpace string
	// Backbone restricts results to concepts of that backbone (e.g. "wcvp").
	// Empty means no backbone filter. It is applied inside the query, ahead
	// of the limit: a caller filtering afterwards would keep almost nothing,
	// since one name can occur once per CDM sec. reference and crowd the
	// single WCVP concept out of the page.
	Backbone string
	// Limit is the caller's target result count; Suggest may return more
	// than Limit candidates (see the Suggest doc comment's fetch-budget
	// note). A value <= 0 uses the adapter's default budget.
	Limit int
	// MatchMode selects how strictly a candidate's name must match q.
	// "" and "name_start" (the default) require at least one of the
	// concept's names (accepted or synonym) to have its FULL canonicalized
	// form start with q's prefix — not merely contain a matching FTS5
	// token anywhere in it. "anywhere" restores the plain FTS5 prefix
	// behavior, where a hit on any token (e.g. a species epithet) is
	// enough. See the sqlite adapter's Suggest doc comment for the SP7
	// finding ("ca" matching an epithet like "canescens") this guards
	// against.
	MatchMode string
}

// MatchCandidate is one row returned by Repository.MatchExact: a concept
// together with the specific name that matched and the role that name
// plays for that concept.
type MatchCandidate struct {
	Concept     domain.Concept
	MatchedName domain.Name
	Role        string // accepted|synonym
	// Homotypic is meaningful only for a synonym-role match: whether the
	// concept_name link is homotypic (same nomenclatural type as the accepted
	// name — a recombination/basionym, i.e. the genuine name-bearer). nil when
	// unknown/unproven, exactly as concept_name.homotypic NULL means "unknown",
	// never "known heterotypic". The match tie-break uses it to prefer, among
	// otherwise-tied concepts, the one for which the queried name is accepted
	// or homotypic (see internal/application/match.go's classify).
	Homotypic *bool
}

// AggregateConceptSummary is the minimal shape Repository.AggregateConcepts
// (and application.ComputeConceptAgreement, its only caller) needs to
// enumerate one backbone's aggregate concepts: just enough to name-match
// across eurosl/germansl without paying for the full Concept()'s synonyms/
// xrefs/distributions, which that use case never touches.
type AggregateConceptSummary struct {
	ConceptID string
	Canonical string
}

// ConceptRelations is Repository.ConceptRelationsInSec' result: the queried
// concept itself plus its one-hop edges into the requested sec. space. The
// concept is returned alongside the edges rather than fetched separately so
// "this id does not exist" and "this id has no relations" are decided by
// one query, in one place.
type ConceptRelations struct {
	Source domain.Concept
	Edges  []ConceptRelationEdge
}

// ConceptRelationEdge is one stored concept_relation row seen from one of
// its two ends (the "source" end a Repository.ConceptRelationsInSec query
// started at).
//
// Relation is the value AS STORED, in the direction the source states it —
// never pre-inverted. Outgoing is what disambiguates that direction:
// "A includes B" and "B included_in A" are different statements, and a
// consumer that cannot tell which one it got cannot use either. Partner is
// the concept at the other end, PartnerSec its resolved sec. reference
// space (Title empty if the space has no sec_reference row).
type ConceptRelationEdge struct {
	Partner    domain.Concept
	PartnerSec domain.SecReference
	Relation   domain.Relation
	// Outgoing is true when the queried concept is the row's from_concept
	// (the statement reads source -> partner) and false when it is the
	// to_concept (the statement reads partner -> source).
	Outgoing bool
	// Source is the id of the backbone/source asserting the relation
	// (e.g. "cdm"), empty if the row carries none.
	Source string
}

// SynonymName is one synonym name Repository.Concept returns for a concept:
// the name itself, plus whether its concept_name link is marked homotypic.
// Homotypic is nil when unknown/unproven (see the ingest homotypic rule in
// internal/application/ingest.go) — never a pointer to false, since NULL in
// the underlying concept_name.homotypic column means "unknown", not
// "known heterotypic".
type SynonymName struct {
	domain.Name
	Homotypic *bool
}

// IngestTx batches the writes of a single backbone ingest into one
// transaction, so a partial/failed import never leaves the index in a
// half-written state.
type IngestTx interface {
	UpsertName(n domain.Name) error
	UpsertConcept(c domain.Concept) error
	LinkName(conceptID, nameID, role string, homotypic *bool) error
	// AddXref writes one cross-reference for conceptID, attributed to the
	// xref_source id given by source. source is "" for xrefs derived by the
	// backbone ingest itself from a taxon row (those are already covered by
	// the backbone's own redistribution value); every xref written by an
	// xref-source ingest must name its source, since that attribution is
	// what ExportBundle's redistribution gate joins against.
	AddXref(conceptID string, x domain.Xref, source string) error
	AddDistribution(conceptID string, d domain.Distribution) error
	// UpsertArea records one (scheme, code) area's human-readable name, keyed
	// by (scheme, code) — first non-empty name wins (INSERT OR IGNORE), so it
	// is safe to call once per distinct area. Backs Repository.Areas.
	UpsertArea(a domain.Area) error
	// AddTraitValue writes one trait_value row for conceptID. A nil
	// tv.NicheWidth/tv.NSystems must be persisted as SQL NULL, not as a
	// 0/0.0 literal — see domain.TraitValue's doc comment on why nil and
	// zero are never interchangeable here.
	AddTraitValue(conceptID string, tv domain.TraitValue) error
	// UpsertTraitVocabulary records one (vocab, version) metadata row,
	// joined onto trait_value reads by Repository.Traits.
	UpsertTraitVocabulary(meta domain.TraitVocabMeta) error
	// UpsertSecReference records one sec. reference space (id + citation
	// title), so a concept's taxon_concept.sec_reference id can be resolved
	// back to the flora it names instead of staying an opaque UUID.
	UpsertSecReference(s domain.SecReference) error
	// AddConceptRelation writes one typed concept relation. Both ends are
	// foreign keys onto taxon_concept, so the caller must have written (or
	// verified) both concepts first — see application.IngestCDM's two-phase
	// resolution. The relation is stored in the DIRECTION THE SOURCE STATES
	// IT; the inverse row is never synthesized (domain.Relation.Inverse
	// exists for query-time traversal instead).
	AddConceptRelation(fromID, toID string, rel domain.Relation, source string) error
	// UpsertXrefSource records one xref-source provenance row (id, version,
	// license, manifest_sha, redistribution), which AddXref's source
	// attribution references and ExportBundle's redistribution gate reads.
	UpsertXrefSource(meta domain.XrefSourceMeta) error
	// UpsertNameSpace records one name-space provenance row (SP9/UC4), the
	// name-space counterpart of UpsertXrefSource. AddNameSpaceEntry's space
	// column is an FK onto it, and ExportBundle's redistribution gate reads
	// it — FloraVeg's redistribution is "unknown", so a bundle carrying its
	// entries is refused unless --force-include-restricted is set.
	UpsertNameSpace(meta domain.NameSpaceMeta) error
	// AddNameSpaceEntry attaches one name-space spelling to conceptID. Both
	// e.Space and conceptID are foreign keys, so the caller must have
	// upserted the space and resolved the concept first — see
	// application.IngestNameSpace's two-phase resolution.
	AddNameSpaceEntry(conceptID string, e domain.NameSpaceEntry) error
	// AddAggregateMember records one aggregate->member edge (concept_
	// aggregate). Both ids must already be written within this transaction
	// or a prior one.
	AddAggregateMember(aggregateConceptID, memberConceptID string) error
	// ResolveNameSpaceMember reads name_space_entry for (space, extID) and
	// returns its concept_id, or "" if no such entry exists — NOT an error,
	// since a Fall-A crosswalk (Task 4) may simply not have resolved that
	// row. Reads WITHIN this same open transaction, which is safe: it is
	// the same IngestTx, not a second Repository call while a foreign
	// transaction is open.
	ResolveNameSpaceMember(space, extID string) (string, error)
	// UpsertClassification records family/order/class for conceptID — see
	// Task 2's schema (taxon_concept.family/order_name/class_name). Empty
	// strings are written as SQL NULL, never as "".
	UpsertClassification(conceptID string, family, orderName, className string) error
	// AddVernacularName writes one vernacular-name row (see the existing
	// `vernacular` table, schema.sql:176).
	AddVernacularName(conceptID string, v domain.VernacularName) error
	// Finalize (re)builds the FTS5 autosuggest index (fts_name/fts_name_map)
	// for every name this transaction has linked to a concept (both the
	// accepted name and its synonyms), so Suggest can find them. Callers
	// must call Finalize after all UpsertName/UpsertConcept/LinkName calls
	// for this ingest and before Commit — it is not implicit in Commit,
	// since it needs to see the transaction's own uncommitted writes.
	Finalize() error
	Commit() error
	Rollback() error
}

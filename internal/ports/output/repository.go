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
	// concept, its synonym names, its cross-references, and its
	// distribution. Returns domain.ErrNotFound (wrapped) if id is unknown.
	Concept(ctx context.Context, id string) (*domain.Concept, []domain.Name, []domain.Xref, []domain.Distribution, error)
	// ConceptByXref resolves a taxon_concept via a cross-reference to an
	// external authority (e.g. authority="powo", extID="396681-1").
	ConceptByXref(ctx context.Context, authority, extID string) (*domain.Concept, error)
	// MatchExact returns every name (accepted or synonym) whose canonical
	// form equals canon, leaving classification (exact vs. exact_author,
	// etc.) to the application layer.
	MatchExact(ctx context.Context, canon string) ([]MatchCandidate, error)
	// BackboneVersions lists every ingested backbone artifact.
	BackboneVersions(ctx context.Context) ([]domain.BackboneVersion, error)

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
	// Limit is the caller's target result count; Suggest may return more
	// than Limit candidates (see the Suggest doc comment's fetch-budget
	// note). A value <= 0 uses the adapter's default budget.
	Limit int
}

// MatchCandidate is one row returned by Repository.MatchExact: a concept
// together with the specific name that matched and the role that name
// plays for that concept.
type MatchCandidate struct {
	Concept     domain.Concept
	MatchedName domain.Name
	Role        string // accepted|synonym
}

// IngestTx batches the writes of a single backbone ingest into one
// transaction, so a partial/failed import never leaves the index in a
// half-written state.
type IngestTx interface {
	UpsertName(n domain.Name) error
	UpsertConcept(c domain.Concept) error
	LinkName(conceptID, nameID, role string, homotypic *bool) error
	AddXref(conceptID string, x domain.Xref) error
	AddDistribution(conceptID string, d domain.Distribution) error
	// AddTraitValue writes one trait_value row for conceptID. A nil
	// tv.NicheWidth/tv.NSystems must be persisted as SQL NULL, not as a
	// 0/0.0 literal — see domain.TraitValue's doc comment on why nil and
	// zero are never interchangeable here.
	AddTraitValue(conceptID string, tv domain.TraitValue) error
	// UpsertTraitVocabulary records one (vocab, version) metadata row,
	// joined onto trait_value reads by Repository.Traits.
	UpsertTraitVocabulary(meta domain.TraitVocabMeta) error
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

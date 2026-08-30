package httpx

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gorilla/mux"

	"github.com/jobrunner/hostus/internal/application"
	"github.com/jobrunner/hostus/internal/domain"
	"github.com/jobrunner/hostus/internal/httperr"
	"github.com/jobrunner/hostus/internal/ports/output"
)

// matchTypeUnresolvable is the wire value used for a MatchResult whose
// domain.MatchType is the zero value (application.MatchNames' encoding of
// "no exact/exact_author/aggregate_alias classification found"). It is not
// a domain.MatchType constant because UNRESOLVABLE is an outcome the HTTP
// adapter renders, not a classification the matcher assigns.
const matchTypeUnresolvable = "unresolvable"

// backboneRefDTO identifies the backbone artifact a concept was ingested
// from, per spec §B's concept shape.
type backboneRefDTO struct {
	ID      string `json:"id"`
	Version string `json:"version"`
}

// synonymDTO is one synonym name grouped under a concept. Homotypic is
// omitted (nil) unless T7's ingest homotypic rule proved it true — a NULL
// concept_name.homotypic means "unknown", never "known heterotypic" (see
// output.SynonymName's doc comment), so it must never be rendered as a
// literal false.
type synonymDTO struct {
	Canonical  string `json:"canonical"`
	Authorship string `json:"authorship,omitempty"`
	Homotypic  *bool  `json:"homotypic,omitempty"`
}

// classificationDTO is one ancestor entry of a concept's classification
// chain, per spec §B's concept shape (§4.3-derived parent linkage).
type classificationDTO struct {
	ConceptID string `json:"concept_id"`
	Canonical string `json:"canonical"`
	Rank      string `json:"rank"`
}

// distributionDTO is one reference-area assignment for a concept, per
// spec §4.3's distribution table (area_scheme, area_code — e.g.
// {"area_scheme": "wgsrpd_l3", "area_code": "GER"}).
type distributionDTO struct {
	AreaScheme string `json:"area_scheme"`
	AreaCode   string `json:"area_code"`
}

// classificationInfoDTO is the classification-above-family object introduced
// by the namespace/classification redesign spec §4 (domain.Concept.Family/
// OrderName/ClassName, Task 4). It is a DIFFERENT field from
// conceptDTO.ParentChain (the pre-existing ancestor-chain array, spec §4.3):
// the two happened to want the same JSON key ("classification"), so
// ParentChain took the rename to "parent_chain" — see its own doc comment.
// Each sub-field is itself omitempty: a concept may carry only Family, for
// instance.
type classificationInfoDTO struct {
	Family string `json:"family,omitempty"`
	Order  string `json:"order,omitempty"`
	Class  string `json:"class,omitempty"`
}

// vernacularNameDTO is one vernacular (common) name for a concept, per spec
// §4's vernacular_names shape.
type vernacularNameDTO struct {
	Language string `json:"language"`
	Name     string `json:"name"`
	// Source is hardcoded to "germansl": AddVernacularName (Task 4) has
	// exactly one writer today (the GermanSL ingest), and neither
	// domain.VernacularName nor the vernacular table itself carries a
	// provenance column. Revisit this the day a second writer exists — see
	// Task 9's report for the ruling.
	Source string `json:"source,omitempty"`
}

// aggregateMemberDTO is one WCVP member of a Fall-B aggregate/collective
// concept (spec §5's members[] shape).
type aggregateMemberDTO struct {
	ConceptID string `json:"concept_id"`
	Name      string `json:"name"`
}

// aggregateMembershipDTO is one name space's aggregate that a SPECIES
// concept belongs to (spec §4's aggregate_memberships[] shape, the Fall-A
// back-reference to Fall-B). AggregateConceptID is omitted when
// output.Repository.AggregatesByMember could not resolve it (e.g. no
// concept_aggregate edge was ingested for this member/space pair) — the
// name space and the aggregate's own spelling (from NameSpaceEntry.Name)
// are still meaningful without it.
type aggregateMembershipDTO struct {
	NameSpace          string `json:"name_space"`
	AggregateConceptID string `json:"aggregate_concept_id,omitempty"`
	AggregateName      string `json:"aggregate_name,omitempty"`
}

// aggregateRanksForMembers is the Fall-B collective-rank set (spec §6) whose
// concepts render members[] instead of/alongside the ordinary shape: a
// SPECIES_AGGREGATE/GENUS_AGGREGATE/SECTION/SUBSECTION/SUBGENUS concept's
// own rows in concept_aggregate name its WCVP members.
var aggregateRanksForMembers = map[domain.Rank]bool{
	domain.RankSpeciesAggregate: true,
	domain.RankGenusAggregate:   true,
	domain.RankSection:          true,
	domain.RankSubsection:       true,
	domain.RankSubgenus:         true,
}

// conceptDTO is the wire shape for GET /v1/concept/{id} and GET /v1/xref,
// per spec §B. It is kept in the http adapter (not domain) since it is a
// response-rendering concern, not a domain concept.
type conceptDTO struct {
	ConceptID string `json:"concept_id"`
	Display   string `json:"display"`
	Canonical string `json:"canonical"`
	// VernacularDE is always empty in SP1: Repository.Concept does not
	// surface the vernacular table yet (no method returns it). Left as an
	// omitempty field so a future task can populate it without a shape
	// change.
	VernacularDE string `json:"vernacular_de,omitempty"`
	Rank         string `json:"rank"`
	// RankVerbatim is the original source "taxonrank" spelling (e.g.
	// WCVP's "proles") when Rank is "OTHER" — the one case where the
	// canonical Rank value alone would otherwise hide which exotic rank
	// this concept actually carries. Omitted entirely (never an empty
	// string) for every canonically-ranked concept, same honesty pattern
	// as synonymDTO.Homotypic: absence means "not applicable", not
	// "unknown".
	RankVerbatim string         `json:"rank_verbatim,omitempty"`
	Status       string         `json:"status"`
	Backbone     backboneRefDTO `json:"backbone"`
	// Xrefs maps authority to ALL of its ext_ids for this concept, never
	// just one: SP4's Wikidata-bridge ingest measured that a concept can
	// legitimately carry several ids for one authority (954 wikidata, 635
	// gbif, 299 wfo, 63 inat, 39 colxr, 3 floraveg in the full index) — a
	// map[string]string would silently keep only the last one written,
	// which is exactly the bug this shape replaces. conceptXrefs orders its
	// rows by (authority, ext_id), so each slice here is already
	// deterministically sorted — never dependent on ingest/query order.
	Xrefs map[string][]string `json:"xrefs,omitempty"`
	// ParentChain is the ancestor chain (root-first — see
	// output.Repository.Classification's doc comment), omitted when empty
	// (a top-level concept with no ingested parent, or one whose backbone
	// simply doesn't carry parent linkage). Named "parent_chain" (not
	// "classification") since the namespace/classification redesign spec §4
	// claimed the "classification" key for a different, NEW field
	// (Classification below) — see classificationInfoDTO's doc comment.
	ParentChain []classificationDTO `json:"parent_chain,omitempty"`
	// Classification is the family/order/class object introduced by spec §4
	// (domain.Concept.Family/OrderName/ClassName). nil (field omitted)
	// when none of the three is known — never an object of all-empty
	// strings.
	Classification *classificationInfoDTO `json:"classification,omitempty"`
	Synonyms       []synonymDTO           `json:"synonyms"`
	Distribution   []distributionDTO      `json:"distribution,omitempty"`
	// Sec names the concept's sec. reference space (id + title), present only
	// for a sec-bearing concept (CDM). Since CDM added many concepts of the
	// SAME name — one per reference work — this is what tells two otherwise
	// identical results apart (SP5). Omitted (never empty) for a concept with
	// no sec. reference (WCVP), so the SP1 shape is unchanged.
	Sec *secReferenceDTO `json:"sec,omitempty"`
	// VernacularNames lists every ingested common name (spec §4), alongside
	// the legacy VernacularDE field above. Omitted when the concept has none.
	VernacularNames []vernacularNameDTO `json:"vernacular_names,omitempty"`
	// Members lists a Fall-B aggregate/collective concept's WCVP members
	// (spec §5), rendered only when Rank is one of aggregateRanksForMembers.
	Members []aggregateMemberDTO `json:"members,omitempty"`
	// AggregateMemberships lists every name space whose aggregate this
	// SPECIES concept belongs to (spec §4's Fall-A back-reference),
	// rendered only for Rank == domain.RankSpecies with at least one
	// NameSpaceEntry.Aggregate == true.
	AggregateMemberships []aggregateMembershipDTO `json:"aggregate_memberships,omitempty"`
}

// conceptToDTO renders a resolved concept (as returned by
// Repository.Concept/Classification) into the wire shape shared by
// /v1/concept/{id} and /v1/xref.
func conceptToDTO(c *domain.Concept, synonyms []output.SynonymName, xrefs []domain.Xref, distribution []domain.Distribution, classification []domain.ClassificationEntry) conceptDTO {
	display := c.AcceptedName.Canonical
	if c.AcceptedName.Authorship != "" {
		display = display + " " + c.AcceptedName.Authorship
	}

	var xrefMap map[string][]string
	if len(xrefs) > 0 {
		xrefMap = make(map[string][]string, len(xrefs))
		for _, x := range xrefs {
			xrefMap[x.Authority] = append(xrefMap[x.Authority], x.ExtID)
		}
	}

	syns := make([]synonymDTO, len(synonyms))
	for i, s := range synonyms {
		syns[i] = synonymDTO{Canonical: s.Canonical, Authorship: s.Authorship, Homotypic: s.Homotypic}
	}

	var dists []distributionDTO
	if len(distribution) > 0 {
		dists = make([]distributionDTO, len(distribution))
		for i, d := range distribution {
			dists[i] = distributionDTO{AreaScheme: d.AreaScheme, AreaCode: d.AreaCode}
		}
	}

	var parentChain []classificationDTO
	if len(classification) > 0 {
		parentChain = make([]classificationDTO, len(classification))
		for i, entry := range classification {
			parentChain[i] = classificationDTO{ConceptID: entry.ConceptID, Canonical: entry.Canonical, Rank: string(entry.Rank)}
		}
	}

	return conceptDTO{
		ConceptID:      c.ID,
		Display:        display,
		Canonical:      c.AcceptedName.Canonical,
		Rank:           string(c.Rank),
		RankVerbatim:   c.RankVerbatim,
		Status:         string(c.Status),
		Backbone:       backboneRefDTO{ID: c.BackboneID, Version: c.BackboneVersion},
		Xrefs:          xrefMap,
		ParentChain:    parentChain,
		Classification: classificationInfo(c),
		Synonyms:       syns,
		Distribution:   dists,
	}
}

// classificationInfo builds conceptDTO.Classification from c's
// Family/OrderName/ClassName (spec §4), or nil if none of the three is
// known.
func classificationInfo(c *domain.Concept) *classificationInfoDTO {
	if c.Family == "" && c.OrderName == "" && c.ClassName == "" {
		return nil
	}
	return &classificationInfoDTO{Family: c.Family, Order: c.OrderName, Class: c.ClassName}
}

// classificationInfoFromMatch is classificationInfo's counterpart for
// application.MatchResult.Classification (Task 10): same "nil unless
// something is known" rule, applied to the domain.Classification struct
// matchNamesFiltered fills in rather than a *domain.Concept.
func classificationInfoFromMatch(cl domain.Classification) *classificationInfoDTO {
	if cl.Family == "" && cl.OrderName == "" && cl.ClassName == "" {
		return nil
	}
	return &classificationInfoDTO{Family: cl.Family, Order: cl.OrderName, Class: cl.ClassName}
}

// matchNameDTO is one entry of POST /v1/match's request body, per §B.2.
type matchNameDTO struct {
	ID       string `json:"id"`
	Verbatim string `json:"verbatim"`
}

// matchRequestDTO is the POST /v1/match request body. TargetSpace (SP9/UC4)
// selects an ingested name space to resolve each match into; without it the
// response is byte-for-byte the SP1 shape. EntryBackbone/EntrySec (SP5) narrow
// verbatim RESOLUTION to one backbone / sec. reference space, so a name shared
// across the multi-backbone index resolves to one concept instead of an
// ambiguous tie; they apply to every name in the batch. (They replace the
// former, never-implemented `sec_hint` field — an unknown JSON field is
// ignored by the decoder, so old clients that still send `sec_hint` are
// unaffected.)
type matchRequestDTO struct {
	Names         []matchNameDTO `json:"names"`
	TargetSpace   string         `json:"target_space,omitempty"`
	EntryBackbone string         `json:"entry_backbone,omitempty"`
	EntrySec      string         `json:"entry_sec,omitempty"`
}

// matchResultDTO is one entry of POST /v1/match's response, per §B.2. An
// UNRESOLVABLE result is a normal element of this array with a 200 status,
// never an HTTP error.
type matchResultDTO struct {
	ID             string   `json:"id"`
	MatchType      string   `json:"match_type"`
	Confidence     float64  `json:"confidence"`
	ConceptID      string   `json:"concept_id,omitempty"`
	Candidates     []string `json:"candidates,omitempty"`
	RequiresReview bool     `json:"requires_review,omitempty"`
	Note           string   `json:"note,omitempty"`

	// The three UC4 fields below appear ONLY when the request named a
	// target_space; on the plain path they stay zero and omitempty drops them,
	// leaving the SP1 shape untouched.
	//
	// TargetSpaceName is the ESy-compatible spelling the target space uses.
	// AggregatePolicy is the tri-state (known/unresolvable/absent) — absent
	// (empty, dropped) means no aggregate is involved.
	// ESyDiagnosticRelevance is ALWAYS esyRelevanceNotDeterminable on the
	// target-space path: it is emitted precisely so its meaning ("not
	// determinable, ESy rule set not available") is conspicuous rather than
	// silently missing — a consumer must never read its absence as "not
	// relevant". See docs/reference/http-api.md.
	TargetSpaceName        string `json:"target_space_name,omitempty"`
	AggregatePolicy        string `json:"aggregate_policy,omitempty"`
	ESyDiagnosticRelevance string `json:"esy_diagnostic_relevance,omitempty"`

	// Classification and AggregateResolution are Task 10's additions,
	// unconditional on target_space (unlike the three UC4 fields above):
	// Classification is rendered whenever any of Family/Order/Class is
	// known (reusing classificationInfoDTO/classificationInfo, exactly as
	// GET /v1/concept/{id} does), and AggregateResolution only when the
	// match carried one (an aggregate/collective-rank hit).
	Classification      *classificationInfoDTO  `json:"classification,omitempty"`
	AggregateResolution *aggregateResolutionDTO `json:"aggregate_resolution,omitempty"`
}

// aggregateResolutionOptionDTO is one name space's entry of a
// matchResultDTO.aggregate_resolution.options[] array (Task 10).
type aggregateResolutionOptionDTO struct {
	NameSpace          string `json:"name_space"`
	Status             string `json:"status,omitempty"`
	AggregateConceptID string `json:"aggregate_concept_id,omitempty"`
	MemberCount        int    `json:"member_count,omitempty"`
}

// aggregateResolutionDTO is POST /v1/match's aggregate_resolution object
// (Task 10), present only for a result whose query or resolved concept is
// an aggregate/collective rank.
type aggregateResolutionDTO struct {
	RequestedNameSpace string                         `json:"requested_name_space"`
	Status             string                         `json:"status,omitempty"`
	MemberCount        int                            `json:"member_count,omitempty"`
	Options            []aggregateResolutionOptionDTO `json:"options"`
	Agreement          string                         `json:"agreement,omitempty"`
}

// aggregateResolutionToDTO renders application.MatchResult.AggregateResolution
// as the wire shape, or nil if res is nil (the caller must check res != nil
// before rendering the containing field, since matchResultDTO.
// AggregateResolution is itself omitempty on a nil pointer).
func aggregateResolutionToDTO(res *domain.AggregateResolution) *aggregateResolutionDTO {
	if res == nil {
		return nil
	}
	options := make([]aggregateResolutionOptionDTO, len(res.Options))
	for i, o := range res.Options {
		options[i] = aggregateResolutionOptionDTO{
			NameSpace:          o.NameSpace,
			Status:             string(o.Status),
			AggregateConceptID: o.AggregateConceptID,
			MemberCount:        o.MemberCount,
		}
	}
	return &aggregateResolutionDTO{
		RequestedNameSpace: res.RequestedNameSpace,
		Status:             string(res.Status),
		MemberCount:        res.MemberCount,
		Options:            options,
		Agreement:          string(res.Agreement),
	}
}

// esyRelevanceNotDeterminable is the sentinel value of every
// esy_diagnostic_relevance field while the ESy rule set is not ingested (SP9).
// It is a self-describing string, never null and never absent on the
// target-space path, so no consumer in any language can read it as a falsy
// "not relevant" — that false negative is exactly what UC4 exists to prevent.
const esyRelevanceNotDeterminable = "not_determinable"

// matchResponseDTO is the POST /v1/match response envelope.
type matchResponseDTO struct {
	BackboneVersions map[string]string `json:"backbone_versions"`
	Results          []matchResultDTO  `json:"results"`
}

// writeJSON encodes v as a 200 OK JSON response body, setting Content-Type
// first so it is present even if Encode fails partway through writing the
// body. Every current success path (concept, match, suggest, synonyms) is a
// 200; error bodies go through httperr.Write, which owns its own status —
// so writeJSON takes no status parameter (a hardcoded one that every caller
// already agreed on, rather than a param unparam would flag as dead).
func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(v)
}

// writeConcept resolves id via repo.Concept and writes it as a conceptDTO,
// or a 404 NOT_FOUND envelope if id is unknown. Shared by handleConcept and
// handleXref (which resolves its own id via ConceptByXref first) so both
// endpoints render the identical concept shape from the identical query
// path.
func writeConcept(w http.ResponseWriter, r *http.Request, repo output.Repository, id string) {
	c, synonyms, xrefs, distribution, err := repo.Concept(r.Context(), id)
	if errors.Is(err, domain.ErrNotFound) {
		httperr.Write(w, http.StatusNotFound, httperr.NotFound, "concept not found")
		return
	}
	if err != nil {
		httperr.InternalError(w)
		return
	}
	classification, err := repo.Classification(r.Context(), id)
	if err != nil {
		httperr.InternalError(w)
		return
	}
	dto := conceptToDTO(c, synonyms, xrefs, distribution, classification)
	// A sec-bearing concept (CDM) carries its reference space so same-name
	// concepts are distinguishable (SP5). A missing sec_reference row is
	// context, not the answer — omit it rather than fail the concept.
	if c.SecReference != "" {
		if sr, err := repo.SecReferenceByID(r.Context(), c.SecReference); err == nil {
			dto.Sec = &secReferenceDTO{ID: sr.ID, Title: sr.Title}
		}
	}
	if vnames, err := repo.VernacularNames(r.Context(), id); err == nil {
		dto.VernacularNames = vernacularNamesToDTO(vnames)
	}
	if aggregateRanksForMembers[c.Rank] {
		dto.Members = aggregateMembers(r.Context(), repo, id)
	}
	if c.Rank == domain.RankSpecies {
		dto.AggregateMemberships = aggregateMembershipsFor(r.Context(), repo, id)
	}
	writeJSON(w, dto)
}

// vernacularNameSource is the hardcoded value of every rendered
// vernacular_names[].source: AddVernacularName (Task 4) has exactly one
// writer today, the GermanSL ingest — see vernacularNameDTO's doc comment.
const vernacularNameSource = "germansl"

// vernacularNamesToDTO renders Repository.VernacularNames' result as the
// wire shape, stamping every entry's Source with vernacularNameSource.
func vernacularNamesToDTO(vnames []domain.VernacularName) []vernacularNameDTO {
	if len(vnames) == 0 {
		return nil
	}
	out := make([]vernacularNameDTO, len(vnames))
	for i, v := range vnames {
		out[i] = vernacularNameDTO{Language: v.Language, Name: v.Name, Source: vernacularNameSource}
	}
	return out
}

// aggregateMembers resolves a Fall-B aggregate/collective concept's WCVP
// members (spec §5) via Repository.AggregateMembers + one Repository.Concept
// call per member id (N+1 — acceptable at the typically small member counts
// these concepts carry; see Task 9's report). A member id that no longer
// resolves (should not happen given FK integrity, but Concept can still
// error) is silently skipped rather than failing the whole response.
func aggregateMembers(ctx context.Context, repo output.Repository, aggregateConceptID string) []aggregateMemberDTO {
	memberIDs, err := repo.AggregateMembers(ctx, aggregateConceptID)
	if err != nil || len(memberIDs) == 0 {
		return nil
	}
	out := make([]aggregateMemberDTO, 0, len(memberIDs))
	for _, memberID := range memberIDs {
		mc, _, _, _, err := repo.Concept(ctx, memberID)
		if err != nil {
			continue
		}
		out = append(out, aggregateMemberDTO{ConceptID: memberID, Name: mc.AcceptedName.Canonical})
	}
	return out
}

// aggregateMembershipsFor resolves a SPECIES concept's Fall-A back-reference
// into every name space's aggregate it belongs to (spec §4), from its
// NameSpaceEntry rows flagged Aggregate == true. AggregateConceptID is
// resolved via Repository.AggregatesByMember, narrowed to the entry's own
// space by that space's "<space>:concept:" id prefix (the id scheme
// internal/application/nativespace_ingest.go's Fall-B ingest assigns) —
// left empty (omitted on the wire) if no aggregate concept resolves for
// that space, so a caller still gets the space + aggregate name.
func aggregateMembershipsFor(ctx context.Context, repo output.Repository, conceptID string) []aggregateMembershipDTO {
	entries, err := repo.NameSpaceEntries(ctx, conceptID, nil)
	if err != nil {
		return nil
	}
	var aggregateIDs []string
	var out []aggregateMembershipDTO
	for _, e := range entries {
		if !e.Aggregate {
			continue
		}
		membership := aggregateMembershipDTO{NameSpace: e.Space, AggregateName: e.Name}
		if aggregateIDs == nil {
			aggregateIDs, _ = repo.AggregatesByMember(ctx, conceptID)
		}
		prefix := e.Space + ":concept:"
		for _, aggID := range aggregateIDs {
			if strings.HasPrefix(aggID, prefix) {
				membership.AggregateConceptID = aggID
				break
			}
		}
		out = append(out, membership)
	}
	return out
}

// handleConcept serves GET /v1/concept/{id}.
func handleConcept(repo output.Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeConcept(w, r, repo, mux.Vars(r)["id"])
	}
}

// handleXref serves GET /v1/xref?authority=&id=, reverse-resolving a
// cross-reference to a concept and rendering it via the same conceptDTO
// mapping as /v1/concept/{id}.
func handleXref(repo output.Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authority := r.URL.Query().Get("authority")
		id := r.URL.Query().Get("id")
		if authority == "" || id == "" {
			httperr.InvalidQueryError(w, "authority and id query parameters are required")
			return
		}

		c, err := repo.ConceptByXref(r.Context(), authority, id)
		if errors.Is(err, domain.ErrNotFound) {
			httperr.Write(w, http.StatusNotFound, httperr.NotFound, "concept not found")
			return
		}
		if err != nil {
			httperr.InternalError(w)
			return
		}
		writeConcept(w, r, repo, c.ID)
	}
}

// handleMatch serves POST /v1/match: batch verbatim-name resolution via
// application.MatchNames. A per-item UNRESOLVABLE outcome is rendered as a
// normal 200 result element (matchTypeUnresolvable), never as an HTTP
// error; only a malformed request body is a (400 INVALID_QUERY) HTTP error.
func handleMatch(repo output.Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body matchRequestDTO
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			httperr.InvalidQueryError(w, "malformed request body")
			return
		}

		reqs := make([]application.MatchRequest, len(body.Names))
		for i, n := range body.Names {
			reqs[i] = application.MatchRequest{ID: n.ID, Verbatim: n.Verbatim}
		}

		results, err := application.MatchInSpace(r.Context(), repo, reqs, body.TargetSpace,
			application.MatchFilter{Backbone: body.EntryBackbone, Sec: body.EntrySec})
		if errors.Is(err, application.ErrUnknownTargetSpace) {
			httperr.InvalidQueryError(w, "unknown target_space "+strconv.Quote(body.TargetSpace))
			return
		}
		if errors.Is(err, application.ErrUnknownBackbone) {
			httperr.InvalidQueryError(w, "unknown entry_backbone "+strconv.Quote(body.EntryBackbone))
			return
		}
		if errors.Is(err, application.ErrUnknownSec) {
			httperr.InvalidQueryError(w, "unknown entry_sec "+strconv.Quote(body.EntrySec))
			return
		}
		if err != nil {
			httperr.InternalError(w)
			return
		}

		versions, err := repo.BackboneVersions(r.Context())
		if err != nil {
			httperr.InternalError(w)
			return
		}
		backboneVersions := make(map[string]string, len(versions))
		for _, v := range versions {
			backboneVersions[v.ID] = v.Version
		}

		writeJSON(w, matchResponseDTO{
			BackboneVersions: backboneVersions,
			Results:          matchResultsToDTO(results, body.TargetSpace != ""),
		})
	}
}

// matchResultsToDTO renders application.MatchInSpace's results as the wire
// shape, mapping the zero-value domain.MatchType (UNRESOLVABLE) to
// matchTypeUnresolvable. targetSpace reports whether the request named a
// target_space: only then are the three UC4 fields emitted, and
// esy_diagnostic_relevance is set on EVERY result to the sentinel (its whole
// point is to be conspicuously present, per its DTO comment), while
// target_space_name/aggregate_policy carry whatever the matcher resolved
// (empty ones drop via omitempty).
func matchResultsToDTO(results []application.MatchResult, targetSpace bool) []matchResultDTO {
	out := make([]matchResultDTO, len(results))
	for i, res := range results {
		matchType := string(res.MatchType)
		if matchType == "" {
			matchType = matchTypeUnresolvable
		}
		dto := matchResultDTO{
			ID:             res.ID,
			MatchType:      matchType,
			Confidence:     res.Confidence,
			ConceptID:      res.ConceptID,
			Candidates:     res.Candidates,
			RequiresReview: res.RequiresReview,
			Note:           res.Note,
		}
		if targetSpace {
			dto.TargetSpaceName = res.TargetSpaceName
			dto.AggregatePolicy = string(res.AggregatePolicy)
			dto.ESyDiagnosticRelevance = esyRelevanceNotDeterminable
		}
		dto.Classification = classificationInfoFromMatch(res.Classification)
		dto.AggregateResolution = aggregateResolutionToDTO(res.AggregateResolution)
		out[i] = dto
	}
	return out
}

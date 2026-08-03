package httpx

import (
	"encoding/json"
	"errors"
	"net/http"

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
	// as synonymDTO.Homotypic/traits' niche_width: absence means "not
	// applicable", not "unknown".
	RankVerbatim string            `json:"rank_verbatim,omitempty"`
	Status       string            `json:"status"`
	Backbone     backboneRefDTO    `json:"backbone"`
	Xrefs        map[string]string `json:"xrefs,omitempty"`
	// Classification is the parent chain (root-first — see
	// output.Repository.Classification's doc comment), omitted when empty
	// (a top-level concept with no ingested parent, or one whose backbone
	// simply doesn't carry parent linkage).
	Classification []classificationDTO `json:"classification,omitempty"`
	Synonyms       []synonymDTO        `json:"synonyms"`
	Distribution   []distributionDTO   `json:"distribution,omitempty"`
}

// conceptToDTO renders a resolved concept (as returned by
// Repository.Concept/Classification) into the wire shape shared by
// /v1/concept/{id} and /v1/xref.
func conceptToDTO(c *domain.Concept, synonyms []output.SynonymName, xrefs []domain.Xref, distribution []domain.Distribution, classification []domain.ClassificationEntry) conceptDTO {
	display := c.AcceptedName.Canonical
	if c.AcceptedName.Authorship != "" {
		display = display + " " + c.AcceptedName.Authorship
	}

	var xrefMap map[string]string
	if len(xrefs) > 0 {
		xrefMap = make(map[string]string, len(xrefs))
		for _, x := range xrefs {
			xrefMap[x.Authority] = x.ExtID
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

	var classif []classificationDTO
	if len(classification) > 0 {
		classif = make([]classificationDTO, len(classification))
		for i, entry := range classification {
			classif[i] = classificationDTO{ConceptID: entry.ConceptID, Canonical: entry.Canonical, Rank: string(entry.Rank)}
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
		Classification: classif,
		Synonyms:       syns,
		Distribution:   dists,
	}
}

// matchNameDTO is one entry of POST /v1/match's request body, per §B.2.
type matchNameDTO struct {
	ID       string `json:"id"`
	Verbatim string `json:"verbatim"`
}

// matchRequestDTO is the POST /v1/match request body. TargetSpace/SecHint
// are accepted (per §B.2's example payload) but unused in SP1: sec.-space
// translation is out of scope until SP5 ("POST /v1/translate").
type matchRequestDTO struct {
	Names       []matchNameDTO `json:"names"`
	TargetSpace string         `json:"target_space,omitempty"`
	SecHint     string         `json:"sec_hint,omitempty"`
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
}

// matchResponseDTO is the POST /v1/match response envelope.
type matchResponseDTO struct {
	BackboneVersions map[string]string `json:"backbone_versions"`
	Results          []matchResultDTO  `json:"results"`
}

// writeJSON encodes v as a 200 OK JSON response body, setting Content-Type
// first so it is present even if Encode fails partway through writing the
// body. Every current success path (concept, match, suggest, traits) is a
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
	writeJSON(w, conceptToDTO(c, synonyms, xrefs, distribution, classification))
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

		results, err := application.MatchNames(r.Context(), repo, reqs)
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
			Results:          matchResultsToDTO(results),
		})
	}
}

// matchResultsToDTO renders application.MatchNames' results as the wire
// shape, mapping the zero-value domain.MatchType (UNRESOLVABLE) to
// matchTypeUnresolvable.
func matchResultsToDTO(results []application.MatchResult) []matchResultDTO {
	out := make([]matchResultDTO, len(results))
	for i, res := range results {
		matchType := string(res.MatchType)
		if matchType == "" {
			matchType = matchTypeUnresolvable
		}
		out[i] = matchResultDTO{
			ID:             res.ID,
			MatchType:      matchType,
			Confidence:     res.Confidence,
			ConceptID:      res.ConceptID,
			Candidates:     res.Candidates,
			RequiresReview: res.RequiresReview,
			Note:           res.Note,
		}
	}
	return out
}

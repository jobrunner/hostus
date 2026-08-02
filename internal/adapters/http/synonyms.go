package httpx

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"github.com/gorilla/mux"

	"github.com/jobrunner/hostus/internal/application"
	"github.com/jobrunner/hostus/internal/domain"
	"github.com/jobrunner/hostus/internal/httperr"
	"github.com/jobrunner/hostus/internal/ports/output"
)

// synonymOrdering states, in the payload itself, the rule that decided the
// order of `synonyms`. A caller who receives three names out of 26 needs to
// know what "the top three" was sorted by; every field the rule mentions is
// rendered per item, so the claim is checkable rather than decorative.
const synonymOrdering = "publishable first, then homotypic before unknown before heterotypic, the basionym first within its typification block, then name_id"

// synonymDetailDTO is one synonym in a GET /v1/concept/{id}/synonyms
// response, together with the reasoning behind its verdict and its
// position.
//
// Every field that carries a judgement is present unconditionally, even
// when it is false or the boring value: `is_basionym: false` and
// `publishable: true` are answers, and omitting them would leave a client
// unable to tell "not the basionym" from "hostus did not look".
//
// Only two fields are omitted when empty, and in both cases absence is
// itself the statement:
//
//   - nom_status — the source recorded nothing. That is not the same as
//     "checked and found clean" (domain.JudgementAbsent), which is why
//     nom_status_judgement is rendered ALWAYS and says `absent` explicitly.
//     Rendering the raw cell as "" would suggest a value of empty string.
//   - exclusion — the synonym was not excluded. A `"exclusion": ""` would
//     read as a nameless exclusion rule.
//
// typification is the tri-state of domain.Typification. Note that
// `heterotypic` cannot occur on the current index: concept_name.homotypic
// is 1 (271.821 rows) or NULL (1.133.475 rows) and never 0, so a synonym is
// either proven homotypic or `unknown`. SP3 deliberately refused to guess
// heterotypy, and hostus does not collapse an unknown onto it. The value
// exists in the model because the column is a tri-state, not because a
// response will show it today.
type synonymDetailDTO struct {
	// Position is the 1-based rank in this response's `synonyms` array,
	// echoed so a truncated list still says where each entry stood.
	Position   int    `json:"position"`
	NameID     string `json:"name_id"`
	Canonical  string `json:"canonical"`
	Authorship string `json:"authorship,omitempty"`
	// Rank is the SYNONYM's own taxonomic rank, not the caller's
	// publication level (that is echoed once, as publication_rank).
	Rank               string `json:"rank"`
	Typification       string `json:"typification"`
	IsBasionym         bool   `json:"is_basionym"`
	NomStatus          string `json:"nom_status,omitempty"`
	NomStatusJudgement string `json:"nom_status_judgement"`
	Publishable        bool   `json:"publishable"`
	Exclusion          string `json:"exclusion,omitempty"`
	Reason             string `json:"reason"`
}

// synonymSummaryDTO is the auditable counterpart to the list: what the
// concept HAS, versus what this response CARRIES.
//
// Total/Publishable/Absent/Excluded/UnclassifiedStatuses always describe
// every synonym of the concept, never the returned page — a filter that
// dropped 20 of 26 rows and did not say so would be indistinguishable from
// a broken query. Returned/Truncated describe the page, and Truncated is
// kept OUT of Excluded on purpose: a truncated synonym was not judged
// irrelevant, it just did not fit under `max`.
type synonymSummaryDTO struct {
	Total       int `json:"total"`
	Publishable int `json:"publishable"`
	Returned    int `json:"returned"`
	Truncated   int `json:"truncated"`
	// Absent counts the publishable synonyms that rest on an EMPTY
	// nom_status — "nothing was recorded" rather than "recorded as sound".
	Absent int `json:"absent"`
	// Excluded maps a domain.SynonymExclusion to its count. Always present
	// (an empty object when nothing was excluded), so a client can read it
	// without a nil check.
	Excluded map[string]int `json:"excluded"`
	// UnclassifiedStatuses lists the distinct raw nom_status values no rule
	// covered. They were withheld, so they must be visible verbatim — this
	// is the list from which the rule table gets extended.
	UnclassifiedStatuses []string `json:"unclassified_statuses"`
}

// synonymsResponseDTO is the GET /v1/concept/{id}/synonyms envelope.
type synonymsResponseDTO struct {
	ConceptID string `json:"concept_id"`
	// Relevance is the RESOLVED mode ("all" or "publication"), never the
	// caller's raw parameter, so a response says which rules produced it.
	Relevance string `json:"relevance"`
	// PublicationRank is the resolved publication level, omitted when the
	// caller asked for no rank exclusion at all.
	PublicationRank string             `json:"publication_rank,omitempty"`
	Ordering        string             `json:"ordering"`
	Synonyms        []synonymDetailDTO `json:"synonyms"`
	Summary         synonymSummaryDTO  `json:"summary"`
}

// synonymsToDTO renders application.Synonyms' result as the wire shape.
func synonymsToDTO(res application.SynonymsResult) synonymsResponseDTO {
	synonyms := make([]synonymDetailDTO, len(res.Synonyms))
	for i, r := range res.Synonyms {
		synonyms[i] = synonymDetailDTO{
			Position:           i + 1,
			NameID:             r.Candidate.NameID,
			Canonical:          r.Candidate.Canonical,
			Authorship:         r.Candidate.Authorship,
			Rank:               string(r.Candidate.Rank),
			Typification:       string(r.Typification),
			IsBasionym:         r.Candidate.IsBasionym,
			NomStatus:          r.Candidate.NomStatus,
			NomStatusJudgement: string(r.Status.Judgement),
			Publishable:        r.Publishable,
			Exclusion:          string(r.Exclusion),
			Reason:             r.Reason,
		}
	}

	excluded := make(map[string]int, len(res.Summary.Excluded))
	for rule, n := range res.Summary.Excluded {
		excluded[string(rule)] = n
	}
	unclassified := res.Summary.UnclassifiedStatuses
	if unclassified == nil {
		unclassified = []string{}
	}

	return synonymsResponseDTO{
		ConceptID:       res.ConceptID,
		Relevance:       res.Relevance,
		PublicationRank: res.PublicationRank,
		Ordering:        synonymOrdering,
		Synonyms:        synonyms,
		Summary: synonymSummaryDTO{
			Total:                res.Summary.Total,
			Publishable:          res.Summary.Publishable,
			Returned:             len(synonyms),
			Truncated:            res.Truncated,
			Absent:               res.Summary.Absent,
			Excluded:             excluded,
			UnclassifiedStatuses: unclassified,
		},
	}
}

// parseSynonymMax parses the `max` query parameter. An empty parameter
// yields 0, which application.Synonyms reads as "no truncation"; the range
// check itself lives there (application.MaxSynonymLimit), so the bound is
// stated once. A non-numeric value is rejected here, since the application
// layer never sees the string.
func parseSynonymMax(param string) (int, error) {
	if param == "" {
		return 0, nil
	}
	return strconv.Atoi(param)
}

// handleSynonyms serves GET /v1/concept/{id}/synonyms?relevance=&rank=&max=
// (spec §UC5).
//
// Contract, in the order a caller trips over it:
//   - no `relevance` means the UNFILTERED list (application.RelevanceAll) —
//     the publication filter is strong enough that it must be asked for;
//   - an unknown `relevance`, an unsupported `rank`, a non-numeric `max` or
//     a `max` outside [0, application.MaxSynonymLimit] all report 400
//     INVALID_QUERY, naming the offending value;
//   - an unknown concept id reports 404 NOT_FOUND;
//   - a known concept with no synonyms reports 200 with an empty
//     `synonyms` array and a zeroed summary — it is not a 404, since the
//     concept itself is real (same rule as /v1/concept/{id}/traits).
func handleSynonyms(repo output.Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()

		max, err := parseSynonymMax(query.Get("max"))
		if err != nil {
			httperr.InvalidQueryError(w, "max must be an integer")
			return
		}

		res, err := application.Synonyms(r.Context(), repo, application.SynonymsRequest{
			ConceptID:       mux.Vars(r)["id"],
			Relevance:       query.Get("relevance"),
			PublicationRank: query.Get("rank"),
			Max:             max,
		})
		if err != nil {
			writeSynonymsError(w, err, query)
			return
		}
		writeJSON(w, synonymsToDTO(res))
	}
}

// writeSynonymsError maps application.Synonyms' named failures onto the
// error contract.
//
// Each 400 message is composed HERE from the raw query value rather than
// forwarded from the application error, for the reason parseSuggestRanks
// documents: the application text is prefixed ("application: ...") and
// would leak an internal package name into a client-facing message. The
// offending value itself is what the caller needs, and it is repeated
// verbatim. Anything unrecognized is a 500 — a storage failure must not
// masquerade as "no such concept" or as an empty list, both of which would
// be false statements about the data.
func writeSynonymsError(w http.ResponseWriter, err error, query url.Values) {
	switch {
	case errors.Is(err, application.ErrInvalidRelevance):
		httperr.InvalidQueryError(w, fmt.Sprintf("unknown relevance %q (supported: %s, %s)",
			query.Get("relevance"), application.RelevanceAll, application.RelevancePublication))
	case errors.Is(err, application.ErrInvalidPublicationRank):
		httperr.InvalidQueryError(w, fmt.Sprintf("unsupported rank %q (supported: %s)",
			query.Get("rank"), application.PublicationRankSpecies))
	case errors.Is(err, application.ErrInvalidMax):
		httperr.InvalidQueryError(w, fmt.Sprintf("max %q is not in [0, %d]",
			query.Get("max"), application.MaxSynonymLimit))
	case errors.Is(err, domain.ErrNotFound):
		httperr.Write(w, http.StatusNotFound, httperr.NotFound, "concept not found")
	default:
		httperr.InternalError(w)
	}
}

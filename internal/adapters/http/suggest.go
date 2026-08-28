package httpx

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/jobrunner/hostus/internal/application"
	"github.com/jobrunner/hostus/internal/domain"
	"github.com/jobrunner/hostus/internal/httperr"
	"github.com/jobrunner/hostus/internal/ports/output"
)

// suggestItemDTO is one autosuggest candidate, per spec §B.1. PrefixHit is
// deliberately not rendered: it is a ranking signal domain.RankSuggestions
// consumes internally, not something the frontend autosuggest field needs
// on the wire.
type suggestItemDTO struct {
	ConceptID string `json:"concept_id"`
	Display   string `json:"display"`
	Canonical string `json:"canonical"`
	// VernacularDE is omitted when empty: SP1/SP2 ingest does not yet
	// populate every concept's German vernacular name (see conceptDTO's
	// analogous field for the same rationale).
	VernacularDE string  `json:"vernacular_de,omitempty"`
	Rank         string  `json:"rank"`
	Status       string  `json:"status"`
	InArea       bool    `json:"in_area"`
	Score        float64 `json:"score"`
	// Aggregate is true when the concept was reached via an aggregate
	// name-space spelling (e.g. "Achillea millefolium aggr."); the console
	// badges such hits. Omitted when false — the SP1/SP2 shape is unchanged
	// for a plain hit.
	Aggregate bool `json:"aggregate,omitempty"`
	// Sec names the candidate's sec. reference space (id + title), present
	// only for a sec-bearing (CDM) concept. Since CDM holds many concepts of
	// the SAME name — one per reference work, all otherwise identical here
	// down to the score — this is what distinguishes them (SP5). Omitted for a
	// concept with no sec. reference (WCVP), so the SP1/SP2 shape is unchanged.
	Sec *secReferenceDTO `json:"sec,omitempty"`
	// TargetSpaceName is the candidate's spelling in the requested
	// target_space, present only when one was requested AND this concept has
	// an entry there. Its ABSENCE is the useful half: it says this candidate
	// cannot be carried into that space, which is what a caller picking a
	// concept for downstream use needs to see while choosing.
	TargetSpaceName string `json:"target_space_name,omitempty"`
}

// suggestResponseDTO is the GET /v1/suggest response envelope, per spec
// §B.1.
type suggestResponseDTO struct {
	BackboneVersions map[string]string `json:"backbone_versions"`
	Results          []suggestItemDTO  `json:"results"`
}

// suggestResponseToDTO renders application.Suggest's result as the wire
// shape.
func suggestResponseToDTO(resp application.SuggestResponse) suggestResponseDTO {
	results := make([]suggestItemDTO, len(resp.Results))
	for i, item := range resp.Results {
		results[i] = suggestItemDTO{
			ConceptID:    item.ConceptID,
			Display:      item.Display,
			Canonical:    item.Canonical,
			VernacularDE: item.VernacularDE,
			Rank:         string(item.Rank),
			Status:       string(item.Status),
			InArea:       item.InArea,
			Score:        item.Score,
			Aggregate:    item.Aggregate,

			TargetSpaceName: item.TargetSpaceName,
		}
	}
	return suggestResponseDTO{
		BackboneVersions: resp.BackboneVersions,
		Results:          results,
	}
}

// attachSuggestSec fills each result's Sec {id,title} for a sec-bearing
// concept, resolving the title from the concept's sec_reference id. items and
// dtos are parallel (same order, same length). A missing sec_reference row is
// context, not the answer — leave Sec absent rather than fail the suggest.
// Resolved per item (no cache): §B.1 caps suggest at a small limit, so this is
// a handful of point lookups at most.
func attachSuggestSec(ctx context.Context, repo output.Repository, items []domain.SuggestItem, dtos []suggestItemDTO) {
	for i := range items {
		if items[i].SecReference == "" {
			continue
		}
		if sr, err := repo.SecReferenceByID(ctx, items[i].SecReference); err == nil {
			dtos[i].Sec = &secReferenceDTO{ID: sr.ID, Title: sr.Title}
		}
	}
}

// parseSuggestRanks splits the comma-separated `rank` query parameter into
// domain.Rank values via domain.ParseRank. An empty param returns (nil,
// nil) — no rank filter, per output.SuggestOpts.Ranks' documented "empty
// means every rank is eligible" convention. Any unrecognized token is
// reported as a fresh error naming just the offending token (e.g. `unknown
// rank "foo"`), rather than propagating domain.ParseRank's own error
// verbatim — concatenating that one (which already reads `domain: unknown
// taxon rank "foo"`) produced a doubled-up, internals-leaking 400 message.
func parseSuggestRanks(param string) ([]domain.Rank, error) {
	if param == "" {
		return nil, nil
	}
	tokens := strings.Split(param, ",")
	ranks := make([]domain.Rank, 0, len(tokens))
	for _, tok := range tokens {
		trimmed := strings.TrimSpace(tok)
		rank, err := domain.ParseRank(trimmed)
		if err != nil {
			return nil, fmt.Errorf("unknown rank %q", trimmed)
		}
		ranks = append(ranks, rank)
	}
	return ranks, nil
}

// parseSuggestLimit parses the `limit` query parameter as an integer. An
// empty param returns (0, nil) — application.Suggest treats <= 0 as "use
// the default limit". A non-numeric param is reported as an error; the
// numeric value (including 0 or negative) is passed through unvalidated,
// since application.Suggest already defaults/caps it.
func parseSuggestLimit(param string) (int, error) {
	if param == "" {
		return 0, nil
	}
	return strconv.Atoi(param)
}

// validSuggestMatchModes are the only accepted `match_mode` query values —
// "" and "name_start" are equivalent (both select the default), "anywhere"
// restores the pre-SP7 plain FTS5 prefix behavior. Anything else is 400
// INVALID_QUERY: an unrecognized mode silently falling back to the default
// would hide a caller's typo behind a behavior change they didn't ask for.
var validSuggestMatchModes = map[string]bool{
	"":           true,
	"name_start": true,
	"anywhere":   true,
}

// parseSuggestMatchMode validates the `match_mode` query parameter against
// validSuggestMatchModes, returning it unchanged when valid.
func parseSuggestMatchMode(param string) (string, error) {
	if !validSuggestMatchModes[param] {
		return "", fmt.Errorf("unknown match_mode %q", param)
	}
	return param, nil
}

// handleSuggest serves GET /v1/suggest?q=&area=&rank=&limit=&match_mode=,
// the frontend autosuggest endpoint, per spec §B.1. A missing/empty q, an
// unknown rank token, a non-numeric limit, or an unrecognized match_mode
// all report 400 INVALID_QUERY.
func handleSuggest(repo output.Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()

		ranks, err := parseSuggestRanks(query.Get("rank"))
		if err != nil {
			httperr.InvalidQueryError(w, err.Error())
			return
		}

		limit, err := parseSuggestLimit(query.Get("limit"))
		if err != nil {
			httperr.InvalidQueryError(w, "limit must be an integer")
			return
		}

		matchMode, err := parseSuggestMatchMode(query.Get("match_mode"))
		if err != nil {
			httperr.InvalidQueryError(w, err.Error())
			return
		}

		entryBackbone := query.Get("entry_backbone")
		targetSpace := query.Get("target_space")
		resp, err := application.Suggest(r.Context(), repo, application.SuggestRequest{
			Q:             query.Get("q"),
			Area:          query.Get("area"),
			Ranks:         ranks,
			Limit:         limit,
			EntryBackbone: entryBackbone,
			TargetSpace:   targetSpace,
			MatchMode:     matchMode,
		})
		if errors.Is(err, application.ErrEmptyQuery) {
			httperr.InvalidQueryError(w, "q query parameter is required")
			return
		}
		if errors.Is(err, application.ErrUnknownBackbone) {
			httperr.InvalidQueryError(w, "unknown entry_backbone "+strconv.Quote(entryBackbone))
			return
		}
		if errors.Is(err, application.ErrUnknownTargetSpace) {
			httperr.InvalidQueryError(w, "unknown target_space "+strconv.Quote(targetSpace))
			return
		}
		if err != nil {
			httperr.InternalError(w)
			return
		}

		dto := suggestResponseToDTO(resp)
		attachSuggestSec(r.Context(), repo, resp.Results, dto.Results)
		writeJSON(w, dto)
	}
}

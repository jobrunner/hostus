package httpx

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
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
	// MatchedName is the name that actually triggered the hit (canonical +
	// authorship + accepted/synonym role), which for a homonym-driven
	// synonym match is NOT the concept's own accepted name in canonical/
	// display — "Inula hirta L." and "Inula hirta Pollich" reach two
	// different concepts and are indistinguishable without it. A pointer
	// with omitempty so it is absent (not an empty object) when the
	// triggering name could not be determined.
	MatchedName *matchedNameDTO `json:"matched_name,omitempty"`
	// TargetSpaceName is the candidate's spelling in the requested
	// target_space, present only when one was requested AND this concept has
	// an entry there. Its ABSENCE is the useful half: it says this candidate
	// cannot be carried into that space, which is what a caller picking a
	// concept for downstream use needs to see while choosing.
	TargetSpaceName string `json:"target_space_name,omitempty"`
}

// matchedNameDTO is suggestItemDTO.MatchedName's nested object: the name
// behind the hit. Every field but nom_status is required on the wire — when
// there is no matched name at all the whole object is absent, so a present
// object always carries them (authorship and role can legitimately be empty
// strings for a name the backbone stored without them).
//
// nom_status/nom_status_judgement are named and split exactly as in
// synonymDetailDTO (see its doc comment for why the judgement is rendered
// ALWAYS while the raw cell is omitempty): both endpoints answer the same
// question about the same WCVP column, and a client should not have to learn
// two vocabularies for it.
//
// This pair is what makes the ranking legible. domain.RankSuggestions
// priority 2 demotes a candidate whose triggering name is disqualifying —
// "Inula hirta Pollich", a later illegitimate homonym, loses to "Inula hirta
// L." — and a demotion with no visible reason is indistinguishable from a
// broken sort. The row is never hidden: whoever meets the name in older
// literature must still be able to look up what became of it.
type matchedNameDTO struct {
	Canonical  string `json:"canonical"`
	Authorship string `json:"authorship"`
	Role       string `json:"role"`
	// NomStatus is the normalized raw WCVP nom_status cell, omitted when the
	// source recorded nothing — rendering "" would claim the cell held an
	// empty value.
	NomStatus string `json:"nom_status,omitempty"`
	// NomStatusJudgement is domain.ClassifyNomStatus' verdict, always
	// present: `absent` ("nothing recorded") is an answer, and leaving the
	// key out would let a client read it as "checked and found clean".
	NomStatusJudgement string `json:"nom_status_judgement"`
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

			MatchedName:     matchedNameToDTO(item.MatchedName),
			TargetSpaceName: item.TargetSpaceName,
		}
	}
	return suggestResponseDTO{
		BackboneVersions: resp.BackboneVersions,
		Results:          results,
	}
}

// matchedNameToDTO renders the triggering name, or nil when there is none
// to render. An empty Canonical is domain.MatchedName's documented "could
// not be determined" state (see its doc comment), and an object whose only
// content is empty strings would claim to answer "which name matched?"
// while saying nothing.
//
// That state also carries an empty NomStatusJudgement — "not computed",
// which is NOT domain.JudgementAbsent. Omitting the whole object resolves
// it: there is no name, so there is nothing to judge, and the response makes
// no claim either way. Once a name IS present, the judgement is mandatory,
// so an unset one normalizes to JudgementAbsent — for a named name, "nothing
// recorded" is precisely what that verdict says.
func matchedNameToDTO(n domain.MatchedName) *matchedNameDTO {
	if n.Canonical == "" {
		return nil
	}
	judgement := n.NomStatusJudgement
	if judgement == "" {
		judgement = domain.JudgementAbsent
	}
	return &matchedNameDTO{
		Canonical:          n.Canonical,
		Authorship:         n.Authorship,
		Role:               n.Role,
		NomStatus:          n.NomStatus,
		NomStatusJudgement: string(judgement),
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

// parseSuggestRequireTargetSpace parses the `require_target_space` query
// parameter as a boolean. A missing/empty parameter is false (no filter).
// Anything strconv.ParseBool rejects is an error, NOT a silent false: a
// filter that quietly fails to apply is the exact failure class this
// parameter was added to remove — the caller would read an unnarrowed list
// as a narrowed one.
func parseSuggestRequireTargetSpace(param string) (bool, error) {
	if param == "" {
		return false, nil
	}
	v, err := strconv.ParseBool(param)
	if err != nil {
		return false, fmt.Errorf("require_target_space must be a boolean, got %q", param)
	}
	return v, nil
}

// parseSuggestRequest turns the query string into the application request,
// or an error whose message IS the 400 INVALID_QUERY text (every parse
// failure on this endpoint is a caller error, so there is no second error
// class to distinguish here).
func parseSuggestRequest(query url.Values) (application.SuggestRequest, error) {
	ranks, err := parseSuggestRanks(query.Get("rank"))
	if err != nil {
		return application.SuggestRequest{}, err
	}
	limit, err := parseSuggestLimit(query.Get("limit"))
	if err != nil {
		return application.SuggestRequest{}, errors.New("limit must be an integer")
	}
	matchMode, err := parseSuggestMatchMode(query.Get("match_mode"))
	if err != nil {
		return application.SuggestRequest{}, err
	}
	requireTargetSpace, err := parseSuggestRequireTargetSpace(query.Get("require_target_space"))
	if err != nil {
		return application.SuggestRequest{}, err
	}
	targetSpace := query.Get("target_space")
	// The combination, not either half, is the mistake: without a space there
	// is nothing to require an entry in, and silently ignoring the flag would
	// hand back an unfiltered list to a caller who asked for a filtered one.
	if requireTargetSpace && targetSpace == "" {
		return application.SuggestRequest{}, errors.New("require_target_space needs a target_space to require an entry in")
	}
	return application.SuggestRequest{
		Q:                  query.Get("q"),
		Area:               query.Get("area"),
		Ranks:              ranks,
		Limit:              limit,
		EntryBackbone:      query.Get("entry_backbone"),
		TargetSpace:        targetSpace,
		RequireTargetSpace: requireTargetSpace,
		MatchMode:          matchMode,
	}, nil
}

// suggestInvalidQueryMessage maps application.Suggest's caller-error
// sentinels to their 400 INVALID_QUERY message, echoing the offending value
// from req. The bool is false for anything else — an infrastructure failure,
// which the handler turns into a 500.
func suggestInvalidQueryMessage(err error, req application.SuggestRequest) (string, bool) {
	if errors.Is(err, application.ErrEmptyQuery) {
		return "q query parameter is required", true
	}
	if errors.Is(err, application.ErrUnknownBackbone) {
		return "unknown entry_backbone " + strconv.Quote(req.EntryBackbone), true
	}
	if errors.Is(err, application.ErrUnknownTargetSpace) {
		return "unknown target_space " + strconv.Quote(req.TargetSpace), true
	}
	if errors.Is(err, application.ErrUnknownArea) {
		// "not available in this index", never "unknown": the value may be a
		// real WGSRPD code this index just carries no data for (a bundle
		// scope), and a first line reading "unknown" sends the reader looking
		// for a typo instead of at the scope. The sentence must survive being
		// cut after its first clause — in a log line or an error toast, that
		// is all anyone sees.
		return "area " + strconv.Quote(req.Area) + " is not available in this index — GET /v1/areas lists the areas it carries data for", true
	}
	return "", false
}

// handleSuggest serves
// GET /v1/suggest?q=&area=&rank=&limit=&match_mode=&require_target_space=,
// the frontend autosuggest endpoint, per spec §B.1. A missing/empty q, an
// unknown rank token, a non-numeric limit, an unrecognized match_mode, a
// non-boolean or space-less require_target_space, or an area the index does
// not know all report 400 INVALID_QUERY.
func handleSuggest(repo output.Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		req, err := parseSuggestRequest(r.URL.Query())
		if err != nil {
			httperr.InvalidQueryError(w, err.Error())
			return
		}

		resp, err := application.Suggest(r.Context(), repo, req)
		if err != nil {
			if msg, ok := suggestInvalidQueryMessage(err, req); ok {
				httperr.InvalidQueryError(w, msg)
				return
			}
			httperr.InternalError(w)
			return
		}

		dto := suggestResponseToDTO(resp)
		attachSuggestSec(r.Context(), repo, resp.Results, dto.Results)
		writeJSON(w, dto)
	}
}

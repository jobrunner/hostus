package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/jobrunner/hostus/internal/cache"
	"github.com/jobrunner/hostus/internal/gbif"
	"github.com/jobrunner/hostus/internal/httperr"
	"github.com/jobrunner/hostus/internal/middleware"
	"github.com/jobrunner/hostus/internal/taxonomy"
)

const (
	defaultLimit = 20
	maxLimit     = 100
	minQueryLen  = 3
)

type SuggestHandler struct {
	gbifClient  *gbif.Client
	cache       *cache.Cache
	loadShedder *middleware.LoadShedder
	logger      *slog.Logger
}

func NewSuggestHandler(gbifClient *gbif.Client, cache *cache.Cache, loadShedder *middleware.LoadShedder, logger *slog.Logger) *SuggestHandler {
	return &SuggestHandler{
		gbifClient:  gbifClient,
		cache:       cache,
		loadShedder: loadShedder,
		logger:      logger,
	}
}

// @Summary      Search taxa for autosuggest
// @Description  Returns vascular plant taxa matching the query, grouped by accepted name with synonyms
// @Tags         taxa
// @Accept       json
// @Produce      json
// @Param        q      query    string  true   "Search query (min 3 characters)"
// @Param        limit  query    int     false  "Maximum number of results (default 20, max 100)"
// @Success      200    {array}  taxonomy.TaxonSuggestion
// @Failure      400    {object} ErrorResponse
// @Failure      429    {object} ErrorResponse
// @Failure      502    {object} ErrorResponse
// @Failure      503    {object} ErrorResponse
// @Failure      504    {object} ErrorResponse
// @Router       /api/v1/taxa/suggest [get]
func (h *SuggestHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if len(query) < minQueryLen {
		httperr.InvalidQueryError(w, "Query must be at least 3 characters")
		return
	}

	limit := defaultLimit
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		var err error
		limit, err = strconv.Atoi(limitStr)
		if err != nil || limit < 1 {
			httperr.InvalidQueryError(w, "Invalid limit parameter")
			return
		}
		if limit > maxLimit {
			limit = maxLimit
		}
	}

	cacheKey := query + ":" + strconv.Itoa(limit)
	if cached, ok := h.cache.Get(cacheKey); ok {
		h.writeJSON(w, cached)
		return
	}

	// Fetch more results to account for grouping
	fetchLimit := limit * 3
	if fetchLimit > 300 {
		fetchLimit = 300
	}

	results, err := h.gbifClient.Search(r.Context(), gbif.SearchParams{
		Query: query,
		Limit: fetchLimit,
	})
	if err != nil {
		h.handleUpstreamError(w, err)
		return
	}

	h.loadShedder.RecordSuccess()

	suggestions := taxonomy.MapAndGroup(results.Results)

	// Limit the results
	if len(suggestions) > limit {
		suggestions = suggestions[:limit]
	}

	h.cache.Set(cacheKey, suggestions)
	h.writeJSON(w, suggestions)
}

func (h *SuggestHandler) handleUpstreamError(w http.ResponseWriter, err error) {
	h.loadShedder.RecordError()

	if errors.Is(err, context.DeadlineExceeded) {
		httperr.GBIFTimeoutError(w)
		return
	}

	h.logger.Error("gbif request failed", slog.String("error", err.Error()))
	httperr.GBIFUnavailableError(w)
}

func (h *SuggestHandler) writeJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(data); err != nil {
		h.logger.Error("failed to encode response", slog.String("error", err.Error()))
	}
}

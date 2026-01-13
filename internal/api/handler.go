package api

import (
	"log/slog"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/jobrunner/hostus/internal/cache"
	"github.com/jobrunner/hostus/internal/gbif"
	"github.com/jobrunner/hostus/internal/middleware"
)

type HandlerConfig struct {
	GBIFClient  *gbif.Client
	Cache       *cache.Cache
	LoadShedder *middleware.LoadShedder
	Logger      *slog.Logger
}

type Handler struct {
	suggestHandler *SuggestHandler
	config         HandlerConfig
}

func NewHandler(cfg HandlerConfig) *Handler {
	return &Handler{
		suggestHandler: NewSuggestHandler(cfg.GBIFClient, cfg.Cache, cfg.LoadShedder, cfg.Logger),
		config:         cfg,
	}
}

func NewRouter(h *Handler) *mux.Router {
	r := mux.NewRouter()

	// API routes
	api := r.PathPrefix("/api/v1").Subrouter()
	api.Handle("/taxa/suggest", h.suggestHandler).Methods(http.MethodGet)

	// OpenAPI spec
	r.HandleFunc("/openapi", ServeOpenAPI).Methods(http.MethodGet)
	r.HandleFunc("/openapi.json", ServeOpenAPI).Methods(http.MethodGet)

	// Metrics
	r.Handle("/metrics", promhttp.Handler()).Methods(http.MethodGet)

	// Health check
	r.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}).Methods(http.MethodGet)

	return r
}

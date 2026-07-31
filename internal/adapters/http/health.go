package httpx

import (
	"net/http"

	"github.com/jobrunner/hostus/internal/ports/output"
)

// handleHealthLive answers the liveness probe. It never depends on
// downstream state: as long as the process can serve HTTP, it is alive.
func handleHealthLive(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
}

// handleHealthReady answers the readiness probe, gated on repo: nil (no
// SQLite database configured, or it failed to open — see internal/app.New)
// always reports not-ready, and an opened-but-empty database (no backbone
// ever ingested) also reports not-ready, since there is nothing yet worth
// serving to a frontend.
func handleHealthReady(repo output.Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if repo == nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		versions, err := repo.BackboneVersions(r.Context())
		if err != nil || len(versions) == 0 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}
}

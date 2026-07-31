package httpx

import "net/http"

// handleHealthLive answers the liveness probe. It never depends on
// downstream state: as long as the process can serve HTTP, it is alive.
func handleHealthLive(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
}

// handleHealthReady answers the readiness probe. In this skeleton there is
// nothing to check yet (no DB, no upstream dependency wired), so readiness
// mirrors liveness. SP1 will add real checks (e.g. GBIF reachability, cache
// warm-up) once those dependencies exist.
func handleHealthReady(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
}

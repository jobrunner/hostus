package httpx

import (
	"net/http"

	"github.com/jobrunner/hostus/internal/httperr"
	"github.com/jobrunner/hostus/internal/ports/output"
)

// secListResponseDTO is the GET /v1/sec response envelope: every ingested
// sec. reference space as {id, title}. It exists so a client — notably the
// test console's target_space / entry_sec fields — can offer a picker built
// from real ids instead of a free-text guess, where a mistyped space is
// otherwise indistinguishable from an empty result (see the SP8 note in
// docs/explanation/known-gaps.md, now closed).
type secListResponseDTO struct {
	SecReferences []secReferenceDTO `json:"sec_references"`
}

// handleSec serves GET /v1/sec, listing the sec_reference lookup table in id
// order (the order repo.SecReferences already guarantees). The list is a
// plain read with no parameters — an unknown-space question is answered by
// its ABSENCE from this list, so there is no per-id error path here.
func handleSec(repo output.Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		refs, err := repo.SecReferences(r.Context())
		if err != nil {
			httperr.InternalError(w)
			return
		}
		// len (not append from nil) so an empty index marshals to [] rather
		// than null — the wire contract is "a list, possibly empty".
		dtos := make([]secReferenceDTO, len(refs))
		for i, s := range refs {
			dtos[i] = secReferenceDTO{ID: s.ID, Title: s.Title}
		}
		writeJSON(w, secListResponseDTO{SecReferences: dtos})
	}
}

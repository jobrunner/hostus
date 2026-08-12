package httpx

import (
	"net/http"

	"github.com/jobrunner/hostus/internal/httperr"
	"github.com/jobrunner/hostus/internal/ports/output"
)

// areaDTO is one distribution area on the wire: its code, human-readable name
// (empty when the source carried none) and scheme.
type areaDTO struct {
	Code   string `json:"code"`
	Name   string `json:"name,omitempty"`
	Scheme string `json:"scheme"`
}

// areaListResponseDTO is the GET /v1/areas envelope: every distribution area
// that carries data, each with its name where known. It lets a client offer a
// "Germany (GER)" picker instead of a bare WGSRPD code — the codes are WGSRPD
// (not ISO) and rarely remembered.
type areaListResponseDTO struct {
	Areas []areaDTO `json:"areas"`
}

// handleAreas serves GET /v1/areas, listing the distribution areas with data
// (Repository.Areas) in (scheme, code) order. No parameters; the only error
// path is a repository failure.
func handleAreas(repo output.Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		areas, err := repo.Areas(r.Context())
		if err != nil {
			httperr.InternalError(w)
			return
		}
		dtos := make([]areaDTO, len(areas))
		for i, a := range areas {
			dtos[i] = areaDTO{Code: a.Code, Name: a.Name, Scheme: a.Scheme}
		}
		writeJSON(w, areaListResponseDTO{Areas: dtos})
	}
}

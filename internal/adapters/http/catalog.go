package httpx

import (
	"net/http"

	"github.com/jobrunner/hostus/internal/httperr"
	"github.com/jobrunner/hostus/internal/ports/output"
)

// The catalog endpoints answer "what can I ask for in this index?". They exist
// for the same reason /v1/sec and /v1/areas do: a picker has to be filled
// BEFORE the first query, so a client cannot read the choices off a result
// envelope. Which backbones and name spaces an index holds is a per-deployment
// fact — both are optional manifest entries — so a client that hard-codes them
// is wrong on every index but the one it was written against.

type backboneDTO struct {
	ID      string `json:"id"`
	Version string `json:"version"`
}

type backboneListResponseDTO struct {
	Backbones []backboneDTO `json:"backbones"`
}

type spaceDTO struct {
	ID      string `json:"id"`
	Version string `json:"version"`
	// Redistribution repeats the ingest-time gate ("allowed"|"restricted"|
	// "unknown"). It is advisory here: it does not restrict querying, only
	// what ExportBundle may ship, and a console showing it saves a surprise
	// at export time.
	Redistribution string `json:"redistribution"`
}

type spaceListResponseDTO struct {
	Spaces []spaceDTO `json:"spaces"`
}

// handleBackbones serves GET /v1/backbones: the ingested backbones and their
// pinned versions, i.e. exactly the values entry_backbone accepts.
func handleBackbones(repo output.Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		versions, err := repo.BackboneVersions(r.Context())
		if err != nil {
			httperr.InternalError(w)
			return
		}
		// len (not append from nil) so an empty index marshals to [] rather
		// than null — the wire contract is "a list, possibly empty".
		dtos := make([]backboneDTO, len(versions))
		for i, v := range versions {
			dtos[i] = backboneDTO{ID: v.ID, Version: v.Version}
		}
		writeJSON(w, backboneListResponseDTO{Backbones: dtos})
	}
}

// handleSpaces serves GET /v1/spaces: the ingested name spaces, i.e. exactly
// the values target_space accepts on /v1/match and /v1/suggest.
func handleSpaces(repo output.Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		spaces, err := repo.NameSpaces(r.Context())
		if err != nil {
			httperr.InternalError(w)
			return
		}
		dtos := make([]spaceDTO, len(spaces))
		for i, s := range spaces {
			dtos[i] = spaceDTO{ID: s.ID, Version: s.Version, Redistribution: string(s.Redistribution)}
		}
		writeJSON(w, spaceListResponseDTO{Spaces: dtos})
	}
}

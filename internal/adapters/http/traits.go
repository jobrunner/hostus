package httpx

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/gorilla/mux"

	"github.com/jobrunner/hostus/internal/domain"
	"github.com/jobrunner/hostus/internal/httperr"
	"github.com/jobrunner/hostus/internal/ports/output"
)

// scaleDTO is the (min, max, normalized) triple domain.ScaleFor reports for
// one (vocab, dim) combination, per spec §UC1's "store the pointer, not the
// number" guard: it lets a client tell that a EIVE value of 4.2 is not
// comparable to a Tichý value of 4.2 even though both are plain float64s.
type scaleDTO struct {
	Min        float64 `json:"min"`
	Max        float64 `json:"max"`
	Normalized bool    `json:"normalized"`
}

// traitValueDTO is one indicator value within a traitSetDTO.
//
// NicheWidth/NSystems are *float64/*int, not float64/int: EIVE populates
// both, Tichý/Midolo populate neither (domain.TraitValue's doc comment). A
// nil pointer must render as an OMITTED field (omitempty), never as a
// silently-invented 0 — that would fabricate data the ingested vocabulary
// never asserted.
//
// Resolution follows the same rule for the same reason: it is OMITTED for
// the ordinary case (an exact canonical match between the vocabulary's
// taxon name and the concept's name), and present only when a deterministic
// normalisation rule was needed to reach this concept
// (domain.TraitValue.Resolution). Two of those rules —
// `aggregate_to_nominate` and `autonym` — equate circumscriptions that are
// not identical, so a client that renders an aggregate's collective mean as
// if it were measured on the nominate species would be asserting something
// the vocabulary never said. Absence of the field is the positive statement
// "this value matched exactly"; it is never a stand-in for "unknown".
//
// Scale is rendered PER VALUE, not once per traitSetDTO: Tichý's own
// per-dimension ranges genuinely differ (T is 1-12, L is 1-9 —
// domain.ScaleFor's doc comment), so one scale field shared by an entire
// TraitSet would misrepresent every dimension but the one it happened to
// match. Per-value is the only placement that is never a lie.
type traitValueDTO struct {
	Dim        string   `json:"dim"`
	Value      float64  `json:"value"`
	NicheWidth *float64 `json:"niche_width,omitempty"`
	NSystems   *int     `json:"n_systems,omitempty"`
	Resolution string   `json:"resolution,omitempty"`
	Scale      scaleDTO `json:"scale"`
}

// traitSetDTO is one vocabulary's worth of trait data for a concept.
//
// Taxonomy is omitted (omitempty) when empty rather than rendered as an
// empty string: output.Repository.Traits LEFT JOINs trait_vocabulary, so a
// trait_value row lacking a matching metadata row yields Taxonomy == "" —
// rendering that as `"taxonomy": ""` would assert a (false) namespace of
// "no namespace" instead of honestly saying "unknown".
type traitSetDTO struct {
	Vocab        string          `json:"vocab"`
	VocabVersion string          `json:"vocab_version"`
	Taxonomy     string          `json:"taxonomy,omitempty"`
	Values       []traitValueDTO `json:"values"`
}

// traitsResponseDTO is the GET /v1/concept/{id}/traits response envelope.
// Traits is never merged across vocabularies (domain.TraitSet's doc
// comment): each element of Traits is exactly one (vocab, version) group,
// even when two sets share a concept.
type traitsResponseDTO struct {
	ConceptID string        `json:"concept_id"`
	Traits    []traitSetDTO `json:"traits"`
}

// traitSetsToDTO renders Repository.Traits' result as the wire shape.
func traitSetsToDTO(conceptID string, sets []domain.TraitSet) traitsResponseDTO {
	traits := make([]traitSetDTO, len(sets))
	for i, set := range sets {
		values := make([]traitValueDTO, len(set.Values))
		for j, v := range set.Values {
			min, max, normalized := domain.ScaleFor(v.Vocab, v.Dim)
			values[j] = traitValueDTO{
				Dim:        string(v.Dim),
				Value:      v.Value,
				NicheWidth: v.NicheWidth,
				NSystems:   v.NSystems,
				Resolution: v.Resolution,
				Scale:      scaleDTO{Min: min, Max: max, Normalized: normalized},
			}
		}
		traits[i] = traitSetDTO{
			Vocab:        string(set.Vocab),
			VocabVersion: set.VocabVersion,
			Taxonomy:     set.Taxonomy,
			Values:       values,
		}
	}
	return traitsResponseDTO{ConceptID: conceptID, Traits: traits}
}

// parseTraitVocabs splits the comma-separated `vocab` query parameter into
// domain.TraitVocab values via domain.ParseTraitVocab. An empty param
// returns (nil, nil) — no vocab filter, per output.Repository.Traits'
// documented "nil or empty means every vocabulary" convention. Any
// unrecognized token is reported as a fresh error naming just the
// offending token, mirroring parseSuggestRanks' rationale for not
// propagating domain.ParseTraitVocab's own (already-prefixed) error text
// verbatim into the 400 message.
func parseTraitVocabs(param string) ([]domain.TraitVocab, error) {
	if param == "" {
		return nil, nil
	}
	tokens := strings.Split(param, ",")
	vocabs := make([]domain.TraitVocab, 0, len(tokens))
	for _, tok := range tokens {
		trimmed := strings.TrimSpace(tok)
		vocab, err := domain.ParseTraitVocab(trimmed)
		if err != nil {
			return nil, errUnknownVocab{token: trimmed}
		}
		vocabs = append(vocabs, vocab)
	}
	return vocabs, nil
}

// errUnknownVocab reports an unrecognized `vocab` query token, per
// parseTraitVocabs' doc comment.
type errUnknownVocab struct{ token string }

func (e errUnknownVocab) Error() string {
	return fmt.Sprintf("unknown vocab %q", e.token)
}

// handleTraits serves GET /v1/concept/{id}/traits?vocab=eive,tichy2023, per
// spec §UC1. An unknown concept id reports 404 NOT_FOUND; an unrecognized
// vocab token reports 400 INVALID_QUERY; a concept that exists but has no
// ingested trait values reports 200 with an empty `traits` array — it is
// not a 404, since the concept itself is real and known.
func handleTraits(repo output.Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := mux.Vars(r)["id"]

		vocabs, err := parseTraitVocabs(r.URL.Query().Get("vocab"))
		if err != nil {
			httperr.InvalidQueryError(w, err.Error())
			return
		}

		sets, err := repo.Traits(r.Context(), id, vocabs)
		if errors.Is(err, domain.ErrNotFound) {
			httperr.Write(w, http.StatusNotFound, httperr.NotFound, "concept not found")
			return
		}
		if err != nil {
			httperr.InternalError(w)
			return
		}

		writeJSON(w, traitSetsToDTO(id, sets))
	}
}

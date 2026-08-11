package httpx

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/jobrunner/hostus/internal/application"
	"github.com/jobrunner/hostus/internal/domain"
	"github.com/jobrunner/hostus/internal/httperr"
	"github.com/jobrunner/hostus/internal/ports/output"
)

// Wire values for TranslateResult's outcome. They are rendered as an
// explicit enum rather than being inferred from an empty array, so a client
// cannot read "no relation recorded" as a transport hiccup or a truncated
// list — and so the empty case is impossible to overlook.
const (
	translateResultTranslated = "translated"
	translateResultNoRelation = "no_relation_recorded"
)

// Wire values for a candidate's direction, spelling out which of the two
// possible statements the client got. "A includes B" and "B included_in A"
// are different claims; the direction field plus the verbatim statement
// block is what keeps them apart.
const (
	directionSourceToTarget = "source_to_target"
	directionTargetToSource = "target_to_source"
)

// secReferenceDTO is one sec. reference space on the wire.
type secReferenceDTO struct {
	ID    string `json:"id"`
	Title string `json:"title,omitempty"`
}

// translateRequestDTO is the POST /v1/translate request body. Exactly one
// of ConceptID/Verbatim must be set.
type translateRequestDTO struct {
	ConceptID string `json:"concept_id,omitempty"`
	Verbatim  string `json:"verbatim,omitempty"`
	// EntryBackbone/EntrySec (SP5) narrow the Verbatim RESOLUTION to one
	// backbone / sec. reference space, so an ambiguous name resolves to a
	// single source concept. Ignored on the concept_id entry.
	EntryBackbone string `json:"entry_backbone,omitempty"`
	EntrySec      string `json:"entry_sec,omitempty"`
	// TargetSpace is the id of the sec. reference space to translate into.
	TargetSpace string `json:"target_space"`
	// MaxHops must be 1 (or omitted) — see application.MaxTranslateHops.
	MaxHops int `json:"max_hops,omitempty"`
	// IncludeNameCandidates opts into the explicitly NON-relational,
	// review-only block of same-name concepts in the target space.
	IncludeNameCandidates bool `json:"include_name_candidates,omitempty"`
}

// translateEntryDTO records how the source concept was reached.
type translateEntryDTO struct {
	Mode       string  `json:"mode"`
	Verbatim   string  `json:"verbatim,omitempty"`
	MatchType  string  `json:"match_type,omitempty"`
	Confidence float64 `json:"confidence,omitempty"`
	Note       string  `json:"note,omitempty"`
}

// relationStatementDTO is the stored concept_relation row, verbatim: the
// ordered pair and the relation the source asserts between them. It is
// always present, so a client can reconstruct the source's claim without
// trusting any derived field.
type relationStatementDTO struct {
	From     string `json:"from"`
	Relation string `json:"relation"`
	To       string `json:"to"`
}

// translateCandidateDTO is one concept in the target space reached by one
// stored relation.
//
// The field naming is load-bearing. There is deliberately NO field called
// plain "relation": the stored triple's relation is direction-dependent,
// and CDM only ever emits the "Includes" direction, so incoming edges are
// common — a client writing `if c.relation == "includes"` on an incoming
// edge would read the claim exactly backwards. The invitingly-short name is
// therefore not offered at all:
//
//   - StoredRelation is the row as stored, and says so in its name. It is
//     fully redundant with Statement.Relation, kept only so the stored
//     vocabulary is readable without descending into Statement.
//   - RelationFromSource is the DIRECTION-SAFE reading (source -> candidate)
//     and is always present, explicitly null when no sound inverse exists.
//     A *string rather than omitempty for the same reason IsEquality is not
//     omitempty: an absent field reads as "unknown", an explicit null is a
//     statement — and absence would otherwise coincide exactly with the
//     pro-parte case where a lazy client most needs to be stopped.
//   - HasInverse makes that null checkable without null-handling.
//
// IsEquality is the ONLY field a consumer may read as "the same taxon".
type translateCandidateDTO struct {
	ConceptID          string               `json:"concept_id"`
	Canonical          string               `json:"canonical"`
	Authorship         string               `json:"authorship,omitempty"`
	Rank               string               `json:"rank"`
	Status             string               `json:"status"`
	Sec                secReferenceDTO      `json:"sec"`
	StoredRelation     string               `json:"stored_relation"`
	RelationFromSource *string              `json:"relation_from_source"`
	HasInverse         bool                 `json:"has_inverse"`
	Direction          string               `json:"direction"`
	Statement          relationStatementDTO `json:"statement"`
	IsEquality         bool                 `json:"is_equality"`
	Hops               int                  `json:"hops"`
	Source             string               `json:"source,omitempty"`
	Note               string               `json:"note,omitempty"`
}

// translateNameCandidateDTO is a same-name concept in the target space. It
// carries no relation field at all — there is no relation — and always
// requires review. It lives under its own top-level key so that a client
// iterating `candidates` can never encounter one.
type translateNameCandidateDTO struct {
	ConceptID      string          `json:"concept_id"`
	Canonical      string          `json:"canonical"`
	Authorship     string          `json:"authorship,omitempty"`
	Sec            secReferenceDTO `json:"sec"`
	RequiresReview bool            `json:"requires_review"`
}

// translateSourceDTO identifies the concept that was translated.
type translateSourceDTO struct {
	ConceptID  string          `json:"concept_id"`
	Canonical  string          `json:"canonical"`
	Authorship string          `json:"authorship,omitempty"`
	Rank       string          `json:"rank"`
	Sec        secReferenceDTO `json:"sec"`
}

// translateResponseDTO is the POST /v1/translate response envelope — the
// derived sec_inference structure (architecture spec §4.3: derived, never a
// persisted table).
type translateResponseDTO struct {
	Source      translateSourceDTO `json:"source"`
	TargetSpace secReferenceDTO    `json:"target_space"`
	Entry       translateEntryDTO  `json:"entry"`
	// MaxHops is echoed on every response so the one-hop boundary is stated
	// rather than assumed.
	MaxHops int    `json:"max_hops"`
	Result  string `json:"result"`
	// Candidates is never omitempty: the empty array IS the answer in the
	// no-relation case, and omitting it would make that answer invisible.
	Candidates       []translateCandidateDTO     `json:"candidates"`
	NameCandidates   []translateNameCandidateDTO `json:"unrelated_name_candidates,omitempty"`
	RequiresReview   bool                        `json:"requires_review"`
	Note             string                      `json:"note,omitempty"`
	BackboneVersions map[string]string           `json:"backbone_versions"`
}

// handleTranslate serves POST /v1/translate: concept translation between
// sec. reference spaces (UC6).
//
// Status mapping: malformed body / contradictory request / more than one
// hop -> 400 INVALID_QUERY; unknown concept id or unknown target space ->
// 404 NOT_FOUND; a verbatim name that cannot be pinned to one concept ->
// 422 UNRESOLVABLE. "No relation recorded" is NOT an error — it is a 200
// with an empty candidates array and an explanatory note, because it is a
// truthful answer about the data, not a failure of the request.
func handleTranslate(repo output.Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body translateRequestDTO
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			httperr.InvalidQueryError(w, "malformed request body")
			return
		}

		res, err := application.Translate(r.Context(), repo, application.TranslateRequest{
			ConceptID:             body.ConceptID,
			Verbatim:              body.Verbatim,
			Filter:                application.MatchFilter{Backbone: body.EntryBackbone, Sec: body.EntrySec},
			TargetSec:             body.TargetSpace,
			MaxHops:               body.MaxHops,
			IncludeNameCandidates: body.IncludeNameCandidates,
		})
		if errors.Is(err, application.ErrUnknownBackbone) {
			httperr.InvalidQueryError(w, "unknown entry_backbone "+strconv.Quote(body.EntryBackbone))
			return
		}
		if errors.Is(err, application.ErrUnknownSec) {
			httperr.InvalidQueryError(w, "unknown entry_sec "+strconv.Quote(body.EntrySec))
			return
		}
		if err != nil {
			writeTranslateError(w, err)
			return
		}

		versions, err := repo.BackboneVersions(r.Context())
		if err != nil {
			httperr.InternalError(w)
			return
		}
		backboneVersions := make(map[string]string, len(versions))
		for _, v := range versions {
			backboneVersions[v.ID] = v.Version
		}

		dto := translateToDTO(res)
		dto.BackboneVersions = backboneVersions
		writeJSON(w, dto)
	}
}

// writeTranslateError maps application.Translate's named failures onto the
// error contract. Every branch is a distinct client-visible cause: a typo'd
// target space must not look like an unresolvable name, and neither may
// look like an empty answer.
func writeTranslateError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, application.ErrInvalidTranslateRequest):
		httperr.InvalidQueryError(w, "concept_id or verbatim (exactly one) and target_space are required")
	case errors.Is(err, application.ErrMultiHopUnsupported):
		httperr.InvalidQueryError(w, "translate follows exactly one relation hop; max_hops must be 1")
	case errors.Is(err, application.ErrUnresolvableName):
		httperr.Write(w, http.StatusUnprocessableEntity, httperr.Unresolvable, "verbatim name cannot be resolved to a single concept")
	case errors.Is(err, domain.ErrNotFound):
		httperr.Write(w, http.StatusNotFound, httperr.NotFound, "concept or sec. reference space not found")
	default:
		httperr.InternalError(w)
	}
}

// translateToDTO renders the derived result. It adds no judgement of its
// own: every honesty-critical value (is_equality, the relation vocabulary,
// the absent inverse) comes from the application/domain layer, and this
// function only names the wire fields.
func translateToDTO(res application.TranslateResult) translateResponseDTO {
	out := translateResponseDTO{
		Source: translateSourceDTO{
			ConceptID:  res.Source.ID,
			Canonical:  res.Source.AcceptedName.Canonical,
			Authorship: res.Source.AcceptedName.Authorship,
			Rank:       string(res.Source.Rank),
			Sec:        secToDTO(res.SourceSec),
		},
		TargetSpace: secToDTO(res.TargetSec),
		Entry: translateEntryDTO{
			Mode:       res.Entry.Mode,
			Verbatim:   res.Entry.Verbatim,
			MatchType:  string(res.Entry.MatchType),
			Confidence: res.Entry.Confidence,
			Note:       res.Entry.Note,
		},
		MaxHops:        res.MaxHops,
		Result:         translateResultNoRelation,
		Candidates:     make([]translateCandidateDTO, 0, len(res.Candidates)),
		RequiresReview: res.RequiresReview,
		Note:           res.Note,
	}
	if res.HasRelation() {
		out.Result = translateResultTranslated
	}

	for _, c := range res.Candidates {
		direction := directionTargetToSource
		if c.Outgoing {
			direction = directionSourceToTarget
		}
		// An empty RelationFromSource means "no sound inverse", which goes
		// on the wire as an explicit null rather than a missing key.
		var fromSource *string
		if c.RelationFromSource != "" {
			s := string(c.RelationFromSource)
			fromSource = &s
		}
		out.Candidates = append(out.Candidates, translateCandidateDTO{
			ConceptID:          c.Concept.ID,
			Canonical:          c.Concept.AcceptedName.Canonical,
			Authorship:         c.Concept.AcceptedName.Authorship,
			Rank:               string(c.Concept.Rank),
			Status:             string(c.Concept.Status),
			Sec:                secToDTO(c.Sec),
			StoredRelation:     string(c.Relation),
			RelationFromSource: fromSource,
			HasInverse:         fromSource != nil,
			Direction:          direction,
			Statement:          relationStatementDTO{From: c.StatementFrom, Relation: string(c.Relation), To: c.StatementTo},
			IsEquality:         c.IsEquality,
			Hops:               c.Hops,
			Source:             c.Source,
			Note:               c.Note,
		})
	}

	for _, n := range res.NameCandidates {
		out.NameCandidates = append(out.NameCandidates, translateNameCandidateDTO{
			ConceptID:      n.Concept.ID,
			Canonical:      n.Concept.AcceptedName.Canonical,
			Authorship:     n.Concept.AcceptedName.Authorship,
			Sec:            secToDTO(n.Sec),
			RequiresReview: n.RequiresReview,
		})
	}
	return out
}

func secToDTO(s domain.SecReference) secReferenceDTO {
	return secReferenceDTO{ID: s.ID, Title: s.Title}
}

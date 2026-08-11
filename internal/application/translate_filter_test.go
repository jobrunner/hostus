package application_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jobrunner/hostus/internal/application"
	"github.com/jobrunner/hostus/internal/domain"
	"github.com/jobrunner/hostus/internal/ports/output"
)

// TestTranslate_EntrySecDisambiguatesVerbatim is the UC6 lever: "Abies alba"
// exists in two sec spaces, so the verbatim entry is ambiguous and translates
// nothing; entry_sec=<source space> narrows it to one concept, which then
// translates over its real relation.
func TestTranslate_EntrySecDisambiguatesVerbatim(t *testing.T) {
	repo := translateRepo()
	repo.edges["cdm:concept:roth|"+secWH98] = []output.ConceptRelationEdge{edgeTo(domain.RelationCongruent, true, repo)}

	_, err := application.Translate(context.Background(), repo, application.TranslateRequest{Verbatim: "Abies alba", TargetSec: secWH98})
	if !errors.Is(err, application.ErrUnresolvableName) {
		t.Fatalf("no filter: err = %v, want ErrUnresolvableName (ambiguous across sec spaces)", err)
	}

	res := translate(t, repo, application.TranslateRequest{
		Verbatim: "Abies alba", TargetSec: secWH98,
		Filter: application.MatchFilter{Sec: secRothmaler},
	})
	if res.Entry.Mode != application.EntryModeVerbatim {
		t.Errorf("Entry.Mode = %q, want verbatim", res.Entry.Mode)
	}
	if len(res.Candidates) != 1 || res.Candidates[0].Relation != domain.RelationCongruent {
		t.Errorf("Candidates = %+v, want exactly one congruent candidate", res.Candidates)
	}
}

// TestTranslate_ConceptIDIgnoresFilter pins that on the concept_id entry the
// filter is not even validated — an id resolves unambiguously on its own.
func TestTranslate_ConceptIDIgnoresFilter(t *testing.T) {
	repo := translateRepo()
	repo.edges["cdm:concept:roth|"+secWH98] = []output.ConceptRelationEdge{edgeTo(domain.RelationCongruent, true, repo)}

	res := translate(t, repo, application.TranslateRequest{
		ConceptID: "cdm:concept:roth", TargetSec: secWH98,
		Filter: application.MatchFilter{Sec: "does-not-exist"},
	})
	if len(res.Candidates) != 1 {
		t.Fatalf("concept_id + bogus filter: got %d candidates, want 1 (filter must be ignored)", len(res.Candidates))
	}
}

// TestTranslate_UnknownEntrySecRejected pins that an unknown entry_sec on the
// verbatim path is ErrUnknownSec (HTTP renders 400), not a silent miss.
func TestTranslate_UnknownEntrySecRejected(t *testing.T) {
	repo := translateRepo()
	_, err := application.Translate(context.Background(), repo, application.TranslateRequest{
		Verbatim: "Abies alba", TargetSec: secWH98,
		Filter: application.MatchFilter{Sec: "nope"},
	})
	if !errors.Is(err, application.ErrUnknownSec) {
		t.Fatalf("err = %v, want ErrUnknownSec", err)
	}
}

package application_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jobrunner/hostus/internal/application"
	"github.com/jobrunner/hostus/internal/domain"
	"github.com/jobrunner/hostus/internal/ports/output"
)

// TestSuggest_UnknownEntryBackboneIsRejected: naming a backbone that is not
// ingested is a caller error, not an empty page. An empty result would read as
// "no such plant" and hide the typo.
func TestSuggest_UnknownEntryBackboneIsRejected(t *testing.T) {
	repo := &suggestBackboneRepo{}

	_, err := application.Suggest(context.Background(), repo, application.SuggestRequest{
		Q: "coryn", EntryBackbone: "bogus",
	})
	if !errors.Is(err, application.ErrUnknownBackbone) {
		t.Fatalf("err = %v, want ErrUnknownBackbone", err)
	}
	if repo.suggestCalls != 0 {
		t.Errorf("repo.Suggest called %d times, want 0 — validation must reject before querying",
			repo.suggestCalls)
	}
}

// TestSuggest_KnownEntryBackboneReachesTheRepository: the validated value is
// handed to the repository as SuggestOpts.Backbone, where it filters inside the
// query (ahead of the limit).
func TestSuggest_KnownEntryBackboneReachesTheRepository(t *testing.T) {
	repo := &suggestBackboneRepo{}

	if _, err := application.Suggest(context.Background(), repo, application.SuggestRequest{
		Q: "coryn", EntryBackbone: "wcvp",
	}); err != nil {
		t.Fatalf("Suggest: %v", err)
	}
	if repo.gotOpts.Backbone != "wcvp" {
		t.Errorf("SuggestOpts.Backbone = %q, want %q", repo.gotOpts.Backbone, "wcvp")
	}
}

// TestSuggest_EmptyEntryBackboneSkipsValidation: the option is opt-in, so an
// omitted value must not cost a backbone lookup nor reject anything.
func TestSuggest_EmptyEntryBackboneSkipsValidation(t *testing.T) {
	repo := &suggestBackboneRepo{}

	if _, err := application.Suggest(context.Background(), repo, application.SuggestRequest{
		Q: "coryn",
	}); err != nil {
		t.Fatalf("Suggest: %v", err)
	}
	if repo.gotOpts.Backbone != "" {
		t.Errorf("SuggestOpts.Backbone = %q, want empty", repo.gotOpts.Backbone)
	}
}

// suggestBackboneRepo is a Repository whose only ingested backbone is wcvp.
type suggestBackboneRepo struct {
	output.Repository
	gotOpts      output.SuggestOpts
	suggestCalls int
}

func (r *suggestBackboneRepo) Suggest(_ context.Context, _ string, opts output.SuggestOpts) ([]domain.SuggestItem, error) {
	r.suggestCalls++
	r.gotOpts = opts
	return nil, nil
}

func (r *suggestBackboneRepo) BackboneVersions(context.Context) ([]domain.BackboneVersion, error) {
	return []domain.BackboneVersion{{ID: "wcvp", Version: "2026-06-15"}}, nil
}

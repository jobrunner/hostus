package telemetry_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	otelapi "go.opentelemetry.io/otel"

	"github.com/jobrunner/hostus/internal/adapters/telemetry"
	"github.com/jobrunner/hostus/internal/domain"
	"github.com/jobrunner/hostus/internal/ports/output"
)

// setRecordingProvider installs a TracerProvider backed by a
// tracetest.SpanRecorder as the GLOBAL otel provider for the duration of the
// test, restoring the previous provider on cleanup. telemetry.TraceRepository
// resolves its tracer via otel.Tracer(...) AT CONSTRUCTION TIME (it binds to
// whichever provider is current when the constructor runs — see
// tracedrepo.go's doc comment), so callers MUST call TraceRepository AFTER
// this, never before.
func setRecordingProvider(t *testing.T) *tracetest.SpanRecorder {
	t.Helper()
	rec := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec))
	prev := otelapi.GetTracerProvider()
	otelapi.SetTracerProvider(tp)
	t.Cleanup(func() {
		otelapi.SetTracerProvider(prev)
	})
	return rec
}

// fakeRepo is a permissive stand-in for output.Repository: every method
// returns zero values and errFake (or nil, controlled per-test via the
// fields below), so both the happy-path and error-path tests can drive it
// without a real database.
type fakeRepo struct {
	// err, when non-nil, is returned by MatchExact instead of nil.
	err error
	// panic, when true, makes Suggest panic instead of returning — used by
	// TestTraceRepository_PanicStillEndsSpan to exercise the decorator's
	// panic path.
	panic bool
}

// panicSentinel is what fakeRepo.Suggest panics with when f.panic is true;
// a distinct type (not a plain string) so the test can assert recover()
// actually saw THIS panic and not some unrelated one.
type panicSentinel struct{}

var errFake = errors.New("fake repository failure")

func (f *fakeRepo) Concept(ctx context.Context, id string) (*domain.Concept, []output.SynonymName, []domain.Xref, []domain.Distribution, error) {
	return nil, nil, nil, nil, nil
}
func (f *fakeRepo) SynonymCandidates(ctx context.Context, conceptID string) ([]domain.SynonymCandidate, error) {
	return nil, nil
}
func (f *fakeRepo) Classification(ctx context.Context, conceptID string) ([]domain.ClassificationEntry, error) {
	return nil, nil
}
func (f *fakeRepo) ConceptByXref(ctx context.Context, authority, extID string) (*domain.Concept, error) {
	return nil, nil
}
func (f *fakeRepo) ConceptIDsByXref(ctx context.Context, authority string, extIDs []string) (map[string]string, error) {
	return nil, nil
}
func (f *fakeRepo) ExistingConceptIDs(ctx context.Context, ids []string) (map[string]bool, error) {
	return nil, nil
}
func (f *fakeRepo) SecReferences(ctx context.Context) ([]domain.SecReference, error) {
	return nil, nil
}
func (f *fakeRepo) Areas(ctx context.Context) ([]domain.Area, error) {
	return nil, nil
}
func (f *fakeRepo) SecReferenceByID(ctx context.Context, id string) (domain.SecReference, error) {
	return domain.SecReference{}, nil
}
func (f *fakeRepo) ConceptRelationsInSec(ctx context.Context, conceptID, targetSec string) (output.ConceptRelations, error) {
	return output.ConceptRelations{}, nil
}
func (f *fakeRepo) MatchExact(ctx context.Context, canon string) ([]output.MatchCandidate, error) {
	if f.err != nil {
		return nil, f.err
	}
	return nil, nil
}
func (f *fakeRepo) MatchFuzzyCandidates(ctx context.Context, canon string, limit int, backbone, sec string) ([]output.MatchCandidate, error) {
	return nil, nil
}
func (f *fakeRepo) BackboneVersions(ctx context.Context) ([]domain.BackboneVersion, error) {
	return nil, nil
}
func (f *fakeRepo) BuildDistributionClosure(ctx context.Context) error {
	return nil
}
func (f *fakeRepo) NameSpaceEntries(ctx context.Context, conceptID string, spaces []string) ([]domain.NameSpaceEntry, error) {
	return nil, nil
}
func (f *fakeRepo) NameSpaces(ctx context.Context) ([]domain.NameSpaceMeta, error) {
	return nil, nil
}
func (f *fakeRepo) AggregateMembers(ctx context.Context, aggregateConceptID string) ([]string, error) {
	return nil, nil
}
func (f *fakeRepo) AggregatesByMember(ctx context.Context, memberConceptID string) ([]string, error) {
	return nil, nil
}
func (f *fakeRepo) VernacularNames(ctx context.Context, conceptID string) ([]domain.VernacularName, error) {
	return nil, nil
}
func (f *fakeRepo) AggregateConcepts(ctx context.Context, backboneID string, ranks []domain.Rank) ([]output.AggregateConceptSummary, error) {
	return nil, nil
}
func (f *fakeRepo) WriteConceptAgreement(ctx context.Context, pairs []domain.ConceptAgreementPair) error {
	return nil
}
func (f *fakeRepo) ConceptAgreement(ctx context.Context, conceptID string) (*domain.ConceptAgreementPair, error) {
	return nil, nil
}
func (f *fakeRepo) Suggest(ctx context.Context, q string, opts output.SuggestOpts) ([]domain.SuggestItem, error) {
	if f.panic {
		panic(panicSentinel{})
	}
	return []domain.SuggestItem{{}}, nil
}
func (f *fakeRepo) BeginIngest(ctx context.Context, bv domain.BackboneVersion) (output.IngestTx, error) {
	return nil, nil
}
func (f *fakeRepo) BeginTraitIngest(ctx context.Context) (output.IngestTx, error) {
	return nil, nil
}

var _ output.Repository = (*fakeRepo)(nil)

// TestTraceRepository_SpansAndErrorStatus pins the debug-MCP gap found on
// 2026-09-05: a 7.9s /v1/suggest trace contained ONE span (the otelmux HTTP
// span) and nothing below — the repository layer was invisible, and the
// slow-query analysis had to fall back to EXPLAIN (see
// docs/superpowers/specs/2026-09-05-suggest-plan-fix-repo-tracing.md,
// decisions 4+5). Every repository call must emit a child span named
// repo.<Method>; an error must be recorded and set the span status to
// Error, and the returned values/error must pass through unchanged.
func TestTraceRepository_SpansAndErrorStatus(t *testing.T) {
	rec := setRecordingProvider(t)
	repo := telemetry.TraceRepository(&fakeRepo{})

	got, err := repo.Suggest(context.Background(), "querc", output.SuggestOpts{})
	if err != nil {
		t.Fatalf("Suggest returned unexpected error: %v", err)
	}
	want := []domain.SuggestItem{{}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Suggest result = %#v, want %#v (must pass through 1:1)", got, want)
	}

	ended := rec.Ended()
	if len(ended) != 1 {
		t.Fatalf("got %d ended spans, want exactly 1", len(ended))
	}
	if name := ended[0].Name(); name != "repo.Suggest" {
		t.Fatalf("span name = %q, want %q", name, "repo.Suggest")
	}
	if code := ended[0].Status().Code; code != codes.Unset {
		t.Fatalf("span status code = %v, want Unset (no error)", code)
	}
}

// TestTraceRepository_ErrorRecordedAndPassedThrough is
// TestTraceRepository_SpansAndErrorStatus's error-path counterpart (split
// into its own test so gocognit stays under this repo's per-function
// complexity budget): an errored call must set the span's status to Error,
// record an exception event, and pass the error back to the caller
// UNCHANGED (errors.Is against the exact sentinel).
func TestTraceRepository_ErrorRecordedAndPassedThrough(t *testing.T) {
	rec := setRecordingProvider(t)
	repo := telemetry.TraceRepository(&fakeRepo{err: errFake})

	_, err := repo.MatchExact(context.Background(), "quercus robur")
	if !errors.Is(err, errFake) {
		t.Fatalf("MatchExact error = %v, want errFake unchanged", err)
	}

	ended := rec.Ended()
	if len(ended) != 1 {
		t.Fatalf("got %d ended spans, want exactly 1", len(ended))
	}
	span := ended[0]
	if name := span.Name(); name != "repo.MatchExact" {
		t.Fatalf("span name = %q, want %q", name, "repo.MatchExact")
	}
	if code := span.Status().Code; code != codes.Error {
		t.Fatalf("span status code = %v, want Error", code)
	}
	found := false
	for _, ev := range span.Events() {
		if ev.Name == "exception" {
			found = true
		}
	}
	if !found {
		t.Fatal("no RecordError (exception) event found on the errored span")
	}
}

// TestTraceRepository_PanicStillEndsSpan pins the fix-round-1 finding: the
// original implementation called span.End() synchronously AFTER the inner
// call returned, so a panicking inner Repository method left its span open
// forever (never exported) — exactly the failure mode tracing exists to
// make visible. span.End() must run via `defer`, so it executes on the way
// out even when the inner call panics. This test asserts BOTH halves: (a)
// the panic propagates to the caller unchanged (no silent recover), and (b)
// the span was nonetheless ended and recorded.
func TestTraceRepository_PanicStillEndsSpan(t *testing.T) {
	rec := setRecordingProvider(t)
	repo := telemetry.TraceRepository(&fakeRepo{panic: true})

	func() {
		defer func() {
			got := recover()
			if got == nil {
				t.Fatal("expected the panic to propagate out of Suggest, got none")
			}
			if _, ok := got.(panicSentinel); !ok {
				t.Fatalf("recovered value = %#v, want panicSentinel{}", got)
			}
		}()
		_, _ = repo.Suggest(context.Background(), "querc", output.SuggestOpts{})
		t.Fatal("unreachable: Suggest should have panicked")
	}()

	ended := rec.Ended()
	if len(ended) != 1 {
		t.Fatalf("got %d ended spans after a panicking call, want exactly 1 (span.End() must still run)", len(ended))
	}
	if name := ended[0].Name(); name != "repo.Suggest" {
		t.Fatalf("span name = %q, want %q", name, "repo.Suggest")
	}
}

// TestTraceRepository_CoversEveryPortMethod pins completeness WITHOUT
// listing methods by hand (a new port method must not silently bypass
// tracing): it reflects over output.Repository's method set, invokes each
// via reflection against fakeRepo (through the decorator), and asserts one
// span named repo.<Method> per call. Every output.Repository method takes a
// context.Context as its first parameter (checked below and enforced by
// t.Fatal if that ever stops being true) — there is nothing to exempt today.
func TestTraceRepository_CoversEveryPortMethod(t *testing.T) {
	repoType := reflect.TypeOf((*output.Repository)(nil)).Elem()

	for i := 0; i < repoType.NumMethod(); i++ {
		m := repoType.Method(i)
		t.Run(m.Name, func(t *testing.T) {
			rec := setRecordingProvider(t)
			repo := telemetry.TraceRepository(&fakeRepo{})
			repoVal := reflect.ValueOf(repo)

			method := repoVal.MethodByName(m.Name)
			mt := method.Type()
			if mt.NumIn() == 0 || mt.In(0) != reflect.TypeOf((*context.Context)(nil)).Elem() {
				t.Fatalf("method %s does not take context.Context as its first parameter — the reflection test's no-exemptions assumption is stale, update it", m.Name)
			}

			args := make([]reflect.Value, mt.NumIn())
			args[0] = reflect.ValueOf(context.Background())
			for j := 1; j < mt.NumIn(); j++ {
				args[j] = reflect.Zero(mt.In(j))
			}

			method.Call(args)

			ended := rec.Ended()
			if len(ended) != 1 {
				t.Fatalf("method %s: got %d ended spans, want exactly 1", m.Name, len(ended))
			}
			wantName := "repo." + m.Name
			if got := ended[0].Name(); got != wantName {
				t.Fatalf("method %s: span name = %q, want %q", m.Name, got, wantName)
			}
		})
	}
}

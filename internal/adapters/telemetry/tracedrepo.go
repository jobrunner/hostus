package telemetry

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/jobrunner/hostus/internal/domain"
	"github.com/jobrunner/hostus/internal/ports/output"
)

// tracedRepository wraps an output.Repository so every port call emits a
// child span (repo.<Method>) under whatever span was already active on ctx
// (the otelmux HTTP span, on the serve path). This exists because a debug-
// MCP trace of a 7.9s /v1/suggest request (2026-09-05) showed exactly ONE
// span end to end — the otelmux HTTP span — with nothing underneath it: the
// repository layer was completely invisible to tracing, and the slow-query
// analysis had to fall back to EXPLAIN instead of a real span breakdown
// (see docs/superpowers/specs/2026-09-05-suggest-plan-fix-repo-tracing.md,
// decisions 4+5).
//
// Deliberately NO query text or bind parameters are recorded as span
// attributes: a verbatim species/concept-id/canon string is exactly the
// kind of high-cardinality, potentially sensitive value span attributes
// should not carry (it would blow up cardinality in any real OTLP backend
// and, for a genus/epithet query typed by a real user, is not something to
// fan out to telemetry unreviewed). The span's presence, name, timing, and
// error status already answer "was this call slow / did it fail" without
// that cost — that is the question the MCP gap above needed answered.
//
// This decorator is wired ONLY on the serve path (see
// internal/app/app.go's openRepo). Ingest and bundle-export code paths open
// an *sqlite.DB directly and are deliberately left unwrapped: an ingest
// batches millions of repository/IngestTx calls, and a span per call there
// would be pure overhead with no one reading it (nothing exports ingest
// traces today, and the debug MCP's ring buffer would just get flooded).
type tracedRepository struct {
	inner  output.Repository
	tracer trace.Tracer
}

// TraceRepository wraps inner so every output.Repository call it serves
// emits a "repo.<Method>" child span, with errors recorded on the span
// (RecordError + Status=Error) and passed through unchanged to the caller.
//
// The tracer is resolved via otel.Tracer(...) at CONSTRUCTION time (OTel's
// otel.Tracer binds to whichever TracerProvider is current when called, not
// per-call) — callers that install a different TracerProvider later (e.g.
// tests using otel.SetTracerProvider with a tracetest.SpanRecorder) must
// call TraceRepository AFTER installing it, or the spans will go to the
// provider that was current before.
func TraceRepository(inner output.Repository) output.Repository {
	return &tracedRepository{
		inner:  inner,
		tracer: otel.Tracer("hostus/repository"),
	}
}

// Unwrap returns the wrapped output.Repository. It exists solely so
// adapter-specific test seams that type-assert the concrete repository type
// (e.g. internal/app's TestOpenRepo_UsesConfiguredMaxReadConns asserting
// *sqlite.DB for pool introspection) keep working through the decorator:
// such a test asserts `repo.(interface{ Unwrap() output.Repository }).
// Unwrap().(*sqlite.DB)` instead of asserting the decorator's own type.
func (r *tracedRepository) Unwrap() output.Repository {
	return r.inner
}

func (r *tracedRepository) finish(span trace.Span, err error) {
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	span.End()
}

func (r *tracedRepository) Concept(ctx context.Context, id string) (*domain.Concept, []output.SynonymName, []domain.Xref, []domain.Distribution, error) {
	ctx, span := r.tracer.Start(ctx, "repo.Concept")
	c, syn, xrefs, dist, err := r.inner.Concept(ctx, id)
	r.finish(span, err)
	return c, syn, xrefs, dist, err
}

func (r *tracedRepository) SynonymCandidates(ctx context.Context, conceptID string) ([]domain.SynonymCandidate, error) {
	ctx, span := r.tracer.Start(ctx, "repo.SynonymCandidates")
	out, err := r.inner.SynonymCandidates(ctx, conceptID)
	r.finish(span, err)
	return out, err
}

func (r *tracedRepository) Classification(ctx context.Context, conceptID string) ([]domain.ClassificationEntry, error) {
	ctx, span := r.tracer.Start(ctx, "repo.Classification")
	out, err := r.inner.Classification(ctx, conceptID)
	r.finish(span, err)
	return out, err
}

func (r *tracedRepository) ConceptByXref(ctx context.Context, authority, extID string) (*domain.Concept, error) {
	ctx, span := r.tracer.Start(ctx, "repo.ConceptByXref")
	out, err := r.inner.ConceptByXref(ctx, authority, extID)
	r.finish(span, err)
	return out, err
}

func (r *tracedRepository) ConceptIDsByXref(ctx context.Context, authority string, extIDs []string) (map[string]string, error) {
	ctx, span := r.tracer.Start(ctx, "repo.ConceptIDsByXref")
	out, err := r.inner.ConceptIDsByXref(ctx, authority, extIDs)
	r.finish(span, err)
	return out, err
}

func (r *tracedRepository) ExistingConceptIDs(ctx context.Context, ids []string) (map[string]bool, error) {
	ctx, span := r.tracer.Start(ctx, "repo.ExistingConceptIDs")
	out, err := r.inner.ExistingConceptIDs(ctx, ids)
	r.finish(span, err)
	return out, err
}

func (r *tracedRepository) SecReferences(ctx context.Context) ([]domain.SecReference, error) {
	ctx, span := r.tracer.Start(ctx, "repo.SecReferences")
	out, err := r.inner.SecReferences(ctx)
	r.finish(span, err)
	return out, err
}

func (r *tracedRepository) Areas(ctx context.Context) ([]domain.Area, error) {
	ctx, span := r.tracer.Start(ctx, "repo.Areas")
	out, err := r.inner.Areas(ctx)
	r.finish(span, err)
	return out, err
}

func (r *tracedRepository) SecReferenceByID(ctx context.Context, id string) (domain.SecReference, error) {
	ctx, span := r.tracer.Start(ctx, "repo.SecReferenceByID")
	out, err := r.inner.SecReferenceByID(ctx, id)
	r.finish(span, err)
	return out, err
}

func (r *tracedRepository) ConceptRelationsInSec(ctx context.Context, conceptID, targetSec string) (output.ConceptRelations, error) {
	ctx, span := r.tracer.Start(ctx, "repo.ConceptRelationsInSec")
	out, err := r.inner.ConceptRelationsInSec(ctx, conceptID, targetSec)
	r.finish(span, err)
	return out, err
}

func (r *tracedRepository) MatchExact(ctx context.Context, canon string) ([]output.MatchCandidate, error) {
	ctx, span := r.tracer.Start(ctx, "repo.MatchExact")
	out, err := r.inner.MatchExact(ctx, canon)
	r.finish(span, err)
	return out, err
}

func (r *tracedRepository) MatchFuzzyCandidates(ctx context.Context, canon string, limit int, backbone, sec string) ([]output.MatchCandidate, error) {
	ctx, span := r.tracer.Start(ctx, "repo.MatchFuzzyCandidates")
	out, err := r.inner.MatchFuzzyCandidates(ctx, canon, limit, backbone, sec)
	r.finish(span, err)
	return out, err
}

func (r *tracedRepository) BackboneVersions(ctx context.Context) ([]domain.BackboneVersion, error) {
	ctx, span := r.tracer.Start(ctx, "repo.BackboneVersions")
	out, err := r.inner.BackboneVersions(ctx)
	r.finish(span, err)
	return out, err
}

func (r *tracedRepository) BuildDistributionClosure(ctx context.Context) error {
	ctx, span := r.tracer.Start(ctx, "repo.BuildDistributionClosure")
	err := r.inner.BuildDistributionClosure(ctx)
	r.finish(span, err)
	return err
}

func (r *tracedRepository) NameSpaceEntries(ctx context.Context, conceptID string, spaces []string) ([]domain.NameSpaceEntry, error) {
	ctx, span := r.tracer.Start(ctx, "repo.NameSpaceEntries")
	out, err := r.inner.NameSpaceEntries(ctx, conceptID, spaces)
	r.finish(span, err)
	return out, err
}

func (r *tracedRepository) NameSpaces(ctx context.Context) ([]domain.NameSpaceMeta, error) {
	ctx, span := r.tracer.Start(ctx, "repo.NameSpaces")
	out, err := r.inner.NameSpaces(ctx)
	r.finish(span, err)
	return out, err
}

func (r *tracedRepository) AggregateMembers(ctx context.Context, aggregateConceptID string) ([]string, error) {
	ctx, span := r.tracer.Start(ctx, "repo.AggregateMembers")
	out, err := r.inner.AggregateMembers(ctx, aggregateConceptID)
	r.finish(span, err)
	return out, err
}

func (r *tracedRepository) AggregatesByMember(ctx context.Context, memberConceptID string) ([]string, error) {
	ctx, span := r.tracer.Start(ctx, "repo.AggregatesByMember")
	out, err := r.inner.AggregatesByMember(ctx, memberConceptID)
	r.finish(span, err)
	return out, err
}

func (r *tracedRepository) VernacularNames(ctx context.Context, conceptID string) ([]domain.VernacularName, error) {
	ctx, span := r.tracer.Start(ctx, "repo.VernacularNames")
	out, err := r.inner.VernacularNames(ctx, conceptID)
	r.finish(span, err)
	return out, err
}

func (r *tracedRepository) AggregateConcepts(ctx context.Context, backboneID string, ranks []domain.Rank) ([]output.AggregateConceptSummary, error) {
	ctx, span := r.tracer.Start(ctx, "repo.AggregateConcepts")
	out, err := r.inner.AggregateConcepts(ctx, backboneID, ranks)
	r.finish(span, err)
	return out, err
}

func (r *tracedRepository) WriteConceptAgreement(ctx context.Context, pairs []domain.ConceptAgreementPair) error {
	ctx, span := r.tracer.Start(ctx, "repo.WriteConceptAgreement")
	err := r.inner.WriteConceptAgreement(ctx, pairs)
	r.finish(span, err)
	return err
}

func (r *tracedRepository) ConceptAgreement(ctx context.Context, conceptID string) (*domain.ConceptAgreementPair, error) {
	ctx, span := r.tracer.Start(ctx, "repo.ConceptAgreement")
	out, err := r.inner.ConceptAgreement(ctx, conceptID)
	r.finish(span, err)
	return out, err
}

func (r *tracedRepository) Suggest(ctx context.Context, q string, opts output.SuggestOpts) ([]domain.SuggestItem, error) {
	ctx, span := r.tracer.Start(ctx, "repo.Suggest")
	out, err := r.inner.Suggest(ctx, q, opts)
	r.finish(span, err)
	return out, err
}

func (r *tracedRepository) BeginIngest(ctx context.Context, bv domain.BackboneVersion) (output.IngestTx, error) {
	ctx, span := r.tracer.Start(ctx, "repo.BeginIngest")
	out, err := r.inner.BeginIngest(ctx, bv)
	r.finish(span, err)
	return out, err
}

func (r *tracedRepository) BeginTraitIngest(ctx context.Context) (output.IngestTx, error) {
	ctx, span := r.tracer.Start(ctx, "repo.BeginTraitIngest")
	out, err := r.inner.BeginTraitIngest(ctx)
	r.finish(span, err)
	return out, err
}

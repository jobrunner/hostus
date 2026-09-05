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

// end marks span's outcome from *errp (nil until the deferring method's
// named return is actually assigned) and ends the span. It is always called
// via `defer r.end(span, &err)` — a NAMED return, so the defer captures the
// value the method is actually returning, including one set by a plain
// `return` after the inner call, not a stale value read before the call
// completed.
//
// Deliberately no recover() here: if r.inner panics, the panic propagates
// unchanged past this defer to whatever the caller (ultimately net/http's
// per-request recoverer) does with it — this decorator does not turn a
// panic into an error return, since that would be a silent behavior change
// upstream code doesn't expect. What defer DOES buy here is that span.End()
// still runs on the way out via the panicking goroutine's deferred call
// stack, so the span is not left open/unexported (the exact failure mode
// tracing exists to make visible) even though it ends with Status Unset —
// there is no error value to mark it Error with, since err was never
// assigned before the panic. See TestTraceRepository_PanicStillEndsSpan.
func (r *tracedRepository) end(span trace.Span, errp *error) {
	if errp != nil && *errp != nil {
		span.RecordError(*errp)
		span.SetStatus(codes.Error, (*errp).Error())
	}
	span.End()
}

func (r *tracedRepository) Concept(ctx context.Context, id string) (c *domain.Concept, syn []output.SynonymName, xrefs []domain.Xref, dist []domain.Distribution, err error) {
	ctx, span := r.tracer.Start(ctx, "repo.Concept")
	defer r.end(span, &err)
	c, syn, xrefs, dist, err = r.inner.Concept(ctx, id)
	return c, syn, xrefs, dist, err
}

func (r *tracedRepository) SynonymCandidates(ctx context.Context, conceptID string) (out []domain.SynonymCandidate, err error) {
	ctx, span := r.tracer.Start(ctx, "repo.SynonymCandidates")
	defer r.end(span, &err)
	out, err = r.inner.SynonymCandidates(ctx, conceptID)
	return out, err
}

func (r *tracedRepository) Classification(ctx context.Context, conceptID string) (out []domain.ClassificationEntry, err error) {
	ctx, span := r.tracer.Start(ctx, "repo.Classification")
	defer r.end(span, &err)
	out, err = r.inner.Classification(ctx, conceptID)
	return out, err
}

func (r *tracedRepository) ConceptByXref(ctx context.Context, authority, extID string) (out *domain.Concept, err error) {
	ctx, span := r.tracer.Start(ctx, "repo.ConceptByXref")
	defer r.end(span, &err)
	out, err = r.inner.ConceptByXref(ctx, authority, extID)
	return out, err
}

func (r *tracedRepository) ConceptIDsByXref(ctx context.Context, authority string, extIDs []string) (out map[string]string, err error) {
	ctx, span := r.tracer.Start(ctx, "repo.ConceptIDsByXref")
	defer r.end(span, &err)
	out, err = r.inner.ConceptIDsByXref(ctx, authority, extIDs)
	return out, err
}

func (r *tracedRepository) ExistingConceptIDs(ctx context.Context, ids []string) (out map[string]bool, err error) {
	ctx, span := r.tracer.Start(ctx, "repo.ExistingConceptIDs")
	defer r.end(span, &err)
	out, err = r.inner.ExistingConceptIDs(ctx, ids)
	return out, err
}

func (r *tracedRepository) SecReferences(ctx context.Context) (out []domain.SecReference, err error) {
	ctx, span := r.tracer.Start(ctx, "repo.SecReferences")
	defer r.end(span, &err)
	out, err = r.inner.SecReferences(ctx)
	return out, err
}

func (r *tracedRepository) Areas(ctx context.Context) (out []domain.Area, err error) {
	ctx, span := r.tracer.Start(ctx, "repo.Areas")
	defer r.end(span, &err)
	out, err = r.inner.Areas(ctx)
	return out, err
}

func (r *tracedRepository) SecReferenceByID(ctx context.Context, id string) (out domain.SecReference, err error) {
	ctx, span := r.tracer.Start(ctx, "repo.SecReferenceByID")
	defer r.end(span, &err)
	out, err = r.inner.SecReferenceByID(ctx, id)
	return out, err
}

func (r *tracedRepository) ConceptRelationsInSec(ctx context.Context, conceptID, targetSec string) (out output.ConceptRelations, err error) {
	ctx, span := r.tracer.Start(ctx, "repo.ConceptRelationsInSec")
	defer r.end(span, &err)
	out, err = r.inner.ConceptRelationsInSec(ctx, conceptID, targetSec)
	return out, err
}

func (r *tracedRepository) MatchExact(ctx context.Context, canon string) (out []output.MatchCandidate, err error) {
	ctx, span := r.tracer.Start(ctx, "repo.MatchExact")
	defer r.end(span, &err)
	out, err = r.inner.MatchExact(ctx, canon)
	return out, err
}

func (r *tracedRepository) MatchFuzzyCandidates(ctx context.Context, canon string, limit int, backbone, sec string) (out []output.MatchCandidate, err error) {
	ctx, span := r.tracer.Start(ctx, "repo.MatchFuzzyCandidates")
	defer r.end(span, &err)
	out, err = r.inner.MatchFuzzyCandidates(ctx, canon, limit, backbone, sec)
	return out, err
}

func (r *tracedRepository) BackboneVersions(ctx context.Context) (out []domain.BackboneVersion, err error) {
	ctx, span := r.tracer.Start(ctx, "repo.BackboneVersions")
	defer r.end(span, &err)
	out, err = r.inner.BackboneVersions(ctx)
	return out, err
}

func (r *tracedRepository) BuildDistributionClosure(ctx context.Context) (err error) {
	ctx, span := r.tracer.Start(ctx, "repo.BuildDistributionClosure")
	defer r.end(span, &err)
	err = r.inner.BuildDistributionClosure(ctx)
	return err
}

func (r *tracedRepository) NameSpaceEntries(ctx context.Context, conceptID string, spaces []string) (out []domain.NameSpaceEntry, err error) {
	ctx, span := r.tracer.Start(ctx, "repo.NameSpaceEntries")
	defer r.end(span, &err)
	out, err = r.inner.NameSpaceEntries(ctx, conceptID, spaces)
	return out, err
}

func (r *tracedRepository) NameSpaces(ctx context.Context) (out []domain.NameSpaceMeta, err error) {
	ctx, span := r.tracer.Start(ctx, "repo.NameSpaces")
	defer r.end(span, &err)
	out, err = r.inner.NameSpaces(ctx)
	return out, err
}

func (r *tracedRepository) AggregateMembers(ctx context.Context, aggregateConceptID string) (out []string, err error) {
	ctx, span := r.tracer.Start(ctx, "repo.AggregateMembers")
	defer r.end(span, &err)
	out, err = r.inner.AggregateMembers(ctx, aggregateConceptID)
	return out, err
}

func (r *tracedRepository) AggregatesByMember(ctx context.Context, memberConceptID string) (out []string, err error) {
	ctx, span := r.tracer.Start(ctx, "repo.AggregatesByMember")
	defer r.end(span, &err)
	out, err = r.inner.AggregatesByMember(ctx, memberConceptID)
	return out, err
}

func (r *tracedRepository) VernacularNames(ctx context.Context, conceptID string) (out []domain.VernacularName, err error) {
	ctx, span := r.tracer.Start(ctx, "repo.VernacularNames")
	defer r.end(span, &err)
	out, err = r.inner.VernacularNames(ctx, conceptID)
	return out, err
}

func (r *tracedRepository) AggregateConcepts(ctx context.Context, backboneID string, ranks []domain.Rank) (out []output.AggregateConceptSummary, err error) {
	ctx, span := r.tracer.Start(ctx, "repo.AggregateConcepts")
	defer r.end(span, &err)
	out, err = r.inner.AggregateConcepts(ctx, backboneID, ranks)
	return out, err
}

func (r *tracedRepository) WriteConceptAgreement(ctx context.Context, pairs []domain.ConceptAgreementPair) (err error) {
	ctx, span := r.tracer.Start(ctx, "repo.WriteConceptAgreement")
	defer r.end(span, &err)
	err = r.inner.WriteConceptAgreement(ctx, pairs)
	return err
}

func (r *tracedRepository) ConceptAgreement(ctx context.Context, conceptID string) (out *domain.ConceptAgreementPair, err error) {
	ctx, span := r.tracer.Start(ctx, "repo.ConceptAgreement")
	defer r.end(span, &err)
	out, err = r.inner.ConceptAgreement(ctx, conceptID)
	return out, err
}

func (r *tracedRepository) Suggest(ctx context.Context, q string, opts output.SuggestOpts) (out []domain.SuggestItem, err error) {
	ctx, span := r.tracer.Start(ctx, "repo.Suggest")
	defer r.end(span, &err)
	out, err = r.inner.Suggest(ctx, q, opts)
	return out, err
}

func (r *tracedRepository) BeginIngest(ctx context.Context, bv domain.BackboneVersion) (out output.IngestTx, err error) {
	ctx, span := r.tracer.Start(ctx, "repo.BeginIngest")
	defer r.end(span, &err)
	out, err = r.inner.BeginIngest(ctx, bv)
	return out, err
}

func (r *tracedRepository) BeginTraitIngest(ctx context.Context) (out output.IngestTx, err error) {
	ctx, span := r.tracer.Start(ctx, "repo.BeginTraitIngest")
	defer r.end(span, &err)
	out, err = r.inner.BeginTraitIngest(ctx)
	return out, err
}

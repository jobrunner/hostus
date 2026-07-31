package telemetry_test

import (
	"context"
	"log/slog"
	"math"
	"testing"
	"time"

	"go.opentelemetry.io/otel/sdk/trace"

	"github.com/jobrunner/hostus/internal/adapters/telemetry"
	"github.com/jobrunner/hostus/internal/config"
)

// newTestProvider builds a TracerProvider that always samples and exports
// only to the given MemoryExporter, synchronously (SimpleSpanProcessor), so
// tests can assert on captured spans immediately after End() without a
// ForceFlush race.
func newTestProvider(exp *telemetry.MemoryExporter) *trace.TracerProvider {
	return trace.NewTracerProvider(
		trace.WithSampler(trace.AlwaysSample()),
		trace.WithSpanProcessor(trace.NewSimpleSpanProcessor(exp)),
	)
}

func TestMemoryExporterCapturesSpan(t *testing.T) {
	exp := telemetry.NewMemoryExporter(16)
	tp := trace.NewTracerProvider(trace.WithSpanProcessor(trace.NewSimpleSpanProcessor(exp)))
	_, span := tp.Tracer("t").Start(context.Background(), "op")
	span.End()
	_ = tp.ForceFlush(context.Background())
	if got := exp.Spans(); len(got) != 1 || got[0].Name != "op" {
		t.Fatalf("want 1 span 'op', got %+v", got)
	}
}

func TestRingLogFiltersByLevel(t *testing.T) {
	rl := telemetry.NewRingLog(16)
	l := slog.New(rl)
	l.Info("hi")
	l.Error("boom")
	if errs := rl.Records(slog.LevelError, 10); len(errs) != 1 || errs[0].Msg != "boom" {
		t.Fatalf("want 1 error 'boom', got %+v", errs)
	}
}

// TestMemoryExporterWrapsAtCapacity verifies the ring buffer overwrites the
// oldest spans once full, retaining only the most recent `capacity` spans.
func TestMemoryExporterWrapsAtCapacity(t *testing.T) {
	exp := telemetry.NewMemoryExporter(3)
	tp := newTestProvider(exp)
	tr := tp.Tracer("t")

	names := []string{"a", "b", "c", "d", "e"}
	for _, n := range names {
		_, span := tr.Start(context.Background(), n)
		span.End()
	}

	got := exp.Spans()
	if len(got) != 3 {
		t.Fatalf("want 3 spans retained, got %d: %+v", len(got), got)
	}
	wantNames := []string{"c", "d", "e"}
	for i, w := range wantNames {
		if got[i].Name != w {
			t.Errorf("Spans()[%d].Name = %q, want %q (oldest-first order after wrap)", i, got[i].Name, w)
		}
	}
}

// TestMemoryExporterTraceFiltersToOneTrace verifies Trace(traceID) returns
// only spans belonging to that trace, ignoring spans from other traces.
func TestMemoryExporterTraceFiltersToOneTrace(t *testing.T) {
	exp := telemetry.NewMemoryExporter(16)
	tp := newTestProvider(exp)
	tr := tp.Tracer("t")

	ctx, root := tr.Start(context.Background(), "root")
	_, child := tr.Start(ctx, "child")
	child.End()
	traceID := root.SpanContext().TraceID().String()
	root.End()

	// A second, unrelated trace that must not leak into the filtered result.
	_, other := tr.Start(context.Background(), "unrelated")
	other.End()

	got := exp.Trace(traceID)
	if len(got) != 2 {
		t.Fatalf("want 2 spans for trace %s, got %d: %+v", traceID, len(got), got)
	}
	for _, s := range got {
		if s.TraceID != traceID {
			t.Errorf("Trace(%s) returned span from trace %s", traceID, s.TraceID)
		}
	}

	if all := exp.Spans(); len(all) != 3 {
		t.Fatalf("want 3 spans total across both traces, got %d", len(all))
	}
}

// TestRingLogInjectsTraceAndSpanID verifies that logging through a context
// carrying a live span attaches that span's trace_id/span_id to the log
// record, and that context-free logging leaves them empty.
func TestRingLogInjectsTraceAndSpanID(t *testing.T) {
	exp := telemetry.NewMemoryExporter(16)
	tp := newTestProvider(exp)
	tr := tp.Tracer("t")

	rl := telemetry.NewRingLog(16)
	l := slog.New(rl)

	ctx, span := tr.Start(context.Background(), "op")
	wantTraceID := span.SpanContext().TraceID().String()
	wantSpanID := span.SpanContext().SpanID().String()
	l.InfoContext(ctx, "inside span")
	span.End()

	l.Info("outside span") // no context -> no span attached

	recs := rl.Records(slog.LevelDebug, 10)
	if len(recs) != 2 {
		t.Fatalf("want 2 records, got %d: %+v", len(recs), recs)
	}

	// Records() returns newest first.
	outside, inside := recs[0], recs[1]
	if inside.Msg != "inside span" || outside.Msg != "outside span" {
		t.Fatalf("unexpected record order: %+v", recs)
	}
	if inside.TraceID != wantTraceID || inside.SpanID != wantSpanID {
		t.Errorf("inside span: TraceID=%q SpanID=%q, want %q/%q", inside.TraceID, inside.SpanID, wantTraceID, wantSpanID)
	}
	if outside.TraceID != "" || outside.SpanID != "" {
		t.Errorf("outside span: want empty TraceID/SpanID, got %q/%q", outside.TraceID, outside.SpanID)
	}
}

// TestRingLogRecordsLimitHonored verifies the limit parameter caps the
// number of returned records even when more matching records exist.
func TestRingLogRecordsLimitHonored(t *testing.T) {
	rl := telemetry.NewRingLog(16)
	l := slog.New(rl)

	for i := 0; i < 5; i++ {
		l.Info("msg")
	}

	got := rl.Records(slog.LevelInfo, 2)
	if len(got) != 2 {
		t.Fatalf("want 2 records (limit honored), got %d", len(got))
	}
}

// TestNewMemoryExporterCapacityFloor verifies that a non-positive requested
// capacity is floored to 1 rather than producing a zero-length (and thus
// modulo-by-zero-panicking) ring buffer.
func TestNewMemoryExporterCapacityFloor(t *testing.T) {
	for _, capacity := range []int{0, -1, -100} {
		exp := telemetry.NewMemoryExporter(capacity)
		tp := newTestProvider(exp)
		tr := tp.Tracer("t")

		_, s1 := tr.Start(context.Background(), "first")
		s1.End()
		_, s2 := tr.Start(context.Background(), "second")
		s2.End()

		got := exp.Spans()
		if len(got) != 1 {
			t.Fatalf("capacity=%d: want exactly 1 retained span (floor of 1), got %d: %+v", capacity, len(got), got)
		}
		if got[0].Name != "second" {
			t.Errorf("capacity=%d: want the most recent span retained, got %q", capacity, got[0].Name)
		}
	}
}

// TestNewRingLogCapacityFloor verifies that a non-positive requested
// capacity is floored to 1 for RingLog too.
func TestNewRingLogCapacityFloor(t *testing.T) {
	for _, capacity := range []int{0, -1, -100} {
		rl := telemetry.NewRingLog(capacity)
		l := slog.New(rl)
		l.Info("first")
		l.Info("second")

		got := rl.Records(slog.LevelDebug, 0)
		if len(got) != 1 {
			t.Fatalf("capacity=%d: want exactly 1 retained record (floor of 1), got %d: %+v", capacity, len(got), got)
		}
		if got[0].Msg != "second" {
			t.Errorf("capacity=%d: want the most recent record retained, got %q", capacity, got[0].Msg)
		}
	}
}

// TestMemoryExporterSpanDurationAccurate pins the DurationMS computation
// against an independently-computed expectation (using time.Millisecond
// directly, rather than the Microseconds()/1000.0 the implementation uses),
// so a mutation of the divisor or operator is caught even though both
// values ultimately derive from the same span timestamps.
func TestMemoryExporterSpanDurationAccurate(t *testing.T) {
	exp := telemetry.NewMemoryExporter(4)
	tp := newTestProvider(exp)
	tr := tp.Tracer("t")

	const sleep = 30 * time.Millisecond
	_, span := tr.Start(context.Background(), "timed")
	time.Sleep(sleep)
	span.End()

	got := exp.Spans()
	if len(got) != 1 {
		t.Fatalf("want 1 span, got %d", len(got))
	}
	rec := got[0]

	wantMS := float64(rec.End.Sub(rec.Start)) / float64(time.Millisecond)
	if math.Abs(rec.DurationMS-wantMS) > 0.01 {
		t.Fatalf("DurationMS = %f, want %f (derived independently from Start/End)", rec.DurationMS, wantMS)
	}
	// Sanity: the sleep must actually show up, ruling out a mutant that
	// zeroes or wildly rescales the duration while still matching the
	// (self-referential) formula above by coincidence.
	if rec.DurationMS < float64(sleep/time.Millisecond) {
		t.Fatalf("DurationMS = %f, want >= %v ms (the sleep duration)", rec.DurationMS, sleep/time.Millisecond)
	}
}

// TestRingLogWrapsAtCapacity verifies the log ring buffer overwrites the
// oldest records once full, retaining only the most recent `capacity`
// records, mirroring TestMemoryExporterWrapsAtCapacity for spans.
func TestRingLogWrapsAtCapacity(t *testing.T) {
	rl := telemetry.NewRingLog(3)
	l := slog.New(rl)

	for _, msg := range []string{"a", "b", "c", "d", "e"} {
		l.Info(msg)
	}

	got := rl.Records(slog.LevelDebug, 0)
	if len(got) != 3 {
		t.Fatalf("want 3 records retained, got %d: %+v", len(got), got)
	}
	// Records() is newest-first.
	wantMsgs := []string{"e", "d", "c"}
	for i, w := range wantMsgs {
		if got[i].Msg != w {
			t.Errorf("Records()[%d].Msg = %q, want %q (newest-first order after wrap)", i, got[i].Msg, w)
		}
	}
}

// TestRingLogRecordsUnlimitedWhenZero verifies limit<=0 means "no limit":
// every retained record at/above the level floor is returned, even when
// more records exist than any single non-zero limit used elsewhere in the
// test suite.
func TestRingLogRecordsUnlimitedWhenZero(t *testing.T) {
	rl := telemetry.NewRingLog(16)
	l := slog.New(rl)
	for i := 0; i < 9; i++ {
		l.Info("msg")
	}

	got := rl.Records(slog.LevelInfo, 0)
	if len(got) != 9 {
		t.Fatalf("want all 9 records with limit=0 (unlimited), got %d", len(got))
	}
}

// TestSetupWithoutOTLPStillActivatesMemoryExporters verifies that when
// telemetry is disabled (or has no endpoint), Setup still wires the
// always-on in-memory span exporter and log ring: the debug MCP must be
// able to see spans even with no external collector configured.
func TestSetupWithoutOTLPStillActivatesMemoryExporters(t *testing.T) {
	cfg := &config.Config{
		Telemetry: config.TelemetryConfig{
			Enabled:     false,
			Endpoint:    "",
			SampleRatio: 1.0,
		},
	}

	providers, shutdown, err := telemetry.Setup(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Setup returned error: %v", err)
	}
	if providers.Memory == nil || providers.Log == nil {
		t.Fatal("in-memory exporters must always be installed, even with telemetry disabled")
	}

	_, span := providers.TracerProvider.Tracer("t").Start(context.Background(), "op")
	span.End()

	if got := providers.Memory.Spans(); len(got) != 1 || got[0].Name != "op" {
		t.Fatalf("want 1 span 'op' captured via always-on memory exporter, got %+v", got)
	}

	if err := shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown returned error: %v", err)
	}
}

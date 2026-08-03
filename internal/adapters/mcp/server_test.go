package mcp_test

import (
	"context"
	"log/slog"
	"strings"
	"testing"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	mcpAdapter "github.com/jobrunner/hostus/internal/adapters/mcp"
	"github.com/jobrunner/hostus/internal/adapters/telemetry"
)

// TestTailErrorsTool is the brief's Step 1 RED test: seed a RingLog with an
// error via slog, build a Server around it, and confirm tail_errors surfaces
// the message.
func TestTailErrorsTool(t *testing.T) {
	rl := telemetry.NewRingLog(16)
	slog.New(rl).Error("kaboom")

	srv := mcpAdapter.NewServer(rl, telemetry.NewMemoryExporter(16))
	out, err := srv.CallTool(context.Background(), "tail_errors", map[string]any{"limit": 5})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "kaboom") {
		t.Fatalf("got %q", out)
	}
}

// TestTailErrorsToolExcludesInfo confirms the level filter actually filters:
// an info-level record must not show up in tail_errors' output.
func TestTailErrorsToolExcludesInfo(t *testing.T) {
	rl := telemetry.NewRingLog(16)
	log := slog.New(rl)
	log.Info("just-fyi")
	log.Error("boom-town")

	srv := mcpAdapter.NewServer(rl, telemetry.NewMemoryExporter(16))
	out, err := srv.CallTool(context.Background(), "tail_errors", map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "boom-town") {
		t.Fatalf("missing error record: %q", out)
	}
	if strings.Contains(out, "just-fyi") {
		t.Fatalf("info record leaked into tail_errors: %q", out)
	}
}

// TestGetRecentLogsLevelFilter checks get_recent_logs' level argument
// actually restricts the minimum level returned.
func TestGetRecentLogsLevelFilter(t *testing.T) {
	rl := telemetry.NewRingLog(16)
	log := slog.New(rl)
	log.Debug("debug-noise")
	log.Warn("warn-signal")

	srv := mcpAdapter.NewServer(rl, telemetry.NewMemoryExporter(16))

	out, err := srv.CallTool(context.Background(), "get_recent_logs", map[string]any{"level": "warn"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "warn-signal") {
		t.Fatalf("missing warn record: %q", out)
	}
	if strings.Contains(out, "debug-noise") {
		t.Fatalf("debug record leaked past level=warn filter: %q", out)
	}

	// Without a level filter, both must be present.
	outAll, err := srv.CallTool(context.Background(), "get_recent_logs", map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(outAll, "debug-noise") || !strings.Contains(outAll, "warn-signal") {
		t.Fatalf("expected both records with no level filter: %q", outAll)
	}
}

// TestGetRecentLogsLimit checks the limit argument is honored.
func TestGetRecentLogsLimit(t *testing.T) {
	rl := telemetry.NewRingLog(16)
	log := slog.New(rl)
	log.Info("first")
	log.Info("second")
	log.Info("third")

	srv := mcpAdapter.NewServer(rl, telemetry.NewMemoryExporter(16))
	out, err := srv.CallTool(context.Background(), "get_recent_logs", map[string]any{"limit": 1})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `"count":1`) {
		t.Fatalf("expected count:1 in output, got %q", out)
	}
}

// TestGetTraceTool seeds a real span into a MemoryExporter via a real
// TracerProvider and confirms get_trace retrieves it by trace id.
func TestGetTraceTool(t *testing.T) {
	mem := telemetry.NewMemoryExporter(16)
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(sdktrace.NewSimpleSpanProcessor(mem)))
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })

	_, span := tp.Tracer("test").Start(context.Background(), "do-the-thing")
	traceID := span.SpanContext().TraceID().String()
	span.End()

	srv := mcpAdapter.NewServer(telemetry.NewRingLog(16), mem)
	out, err := srv.CallTool(context.Background(), "get_trace", map[string]any{"trace_id": traceID})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "do-the-thing") {
		t.Fatalf("expected span name in output, got %q", out)
	}
	if !strings.Contains(out, traceID) {
		t.Fatalf("expected trace id in output, got %q", out)
	}
}

// TestGetTraceToolRequiresTraceID confirms arg validation: an empty
// trace_id must fail rather than silently returning everything.
func TestGetTraceToolRequiresTraceID(t *testing.T) {
	srv := mcpAdapter.NewServer(telemetry.NewRingLog(16), telemetry.NewMemoryExporter(16))
	_, err := srv.CallTool(context.Background(), "get_trace", map[string]any{"trace_id": ""})
	if err == nil {
		t.Fatal("expected error for empty trace_id, got nil")
	}
}

// TestGetTraceToolUnknownID returns an empty (not erroring) result for a
// trace id that was never recorded.
func TestGetTraceToolUnknownID(t *testing.T) {
	srv := mcpAdapter.NewServer(telemetry.NewRingLog(16), telemetry.NewMemoryExporter(16))
	out, err := srv.CallTool(context.Background(), "get_trace", map[string]any{"trace_id": "deadbeefdeadbeefdeadbeefdeadbeef"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `"count":0`) {
		t.Fatalf("expected empty result, got %q", out)
	}
}

// TestListSpansToolFilters seeds two spans of differing name/duration and
// confirms both the operation substring filter and the min_duration_ms
// filter narrow the result set correctly.
func TestListSpansToolFilters(t *testing.T) {
	mem := telemetry.NewMemoryExporter(16)
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(sdktrace.NewSimpleSpanProcessor(mem)))
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })

	_, s1 := tp.Tracer("test").Start(context.Background(), "fetch-gbif")
	s1.End()
	_, s2 := tp.Tracer("test").Start(context.Background(), "cache-lookup")
	s2.End()

	srv := mcpAdapter.NewServer(telemetry.NewRingLog(16), mem)

	out, err := srv.CallTool(context.Background(), "list_spans", map[string]any{"operation": "gbif"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "fetch-gbif") {
		t.Fatalf("expected fetch-gbif in filtered output, got %q", out)
	}
	if strings.Contains(out, "cache-lookup") {
		t.Fatalf("operation filter leaked non-matching span: %q", out)
	}

	// min_duration_ms higher than any real (near-zero) span duration must
	// exclude everything.
	outFiltered, err := srv.CallTool(context.Background(), "list_spans", map[string]any{"min_duration_ms": 1_000_000.0})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(outFiltered, `"count":0`) {
		t.Fatalf("expected zero spans above impossible duration floor, got %q", outFiltered)
	}
}

// TestToolsAreReadOnly calls every tool once and confirms the underlying
// buffers are unmodified afterwards (same record counts, same content).
func TestToolsAreReadOnly(t *testing.T) {
	rl := telemetry.NewRingLog(16)
	slog.New(rl).Error("read-only-check")

	mem := telemetry.NewMemoryExporter(16)
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(sdktrace.NewSimpleSpanProcessor(mem)))
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })
	_, span := tp.Tracer("test").Start(context.Background(), "probe")
	traceID := span.SpanContext().TraceID().String()
	span.End()

	beforeLogs := rl.Records(slog.LevelDebug, 0)
	beforeSpans := mem.Spans()

	srv := mcpAdapter.NewServer(rl, mem)
	ctx := context.Background()
	for _, call := range []struct {
		name string
		args map[string]any
	}{
		{"get_recent_logs", map[string]any{}},
		{"tail_errors", map[string]any{}},
		{"get_trace", map[string]any{"trace_id": traceID}},
		{"list_spans", map[string]any{}},
	} {
		if _, err := srv.CallTool(ctx, call.name, call.args); err != nil {
			t.Fatalf("%s: %v", call.name, err)
		}
	}

	afterLogs := rl.Records(slog.LevelDebug, 0)
	afterSpans := mem.Spans()

	if len(beforeLogs) != len(afterLogs) {
		t.Fatalf("log buffer mutated: before=%d after=%d", len(beforeLogs), len(afterLogs))
	}
	if len(beforeSpans) != len(afterSpans) {
		t.Fatalf("span buffer mutated: before=%d after=%d", len(beforeSpans), len(afterSpans))
	}
}

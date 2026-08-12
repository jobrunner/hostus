// Package telemetry wires the OpenTelemetry SDK for hostus and provides
// always-on in-memory span and log ring buffers so the debug MCP surface
// (task S10) can inspect recent activity without an external collector.
package telemetry

import (
	"context"
	"log/slog"
	"sync"
	"time"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// SpanRecord is a flattened, MCP-friendly snapshot of a finished span.
type SpanRecord struct {
	TraceID    string
	SpanID     string
	Name       string
	Start      time.Time
	End        time.Time
	DurationMS float64
	Attrs      map[string]string
}

// LogRecord is a flattened, MCP-friendly snapshot of a slog record.
type LogRecord struct {
	Time    time.Time
	Level   string
	Msg     string
	TraceID string
	SpanID  string
	Attrs   map[string]string
}

// MemoryExporter is an sdktrace.SpanExporter backed by a fixed-capacity ring
// buffer of finished spans. It is always installed regardless of OTLP
// configuration so the debug MCP always has recent spans to inspect.
type MemoryExporter struct {
	mu       sync.RWMutex
	capacity int
	buf      []SpanRecord // ring buffer storage
	next     int          // index the next span is written to
	count    int          // number of valid entries in buf (caps at capacity)
}

// NewMemoryExporter creates a MemoryExporter that retains up to capacity
// spans. Capacity <= 0 is treated as 1 to guarantee the buffer is usable.
func NewMemoryExporter(capacity int) *MemoryExporter {
	if capacity <= 0 {
		capacity = 1
	}
	return &MemoryExporter{
		capacity: capacity,
		buf:      make([]SpanRecord, capacity),
	}
}

// ExportSpans implements sdktrace.SpanExporter.
func (e *MemoryExporter) ExportSpans(_ context.Context, spans []sdktrace.ReadOnlySpan) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, s := range spans {
		e.buf[e.next] = toSpanRecord(s)
		e.next = (e.next + 1) % e.capacity
		if e.count < e.capacity {
			e.count++
		}
	}
	return nil
}

// Shutdown implements sdktrace.SpanExporter. It is a no-op: the buffer is
// in-memory and must remain readable by the debug MCP after shutdown.
func (e *MemoryExporter) Shutdown(_ context.Context) error { return nil }

// Spans returns all retained spans, oldest first.
func (e *MemoryExporter) Spans() []SpanRecord {
	e.mu.RLock()
	defer e.mu.RUnlock()
	out := make([]SpanRecord, 0, e.count)
	start := e.next - e.count
	for i := 0; i < e.count; i++ {
		idx := (start + i) % e.capacity
		if idx < 0 {
			idx += e.capacity
		}
		out = append(out, e.buf[idx])
	}
	return out
}

// Trace returns all retained spans belonging to the given trace ID, oldest
// first.
func (e *MemoryExporter) Trace(traceID string) []SpanRecord {
	all := e.Spans()
	out := make([]SpanRecord, 0, len(all))
	for _, s := range all {
		if s.TraceID == traceID {
			out = append(out, s)
		}
	}
	return out
}

func toSpanRecord(s sdktrace.ReadOnlySpan) SpanRecord {
	sc := s.SpanContext()
	start := s.StartTime()
	end := s.EndTime()

	attrs := make(map[string]string, len(s.Attributes()))
	for _, kv := range s.Attributes() {
		attrs[string(kv.Key)] = kv.Value.String()
	}

	return SpanRecord{
		TraceID:    sc.TraceID().String(),
		SpanID:     sc.SpanID().String(),
		Name:       s.Name(),
		Start:      start,
		End:        end,
		DurationMS: float64(end.Sub(start).Microseconds()) / 1000.0,
		Attrs:      attrs,
	}
}

// ringLogCore holds the ring buffer storage and its mutex. RingLog and every
// handler derived from it via WithAttrs/WithGroup share a single *ringLogCore
// pointer, so all of them write into the same synchronized ring — matching
// slog.Handler's contract that .With(...) returns a handler for the same
// underlying sink, only preformatted attrs/groups differ. Cloning buf/next/
// count per-handler (as an earlier version of this file did) let a parent
// and a derived handler race on two independent mutexes over one backing
// array, silently corrupting/losing records; see the S7 review fix.
type ringLogCore struct {
	mu       sync.RWMutex
	capacity int
	buf      []LogRecord
	next     int
	count    int
}

// RingLog is a slog.Handler backed by a fixed-capacity ring buffer of log
// records. It injects trace_id/span_id from the record's context when a
// valid span is present, so a log line can be correlated back to a trace
// captured by MemoryExporter.
type RingLog struct {
	core   *ringLogCore
	attrs  []slog.Attr
	groups []string
}

// NewRingLog creates a RingLog that retains up to capacity log records.
// Capacity <= 0 is treated as 1 to guarantee the buffer is usable.
func NewRingLog(capacity int) *RingLog {
	if capacity <= 0 {
		capacity = 1
	}
	return &RingLog{
		core: &ringLogCore{
			capacity: capacity,
			buf:      make([]LogRecord, capacity),
		},
	}
}

// Enabled implements slog.Handler. RingLog always records; level filtering
// happens on read via Records.
func (r *RingLog) Enabled(_ context.Context, _ slog.Level) bool { return true }

// Handle implements slog.Handler.
func (r *RingLog) Handle(ctx context.Context, rec slog.Record) error {
	attrs := make(map[string]string)
	for _, a := range r.attrs {
		attrs[a.Key] = a.Value.String()
	}
	rec.Attrs(func(a slog.Attr) bool {
		attrs[a.Key] = a.Value.String()
		return true
	})

	var traceID, spanID string
	if sc := spanContextFromContext(ctx); sc.valid {
		traceID = sc.traceID
		spanID = sc.spanID
	}

	lr := LogRecord{
		Time:    rec.Time,
		Level:   rec.Level.String(),
		Msg:     rec.Message,
		TraceID: traceID,
		SpanID:  spanID,
		Attrs:   attrs,
	}

	c := r.core
	c.mu.Lock()
	c.buf[c.next] = lr
	c.next = (c.next + 1) % c.capacity
	if c.count < c.capacity {
		c.count++
	}
	c.mu.Unlock()
	return nil
}

// WithAttrs implements slog.Handler. The returned handler shares this
// RingLog's core (same buffer, same mutex) and only carries its own
// preformatted attrs, so records logged through either handler land in the
// same ring, in write order.
func (r *RingLog) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &RingLog{
		core:   r.core,
		attrs:  append(append([]slog.Attr(nil), r.attrs...), attrs...),
		groups: r.groups,
	}
}

// WithGroup implements slog.Handler. hostus does not currently nest
// attributes under groups for the debug ring; the group name is tracked but
// does not prefix keys, keeping Records() output flat and simple to query.
// The returned handler shares this RingLog's core, for the same reason as
// WithAttrs.
func (r *RingLog) WithGroup(name string) slog.Handler {
	return &RingLog{
		core:   r.core,
		attrs:  r.attrs,
		groups: append(append([]string(nil), r.groups...), name),
	}
}

// Records returns up to limit retained log records at or above min level,
// newest first. limit <= 0 means unlimited.
func (r *RingLog) Records(min slog.Level, limit int) []LogRecord {
	c := r.core
	c.mu.RLock()
	defer c.mu.RUnlock()

	out := make([]LogRecord, 0, c.count)
	start := c.next - c.count
	for i := c.count - 1; i >= 0; i-- {
		idx := (start + i) % c.capacity
		if idx < 0 {
			idx += c.capacity
		}
		rec := c.buf[idx]
		lvl, err := parseLevel(rec.Level)
		if err != nil || lvl < min {
			continue
		}
		out = append(out, rec)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out
}

func parseLevel(s string) (slog.Level, error) {
	var lvl slog.Level
	err := lvl.UnmarshalText([]byte(s))
	return lvl, err
}

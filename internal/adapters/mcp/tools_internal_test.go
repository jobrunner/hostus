package mcp

import (
	"testing"

	"github.com/jobrunner/hostus/internal/adapters/telemetry"
)

// TestResolveLimit pins the exact boundary between "use the caller's limit"
// and "fall back to the tool's default": 0 and negative values must fall
// back, and 1 (the smallest positive value) must NOT.
func TestResolveLimit(t *testing.T) {
	cases := []struct {
		name      string
		requested int
		def       int
		want      int
	}{
		{"zero falls back to default", 0, 50, 50},
		{"negative falls back to default", -1, 50, 50},
		{"one is honored, not treated as falsy", 1, 50, 1},
		{"positive below default is honored", 5, 50, 5},
		{"positive equal to default is honored", 50, 50, 50},
		{"positive above default is honored", 200, 50, 200},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := resolveLimit(tc.requested, tc.def); got != tc.want {
				t.Fatalf("resolveLimit(%d, %d) = %d, want %d", tc.requested, tc.def, got, tc.want)
			}
		})
	}
}

// TestFilterSpansDurationBoundary pins the "at least" semantics of
// min_duration_ms: a span whose duration exactly equals the threshold must
// be included (>=), not excluded (>).
func TestFilterSpansDurationBoundary(t *testing.T) {
	all := []telemetry.SpanRecord{
		{Name: "exactly-at-threshold", DurationMS: 10.0},
		{Name: "just-under-threshold", DurationMS: 9.999},
	}

	got := filterSpans(all, "", 10.0, 50)

	if len(got) != 1 {
		t.Fatalf("got %d spans, want exactly 1 (only the >= threshold span): %+v", len(got), got)
	}
	if got[0].Name != "exactly-at-threshold" {
		t.Fatalf("got span %q, want the span exactly at the threshold to be included", got[0].Name)
	}
}

// TestFilterSpansOperationFilter pins the substring-match semantics
// (case-sensitive Contains, not exact-match, not prefix-only) and confirms
// an empty operation matches everything.
func TestFilterSpansOperationFilter(t *testing.T) {
	all := []telemetry.SpanRecord{
		{Name: "fetch-gbif-species", DurationMS: 1},
		{Name: "cache-lookup", DurationMS: 1},
	}

	got := filterSpans(all, "gbif", 0, 50)
	if len(got) != 1 || got[0].Name != "fetch-gbif-species" {
		t.Fatalf("operation substring filter: got %+v, want only fetch-gbif-species", got)
	}

	gotAll := filterSpans(all, "", 0, 50)
	if len(gotAll) != 2 {
		t.Fatalf("empty operation filter: got %d spans, want all 2", len(gotAll))
	}
}

// TestFilterSpansOrderAndLimit pins two things at once: results come back
// newest-first (reverse of the oldest-first input MemoryExporter.Spans
// produces), and the limit cutoff is an inclusive count (len >= limit stops
// exactly at limit, not limit-1 or limit+1).
func TestFilterSpansOrderAndLimit(t *testing.T) {
	all := []telemetry.SpanRecord{
		{Name: "oldest", DurationMS: 0},
		{Name: "middle", DurationMS: 0},
		{Name: "newest", DurationMS: 0},
	}

	got := filterSpans(all, "", 0, 2)
	if len(got) != 2 {
		t.Fatalf("got %d spans, want exactly 2 (limit)", len(got))
	}
	if got[0].Name != "newest" || got[1].Name != "middle" {
		t.Fatalf("got order %+v, want [newest, middle]", got)
	}
}

// TestFilterSpansEmptyInput confirms an empty buffer produces an empty (not
// nil-panicking, not out-of-range) result.
func TestFilterSpansEmptyInput(t *testing.T) {
	got := filterSpans(nil, "", 0, 50)
	if len(got) != 0 {
		t.Fatalf("got %d spans from empty input, want 0", len(got))
	}
}

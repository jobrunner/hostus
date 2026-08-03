package telemetry

import (
	"context"
	"strings"
	"testing"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"

	"github.com/jobrunner/hostus/internal/config"
)

// TestOTLPEnabled pins the exact boolean logic gating OTLP exporter
// construction: both Enabled and a non-empty Endpoint are required. Each of
// the four Enabled x Endpoint-empty combinations is asserted so an
// accidental negation of either operand is caught.
func TestOTLPEnabled(t *testing.T) {
	tests := []struct {
		name     string
		enabled  bool
		endpoint string
		want     bool
	}{
		{"disabled, no endpoint", false, "", false},
		{"disabled, with endpoint", false, "collector:4318", false},
		{"enabled, no endpoint", true, "", false},
		{"enabled, with endpoint", true, "collector:4318", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := otlpEnabled(config.TelemetryConfig{Enabled: tc.enabled, Endpoint: tc.endpoint})
			if got != tc.want {
				t.Errorf("otlpEnabled(Enabled=%v, Endpoint=%q) = %v, want %v", tc.enabled, tc.endpoint, got, tc.want)
			}
		})
	}
}

// samplingDecision runs the sampler as if it were deciding on a root span
// (no parent in ctx) and returns the resulting decision. ParentBased
// delegates root-span decisions to the configured "root" sampler, so this
// isolates exactly the behavior buildSampler configures.
func samplingDecision(t *testing.T, s sdktrace.Sampler) sdktrace.SamplingDecision {
	t.Helper()
	res := s.ShouldSample(sdktrace.SamplingParameters{
		ParentContext: context.Background(),
		TraceID:       trace.TraceID{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		Name:          "test",
		Kind:          trace.SpanKindInternal,
	})
	return res.Decision
}

// TestBuildSamplerBoundaries pins the ratio<=0 / ratio>=1.0 boundaries so a
// boundary or negation mutation on either comparison is caught: ratio 0 and
// negative must never sample, ratio 1 and above must always sample, and
// anything strictly between must use ratio-based sampling (neither of the
// fixed decisions, deterministically, for both endpoints of that range).
//
// At the exact boundaries (ratio 0 and ratio 1.0), TraceIDRatioBased happens
// to make the *same sampling decision* as NeverSample/AlwaysSample (0% and
// 100% probability respectively) — so a boundary mutation that swapped
// NeverSample()/AlwaysSample() for TraceIDRatioBased() at exactly 0 or 1
// would be invisible to a decision-only assertion. Description() exposes
// the underlying sampler type/probability distinctly (e.g.
// "root:AlwaysOffSampler" vs "root:TraceIDRatioBased{0}"), so boundary
// assertions compare that instead.
func TestBuildSamplerBoundaries(t *testing.T) {
	if got := samplingDecision(t, buildSampler(-1)); got != sdktrace.Drop {
		t.Errorf("ratio -1: decision = %v, want Drop (never sample)", got)
	}
	if got := samplingDecision(t, buildSampler(2)); got != sdktrace.RecordAndSample {
		t.Errorf("ratio 2: decision = %v, want RecordAndSample (always sample)", got)
	}

	if got, want := buildSampler(0).Description(), "root:AlwaysOffSampler"; !strings.Contains(got, want) {
		t.Errorf("ratio 0 (boundary): sampler description = %q, want it to contain %q (never-sample root)", got, want)
	}
	if got, want := buildSampler(1).Description(), "root:AlwaysOnSampler"; !strings.Contains(got, want) {
		t.Errorf("ratio 1 (boundary): sampler description = %q, want it to contain %q (always-sample root)", got, want)
	}
	// ParentBased's Description() always mentions AlwaysOffSampler for its
	// fixed remoteParentNotSampled/localParentNotSampled components, so the
	// "must not be never-sample" check has to look at the "root:" segment
	// specifically rather than the whole string.
	if got, notWant := buildSampler(0.5).Description(), "root:AlwaysOffSampler"; strings.Contains(got, notWant) {
		t.Errorf("ratio 0.5: sampler description = %q, must not use the never-sample root", got)
	}
	if got, want := buildSampler(0.5).Description(), "root:TraceIDRatioBased"; !strings.Contains(got, want) {
		t.Errorf("ratio 0.5: sampler description = %q, want it to contain %q (ratio-based root)", got, want)
	}
}

package telemetry

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"

	"github.com/jobrunner/hostus/internal/config"
)

// serviceName identifies hostus spans/metrics in any configured backend.
const serviceName = "hostus"

// spanBufferCapacity and logBufferCapacity size the always-on in-memory
// ring buffers consumed by the debug MCP surface (task S10).
const (
	spanBufferCapacity = 512
	logBufferCapacity  = 1024
)

// Providers bundles the wired trace/metric providers plus the always-on
// in-memory exporters, so callers (app composition root, debug MCP) can
// reach into either.
type Providers struct {
	TracerProvider *sdktrace.TracerProvider
	MeterProvider  *sdkmetric.MeterProvider
	Memory         *MemoryExporter
	Log            *RingLog
}

// spanContext is a tiny trace-id/span-id pair, decoupled from the OTel
// trace package so memory.go (which must stay easy to unit-test without an
// SDK dependency creeping into slog wiring) only needs this shape.
type spanContext struct {
	valid   bool
	traceID string
	spanID  string
}

// spanContextFromContext extracts the current span's identifiers from ctx,
// if any.
func spanContextFromContext(ctx context.Context) spanContext {
	sc := trace.SpanContextFromContext(ctx)
	if !sc.IsValid() {
		return spanContext{}
	}
	return spanContext{
		valid:   true,
		traceID: sc.TraceID().String(),
		spanID:  sc.SpanID().String(),
	}
}

// Setup wires the OTel trace and metric providers. The in-memory span
// exporter (MemoryExporter) and log ring (RingLog) are always installed,
// regardless of telemetry configuration, so the debug MCP always has data
// to inspect. The OTLP exporters are only installed when telemetry is
// enabled and an endpoint is configured.
//
// The returned shutdown func flushes and closes the providers; it is safe
// to call even if OTLP was never configured.
func Setup(ctx context.Context, cfg *config.Config) (*Providers, func(context.Context) error, error) {
	mem := NewMemoryExporter(spanBufferCapacity)
	ringLog := NewRingLog(logBufferCapacity)

	res, err := resource.New(ctx, resource.WithAttributes(semconv.ServiceName(serviceName)))
	if err != nil {
		return nil, nil, fmt.Errorf("building telemetry resource: %w", err)
	}

	sampler := buildSampler(cfg.Telemetry.SampleRatio)

	tpOpts := []sdktrace.TracerProviderOption{
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sampler),
		sdktrace.WithSpanProcessor(sdktrace.NewSimpleSpanProcessor(mem)),
	}

	mpOpts := []sdkmetric.Option{sdkmetric.WithResource(res)}

	if otlpEnabled(cfg.Telemetry) {
		traceExp, err := otlptracehttp.New(ctx, otlptracehttp.WithEndpoint(cfg.Telemetry.Endpoint))
		if err != nil {
			return nil, nil, fmt.Errorf("building OTLP trace exporter: %w", err)
		}
		tpOpts = append(tpOpts, sdktrace.WithSpanProcessor(sdktrace.NewBatchSpanProcessor(traceExp)))

		metricExp, err := otlpmetrichttp.New(ctx, otlpmetrichttp.WithEndpoint(cfg.Telemetry.Endpoint))
		if err != nil {
			return nil, nil, fmt.Errorf("building OTLP metric exporter: %w", err)
		}
		mpOpts = append(mpOpts, sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExp)))
	}

	tp := sdktrace.NewTracerProvider(tpOpts...)
	mp := sdkmetric.NewMeterProvider(mpOpts...)

	otel.SetTracerProvider(tp)
	otel.SetMeterProvider(mp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	shutdown := func(ctx context.Context) error {
		var errs []error
		if err := tp.Shutdown(ctx); err != nil {
			errs = append(errs, fmt.Errorf("shutting down tracer provider: %w", err))
		}
		if err := mp.Shutdown(ctx); err != nil {
			errs = append(errs, fmt.Errorf("shutting down meter provider: %w", err))
		}
		if len(errs) > 0 {
			return fmt.Errorf("telemetry shutdown: %v", errs)
		}
		return nil
	}

	return &Providers{
		TracerProvider: tp,
		MeterProvider:  mp,
		Memory:         mem,
		Log:            ringLog,
	}, shutdown, nil
}

// otlpEnabled reports whether Setup should wire the OTLP exporters: both
// telemetry.enabled and a non-empty endpoint are required. Broken out as a
// named function (rather than inlined into Setup) so the boolean logic is
// directly unit-testable without spinning up a full provider.
func otlpEnabled(cfg config.TelemetryConfig) bool {
	return cfg.Enabled && cfg.Endpoint != ""
}

// buildSampler maps a 0..1 sample ratio onto a parent-based OTel sampler.
// Ratio <= 0 never samples (root spans only start when a parent already
// decided to); ratio >= 1 always samples; anything in between uses
// trace-ID-ratio sampling.
func buildSampler(ratio float64) sdktrace.Sampler {
	if ratio <= 0 {
		return sdktrace.ParentBased(sdktrace.NeverSample())
	}
	if ratio >= 1.0 {
		return sdktrace.ParentBased(sdktrace.AlwaysSample())
	}
	return sdktrace.ParentBased(sdktrace.TraceIDRatioBased(ratio))
}

# Observability

## Prometheus-Metriken

`GET /metrics` liefert Metriken im Prometheus-Text-Format, instrumentiert
über die `Metrics`-Middleware (letztes Glied der Middleware-Chain).

## Tracing

Der Router ist vollständig in eine `otelmux`-Span gewrappt (OpenTelemetry),
sodass jeder Request end-to-end nachverfolgbar ist, sobald ein
Telemetry-Backend konfiguriert ist (siehe `telemetry.*` in der
[Konfiguration](configuration.md)).

## Middleware-Reihenfolge

Die Reihenfolge ist eine bewusste, unveränderliche Randbedingung (siehe
[Architektur](../explanation/architecture.md)):

1. Request-ID
2. Logging
3. Rate-Limiting
4. Load-Shedding
5. Timeouts
6. CORS
7. Metrics

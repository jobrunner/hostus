# ADR-0013: OpenTelemetry von Tag 1

**Status:** Accepted

## Kontext

ortus als Referenz-Harness hat OTel erst nachträglich ergänzt. Für hostus 2.0
wurde entschieden, volle ortus-Parität zu übernehmen (siehe Spec Abschnitt
5.1), aber Observability von Anfang an mitzubauen statt sie nachzurüsten —
insbesondere, weil der neue Ingest-/Index-Pfad (SQLite-Queries, ColDP-Parsing,
Backbone-Ingest) mehr Fehlerfläche hat als der bisherige reine HTTP-Proxy und
weil ein Debug-MCP für Claude Code (ADR-0014) auf strukturierten Traces
aufsetzt.

## Entscheidung

hostus instrumentiert Traces und Metrics ab SP0 mit dem OpenTelemetry Go SDK,
OTLP-Exportern (http/grpc), `otelhttp`/`otelmux`-Instrumentierung der
Middleware-Kette sowie einem `slog`-Handler, der `trace_id`/`span_id` aus dem
Span-Kontext in jede Log-Zeile schreibt. Der Prometheus-Client bleibt
zusätzlich für den `/metrics`-Endpunkt bestehen (kein Ersatz, sondern
Ergänzung — Prometheus bedient das etablierte Scraping, OTel liefert Traces
und ergänzende Metrics/Export-Pfade).

## Konsequenzen

- Middleware-Kette (Request-ID → Logging → Rate-Limiting → Load-Shedding →
  Timeouts → CORS → Metrics) ist durchgängig OTel-instrumentiert; neue
  Handler erben Tracing ohne Zusatzaufwand über `otelmux`.
- Der Debug-MCP (ADR-0014) kann auf einen In-Memory-Span-Exporter parallel
  zum OTLP-Exporter zurückgreifen, weil die Instrumentierung von Anfang an
  vorhanden ist, statt nachträglich in bestehenden Code eingezogen werden zu
  müssen.
- Zusätzliche Laufzeitabhängigkeiten (`go.opentelemetry.io/otel`, `.../sdk`,
  `.../exporters/otlp/...`, `otelmux`) sind Teil der „Allowed Libraries" ab
  hostus 2.0 (siehe `CLAUDE.md`).
- Kein separater Last-Test-/Tracing-Stack (Grafana/Tempo/Loki) — der bewusst
  weggelassen wurde (Spec Abschnitt 5.2); OTel-Daten werden lokal über den
  Debug-MCP und/oder einen extern konfigurierten OTLP-Collector konsumiert.

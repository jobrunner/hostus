# Architektur

hostus ist ein Go-Backend-Service, der als schreibgeschützter
Taxonomie-Gateway zwischen einem Frontend-Autosuggest-Feld und der
GBIF-REST-API vermittelt:

```
Frontend → hostus (dieser Service) → GBIF-REST-API
```

## Kernverantwortlichkeiten

- Proxy-Zugriff auf GBIF `/v1/species/search`
- In-Memory-Caching mit TTL
- Gruppierung von Synonymen unter akzeptierten Taxa
- Rate-Limiting und Load-Shedding zum Schutz des Upstreams

## Hexagonale Struktur

Der Code folgt Ports & Adapters:

- `internal/domain` — Domänenmodelle, frei von Infrastruktur-Details
- `internal/ports/input` / `internal/ports/output` — Schnittstellen nach außen bzw. zu Abhängigkeiten
- `internal/application` — Use Cases, die Domäne und Ports orchestrieren
- `internal/adapters/http` — HTTP-Adapter (Router, Health, Metrics)
- `internal/adapters/mcp` — MCP-Adapter (Model Context Protocol)
- `internal/adapters/telemetry` — Tracing/Metrics-Adapter
- `internal/app` — verdrahtet alle Bausteine zur laufenden Anwendung zusammen

## Middleware-Kette

Die Reihenfolge ist eine bewusste, unveränderliche Randbedingung:

1. **Request-ID** — generiert eine ID für Tracing
2. **Logging** — loggt Request/Response
3. **Rate-Limiting** — schützt vor Überlastung
4. **Load-Shedding** — Circuit Breaker für den Upstream
5. **Timeout** — begrenzt die Request-Laufzeit
6. **CORS** — Cross-Origin-Handling
7. **Metrics** — Prometheus-Instrumentierung

## Synonym-Gruppierung (SP1+)

GBIF liefert eine flache Liste von Taxa. Die künftige Mapper-Logik:

1. gruppiert alle Taxa nach `acceptedKey`,
2. macht Taxa mit `status=ACCEPTED` zu Haupteinträgen,
3. bettet Synonyme unter ihrem akzeptierten Taxon ein.

## Load-Shedding

Schützt vor Kaskadenfehlern:

1. zählt aufeinanderfolgende GBIF-Fehler,
2. ab einem Schwellwert: Fail-Fast ohne Upstream-Call → `503`,
3. nach einem Backoff: ein Probe-Request wird wieder zugelassen,
4. bei Erfolg: Reset des Zählers.

Weiterführende Entscheidungen und ihre Begründung stehen in den
[ADRs](decisions/index.md) und in `architecture/adrs.md` im Repository-Root.

# Architektur

hostus ist ein Go-Backend-Service: ein lokaler Namens- und Merkmalsdienst für
Gefäßpflanzen auf Basis eines eigenen Multi-Backbone-Index, kein zustandsloser
Proxy vor einer einzelnen Upstream-API:

```
Frontend → hostus (dieser Service, lokaler SQLite/FTS5-Index) → optionale
           Ingest-/Enrichment-Quellen (COL XR, WCVP/POWO, Euro+Med,
           FloraVeg.EU, GBIF, ...) — nur im Ingest-Pfad, nicht zur Laufzeit
```

## Kernverantwortlichkeiten

- Bedienung eines lokalen, versionierten Multi-Backbone-Index (SQLite/FTS5),
  gespeist aus fixierten Backbone-/Trait-Artefakten — kein Live-Passthrough
- Gruppierung von Synonymen unter akzeptierten Taxa
- Rate-Limiting und Load-Shedding zum Schutz der Ingest-/Enrichment-Pfade,
  die weiterhin externe APIs aufrufen

## Hexagonale Struktur

Der Code folgt Ports & Adapters:

- `internal/domain` — Domänenmodelle, frei von Infrastruktur-Details
- `internal/ports/input` / `internal/ports/output` — Schnittstellen nach außen bzw. zu Abhängigkeiten
- `internal/application` — Use Cases, die Domäne und Ports orchestrieren
- `internal/adapters/sqlite` — SQLite/FTS5-Repository für den lokalen Index (`modernc.org/sqlite`)
- `internal/adapters/coldp` — ColDP-Importer für den Ingest-Pfad
- `internal/adapters/http` — HTTP-Adapter (Router, Health, Metrics)
- `internal/adapters/mcp` — MCP-Adapter (Model Context Protocol)
- `internal/adapters/telemetry` — Tracing/Metrics-Adapter
- `internal/adapters/bundle` — Offline-Bundle-Export (gefilterte SQLite-Kopie)
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

Der Ingest-Pfad importiert Concept-/Name-Relationen (homotypisch/heterotypisch)
aus den Backbone-Quellen in den lokalen Index. Die künftige Mapper-Logik:

1. gruppiert alle Namen nach akzeptiertem Concept,
2. macht den akzeptierten Namen zum Haupteintrag,
3. bettet Synonyme unter ihrem akzeptierten Taxon ein.

## Load-Shedding

Schützt die Ingest-/Enrichment-Pfade (die weiterhin externe APIs aufrufen)
vor Kaskadenfehlern:

1. zählt aufeinanderfolgende Upstream-Fehler,
2. ab einem Schwellwert: Fail-Fast ohne Upstream-Call → `503`,
3. nach einem Backoff: ein Probe-Request wird wieder zugelassen,
4. bei Erfolg: Reset des Zählers.

Weiterführende Entscheidungen und ihre Begründung stehen in den
[ADRs](decisions/index.md) und in `architecture/adrs.md` im Repository-Root.

# Architecture Decision Records

Jede ADR hält eine wesentliche Entscheidung, ihren Kontext und ihre
Konsequenzen fest.

## hostus 2.0 (SP0, aktuell)

- **[ADR-0009: Lokaler Multi-Backbone-Index](0009-local-multibackbone-index.md)**
  — ersetzt GBIF-als-alleiniger-Provider und "kein Persistenz-Layer".
- **[ADR-0010: SQLite/FTS5-Persistenz via `modernc.org/sqlite`](0010-sqlite-fts5-persistence.md)**
  — CGO-freier Storage-/Such-Layer für den lokalen Index.
- **[ADR-0011: Versionierter Artefaktvertrag](0011-versioned-artifact-contract.md)**
  — `dataset.yaml` + eingebettetes JSON Schema, doppelt validiert.
- **[ADR-0012: Hexagonale Architektur](0012-hexagonal-architecture.md)**
  — Ports & Adapters, Grenzen per Linter erzwungen.
- **[ADR-0013: OpenTelemetry von Tag 1](0013-opentelemetry-from-day-one.md)**
  — Traces + Metrics ab SP0, kein Nachrüsten.
- **[ADR-0014: stdio Debug-MCP](0014-stdio-debug-mcp.md)**
  — read-only Logs/Spans für Claude Code, ohne Netzwerk-Port.

## hostus 1.x (historisch, teils superseded)

Die ADR-001 bis ADR-008 aus der ursprünglichen GBIF-Proxy-Architektur liegen
weiterhin in `architecture/adrs.md` im Repository-Root. ADR-001, ADR-003 und
ADR-008 sind dort als **Superseded** markiert (siehe ADR-0009 oben); die
übrigen (Go + Minimal-Dependencies, Code-First-OpenAPI, Distroless-Container,
Releases nur via Feature-Merge) gelten fort und wurden für hostus 2.0 im
Wortlaut reconciled.

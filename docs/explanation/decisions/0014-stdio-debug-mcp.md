# ADR-0014: stdio Debug-MCP für Claude Code

**Status:** Accepted

## Kontext

Claude Code arbeitet als Entwicklungsagent an hostus und braucht während
Implementierung und Debugging Zugriff auf Laufzeit-Signale (Logs, Traces),
ohne einen externen Observability-Stack (Grafana/Tempo/Loki) betreiben zu
müssen — dieser Last-Test-/Tracing-Stack wurde bewusst weggelassen (Spec
Abschnitt 5.2). Gleichzeitig soll kein Netzwerk-Port für Debug-Zwecke
exponiert werden, der Auth/Absicherung bräuchte.

## Entscheidung

hostus bekommt ein zusätzliches `hostus mcp`-Subcommand, das einen
**stdio-basierten MCP-Server** (`github.com/modelcontextprotocol/go-sdk`)
startet und **read-only** Zugriff auf Logs und Spans bietet:

- `get_recent_logs(level?, limit?)` — Ringpuffer-Logs
- `tail_errors(limit?)` — nur Fehler/Warnungen
- `get_trace(trace_id)` — vollständiger Span-Baum eines Traces
- `list_spans(operation?, min_duration?, limit?)` — Spans filtern (z. B.
  Slow-Query-Suche)

Umgesetzt über einen In-Memory-Ringpuffer-Log-Handler und einen In-Memory-
Span-Exporter, die parallel zum regulären OTLP-Exporter (ADR-0013) laufen.

## Konsequenzen

- Kein zusätzlicher Netzwerk-Port, keine Auth nötig — stdio ist lokal an den
  aufrufenden Claude-Code-Prozess gebunden.
- Debug-MCP ist strikt read-only; er kann keine Ingest-/Schreib-Operationen
  auslösen und ist damit kein zusätzlicher Angriffsvektor auf den Index.
- Zusätzlicher Laufzeit-Overhead durch die In-Memory-Ringpuffer (Log + Span)
  ist bewusst klein zu halten (begrenzte Kapazität, kein unbegrenztes
  Wachstum).
- `github.com/modelcontextprotocol/go-sdk` wird Teil der „Allowed Libraries"
  ab hostus 2.0 (siehe `CLAUDE.md`).

# HTTP-API

!!! note "Stand"
    Dies beschreibt den aktuellen SP0-Skeleton-Stand. Die eigentlichen
    `/v1/*`-Endpunkte (`suggest`, `match`, `concept/{id}`, `xref`,
    `concept/{id}/traits`, `concept/{id}/synonyms`, `translate`) sowie
    `/openapi` folgen in SP1+ (siehe CLAUDE.md für den Build-Plan); die
    OpenAPI-Baseline dafür liegt bereits unter `api/openapi/openapi.yaml`
    und wird dann code-generiert erweitert.

## Health-Endpunkte

### `GET /health/live`

Liveness-Probe. Antwortet `200 OK`, solange der Prozess HTTP bedienen kann —
unabhängig vom Zustand nachgelagerter Abhängigkeiten.

### `GET /health/ready`

Readiness-Probe. Im aktuellen Skeleton identisch zu `/health/live` (es gibt
noch keine Abhängigkeiten wie GBIF-Erreichbarkeit oder Cache-Warm-up zu
prüfen). SP1 ergänzt echte Checks.

## Metrics-Endpunkt

### `GET /metrics`

Prometheus-Metriken im Text-Exposition-Format (`text/plain`). Siehe
[Observability](observability.md) für die Details der Middleware-Chain.

## Fehlerformat

Sobald Fach-Endpunkte hinzukommen, werden Fehler einheitlich als JSON
zurückgegeben:

```json
{
  "error": {
    "code": "ERROR_CODE",
    "message": "Human readable message"
  }
}
```

| Code                  | HTTP | Beschreibung                                          |
|-----------------------|------|--------------------------------------------------------|
| `INVALID_QUERY`       | 400  | Ungültiger Query-Parameter                             |
| `RATE_LIMIT_EXCEEDED` | 429  | Rate-Limit überschritten                                |
| `UPSTREAM_OVERLOADED` | 503  | Load-Shedding aktiv                                     |
| `NOT_FOUND`           | 404  | Unbekannte Concept-/Xref-ID                             |
| `UNRESOLVABLE`        | (SP1+) | Verbatim-Name lässt sich keinem Concept zuordnen; HTTP-Status wird mit dem jeweiligen Endpunkt in SP1+ festgelegt |
| `GBIF_TIMEOUT`        | 504  | GBIF-Anfrage Timeout (nur Ingest-/Enrichment-Pfad)      |
| `GBIF_UNAVAILABLE`    | 502  | GBIF nicht erreichbar (nur Ingest-/Enrichment-Pfad)     |
| `INTERNAL_ERROR`      | 500  | Interner Serverfehler                                   |

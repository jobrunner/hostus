# HTTP-API

!!! note "Stand"
    Dies beschreibt den aktuellen SP0-Skeleton-Stand. Der eigentliche
    Autosuggest-Endpunkt (`GET /api/v1/taxa/suggest`) sowie `/openapi` folgen
    in SP1+; die OpenAPI-Baseline dafür liegt bereits unter
    `api/openapi/openapi.yaml` und wird dann code-generiert erweitert.

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

| Code                  | HTTP | Beschreibung                       |
|-----------------------|------|-------------------------------------|
| `INVALID_QUERY`       | 400  | Ungültiger Query-Parameter          |
| `RATE_LIMIT_EXCEEDED` | 429  | Rate-Limit überschritten            |
| `UPSTREAM_OVERLOADED` | 503  | Load-Shedding aktiv                 |
| `GBIF_TIMEOUT`        | 504  | GBIF-Anfrage Timeout                |
| `GBIF_UNAVAILABLE`    | 502  | GBIF nicht erreichbar               |
| `INTERNAL_ERROR`      | 500  | Interner Serverfehler               |

# Hostus

Ein hochperformanter Backend-Service für Taxonomie-Autosuggest von Gefäßpflanzen.

## Überblick

Hostus ist ein Gateway-Service, der die GBIF API für Pflanzentaxonomie proxied und dabei:

- Ergebnisse im Speicher cached
- Synonyme unter akzeptierten Taxa gruppiert
- Rate-Limiting und Load-Shedding für Upstream-Schutz bietet
- Eine stabile, frontend-optimierte REST-API bereitstellt

## API

### Autosuggest Endpoint

```http
GET /api/v1/taxa/suggest?q={query}&limit={n}
```

| Parameter | Typ    | Pflicht | Beschreibung                                |
|-----------|--------|---------|---------------------------------------------|
| q         | string | ja      | Suchstring (min. 3 Zeichen)                 |
| limit     | int    | nein    | Max. Anzahl Ergebnisse (Standard: 20, Max: 100) |

#### Beispiel-Response

```json
[
  {
    "acceptedKey": 2704178,
    "acceptedName": "Schoenoplectus lacustris",
    "rank": "SPECIES",
    "family": "Cyperaceae",
    "synonyms": [
      {
        "key": 5298174,
        "name": "Scirpus lacustris",
        "status": "SYNONYM"
      }
    ]
  }
]
```

### Weitere Endpoints

| Endpoint    | Beschreibung                    |
|-------------|---------------------------------|
| `/openapi`  | OpenAPI 3.0 Spezifikation       |
| `/metrics`  | Prometheus Metriken             |
| `/health`   | Health-Check                    |

## Konfiguration

Alle Parameter können via `.env`-Datei, Umgebungsvariablen oder CLI-Parameter gesetzt werden.

| Parameter                   | Standard                    | Beschreibung                           |
|-----------------------------|-----------------------------|----------------------------------------|
| `PORT`                      | 8080                        | Server-Port                            |
| `HOST_NAME`                 | localhost                   | Hostname für TLS                       |
| `ENABLE_TLS`                | false                       | HTTPS aktivieren                       |
| `CORS_ORIGINS`              | *                           | Erlaubte CORS-Origins                  |
| `RATE_LIMIT`                | 100                         | Requests pro Sekunde                   |
| `UPSTREAM_ERROR_THRESHOLD`  | 5                           | Fehler bis Load-Shedding               |
| `UPSTREAM_BACKOFF_SECONDS`  | 30                          | Backoff nach Load-Shedding             |
| `CACHE_TTL_SECONDS`         | 300                         | Cache-Lebensdauer                      |
| `LOG_LEVEL`                 | info                        | Log-Level (debug/info/warn/error)      |

### CLI-Beispiel

```bash
./hostus --port=8080 --rate-limit=50 --log-level=debug
```

## Schnellstart

### Mit Docker

```bash
docker run -p 8080:8080 ghcr.io/jobrunner/hostus:latest
```

### Mit Docker Compose

```bash
cp example.env .env
docker-compose up
```

### Lokal

```bash
make build
./hostus
```

## Fehlerformat

Alle Fehler werden einheitlich als JSON zurückgegeben:

```json
{
  "error": {
    "code": "ERROR_CODE",
    "message": "Fehlerbeschreibung"
  }
}
```

### Fehlercodes

| Code                  | HTTP | Beschreibung                       |
|-----------------------|------|------------------------------------|
| `INVALID_QUERY`       | 400  | Ungültiger Query-Parameter         |
| `RATE_LIMIT_EXCEEDED` | 429  | Rate-Limit überschritten           |
| `UPSTREAM_OVERLOADED` | 503  | Load-Shedding aktiv                |
| `GBIF_TIMEOUT`        | 504  | GBIF-Anfrage Timeout               |
| `GBIF_UNAVAILABLE`    | 502  | GBIF nicht erreichbar              |
| `INTERNAL_ERROR`      | 500  | Interner Serverfehler              |

## Lizenz

MIT

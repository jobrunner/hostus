# Hostus

Ein hochperformanter Backend-Service für Taxonomie- und Merkmals-Autosuggest
von Gefäßpflanzen.

## Überblick

Hostus 2.0 ist ein lokaler, schreibgeschützter Namens- und Merkmalsdienst,
kein zustandsloser GBIF-Proxy mehr (das war hostus 1.x). Er betreibt einen
eigenen, versionierten Multi-Backbone-Index (COL XR, WCVP/POWO, Euro+Med,
FloraVeg.EU, plus Merkmals-Vokabulare EIVE/Tichý/Midolo) in SQLite/FTS5,
gespeist aus fixierten Ingest-Artefakten, und:

- Gruppiert Synonyme unter akzeptierten Taxa
- Bietet Rate-Limiting und Load-Shedding für die Ingest-/Enrichment-Pfade
- Stellt eine stabile, frontend-optimierte REST-API bereit
- Liefert OpenTelemetry-Tracing/Metrics sowie einen Debug-MCP-Server für
  Claude Code

GBIF ist dabei höchstens eine von mehreren Ingest-/Enrichment-Quellen, nicht
der alleinige Laufzeit-Provider. Details zur Zielarchitektur stehen in
`docs/superpowers/specs/2026-07-31-hostus-2.0-architecture.md`.

> **Hinweis (SP0):** Dieser Stand ist das Harness/Hexagon-Skeleton. Die
> fachlichen `/v1/*`-Endpunkte unten sind absichtlich Stubs; sie landen
> schrittweise in SP1–SP6 (siehe CLAUDE.md).

## API

### Endpoints (Zielbild, siehe CLAUDE.md für den SP-Baustellenplan)

| Endpoint                          | Beschreibung                                          |
|------------------------------------|--------------------------------------------------------|
| `GET /v1/suggest`                  | Autosuggest, flächenbezogen gerankt (SP2)              |
| `POST /v1/match`                   | Batch-Namensauflösung, verbatim → Concept-Kandidaten (SP1/SP3) |
| `GET /v1/concept/{id}`             | Concept mit Xrefs + Klassifikation (SP1)               |
| `GET /v1/xref`                     | Reverse-Lookup, fremde ID → Concept (SP1/SP4)           |
| `GET /v1/concept/{id}/traits`      | Indikatorwerte je Vokabular (SP3)                       |
| `GET /v1/concept/{id}/synonyms`    | Synonymliste, relevanzfilterbar (SP6)                   |
| `POST /v1/translate`               | Concept-Übersetzung zwischen `sec.`-Referenzräumen (SP5)|
| `GET /openapi`                     | Generierte OpenAPI-Spezifikation                        |
| `GET /metrics`                     | Prometheus-Metriken                                     |
| `GET /health/live`, `/health/ready`| Liveness-/Readiness-Probe (heute schon vorhanden)       |
| `GET /`                            | Eingebettete Testkonsole, standardmäßig **an** (SP8)    |

### Testkonsole

Unter `/` liegt eine einkompilierte Testkonsole: eine einzelne, in sich
geschlossene HTML-Seite ohne Build-Schritt und ohne externe Referenzen, mit
je einem Panel für Suggest, Konzept, Match und Translate. Sie ist ein
**Messinstrument, kein Produkt-UI** — nicht authentifiziert und nicht für
Exposition gehärtet. Sie ist standardmäßig an und wird mit
`HOSTUS_UI_ENABLED=false` oder `hostus serve --ui=false` abgeschaltet; dann
antwortet `/` mit 404 und die API bleibt unverändert. Siehe
[docs/how-to/test-console.md](docs/how-to/test-console.md).

## Konfiguration

Alle Parameter können über `config.yaml`, `HOSTUS_`-Umgebungsvariablen oder
CLI-Flags gesetzt werden (Priorität niedrig → hoch: `config.yaml` <
`HOSTUS_*`-Env-Var < CLI-Flag). Es gibt keinen Dotenv-Loader im Binary;
`.env`/`example.env` sind eine Convenience für `docker-compose`
(`env_file:`), keine eigene Prioritätsstufe. Siehe
[`docs/reference/configuration.md`](docs/reference/configuration.md) für die
vollständige Referenz.

| Umgebungsvariable              | Standard         | Beschreibung                              |
|---------------------------------|------------------|--------------------------------------------|
| `HOSTUS_SERVER_HOST`            | 0.0.0.0          | Bind-Adresse des HTTP-Servers              |
| `HOSTUS_SERVER_PORT`            | 8080             | Server-Port                                |
| `HOSTUS_LOGGING_LEVEL`          | info             | Log-Level (debug/info/warn/error)          |
| `HOSTUS_LOGGING_FORMAT`         | json             | Log-Format (json/text)                     |
| `HOSTUS_METRICS_ENABLED`        | true             | Prometheus-Endpoint aktivieren             |
| `HOSTUS_CORS_ALLOWED_ORIGINS`   | (leer)           | Erlaubte CORS-Origins                      |
| `HOSTUS_TLS_ENABLED`            | false            | HTTPS/CertMagic aktivieren                 |
| `HOSTUS_TELEMETRY_ENABLED`      | false            | OpenTelemetry-Export aktivieren            |
| `HOSTUS_SQLITE_PATH`            | ./data/hostus.db | Pfad zur lokalen SQLite-Indexdatei         |

### CLI-Beispiel

```bash
./hostus --port=8080 --log-level=debug
```

Rate-Limiting ist über die Middleware-Kette aktiv (Default: 20 req/s), ein
CLI-Flag dafür ist noch nicht verdrahtet — landet mit dem SP1-Konfigurations-
Ausbau.

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

| Code                  | HTTP | Beschreibung                                     |
|-----------------------|------|---------------------------------------------------|
| `INVALID_QUERY`       | 400  | Ungültiger Query-Parameter                         |
| `RATE_LIMIT_EXCEEDED` | 429  | Rate-Limit überschritten                           |
| `UPSTREAM_OVERLOADED` | 503  | Load-Shedding aktiv                                |
| `NOT_FOUND`           | 404  | Unbekannte Concept-/Xref-ID                        |
| `UNRESOLVABLE`        | —    | Verbatim-Name lässt sich keinem Concept zuordnen   |
| `GBIF_TIMEOUT`        | 504  | GBIF-Anfrage Timeout (nur Ingest-/Enrichment-Pfad) |
| `GBIF_UNAVAILABLE`    | 502  | GBIF nicht erreichbar (nur Ingest-/Enrichment-Pfad)|
| `INTERNAL_ERROR`      | 500  | Interner Serverfehler                              |

## Lizenz

MIT

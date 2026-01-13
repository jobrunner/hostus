# Hostus - Entwicklerdokumentation

## Voraussetzungen

- Go 1.24+
- Make
- Docker (optional)

### Mit Nix

```bash
# Automatisch via direnv
direnv allow

# Oder manuell
nix develop
```

Die Nix-Entwicklungsumgebung enthält alle benötigten Tools:
- Go 1.24
- golangci-lint
- govulncheck
- staticcheck
- Docker

## Projektstruktur

```
hostus/
├── cmd/hostus/          # Entrypoint
├── internal/
│   ├── api/             # HTTP-Handler, OpenAPI
│   ├── cache/           # In-Memory Cache
│   ├── config/          # Viper-Konfiguration
│   ├── gbif/            # GBIF-Client
│   ├── middleware/      # HTTP-Middleware
│   └── taxonomy/        # Datenmodelle, Mapping
├── Dockerfile
├── docker-compose.yml
├── Makefile
└── .golangci.yml
```

## Build & Test

```bash
# Alle Checks + Build
make all

# Nur Build
make build

# Tests ausführen
make test

# Linting
make lint

# Security-Checks
make security

# Code formatieren
make fmt
```

### Einzelnen Test ausführen

```bash
go test -v -run TestMapAndGroup ./internal/taxonomy/
```

## Lokale Entwicklung

```bash
# Service starten
make run

# Mit eigenem Port
./hostus --port=3000 --log-level=debug
```

### Testen der API

```bash
# Autosuggest
curl "http://localhost:8080/api/v1/taxa/suggest?q=quercus"

# OpenAPI Spec
curl http://localhost:8080/openapi

# Metrics
curl http://localhost:8080/metrics

# Health
curl http://localhost:8080/health
```

## Docker

```bash
# Image bauen
make docker-build

# Lokal ausführen
make docker-run

# Multi-Arch Build (für CI)
docker buildx build --platform linux/amd64,linux/arm64 -t ghcr.io/jobrunner/hostus:latest .
```

## CI/CD

### Workflows

- **ci.yml**: Läuft bei jedem Push/PR
  - Tests
  - Linting
  - Security-Checks
  - Build-Verifikation

- **release.yml**: Läuft bei Merge in main
  - Docker-Image bauen (Multi-Arch)
  - Push zu ghcr.io

### Versionierung

- Version in `VERSION` pflegen (SemVer)
- `CHANGELOG.md` bei jedem PR aktualisieren
- CI blockiert bei fehlenden Updates

## Architekturentscheidungen

Siehe `architecture/adrs.md` für die Architecture Decision Records.

### Middleware-Reihenfolge

Die Middleware-Chain ist bewusst in dieser Reihenfolge:

1. **Request-ID** - Generiert ID für Tracing
2. **Logging** - Loggt Request/Response
3. **Rate-Limiting** - Schützt vor Überlastung
4. **Load-Shedding** - Circuit Breaker für Upstream
5. **Timeout** - Request-Timeout
6. **CORS** - Cross-Origin Handling
7. **Metrics** - Prometheus Instrumentation

### Synonym-Gruppierung

GBIF liefert eine flache Liste. Die Mapper-Logik:

1. Gruppiert alle Taxa nach `acceptedKey`
2. Taxa mit `status=ACCEPTED` werden Haupteinträge
3. Synonyme werden unter ihrem akzeptierten Taxon eingebettet

### Load-Shedding

Schützt vor Kaskadenfehlern:

1. Zählt aufeinanderfolgende GBIF-Fehler
2. Ab Threshold: Fail-Fast ohne Upstream-Call → 503
3. Nach Backoff: Probe-Request erlaubt
4. Bei Erfolg: Reset

## Abhängigkeiten

| Package                              | Zweck               |
|--------------------------------------|---------------------|
| `github.com/gorilla/mux`             | HTTP-Router         |
| `github.com/spf13/viper`             | Konfiguration       |
| `github.com/prometheus/client_golang`| Metriken            |

Keine weiteren externen Abhängigkeiten - bewusst minimal gehalten.

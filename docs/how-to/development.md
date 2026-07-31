# Entwicklungsumgebung einrichten

## Voraussetzungen

- Go 1.26+
- Make
- Docker (optional)

## Mit Nix

```bash
# Automatisch via direnv
direnv allow

# Oder manuell
nix develop
```

Die Nix-Entwicklungsumgebung enthält alle benötigten Tools:

- Go 1.26
- golangci-lint
- govulncheck
- staticcheck
- Docker

## Projektstruktur

```
hostus/
├── cmd/hostus/          # Entrypoint
├── internal/
│   ├── adapters/        # HTTP-, MCP- und Telemetry-Adapter
│   ├── app/             # Anwendungs-Wiring
│   ├── application/     # Anwendungslogik (Use Cases)
│   ├── cache/           # In-Memory Cache
│   ├── config/          # Viper-Konfiguration
│   ├── domain/          # Domänenmodelle
│   ├── httperr/         # Einheitliches Fehlerformat
│   ├── middleware/      # HTTP-Middleware
│   └── ports/           # Input-/Output-Ports (hexagonale Architektur)
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
make security-check

# Code formatieren
make fmt

# Maßgebliche Grün-Prüfung (wie CI)
make verify
```

### Einzelnen Test ausführen

```bash
go test -v -run TestFunctionName ./path/to/package
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
# Health
curl http://localhost:8080/health/live
curl http://localhost:8080/health/ready

# Metrics
curl http://localhost:8080/metrics
```

Die vollständige API-Referenz steht unter [HTTP-API](../reference/http-api.md).

## Docker

```bash
# Image bauen
make docker-build

# Lokal ausführen
docker-compose up

# Multi-Arch Build (für CI)
docker buildx build --platform linux/amd64,linux/arm64 -t ghcr.io/jobrunner/hostus:latest .
```

## CI/CD

### Workflows

- **ci.yml**: Läuft bei jedem Push/PR (Tests, Linting, Security-Checks, Build-Verifikation)
- **release-please.yml** / **docker-release.yml**: Release-Automatisierung und Multi-Arch-Image-Push nach ghcr.io

### Versionierung

- Version in `VERSION` pflegen (SemVer)
- `CHANGELOG.md` bei jedem PR aktualisieren
- CI blockiert bei fehlenden Updates

## Middleware-Reihenfolge

Die Middleware-Chain ist bewusst in dieser Reihenfolge (siehe auch
[Architektur](../explanation/architecture.md)):

1. **Request-ID** — Generiert ID für Tracing
2. **Logging** — Loggt Request/Response
3. **Rate-Limiting** — Schützt vor Überlastung
4. **Load-Shedding** — Circuit Breaker für Upstream
5. **Timeout** — Request-Timeout
6. **CORS** — Cross-Origin Handling
7. **Metrics** — Prometheus Instrumentation

## Abhängigkeiten

| Package                                | Zweck               |
|-----------------------------------------|---------------------|
| `github.com/gorilla/mux`                | HTTP-Router         |
| `github.com/spf13/viper`                | Konfiguration       |
| `github.com/prometheus/client_golang`   | Metriken            |

Keine weiteren externen Abhängigkeiten — bewusst minimal gehalten.

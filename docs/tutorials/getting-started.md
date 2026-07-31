# Erste Schritte

Diese Anleitung bringt hostus lokal zum Laufen.

## Voraussetzungen

- Go 1.26+
- Make
- Docker (optional)

## Mit Nix (empfohlen)

```bash
# Automatisch via direnv
direnv allow

# Oder manuell
nix develop
```

Die Nix-Entwicklungsumgebung enthält alle benötigten Tools: Go 1.26,
golangci-lint, govulncheck, staticcheck, Docker.

## Bauen und starten

```bash
make build
./hostus
```

## Health-Check prüfen

```bash
curl http://localhost:8080/health/live
curl http://localhost:8080/health/ready
```

## Metriken abrufen

```bash
curl http://localhost:8080/metrics
```

Weiter geht es mit den [How-to-Anleitungen](../how-to/index.md) für
Entwicklungs-Workflows oder der [Referenz](../reference/index.md) für die
vollständige API.

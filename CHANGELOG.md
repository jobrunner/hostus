# Changelog

Alle wesentlichen Änderungen an diesem Projekt werden in dieser Datei dokumentiert.

Das Format basiert auf [Keep a Changelog](https://keepachangelog.com/de/1.0.0/),
und dieses Projekt folgt [Semantic Versioning](https://semver.org/lang/de/).

## [Unreleased]

## [0.2.4] - 2026-06-16

### Added
- **CI**: Trivy-Filesystem-Scan im Security-Job (`go.sum`, Dockerfile, IaC). Schwellwert: HIGH/CRITICAL, ignoriert unfixable Findings, blockt PR bei Treffern
- **Release**: Trivy-Image-Scan post-build (HIGH/CRITICAL → Release kippt)
- **Release**: Automatische SBOM-Generierung im CycloneDX- und SPDX-Format, angehängt als Release-Asset
- **Release**: BuildKit-Attestationen (`sbom: true`, `provenance: mode=max`) als OCI-Manifest am Image — abrufbar via `docker buildx imagetools inspect`
- **CI/Release**: SARIF-Upload der Trivy-Findings nach GitHub Security ("Code scanning alerts")
- `.trivyignore`-Datei (leer) für künftige bewusst akzeptierte Findings

### Changed
- Permissions `security-events: write` in `ci.yml` und `release.yml` (SARIF-Upload)

## [0.2.3] - 2026-06-13

### Fixed
- Port-Mapping in docker-compose.yml korrigiert (intern und extern dynamisch)

### Changed
- `docker-compose.yml` verwendet jetzt Registry-Image (`ghcr.io/jobrunner/hostus:latest`)

### Added
- `docker-compose.local.yml` als Override für lokale Image-Builds
- Make-Targets: `start` (Registry-Image), `start-local` (lokaler Build), `stop`

## [0.2.2] - 2026-06-13

### Changed
- GitHub Actions auf Node-24-fähige Major-Versionen angehoben (GitHub erzwingt Node 24 ab 2026-06-16):
  - `actions/checkout`: v4 → v6
  - `actions/setup-go`: v5 → v6
  - `actions/upload-artifact`: v4 → v7
  - `golangci/golangci-lint-action`: v7 → v9
  - `docker/setup-qemu-action`: v3 → v4
  - `docker/setup-buildx-action`: v3 → v4
  - `docker/login-action`: v3 → v4
  - `docker/build-push-action`: v6 → v7
  - `softprops/action-gh-release`: v2 → v3

## [0.2.1] - 2026-06-13

### Fixed
- Release-Workflow (`release.yml`): Permission `contents: write` ergänzt, damit `softprops/action-gh-release` GitHub Releases anlegen darf (vorher 403 "Resource not accessible by integration")

## [0.2.0] - 2026-06-13

### Changed
- Go-Toolchain von 1.24 auf 1.26 angehoben (längeres Security-Support-Fenster bis ~Feb 2027)
- `flake.nix`: `pkgs.go_1_24` → `pkgs.go_1_26` (liefert aktuell 1.26.3; zieht automatisch auf 1.26.4 nach, sobald nixpkgs-unstable nachzieht)
- `flake.lock` auf nixpkgs nixos-unstable @ 2026-06-10 aktualisiert
- `go.mod`: `go 1.24.11` → `go 1.26.0` (Mindestanforderung)
- `Dockerfile`: Build-Image `golang:1.24-alpine` → `golang:1.26.4-alpine` (enthält Stdlib-Fixes für GO-2026-5037 und GO-2026-5039)
- CI (`.github/workflows/ci.yml`): `go-version` 1.24 → 1.26.4 in allen Jobs
- CI: `golangci-lint-action` Version v2.7.2 → v2.12.2 (synchron zur Nix-Toolchain)
- Doku in `CLAUDE.md` und `README.dev.md` auf Go 1.26 aktualisiert

### Fixed
- `Makefile`-Target `fmt` formatiert nur noch Projektquellen (`cmd`, `internal`), nicht den GOMODCACHE-Pfad `.go/mod`
- `.golangci.yml`: `goconst` und `noctx` für Test-Dateien ausgeschlossen, `goconst` zusätzlich für `internal/api/openapi.go` (OpenAPI-Datenliteral)

## [0.1.1] - 2025-01-14

### Fixed
- golangci-lint v2 Konfigurationsformat korrigiert
- CI-Pipeline auf golangci-lint-action v7 aktualisiert

### Changed
- Claude Code lokale Einstellungen in .gitignore aufgenommen

## [0.1.0] - 2025-01-13

### Added
- Initiale Projektstruktur
- Go-Modul mit Abhängigkeiten (gorilla/mux, viper, prometheus)
- Konfiguration via CLI, Environment und .env
- GBIF-Client für Taxonomie-Abfragen
- In-Memory Cache mit TTL
- REST-API Endpoint `/api/v1/taxa/suggest`
- OpenAPI-Spezifikation (Code-first mit swaggo)
- Middleware-Chain: Request-ID, Logging, Rate-Limiting, Load-Shedding, Timeouts, CORS, Metrics
- Prometheus Metrics unter `/metrics`
- Dockerfile (Multi-Arch, Distroless)
- docker-compose für lokale Entwicklung
- GitHub Actions für CI/CD

# Changelog

Alle wesentlichen Änderungen an diesem Projekt werden in dieser Datei dokumentiert.

Das Format basiert auf [Keep a Changelog](https://keepachangelog.com/de/1.0.0/),
und dieses Projekt folgt [Semantic Versioning](https://semver.org/lang/de/).

## [Unreleased]

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

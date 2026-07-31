# Changelog

Alle wesentlichen Änderungen an diesem Projekt werden in dieser Datei dokumentiert.

Das Format basiert auf [Keep a Changelog](https://keepachangelog.com/de/1.0.0/),
und dieses Projekt folgt [Semantic Versioning](https://semver.org/lang/de/).

## [Unreleased]

## [2.0.0-alpha.0] - 2026-07-31

hostus wird vom zustandslosen GBIF-Autosuggest-Proxy zum lokalen
Multi-Backbone-Namens- und Merkmalsservice umgebaut (siehe
`docs/superpowers/specs/2026-07-31-hostus-2.0-architecture.md`). Dieser
Alpha-Tag markiert den Abschluss von **SP0 (Harness & Skelett)**: die
Architektur-Inversion selbst (Multi-Backbone-Index, SQLite/FTS5, Ingest) ist
noch nicht implementiert und folgt in SP1+.

### Added
- Hexagonales Skelett (`internal/domain`, `application`, `ports/{input,output}`,
  `adapters/{http,mcp,telemetry}`, `app`, `config`) mit `depguard`/`gomodguard`-
  erzwungenen Schichtgrenzen
- OTel-Setup (Traces + Metrics, OTLP-Exporter) inkl. In-Memory-Span-/Log-
  Ringpuffer-Exporter, `otelmux`-instrumentierte Middleware-Kette
  (Request-ID → Logging → Rate-Limiting → Load-Shedding → Timeouts → CORS →
  Metrics)
- Stdio-Debug-MCP (`hostus mcp`, `modelcontextprotocol/go-sdk`): `get_recent_logs`,
  `tail_errors`, `get_trace`, `list_spans` — read-only Logs/Spans für Claude Code
- Cobra-CLI (`serve` als Default, `version`, sowie Stubs für `ingest`,
  `validate`, `bundle` — Implementierung folgt in SP1/SP2)
- viper-Konfiguration mit `HOSTUS_`-Präfix, Priorität `.env` < Env < CLI
- Fehler-Envelope um `NOT_FOUND` und `UNRESOLVABLE` erweitert (für die
  künftigen `/v1/concept`, `/v1/match`, `/v1/xref`-Endpunkte)
- `verify`-Gate (`fmt-check vet lint test arch debt-guard` + Compile-Check) als
  kanonischer Grün-Check, Tech-Debt-Ratchet-Trio (`.debt-budget`,
  `.coverage-floors`, `scripts/{debt-guard,coverage-gate}.sh`)
- golangci-lint v2 mit hexagonalen `depguard`-Grenzen, govulncheck, gosec,
  go-licenses-Allowlist
- Nix-Flake + direnv (`go_1_26`, projektlokaler GOPATH/GOCACHE), CGO-frei
- CI-Workflows portiert aus ortus (ohne Last-Test-Stack): `ci`, `docker-release`,
  `release-please`, `mutation`, `fuzz`, `codecharta`, `openapi-diff`,
  `commitlint`, `vuln-scan`, `actions-security` (zizmor), `dependabot-auto-merge`,
  `update-skills-submodule`
- `third_party/claude-skills`-Submodul (`jobrunner/claude-skills`, Branch `main`)
  + `.claude/hooks`/`.githooks` (pre-commit: fmt-check → build → debt-guard)
- MkDocs-Material/Diátaxis-Dokumentationsgerüst (Tutorials/How-to/Referenz/
  Erklärung) + OpenAPI-Baseline für den Doc-Drift-Check
- Sechs neue ADRs (`docs/explanation/decisions/0009`–`0014`): lokaler
  Multi-Backbone-Index, SQLite/FTS5-Persistenz, versionierter Artefaktvertrag
  (`dataset.yaml`), hexagonale Architektur, OpenTelemetry von Tag 1,
  stdio-Debug-MCP
- `.goreleaser.yml` (portiert aus ortus, `release.mode: append`, CGO-frei,
  linux/darwin × amd64/arm64)

### Changed
- `Dockerfile`: Build-Stage injiziert `main.Version`/`main.Commit`/`main.BuildDate` per Ldflags (statt `main.version` aus `VERSION`-Datei) — identische Variablenpfade wie im Makefile, damit `hostus version` im Image echte Build-Infos zeigt
- `Dockerfile`: `USER` auf numerische UID:GID (`65532:65532`, distroless "nonroot") statt Namen umgestellt (hadolint DL3066)
- `flake.nix`: `hadolint` zum devShell hinzugefügt (Dockerfile-Linting reproduzierbar verfügbar, auch für CI in S12)
- `docker-compose.yml`: Kommentar zum Healthcheck ergänzt (Exec-Form-Prozess-Check, kein HTTP-Probe — Grund siehe unten)
- `CLAUDE.md`: Reconciled auf hostus 2.0 — Projektbeschreibung, erlaubte
  Bibliotheken, API-Endpunkte, Fehlercodes und Architektur-Abschnitt spiegeln
  jetzt den Multi-Backbone-Service statt des v1-GBIF-Proxys; Git-/CI/CD-
  Workflow-Regeln unverändert
- `architecture/adrs.md`: ADR-001, ADR-003, ADR-008 als **Superseded**
  markiert (Verweis auf ADR-0009/-0010); ADR-004 (Go + Minimal-Dependencies)
  im Wortlaut auf den hostus-2.0-Stack reconciled

### Removed
- `internal/gbif`, GBIF-gestützter `suggest`-Handler und der GBIF-spezifische
  Taxonomie-Mapper (Architektur-Inversion laut Spec Abschnitt 6)
- `Dockerfile`: `HEALTHCHECK`/`COPY VERSION` entfernt. `gcr.io/distroless/static` hat weder Shell noch HTTP-Client (kein curl/wget) — ein In-Container-Probe gegen `GET /health/live` ist technisch nicht möglich. Liveness wird stattdessen extern vom Orchestrator gegen `GET /health/live` geprüft. Ein `hostus health`-Self-Probe-Subcommand wäre eine Option, aber ungetestete Go-Logik außerhalb dieses Tasks — bewusst nicht umgesetzt

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

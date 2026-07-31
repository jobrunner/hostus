# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

**hostus 2.0** - A local, read-only naming and trait service for vascular
plants, built on a **multi-backbone index** (COL XR, WCVP/POWO, Euro+Med,
FloraVeg.EU, plus trait vocabularies EIVE/Tichý/Midolo) fed by versioned,
pinned artifacts. hostus is *not* a stateless GBIF autosuggest proxy anymore
— that was hostus 1.x. It now runs its own local SQLite/FTS5 index, ingests
backbone data through a dedicated pipeline, and serves seven API endpoints
plus an offline bundle export. GBIF, where still used, is one ingest/
enrichment data source among several, not the runtime's sole provider.

The full target architecture is specified in
`docs/superpowers/specs/2026-07-31-hostus-2.0-architecture.md`. The rewrite
happens in-place across incremental sub-projects (SP0–SP6); this file
reflects the current state plus the invariants that hold across all of them.

## Development Environment

Uses Nix flakes for reproducible development. Enter the dev shell with:
```bash
direnv allow   # or: nix develop
```

Go version: **1.26** (via `pkgs.go_1_26`)

## Build Commands

```bash
make build          # Build the binary
make test           # Run all tests (via gotestsum)
make lint           # Run golangci-lint
make security-check # Run govulncheck + gosec
make verify         # Canonical green-check: fmt-check, vet, lint, test, arch, debt-guard, compile
```

Run a single test:
```bash
go test -v -run TestFunctionName ./path/to/package
```

## Architecture

Hexagonal (Ports & Adapters), not a stateless proxy:

```
internal/
  domain/        # Name, Concept, Trait, Xref, Distribution, Relation — no I/O deps
  application/   # Use cases: Suggest, Match, ResolveConcept, ReverseXref, Traits, Synonyms, Translate, Ingest, Bundle
  ports/
    input/       # interfaces the application offers
    output/      # interfaces the application needs (repository, trait store, ...)
  adapters/
    sqlite/      # SQLite/FTS5 repository (modernc.org/sqlite)
    coldp/       # ColDP importer
    http/        # gorilla/mux router + handlers + middleware chain
    mcp/         # stdio debug-MCP (logs + spans, for Claude Code)
    telemetry/   # OTel setup (traces + metrics)
    bundle/      # offline bundle export (filtered SQLite copy)
  app/           # composition root (wiring)
  config/        # viper
```

Hexagon boundaries are enforced by `depguard`/`gomodguard` in the linter
(`make arch`, part of `make verify`), not just convention.

### Key Responsibilities
- Serve a local, versioned multi-backbone index (SQLite/FTS5) fed by pinned
  backbone/trait artifacts — not a live GBIF passthrough
- Group synonyms under accepted taxa (concept/name relations, typed
  homotypic/heterotypic)
- Rate limiting and load shedding for upstream protection (ingest/enrichment
  paths that still call external APIs)
- OpenTelemetry tracing/metrics from day one, plus a read-only stdio
  debug-MCP for Claude Code (logs + spans)

### HTTP Middleware Chain (order matters)
1. Request-ID
2. Logging
3. Rate-Limiting
4. Load-Shedding
5. Timeouts
6. CORS
7. Metrics

All of the above are OTel-instrumented (`otelmux`).

### API Endpoints
- `GET /v1/suggest?q={query}&limit={n}` - autosuggest, area-ranked (SP2)
- `POST /v1/match` - batch name resolution, verbatim → concept candidates (SP1 exact, SP3 fuzzy)
- `GET /v1/concept/{id}` - concept with xrefs + classification (SP1)
- `GET /v1/xref` - reverse lookup, foreign ID → concept (SP1 base, SP4 enrichment)
- `GET /v1/concept/{id}/traits` - indicator values per vocabulary (SP3)
- `GET /v1/concept/{id}/synonyms` - synonym list, relevance-filterable (SP6)
- `POST /v1/translate` - concept translation between `sec.` reference spaces (SP5)
- `GET /openapi` - generated OpenAPI spec
- `GET /metrics` - Prometheus metrics
- `GET /health/live`, `GET /health/ready` - liveness/readiness

Most of the `/v1/*` endpoints above are not yet implemented as of SP0
(harness/skeleton only) — see the SP0–SP6 build order in the master spec for
when each lands.

## Technical Constraints

### Allowed Libraries Only
- Go standard library
- `github.com/gorilla/mux`
- `github.com/spf13/viper`
- `github.com/spf13/cobra` (CLI: `serve`, `ingest`, `validate`, `bundle`, `mcp`, `version`)
- `modernc.org/sqlite` — pure-Go, CGO-free SQLite/FTS5 driver for the local index (see ADR-0010)
- OpenTelemetry Go SDK (`go.opentelemetry.io/otel`, `.../sdk`, `.../exporters/otlp/...`) + `otelmux` — tracing/metrics from day one (see ADR-0013)
- `github.com/modelcontextprotocol/go-sdk` — stdio debug-MCP (see ADR-0014)
- `github.com/caddyserver/certmagic` (optional TLS)
- Official Prometheus Go client

Dev/test tooling (not a runtime dependency, but part of the sanctioned
toolchain): `gotestsum`, `gremlins` (mutation testing), `golangci-lint` v2,
`goreleaser`.

**No** heavy frameworks, ORMs, or reflection-heavy dependencies. This list is
the hostus 2.0 baseline — the v1.x constraint of "GBIF as sole external
provider" (see `architecture/adrs.md` ADR-001, superseded) no longer applies;
GBIF is now, at most, one ingest/enrichment data source among several.

### GBIF Query Filters (ingest/enrichment path only)

These filters applied to the v1.x GBIF-proxy runtime path. In hostus 2.0 they
are only relevant where GBIF is still used as an ingest or enrichment data
source (e.g. `/v2/species/match` against a pinned `checklistKey`), never as a
live runtime dependency for serving requests:
- `kingdom=Plantae`
- `phylum=Tracheophyta`
- **No** `status=ACCEPTED` filter (synonyms are intentionally included)
- Ranks: FAMILY, GENUS, SPECIES, SUBSPECIES

### Configuration Priority (low → high)
1. `.env` file
2. Environment variables
3. CLI parameters (`--port=443`, `--rate-limit=20`, etc.)

## Code Style

- **Documentation**: `README.md` and `README.dev.md` in German
- **Code comments**: Sparse, English only when necessary
- **OpenAPI**: Must be code-generated (no manual spec maintenance)

## Required Files

- `VERSION` - SemVer version (`vX.Y.Z`)
- `CHANGELOG.md` - Must be updated with every PR
- `Dockerfile` - Multi-arch (amd64, arm64), distroless base
- `docker-compose.yml` - For local testing
- `example.env` - With inline English descriptions
- `.golangci.yml` - Linter configuration

## Error Response Format

```json
{
  "error": {
    "code": "ERROR_CODE",
    "message": "Human readable message"
  }
}
```

Required error codes: `INVALID_QUERY`, `RATE_LIMIT_EXCEEDED`, `UPSTREAM_OVERLOADED`, `GBIF_TIMEOUT`, `GBIF_UNAVAILABLE`, `INTERNAL_ERROR`, `NOT_FOUND`, `UNRESOLVABLE`

`NOT_FOUND` and `UNRESOLVABLE` are new in hostus 2.0 (unknown concept/xref ID;
a verbatim name that cannot be resolved to any concept). The `GBIF_*` codes
are retained only for the ingest/enrichment path where GBIF is still called
(see "GBIF Query Filters" above) — they are not expected on the normal
`/v1/*` serving path once the local index is authoritative.

## Git Workflow

1. **Always create a feature branch** for new tasks (`feature/...`)
2. After PR is merged and CI is green: `git checkout master && git pull`
3. Then create new feature branch for next task
4. Never commit directly to master

## CI/CD Rules

- **VERSION and CHANGELOG.md must be updated in every PR**
- Releases only on feature branch merges (not on push to main)
- Docker images pushed to ghcr.io with `latest` and SemVer tags

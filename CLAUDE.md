# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

**hostus** - A high-performance Go backend service acting as a read-only taxonomy gateway for a frontend autosuggest field (vascular plants). The service proxies requests to the GBIF REST API, caches responses in-memory, and groups synonyms under accepted taxa.

**This is a greenfield project** - implementation follows the specification below.

## Development Environment

Uses Nix flakes for reproducible development. Enter the dev shell with:
```bash
direnv allow   # or: nix develop
```

Go version: **1.24** (via `pkgs.go_1_24`)

## Build Commands

```bash
make build      # Build the binary
make test       # Run all tests
make lint       # Run golangci-lint
make security   # Run govulncheck + staticcheck
```

Run a single test:
```bash
go test -v -run TestFunctionName ./path/to/package
```

## Architecture

```
Frontend → hostus (this service) → GBIF REST API
```

### Key Responsibilities
- Proxy access to GBIF `/v1/species/search`
- In-memory caching with TTL
- Group synonyms under accepted taxa
- Rate limiting and load shedding for upstream protection

### HTTP Middleware Chain (order matters)
1. Request-ID
2. Logging
3. Rate-Limiting
4. Load-Shedding
5. Timeouts
6. CORS
7. Metrics

### API Endpoints
- `GET /api/v1/taxa/suggest?q={query}&limit={n}` - Main autosuggest endpoint
- `GET /openapi` - Generated OpenAPI spec
- `GET /metrics` - Prometheus metrics

## Technical Constraints

### Allowed Libraries Only
- Go standard library
- `github.com/gorilla/mux`
- `github.com/spf13/viper`
- `github.com/caddyserver/certmagic` (optional TLS)
- Official Prometheus Go client

**No** heavy frameworks, ORMs, or reflection-heavy dependencies.

### GBIF Query Filters
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

Required error codes: `INVALID_QUERY`, `RATE_LIMIT_EXCEEDED`, `UPSTREAM_OVERLOADED`, `GBIF_TIMEOUT`, `GBIF_UNAVAILABLE`, `INTERNAL_ERROR`

## CI/CD Rules

- **VERSION and CHANGELOG.md must be updated in every PR**
- Releases only on feature branch merges (not on push to main)
- Docker images pushed to ghcr.io with `latest` and SemVer tags

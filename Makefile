# hostus Makefile
# Alle Standardaufgaben für Entwicklung und CI/CD

.PHONY: all build build-all install run clean help
.PHONY: test test-unit test-integration test-coverage test-race bench mutation fuzz
.PHONY: lint lint-go lint-fix vet
.PHONY: security-check vuln-check gosec licenses
.PHONY: fmt format fmt-check
.PHONY: check verify hooks arch debt debt-guard debt-coverage
.PHONY: doc doc-drift
.PHONY: deps deps-update deps-verify

# Variablen
BINARY_NAME := hostus
MODULE := github.com/jobrunner/hostus
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
LDFLAGS := -ldflags "-X main.Version=$(VERSION) -X main.Commit=$(COMMIT) -X main.BuildDate=$(BUILD_TIME)"

GO := go
GOTEST := gotestsum --format testdox --
GOLINT := golangci-lint

# Verzeichnisse
BUILD_DIR := build
COVERAGE_DIR := coverage

# Standard-Target
all: check build

## Build Targets
build: ## Baue die Anwendung
	@echo "Building $(BINARY_NAME)..."
	$(GO) build $(LDFLAGS) -o $(BINARY_NAME) ./cmd/$(BINARY_NAME)

build-all: ## Baue für alle Plattformen
	@mkdir -p $(BUILD_DIR)
	GOOS=linux GOARCH=amd64 $(GO) build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 ./cmd/$(BINARY_NAME)
	GOOS=linux GOARCH=arm64 $(GO) build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-arm64 ./cmd/$(BINARY_NAME)
	GOOS=darwin GOARCH=amd64 $(GO) build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-amd64 ./cmd/$(BINARY_NAME)
	GOOS=darwin GOARCH=arm64 $(GO) build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64 ./cmd/$(BINARY_NAME)
	GOOS=windows GOARCH=amd64 $(GO) build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-windows-amd64.exe ./cmd/$(BINARY_NAME)

install: build ## Installiere lokal
	$(GO) install $(LDFLAGS) ./cmd/$(BINARY_NAME)

run: build ## Baue und starte die Anwendung
	./$(BINARY_NAME)

## Test Targets
test: ## Führe alle Tests aus
	$(GOTEST) ./...

test-unit: ## Nur Unit-Tests
	$(GOTEST) -short ./...

test-integration: ## Nur Integrationstests
	$(GOTEST) -run Integration ./...

test-coverage: ## Tests mit Coverage-Report
	@mkdir -p $(COVERAGE_DIR)
	$(GO) test -coverprofile=$(COVERAGE_DIR)/coverage.out -covermode=atomic ./...
	$(GO) tool cover -html=$(COVERAGE_DIR)/coverage.out -o $(COVERAGE_DIR)/coverage.html
	$(GO) tool cover -func=$(COVERAGE_DIR)/coverage.out
	@echo "\nCoverage Report: $(COVERAGE_DIR)/coverage.html"

test-race: ## Tests mit Race Detector
	$(GO) test -race ./...

bench: ## Benchmarks ausführen
	$(GO) test -run='^$$' -bench=. -benchmem ./...

mutation: ## Mutation-Testing (gremlins) — package-scoped, green-required (PKG=./internal/... überschreibbar)
	@command -v gremlins >/dev/null 2>&1 || $(GO) install github.com/go-gremlins/gremlins/cmd/gremlins@v0.5.1
	gremlins unleash --dry-run=false $(if $(MUTATION_WORKERS),--workers $(MUTATION_WORKERS),) $(if $(PKG),$(PKG),./...)

# Fuzzt alle Fuzz*-Targets im Modul (FUZZTIME je Target überschreibbar, default
# 30s). Targets werden zur Laufzeit per `go test -list` entdeckt — keine
# statische Paketliste zu pflegen. No-op (exit 0), solange es noch keine
# Fuzz*-Funktionen gibt (SP0-Skeleton; echte Targets landen in SP1).
fuzz: ## Fuzz alle Fuzz*-Targets (FUZZTIME überschreibbar, default 30s) — no-op ohne Targets
	@set -e; \
	ft="$${FUZZTIME:-30s}"; \
	case "$$ft" in ''|*[!0-9smhun]*) echo "FUZZTIME ungueltig (z.B. 30s, 3m)"; exit 1;; esac; \
	found=0; \
	for pkg in $$($(GO) list ./...); do \
		targets=$$($(GO) test -list '^Fuzz' "$$pkg" 2>/dev/null | grep '^Fuzz' || true); \
		for fn in $$targets; do \
			found=1; \
			echo "==> fuzz $$fn ($$pkg) for $$ft"; \
			$(GO) test "$$pkg" -run='^$$' -fuzz="^$${fn}$$" -fuzztime="$$ft"; \
		done; \
	done; \
	if [ "$$found" -eq 0 ]; then \
		echo "no Fuzz* targets yet — nothing to do (they land in SP1)"; \
	fi

## Lint & Analyse Targets
lint: lint-go ## Führe alle Linter aus (Alias für lint-go)

lint-go: ## Go Linting mit golangci-lint
	$(GOLINT) run --timeout=5m ./...

lint-fix: ## Linting mit Auto-Fix
	$(GOLINT) run --fix ./...

vet: ## Go vet
	$(GO) vet ./...

## Security Targets
security-check: vuln-check gosec ## Alle Security Checks

vuln-check: ## Prüfe auf bekannte Vulnerabilities
	govulncheck ./...

gosec: ## Security Scanner (via golangci-lint)
	$(GOLINT) run --enable-only gosec ./...

# Allowed dependency licenses (permissive only). First-party packages are
# ignored (the repo itself isn't classified by go-licenses).
ALLOWED_LICENSES := Apache-2.0,MIT,BSD-3-Clause,BSD-2-Clause,ISC,CC0-1.0,MPL-2.0
licenses: ## Lizenz-Compliance der Abhängigkeiten (go install github.com/google/go-licenses@latest)
	go-licenses check ./cmd/$(BINARY_NAME) --allowed_licenses=$(ALLOWED_LICENSES) --ignore $(MODULE)

## Format Targets
fmt: ## Formatiere Go Code
	$(GO) fmt ./...
	goimports -w -local $(MODULE) ./cmd ./internal

format: fmt ## Alias für fmt

fmt-check: ## Prüfe Formatierung ohne zu ändern (CI/Hook)
	@unformatted=$$(gofmt -l cmd internal); \
	if [ -n "$$unformatted" ]; then \
		echo "❌ Nicht formatiert (gofmt -w ausführen):"; echo "$$unformatted"; exit 1; \
	fi

## Quality Gate
check: fmt vet lint test ## Alle Qualitätsprüfungen (vor Commit)
	@echo "\n✅ Alle Prüfungen bestanden!"

# Kanonische, NICHT-mutierende Grün-Prüfung. Dies ist die maßgebliche Quelle
# für "ist es grün?" — Editor-/LSP-Diagnosen sind bei großen Renames unzuverlässig;
# der Compiler entscheidet. Gleiche Schritte wie die CI.
# Bewusst KEIN Aufruf des `build`-Targets (das schreibt ./hostus); stattdessen
# ein binärloser Compile-Check via `go build ./...`.
verify: fmt-check vet lint test arch debt-guard ## Maßgebliche Grün-Prüfung (gofmt-check+vet+compile+test+lint+arch+debt)
	@echo "Compile-Check (go build ./...)…"
	@$(GO) build ./...
	@echo "\n✅ verify bestanden — Compile/Test/Lint/Format/Arch/Debt grün."

# Schulden-Harness: hält technische Schuld niedrig per Ratchet (siehe docs).
# `debt-guard` ist schnell (grep-basiert) und in `verify` eingebunden; `debt-coverage`
# fährt einen eigenen Coverage-Lauf und prüft die Per-Paket-Floors; `debt` bündelt beide.
debt: debt-guard debt-coverage ## Schulden-Ratchet: Suppression-Budget + Marker + Coverage-Floors

debt-guard: ## Schnelle Schulden-Checks (Suppression-Budget, Debt-Marker)
	@./scripts/debt-guard.sh

debt-coverage: ## Coverage-Floors prüfen (eigener Testlauf)
	@mkdir -p $(COVERAGE_DIR)
	@$(GO) test -coverprofile=$(COVERAGE_DIR)/coverage.out -covermode=atomic ./... >/dev/null
	@./scripts/coverage-gate.sh $(COVERAGE_DIR)/coverage.out

# Architektur-Fitness: hexagonale Import-Grenzen (depguard), Modul-Blocklist
# (gomodguard) und go.mod-Hygiene. Eigenständig aufrufbar für einen fokussierten,
# schnellen Drift-Check; in `verify` eingebunden.
# (depguard/gomodguard laufen auch im vollen `lint`; hier explizit für klare Signale.)
arch: ## Architektur-Fitness: Import-Grenzen + Modul-Hygiene
	$(GOLINT) run --enable-only depguard,gomodguard_v2 ./...
	$(GO) mod tidy -diff
	@echo "✅ arch ok — Import-Grenzen & Modul-Hygiene grün."

hooks: ## Installiere git pre-commit Hook (.githooks)
	git config core.hooksPath .githooks
	@chmod +x .githooks/pre-commit
	@echo "✅ pre-commit Hook aktiv (core.hooksPath=.githooks)."

## Dokumentation
doc: ## Baue die MkDocs-Dokumentation strikt (bricht bei kaputten Links/Nav ab)
	uvx --with mkdocs-material mkdocs build --strict

doc-drift: ## Doku-Drift-Harness: prüft OpenAPI-Baseline ↔ Docs (0 = keine Drift)
	@bash scripts/doc-drift-check.sh

## Clean
clean: ## Räume Build-Artefakte auf
	$(GO) clean
	rm -f $(BINARY_NAME)
	rm -rf $(BUILD_DIR)
	rm -rf $(COVERAGE_DIR)
	rm -f coverage.out coverage.html

## Dependencies
deps: ## Lade Dependencies
	$(GO) mod download

deps-update: ## Aktualisiere Dependencies
	$(GO) get -u ./...
	$(GO) mod tidy

deps-verify: ## Verifiziere Dependencies
	$(GO) mod verify

## Hilfe
help: ## Zeige diese Hilfe
	@echo "hostus - Verfügbare Make-Targets:\n"
	@grep -hE '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2}'
	@echo ""

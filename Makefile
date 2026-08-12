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

test-integration: ## Nur Integrationstests (build-tag `integration`)
	$(GOTEST) -tags=integration -run Integration ./...

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

# Pakete, für die "No results to report." (bei EINZELN gesetztem PKG) eine
# gültige Antwort ist, weil sie keine mutierbare Stelle enthalten — nicht weil
# gremlins v0.5.1 an einem Mehr-Paket-Muster scheitert. Jeder Eintrag braucht
# eine Begründung; ein Paket landet hier nur, wenn tatsächlich geprüft wurde,
# dass es keine Mutanten erzeugt (nicht auf Verdacht).
#   ./internal/httperr — nur Konstanten + ein linearer JSON-Encode ohne
#   Verzweigung/Vergleich/Arithmetik; verifiziert 2026-08-03 (siehe Review I3).
MUTATION_NO_MUTABLE_CODE := ./internal/httperr

# MUTATION_WORKERS (optional): begrenzt gremlins' Worker-Zahl. Ohne die
# Variable bleibt gremlins' Default (CPU-Anzahl) aktiv, damit lokale Läufe
# parallel bleiben; CI setzt MUTATION_WORKERS=1, weil jeder Worker eine eigene
# Neuübersetzung des Pakets im Speicher hält (siehe
# .github/workflows/mutation.yml).
#
# Mutation-Gate: `Not covered` MUSS 0 sein, UND es muss mindestens ein Mutant
# tatsächlich geprüft worden sein (positiver Boden).
#
# Ein überlebender (LIVED) Mutant heißt "ein Test läuft durch die Zeile, prüft
# aber das Ergebnis nicht scharf genug" — der ist begründbar und wird pro Fall
# im Report gerechtfertigt. Ein NOT COVERED Mutant heißt dagegen "kein Test
# führt diesen Code überhaupt aus"; das ist strikt schlechter und nie
# begründbar. Deshalb hängt der harte Fehlschlag genau an `Not covered` und
# nicht an einer Efficacy-Schwelle: eine Schwelle wäre gegen die bestehenden,
# dokumentiert-gerechtfertigten Überlebenden spröde und würde bei jedem
# Refactoring falsch anschlagen.
#
# Fällt die Zeile "Not covered: N" ganz aus dem Report, bricht das Target
# ebenfalls ab — ein Gate, das sein Signal nicht mehr findet, darf nicht
# stillschweigend grün melden. Genau eine Ausnahme: gremlins meldet
# "No results to report.", wenn es keine mutierbare Stelle gibt — UND NUR,
# wenn PKG auf der Allowlist MUTATION_NO_MUTABLE_CODE steht. Vorher (Review
# I3) reichte "PKG ist irgendwie gesetzt", und CI setzt PKG für JEDEN Lauf —
# die Ausnahme war damit für jedes Paket offen, das aus irgendeinem Grund
# (falscher Pfad, kaputte Build-Tags, umbenanntes Verzeichnis, eine künftige
# v0.5.x-Regression) plötzlich keine Mutanten mehr erzeugt. Ein Paket, das
# NICHT auf der Allowlist steht und trotzdem "No results to report." meldet,
# ist deshalb ein harter Fehlschlag, kein Grünmelden.
mutation: ## Mutation-Testing (gremlins) — package-scoped, `Not covered`=0 + Mutantenboden>0 erzwungen (PKG=./internal/... überschreibbar)
	@command -v gremlins >/dev/null 2>&1 || $(GO) install github.com/go-gremlins/gremlins/cmd/gremlins@v0.5.1
	@out=$$(mktemp); rc=$$(mktemp); \
	{ gremlins unleash --dry-run=false $(if $(MUTATION_WORKERS),--workers $(MUTATION_WORKERS),) $(if $(PKG),$(PKG),./...); echo $$? >"$$rc"; } | tee "$$out"; \
	status=$$(cat "$$rc"); \
	notcovered=$$(sed -n 's/.*Not covered: \([0-9][0-9]*\).*/\1/p' "$$out" | tail -1); \
	killed=$$(sed -n 's/.*Killed: \([0-9][0-9]*\).*/\1/p' "$$out" | tail -1); \
	lived=$$(sed -n 's/.*Lived: \([0-9][0-9]*\).*/\1/p' "$$out" | tail -1); \
	empty=$$(grep -c 'No results to report' "$$out" 2>/dev/null | tr -d ' '); \
	if [ "$$empty" = "0" ]; then empty=""; fi; \
	rm -f "$$out" "$$rc"; \
	if [ "$$status" -ne 0 ]; then exit "$$status"; fi; \
	allowed=""; \
	for p in $(MUTATION_NO_MUTABLE_CODE); do \
		if [ "$(PKG)" = "$$p" ]; then allowed=1; fi; \
	done; \
	if [ -z "$$notcovered" ]; then \
		if [ -n "$$empty" ] && [ -n "$$allowed" ]; then \
			echo "✅ mutation: $(PKG) hat keine mutierbare Stelle (Allowlist MUTATION_NO_MUTABLE_CODE) — nichts zu prüfen."; \
			exit 0; \
		fi; \
		if [ -n "$$empty" ]; then \
			echo "❌ mutation: $(PKG) meldet 'No results to report.', steht aber NICHT auf der Allowlist MUTATION_NO_MUTABLE_CODE."; \
			echo "   Entweder hat das Paket wirklich keine mutierbare Stelle — dann mit Begründung zur Allowlist"; \
			echo "   hinzufügen — oder gremlins konnte keine Mutanten erzeugen (falscher Pfad, kaputte Build-Tags,"; \
			echo "   umbenanntes Verzeichnis, v0.5.x-Regression). Ein stillschweigendes Grün ist hier keine Option."; \
			exit 1; \
		fi; \
		echo "❌ mutation: kein gremlins-Report ('Not covered: N' fehlt) — das Gate kann nichts prüfen."; \
		echo "   gremlins v0.5.1 liefert für ein Mehr-Paket-Muster wie ./... 'No results to report.';"; \
		echo "   dieses Target ist deshalb paketweise gedacht: make mutation PKG=./internal/domain"; \
		echo "   (so ruft es auch .github/workflows/mutation.yml auf)."; \
		exit 1; \
	fi; \
	if [ "$$notcovered" -ne 0 ]; then \
		echo "❌ mutation: $$notcovered nicht abgedeckte Mutanten (NOT COVERED) — kein Test führt diesen Code aus."; \
		echo "   Siehe 'NOT COVERED' oben. Hinweis: Bedingungen in case-Armen eines tag-losen switch"; \
		echo "   liegen in Gos Coverage-Modell in KEINEM gezählten Block; dort hilft nur Hochziehen"; \
		echo "   der Bedingung in eine eigene Zuweisung (siehe internal/domain/synonym.go)."; \
		exit 1; \
	fi; \
	total=$$(( $${killed:-0} + $${lived:-0} + $${notcovered:-0} )); \
	if [ "$$total" -eq 0 ]; then \
		echo "❌ mutation: 0 Mutanten insgesamt (Killed+Lived+Not covered) trotz vorhandenem Report — kein positiver Boden."; \
		echo "   Ein Gate, das keinen einzigen Mutanten geprüft hat, prüft nichts. Falls $(PKG) tatsächlich keinen"; \
		echo "   mutierbaren Code hat: mit Begründung zur Allowlist MUTATION_NO_MUTABLE_CODE hinzufügen."; \
		exit 1; \
	fi; \
	echo "✅ mutation: Not covered = 0, $$total Mutant(en) geprüft"

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
	$(GOLINT) run --timeout=5m --build-tags integration ./...

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
#
# modernc.org/mathutil is ignored because go-licenses cannot CLASSIFY its
# licence, not because the licence is a problem. Verified by hand on 2026-08-03
# against modernc.org/mathutil@v1.7.1: the module ships a LICENSE file whose
# text is verbatim BSD-3-Clause ("Redistribution and use in source and binary
# forms ... * Redistributions of source code must retain the above copyright
# notice"), and its Makefile header says "governed by a BSD-style license".
# BSD-3-Clause is already on ALLOWED_LICENSES above, so this is a classifier
# false positive, not an exception to the policy. It reaches us transitively
# via modernc.org/sqlite (ADR-0010). Re-verify if the module is bumped.
LICENSE_IGNORE := $(MODULE),modernc.org/mathutil
licenses: ## Lizenz-Compliance der Abhängigkeiten
	go-licenses check ./cmd/$(BINARY_NAME) --allowed_licenses=$(ALLOWED_LICENSES) --ignore $(LICENSE_IGNORE)

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
verify: fmt-check vet lint test arch debt-guard poc-check ## Maßgebliche Grün-Prüfung (gofmt-check+vet+compile+test+lint+arch+debt+poc)
	@echo "Compile-Check (go build ./...)…"
	@$(GO) build ./...
	@echo "\n✅ verify bestanden — Compile/Test/Lint/Format/Arch/Debt/poc grün."

# poc/ ist ein eigenes Go-Modul (github.com/jobrunner/hostus-poc) mit den
# Messharnessen, die die Zahlen in docs/research/ erzeugen. verify hat es bisher
# NICHT abgedeckt (eigenes Modul, kein go.work; debt-guard nimmt ./poc aus), also
# konnte ein Build-/vet-Bruch dort unbemerkt bleiben (SP7 fand genau so toten
# Tiebreak-Code). poc-check kompiliert und vettet es — bewusst OHNE die
# Hexagon-Lint-Regeln, die nur fuer den Laufzeit-Code gelten.
poc-check: ## poc/ (eigenes Modul) kompiliert + vet-sauber (keine Hexagon-Lints)
	@echo "poc-check (cd poc && go build ./... && go vet ./...)…"
	@cd poc && $(GO) build ./... && $(GO) vet ./...

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

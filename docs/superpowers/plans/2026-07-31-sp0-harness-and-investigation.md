# hostus 2.0 — SP0 Harness & Investigation (Phase R + Phase 0) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Establish the full ortus-parity engineering harness and a green, empty hexagonal service skeleton for hostus 2.0 (Part 2), and complete the source-research and assumption-PoC gates that unblock SP1 (Part 1).

**Architecture:** Two independent tracks that can run in parallel. Part 1 (Investigation) produces documents and checked-in fixtures — no production code, no `verify` participation. Part 2 (SP0) turns the current GBIF proxy into a hexagonal skeleton (`domain`/`application`/`ports`/`adapters`/`app`) with OTel from day one, a stdio debug-MCP exposing logs+spans, the tech-debt ratchet trio, distroless multi-arch Docker, and ~12 CI workflows — all ported from the sibling project `/Users/jbrunner/work/projects/ortus`.

**Tech Stack:** Go 1.26, `modernc.org/sqlite` (pure-Go, no CGO), `gorilla/mux`, `spf13/cobra` + `spf13/viper`, OpenTelemetry Go SDK + `prometheus/client_golang`, `modelcontextprotocol/go-sdk`, Nix flakes + direnv, golangci-lint v2, gotestsum, goreleaser + release-please + cosign, MkDocs Material.

## Global Constraints

- Go version floor: **1.26** (`go_1_26` in flake, `go 1.26.0` in go.mod). Copied verbatim from spec §5.1.
- Module path stays **`github.com/jobrunner/hostus`**. Version target **`v2.0.0`** (this plan lands `2.0.0-alpha.0`).
- SQLite driver is **`modernc.org/sqlite`** (pure-Go, CGO-free) — never `mattn/go-sqlite3`. Distroless must stay buildable, so **`CGO_ENABLED=0`**.
- Config: viper, env prefix **`HOSTUS_`**, key replacer `.`→`_`; precedence `.env` < env < CLI flags.
- Error envelope is exactly `{ "error": { "code": ..., "message": ... } }`. Allowed codes: `INVALID_QUERY`, `RATE_LIMIT_EXCEEDED`, `UPSTREAM_OVERLOADED`, `GBIF_TIMEOUT`, `GBIF_UNAVAILABLE`, `INTERNAL_ERROR`, `NOT_FOUND`, `UNRESOLVABLE`.
- Middleware order (immutable): Request-ID → Logging → Rate-Limiting → Load-Shedding → Timeouts → CORS → Metrics, OTel-instrumented.
- README.md / README.dev.md and all docs are **German**; code comments sparse, English only when necessary.
- Every PR updates **VERSION** and **CHANGELOG.md** (CI enforces).
- No `TODO/FIXME/HACK/XXX` markers in tracked non-PoC code (debt-guard enforces). `poc/**` is exempt from `verify`.
- Load-test stack is explicitly **out of scope**.
- ortus is the porting source of truth at `/Users/jbrunner/work/projects/ortus`; adapt module name → hostus, env prefix → `HOSTUS_`, drop SpatiaLite/CGO, keep distroless.

### Definition of Done for every TDD task (mutation + test-lint gate)

Applies to every task that produces Go logic (S5, S6, S7, S8, S9, S10, and all SP1+ logic). In addition to "tests pass", a task is only done when both hold:

1. **Mutation-green:** run `make mutation PKG=<the touched package>` (gremlins). The package must meet its configured efficacy/coverage thresholds — surviving mutants mean the tests don't pin behavior; add assertions until green. This runs *after* the unit tests pass, before the commit.
2. **Tests are linted:** `_test.go` files pass `golangci-lint` too. `.golangci.yml` (Task S4) must NOT blanket-exempt tests — only narrowly exempt what is genuinely test-only noise (documented per rule). `make lint` covering test files is part of the task's final check.

Pure scaffolding/config tasks (S1–S4, S11–S15) have no logic to mutate; the mutation gate does not apply, but any Go files they add still pass lint. Each TDD task's final step sequence is therefore: run tests (pass) → `make mutation PKG=...` (green) → `make lint` incl. tests (clean) → commit.

---

# Part 1 — Investigation (Phase R + Phase 0)

> These tasks produce `docs/research/**` and `poc/**` + fixtures. They do NOT participate in `make verify`. `poc/` gets its own `go.mod` so PoC deps never leak into the service module. Each PoC's gate verdict (🟢 pass / 🔴 architecture-change-needed) is recorded in its findings file and gates the named SP.

## Task R1: Quellenregister (Phase R)

**Files:**
- Create: `docs/research/quellenregister.md`

**Interfaces:**
- Produces: a validated source register consumed by every ingest pipeline (SP1–SP6) and by the Part-1 PoCs (they need the real URLs it discovers).

- [ ] **Step 1: Run the deep-research harness**

Invoke the `deep-research` skill with this scoped question (weave in the spec §2 source list):

> "For each of these botanical taxonomy data sources, find and validate the authoritative download/API URL, license (with attribution requirement), data format, access/auth method, and the current pinned version/period. Sources: COL XR (GBIF checklistKey 7ddf754f-d193-4cc9-b351-99906754a03b), WCVP/POWO, Euro+Med (ColDP/CDM), FloraVeg.EU/ESy, EIVE 1.0 (Dengler et al. 2023), Tichý et al. 2023, Midolo 2023, GermanSL, EuroSL, Wikidata properties P14607/P846/P10585/P12380/P12100, iNaturalist API, PlantNet API, TaxRef + Bayern 'Bayernstatus', ASK/FIN-Web, Wisskirchen concept relations at portal.cybertaxonomy.org/rotelisten_flora_deutschland, IPNI, WFO. Cross-check every ✅ claim in the appendix of the source document against the primary source."

- [ ] **Step 2: Record the register**

Write `docs/research/quellenregister.md` with one row per source: `Quelle | URL | Lizenz | Format | Zugriff/Auth | Version/Periode | Status`. Status is ✅ validated / ⚠️ order-of-magnitude only / ❌ blocked. Add a citation link per row.

- [ ] **Step 3: Flag blockers**

For every ❌, add a paragraph naming the SP it blocks and the fallback. Wisskirchen/CDM (P8) is the highest risk — call it out explicitly.

- [ ] **Step 4: Commit**

```bash
git add docs/research/quellenregister.md
git commit -m "docs(research): add validated source register (Phase R)"
```

## Task 0.0: PoC module bootstrap

**Files:**
- Create: `poc/go.mod`, `poc/README.md`, `poc/.gitignore`

- [ ] **Step 1: Create an isolated PoC module**

```bash
mkdir -p poc && cd poc
go mod init github.com/jobrunner/hostus-poc
printf '/data/\n*.sqlite\n*.zip\n' > .gitignore
printf '# PoC / Annahmen-Verifikation\n\nThrowaway code verifying spec assumptions against real data. NOT part of `make verify`. Each PoC has a findings file `PXX-findings.md` with a 🟢/🔴 verdict.\n' > README.md
```

- [ ] **Step 2: Exclude poc/ from the service build and verify**

Confirm `poc/` has its own module so `go build ./...` at repo root ignores it. Add `poc/` to the debt-guard ignore list (see Task S3, `scripts/debt-guard.sh` skips `poc/**`).

- [ ] **Step 3: Commit**

```bash
git add poc/go.mod poc/README.md poc/.gitignore
git commit -m "chore(poc): bootstrap isolated PoC module"
```

## Task 0.1: P1 — modernc.org/sqlite FTS5 (gates SP1/SP2)

**Files:**
- Create: `poc/p01_fts5/main.go`, `poc/P01-findings.md`

- [ ] **Step 1: Write the probe**

In `poc/p01_fts5/main.go`: open an in-memory `modernc.org/sqlite` DB, create `CREATE VIRTUAL TABLE t USING fts5(canonical, tokenize='unicode61 remove_diacritics 2')`, insert `Corynephorus canescens`, `Corynephorus divaricatus`, `Festuca ovina`, then run a prefix query `SELECT canonical, bm25(t) FROM t WHERE t MATCH 'coryn*' ORDER BY bm25(t)`.

- [ ] **Step 2: Run it**

```bash
cd poc/p01_fts5 && go get modernc.org/sqlite && go run .
```
Expected: both *Corynephorus* rows returned, `bm25()` produces an orderable score, no "no such module: fts5" error.

- [ ] **Step 3: Record verdict**

Write `poc/P01-findings.md`: does FTS5 compile in the pure-Go driver out of the box (build tag needed?), does `bm25()` work, prefix `*` behavior, `remove_diacritics 2` behavior. Verdict 🟢/🔴. If 🔴 (no FTS5), record the fallback (own trigram/prefix index) and flag spec §4.3 for change.

- [ ] **Step 4: Commit**

```bash
git add poc/p01_fts5 poc/P01-findings.md poc/go.mod poc/go.sum
git commit -m "poc(P1): verify modernc.org/sqlite FTS5 prefix + bm25"
```

## Task 0.2: P2 — WCVP ColDP structure (gates SP1/SP2)

**Files:**
- Create: `poc/p02_wcvp/inspect.sh`, `poc/P02-findings.md`, `internal/adapters/coldp/testdata/wcvp-sample/` (checked-in fixture slice)

- [ ] **Step 1: Fetch and inspect**

Using the WCVP ColDP URL from the Quellenregister, in `poc/p02_wcvp/inspect.sh`: download the archive to `poc/data/`, unzip, and print the header rows + 5 data rows of `NameUsage.tsv` (or `Taxon.tsv`/`Name.tsv` per the actual ColDP layout) and `Distribution.tsv`.

- [ ] **Step 2: Run it**

```bash
cd poc/p02_wcvp && bash inspect.sh
```
Expected: confirm presence of IPNI id column, accepted/synonym linkage column, basionym linkage, and WGSRPD level-3 area codes in Distribution.

- [ ] **Step 3: Cut a fixture**

Extract the rows for the spec reference taxa (*Corynephorus canescens* IPNI 396681-1, *Jacobaea vulgaris* IPNI 226649-1, and a *Festuca ovina* aggregate) plus their synonyms into a tiny valid ColDP slice under `internal/adapters/coldp/testdata/wcvp-sample/`. This fixture is reused by SP1 importer tests.

- [ ] **Step 4: Record verdict**

`poc/P02-findings.md`: exact filenames, exact column names for each field the schema (spec §4.3) needs, ID stability notes. Verdict 🟢/🔴. Map every needed schema column → real ColDP column.

- [ ] **Step 5: Commit**

```bash
git add poc/p02_wcvp poc/P02-findings.md internal/adapters/coldp/testdata/wcvp-sample
git commit -m "poc(P2): verify WCVP ColDP structure + cut importer fixture"
```

## Task 0.3: P3 — GBIF v2 match vs v1 suggest with checklistKey (gates SP1)

**Files:**
- Create: `poc/p03_gbif/probe.sh`, `poc/P03-findings.md`

- [ ] **Step 1: Probe both endpoints**

In `probe.sh`, curl `https://api.gbif.org/v2/species/match?name=Corynephorus%20canescens&checklistKey=7ddf754f-d193-4cc9-b351-99906754a03b` and `https://api.gbif.org/v1/species/suggest?q=Corynephorus&checklistKey=...` (and without the param).

- [ ] **Step 2: Run it**

```bash
cd poc/p03_gbif && bash probe.sh
```
Expected: v2/match honors `checklistKey` and returns a COL-XR usageKey; v1/suggest ignores it (confirms spec §3 P3 and appendix claim).

- [ ] **Step 3: Record verdict + commit**

`poc/P03-findings.md` with the actual JSON keys hostus will consume. Verdict 🟢/🔴.
```bash
git add poc/p03_gbif poc/P03-findings.md
git commit -m "poc(P3): verify GBIF v2 match honors COL-XR checklistKey"
```

## Task 0.4: P4 — PlantNet returns gbif.id + powo.id (gates SP2/UC1)

**Files:**
- Create: `poc/p04_plantnet/probe.sh`, `poc/P04-findings.md`

- [ ] **Step 1: Probe with the real key**

Read the key from env `HOSTUS_PLANTNET_API_KEY`. POST a reference flower image to `https://my-api.plantnet.org/v2/identify/k-central-europe?api-key=$HOSTUS_PLANTNET_API_KEY`. Guard: if the env var is empty, print a clear "set HOSTUS_PLANTNET_API_KEY" message and exit non-zero.

- [ ] **Step 2: Run it**

```bash
cd poc/p04_plantnet && HOSTUS_PLANTNET_API_KEY="$HOSTUS_PLANTNET_API_KEY" bash probe.sh
```
Expected: each result carries `gbif.id` and `powo.id` (spec §3 P4).

- [ ] **Step 3: Record verdict + commit**

`poc/P04-findings.md`: presence/nullability of `gbif.id`/`powo.id`, the `k-*` project caveat, score-vs-confidence separation. Never commit the key.
```bash
git add poc/p04_plantnet poc/P04-findings.md
git commit -m "poc(P4): verify PlantNet returns gbif.id + powo.id"
```

## Task 0.5–0.10: P5–P10 (gate SP3/SP4/SP5)

Each follows the same shape — probe against the Quellenregister URL, write `poc/P0X-findings.md` with a 🟢/🔴 verdict, commit. Concise because they gate later SPs, not SP1.

- [ ] **Task 0.5 — P5 Euro+Med ColDP:** `poc/p05_euromed/` — confirm Euro+Med is obtainable as ColDP and note the CDM-UUID linkage column. Findings + commit `poc(P5): ...`.
- [ ] **Task 0.6 — P6 EIVE/Tichý/Midolo:** `poc/p06_traits/` — download the three trait tables; record columns, the M/N/R/L/T(+S) layout, niche-width column (EIVE only), and the join key to the taxonomy (EuroSL vs FloraVeg). Findings + commit.
- [ ] **Task 0.7 — P7 Wikidata properties:** `poc/p07_wikidata/query.rq` — SPARQL for Q159953 (*Corynephorus canescens*) fetching P14607/P846/P10585/P12380/P12100; confirm each resolves. Findings + commit.
- [ ] **Task 0.8 — P8 Wisskirchen/CDM (highest risk):** `poc/p08_cdm/` — probe the CDM portal API/export for machine-retrievable concept relations. If ❌, document the manual/semi-automatic fallback for SP5. Findings + commit.
- [ ] **Task 0.9 — P9 iNat obscured coords:** `poc/p09_inat/probe.sh` — query `api.inaturalist.org/v1/observations` for a protected taxon; measure obscured-cell size and check `positional_accuracy`/`geoprivacy`. Confirms/corrects the ⚠️ ~0.2° claim. Findings + commit.
- [ ] **Task 0.10 — P10 FloraVeg vs EIVE namespace:** `poc/p10_namespace/` — look up a *Festuca ovina*-group taxon in both FloraVeg.EU and EIVE/EuroSL; document the divergence that forces two namespaces. Findings + commit.

## Task 0.11: Phase 0 gate summary

**Files:**
- Create: `poc/GATE.md`

- [ ] **Step 1: Roll up verdicts**

Table: `PoC | Verdict | Gated SP | Action if 🔴`. State plainly whether SP1 is unblocked (needs P1🟢 P2🟢 P3🟢).

- [ ] **Step 2: Commit**

```bash
git add poc/GATE.md
git commit -m "docs(poc): Phase 0 gate summary"
```

---

# Part 2 — SP0: Harness & Skeleton

> Independent of Part 1. Produces the green hexagonal skeleton + full harness. Each task ends with a verifiable deliverable. Work on branch `feature/sp0-harness` (create at Task S1).

## Task S1: Nix flake, direnv, module bump

**Files:**
- Modify: `flake.nix`, `.envrc`, `go.mod`
- Create: `.envrc.local.sample`
- Reference-port from: `/Users/jbrunner/work/projects/ortus/flake.nix`, `.envrc`

**Interfaces:**
- Produces: a dev shell providing `go_1_26`, `gopls`, `golangci-lint`, `govulncheck`, `gotestsum`, `go-junit-report`, `goreleaser`, `act`, `actionlint`, `jq`, `sqlite`, `staticcheck`, `delve`; project-local `GOPATH=$PWD/.go`; `CGO_ENABLED=0`.

- [ ] **Step 1: Branch**

```bash
git checkout master && git checkout -b feature/sp0-harness
```

- [ ] **Step 2: Port the flake**

Copy ortus `flake.nix` structure. Adaptations: `pname = "hostus"`; drop `libspatialite`, drop `SPATIALITE_LIBRARY_PATH`; set `CGO_ENABLED = "0"`; keep the project-local GOPATH/GOCACHE shellHook; `buildGoModule` with `vendorHash = null`.

- [ ] **Step 3: Port .envrc**

`.envrc`: `use flake` + `dotenv_if_exists .envrc.local`. Create `.envrc.local.sample` documenting `HOSTUS_PLANTNET_API_KEY=` (used only by PoCs).

- [ ] **Step 4: Verify the shell**

Run: `nix develop -c go version`
Expected: `go version go1.26.x`. Then `nix develop -c bash -lc 'echo $CGO_ENABLED'` → `0`.

- [ ] **Step 5: Commit**

```bash
git add flake.nix flake.lock .envrc .envrc.local.sample go.mod
git commit -m "chore(build): nix flake + direnv for hostus 2.0, CGO-free"
```

## Task S2: Strip GBIF, lay down hexagonal skeleton

**Files:**
- Delete: `internal/gbif/`, `internal/taxonomy/`, `internal/api/suggest.go`, `internal/api/suggest_test.go`, `internal/api/openapi.go` (GBIF-shaped)
- Create: `internal/domain/doc.go`, `internal/application/doc.go`, `internal/ports/input/doc.go`, `internal/ports/output/doc.go`, `internal/adapters/doc.go`, `internal/app/doc.go`
- Modify: `cmd/hostus/main.go` (reduce to a stub that compiles)

**Interfaces:**
- Produces: the hexagonal package layout every later task fills in.

- [ ] **Step 1: Remove GBIF domain code**

```bash
git rm -r internal/gbif internal/taxonomy internal/api/suggest.go internal/api/suggest_test.go internal/api/openapi.go
```

- [ ] **Step 2: Create package stubs**

Each `doc.go` is a one-line `// Package X ...` doc comment establishing the package. Reduce `cmd/hostus/main.go` to `func main() {}` calling a stub `app.Run` you create in `internal/app/app.go` (returns nil for now).

- [ ] **Step 3: Verify it builds**

Run: `go build ./...`
Expected: success, no references to deleted packages.

- [ ] **Step 4: Commit**

```bash
git add -A
git commit -m "refactor: remove GBIF proxy, establish hexagonal skeleton"
```

## Task S3: Makefile verify gate + tech-debt ratchet trio

**Files:**
- Create/Modify: `Makefile`, `.debt-budget`, `.coverage-floors`, `.gremlins.yaml`, `scripts/debt-guard.sh`, `scripts/coverage-gate.sh`
- Reference-port from: ortus `Makefile`, `scripts/debt-guard.sh`, `scripts/coverage-gate.sh`, `.coverage-floors`, `.gremlins.yaml`

**Interfaces:**
- Produces: `make verify` = `fmt-check vet lint test arch debt-guard` + `go build ./...`; `make build/test/lint/security/bench/fmt` targets.

- [ ] **Step 1: Port the Makefile**

Adapt ortus Makefile: `MODULE := github.com/jobrunner/hostus`, binary `hostus` from `./cmd/hostus`, `GOTEST := gotestsum --format testdox --`. Keep `build build-all test test-unit test-integration test-coverage test-race bench fmt fmt-check vet lint lint-fix arch security-check vuln-check gosec licenses check verify hooks help`. Drop load-test targets. Keep `mutation` and make it **package-scoped and green-required** for the per-task DoD: `mutation: ; gremlins unleash --dry-run=false $(if $(PKG),$(PKG),./...)` (installs `github.com/go-gremlins/gremlins/cmd/gremlins@v0.5.1`); `.gremlins.yaml` holds per-package efficacy/coverage thresholds so a non-green run exits non-zero.

- [ ] **Step 2: Port debt-guard**

`scripts/debt-guard.sh`: (1) suppression budget from `.debt-budget`; (2) zero `TODO/FIXME/HACK/XXX`; **skip `poc/**` and `third_party/**`**. Drop ortus guard #3 (gpkg/zip extension) — not applicable yet; leave a hook comment.

- [ ] **Step 3: Seed ratchet files**

`.debt-budget` → `0`. `.coverage-floors` → start permissive (`TOTAL 0`); tighten per package as real code lands (SP1+).

- [ ] **Step 4: Verify the gate is green on the empty skeleton**

Run: `make verify`
Expected: PASS (fmt/vet/lint/test/arch/debt-guard all green on stubs).

- [ ] **Step 5: Commit**

```bash
git add Makefile scripts/debt-guard.sh scripts/coverage-gate.sh .debt-budget .coverage-floors
git commit -m "build: verify gate + tech-debt ratchet trio (ex ortus)"
```

## Task S4: golangci-lint v2 with hexagonal depguard

**Files:**
- Modify: `.golangci.yml`
- Create: `internal/application/boundary_test.go`
- Reference-port from: ortus `.golangci.yml`

**Interfaces:**
- Produces: lint config where `depguard` forbids `adapters`→`application`-internal and enforces the hexagon; `gomodguard_v2` blocklists heavy deps.

- [ ] **Step 1: Port config**

Adapt ortus `.golangci.yml` (v2 schema): enable `errcheck staticcheck bodyclose gocognit gocyclo gosec revive depguard gomodguard nolintlint`. `depguard` rules: `domain` may import nothing internal; `application` may import `domain`+`ports` only; `adapters` may not import other `adapters` or `app`. **Test files are linted** (per Global Constraints DoD): do NOT blanket-exempt `_test.go`. Only narrowly relax where genuinely test-only: allow `depguard` cross-boundary imports in `_test.go` (tests legitimately wire adapters together) and permit table-test complexity by raising `gocyclo`/`gocognit` thresholds for `_test.go` rather than disabling them. `gosec` stays on for tests. Document each relaxation with a comment naming the reason.

- [ ] **Step 2: Write a boundary guard test (negative)**

Add `internal/domain/forbidden_probe.go.txt` note is not needed; instead assert the rule exists: create `internal/application/boundary_test.go` that simply documents the intent and compiles. Real enforcement is the linter.

- [ ] **Step 3: Verify depguard rejects a violation**

Temporarily add `import _ "github.com/jobrunner/hostus/internal/adapters/http"` into `internal/domain/doc.go`, run `make lint`, expect a depguard error, then revert.

- [ ] **Step 4: Verify clean lint**

Run: `make lint`
Expected: PASS after revert.

- [ ] **Step 5: Commit**

```bash
git add .golangci.yml internal/application/boundary_test.go
git commit -m "build(lint): golangci v2 with hexagonal depguard boundaries"
```

## Task S5: config package (viper)

**Files:**
- Modify: `internal/config/config.go`
- Create: `internal/config/config_test.go`, `config.yaml.example`
- Reference-port from: ortus `internal/config/config.go`

**Interfaces:**
- Produces: `config.Load(path string) (*Config, error)`; `Config` with sections `server`, `logging`, `metrics`, `tls`, `telemetry`, `sqlite`, `cors` via `mapstructure` tags; env prefix `HOSTUS_`, replacer `.`→`_`.

- [ ] **Step 1: Write the failing test**

```go
func TestLoadPrefersEnvOverDefault(t *testing.T) {
	t.Setenv("HOSTUS_SERVER_PORT", "8443")
	cfg, err := Load("")
	if err != nil { t.Fatal(err) }
	if cfg.Server.Port != 8443 {
		t.Fatalf("got %d, want 8443", cfg.Server.Port)
	}
}
```

- [ ] **Step 2: Run it (fails)**

Run: `go test ./internal/config/ -run TestLoadPrefersEnvOverDefault -v`
Expected: FAIL (Load/Config not defined or wrong).

- [ ] **Step 3: Implement Load + Config + Defaults**

Port ortus's pattern: `viper.SetEnvPrefix("HOSTUS")`, `SetEnvKeyReplacer(strings.NewReplacer(".", "_"))`, `AutomaticEnv()`, `SetConfigName("config")`, search paths `.`, `./config`, `/etc/hostus`; `Defaults()` sets `server.port` default; unmarshal via `go-viper/mapstructure/v2`.

- [ ] **Step 4: Run it (passes)**

Run: `go test ./internal/config/ -v`
Expected: PASS.

- [ ] **Step 5: Write config.yaml.example + commit**

Document every section with inline German comments.
```bash
git add internal/config/config.go internal/config/config_test.go config.yaml.example
git commit -m "feat(config): viper config with HOSTUS_ prefix"
```

## Task S6: httperr with new error codes

**Files:**
- Modify: `internal/httperr/errors.go`
- Create: `internal/httperr/errors_test.go`

**Interfaces:**
- Produces: `httperr.Write(w http.ResponseWriter, status int, code, msg string)` emitting `{"error":{"code","message"}}`; exported code constants incl. `CodeNotFound = "NOT_FOUND"`, `CodeUnresolvable = "UNRESOLVABLE"`.

- [ ] **Step 1: Write the failing test**

```go
func TestWriteEnvelope(t *testing.T) {
	rr := httptest.NewRecorder()
	httperr.Write(rr, http.StatusNotFound, httperr.CodeNotFound, "concept not found")
	var got struct{ Error struct{ Code, Message string } }
	json.Unmarshal(rr.Body.Bytes(), &got)
	if got.Error.Code != "NOT_FOUND" || rr.Code != 404 {
		t.Fatalf("bad envelope: %d %+v", rr.Code, got)
	}
}
```

- [ ] **Step 2: Run (fails)** — `go test ./internal/httperr/ -v` → FAIL.
- [ ] **Step 3: Implement** the envelope writer + the 8 code constants from Global Constraints.
- [ ] **Step 4: Run (passes)** — `go test ./internal/httperr/ -v` → PASS.
- [ ] **Step 5: Commit**

```bash
git add internal/httperr/
git commit -m "feat(httperr): error envelope with NOT_FOUND + UNRESOLVABLE"
```

## Task S7: OTel telemetry adapter with in-memory exporters

**Files:**
- Create: `internal/adapters/telemetry/telemetry.go`, `internal/adapters/telemetry/memory.go`, `internal/adapters/telemetry/telemetry_test.go`

**Interfaces:**
- Produces:
  - `telemetry.Setup(ctx, cfg) (*Providers, func(context.Context) error, error)` wiring trace + metric providers with an OTLP exporter **and** an in-memory exporter (both installed as span processors).
  - `telemetry.NewMemoryExporter(capacity int) *MemoryExporter` — ring buffer of finished spans; `Spans() []SpanRecord`, `Trace(traceID string) []SpanRecord`.
  - `telemetry.NewRingLog(capacity int) *RingLog` implementing `slog.Handler`, injecting `trace_id`/`span_id` from context; `Records(min slog.Level, limit int) []LogRecord`.
  - `SpanRecord{TraceID, SpanID, Name string; Start, End time.Time; DurationMS float64; Attrs map[string]string}`, `LogRecord{Time time.Time; Level, Msg, TraceID, SpanID string; Attrs map[string]string}`.

- [ ] **Step 1: Write the failing test (span round-trips through memory exporter)**

```go
func TestMemoryExporterCapturesSpan(t *testing.T) {
	exp := telemetry.NewMemoryExporter(16)
	tp := trace.NewTracerProvider(trace.WithSpanProcessor(trace.NewSimpleSpanProcessor(exp)))
	_, span := tp.Tracer("t").Start(context.Background(), "op")
	span.End()
	_ = tp.ForceFlush(context.Background())
	if got := exp.Spans(); len(got) != 1 || got[0].Name != "op" {
		t.Fatalf("want 1 span 'op', got %+v", got)
	}
}
```

- [ ] **Step 2: Run (fails)** — `go test ./internal/adapters/telemetry/ -v` → FAIL (undefined).
- [ ] **Step 3: Implement** `MemoryExporter` (implements `sdktrace.SpanExporter`: `ExportSpans`/`Shutdown`, ring buffer, maps `sdktrace.ReadOnlySpan`→`SpanRecord`), `RingLog` (`slog.Handler`), and `Setup` (OTLP http exporter from `cfg.Telemetry.Endpoint` + the memory exporter + a metric provider; return a shutdown func).
- [ ] **Step 4: Run (passes)** — `go test ./internal/adapters/telemetry/ -v` → PASS.
- [ ] **Step 5: Add a RingLog test**

```go
func TestRingLogFiltersByLevel(t *testing.T) {
	rl := telemetry.NewRingLog(16)
	l := slog.New(rl)
	l.Info("hi"); l.Error("boom")
	if errs := rl.Records(slog.LevelError, 10); len(errs) != 1 || errs[0].Msg != "boom" {
		t.Fatalf("want 1 error 'boom', got %+v", errs)
	}
}
```
Run → PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/adapters/telemetry/
git commit -m "feat(telemetry): OTel setup + in-memory span/log exporters"
```

## Task S8: HTTP middleware chain + health endpoints

**Files:**
- Modify (port): `internal/middleware/*.go` (requestid, logging, ratelimit, loadshed, timeout, cors, metrics)
- Create: `internal/adapters/http/router.go`, `internal/adapters/http/health.go`, `internal/adapters/http/router_test.go`

**Interfaces:**
- Consumes: `telemetry` (Task S7) for otel-instrumented logging; `config` (S5).
- Produces: `http.NewRouter(deps Deps) *mux.Router` mounting the middleware chain (exact order from Global Constraints, wrapped by `otelmux.Middleware`) + `GET /health/live`, `GET /health/ready`, `GET /metrics`.

- [ ] **Step 1: Write the failing test**

```go
func TestHealthLive(t *testing.T) {
	r := httpx.NewRouter(httpx.Deps{})
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, httptest.NewRequest("GET", "/health/live", nil))
	if rr.Code != 200 { t.Fatalf("got %d", rr.Code) }
}
```

- [ ] **Step 2: Run (fails)** → FAIL.
- [ ] **Step 3: Implement** the router: keep the existing middleware implementations (port them under `internal/adapters/http/middleware/` or keep `internal/middleware` and import it), assemble in the fixed order, wrap with `otelmux.Middleware("hostus")`, mount health + `promhttp` metrics.
- [ ] **Step 4: Run (passes)** → PASS. Add a test asserting the `X-Request-ID` header is present on responses.
- [ ] **Step 5: Commit**

```bash
git add internal/adapters/http/ internal/middleware/
git commit -m "feat(http): otel-instrumented middleware chain + health"
```

## Task S9: cobra CLI (serve/version/ingest/validate/bundle stubs)

**Files:**
- Modify: `cmd/hostus/main.go`
- Create: `cmd/hostus/root.go`, `cmd/hostus/serve.go`, `cmd/hostus/version.go`, `cmd/hostus/ingest.go`, `cmd/hostus/validate.go`, `cmd/hostus/bundle.go`, `cmd/hostus/version_test.go`
- Reference-port from: ortus `cmd/ortus/main.go`

**Interfaces:**
- Consumes: `app.Run` (S2), `config.Load` (S5).
- Produces: root command `hostus` (default `serve`); `version` prints `-ldflags`-injected `Version/Commit/BuildDate`; `ingest`/`validate`/`bundle` are stubs returning `errNotImplemented` (wired fully in SP1/SP2).

- [ ] **Step 1: Write the failing test**

```go
func TestVersionCommand(t *testing.T) {
	buf := new(bytes.Buffer)
	cmd := newRootCmd()
	cmd.SetOut(buf); cmd.SetArgs([]string{"version"})
	if err := cmd.Execute(); err != nil { t.Fatal(err) }
	if !strings.Contains(buf.String(), "hostus") { t.Fatalf("got %q", buf.String()) }
}
```

- [ ] **Step 2: Run (fails)** → FAIL.
- [ ] **Step 3: Implement** cobra tree; bind flags via `viper.BindPFlag`; `serve` calls `app.Run`; stubs for ingest/validate/bundle.
- [ ] **Step 4: Run (passes)** → PASS. Then `go run ./cmd/hostus version` prints version line.
- [ ] **Step 5: Commit**

```bash
git add cmd/hostus/
git commit -m "feat(cli): cobra root serve + version + ingest/validate/bundle stubs"
```

## Task S10: Debug MCP (logs + spans for Claude Code)

**Files:**
- Create: `internal/adapters/mcp/server.go`, `internal/adapters/mcp/tools.go`, `internal/adapters/mcp/server_test.go`
- Modify: `cmd/hostus/mcp.go` (new subcommand)
- Reference-port from: ortus `internal/adapters/mcp/` (stdio wiring only)

**Interfaces:**
- Consumes: `telemetry.MemoryExporter` + `telemetry.RingLog` (S7).
- Produces: `mcp.NewServer(log *telemetry.RingLog, spans *telemetry.MemoryExporter) *Server` exposing stdio MCP tools:
  - `get_recent_logs(level?, limit?)` → recent `LogRecord`s.
  - `tail_errors(limit?)` → error/warn `LogRecord`s.
  - `get_trace(trace_id)` → `SpanRecord`s of that trace.
  - `list_spans(operation?, min_duration_ms?, limit?)` → filtered `SpanRecord`s.
  All read-only. `hostus mcp` logs to stderr (stdout is JSON-RPC).

- [ ] **Step 1: Write the failing test (tool returns buffered errors)**

```go
func TestTailErrorsTool(t *testing.T) {
	rl := telemetry.NewRingLog(16)
	slog.New(rl).Error("kaboom")
	srv := mcp.NewServer(rl, telemetry.NewMemoryExporter(16))
	out, err := srv.CallTool(context.Background(), "tail_errors", map[string]any{"limit": 5})
	if err != nil { t.Fatal(err) }
	if !strings.Contains(out, "kaboom") { t.Fatalf("got %q", out) }
}
```

- [ ] **Step 2: Run (fails)** → FAIL.
- [ ] **Step 3: Implement** the server with `modelcontextprotocol/go-sdk`, registering the four tools with JSON schemas; `CallTool` is a thin test seam over the SDK dispatch.
- [ ] **Step 4: Run (passes)** → PASS. Add a `get_trace` test that seeds a span via a `MemoryExporter` and retrieves it by trace id.
- [ ] **Step 5: Wire the subcommand + commit**

`cmd/hostus/mcp.go`: `hostus mcp` builds the same telemetry providers used by serve (so the buffers are shared) and serves MCP over stdio.
```bash
git add internal/adapters/mcp/ cmd/hostus/mcp.go
git commit -m "feat(mcp): stdio debug MCP exposing logs + spans"
```

## Task S11: Distroless multi-arch Docker

**Files:**
- Modify: `Dockerfile`, `docker-compose.yml`, `.dockerignore`
- Reference-port from: ortus `Dockerfile` (structure only — hostus is CGO-free so use `gcr.io/distroless/static`)

**Interfaces:**
- Produces: multi-stage build, `CGO_ENABLED=0 go build`, distroless `static` runtime, non-root, `HEALTHCHECK` on `/health/live`, `HOSTUS_*` env.

- [ ] **Step 1: Write the Dockerfile** — builder `golang:1.26` → `CGO_ENABLED=0 GOFLAGS=-trimpath` build → `FROM gcr.io/distroless/static:nonroot`; copy binary; `ENTRYPOINT ["/hostus"]`.
- [ ] **Step 2: Build both arches**

Run: `docker buildx build --platform linux/amd64,linux/arm64 -t hostus:dev .`
Expected: both succeed.

- [ ] **Step 3: Smoke test** — `docker run --rm hostus:dev version` prints version.
- [ ] **Step 4: hadolint** — `hadolint Dockerfile` clean.
- [ ] **Step 5: Commit**

```bash
git add Dockerfile docker-compose.yml .dockerignore
git commit -m "build(docker): CGO-free distroless multi-arch image"
```

## Task S12: CI workflows (ported, load-test excluded)

**Files:**
- Create/Modify under `.github/workflows/`: `ci.yml`, `vuln-scan.yml`, `secret-scan` (in ci or standalone), `actions-security.yml`, `commitlint.yml`, `openapi-diff.yml`, `mutation.yml`, `fuzz.yml`, `codecharta.yml`, `docker-release.yml`, `release-please.yml`, `update-skills-submodule.yml`
- Create: `.github/dependabot.yml`, `release-please-config.json`, `.release-please-manifest.json`, `.commitlintrc.yml`, `.github/zizmor.yml`, `.actrc`, `.gitleaks.toml`
- Reference-port from: the same-named files in ortus `.github/`

**Interfaces:**
- Produces: green CI on the skeleton. `ci.yml` jobs: `version` (VERSION+CHANGELOG check), `lint` (golangci v2), `test` (race+coverage+coverage-gate; **no SpatiaLite install**), `bench`, `security` (govulncheck), `licenses`, `docs` (mkdocs strict), `secret-scan` (gitleaks), `architecture` (`go mod tidy -diff` + debt-guard), `build` (+ `./hostus version` smoke), `docker-lint` (hadolint), `actions-lint` (actionlint+shellcheck), `docker-build` (matrix amd64/arm64), `docker-security` (Trivy→SARIF).

- [ ] **Step 1: Port ci.yml** — copy ortus `ci.yml`; set `GO_VERSION: '1.26.x'`; **remove the `libsqlite3-mod-spatialite`/`SPATIALITE_LIBRARY_PATH` steps**; module → hostus; keep least-privilege `permissions`.
- [ ] **Step 2: Port the rest** — adapt names/paths; `docker-release.yml` keeps SLSA `provenance: mode=max`, SPDX `sbom: true`, cosign keyless.
- [ ] **Step 3: Add supply-chain config** — dependabot (gomod + github-actions + **gitsubmodule** weekly), release-please config+manifest, commitlint, zizmor, gitleaks, actrc.
- [ ] **Step 4: Lint the workflows**

Run: `actionlint` and `make ci-check`
Expected: clean.

- [ ] **Step 5: Dry-run core CI locally**

Run: `act -j lint` (or `make ci-lint`)
Expected: passes on the skeleton.

- [ ] **Step 6: Commit**

```bash
git add .github/ release-please-config.json .release-please-manifest.json .commitlintrc.yml .actrc .gitleaks.toml
git commit -m "ci: port ortus workflows (load-test excluded), CGO-free"
```

## Task S13: claude-skills submodule + git/claude hooks

**Files:**
- Create: `.gitmodules`, `third_party/claude-skills` (submodule), `.githooks/pre-commit`, `.claude/hooks/*.sh`, `.claude/settings.json`
- Reference-port from: ortus `.gitmodules`, `.githooks/`, `.claude/hooks/`

**Interfaces:**
- Produces: the private skills submodule + pre-commit hook (fmt-check → build → debt-guard).

- [ ] **Step 1: Add the submodule**

```bash
git submodule add -b main https://github.com/jobrunner/claude-skills.git third_party/claude-skills
```
(If auth fails in this environment, record the exact `.gitmodules` entry to add manually and continue.)

- [ ] **Step 2: Port hooks** — `.githooks/pre-commit` runs `scripts` fmt-check → `go build ./...` → `debt-guard.sh`; port relevant `.claude/hooks/*.sh` (format-and-lint, check-version-changelog, validate-version, security-scan-docker), adapting to hostus. Register via `make hooks` (`git config core.hooksPath .githooks`).
- [ ] **Step 3: Verify the hook fires**

Run: `make hooks && git commit --allow-empty -m "test: hook" ` then observe fmt/build/debt-guard run; amend/drop the empty commit.

- [ ] **Step 4: Commit**

```bash
git add .gitmodules third_party/claude-skills .githooks .claude
git commit -m "chore: add claude-skills submodule + git/claude hooks"
```

## Task S14: MkDocs Diátaxis + OpenAPI scaffold + doc-drift

**Files:**
- Create: `mkdocs.yml`, `docs/index.md`, `docs/tutorials/`, `docs/how-to/`, `docs/reference/`, `docs/explanation/decisions/` (ADRs live here), `api/openapi/openapi.yaml` (stub)
- Reference-port from: ortus `mkdocs.yml`, `docs/` layout

**Interfaces:**
- Produces: `mkdocs build --strict` green; Diátaxis skeleton; `openapi-diff.yml` has a baseline.

- [ ] **Step 1: Port mkdocs.yml** (Material theme, Diátaxis nav), move existing `README.dev.md` content into `docs/` as appropriate, keep German.
- [ ] **Step 2: Add an OpenAPI baseline stub** (`/health/live`, `/health/ready`, `/metrics`) so `openapi-diff` has something to diff in SP1+.
- [ ] **Step 3: Verify** — `uvx --with mkdocs-material mkdocs build --strict` → PASS.
- [ ] **Step 4: Commit**

```bash
git add mkdocs.yml docs/ api/openapi/
git commit -m "docs: MkDocs Material/Diátaxis scaffold + OpenAPI baseline"
```

## Task S15: ADR supersession + release config + version bump

**Files:**
- Create: `docs/explanation/decisions/0009-local-multibackbone-index.md` … through new ADRs; update `architecture/adrs.md` marking ADR-001/-003/-008 **Superseded**
- Modify: `VERSION` → `2.0.0-alpha.0`, `CHANGELOG.md`, `.goreleaser.yml`
- Reference-port from: ortus `.goreleaser.yml`

**Interfaces:**
- Produces: new ADRs (local multi-backbone index; SQLite/FTS5 persistence; artifact contract; hexagonal; OTel; debug-MCP) superseding the no-persistence/GBIF-only ADRs; goreleaser append-mode config.

- [ ] **Step 1: Write the ADRs** — one file per decision, each with Status/Kontext/Entscheidung/Konsequenzen; mark old ones Superseded with a pointer.
- [ ] **Step 2: goreleaser** — port ortus `.goreleaser.yml`, `release.mode: append`, multi-arch, checksums.
- [ ] **Step 3: Bump VERSION + CHANGELOG** — `2.0.0-alpha.0`; CHANGELOG entry summarizing SP0.
- [ ] **Step 4: Verify** — `make verify` still green; `goreleaser release --snapshot --clean` builds.
- [ ] **Step 5: Commit**

```bash
git add architecture/adrs.md docs/explanation/decisions/ VERSION CHANGELOG.md .goreleaser.yml
git commit -m "docs(adr)+build(release): supersede v1 ADRs, goreleaser, v2.0.0-alpha.0"
```

## Task S16: Final SP0 gate

- [ ] **Step 1: Full green check**

Run: `make verify && make security && go run ./cmd/hostus version && echo '{"jsonrpc":"2.0",...}' | go run ./cmd/hostus mcp` (smoke the MCP handshake).
Expected: verify green, security green, version prints, MCP responds.

- [ ] **Step 2: Update CHANGELOG + open PR**

```bash
git push -u origin feature/sp0-harness
gh pr create --title "SP0: harness + hexagonal skeleton (hostus 2.0)" --body "Full ortus-parity harness (minus load-test), OTel + debug MCP, claude-skills submodule, distroless CGO-free image, ~12 CI workflows. Skeleton is green through make verify. See docs/superpowers/plans/2026-07-31-sp0-harness-and-investigation.md

https://claude.ai/code/session_01KcXepAoctj5rBVV3XVdRaB"
```

---

## Self-Review Notes

- **Spec coverage:** Phase R → R1; Phase 0 P1–P10 → 0.1–0.10 + gate 0.11; hexagonal layout → S2; SQLite/FTS5 driver decision → verified in P1, schema itself lands in SP1 (not SP0, correct); OTel → S7/S8; debug-MCP → S10; verify gate + ratchet trio → S3; depguard boundaries → S4; config/httperr → S5/S6; distroless CGO-free → S11; CI (load-test excluded) → S12; claude-skills submodule → S13; MkDocs → S14; ADR supersession + release + v2.0.0-alpha.0 → S15. The 7 domain endpoints and the ingest/bundle logic are intentionally deferred to SP1–SP6 (stubs only here) — consistent with spec §7 build order.
- **Placeholder scan:** ingest/validate/bundle are explicit stubs (returning `errNotImplemented`), not placeholders — their real plans are SP1/SP2. No TODO/FIXME left in tracked code (debt-guard enforces).
- **Type consistency:** `telemetry.MemoryExporter`/`RingLog`/`SpanRecord`/`LogRecord` defined in S7 are consumed with the same names/signatures in S10; `config.Load`/`Config` from S5 consumed in S9; `httperr` codes from Global Constraints used in S6.

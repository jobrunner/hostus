# hostus 2.0 — SP1 Foundation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax.

**Goal:** Stand up the local backbone core — a SQLite/FTS5-backed read model populated from a WCVP DwC-A ingest, exposing `GET /v1/concept/{id}`, `GET /v1/xref`, and `POST /v1/match` (exact + exact_author).

**Architecture:** Hexagonal. Domain types (Name/Concept/Xref/Distribution) + pure resolution logic; `ports/output` Repository interface; `application` use cases (Ingest, ResolveConcept, ReverseXref, MatchNames); `adapters/sqlite` (modernc.org/sqlite, embedded schema); `adapters/wcvp` (DwC-A reader); `adapters/http` handlers; cobra `ingest`/`validate` wired (bundle stays a stub → SP2). A versioned `dataset.yaml` manifest validated against an embedded JSON Schema gates every ingest.

**Tech Stack:** Go 1.26.5, `modernc.org/sqlite` (FTS5, CGO-free), `gorilla/mux`, `cobra`/`viper`, OTel (from SP0), `github.com/google/jsonschema-go` (manifest validation, as ortus uses).

## Global Constraints

- Builds on SP0 (branch forked from `feature/investigation`): harness, hexagonal skeleton, config, telemetry, httperr, CLI stubs, and the WCVP fixture at `internal/adapters/coldp/testdata/wcvp-sample/` are already present.
- SQLite driver **`modernc.org/sqlite`** (pure-Go, FTS5 works out of the box per PoC P1 — driver name `"sqlite"`, `CREATE VIRTUAL TABLE ... USING fts5`, `remove_diacritics 2`, `bm25()`/`rank` orderable). CGO stays 0.
- **WCVP is DwC-A, not ColDP** (PoC P2). Files are pipe (`|`)-delimited, no quoting. `wcvp_taxon.csv` header (verbatim, incl. the upstream typos `scientfiicname`/`scientfiicnameauthorship`): `taxonid|family|genus|specificepithet|infraspecificepithet|scientfiicname|scientfiicnameauthorship|taxonrank|taxonomicstatus|acceptednameusageid|parentnameusageid|originalnameusageid|namepublishedin|nomenclaturalstatus|taxonremarks|scientificnameid|dynamicproperties|references`. `wcvp_distribution.csv`: `coreid|locality|establishmentmeans|locationid|occurrencestatus|threatstatus` (`locationid` = `TDWG:<L3code>`). `wcvp_replacementNames.csv`: `taxonid|relatednameusageid|relationtype|remarks`.
- **IPNI id: use the `powoid` inside the `dynamicproperties` JSON**, NOT `scientificnameid` (~30–40% empty). `originalnameusageid` = basionym link; `acceptednameusageid` = accepted concept for a synonym (empty ⇒ this row IS accepted); `taxonomicstatus` ∈ {Accepted, Synonym, ...}.
- **GBIF v2 match** (for optional COL-XR enrichment during ingest) uses param `scientificName` (not `name`) and returns ids nested under `usage.*` (PoC P3). Not required for SP1's WCVP-only ingest; keep GBIF out of the request path.
- Error envelope `{"error":{"code","message"}}`; use `httperr` codes incl. `NOT_FOUND` (unknown concept/xref id) and `UNRESOLVABLE` (verbatim name matches nothing). Never `GBIF_*` on the serving path.
- Ranks normalized to: FAMILY, GENUS, SPECIES, SUBSPECIES, VARIETY, FORM. Match types this SP: `exact`, `exact_author`, `aggregate_alias`; `fuzzy` is SP3 (a name that only fuzzy-matches returns no candidate + `UNRESOLVABLE` this SP).
- **Per-task DoD (from SP0):** unit tests pass → `make mutation PKG=<pkg>` green (ZERO surviving mutants; justify equivalents) → `make lint` clean incl. `_test.go` → `make verify` green. Run tooling via `nix develop -c ...`. Tests verify real behavior (real temp SQLite, real fixture), not mocks.
- `depguard`: domain imports nothing internal; application imports domain+ports only; adapters don't import other adapters or app. Manifest validation dep `github.com/google/jsonschema-go` must be allowed by gomodguard (add with justification if blocked).
- release-please owns VERSION/CHANGELOG; accumulate notes under `## [Unreleased]`.

---

## Task 1: Domain types + pure resolution logic

**Files:**
- Create: `internal/domain/taxon.go`, `internal/domain/match.go`, `internal/domain/taxon_test.go`, `internal/domain/match_test.go`

**Interfaces (Produces):**
- `type Rank string` (const FAMILY…FORM) + `ParseRank(string) (Rank, error)` (maps WCVP `taxonrank` spellings, case-insensitive).
- `type Status string` (StatusAccepted, StatusSynonym, …) + `ParseStatus(string) Status`.
- `type Name struct { ID, Canonical, Authorship string; Rank Rank; IPNIID, PublishedIn, NomStatus, BasionymID string }`
- `type Concept struct { ID string; BackboneID, BackboneVersion string; AcceptedName Name; Rank Rank; ParentID, SecReference string; Status Status }`
- `type Xref struct { Authority, ExtID string }`
- `type Distribution struct { AreaScheme, AreaCode string }`
- `type MatchType string` (MatchExact, MatchExactAuthor, MatchAggregateAlias)
- `Canonicalize(name string) string` — trims, collapses whitespace, folds diacritics + case for comparison keys (mirrors FTS `remove_diacritics`). Pure.
- `NormalizeAuthor(a string) string` — normalizes botanical author strings for comparison (strip spaces around `.`/`&`, unify `L.`/`Linnaeus`? keep minimal: whitespace + punctuation normalization; document what it does/doesn't do).
- `ClassifyMatch(queryCanon, queryAuthor, candCanon, candAuthor string) (MatchType, bool)` — returns (exact_author when both canon+author match; exact when canon matches and query has no author; ok=false otherwise). Pure, the heart of §B.2.

- [ ] **Step 1: failing tests** — table tests for `ParseRank`/`ParseStatus` (incl. WCVP spellings), `Canonicalize` (diacritics, whitespace, case), `ClassifyMatch` (exact vs exact_author vs no-match; author present/absent; the *Silene otites*/*otitis* pair must NOT exact-match).

```go
func TestClassifyMatch(t *testing.T) {
	cases := []struct{ qc, qa, cc, ca string; want domain.MatchType; ok bool }{
		{"corynephorus canescens", "(l.) p.beauv.", "corynephorus canescens", "(l.) p.beauv.", domain.MatchExactAuthor, true},
		{"corynephorus canescens", "", "corynephorus canescens", "(l.) p.beauv.", domain.MatchExact, true},
		{"silene otites", "", "silene otitis", "", "", false},
	}
	for _, c := range cases {
		got, ok := domain.ClassifyMatch(c.qc, c.qa, c.cc, c.ca)
		if ok != c.ok || (ok && got != c.want) { t.Fatalf("%+v: got %q,%v", c, got, ok) }
	}
}
```

- [ ] **Step 2: run → RED** (`nix develop -c go test ./internal/domain/ -v`).
- [ ] **Step 3: implement** the types + pure funcs (no I/O, no internal imports).
- [ ] **Step 4: run → GREEN.**
- [ ] **Step 5: DoD** — `make mutation PKG=./internal/domain` (0 survivors), `make lint`, `make verify`.
- [ ] **Step 6: commit** `feat(domain): taxon/name/concept types + pure match classification`.

## Task 2: Repository port + SQLite schema

**Files:**
- Create: `internal/ports/output/repository.go`, `internal/adapters/sqlite/schema.sql` (embedded), `internal/adapters/sqlite/db.go`, `internal/adapters/sqlite/db_test.go`

**Interfaces:**
- Consumes: `domain` (T1).
- Produces (`ports/output`):
  ```go
  type Repository interface {
      Concept(ctx, id string) (*domain.Concept, []domain.Name /*synonyms*/, []domain.Xref, []domain.Distribution, error) // NOT_FOUND → domain.ErrNotFound
      ConceptByXref(ctx, authority, extID string) (*domain.Concept, error)
      MatchExact(ctx, canon string) ([]MatchCandidate, error) // rows whose canonical == canon (accepted + synonyms), for the app layer to classify
      BackboneVersions(ctx) ([]domain.BackboneVersion, error)
      // ingest side:
      BeginIngest(ctx, bv domain.BackboneVersion) (IngestTx, error)
  }
  type MatchCandidate struct { Concept domain.Concept; MatchedName domain.Name; Role string /*accepted|synonym*/ }
  type IngestTx interface { UpsertName(domain.Name) error; UpsertConcept(domain.Concept) error; LinkName(conceptID, nameID, role string, homotypic *bool) error; AddXref(conceptID string, x domain.Xref) error; AddDistribution(conceptID string, d domain.Distribution) error; Commit() error; Rollback() error }
  ```
- `sqlite.Open(path string) (*sqlite.DB, error)` applies the embedded schema (idempotent); `sqlite.DB` implements `output.Repository`. `:memory:` supported for tests.
- `schema.sql` = the tables from spec §4.3 (`backbone_version`, `name`, `taxon_concept`, `concept_name`, `xref`, `distribution`, `vernacular`, `trait_value`, `concept_relation`) + `CREATE VIRTUAL TABLE fts_name USING fts5(canonical, vernacular_de, content='', tokenize='unicode61 remove_diacritics 2')` with a rowid↔concept mapping. (trait_value/concept_relation created but unused until SP3/SP5.)

- [ ] **Step 1: failing test** — `Open(":memory:")` then assert the expected tables + the FTS5 virtual table exist (`SELECT name FROM sqlite_master`), and a `backbone_version` insert/read round-trips.
- [ ] **Step 2: RED.**
- [ ] **Step 3: implement** `schema.sql` (`//go:embed`), `Open` (exec schema, `PRAGMA foreign_keys=ON`), the `DB` struct + `BackboneVersions`. Driver `"sqlite"`.
- [ ] **Step 4: GREEN.**
- [ ] **Step 5: DoD** (`make mutation PKG=./internal/adapters/sqlite`, lint, verify).
- [ ] **Step 6: commit** `feat(sqlite): embedded schema + FTS5 + repository open`.

## Task 3: Repository reads (Concept, ConceptByXref, MatchExact)

**Files:** Modify `internal/adapters/sqlite/db.go`; Create `internal/adapters/sqlite/read.go`, `internal/adapters/sqlite/read_test.go`, `internal/adapters/sqlite/testdata/seed.sql`

**Interfaces:** implements the read methods of `output.Repository` (T2).

- [ ] **Step 1: failing tests** — seed a small `testdata/seed.sql` (2 concepts: *Corynephorus canescens* accepted with synonyms *Weingaertneria canescens*/*Aira canescens*, xrefs powo/colxr, WGSRPD dist; *Jacobaea vulgaris* with synonym *Senecio jacobaea*). Test `Concept(id)` returns accepted name + synonyms + xrefs + distribution; unknown id → `domain.ErrNotFound`. `ConceptByXref("powo","396681-1")` → the *Corynephorus* concept. `MatchExact("corynephorus canescens")` returns both the accepted row and, for a synonym query, the synonym row with its concept.
- [ ] **Step 2: RED.**
- [ ] **Step 3: implement** the SQL reads (joins across concept_name/name/xref/distribution).
- [ ] **Step 4: GREEN.**
- [ ] **Step 5: DoD.**
- [ ] **Step 6: commit** `feat(sqlite): concept/xref/match-exact reads`.

## Task 4: WCVP DwC-A reader

**Files:**
- `git mv internal/adapters/coldp/testdata/wcvp-sample internal/adapters/wcvp/testdata/wcvp-sample`; remove the now-empty `internal/adapters/coldp/` (ColDP/COL-XR importer is a later SP).
- Create: `internal/adapters/wcvp/reader.go`, `internal/adapters/wcvp/reader_test.go`

**Interfaces (Produces):**
- `wcvp.Read(dir string) (*wcvp.Dataset, error)` where `Dataset` yields, via iterator or slices, `TaxonRow`/`DistributionRow`/`ReplacementRow` structs with typed fields mapped from the pipe-delimited columns; plus `TaxonRow.POWOID() string` (parses `dynamicproperties` JSON → `powoid`).
- Reader is format-aware: pipe delimiter, no quoting, the verbatim header (handle the `scientfiicname` typo by mapping it explicitly). Read `poc/P02-findings.md` and the fixture for exact column semantics.

- [ ] **Step 1: failing test** — `Read("testdata/wcvp-sample")`; assert the *Corynephorus canescens* taxon row parses (canonical, author `(L.) P.Beauv.`, rank species, status accepted, `POWOID()=="396681-1"`), a synonym row has a non-empty `acceptednameusageid`, and distribution rows expose `locationid` → L3 code. Assert row counts match the fixture.
- [ ] **Step 2: RED.**
- [ ] **Step 3: implement** the reader (bufio scanner or `encoding/csv` with `Comma='|'` and `LazyQuotes`; JSON-parse dynamicproperties).
- [ ] **Step 4: GREEN** + a malformed-input test (short row, bad JSON in dynamicproperties → skipped/errored deterministically).
- [ ] **Step 5: DoD** (`make mutation PKG=./internal/adapters/wcvp`).
- [ ] **Step 6: commit** `feat(wcvp): DwC-A reader (pipe-delim, powoid)`.

## Task 5: dataset.yaml manifest + Ingest use case

**Files:**
- Create: `internal/application/ingest.go`, `internal/application/ingest_test.go`, `internal/adapters/manifest/manifest.go`, `internal/adapters/manifest/dataset.schema.json` (embedded), `internal/adapters/manifest/manifest_test.go`, `dataset.example.yaml`

**Interfaces:**
- `manifest.Parse(path) (*manifest.Dataset, error)` — reads `dataset.yaml`, validates against the embedded JSON Schema (JSON Schema 2020-12, `github.com/google/jsonschema-go`), unknown-field rejection. `Dataset{ Backbones []Backbone{ ID, Version, License, SourceURL, Path } ; TraitVocabularies []… }`.
- `application.Ingest(ctx, ds *manifest.Dataset, readerFor func(Backbone) (RowSource, error), repo output.Repository) (IngestReport, error)` — for each backbone: open the reader, stream rows, map WCVP rows → domain Name/Concept/concept_name/xref(powo=powoid)/distribution, write via `IngestTx`, record `backbone_version` with `manifest_sha`. Two-pass (names/concepts, then accepted-links + synonym-links) so accepted refs resolve.

- [ ] **Step 1: failing tests** — (a) `manifest.Parse` accepts `dataset.example.yaml`, rejects an unknown top-level key and a missing required field; (b) `Ingest` with the WCVP fixture (wire `readerFor`→`wcvp.Read`) into a `:memory:` repo, then assert via T3 reads: the *Corynephorus* concept resolves with its synonyms + powo xref + WGSRPD distribution, and `BackboneVersions` shows `wcvp` with the manifest sha.
- [ ] **Step 2: RED.**
- [ ] **Step 3: implement** manifest (embedded schema, `KnownFields(true)`), then the Ingest use case (two-pass, deterministic ids — derive concept id from backbone+plant_name_id).
- [ ] **Step 4: GREEN.**
- [ ] **Step 5: DoD** (`make mutation PKG=./internal/application` and `PKG=./internal/adapters/manifest`).
- [ ] **Step 6: commit** `feat(ingest): dataset.yaml manifest + WCVP ingest use case`.

## Task 6: Match use case (exact + exact_author)

**Files:** Create `internal/application/match.go`, `internal/application/match_test.go`

**Interfaces:**
- `application.MatchNames(ctx, repo output.Repository, reqs []MatchRequest) ([]MatchResult, error)` where `MatchRequest{ ID, Verbatim string }` and `MatchResult{ ID string; MatchType domain.MatchType; Confidence float64; ConceptID string; Candidates []string; RequiresReview bool; Note string }`.
- Logic: parse verbatim into (canonical, authorship) — reuse a small name-splitter (canonical = words up to the author token; keep simple + tested); `repo.MatchExact(canon)`; run `domain.ClassifyMatch` against each candidate; pick exact_author > exact; aggregate names ending in ` agg.`/`agg.`/`aggr.` → `aggregate_alias` with `note:"Aggregat, keine Kleinartauflösung"`; zero matches → `UNRESOLVABLE` (no ConceptID, RequiresReview per policy). Confidence: 0.99 exact_author, 0.95 aggregate_alias, 0.9 exact-no-author. Fuzzy is out of scope (SP3) — do not fuzzy-match here.

- [ ] **Step 1: failing tests** — batch of the spec §B.2 examples against a seeded repo: `Senecio jacobaea L.`→exact_author→the Jacobaea concept; `Festuca ovina agg.`→aggregate_alias; `Silene otitis` (typo for the seeded `Silene otites`)→ UNRESOLVABLE (no fuzzy this SP); a bare `Corynephorus canescens` (no author)→exact.
- [ ] **Step 2: RED.**
- [ ] **Step 3: implement** the name-splitter + MatchNames.
- [ ] **Step 4: GREEN.**
- [ ] **Step 5: DoD** (`make mutation PKG=./internal/application`).
- [ ] **Step 6: commit** `feat(match): exact + exact_author + aggregate_alias resolution`.

## Task 7: HTTP handlers (concept, xref, match)

**Files:** Create `internal/adapters/http/taxa.go`, `internal/adapters/http/taxa_test.go`; Modify `internal/adapters/http/router.go` (mount routes, extend `Deps` with the use-case handles / repo).

**Interfaces:** `GET /v1/concept/{id}`, `GET /v1/xref?authority=&id=`, `POST /v1/match`. JSON shapes per spec §B (concept with display/canonical/xrefs/classification/synonyms; match results array). Errors via `httperr`: unknown id → 404 `NOT_FOUND`; unresolvable → the match result carries `UNRESOLVABLE`, not an HTTP error (batch endpoint returns 200 with per-item verdicts); missing query params → 400 `INVALID_QUERY`.

- [ ] **Step 1: failing tests** — spin the router with an in-memory repo seeded (via T5 ingest of the fixture, or T3 seed): `GET /v1/concept/{corynephorus-id}`→200 with canonical + synonyms + powo xref; `GET /v1/concept/does-not-exist`→404 NOT_FOUND envelope; `GET /v1/xref?authority=powo&id=396681-1`→200 the concept; `POST /v1/match` with the §B.2 batch→200, per-item match types.
- [ ] **Step 2: RED.**
- [ ] **Step 3: implement** handlers + DTO mapping (keep DTOs in the http adapter, not domain). Wire into `NewRouter` after the middleware chain.
- [ ] **Step 4: GREEN** (+ assert the error envelope shape + Content-Type).
- [ ] **Step 5: DoD** (`make mutation PKG=./internal/adapters/http`).
- [ ] **Step 6: commit** `feat(http): /v1/concept, /v1/xref, /v1/match handlers`.

## Task 8: CLI wiring (ingest, validate) + app composition

**Files:** Modify `cmd/hostus/ingest.go`, `cmd/hostus/validate.go`, `cmd/hostus/serve.go`, `internal/app/app.go`

**Interfaces:** `hostus validate --dataset dataset.yaml` → parse+schema-validate the manifest, print OK or the validation error, no DB write. `hostus ingest --dataset dataset.yaml --db hostus.sqlite` → open SQLite, run `application.Ingest`, print the IngestReport (counts per backbone). `serve` and `app.New` open the configured SQLite (`cfg.SQLite.Path`) read-only and inject the repo into the HTTP `Deps`; `/health/ready` returns 200 only when the DB opens and has ≥1 `backbone_version` row (else 503). `bundle` stays the `errNotImplemented` stub (SP2).

- [ ] **Step 1: failing tests** — `hostus validate --dataset <fixture manifest>` exits 0; with a bad manifest exits non-zero. A cmd-level test that `ingest` into a temp DB then `serve` (on `:0`) answers `/v1/concept/{id}` 200. `/health/ready` is 503 on an empty DB, 200 after ingest.
- [ ] **Step 2: RED.**
- [ ] **Step 3: implement** the cobra wiring + `app.New` repo injection + readiness gate. Add a `dataset.yaml` under `internal/adapters/wcvp/testdata/` (or a test fixture manifest) pointing at the fixture dir.
- [ ] **Step 4: GREEN.**
- [ ] **Step 5: DoD** (`make mutation PKG=./cmd/hostus` and `PKG=./internal/app`).
- [ ] **Step 6: commit** `feat(cli): wire ingest + validate; readiness gated on DB`.

## Task 9: End-to-end integration + OpenAPI + verify

**Files:** Create `internal/app/integration_test.go`; Modify `api/openapi/openapi.yaml`; docs under `docs/reference/http-api.md`; `CHANGELOG.md` (`[Unreleased]`).

- [ ] **Step 1: integration test** — from a clean temp dir: ingest the WCVP fixture via the real CLI/app path → start the server → assert end-to-end: `GET /v1/concept/{corynephorus}` (synonyms + powo xref + WGSRPD), `GET /v1/xref?authority=powo&id=396681-1`, `POST /v1/match` (the §B.2 batch), `/health/ready`=200, `/metrics` exposes request counters. Build-tagged `integration` (run via `make test-integration`).
- [ ] **Step 2: run it green.**
- [ ] **Step 3:** regenerate/extend `api/openapi/openapi.yaml` for the three new endpoints (keep it accurate — no aspirational fields); update `docs/reference/http-api.md` (German). Ensure `openapi-diff` + `mkdocs --strict` pass.
- [ ] **Step 4:** run the `verify` skill (drive the ingest+serve flow) to confirm real behavior end-to-end.
- [ ] **Step 5: DoD** — full `make verify` + `make security-check` green; `make test-integration` green.
- [ ] **Step 6: commit** `test(sp1): e2e ingest→serve integration + OpenAPI for /v1 core`.

---

## Self-Review Notes
- Spec coverage: schema §4.3 → T2; WCVP ingest (D.3 step 1) → T4/T5; `/match` exact/exact_author §B.2 → T6; `/concept` + `/xref` → T3/T7; artifact contract §4.4 (dataset.yaml + embedded schema, manifest_sha) → T5; CLI §4.8 ingest/validate → T8; bundle deferred to SP2 (stub) — consistent with build order. Fuzzy match, traits, suggest, translate, synonyms all correctly out of SP1.
- PoC findings baked in: WCVP DwC-A pipe format + `scientfiicname` typo + powoid (T4 constraints), modernc FTS5 (T2), NOT_FOUND/UNRESOLVABLE (T3/T6/T7).
- Types consistent: `output.Repository`/`MatchCandidate`/`IngestTx` (T2) consumed by T3/T5/T6; `domain.ClassifyMatch` (T1) consumed by T6; `wcvp.Read` (T4) consumed by T5.
- No placeholders: each task has real interfaces + concrete test cases; implementers fill bodies guided by the fixture + `poc/P02-findings.md` for exact column semantics.

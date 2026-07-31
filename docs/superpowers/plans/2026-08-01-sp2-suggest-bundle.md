# hostus 2.0 — SP2 Suggest + Offline-Bundle Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax.

**Goal:** Ship `GET /v1/suggest` (FTS5 prefix, area-ranked) and the offline SQLite bundle export (`hostus bundle`), and fold in the SP1-review tracked hardening (WAL, match-ambiguity flag).

**Architecture:** Builds on SP1. Suggest queries the existing `fts_name` FTS5 table (populated at ingest in SP1) with a prefix match, joins to `taxon_concept` + `distribution` for `in_area`, and ranks per spec §B.1. The offline bundle is a filtered copy of the same schema (concepts whose distribution intersects the requested area, plus their names/xrefs/distribution + FTS rows + a `snapshot_version`), producing a standalone SQLite file the field app queries with no network.

**Tech Stack:** Go 1.26.5, `modernc.org/sqlite` (FTS5), the SP1 hexagonal layers.

## Global Constraints

- Builds on SP1 (branch forked from `feature/sp1-foundation`): domain/ports/sqlite/wcvp/application/http/cli all present; `fts_name` (`unicode61 remove_diacritics 2`) + `fts_name_map` populated at ingest; `name.canonical_fold` = `domain.Canonicalize(canonical)`; ids = `<backbone>:concept|name:<taxonid>`; accepted ⇔ `wcvp.TaxonRow.IsAccepted()` self-reference; `distribution(area_scheme='wgsrpd_l3', area_code=<L3>)` populated from WCVP `locationid`.
- SQLite driver `modernc.org/sqlite`, CGO-free. Error envelope `{"error":{"code","message"}}` with `INVALID_QUERY`/`NOT_FOUND`.
- **Suggest ranking (spec §B.1, in priority order):** 1) prefix hit on genus or species; 2) `in_area == true`; 3) `status == accepted` before synonym; 4) rank (species before subspecies before variety); 5) string score (bm25) as tiebreaker only.
- **`in_area`:** area filter uses WGSRPD-L3 codes from `distribution`. Query param `area` accepts a WGSRPD-L3 code (e.g. `GER`) or a country/region code the service maps to L3 (for SP2 support WGSRPD-L3 directly + a small documented alias for `DE`→German L3 codes; Euro+Med/Bayern area schemes are later — note it). `in_area` is computed, not stored per-name.
- **depguard:** unchanged from SP1 (domain pure; application→domain+ports; adapters no cross-adapter; http/mcp carve-outs; app/cmd composition root).
- Per-task DoD (SP0/SP1): unit tests pass → `make mutation PKG=<pkg>` ZERO survivors → `make lint` clean incl `_test.go` → `make verify` green. Tooling via `nix develop -c`. Tests use real temp SQLite + the WCVP fixture, not mocks. (If `make mutation` panics on a stray `./.go/`, `chmod -R u+w ./.go && rm -rf ./.go` first.)
- release-please owns VERSION/CHANGELOG; accumulate under `## [Unreleased]`.

---

## Task 1: Suggest domain types + ranking (pure)

**Files:** Create `internal/domain/suggest.go`, `internal/domain/suggest_test.go`

**Interfaces (Produces):**
- `type SuggestItem struct { ConceptID, Canonical, Display, VernacularDE string; Rank Rank; Status Status; InArea bool; PrefixHit bool; Score float64 }` (Score = bm25, lower = better as SQLite returns it; document the sign convention).
- `RankSuggestions(items []SuggestItem) []SuggestItem` — stable-sorts per the §B.1 priority (prefix hit, then in_area, then accepted-before-synonym, then rank order species<subspecies<variety<form, then Score as tiebreaker). Pure.
- `RankOrder(r Rank) int` — the ordinal used in step 4 (FAMILY/GENUS/SPECIES/SUBSPECIES/VARIETY/FORM).

- [ ] **Step 1: failing tests** — table cases proving each priority level dominates the ones below it: an in-area synonym outranks an out-of-area accepted only if... (no — accepted-vs-synonym is step 3, in_area is step 2, so in_area wins over accepted): construct pairs isolating each rule (prefix>in_area>accepted>rank>score). Assert stable order for equal keys.

```go
func TestRankSuggestions_InAreaBeatsAccepted(t *testing.T) {
	items := []domain.SuggestItem{
		{ConceptID: "a", InArea: false, Status: domain.StatusAccepted, PrefixHit: true, Score: 0.1},
		{ConceptID: "b", InArea: true,  Status: domain.StatusSynonym,  PrefixHit: true, Score: 0.9},
	}
	got := domain.RankSuggestions(items)
	if got[0].ConceptID != "b" { t.Fatalf("in_area must outrank accepted: %v", got) }
}
```

- [ ] **Step 2: RED** (`go test ./internal/domain/`).
- [ ] **Step 3: implement** `SuggestItem`, `RankOrder`, `RankSuggestions` (use `sort.SliceStable` with a multi-key comparator). Pure, no internal imports.
- [ ] **Step 4: GREEN.**
- [ ] **Step 5: DoD** — `make mutation PKG=./internal/domain` (0 survivors), lint, verify.
- [ ] **Step 6: commit** `feat(domain): suggest item + area-aware ranking`.

## Task 2: Repository.Suggest (FTS5 prefix + in_area)

**Files:** Modify `internal/ports/output/repository.go` (add `Suggest`); Create `internal/adapters/sqlite/suggest.go`, `internal/adapters/sqlite/suggest_test.go`

**Interfaces:**
- Add to `output.Repository`: `Suggest(ctx, q string, opts SuggestOpts) ([]domain.SuggestItem, error)` where `SuggestOpts{ Area string; Ranks []domain.Rank; Limit int }`.
- `sqlite` impl: FTS5 prefix query `SELECT ... , bm25(fts_name) FROM fts_name JOIN fts_name_map ON ... JOIN taxon_concept ... WHERE fts_name MATCH ?` with the query turned into a prefix token (`domain.Canonicalize(q) || '*'`, escaped); join `name`/`concept_name`/`taxon_concept` to get canonical/rank/status; compute `InArea` via `EXISTS(SELECT 1 FROM distribution WHERE concept_id=... AND area_scheme='wgsrpd_l3' AND area_code IN (<area→L3 set>))`; `PrefixHit` from the FTS match; apply `Ranks` filter + `Limit` (fetch a few extra before ranking so ranking isn't truncated by SQL LIMIT — document the fetch-then-rank budget). Returns unranked `SuggestItem`s (the app layer calls `domain.RankSuggestions`).

- [ ] **Step 1: failing tests** — seed the WCVP fixture (or T3 seed) incl. distribution; `Suggest("coryn", {Area:"GER", Limit:10})` returns the *Corynephorus* concept(s) with `InArea` set correctly for a German L3 code present/absent; a prefix that matches a synonym returns it with its accepted concept; `Ranks:[SPECIES]` filters out non-species; empty `q`/too-short → return empty or all per policy (define + test).
- [ ] **Step 2: RED.**
- [ ] **Step 3: implement** the FTS5 prefix query + in_area EXISTS + the area→L3 alias (WGSRPD-L3 passthrough + a small `DE` alias map, documented). Parameterized SQL; escape the FTS query token.
- [ ] **Step 4: GREEN** (+ a test that a MATCH special char in `q` doesn't break the query — sanitize).
- [ ] **Step 5: DoD** — `make mutation PKG=./internal/adapters/sqlite` (0 survivors), lint, verify.
- [ ] **Step 6: commit** `feat(sqlite): FTS5 prefix suggest with in_area`.

## Task 3: Suggest use case

**Files:** Create `internal/application/suggest.go`, `internal/application/suggest_test.go`

**Interfaces:** `application.Suggest(ctx, repo output.Repository, req SuggestRequest) (SuggestResponse, error)` — `SuggestRequest{ Q, Area string; Ranks []domain.Rank; Limit int }`; calls `repo.Suggest` (fetch budget ≥ Limit), `domain.RankSuggestions`, truncates to `Limit`, builds `SuggestResponse{ BackboneVersions map[string]string; Results []SuggestItem }`. Validate: empty `q` → error (handler maps to INVALID_QUERY); `Limit<=0` → default (e.g. 10), cap at a max (e.g. 50).

- [ ] **Step 1: failing tests** — against a seeded repo: `Suggest({Q:"coryn", Area:"GER", Limit:5})` returns results ranked (in_area first, accepted before synonym); limit honored; empty q → error; default/cap limit applied; `BackboneVersions` populated from the repo.
- [ ] **Step 2–4: RED→implement→GREEN.**
- [ ] **Step 5: DoD** — `make mutation PKG=./internal/application` (0 survivors), lint, verify.
- [ ] **Step 6: commit** `feat(suggest): area-ranked suggest use case`.

## Task 4: HTTP GET /v1/suggest

**Files:** Create `internal/adapters/http/suggest.go`, `internal/adapters/http/suggest_test.go`; Modify `internal/adapters/http/router.go` (mount route); `api/openapi/openapi.yaml`; `docs/reference/http-api.md`

**Interfaces:** `GET /v1/suggest?q=&area=&rank=species,subspecies&limit=10` → 200 `{backbone_versions{}, results:[{concept_id, display, canonical, vernacular_de?, rank, status, in_area, score}...]}` per spec §B.1. Missing/empty `q` → 400 `INVALID_QUERY`. `rank` = comma list parsed to `[]domain.Rank` (unknown rank → 400). `limit` parsed (bad → 400 or default; pick + test).

- [ ] **Step 1: failing tests** — seeded router: `GET /v1/suggest?q=coryn&area=GER` → 200, results include Corynephorus with `in_area:true`; `q` missing → 400 INVALID_QUERY envelope; `rank=species` filters; `limit=1` truncates; malformed `limit`/`rank` → 400.
- [ ] **Step 2–4: RED→implement→GREEN** (DTO in the http adapter; wire via the injected repo + `application.Suggest`).
- [ ] **Step 5:** update OpenAPI (Suggest path + schema, accurate) + German docs; `mkdocs --strict` + `openapi-diff` still pass.
- [ ] **Step 6: DoD** — `make mutation PKG=./internal/adapters/http` (0 survivors), lint, verify.
- [ ] **Step 7: commit** `feat(http): GET /v1/suggest`.

## Task 5: Offline bundle export + `hostus bundle`

**Files:** Create `internal/adapters/sqlite/bundle.go`, `internal/adapters/sqlite/bundle_test.go`; Modify `cmd/hostus/bundle.go` (wire the stub), `internal/app` (bundle wiring)

**Interfaces:**
- `sqlite.ExportBundle(ctx, srcDB *DB, out string, opts BundleOpts) (BundleReport, error)` — `BundleOpts{ Area string; SnapshotVersion string }`. Produces a NEW standalone SQLite file at `out` with the SAME schema (via the embedded schema) containing only concepts whose distribution intersects `Area` (or all if Area empty), plus their names/concept_name/xref/distribution/vernacular + the `fts_name`/`fts_name_map` rows, the relevant `backbone_version` rows, and a `bundle_meta(snapshot_version, area, created_at, source_manifest_sha)` table (add `bundle_meta` to schema.sql or create it in the bundle only). The bundle must be independently queryable by the same `sqlite.Open` + `Suggest`/`Concept` reads (prove it).
- `hostus bundle --db hostus.sqlite --area DE-BY --out bundle.sqlite [--snapshot v1]` → runs ExportBundle, prints the BundleReport (concept/name counts, size). Remove the `errNotImplemented` stub.

- [ ] **Step 1: failing tests** — ingest fixture into a temp DB → `ExportBundle(area:"GER", out)` → open the bundle with `sqlite.Open` → `Suggest("coryn", {Area:"GER"})` against the BUNDLE returns Corynephorus (proving the bundle is a working standalone index); `Concept(id)` works on the bundle; `bundle_meta` has the snapshot_version; concepts NOT in the area are absent; the FTS rows were copied (suggest works, not just direct reads).
- [ ] **Step 2–4: RED→implement→GREEN.**
- [ ] **Step 5:** wire `cmd/hostus/bundle.go`; a cmd-level test: `hostus bundle` on a temp DB writes a non-empty bundle file.
- [ ] **Step 6: DoD** — `make mutation PKG=./internal/adapters/sqlite` and `PKG=./cmd/hostus` (0 survivors), lint, verify.
- [ ] **Step 7: commit** `feat(bundle): offline SQLite/FTS5 bundle export + hostus bundle`.

## Task 6: SP1-debt hardening (WAL + match ambiguity)

**Files:** Modify `internal/adapters/sqlite/db.go` (WAL), `internal/application/match.go` (+ test)

**Interfaces:**
- `sqlite.Open`: set `PRAGMA journal_mode=WAL` + `PRAGMA busy_timeout=5000` (via DSN `?_pragma=...` or an exec after open). Keep `SetMaxOpenConns(1)` for writers; document that WAL now allows a concurrent reader (serve) alongside an ingest writer. Add a test proving WAL is active and a second `Open` (reader) can read while a write tx is open (bounded — don't over-engineer; a focused concurrency test).
- `application.MatchNames`: when ≥2 candidates tie at the SAME winning strength (e.g. two exact_author hits on different concepts), set `RequiresReview=true` and populate `Candidates` with the tied names instead of silently first-matching (the SP1-review deferral).

- [ ] **Step 1: failing tests** — WAL: `Open` then `PRAGMA journal_mode` == `wal`; a concurrency test (writer tx open, reader Open reads a committed row). Match ambiguity: seed two concepts with the same canonical+author (a real homonym-ish case), `MatchNames` on that name → `RequiresReview=true`, `Candidates` lists both, no arbitrary ConceptID.
- [ ] **Step 2–4: RED→implement→GREEN.**
- [ ] **Step 5: DoD** — `make mutation PKG=./internal/adapters/sqlite` and `PKG=./internal/application` (0 survivors), lint, verify. Confirm SP1's existing tests + the FK/`:memory:` behavior still hold with WAL.
- [ ] **Step 6: commit** `fix(sp2): WAL + busy_timeout; match ambiguity flags RequiresReview`.

## Task 7: End-to-end integration + docs + verify

**Files:** Modify `internal/app/integration_test.go` (extend); `api/openapi/openapi.yaml`; `docs/reference/http-api.md`; `CHANGELOG.md`

- [ ] **Step 1: integration test** (`//go:build integration`) — extend the SP1 e2e: after ingest+serve, `GET /v1/suggest?q=coryn&area=GER` → 200 area-ranked results; then `hostus bundle --area GER` → open the bundle → `Suggest` against it OFFLINE returns Corynephorus; `/metrics` counts the suggest requests. Runnable via `make test-integration`.
- [ ] **Step 2:** run it green.
- [ ] **Step 3:** OpenAPI + German docs for `/v1/suggest` + the bundle CLI; CHANGELOG `[Unreleased]` SP2 summary; `mkdocs --strict` + `openapi-diff` pass.
- [ ] **Step 4:** real verify — build, ingest, serve, curl `/v1/suggest`, export a bundle, query it offline; paste outputs into the report.
- [ ] **Step 5: DoD** — `make verify` + `make security-check` + `make test-integration` green.
- [ ] **Step 6: commit** `test(sp2): e2e suggest + offline bundle integration + OpenAPI`.

---

## Self-Review Notes
- Spec coverage: `/v1/suggest` §B.1 (ranking incl. in_area) → T1–T4; offline bundle (§4.5, D.3 step 4) → T5; SP1-review carried debt (WAL, match ambiguity) → T6; e2e + docs → T7. Fold-in of classification/homotypic + Euro+Med/Bayern area schemes explicitly deferred (SP3+); PlantNet path is client-side (out of hostus).
- PoC alignment: FTS5 prefix + bm25 (P1 🟢); WGSRPD-L3 in_area from WCVP `locationid` (P2); ASK/FIN-Web/Bayernstatus area quality is license-blocked (R1) — `area` for Bavaria is best-effort via WGSRPD-L3, noted.
- Types consistent: `domain.SuggestItem`/`RankSuggestions` (T1) consumed by T2/T3; `output.Repository.Suggest` (T2) by T3; `application.Suggest` (T3) by T4; bundle reads reuse SP1's `Open`/`Suggest`/`Concept`.

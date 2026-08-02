# hostus 2.0 — SP5 `sec.` + concept_relation + `/translate` Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax.

**Goal:** Make the `sec.` reference space a first-class citizen of the index, import typed concept relations between those spaces from the CDM `rl_standardliste` dataset, and serve `POST /v1/translate` — "this concept *sec.* Rothmaler, what is it *sec.* Wisskirchen & Haeupler 1998, and how exactly do the two relate?"

**Why this is different from SP1–SP4.** Every prior milestone treated a taxon concept as a single global truth keyed by name. UC6 exists because that is false: the same name denotes *different circumscriptions* in different floras, and the difference is the scientifically interesting part. SP5 is the first milestone where two rows for the same name are deliberately **not** merged.

**Architecture:** the CDM dataset becomes a **second backbone** (`cdm-rl-standardliste`), ingested with `taxon_concept.sec_reference` populated per concept — the column has existed since SP1 and has been NULL until now. Typed relations between those concepts go into the existing `concept_relation` table. `POST /v1/translate` walks that graph. The bridge from our WCVP concepts into the CDM space reuses the SP3 name-crosswalk machinery, not a new mechanism.

**Tech Stack:** the SP0–SP4 stack. One new pipeline (`pipelines/cdm/build.sh`) against the CDM REST API — no new Go dependency.

---

## Global Constraints

- Builds on `feature/sp4-xref` (SP0–SP4 + reality-check + hardening). Branch: `feature/sp5-sec-translate`.
- **Run everything FOREGROUND with bounded timeouts. No monitors, no background-and-poll** — eight agents have stalled that way in this project; several had to be rescued. The one exception is the crawl itself, which is a `build.sh` run to completion, resumable, and never a Go test dependency.
- **Never read from the repository while an ingest transaction is open** (`SetMaxOpenConns(1)` → real deadlock; cost SP3 an escalation). Strictly two-phase: resolve everything, then open the tx.
- Hexagonal + depguard: `internal/application` imports `internal/domain` and `internal/ports` only. Composition root is `internal/app`. The allowed-libraries list in `CLAUDE.md` is exhaustive.
- Per-task DoD for code tasks: `go test -timeout 180s ./...` → `make mutation PKG=<each touched pkg>` **zero new unjustified survivors** → `make lint` clean **including `_test.go`** → `make verify` → `make security-check` → `mkdocs --strict`. Use `nix develop -c` for everything.
- **LSP diagnostics in this repo are frequently STALE** (GOPROXY=off outside nix). Trust only `nix develop -c go build ./...` and `go vet ./...`.
- If `make mutation` fails with a permission/copy error: `chmod -R u+w ./.go && rm -rf ./.go`, then retry. Gremlins v0.5.1 works fine here.
- Measurement steps are not TDD, but: **a number you did not measure is not a number you may report.** Show the command.
- Docs German; code comments sparse and English. Never commit bulk data. Never read, print, or modify `.envrc.local`.

### Crawl etiquette (owner decision, 2026-08-02) — binding

- **Identify honestly.** Every request carries
  `User-Agent: hostus/2.0 (+https://github.com/jobrunner/hostus; jo.brunner@mayflower.de) taxonomic-concept-research`.
  **Verified working 2026-08-02:** `/classification`, `/taxon`, and
  `/portal/taxon/{uuid}/taxonRelationships` all return 200 with this UA. P8's
  403 came from the *Drupal portal's* WAF, not from `api.cybertaxonomy.org`.
- **Never substitute a browser User-Agent to get past a block.** If the API
  starts refusing the honest UA, **stop the crawl** and report it — do not
  work around it. The licence conversation with BGBM/EDIT is pending anyway.
- Rate-limit to **≤1 request/second**, single-threaded, with backoff on 429/5xx.
- The crawl must be **resumable** (an on-disk cache keyed by uuid, like
  `pipelines/wikidata`'s `.cache/`), so an interrupted run never re-fetches
  what it already has and a retry costs nothing to the server.
- `api.cybertaxonomy.org/robots.txt` returns 403 (no file served) — no explicit
  permission and no explicit prohibition. Record that as-is; do not read it as
  either.

### Licensing — this source is the strictest in the project

P8 found **no licence anywhere**: not on the portal, not on the API, not in any
JSON payload. The data is derived from copyrighted flora literature (Wisskirchen
& Haeupler 1998, Hegi, Rothmaler, Flora Europaea, Med-Checklist …). The owner's
standing frame applies — *local evaluation is in scope, redistribution is
gated* — so:

- `redistribution: unknown` for `cdm-rl-standardliste`, machine-enforced by the
  existing bundle gate. It must be **impossible** to ship this data in a bundle
  without `--force-include-restricted`.
- `/v1/translate` is served from the **local** index; the endpoint documentation
  must state plainly that the underlying relation data is not redistributable
  without written clearance from BGBM/EDIT.
- Since SP4 the gate covers backbones, trait vocabularies and xref sources.
  **Verify the gate actually catches this new backbone** — do not assume.

---

## Task 1: Measure whether the two-hop method survives at scale

**This task decides the shape of every task after it. Nothing downstream may be built until its numbers exist.**

**Files:** `poc/p08b_cdm_sample/probe.sh` (+ a Python helper), `docs/research/cdm-sample.md`

P8 proved the method on **one genus** (*Abies*): fetch a concept's
`/portal/taxon/{uuid}/taxonRelationships` to get typed relations with a
relationship uuid, then find the partner concept by looking at other concepts
sharing the same accepted name and matching relationship uuids. It worked
cleanly for 3 siblings. Whether it survives 51.466 concepts — homonyms,
concepts with many relations, names shared across families — is **unmeasured**,
and the reality-check milestone exists precisely because this project has been
burned by generalising from a 20-row fixture.

Take a **stratified sample of 300–500 concepts** — not one genus. Draw across
several families and deliberately include hard cases: names occurring in many
classifications, concepts with >1 relationship, and at least one known homonym.
Document how the sample was drawn (it must be reproducible: a fixed seed or an
explicit uuid list committed to the repo).

**Measure and report, each with the command that produced it:**
- **Relation density:** what share of sampled concepts have ≥1 relationship at all? (If most have none, the whole graph is thin and `/translate` is nearly empty — that is a finding that would rescope SP5.)
- **Two-hop resolution rate:** of all relationships found, what share resolve to *exactly one* partner concept? What share are ambiguous (uuid matches several candidates), and what share dangle (no partner found)?
- **Why the failures fail** — categorise ~15 unresolved cases by hand.
- **Relation-type distribution:** `representation_L10n` / `symbol` values and their counts, split by the `conceptRelationship: true|false` flag (true = genuine Berendsohn concept relation, false = misapplied name). Note which types actually occur — the schema's `congruent|includes|included_in|overlaps|disjoint` vocabulary is an *assumption* until measured, exactly like `domain.ParseRank`'s 6-vs-34 ranks were.
- **`sec.` resolvability:** does every sampled concept carry a structured `secSource`, and do those resolve to the 18 classifications?
- **Crawl cost:** measured mean/p95 request latency → a projected wall-clock for the full 51.466-concept crawl at 1 req/s. State it as a range.

- [ ] **Step 1:** write the probe; draw and commit the reproducible sample list.
- [ ] **Step 2:** run it against the real API with the honest UA and the 1 req/s limit. If the honest UA is refused, STOP and report.
- [ ] **Step 3:** write `docs/research/cdm-sample.md` (German) with every number above.
- [ ] **Step 4: a written go/no-go** with a recommendation for Task 2's scope: full crawl, a subset of classifications, or rescope. **A low resolution rate is a finding, not a failure** — say so plainly and say what it costs UC6.
- [ ] **Step 5: commit** `docs(research): CDM two-hop method measured on a stratified sample`.

## Task 2: CDM crawl pipeline → canonical artifacts

**Files:** `pipelines/cdm/{build.sh,crawl.py,convert.py,README}`, `pipelines/README.md`

Scope comes from Task 1's verdict — follow it; do not silently widen it.

Follow the established pipeline pattern exactly (`pipelines/wikidata` is the
closest model): pinned source → resumable on-disk cache → canonical
pipe-delimited CSV → printed summary. Emit **two** artifacts:

1. `cdm-concepts-canonical.csv`:
   `concept_uuid|scientific_name|authorship|rank|status|sec_uuid|sec_title|classification_uuid|parent_uuid`
2. `cdm-relations-canonical.csv`:
   `from_uuid|to_uuid|relation_type|relation_symbol|is_concept_relation|relationship_uuid`

Both carry the raw CDM vocabulary in `relation_type`/`rank` — **do not map to
hostus's vocabulary in the pipeline.** Mapping is a domain decision and belongs
in Task 3, where it is testable and where an unknown value can fail loudly. This
is the lesson from `ParseRank`.

The summary must print: concepts fetched, relations found, resolved / ambiguous
/ dangling counts, and the per-type distribution — the same numbers Task 1
measured on the sample, now at full scope, so the two can be compared.

- [ ] **Step 1:** write `build.sh` + crawler with the honest UA, ≤1 req/s, resumable cache, backoff.
- [ ] **Step 2:** run it at the scope Task 1 endorsed. Report the real wall-clock against Task 1's projection.
- [ ] **Step 3:** verify the P8 reference concept appears with its expected relations (*Abies alba* sec. Wisskirchen & Haeupler 1998, uuid `872088a4-95f4-472c-ae79-a29028bb3fbf`).
- [ ] **Step 4:** document both CSV contracts in `pipelines/README.md`, including the licence status (`redistribution: unknown`) and the crawl etiquette actually used.
- [ ] **Step 5: commit** `feat(pipelines): CDM concept + relation harvest`.

## Task 3: `sec.` as a first-class concept dimension (TDD)

**Files:** `internal/domain/{sec.go,relation.go}` (+ tests), `internal/adapters/cdm/reader.go` (+ tests + fixture), `internal/application/cdm_ingest.go` (+ tests), `internal/adapters/sqlite/{schema.sql,db.go,read.go}` (+ tests), manifest support, `internal/app` wiring, `cmd/hostus/ingest.go`

**Interfaces:**
- `domain.SecReference{ ID, Title string }` — the bibliographic identity of a reference space.
- `domain.Relation` — a typed enum with `ParseRelation(string) (Relation, error)` mapping the **measured** CDM vocabulary from Task 1/2 onto `congruent|includes|included_in|overlaps|disjoint`. An unmapped value must **fail loudly**, never be silently coerced. Misapplied-name relations (`conceptRelationship: false`) are **not** concept relations — decide explicitly whether to store them separately or drop them, and document which.
- `cdm.ReadConcepts(path)` / `cdm.ReadRelations(path)` mirroring the traits and xref readers: pipe CSV, collected errors with line numbers, never panics.
- `application.IngestCDM(ctx, repo, concepts, relations, meta) (CDMIngestReport, error)` — **strictly two-phase**, application-level DTO bridge (depguard).
- `dataset.yaml` gains a `cdm_source:` (or an entry in `backbones:`) with `redistribution: unknown`, schema-validated like every other source.

**Design decisions this task must make explicitly and document:**
- CDM concepts are ingested as a **second backbone**, keyed `cdm:concept:<uuid>`, with `sec_reference` populated. They are deliberately *separate rows* from the WCVP concepts for the same name — that separation is the feature.
- `concept_relation` FKs both ends to `taxon_concept`, so a relation may only be written once **both** concepts exist. Two-phase, and unresolvable ends are counted and sampled, never silently dropped.
- Directionality: `includes` / `included_in` are inverses. Decide whether both directions are stored or one canonical direction plus inversion at query time, and test the choice.

- [ ] **Step 1: failing tests** — a concept with a `sec.` round-trips through ingest and is readable; two concepts sharing a name but differing in `sec.` stay distinct rows; a relation between them is written with the mapped type; an unknown CDM relation type fails loudly with the offending value in the error; a relation whose partner is missing is counted + sampled, writes nothing, and does not abort the ingest; misapplied-name rows follow the documented rule; the `redistribution: unknown` gate refuses a bundle containing CDM data by default and records it under `--force-include-restricted`.
- [ ] **Step 2: RED → implement → GREEN.**
- [ ] **Step 3:** ingest the real Task 2 artifacts. Report: concepts, relations written, unresolved, per-type counts, and how many distinct `sec.` spaces landed.
- [ ] **Step 4: DoD** on every touched package.
- [ ] **Step 5: commit** `feat(sec): CDM concepts and typed concept relations`.

## Task 4: `POST /v1/translate` (TDD)

**Files:** `internal/application/translate.go` (+ tests), `internal/ports/input`, `internal/adapters/http/translate.go` (+ tests), `internal/adapters/sqlite/read.go`, `api/openapi/openapi.yaml`, `docs/reference/http-api.md`, `docs/how-to/` (German UC6 how-to)

**Request:** a source concept — by hostus concept id, or by verbatim name (reusing `/v1/match`'s resolution, with the same `requires_review` discipline for fuzzy hits) — plus a target `sec.` space.

**Response:** the candidate concepts in the target space, each with the **typed relation** and the path it came from. `sec_inference` is a derived response structure, **not** a persisted table (architecture spec §4.3) — build it in the application layer.

Non-negotiables, because this endpoint is the one that can quietly lie:
- **A translation is never presented as an equality unless the relation is `congruent`.** `includes` / `overlaps` must be visible in the response as what they are; a consumer must not be able to read an `overlaps` result as "the same taxon".
- If no relation exists, return an explicit empty/`UNRESOLVABLE` answer — **never** fall back to a name match and present it as a concept translation. A name match between two `sec.` spaces is exactly the conflation UC6 exists to prevent. If a name-based fallback is offered at all, it must be a separately labelled field with `requires_review: true`.
- Multi-hop: decide explicitly whether translation follows chains (A congruent B, B includes C). Default to **one hop** and say so; a transitive chain across relation types is not sound in general (congruent∘includes is defensible, overlaps∘overlaps is not).
- Unknown concept id → `NOT_FOUND`. Unresolvable name → `UNRESOLVABLE`. Both codes already exist.

- [ ] **Step 1: failing tests** — congruent translation returns the partner with type `congruent`; an `includes` relation is labelled as such and not flattened to equality; a concept with no relation into the target space returns the explicit empty answer, not a name guess; unknown id → `NOT_FOUND`; unresolvable name → `UNRESOLVABLE`; a fuzzy name entry carries `requires_review: true`; the one-hop boundary is enforced.
- [ ] **Step 2: RED → implement → GREEN.**
- [ ] **Step 3:** OpenAPI (generated, never hand-maintained) + `docs/reference/http-api.md` + a German UC6 how-to that states the licence caveat and the one-hop boundary.
- [ ] **Step 4: DoD.**
- [ ] **Step 5: commit** `feat(http): POST /v1/translate between sec. reference spaces`.

## Task 5: End-to-end + measured verdict

**Files:** `internal/app/integration_test.go`, `docs/research/reality-check.md`, `CHANGELOG.md`

- [ ] **Step 1:** extend the `integration`-tagged e2e: ingest WCVP + the CDM fixture → serve → `POST /v1/translate` from a WCVP concept into a CDM `sec.` space returns the expected typed relation; assert **specific** ids and the relation type, not merely a 200.
- [ ] **Step 2:** `make test-integration` green.
- [ ] **Step 3:** the SP5 verdict in `docs/research/reality-check.md` (German): coverage of the relation graph, how many of our WCVP concepts can actually be translated into at least one `sec.` space, and **what that means for UC6**. Verdict in the established form — **hält / hält mit Auflagen / hält nicht**. Restate the licence position plainly.
- [ ] **Step 4:** CHANGELOG `[Unreleased]` (do **not** touch `VERSION` — release-please owns it); `mkdocs --strict`; `make verify` + `make security-check` + `make test-integration` green.
- [ ] **Step 5: commit** `test(sp5): e2e concept translation + coverage verdict`.

---

## Self-Review Notes

- **Task 1 is a real gate, not a warm-up.** P8 validated the two-hop join on a single genus; SP4 taught that a method's coverage is the unknown, not its correctness. If the resolution rate is poor, Tasks 2–5 change shape — which is why the go/no-go is a written deliverable.
- **The relation vocabulary is measured, never assumed.** `congruent|includes|included_in|overlaps|disjoint` is what the SP1 schema guessed. `ParseRank` made the same kind of guess (6 ranks vs WCVP's 34) and it aborted the full ingest after 5,4 s. `ParseRelation` must fail loudly on the first unmapped value.
- **The `sec.` separation is the whole point.** Every earlier milestone merged rows for the same name; SP5 must not. The e2e should make that visible by asserting two distinct concepts for one name.
- **`/translate` is the easiest endpoint in this project to make quietly wrong** — returning a plausible partner where the honest answer is "no relation recorded". Hence the hard rule against a silent name-match fallback.
- Licensing is stricter here than anywhere else in hostus, and the SP4 review found the redistribution gate had a hole nobody noticed because each task was locally correct. Task 3 therefore *verifies* the gate against this backbone rather than trusting that it generalises.

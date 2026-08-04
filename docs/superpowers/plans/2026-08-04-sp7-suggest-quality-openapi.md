# hostus 2.0 — SP7 Suggest-Qualität + generiertes OpenAPI Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax.

**Goal:** Make `GET /v1/suggest` return the *right* candidates fast — area-scoped by default, hierarchically diverse rather than flooded by one genus's species — and stop hand-maintaining the OpenAPI spec.

**Why now.** The reality-check measured the suggest hot spot precisely: short 2-character prefixes hit 20–46k concepts, p50 up to **373 ms**, and named the lever without pulling it. Separately, `CLAUDE.md` requires the OpenAPI spec to be code-generated; it is hand-written for **seven** endpoints and `make doc-drift` skips the check, so the published contract can drift from the handlers unnoticed.

**Owner decisions (2026-08-04), binding:**
1. **Area is the primary restriction**, not a blind cap: a suggest scoped to a country/area list is the normal case. *"Alles andere ist Quatsch."*
2. **A cap of 2000 is an acceptable fallback** for the case where the user is looking up a name and has no distribution in mind (no area given).
3. **Results must be hierarchical, not flat.** For prefix `Ac`, the answer must surface *Acer* (the genus) alongside species — not bury 142 matching genera under thousands of species.

**Measured starting position (real index, 440.534 concepts / 1.983.859 distribution rows / 381 areas):**

| Fact | Value | Consequence |
|---|---|---|
| Accepted-concept ranks | SPECIES 368.928 · SUBSPECIES 27.942 · VARIETY 25.727 · GENUS 16.868 · FORM 633 · notho* 428 | Genus↔species diversity is buildable today |
| **FAMILY concepts** | **0** | **Families do not exist in this index.** `Aceraceae` cannot be suggested. Build the mechanism rank-agnostically so families slot in later |
| `parent_id` populated | 423.631 / 440.534 (96,2 %) | The hierarchy is walkable |
| Prefix `ac%` | 5.558 concepts, of which **142 genera**, 270 *Acer* species | The flooding is real and measurable |
| Distribution | 1.983.859 rows over 381 WGSRPD-L3 areas | Area scoping has real data behind it |

Families would later come from the already-crawled CDM dataset (629 families, German flora only, `redistribution: unknown`) or an additional backbone. **Out of scope here** — but nothing in this milestone may hard-code "genus is the top rank".

**Tech Stack:** the SP0–SP6 stack. For Task 4, one new code-generation dependency is expected — it must be dev/test tooling, not a runtime import (see Global Constraints).

---

## Global Constraints

- Branch `feature/sp7-suggest-openapi`, forked from `master` (`71d6604`, the fully merged stack).
- **Run everything FOREGROUND with bounded timeouts. No monitors, no background-and-poll** — eight agents have stalled that way in this project.
- **Never read from the repository while an ingest transaction is open** (`SetMaxOpenConns(1)` → real deadlock; cost SP3 an escalation, re-verified in SP4 and SP5).
- Hexagonal + depguard: `internal/application` imports `internal/domain` and `internal/ports` only; composition root is `internal/app`.
- The runtime allowed-libraries list in `CLAUDE.md` is exhaustive. A code generator is **dev tooling**: it belongs in `flake.nix` and may not appear in the binary's import graph. `make arch` must stay green.
- Per-task DoD, all FOREGROUND: `go test -timeout 180s ./...` → `make mutation PKG=<each touched pkg>` (`Not covered` must be 0 — the gate enforces it) → `make lint` clean **including `_test.go`**, both build-tag passes → `make verify` → `make test-integration` → `make security-check` → `make licenses` → `mkdocs --strict`.
- **LSP diagnostics in this repo are frequently STALE.** Trust only `nix develop -c go build ./...` and `go vet ./...`. If `make mutation` hits a permission error: `chmod -R u+w ./.go && rm -rf ./.go`, retry.
- Measurement steps are not TDD, but **a number you did not measure is not a number you may report.** Show the command. Latency is only meaningful as a 3–5-run band, never a single run (measured lesson: p95 spans 225–316 ms across repeat runs, CV 9,0 %).
- A real full index for measuring is at `/tmp/full-real.sqlite` (**open read-only**; `sqlite.Open` applies migrations, so copy it if you need a writable one, and delete the copy).
- Docs German; code comments sparse and English. Never commit bulk data. Never read, print, or modify `.envrc.local`. Do not touch `VERSION`; CHANGELOG under `[Unreleased]`.

---

## Task 1: Measure the three levers before choosing between them

**Files:** `poc/measure/` (extend the existing harness), `docs/research/suggest-quality.md`

Nothing is implemented in this task. Its output decides Tasks 2 and 3, and the owner has already ruled out "blind cap only" — so measure what the ruled-in design actually costs and buys.

Against the full real index, for a fixed prefix set that **includes** the known-bad 2-char cases (`ca`, `al`, `sa`, `tr`, `ac`) and some realistic 3–5-char ones, report p50/p95 as a **3–5-run band**:

- **Baseline** — today's behaviour, no area, no cap.
- **Area-scoped** — with a Central-Europe area list (the WGSRPD-L3 codes for DE/AT/CH and neighbours; the bundle code already knows this set — reuse it, do not invent a second list). How far does p95 fall, and **how many candidates remain** for `ac` in-area vs global?
- **Cap 2000, no area** — the fallback path. p95, and **how often the cap actually truncates** across the prefix set.
- **What the cap costs:** for each capped prefix, how many *genera* would be lost if the cap were applied before rank diversity. This is the number that justifies the ordering in Task 2.

Also measure the composition problem directly: for `ac`, `ca`, `al` — the rank histogram of the top 10 results today. That is the evidence that flooding is real, and the before-picture for Task 2.

- [ ] **Step 1:** extend the harness; add a `--runs` flag if it still lacks one (a known, recorded gap).
- [ ] **Step 2:** run all four scenarios; write `docs/research/suggest-quality.md` (German) with every number and its command.
- [ ] **Step 3:** a short recommendation for Task 2's ranking order, grounded in the measured numbers.
- [ ] **Step 4: commit** `docs(research): suggest latency and composition measured`.

## Task 2: Hierarchical result composition (TDD, pure domain)

**Files:** `internal/domain/suggest.go` (+ tests)

`domain.RankSuggestions` today implements spec §B.1: PrefixHit → InArea → accepted → RankOrder → Score, `sort.SliceStable`. It orders correctly but does not *compose* — 142 genera can sit below thousands of species because rank is the fourth key, not a grouping.

Add **rank diversity** so a short prefix returns a usable mixture. The rule must be:
- **Rank-agnostic.** Do not hard-code "genus is the top rank". Families are absent today (0 rows) and may arrive later; the mechanism must place them correctly without a change.
- **Deterministic**, with a total tiebreaker so any permutation of the input yields identical output (`sort.SliceStable` alone does not give that — a lesson from SP6's `RankSynonyms`, which needed an explicit final key).
- **Explainable in the response.** A caller must be able to see *why* an entry is present — same discipline as SP6's exclusion summary and SP3's `trait_value.resolution`.

Exact shape is Task 1's recommendation to make; the obvious candidate is a bounded quota per rank (e.g. reserve slots for broader ranks before filling with species), but **do not implement a quota Task 1 did not justify with numbers**.

- [ ] **Step 1: failing tests** — prefix `ac` with 142 genera and thousands of species returns genera in the visible head, not buried; a prefix matching only species is unaffected; a hypothetical FAMILY row sorts above GENUS **without any code change** (pin this — it is the rank-agnosticism guarantee); ordering is identical across shuffled input; the reason for each entry is present.
- [ ] **Step 2: RED → implement → GREEN.** — [ ] **Step 3: DoD.**
- [ ] **Step 4: commit** `feat(domain): hierarchical rank diversity in suggest ranking`.

## Task 3: Area-first scoping and the 2000 fallback (TDD)

**Files:** `internal/application/suggest.go`, `internal/adapters/sqlite/suggest.go`, `internal/adapters/http/suggest.go` (+ tests), `docs/reference/http-api.md`

- **`area` accepts a list**, not a single value (`?area=DE,AT,CH`). `hostus bundle` already takes multi-area — reuse that parsing and the same WGSRPD-L3 validation; a second, divergent implementation is a defect.
- **With `area`:** scope candidates via `distribution` before ranking. This is the normal path.
- **Without `area`:** apply the **2000-candidate cap** — explicitly the fallback for "I am looking up a name and do not know the distribution".
- **The cap must be visible, never silent.** If it truncated, say so in the response (`"truncated": true` plus how many were considered). A suggest that quietly drops candidates is indistinguishable from a thin index — this project's standing rule is that loss is visible.
- **Order matters:** cap *after* rank diversity, never before, or the cap eats exactly the genera Task 2 exists to surface. Task 1 measures what that would have cost; pin it with a test.
- Unknown area code → `INVALID_QUERY` naming the offending value.

- [ ] **Step 1: failing tests** — a multi-area query scopes to those areas; an out-of-area concept is absent; no `area` applies the cap and reports truncation; the cap runs after diversity (a test that fails if the order is swapped); an unknown area code is rejected by name; `area` + a long prefix behaves as before.
- [ ] **Step 2: RED → implement → GREEN.**
- [ ] **Step 3: re-measure** the Task 1 scenarios against the implementation and record the before/after in `docs/research/suggest-quality.md`. State plainly whether the 373 ms case is fixed.
- [ ] **Step 4: DoD.** — [ ] **Step 5: commit** `feat(suggest): area-first scoping with a visible candidate cap`.

## Task 4: Generate the OpenAPI spec

**Files:** `api/openapi/`, a generator under `internal/` or `tools/`, `Makefile`, `flake.nix`, `scripts/doc-drift-check.sh`, `.github/workflows/`

`CLAUDE.md`: *"OpenAPI: Must be code-generated (no manual spec maintenance)."* It is hand-written for seven endpoints, and `doc-drift-check.sh` reports *"OpenAPI embedded/api-copy sync (skipped: no embedded spec yet)"* and *"routes ↔ spec contract test (skipped)"* — so nothing catches drift.

- Generate the spec from the handlers/DTOs. **The generator is dev tooling**: it must not enter the binary's import graph, and `make arch` must stay green.
- **A contract test must fail when a route exists without a spec entry, or a spec entry without a route.** That is the check that has been skipped since SP0 and is the actual point of this task — a generated-but-unverified spec is no better than a hand-written one.
- Un-skip both `doc-drift-check.sh` branches and wire the check into `make verify` so it cannot silently regress.
- The current hand-written spec is **accurate today** (verified during SP4 and SP6 reviews). Diff the generated output against it and **explain every difference** — a difference is either a generator gap or a latent hand-maintenance error, and you must say which.

- [ ] **Step 1:** choose and wire the generator; record why in a short ADR under `docs/explanation/decisions/`.
- [ ] **Step 2:** generate; diff against the hand-written spec; explain every delta.
- [ ] **Step 3:** the routes ↔ spec contract test, proven to fail both ways (add a route without spec; add a spec entry without a route). Say what you injected.
- [ ] **Step 4:** un-skip `doc-drift-check.sh`; wire into `make verify`; remove the now-stale known-gaps entry.
- [ ] **Step 5: DoD** + `make licenses` (a new tool may pull dependencies). — [ ] **Step 6: commit** `build(openapi): generate the spec and gate route/spec drift`.

## Task 5: Documentation pass + verdict

**Files:** `docs/reference/http-api.md`, `docs/how-to/`, `docs/explanation/known-gaps.md`, `docs/research/suggest-quality.md`, `CHANGELOG.md`

- [ ] **Step 1:** German how-to for the suggest path — when to pass `area`, what the cap means, and that **families are not in the index** (0 rows; genus is the broadest rank available today) so nobody waits for `Aceraceae`.
- [ ] **Step 2:** refresh the known-gaps page: drop what this milestone closed, and correct the **stale offline-bundle size** — SP6's Task 1 populated `published_in`/`nom_status` and grew the bundle by a measured ~50 MB, while `docs/how-to/offline-bundle.md` still cites 108,9 MB.
- [ ] **Step 3:** the SP7 verdict in `docs/research/suggest-quality.md` — **hält / hält mit Auflagen / hält nicht** — with the before/after latency band and an honest statement of what area scoping cannot fix.
- [ ] **Step 4:** CHANGELOG `[Unreleased]`; full gate green.
- [ ] **Step 5: commit** `docs(sp7): suggest quality how-to and measured verdict`.

---

## Self-Review Notes

- **Task 1 is a real gate.** The owner ruled out a blind cap; the remaining design still has choices (quota shape, cap position) that should be settled by numbers, not taste. This project has been burned by both extremes: assuming a vocabulary (`ParseRank`, `ParseRelation`) and reporting a metric without its control (SP6's corridor claim).
- **The family gap is a data fact, not a design decision.** 0 rows. Task 2 must not encode "genus is the top" or the mechanism silently breaks when families arrive.
- **Cap-after-diversity is the whole point.** Capping first would eat the genera that Task 2 exists to surface — the same shape as SP3's bug where CSV row order decided which trait value won.
- **A generated spec nobody checks is not an improvement.** Task 4's deliverable is the contract test, not the generator.
- Latency claims need a 3–5-run band; the single-run p95 already produced one false regression in this project (the p95 investigation, PR #15).

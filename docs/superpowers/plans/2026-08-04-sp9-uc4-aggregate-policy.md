# hostus 2.0 — SP9 UC4: `target_space` und `aggregate_policy` Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax.

**Goal:** Close the buildable half of UC4 — `POST /v1/match` gains `target_space=floraveg` and returns an `aggregate_policy` per entry — and make the unbuildable half **visibly absent** instead of silently missing.

**Why.** The source document specifies that hostus returns, per matched entry, an ESy-compatible name, `aggregate_policy` and `esy_diagnostic_relevance`. Measured on `master`: **all three are absent from production code** (`grep` finds 0 occurrences of the latter two). UC4 is the only use case with a genuine gap against the specification rather than a measured data ceiling.

**The split, measured 2026-08-04:**

| Field | Status | Evidence |
|---|---|---|
| `aggregate_policy` | **buildable now** | The FloraVeg name list carries aggregates as taxa in their own right: 16.403 rows, **308 aggregate entries**, incl. the source doc's own example — `Festuca ovina` (5647), `Festuca ovina aggr.` (5648), `Festuca ovina s. l.` (5649), all `accepted` |
| `esy_diagnostic_relevance` | **blocked on data** | The FloraVeg pipeline harvested only a name list (`taxon\|rank\|status\|accepted_taxon\|source_id`). **The ESy rule set was never obtained** — SP3 scoped it out explicitly ("the ESy expert system itself is a separate artifact"). Without the rules, "is this name a differential species in a rule" is unanswerable |

**This matters more than it looks.** The source document calls the third case the important one and the easiest to miss: if an ESy rule rests on a microspecies that could not be identified in the field, the answer is **"not decidable"**, not "habitat not met", and that distinction must survive the whole chain. SP9 cannot deliver it. What SP9 must therefore deliver is that nobody mistakes its absence for a negative answer.

**Tech Stack:** the existing stack. No new dependency.

---

## Global Constraints

- Branch `feature/sp9-uc4-aggregate`, forked from `master` (`a339cae`).
- **Run everything FOREGROUND with bounded timeouts. No monitors, no background-and-poll** — eight agents have stalled that way in this project.
- **Never read from the repository while an ingest transaction is open** (`SetMaxOpenConns(1)` → real deadlock; cost SP3 an escalation, re-verified in SP4/SP5/SP6).
- Hexagonal + depguard: `internal/application` imports `internal/domain` and `internal/ports` only. The `CLAUDE.md` runtime library list is exhaustive — no new dependency.
- Per-task DoD, all FOREGROUND: `go test -timeout 180s ./...` → `make mutation PKG=<each touched pkg>` (`Not covered: 0` and a positive mutant floor are enforced) → `make lint` clean **including `_test.go`**, both build-tag passes → `make verify` → `make test-integration` → `make security-check` → `make licenses` → `mkdocs --strict`.
- **LSP diagnostics here are frequently STALE** — trust only `nix develop -c go build ./...` and `go vet ./...`. If `make mutation` hits a permission error: `chmod -R u+w ./.go && rm -rf ./.go`, retry.
- Docs German; code comments sparse and English. Never commit bulk data. Never read, print, or modify `.envrc.local`. Do not touch `VERSION`; CHANGELOG under `[Unreleased]`.
- **A number you did not measure is not a number you may report.**
- FloraVeg's licence is **unclear** (`redistribution: unknown` — see `pipelines/README.md`). Local evaluation is in scope; the bundle gate must keep refusing it. Verify, do not assume, that the gate covers whatever this milestone ingests.

---

## Task 1: Ingest the FloraVeg name space (TDD) + measure what it can answer

**Files:** `internal/adapters/floraveg/` or reuse an existing reader, `internal/application/`, manifest support, `internal/app` wiring, `docs/research/`

**First, establish the ground truth — do not assume:** is the FloraVeg name list ingested anywhere today? Check `master` before writing code. The pipeline exists (`pipelines/floraveg/output/`), but an artifact on disk is not an ingested namespace.

Then ingest it as a name space usable as a match target, and **measure**:
- how many of the 16.403 FloraVeg names resolve onto a WCVP concept (this is the SP3 crosswalk machinery — reuse it, do not build a second one);
- how many of the **308 aggregates** resolve;
- how many WCVP concepts gain a FloraVeg counterpart, absolute and as a share.

A low number is a finding, not a failure — say it plainly.

- [ ] **Step 1: failing tests** — a FloraVeg row round-trips; an aggregate row is recognised as an aggregate; loss is counted and sampled, never silently dropped; the redistribution gate refuses a bundle carrying FloraVeg data by default and records it under `--force-include-restricted`.
- [ ] **Step 2: RED → implement → GREEN.** — [ ] **Step 3: measure** against the real index; write the numbers to `docs/research/`.
- [ ] **Step 4: DoD.** — [ ] **Step 5: commit** `feat(floraveg): ingest the FloraVeg name space`.

## Task 2: `target_space` and `aggregate_policy` on `/v1/match` (TDD)

**Files:** `internal/domain/`, `internal/application/match.go`, `internal/adapters/http/`, `api/openapi/openapi.yaml`, `docs/reference/http-api.md` (+ tests)

- `POST /v1/match` accepts `target_space` (initially `floraveg`; an unknown value → `INVALID_QUERY` naming it). Without it, behaviour is **exactly** as today — pin that with a test, because `/v1/match` is the endpoint UC3 and UC6 also use.
- Each entry gains `aggregate_policy`, a **tri-state**, not a boolean:
  - `known` — the target space carries the aggregate as a taxon of its own (measured: 308 such rows);
  - `unresolvable` — the target space knows only the microspecies. Per the source document this additionally means **coverage must not be distributed onto the microspecies**; say so in the field's documentation, since the consumer is what acts on it;
  - **absent/`unknown`** where no aggregate is involved at all. Do not emit `known` for a plain species — that would make the field meaningless.
- **`esy_diagnostic_relevance` must be conspicuously absent, not quietly missing.** Decide and document one of: omit it entirely and say why in the API reference, or emit an explicit `null` with a documented meaning of "not determinable, rule set not available". Never emit a value that could read as "not relevant" — that is exactly the false negative the source document warns about.

- [ ] **Step 1: failing tests** — no `target_space` ⇒ byte-identical response to today; `target_space=floraveg` sets the policy; *Festuca ovina agg.* yields `known` (the doc's own example, now answerable); an aggregate absent from FloraVeg yields `unresolvable`; a plain species carries no policy; an unknown target space is rejected by name; whichever `esy_diagnostic_relevance` shape was chosen is pinned.
- [ ] **Step 2: RED → implement → GREEN.** — [ ] **Step 3:** OpenAPI + `docs/reference/http-api.md`. — [ ] **Step 4: DoD.**
- [ ] **Step 5: commit** `feat(match): target_space and aggregate_policy for UC4`.

## Task 3: e2e, German how-to and an honest verdict

**Files:** `internal/app/integration_test.go`, `docs/how-to/`, `docs/explanation/known-gaps.md`, `docs/research/`, `CHANGELOG.md`

- [ ] **Step 1:** extend the `integration`-tagged e2e: ingest WCVP + FloraVeg → `POST /v1/match` with `target_space=floraveg` over a small relevé returns the expected policies. Assert **specific names and policies**, not just a 200.
- [ ] **Step 2:** German UC4 how-to with the source document's worked relevé (*Corynephorus canescens* 40 %, *Festuca ovina* agg. 15 %, *Jacobaea vulgaris* 2 %, *Rumex acetosella* 5 %), and an explicit **"was fehlt"** section: no `esy_diagnostic_relevance`, because the ESy rule set is not obtained — with the consequence spelled out, that the "not decidable" case the source document calls the most important one cannot currently be distinguished from "not met".
- [ ] **Step 3:** known-gaps entry for the ESy rule set (finding / impact / next step: probe whether it is machine-obtainable, as P8 did for Wisskirchen, including its own licence question).
- [ ] **Step 4:** the SP9 verdict in `docs/research/` — **hält / hält mit Auflagen / hält nicht** — with the measured crosswalk rates and what they mean for UC4.
- [ ] **Step 5:** CHANGELOG `[Unreleased]`; full gate green. — [ ] **Step 6: commit** `docs(sp9): UC4 how-to, e2e and verdict`.

---

## Self-Review Notes

- **The absent field is the risky part, not the present one.** A consumer that reads a missing `esy_diagnostic_relevance` as "not relevant" would silently invert the very distinction UC4 exists to preserve. That is a documentation and API-shape decision, and it deserves more care than the field that works.
- **`aggregate_policy` is tri-state on purpose.** Emitting `known` for every ordinary species would drain the field of meaning — the same failure as SP6's `absent` vs `unclassified`, where conflating "nothing recorded" with "recorded clean" would have been a real defect.
- Reuse the SP3 crosswalk. A second name-resolution path would diverge, and this project has already found the same silent-loss class twice in one milestone through duplicated mappers.
- Measure the crosswalk before believing it: SP3's trait crosswalk needed deterministic normalisation to move from 87,8 % to 98,0 %, and nobody predicted that from the raw hit rate.

# hostus 2.0 — Reality-Check Meilenstein (Volldaten) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax.

**Goal:** Answer, with measurements against the FULL real datasets, whether the hostus 2.0 concept holds together — and make the local-use-vs-redistribution distinction machine-enforced instead of a documentation promise.

**Why this milestone exists:** every correctness proof so far rests on a 20-taxon fixture. The questions that decide whether the design is viable (crosswalk hit rate, bundle size, suggest latency, ingest cost, namespace overlap) are all unanswered and all measurable. This milestone answers them before more features are stacked on the assumption that they're fine.

**Licensing frame (owner decision, 2026-08-01):** the sources without a findable license are being used **locally and privately for evaluation**, not redistributed. That is a different act from shipping them in a served index — the data is publicly offered, "no findable license" means unspecified rather than prohibited, and non-commercial scientific research use is privileged in German law (§60c UrhG; §87c for the database right). So: **local ingest of everything obtainable is in scope; redistribution stays gated** — and the gate becomes code, not prose.

**Tech Stack:** the existing SP0–SP3 stack. New work is one small feature (the redistribution gate), pipeline extensions, and a measurement harness.

## Global Constraints

- Builds on SP3 (branch forked from `feature/sp3-traits`). Existing: hexagonal layers, `dataset.yaml` + embedded JSON Schema, WCVP DwC-A ingest, trait pipelines/crosswalk, FTS5 suggest, bundle export, fuzzy match.
- **Never read from the repository while an ingest transaction is open** (`SetMaxOpenConns(1)` → real deadlock; cost SP3 an escalation). Two-phase everywhere.
- Per-task DoD for code tasks: tests pass → `make mutation PKG=<pkg>` ZERO new unjustified survivors → `make lint` clean incl `_test.go` → `make verify` green. **Run tests FOREGROUND with bounded timeouts** (`go test -timeout 120s ./...`); never background-and-poll.
- Measurement tasks are not TDD — but every number reported must come from a real run with the command shown, and nothing may be estimated or extrapolated silently. **A number you did not measure is not a number you may report.**
- Full downloads live under `data/` or `poc/data/` (both gitignored). **Never commit bulk data.** Fixtures stay small.
- Docs German; `dataset.yaml` schema changes must keep `make verify` + `mkdocs --strict` green.

---

## Task 1: `redistribution` gate (code, TDD)

**Files:** `internal/adapters/manifest/{dataset.schema.json,manifest.go,manifest_test.go,testdata/*}`; `internal/domain/` (a small enum); `internal/adapters/sqlite/{schema.sql,db.go,bundle.go,+tests}`; `cmd/hostus/bundle.go`; `dataset.example.yaml`; `docs/`

**Interfaces:**
- `dataset.yaml`: every backbone AND trait-vocabulary entry gains a required `redistribution: allowed | restricted | unknown` field (schema-validated; unknown values rejected). `allowed` = a clear license permitting redistribution (WCVP, EIVE, Tichý, Midolo, IPNI, WFO, COL-XR); `restricted`/`unknown` = everything else.
- Persist it: `backbone_version.redistribution` and `trait_vocabulary.redistribution` columns; `domain.Redistribution` type with `ParseRedistribution`.
- **`ExportBundle` refuses** to include data from a source that is not `allowed`: by default it fails with a clear error naming the offending sources; `--force-include-restricted` (CLI) overrides AND records the fact in `bundle_meta.restricted_sources` so a bundle can never silently carry unclearable data.
- `hostus ingest` prints a one-line notice per non-`allowed` source ("lokal genutzt, nicht redistribuierbar").

- [ ] **Step 1: failing tests** — manifest: missing/invalid `redistribution` rejected with a distinct error; parsed correctly. Bundle: exporting a DB containing a `restricted` source fails by default with the source named; with `--force-include-restricted` it succeeds AND `bundle_meta.restricted_sources` lists it; an all-`allowed` DB exports unchanged. Ingest prints the notice.
- [ ] **Step 2: RED.** — [ ] **Step 3: implement.** — [ ] **Step 4: GREEN.**
- [ ] **Step 5: DoD** — mutation on every touched package (0 new unjustified survivors), lint, verify, test-integration, mkdocs strict.
- [ ] **Step 6: commit** `feat(licensing): redistribution gate on bundle export`.

## Task 2: Acquire the remaining sources (pipelines)

**Files:** `pipelines/{germansl,eurosl,floraveg,euromed}/build.sh` (+ `convert.py` where needed), `pipelines/README.md`

Extend the established pattern (pinned source → canonical artifact → printed summary). Per source:
- **GermanSL** — `germansl.infinitenature.org/downloads/`; zip (TURBOVEG) + XLSX/SQLite exports. Emit a canonical **name-list CSV** (`taxon|rank|status|accepted_taxon|id`), pipe-delimited like the other canonical artifacts. Record version.
- **EuroSL** — `eurosl.infinitenature.org/downloads/`; a single SQLite file. Emit the same canonical name-list shape (read it with the existing `modernc.org/sqlite` from a small Go probe under `poc/`, or `sqlite3` if simpler in the pipeline).
- **FloraVeg.EU** — `floraveg.eu/download/`; Excel per topic. Emit canonical CSV for the taxon/name list (the ESy expert system itself is a separate artifact — out of scope here).
- **Euro+Med** — R1 found NO bulk export (a technical, not licensing, blocker). **Probe the CDM REST API first** (PoC P8 proved `api.cybertaxonomy.org` works for the Wisskirchen instance) — find Euro+Med's CDM instance and whether a paged taxon export is feasible. If yes, emit the canonical name list; **if not, record that clearly and move on — do not fake it.**

- [ ] **Step 1:** for each source, write + RUN `build.sh` against the real download; capture the summary (rows, distinct taxa, version, observed fields).
- [ ] **Step 2:** document the canonical name-list contract in `pipelines/README.md`.
- [ ] **Step 3:** for Euro+Med, report the probe result honestly (obtainable / not obtainable + why).
- [ ] **Step 4: commit** `feat(pipelines): GermanSL, EuroSL, FloraVeg (+ Euro+Med probe)`.

## Task 3: Full-scale ingest + measurement harness

**Files:** `poc/measure/` (a small Go program or shell harness, gitignored data), `docs/research/reality-check.md` (the report)

Run the REAL thing end to end and measure. Nothing here is estimated.

- [ ] **Step 1: full WCVP ingest** — download the complete WCVP DwC-A, ingest into a fresh DB. Measure: wall-clock, peak RSS, resulting DB size, row counts (names/concepts/synonyms/distributions), and whether `SetMaxOpenConns(1)` + the in-memory two-phase maps survive ~1.4M names (if it OOMs or takes absurdly long, **that is the finding** — record it precisely, don't paper over it).
- [ ] **Step 2: full trait ingest + crosswalk hit rate** — ingest EIVE (14,835 taxa), Tichý (8,909), Midolo against the full WCVP. **This is the headline number:** matched / unmatched / ambiguous per vocabulary, as a percentage. Also: how many WCVP accepted concepts end up with ANY trait values, and how many have BOTH EIVE and Tichý (the namespace-overlap question). Sample and categorize ~20 unmatched names to explain WHY they miss (orthographic variants? hybrids? authorship? genuinely absent from WCVP?).
- [ ] **Step 3: suggest latency** — with the full index, measure `GET /v1/suggest` p50/p95 for a handful of realistic prefixes (2–5 chars, common genera), with and without `area`. Report the actual numbers.
- [ ] **Step 4: bundle size** — export a Central-Europe-scoped bundle (WGSRPD-L3 codes for DE/AT/CH and neighbours) and report its real size against the spec's 10–20 MB claim, plus what's in it (concepts/names/traits/FTS).
- [ ] **Step 5: ingest the other name lists** (GermanSL/EuroSL/FloraVeg from Task 2, as far as obtained) and measure how much they'd change the crosswalk hit rate if used as a bridge (do they resolve names WCVP misses?). This is the concrete evidence for whether those license-unclear sources are actually worth the eventual clearance conversation.

## Task 4: Verdict report

**Files:** `docs/research/reality-check.md`, `CHANGELOG.md`

- [ ] **Step 1:** write the report (German): one section per measured question, each with the real number, the command that produced it, and a plain verdict — **hält / hält mit Auflagen / hält nicht**.
- [ ] **Step 2:** an explicit "was das für die sechs Use Cases heißt" section — especially UC1/UC4 (do enough taxa actually carry Zeigerwerte?) and UC1's offline bundle (size/latency).
- [ ] **Step 3:** a "was jetzt zu entscheiden ist" section: if the crosswalk hit rate is poor, name the options (better name normalization, use GermanSL/EuroSL as a bridge, accept partial coverage) with the measured trade-off for each — so the licensing conversation with colleagues can be had with numbers in hand.
- [ ] **Step 4:** CHANGELOG `[Unreleased]`; `mkdocs --strict` green.
- [ ] **Step 5: commit** `docs(research): reality-check measurements and verdict`.

---

## Self-Review Notes
- This milestone deliberately produces mostly *knowledge*, plus one small enforcement feature. That is the point: everything downstream (SP4–SP6, and the conversation with the data owners) is better decided with these numbers than without.
- The redistribution gate replaces a documentation promise with a machine check, so widening local use does not quietly widen what gets shipped.
- Honesty rule restated because it matters most here: a number that wasn't measured doesn't go in the report. "Euro+Med not obtainable" is a perfectly good result; a plausible-looking invented figure is not.

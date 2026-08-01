# hostus 2.0 — Hardening (Reality-Check-Fixes) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax.

**Goal:** Make hostus actually work on the full real datasets — fix the two ingest blockers, the bundle design gap, and invest in the measured highest-leverage improvement (name normalisation) — each proven against the reality-check baseline.

**Why these and in this order:** the reality check (PR #13, `docs/research/reality-check.md`) measured three defects and one strategic finding. The ingest blockers make everything else untestable at scale, so they come first. Bundle scoping is a real design gap. Name normalisation is, by measurement, worth far more than the licensing clearance (~0.34 %) that was previously assumed to be the bottleneck.

**Baseline to beat (all measured, all reproducible from `docs/research/reality-check.md`):**
| Metric | Baseline |
|---|---|
| Full WCVP ingest | **fails** — abort after 5.37 s on rank `"proles"`; rank-filtered run killed at 22:48 min (quadratic: 50k/100k/200k = 65/293/1338 s) |
| Ingest with 8 ad-hoc FK indexes | 276.70 s, peak RSS 2.97 GiB, DB 908 MiB, 1,437,761 names / 440,098 concepts / 953,262 synonyms / 1,982,550 distributions |
| Crosswalk per taxon | EIVE 87.76 % · Tichý 95.73 % · Midolo 96.41 % (ambiguous 10.98 / 15.73 / 18.11 %) |
| Suggest | p50 36.4 ms / p95 220.2 ms (with `area`: 38.7 / 253.8) |
| Bundle (GER) | 108.9 MB (20.5 MiB gzipped) vs the design's 10–20 MB; multi-area impossible; un-scoped export fails on the SQL variable limit |

## Global Constraints

- Builds on the reality-check branch. Existing: SP0–SP3 stack + the `redistribution` gate + seven canonical pipelines with full local data in each `pipelines/*/output/` (gitignored).
- **Never read from the repository while an ingest transaction is open** (`SetMaxOpenConns(1)` → real deadlock; cost SP3 an escalation). Two-phase everywhere.
- **Run long jobs FOREGROUND with generous bounded timeouts**; do not background-and-poll (four agents stalled that way).
- Per-task DoD: tests pass → `make mutation PKG=<pkg>` ZERO new unjustified survivors → `make lint` clean incl `_test.go` → `make verify` green. Measurement steps additionally: **a number you did not measure is not a number you may report.**
- Never commit bulk data or DB files. Docs German.
- **Every performance claim must be a before/after against the baseline above, produced by the same command.**

---

## Task 1: Rank vocabulary — never abort ingest on an unknown rank

**Files:** `internal/domain/taxon.go` (+ tests); `internal/application/ingest.go` (+ tests); `internal/adapters/http/` if the wire shape changes

**The defect:** `ParseRank` knows 6 ranks; WCVP uses 34. The ingest of 1.4 M rows aborts on the first `"proles"` (row 542377). Aborting a whole backbone because 0.775 % of rows carry an exotic infraspecific rank is the wrong failure mode.

**Measured rank inventory (real WCVP, `wcvp_taxon.csv`, use these exact strings):**
`Species` 1,048,616 · `Variety` 229,446 · `Subspecies` 73,948 · `Form` 43,609 · `Genus` 42,142 · `Subvariety` 3,350 · *(empty)* 2,744 · `proles` 2,351 · `lusus` 660 · `Subform` 645 · `nothosubsp.` 552 · `microgène` 371 · `Convariety` 184 · `nothovar.` 134 · `monstr.` 90 · `grex` 41 · `subproles` 18 · `stirps` 17 · `provar.` 16 · `nothof.` 15 · `psp.` 6 · `modif.` 6 · `mut.` 5 · `sublusus` 4 · `subap.` 3 · `subsubsp.` 2 · `subspecioid` 2 · `positio` 1 · `nid` 1 · `micromorphe` 1 · `microf.` 1 · `group` 1 · `ecas.` 1 · `agamosp.` 1. (`FAMILY` never occurs in the WCVP core — keep it, other backbones use it.)

**Design (implement this, don't invent an alternative):**
- Keep a **canonical** rank set the service reasons about — the existing six plus the infraspecific ranks that appear in volume and have a clear position: `SUBVARIETY`, `SUBFORM`, and the nothotaxon ranks (`nothosubsp.` → a hybrid-subspecies rank, `nothovar.`, `nothof.`). Choose names consistent with the existing constants.
- Everything else (including the empty string) maps to a single **`RankOther`** value that **preserves the verbatim source string** so nothing is lost for display/debugging (e.g. keep the raw value on the `Name`/`Concept` or in a `rank_verbatim` column — pick the smallest change that doesn't lose information, and say which).
- **`ParseRank` must never be the reason an ingest dies.** Either it returns `RankOther` for unknowns (and a separate strict variant exists for API input validation, where an unknown `rank=` query param SHOULD still be a 400), or the ingest catches and degrades. **Decide and document; the API's strict behaviour must not regress** (SP2's `?rank=bogus` → 400 test must still pass).
- The ingest must **count** unknown/other ranks and surface them in its report (like the crosswalk's unmatched reporting): `ranks: other=N (proles 2351, lusus 660, …)` — visible, not silent.
- `RankOrder` (suggest ranking priority 4) needs sensible ordinals for the new ranks: family < genus < species < subspecies < variety < subvariety < form < subform < other.

- [ ] **Step 1: failing tests** — `ParseRank` (or its ingest-facing variant) returns `RankOther` for every exotic string in the inventory above incl. the empty string, and never errors; the strict API path still rejects `bogus`; `RankOrder` orders the extended set correctly; the ingest report counts and lists the other-ranks; an ingest containing `proles` completes instead of aborting.
- [ ] **Step 2: RED.** — [ ] **Step 3: implement.** — [ ] **Step 4: GREEN.**
- [ ] **Step 5: DoD** — mutation on every touched package (0 new unjustified), lint, verify, test-integration, mkdocs strict.
- [ ] **Step 6: commit** `fix(domain): tolerate the full WCVP rank vocabulary; never abort ingest`.

## Task 2: FK indexes — turn the quadratic ingest linear

**Files:** `internal/adapters/sqlite/schema.sql` (+ tests)

**The defect:** every `REFERENCES` column except one is unindexed. With `INSERT OR REPLACE` and `foreign_keys=ON`, each insert triggers a table scan per FK check → measured quadratic scaling (50k/100k/200k rows = 65/293/1338 s, ×4.5) vs ~linear with indexes (5/11/25 s, ×2.2).

- Add indexes covering **every** FK/lookup column the ingest and reads use: `taxon_concept(backbone_id)`, `taxon_concept(accepted_name)`, `taxon_concept(parent_id)`, `name(basionym_id)`, `concept_name(concept_id)`, `concept_name(name_id)`, `xref(concept_id)`, `distribution(concept_id)`, `trait_value(concept_id)`, `vernacular(concept_id)`, plus anything else the reality-check run needed (it used 8 — verify the exact set against `docs/research/reality-check.md` and add any the reads benefit from).
- Note the trade-off honestly in a schema comment: indexes cost write time and DB size — the measurement says that cost is far smaller than the FK-scan cost.

- [ ] **Step 1: failing/verification test** — a test asserting each expected index exists (`sqlite_master`); plus a **scaling test**: ingest N and 2N synthetic rows and assert the time ratio is closer to linear than quadratic (keep N small enough to be fast and the assertion loose enough not to be flaky — e.g. ratio < 3.0; document why that threshold).
- [ ] **Step 2: RED → implement → GREEN.**
- [ ] **Step 3: measure** — re-run the 50k/100k/200k scaling measurement from the reality check with the new schema and report the real numbers next to the baseline (65/293/1338 s).
- [ ] **Step 4: DoD** — mutation, lint, verify, test-integration.
- [ ] **Step 5: commit** `perf(sqlite): index FK columns — ingest from quadratic to linear`.

## Task 3: Prove it — full WCVP + traits ingest end to end

**Files:** `docs/research/reality-check.md` (a new "nach Hardening" section), `.superpowers/.../task-3-report.md`

- [ ] **Step 1:** run the REAL full ingest (`hostus ingest`) of the complete WCVP DwC-A with the T1+T2 fixes, unmodified schema, no ad-hoc indexes. Report wall-clock, peak RSS, DB size, row counts, and the other-ranks report — against the baseline (fails / 276.70 s with ad-hoc indexes).
- [ ] **Step 2:** ingest EIVE + Tichý + Midolo against it; re-report the crosswalk hit rates against the baseline (87.76 / 95.73 / 96.41 % per taxon). They should be unchanged or slightly better (more ranks admitted ⇒ more concepts).
- [ ] **Step 3:** re-measure suggest p50/p95 on the full index (baseline 36.4 / 220.2 ms).
- [ ] **Step 4: commit** `docs(research): post-hardening full-scale measurements`.

## Task 4: Bundle — multi-area scoping and size

**Files:** `internal/adapters/sqlite/bundle.go` (+ tests), `cmd/hostus/bundle.go`, `internal/ports/output/`, docs

**The defects:** (a) `--area` is single-valued, so "Mitteleuropa" can't be expressed; (b) an un-scoped export fails — 440,098 concept ids become SQL placeholders → "too many SQL variables"; (c) the GER bundle is 108.9 MB vs the 10–20 MB design assumption.

- **(a) Multi-area:** accept a comma list (`--area DE,AT,CH,...`) mapping to a WGSRPD-L3 set; the existing single-value behaviour stays valid. Update `BundleOpts`.
- **(b) Scale:** stop binding one placeholder per concept id. Use a temporary table (or `json_each` on a single bound JSON array, as the codebase already does elsewhere) so the export is independent of scope size. An un-scoped full export must succeed.
- **(c) Size:** measure where the 108.9 MB actually goes (names? distributions? FTS?) **before** optimising, then reduce what's defensible for a field bundle — e.g. do offline clients need every synonym and every global distribution row, or only the in-scope ones? Any reduction must be a deliberate, documented product decision, not silent data loss: state exactly what a bundle no longer contains. Report the new size for a Central-Europe scope against the 108.9 MB baseline and the 10–20 MB design target; **if the target is unreachable without losing something the use cases need, say so and state the honest number** — the design assumption may simply have been wrong.

- [ ] **Step 1: failing tests** — multi-area scoping selects the union of areas; an un-scoped export succeeds (no SQL-variable error) — use a fixture large enough to prove the placeholder problem is gone; the bundle still opens standalone and serves Concept/Suggest/Traits; the redistribution gate still applies.
- [ ] **Step 2: RED → implement → GREEN.**
- [ ] **Step 3: measure** the real Central-Europe bundle size + composition breakdown, before and after any reduction.
- [ ] **Step 4: DoD** — mutation, lint, verify, test-integration, mkdocs.
- [ ] **Step 5: commit** `feat(bundle): multi-area scoping, scope-independent export, measured size`.

## Task 5: Name normalisation — the measured highest-leverage improvement

**Files:** `internal/domain/` (normalisation + tests), `internal/application/traits_ingest.go`, docs

**Why:** the unmatched sample showed the misses are dominated by **aggregates, hybrids, autonyms and orthography** — not by absent taxa (only 1/4/0 of 20 genuinely absent). The licensing route recovers ~0.34 %; this route addresses 12.47 % (EIVE) / 4.10 / 3.59 unmatched plus part of the 10.98–18.11 % ambiguous share.

Implement, each as a separately-testable pure step, and **measure the marginal gain of each** against the baseline hit rate:
- **Aggregates**: `Festuca ovina agg.` / `s.l.` / `aggr.` — already partly handled in matching; make sure the trait crosswalk benefits too.
- **Hybrids**: the `×`/`x` hybrid marker (`×Aegilotriticum`, `Festuca × arundinacea`) — normalise the marker and spacing.
- **Autonyms**: `Festuca ovina subsp. ovina` ↔ `Festuca ovina` — decide and document the rule (an autonym is the same taxon as its parent species for trait purposes, or it isn't — pick one, justify it botanically, test it).
- **Orthography**: the diacritic fold already exists (`domain.Canonicalize`, parity-tested against `unicode61`); extend to the specific patterns the sample showed (say which).
- Do **not** turn this into fuzzy matching — that exists and is deliberately `requires_review`. This is deterministic normalisation.

- [ ] **Step 1: failing tests** per normalisation rule, using REAL failing names from the reality-check unmatched sample (cite them).
- [ ] **Step 2: RED → implement → GREEN.**
- [ ] **Step 3: measure** — re-run the full trait crosswalk and report the new hit rates against the 87.76 / 95.73 / 96.41 % baseline, **per normalisation rule** where feasible, so the value of each is visible.
- [ ] **Step 4: DoD** — mutation (this is pure domain logic — hold it to zero survivors), lint, verify, test-integration.
- [ ] **Step 5: commit** `feat(domain): deterministic name normalisation (aggregates, hybrids, autonyms, orthography)`.

## Task 6: Final re-measurement + verdict update

**Files:** `docs/research/reality-check.md`, `CHANGELOG.md`

- [ ] **Step 1:** one consolidated before/after table for every baseline metric.
- [ ] **Step 2:** update the per-measurement verdicts (`hält` / `hält mit Auflagen` / `hält nicht`) to their post-hardening state, keeping the original verdicts visible as history.
- [ ] **Step 3:** update "was jetzt zu entscheiden ist" — what remains open, and whether the licensing recommendation still stands after normalisation (re-measure the bridge gain if normalisation changed it).
- [ ] **Step 4:** CHANGELOG; `mkdocs --strict`; `make verify` + `make security-check` + `make test-integration` green.
- [ ] **Step 5: commit** `docs(research): post-hardening verdicts`.

---

## Self-Review Notes
- Order is dependency-driven: T1+T2 unblock any full-scale run, T3 proves them, T4 and T5 are independent improvements each measured against the same baseline, T6 consolidates.
- Every task carries a measured before/after — this milestone exists because unmeasured assumptions (20-taxon fixture, "10–20 MB", "licensing is the bottleneck") turned out to be wrong.
- T5 is the one with the most product judgement in it (especially the autonym rule); it must stay deterministic and must not quietly become fuzzy matching.

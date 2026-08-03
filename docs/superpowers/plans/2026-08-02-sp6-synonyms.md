# hostus 2.0 — SP6 Synonyme mit Relevanzfilter Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax.

**Goal:** Serve `GET /v1/concept/{id}/synonyms` — the accepted name plus the *one to three* synonyms that actually belong in a publication, filtered by explicit, inspectable criteria.

**Why this milestone exists (UC5, source doc §UC5):** *"Das Problem ist Filterung, nicht Beschaffung."* POWO lists over twenty synonyms for *Corynephorus canescens*, including `nom. nud.`, `pro syn.` and a dozen 19th-century varieties. A publication needs one to three. Measured on our real index: **4,09 synonyms per concept on average, 1127 at the maximum.** Returning the raw list is not a feature.

**Architecture:** a scoring/filtering pass in `internal/application` over data the index already holds, exposed through one new read endpoint. No new backbone, no new external source, no new table. `/v1/concept/{id}` keeps returning its unfiltered synonym list unchanged — this endpoint is additive.

**Tech Stack:** the SP0–SP5 stack. No new dependency.

---

## Global Constraints

- Branch `feature/sp6-synonyms`, forked from `feature/sp5-sec-translate` at `6db51fd`. **Work in the worktree `/Users/jbrunner/work/projects/hostus-sp6`.**
- **A CDM crawl is running in the main worktree** (`/Users/jbrunner/work/projects/hostus`, ~11 h left). Do not run `pipelines/cdm/build.sh` or `crawl.py`, do not write under `pipelines/cdm/.cache/`, and make **no** request to `api.cybertaxonomy.org`. SP6 needs none of it.
- **Run everything FOREGROUND with bounded timeouts. No monitors, no background-and-poll** — eight agents have stalled that way in this project.
- **Never read from the repository while an ingest transaction is open** (`SetMaxOpenConns(1)` → real deadlock; cost SP3 an escalation, re-verified in SP4 and SP5). Two-phase.
- Hexagonal + depguard: `internal/application` imports `internal/domain` and `internal/ports` only. Composition root is `internal/app`. The allowed-libraries list in `CLAUDE.md` is exhaustive.
- Per-task DoD, all FOREGROUND: `go test -timeout 180s ./...` → `make mutation PKG=<each touched pkg>` **zero new unjustified survivors** → `make lint` clean **including `_test.go`** (it now also runs `--build-tags integration`) → `make verify` → `make test-integration` → `make security-check` → `mkdocs --strict`.
- **LSP diagnostics in this repo are frequently STALE.** Trust only `nix develop -c go build ./...` and `go vet ./...`. If `make mutation` hits a permission error: `chmod -R u+w ./.go && rm -rf ./.go`, retry.
- Docs German; code comments sparse and English. Never commit bulk data. Never read, print, or modify `.envrc.local`. Do not touch `VERSION`; CHANGELOG entries go under `[Unreleased]`.
- Measurement steps are not TDD, but **a number you did not measure is not a number you may report.** Show the command.

### Measured starting position (2026-08-02, real index, 440.534 concepts / 1.448.984 names)

UC5 names five relevance criteria. Measured against what the index actually holds, **two are deliverable today, one is partial, and two are not**:

| UC5 criterion | Data needed | Measured status |
|---|---|---|
| Nomenclatural status (`nom. nud.`, `nom. superfl.`, `pro syn.` ausschließen) | `name.nom_status` | **0 of 1.448.984 populated** — see Task 1, this is a fixable ingest defect |
| Rank (Varietäten/Formen ausschließen) | `name.rank` | ✅ available; 201.957 VARIETY + 42.681 FORM + 3.328 SUBVARIETY + 641 SUBFORM synonym names |
| Homotypisch vor heterotypisch | `concept_name.homotypic` | ⚠️ partial: 271.821 known-true, **692.941 NULL** (unknown, deliberately not guessed — SP3 decision) |
| Im Bezugsraum verwendet | per-*name* area | ❌ **not expressible.** `distribution` is per *concept*; a synonym is a *name*. There is no per-name area data in the index |
| In Standardwerken verwendet (Rothmaler/Oberdorfer) | publication usage | ❌ not available today. ROTHMALER, OBERDORFER, HEGI and SCHMEIL-FITSCHEN are literally four of SP5's 18 CDM classifications, but that data is `redistribution: unknown` and its crawl is unfinished |

**This is the milestone's central honesty problem.** Two of five advertised criteria cannot be delivered, and the endpoint must not pretend otherwise — see Task 4.

---

## Task 1: Stop dropping `nom_status` and `published_in` (TDD)

**Files:** `internal/application/ingest.go` (+ tests), `internal/adapters/wcvp/reader.go` if needed, `docs/research/`

**The defect:** `internal/adapters/wcvp/reader.go:113,171-172` parses `namepublishedin` and `nomenclaturalstatus`; `domain.Name` has `PublishedIn` and `NomStatus` fields; `internal/adapters/sqlite/db.go:481-483` writes both columns. But the mapper at `internal/application/ingest.go:331-337` builds `domain.Name` **without either field**, so both are silently empty for every one of the 1.448.984 ingested names. Read from the file, dropped in the middle, written as empty.

Without `nom_status` the single criterion UC5 calls out by name — excluding *Corynephorus incanescens* Bubani as `nom. superfl.` — is undeliverable. This task is a prerequisite, not a cleanup.

- [ ] **Step 1: failing test** — a WCVP fixture row carrying a non-empty `nomenclaturalstatus` and `namepublishedin` must round-trip through `Ingest` and be readable from the `name` row. Assert both fields; assert an empty source value stays empty rather than becoming a placeholder.
- [ ] **Step 2: RED → implement → GREEN.** One-line-ish fix in the mapper; check no other `domain.Name` construction site drops fields the same way (`grep -rn 'domain.Name{'`).
- [ ] **Step 3: measure against the real data.** Re-ingest the full WCVP DwC-A (~280 s per the reality-check) or, if that artifact is not available in this worktree, ingest the largest available real slice and say exactly what you measured. Report: how many names now carry a `nom_status`, **the distribution of distinct values** (this is the vocabulary Task 2 must map — do not assume `nom. nud.`/`nom. superfl.`/`pro syn.` is the whole set; `ParseRank` assumed 6 ranks against 34 real ones and aborted the full ingest), and how many carry a `published_in`.
- [ ] **Step 4: DoD.**
- [ ] **Step 5: commit** `fix(ingest): carry nomenclatural status and publication through to the index`.

## Task 2: The relevance model (TDD, pure domain)

**Files:** `internal/domain/synonym.go` (+ tests)

Keep this task free of I/O so the rules are testable in isolation and the judgement is inspectable.

### What Task 1 measured — this replaces the vocabulary assumption this task originally carried

`nom_status` is populated on **99.252 of 1.448.984 names (6,85 %)**, 92.492 of them on synonym-role names, across **1.304 distinct values**. The top 20 cover 95,8 %; 1.225 values have fewer than 10 hits. A closed enum over three doc-named values is not a viable design, and neither is fail-loud parsing — it would abort on the long tail constantly.

The decisive measurement: **the source doc's own worked example does not match its own rule.** *Corynephorus incanescens* Bubani (`wcvp:name:405842`) carries `", nom. illeg. superfl."`, **not** `", nom. superfl."`. Equality matching on the three values UC5 names would miss the very case UC5 is explained with:

| match | names hit |
|---|---|
| exact `", nom. superfl."` | 1.716 |
| contains `superfl` | **12.502** |
| contains `illeg` | 49.705 |
| contains `not validly publ` | 18.623 |
| contains `nom. nud` | 9.230 |
| contains `pro syn` | 6.224 |

Further measured traps, all of which the design must survive:
- Every value carries a leading `", "` (99,86 %) — WCVP concatenates a citation fragment with the status.
- 684 cells carry **several** statuses (`", nom. illeg., later homonym"`), but splitting on `,` is wrong too: commas occur *inside* single statuses (`", contrary to Art. 23.6. (ICN, 2012)."`).
- **141 values are not statuses at all** — citation fragments (`"[Cusc.: 184]"`) or free text (`published as "mutatio nova"`); some mix both.
- Spelling variants coexist: `without a Latin descr.` / `without latin descr.` / `sine descr. lat.`; `nom. rej.` / `nom. rejic.`; `nom. altern.` / `nom. alt.`
- Five values need a **botanical, not technical** decision and must be surfaced rather than guessed: `", sensu auct."` (1.115 — a misapplication, not a nomenclatural defect), `", tentatively listed as a synonym."` (290), `", fossil name."` (272), `", isonym"` (9), `", not validly publ.?"` (8, with a literal question mark).

**Interfaces:**
- `domain.NomStatusJudgement` — classify a raw `nom_status` string by **token containment**, not equality, into a small set of publication judgements (at minimum: *disqualifying* / *acceptable* / *unclassified*). Normalise the leading `", "` and case before matching. Every token rule must be traceable to a count measured in Task 1 — no rule for a token nobody observed.
- The long tail gets an explicit **`unclassified`** outcome. It must **not** silently mean "fine to publish": an unclassified synonym is either withheld from the publication list or returned with its raw status visible and flagged — decide which, justify it, and make the count visible in the response (Task 3's exclusion summary). This is the same discipline as `trait_value.resolution`: an approximation is recorded in the data, not just in a comment.
- Report the five botanical-decision values as an explicit, named open item rather than classifying them silently.
- `domain.SynonymRelevance` — the decision for one synonym, carrying **why**: which rules excluded or ranked it. A consumer must be able to see the reason, not just the verdict.
- `domain.RankSynonyms(items []SynonymCandidate, opts) []SynonymRelevance` — deterministic ordering, `sort.SliceStable`, same discipline as `domain.RankSuggestions` (§B.1).

**Rules, in this priority (source doc §UC5):**
1. **Exclude by nomenclatural status** — by token containment over the measured tokens. Excluded, not down-ranked: a `nom. nud.` does not belong in a publication at any position. The *Corynephorus incanescens* case (`", nom. illeg. superfl."`) must be excluded — it is the acceptance test for this rule.
2. **Exclude by rank** when the caller publishes at species level — `VARIETY`, `FORM`, `SUBVARIETY`, `SUBFORM`. Caller-controlled, not hard-wired.
3. **Homotypic before heterotypic.** `homotypic` is a **tri-state**: `true` (271.821 rows, basionym-proven), `false`, and `NULL` (692.941 rows — *unknown*, an SP3 decision not to guess). **NULL must never be treated as `false`.** Rank known-homotypic first, then unknown, then known-heterotypic — and say in the response which of the three it was.
4. **Basionym first among homotypic synonyms** (`name.basionym_id`, 429.172 names carry one) — the source doc's worked example puts *Aira canescens* L. first for exactly this reason.
5. Stable tiebreaker last, so identical input always yields identical output.

- [ ] **Step 1: failing tests** — *Corynephorus incanescens* with the real `", nom. illeg. superfl."` string is excluded and the reason is stated (equality matching would miss it, so pin containment explicitly); an unclassified long-tail value takes the documented `unclassified` path and is counted, never silently published; varieties are excluded only when the caller asks; a NULL-homotypic synonym ranks between known-homotypic and known-heterotypic and is **not** reported as heterotypic; the basionym leads the homotypic block; ordering is stable across shuffled input.
- [ ] **Step 2: RED → implement → GREEN.** — [ ] **Step 3: DoD.**
- [ ] **Step 4: commit** `feat(domain): publication relevance model for synonyms`.

## Task 3: Serve `GET /v1/concept/{id}/synonyms` (TDD)

**Files:** `internal/application/synonyms.go` (+ tests), `internal/adapters/sqlite/read.go` (+ tests), `internal/adapters/http/synonyms.go` (+ tests), `internal/adapters/http/router.go`, `api/openapi/openapi.yaml`, `docs/reference/http-api.md`

```
GET /v1/concept/{id}/synonyms?relevance=publication&rank=species&max=3
```

- `relevance` — `publication` applies the Task 2 rules; **the unfiltered list must stay reachable** (default, or an explicit value) so this endpoint never becomes the only door and quietly hides data. Decide which is the default and justify it.
- `max` — cap. Bounded, with a documented maximum; reject absurd values rather than allocating.
- Unknown concept id → `NOT_FOUND`. Invalid `relevance`/`rank` → `INVALID_QUERY` naming the offending value.
- The repository needs `nom_status`, `basionym_id` and the tri-state `homotypic` per synonym — extend the read, and keep `Concept()`'s existing `SynonymName` shape unchanged so `/v1/concept/{id}` is untouched.

**The response must carry its own reasoning.** Each returned synonym states its `homotypic` tri-state, its `nom_status`, whether it is the basionym, and why it ranked where it did. Each *excluded* synonym — at least in aggregate — must be visible: `"excluded": {"nom_status": 4, "rank": 12}`. A filter that silently drops 20 of 23 synonyms without saying so is indistinguishable from a broken query, and this project's whole discipline is that loss is visible (`hostus ingest` prints `matched/unmatched/ambiguous`; `trait_value.resolution` records approximations).

- [ ] **Step 1: failing tests** — filtered vs unfiltered differ on a real fixture concept; `max` truncates *after* ranking, never before; the exclusion summary counts match what was removed; `NOT_FOUND`; `INVALID_QUERY` names the bad value; a concept with zero synonyms returns an empty list, not an error.
- [ ] **Step 2: RED → implement → GREEN.**
- [ ] **Step 3:** OpenAPI + `docs/reference/http-api.md`. (Note: OpenAPI is hand-maintained here despite CLAUDE.md — a documented, tracked deviation since S14. Follow the existing precedent; do not build a generator in this task.)
- [ ] **Step 4: DoD.**
- [ ] **Step 5: commit** `feat(http): GET /v1/concept/{id}/synonyms with publication relevance`.

## Task 4: Honesty about the two criteria we cannot deliver + verdict

**Files:** German how-to under `docs/how-to/`, `docs/research/reality-check.md`, `internal/app/integration_test.go`, `CHANGELOG.md`

UC5 advertises five criteria; SP6 delivers two fully, one partially, and two not at all. **The documentation must say so plainly, in the endpoint's own how-to, not buried in a research file.** A user who reads `relevance=publication` and assumes regional filtering happened will publish the wrong synonym list.

- [ ] **Step 1:** German UC5 how-to — the worked *Corynephorus canescens* example from the source doc, and an explicit "was dieser Filter **nicht** kann" section: (a) **no regional filtering** — `distribution` is per concept, a synonym is a name, so "im Bezugsraum verwendet" is not expressible with the current schema; say what it would take. (b) **no standard-work filtering** — Rothmaler/Oberdorfer usage would come from SP5's CDM classifications, which are `redistribution: unknown`; say that too. (c) `homotypic` is tri-state and 692.941 rows are *unknown*, not heterotypic.
- [ ] **Step 2:** extend the `integration`-tagged e2e: ingest → serve → the filtered list is shorter than the unfiltered one for a concept that has excludable synonyms, and the exclusion summary explains the difference. Assert **specific** names, not just counts. `make test-integration` green.
- [ ] **Step 3:** measure on the real index and write the SP6 verdict into `docs/research/reality-check.md`: for how many concepts does the publication filter actually change the answer; the distribution of resulting list lengths (does it really land at one to three?); how many concepts are left with **zero** publishable synonyms and whether that is right. Verdict in the established form — **hält / hält mit Auflagen / hält nicht**.
- [ ] **Step 4:** CHANGELOG `[Unreleased]`; full gate green.
- [ ] **Step 5: commit** `test(sp6): e2e synonym relevance + measured verdict`.

---

## Self-Review Notes

- **Task 1 is a prerequisite disguised as a bug fix.** Without `nom_status` the headline criterion is undeliverable, and the defect is the same silent-loss shape this project keeps finding: parsed, then dropped by one mapping line, with the column sitting empty and nothing complaining.
- **The status vocabulary is measured, never assumed.** `ParseRank` assumed six ranks against WCVP's 34 and aborted the full ingest after 5,4 s; `ParseRelation` had to be widened from five to seven values after measurement. `ParseNomStatus` gets the same treatment — Task 1 measures the vocabulary *before* Task 2 encodes it.
- **The tri-state `homotypic` is the subtle trap.** 692.941 NULLs mean *unknown*; SP3 deliberately refused to guess. Ranking them as heterotypic would silently demote most of the corpus on a fact nobody established.
- **A filter's honesty is measured by what it says it removed.** Hence the exclusion summary in the response, not just a shorter list.
- Two of five criteria are undeliverable, and the plan's job is to make that visible rather than to quietly ship three-fifths of a feature and call it done.

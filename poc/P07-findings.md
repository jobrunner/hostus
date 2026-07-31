# PoC P7 — Wikidata taxon-database xref properties (Phase 0, Task 0.7, Gate SP4)

## Goal

Verify the hostus 2.0 spec §7 SP4 / D.3 step 5 assumption: Wikidata's taxon
items expose the cross-reference properties hostus needs to enrich a GBIF
result with GBIF-taxon-ID, Catalogue of Life, Euro+Med, and FloraVeg.EU
identifiers — and confirm the property numbers named in the spec
(P14607/P846/P10585/P12380/P12100) are the *current, correct* ones and not
deprecated/renamed.

## Method

`poc/p07_wikidata/probe.sh` (`set -euo pipefail`), run via
`nix develop -c bash poc/p07_wikidata/probe.sh`. Hits, with a descriptive
`User-Agent` header (required by WDQS/Wikidata):

1. `Special:EntityData/<QID>.json` — full entity dump (ground truth for
   which statements actually exist) for both reference taxa.
2. `query.wikidata.org/sparql` — `poc/p07_wikidata/query.rq`, a `VALUES`-based
   SPARQL query joining both taxa against the candidate property list, with
   `wikibase:label` service to pull each property's current English label.
3. `Special:EntityData/<PID>.json` for each candidate property — independent
   confirmation of the property's current label/description (catches
   renamed/deprecated IDs without relying on the SPARQL label service alone).
4. `wbsearchentities` search for "World Flora Online ID" — the spec only
   said "the WFO id property" without a number, so this discovers it.

Raw JSON saved under `poc/data/*.json` (gitignored via `poc/.gitignore`).

Reference taxa:
- *Corynephorus canescens* = **Q159953** (verified in spec appendix — correct).
- *Jacobaea vulgaris* = **Q15630491** — **the QID guessed in the task brief,
  Q159749, does not resolve to this taxon** (that ID looks unused/wrong).
  Resolved the correct QID via `wbsearchentities` before proceeding.

## Property verification (current label, via independent entity-data fetch)

| Property | Label (verified) | Notes |
|---|---|---|
| P14607 | GBIF taxon ID | Description: "taxon identifier in GBIF **after 2026 update**". Correct, current property. |
| P846 | GBIF-species-ID (before 2026 update) | Description explicitly says **"now use P14607"** — P846 is the legacy/superseded property, kept for historical statements. Spec's framing of P846 as "legacy/old" is confirmed correct. |
| P10585 | Catalogue of Life ID | Correct, current. |
| P12380 | Euro+Med PlantBase taxon ID | Correct, current. |
| P12100 | FloraVeg.EU taxon ID | Correct, current. |
| P961 | IPNI plant ID | Correct, current. |
| P5037 | Plants of the World Online ID | Correct, current. |
| P7715 | World Flora Online ID | **Not in the spec's list at all** — discovered via `wbsearchentities` search for "World Flora Online ID". This is the correct WFO property to use if hostus wants WFO xrefs. |

All five spec-named property IDs (P14607, P846, P10585, P12380, P12100) are
**correct and current** — none are deprecated/renamed. No corrections needed
to the property numbers themselves.

## Resolved values

| Property | Label | Corynephorus canescens (Q159953) | Jacobaea vulgaris (Q15630491) | Present? |
|---|---|---|---|---|
| P14607 | GBIF taxon ID (new) | *(absent)* | *(absent)* | **Neither taxon has this populated yet** |
| P846 | GBIF taxon ID (legacy) | `5290194` | `5388602` | Both |
| P10585 | Catalogue of Life ID | `YQW8` | `3QJJ5` | Both |
| P12380 | Euro+Med PlantBase taxon ID | *(absent)* | *(absent)* | Neither |
| P12100 | FloraVeg.EU taxon ID | `Corynephorus canescens` | *(absent)* | Corynephorus only |
| P961 | IPNI plant ID | `396681-1` | `226649-1` | Both |
| P5037 | POWO ID | `urn:lsid:ipni.org:names:396681-1` | `urn:lsid:ipni.org:names:226649-1` | Both (stored as IPNI-format LSID, which is POWO's actual ID scheme) |
| P7715 | World Flora Online ID | *(absent)* | `wfo-0000031875` | Jacobaea only |

## Cross-validation against PoC P3 (GBIF v2/match + COL-XR)

This is the important coverage/consistency finding for SP4:

- **P846 (legacy GBIF id) values match the GBIF *backbone* usage keys**
  found in `poc/P03-findings.md` exactly: `5290194` (Corynephorus) and
  `5388602` (Jacobaea vulgaris) — byte-for-byte identical to the
  `usage.key` GBIF returned from `v2/species/match` **without**
  `checklistKey` (i.e. the plain GBIF backbone).
- **P10585 (Catalogue of Life ID) values match the COL-XR `usageKey`s**
  from P03 exactly: `YQW8` and `3QJJ5` — identical to `usage.key` from
  `v2/species/match` **with** `checklistKey=7ddf754f-...` (COL-XR).

This confirms: (a) Wikidata's GBIF-id property really does point at the GBIF
*backbone* classification (not COL-XR), so it is a reliable, independently
sourced key to join a GBIF backbone hit to a Wikidata item; and (b)
Wikidata's Catalogue-of-Life property is consistent with GBIF's own COL-XR
resolution, meaning hostus could use either path (GBIF v2/match+checklistKey,
or Wikidata P10585) to get the same COL identifier, which is a useful
cross-check/fallback option for SP4.

- **P14607 (the "new" GBIF id, post-2026 rename) is not yet populated on
  either test taxon**, despite the property itself being live and correctly
  labeled. This looks like an in-progress Wikidata data migration (P846 →
  P14607) that hasn't reached these items yet. **Implication for hostus:**
  a SP4 lookup that queries P14607 *only* will silently miss data that is
  present under P846. hostus must query **both** P14607 and P846 (preferring
  P14607 when both exist, since it's the forward-looking property) rather
  than assuming the "new" property alone is sufficient.
- **P12380 (Euro+Med) is sparse** — absent for both reference taxa despite
  both being unremarkable, well-documented European vascular plants. This
  property should be treated as optional/best-effort enrichment, not a
  required field.
- **P12100 (FloraVeg.EU) and P7715 (WFO) are each present for only one of
  the two taxa** — further evidence that per-database coverage on Wikidata
  is uneven even for common species, reinforcing that all of these xrefs
  should be modeled as optional in the SP4 response schema.

## Verdict

**⚠️ Partial — viable for SP4 but only if implemented as "best-effort,
multi-property" enrichment, not a single-property lookup.**

Rationale:
- Properties are correctly identified in the spec (no wrong/deprecated
  Pxxxx among the five named), and cross-validate cleanly against GBIF/COL
  data independently obtained in PoC P3 — Wikidata is a trustworthy xref
  source when data exists.
- However, coverage is inconsistent per property even for two mainstream,
  well-known European vascular plants: P14607 (0/2), P12380 (0/2), P12100
  (1/2), P7715/WFO (1/2). Only P846 (legacy GBIF), P10585 (COL), P961
  (IPNI), and P5037 (POWO) were present for both test taxa.
- SP4 implementation should: (1) query P14607 with P846 fallback for GBIF
  ids, (2) treat P12380/P12100/WFO as optional/nullable fields, (3) rely on
  P10585/P961/P5037 as the more dependable baseline xrefs, and (4) not error
  or degrade the response when any single property is missing.

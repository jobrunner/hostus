# PoC P3 — GBIF v2/species/match vs COL-XR checklistKey (Phase 0, Task 0.3, Gate SP1)

## Goal

Verify the hostus 2.0 spec §3 P3 / appendix assumption: GBIF's `/v2/species/match`
resolves names against the COL-XR (Catalogue of Life Extended Reconciliation)
checklist when given `checklistKey=7ddf754f-d193-4cc9-b351-99906754a03b`, while
`/v1/species/suggest` has no such parameter support and always answers from the
default GBIF backbone.

## Method

Shell/curl probe (`poc/p03_gbif/probe.sh`, `set -euo pipefail`), run inside the
project's Nix dev shell (`nix develop -c bash poc/p03_gbif/probe.sh`). Raw JSON
responses saved under `poc/data/*.json` (gitignored). No code touched outside
`poc/p03_gbif/` and this file.

Reference taxa:
- *Corynephorus canescens* (L.) P.Beauv. (IPNI 396681-1)
- *Jacobaea vulgaris* Gaertn. (IPNI 226649-1)

## Endpoint contract discovery (deviation from the task's assumed URL)

The task description assumed `?name=...` for v2/match. That parameter does
**not** work as expected:

```
GET https://api.gbif.org/v2/species/match?name=Corynephorus%20canescens&checklistKey=...
→ {"diagnostics":{"matchType":"NONE","note":"No name given", ...},"synonym":false}
```

The current (2026) GBIF v2 species-match contract requires **`scientificName`**,
not `name`:

```
GET https://api.gbif.org/v2/species/match?scientificName=Corynephorus%20canescens&checklistKey=7ddf754f-d193-4cc9-b351-99906754a03b
```

This returns a proper match. **This is an important correction for the hostus
2.0 spec/implementation**: any client code calling v2/match must use
`scientificName`, or it will silently get `matchType: NONE` with no error
status (HTTP 200).

## Results

### 1. v2/match WITH checklistKey (COL-XR) — Corynephorus canescens

```
GET https://api.gbif.org/v2/species/match?scientificName=Corynephorus%20canescens&checklistKey=7ddf754f-d193-4cc9-b351-99906754a03b
```

```json
{
  "usage": {
    "key": "YQW8",
    "name": "Corynephorus canescens (L.) P.Beauv.",
    "canonicalName": "Corynephorus canescens",
    "rank": "SPECIES",
    "status": "ACCEPTED"
  },
  "classification": [ /* Eukaryota > Plantae > Pteridobiotina > Tracheophyta > Liliopsida > Poales > Poaceae > Corynephorus > Corynephorus canescens */ ],
  "diagnostics": { "matchType": "EXACT", "confidence": 98 },
  "synonym": false
}
```

Note the COL-XR usage key is an **alphanumeric string** (`YQW8`), and the
classification tree includes COL-specific ranks not present in the GBIF
backbone (`DOMAIN`, `SUBKINGDOM`) — this is clearly the COL checklist, not
backbone.

### 2. v2/match WITHOUT checklistKey (default GBIF backbone) — same taxon

```
GET https://api.gbif.org/v2/species/match?scientificName=Corynephorus%20canescens
```

```json
{
  "usage": { "key": "5290194", "canonicalName": "Corynephorus canescens", "status": "ACCEPTED" },
  "classification": [ /* Plantae > Tracheophyta > Liliopsida > Poales > Poaceae > Corynephorus > Corynephorus canescens */ ],
  "diagnostics": { "matchType": "EXACT", "confidence": 99 },
  "synonym": false
}
```

The `usage.key` differs entirely (`5290194` numeric backbone key vs. `YQW8`
COL-XR key), and the classification tree is shallower (no DOMAIN/SUBKINGDOM
ranks) — **conclusive proof v2/match honors `checklistKey` and switches which
checklist answers the query.**

### 3. v1/suggest WITH checklistKey (should be ignored)

```
GET https://api.gbif.org/v1/species/suggest?q=Corynephorus&checklistKey=7ddf754f-d193-4cc9-b351-99906754a03b
```

Returns 20 backbone-style suggestion records (numeric `key`/`nubKey`, full
`kingdom`/`phylum`/... fields — the v1 backbone shape, not COL-XR's compact
`usage`/`classification` shape).

### 4. v1/suggest WITHOUT checklistKey

```
GET https://api.gbif.org/v1/species/suggest?q=Corynephorus
```

Also returns 20 records. Diffing both response sets after sorting by `key`
shows they are **byte-for-byte identical in content** (only ordering of the
top two entries differs between calls — an artifact of relevance
ranking/caching jitter, not of the `checklistKey` param). Same `key`, `nubKey`,
`kingdom`, `classification` for every entry in both responses.

**Conclusion: `/v1/species/suggest` silently ignores `checklistKey` — it has
no effect on the result set, confirming the appendix claim "v1/species/suggest
without checklistKey support."** No error, no warning — HTTP 200 both ways.

### 5. v2/match WITH checklistKey — Jacobaea vulgaris (second data point)

```
GET https://api.gbif.org/v2/species/match?scientificName=Jacobaea%20vulgaris&checklistKey=7ddf754f-d193-4cc9-b351-99906754a03b
```

```json
{
  "usage": { "key": "3QJJ5", "canonicalName": "Jacobaea vulgaris", "status": "ACCEPTED" },
  "classification": [ /* ... Asteraceae > Asteroideae > Senecioneae > Senecioniinae > Jacobaea > Jacobaea vulgaris */ ],
  "diagnostics": { "matchType": "EXACT", "confidence": 98 },
  "synonym": false
}
```

### 6. v2/match WITHOUT checklistKey — Jacobaea vulgaris (backbone comparison)

```
GET https://api.gbif.org/v2/species/match?scientificName=Jacobaea%20vulgaris
```

```json
{
  "key": "5388602", "canonicalName": "Jacobaea vulgaris", "status": "ACCEPTED",
  ... matchType: EXACT, confidence: 97
}
```

Again, `usage.key` differs completely (`3QJJ5` COL-XR vs `5388602` backbone),
confirming the pattern holds for a second reference taxon.

## JSON keys hostus will consume from v2/match

From the top-level response object:

- `usage.key` — the resolved usageKey (string; alphanumeric for COL-XR,
  numeric-as-string for GBIF backbone). **This is the taxon identifier hostus
  must key its cache/synonym-grouping on.**
- `usage.scientificName` is NOT present as a field name — the actual field is
  `usage.name` (full name incl. authorship) and `usage.canonicalName` (name
  without authorship). Spec/implementation should use `canonicalName` for
  display/matching and `name` for the full scientific name string.
- `usage.status` — e.g. `"ACCEPTED"` (also `"SYNONYM"` for synonym usages,
  not exercised in this probe but present in the v2 contract).
- `diagnostics.matchType` — e.g. `"EXACT"`, `"NONE"`, presumably also
  `"FUZZY"`/`"HIGHERRANK"` etc. per GBIF conventions.
- `diagnostics.confidence` — integer 0–100.
- `classification` — array of `{key, name, rank}` objects, ordered from
  highest (kingdom/domain) to the matched rank itself. This is what hostus
  will use to build/verify the kingdom=Plantae / phylum=Tracheophyta filter
  and to group synonyms under accepted taxa.
- `synonym` — boolean, top-level (not nested under diagnostics).

**Correction vs. the task's assumed key list**: there is no top-level
`usageKey` field (it's nested at `usage.key`), and there is no top-level
`scientificName` field on the match response (it's `usage.name` /
`usage.canonicalName`). The implementation/spec should be updated to reflect
the actual nesting.

## Resolved COL-XR usageKeys (fixture values)

| Taxon | COL-XR usageKey (`usage.key`) | GBIF backbone key (`usage.key`, no checklistKey) |
|---|---|---|
| *Corynephorus canescens* (L.) P.Beauv. | `YQW8` | `5290194` |
| *Jacobaea vulgaris* Gaertn. | `3QJJ5` | `5388602` |

These should be used as the expected COL-XR usageKey fixtures in hostus 2.0
tests.

## Summary

| Question | Finding |
|---|---|
| Does v2/match honor `checklistKey`? | **Yes** — different `usage.key` and classification tree per checklist, verified for 2 taxa. |
| Does v1/suggest honor `checklistKey`? | **No** — identical result sets with/without the param (verified via content diff, not just eyeballing). |
| Correct v2/match query param? | `scientificName`, not `name` (the latter silently returns `matchType: NONE`, HTTP 200 — a footgun to flag for implementation). |
| Where do the identifiers live in the JSON? | `usage.key` (not top-level `usageKey`), `usage.name`/`usage.canonicalName` (not top-level `scientificName`). |

## Verdict: 🟢 (assumption holds)

The core SP1 assumption — that GBIF v2/species/match can resolve against the
COL-XR checklist via `checklistKey` while v1/species/suggest cannot — is
**confirmed** with live API evidence for two independent reference taxa.

Two corrections to carry into the spec/implementation (non-blocking, but must
be reflected in code and docs):
1. Use query param `scientificName`, not `name`, when calling v2/match.
2. The identifier and name fields are nested under `usage.*`
   (`usage.key`, `usage.name`, `usage.canonicalName`, `usage.status`), not
   flat top-level fields as the task brief assumed.

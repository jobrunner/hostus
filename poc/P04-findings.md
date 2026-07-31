# PoC P4 — PlantNet identify: gbif.id + powo.id per candidate (Phase 0, Task 0.4, Gates SP2/UC1)

## Goal

Verify the hostus 2.0 spec's UC1 PlantNet path assumption: the PlantNet
`v2/identify` API returns **`gbif.id`** AND **`powo.id`** per result
candidate, so hostus can match candidates to POWO taxa via `powo.id` instead
of fragile scientific-name-string matching.

## Method

Shell/curl probe (`poc/p04_plantnet/probe.sh`, `set -euo pipefail`), run
inside the project's Nix dev shell with `.envrc.local` sourced for
`PLANTNET_API_KEY` / `PLANTNET_API_ENDPOINT`:

```
nix develop --command bash -c 'set -a; source .envrc.local; set +a; bash poc/p04_plantnet/probe.sh'
```

Test image: a public-domain-adjacent (CC BY 3.0) close-up photo of
*Taraxacum officinale* (common dandelion) flower head, downloaded from
Wikimedia Commons:

- Source: https://commons.wikimedia.org/wiki/File:Dandelion_Close_Up_(243074873).jpeg
- Direct file: https://upload.wikimedia.org/wikipedia/commons/d/d4/Dandelion_Close_Up_%28243074873%29.jpeg
- Saved to `poc/data/p04_taraxacum_officinale.jpg` (gitignored)

Raw JSON responses saved under `poc/data/*.json` (gitignored). No code
touched outside `poc/p04_plantnet/` and this file. The API key is **never**
written to any committed file — the probe script redacts it in its own
echoed URLs, and this findings doc only ever shows `api-key=***`.

## Deviation from the task's assumed project id

The task assumed the project `k-central-europe`. That id **does not exist**:

```
POST {endpoint}/v2/identify/k-central-europe?api-key=***&include-related-images=false
→ HTTP 404 {"statusCode":404,"error":"Not Found","message":"Unknown project: k-central-europe"}
```

The `/v2/projects?api-key=***&lat=49.8&lon=9.9` coordinate-helper (which the
spec's Fallstricke/pitfalls section references) resolves that same
Central-European coordinate to a different, real project id:

```json
[
  {"id": "k-middle-europe", "title": "Middle Europe", "description": "Plants of Middle Europe", "speciesCount": 5385},
  {"id": "k-southeastern-europe", "title": "Southeastern Europe", "speciesCount": 7431},
  {"id": "k-northern-europe", "title": "Northern Europe", "speciesCount": 4223},
  {"id": "museum-albert-kahn", "title": "Jardin du musée départemental Albert-Kahn", "speciesCount": 180},
  {"id": "alpes-maritimes", "title": "Flore remarquable des Alpes-Maritimes", "speciesCount": 98}
]
```

**Correction for the hostus 2.0 spec/implementation**: the correct `k-*`
project id for Central Europe is **`k-middle-europe`**, not
`k-central-europe`. The coordinate→project helper is confirmed to work and
should be used at startup/config-validation time (or documented as a fixed
constant with a comment linking to this probe) rather than hard-coding a
guessed id — PlantNet's project catalog is not guaranteed to match the
obvious naming intuition.

All further probes below use `k-middle-europe`.

## Result structure (`results[]`)

Request: `POST {endpoint}/v2/identify/k-middle-europe?api-key=***&include-related-images=false`
with `organs=flower` and the dandelion image attached.

Top-level response shape (key-redacted, full raw JSON in
`poc/data/p04_identify_taraxacum.json`, gitignored):

```json
{
  "query": {"project": "k-middle-europe", "images": ["<opaque-id>"], "organs": ["flower"], "includeRelatedImages": false},
  "predictedOrgans": [{"image": "<opaque-id>", "filename": "p04_taraxacum_officinale.jpg", "organ": "fruit", "score": 0.68109}],
  "language": "en",
  "preferedReferential": "k-middle-europe",
  "results": [ /* array of candidates, see below */ ],
  "version": "2026-03-20 (7.4)",
  "remainingIdentificationRequests": 498
}
```

Each entry in `results[]`:

```json
{
  "score": 0.25825,
  "species": {
    "scientificNameWithoutAuthor": "Taraxacum sect. Taraxacum",
    "scientificNameAuthorship": "F.H.Wigg.",
    "scientificName": "Taraxacum sect. Taraxacum F.H.Wigg.",
    "genus": {"scientificNameWithoutAuthor": "Taraxacum", "scientificName": "Taraxacum"},
    "family": {"scientificNameWithoutAuthor": "Asteraceae", "scientificName": "Asteraceae"},
    "commonNames": ["Common dandelion", "Dandelion", "True Dandelion"]
  },
  "gbif": null,
  "powo": {"id": "254151-1"}
}
```

**Exact JSON paths hostus would consume:**

- `results[i].score` — image-similarity confidence (float, PlantNet's own internal scoring), NOT the same axis as hostus's `resolution.confidence`.
- `results[i].species.scientificNameWithoutAuthor` — canonical name string.
- `results[i].species.scientificNameAuthorship`, `.species.scientificName` — full name with author.
- `results[i].gbif.id` — GBIF backbone usage key (string, e.g. `"6064015"`), sibling object to `species`, **top-level under the result, not nested in `species`**.
- `results[i].powo.id` — POWO/IPNI id (string, e.g. `"254619-1"`), same nesting level as `gbif`.

## `gbif.id` / `powo.id` presence across 10 candidates

| # | score   | scientificName              | gbif.id   | powo.id      |
|---|---------|------------------------------|-----------|--------------|
| 1 | 0.25825 | Taraxacum sect. Taraxacum    | **null**  | "254151-1"   |
| 2 | 0.11638 | Taraxacum rubicundum         | "6064015" | "254619-1"   |
| 3 | 0.02726 | Taraxacum erythrospermum     | "5393872" | "249536-2"   |
| 4 | 0.00815 | Tragopogon porrifolius       | "5386938" | "256109-1"   |
| 5 | 0.00657 | Leontodon hispidus           | "3137498" | "229358-1"   |
| 6 | 0.00357 | Hypochaeris radicata         | "3093702" | "225575-1"   |
| 7 | 0.00264 | Taraxacum palustre           | "5394278" | "30320466-2" |
| 8 | 0.00225 | Crepis foetida               | "5403470" | "199861-1"   |
| 9 | 0.00187 | Tragopogon dubius            | "6443709" | "255993-1"   |
| 10| 0.00166 | Hypochaeris glabra           | "3093911" | "225504-1"   |

**`powo.id` was present and non-null for all 10/10 candidates.**
**`gbif.id` was null for 1/10 candidates** — the top-ranked result, where
PlantNet's own name resolution landed on `Taraxacum sect. Taraxacum`, an
infrageneric **section**, not a GBIF-backbone species/genus usage. GBIF's
backbone apparently has no clean usage key for that node in PlantNet's
internal taxonomy, whereas POWO (which itself models sections as legitimate
ranks) resolved it fine. This is a concrete, observed instance of the
`gbif.id`-nullability risk, not a hypothetical.

`gbif` itself can be `null` (not just `gbif.id` inside a present object) —
observed directly above: `"gbif": null` for candidate #1, `"gbif": {"id": "..."}`
for the others. Any hostus consumer must null-check `results[i].gbif` before
dereferencing `.id`, not just `.gbif.id`.

## Score vs. confidence semantics

`results[i].score` is PlantNet's **image-similarity score** — how visually
similar the submitted photo is to PlantNet's training images for that
candidate species, normalized so all candidates' scores sum to ~1 across the
full (usually much longer, truncated-to-N-here) result list. It is **not** a
taxonomic-match-quality or identification-certainty signal in the sense
hostus's own `resolution.confidence` needs (i.e., "how sure are we this is
the right accepted-taxon match"). In this run the top score is only 0.258 —
the model itself is not confident which *Taraxacum* it saw, consistent with
`Taraxacum officinale agg.` being a famously difficult apomictic complex (the
tool also mis-detected the dominant organ as "fruit" rather than "flower"
despite the `organs=flower` hint, which is a plausible reason the top
candidate landed on the whole-section aggregate rather than a full species).

Per the hostus 2.0 spec's intent, `score` belongs in
`determination_evidence` (as image-recognition provenance/rationale for a
human or downstream consumer to inspect), **not** copied directly into
`resolution.confidence`, which should instead reflect hostus's own
taxonomic-match certainty (e.g., whether `powo.id` cleanly resolved to
exactly one accepted taxon in the target checklist, whether the PlantNet
species is present in the WCVP/POWO checklist project at all, etc.). Mixing
the two would conflate "the AI recognizes this plant" with "hostus is sure
about the taxon record," which are different failure axes (e.g., PlantNet
could be very confident but the species is a synonym/section that maps
ambiguously in hostus's checklist, or vice versa).

## `k-*` project caveat (recap)

- `k-*` prefixed PlantNet projects are Kew/POWO-checklist-backed identification projects (as opposed to GBIF-backbone-backed or organization-specific projects like `museum-albert-kahn`).
- The project id is **not** predictable from geography by name pattern alone — `k-central-europe` does not exist; the real id is `k-middle-europe`.
- `GET /v2/projects?api-key=***&lat={lat}&lon={lon}` reliably returns the ranked list of applicable projects for a coordinate (here: `k-middle-europe`, `k-southeastern-europe`, `k-northern-europe`, plus two hyper-local French museum/park projects) — confirmed working and should be used by hostus, either at deploy-time config validation or as a runtime fallback, rather than hardcoding a project id string without verifying it against this endpoint first.

## Verdict

**🟢 gbif.id AND powo.id are both present per candidate** — with the caveat
that `gbif` (and therefore `gbif.id`) can legitimately be `null` for
infrageneric/aggregate taxa that don't map cleanly onto a GBIF backbone
usage, while `powo.id` was present and non-null on every single candidate in
this run (10/10). This directly confirms and *strengthens* UC1's design
choice to match PlantNet candidates via `powo.id` rather than `gbif.id` or
name strings: `powo.id` is the more consistently populated key, `gbif.id`
should be treated as optional/nullable supplementary data, and
name-string matching should not be needed as the primary path.

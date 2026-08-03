# PoC P8 — Wisskirchen concept relations (CDM portal), Phase 0, Task 0.8, Gate SP5

## Goal

This is the highest-risk PoC in the investigation (architecture spec: "P8 …
das größte Risiko"). It answers one question for SP5 (`POST /v1/translate`,
UC6): is the Wisskirchen (1998) concept-relation data — the typed `sec.`
relationships between taxon concepts from different floras/checklists that
`sec_inference`/`/translate` needs — **machine-retrievable** via the CDM
portal's REST API, or only available as human-readable HTML / a flat CSV
without the relations?

## Method

`poc/p08_cdm/probe.sh` (`set -euo pipefail`), run via
`nix develop -c bash poc/p08_cdm/probe.sh`. curl-only, no auth. Raw
responses saved under `poc/data/p08_*.{html,json}`.

**User-Agent note:** the Drupal portal's WAF returns HTTP 403 for a
descriptive bot-style User-Agent (the pattern used in every other PoC in
this investigation, e.g. `hostus-poc-P8/0.1 (...)`), but HTTP 200 for an
ordinary browser UA string. The CDM server at `api.cybertaxonomy.org` does
not appear to filter by User-Agent either way. The probe script uses a
browser UA throughout for reproducibility; a production integration would
need to clarify with BGBM whether identifying itself as a bot/crawler is
acceptable, or whether it must present as a browser.

## Discovery: the actual CDM REST API base

The quellenregister's guessed base (`api.cybertaxonomy.org` alone, or a
`rotelisten_flora_deutschland`-named path) is close but not quite right.
The real base was found by grepping the portal's rendered HTML for
`api.cybertaxonomy.org` links:

```
https://api.cybertaxonomy.org/rl_standardliste/
```

The CDM *datasource name* for this portal is `rl_standardliste`, not
`rotelisten_flora_deutschland` (that string is only the Drupal portal's
URL slug). The root URL redirects to a Springfox/Swagger landing page
titled "CDM remote API", confirming this is a full **CDM Server / EDIT
platform REST API** (Jetty 9.4, `cdmlib-remote` servlet), not a
portal-only proxy.

## Endpoints found, and what they return

| Endpoint | Returns JSON? | Content |
|---|---|---|
| `GET /rl_standardliste/` | Swagger landing page (HTML) | confirms API exists, links to `doc/` |
| `GET /rl_standardliste/classification` | ✅ JSON | list of all 17 classifications (checklists) in this dataset, each with uuid + titleCache, e.g. `4ea7fe85-...` = "WISSKIRCHEN & HAEUPLER, Standardliste der Farn- und Blütenpflanzen Deutschlands. 1998" |
| `GET /rl_standardliste/taxon` | ✅ JSON | flat paged list of **all 51,466 taxon concepts** in the dataset, each `titleCache` already showing its `sec.` reference inline, e.g. `"Abies alba Mill. sec. Wisskirchen & Haeupler 1998"` |
| `GET /rl_standardliste/taxon/{uuid}` | ✅ JSON | full taxon-concept record incl. a structured **`secSource`** object (citation with `titleCache`, `uuid`, `datePublished`, etc. — a fully resolved bibliographic reference, not just a string) |
| `GET /rl_standardliste/taxon/{uuid}/synonyms` | ✅ JSON | synonym relations for that concept |
| `GET /rl_standardliste/taxon/{uuid}/relationsToThisTaxon`, `.../relationsFromThisTaxon`, `.../taxonRelations` | ✅ JSON (stub) | list of `TaxonRelationship` objects, but **only** `uuid`/`created`/`updated`/`doubtful` — no relation type, no partner taxon |
| `GET /rl_standardliste/portal/taxon/{uuid}/taxonRelationships` | ✅ JSON (**expanded**) | full `TaxonRelationship` DTOs including a typed **`type`** object (`representation_L10n`, e.g. `"Congruent to"`, `"is misapplied name for"`; `symbol`, e.g. `"≜"`; `conceptRelationship: true/false` flag distinguishing genuine concept relations from misapplied-name relations) |
| `GET /rl_standardliste/checklist` / `.../checklist/export` (doc pages) | HTML docs | documents a flat "Checklist Export API": one row per accepted taxon with `scientificName`, `author`, `rank`, `parentUuid`, a single `taxonConceptID` — **no relation types, no partner sec.** by design |
| `GET /rl_standardliste/checklist/export?classification={uuid}&pageSize=…&pageNumber=…` | ⚠️ JSON but broken in practice | returns correct `count` (e.g. 41 records for the Wisskirchen classification test slice) and pagination metadata, but `records: []` was **empty on every page we tried** — this endpoint did not actually deliver usable rows during our test, independent of the structural limitation already documented above |

## The critical test: typed concept relations + sec., end to end

Reference concept: **`Abies alba Mill. sec. Wisskirchen & Haeupler 1998`**
(uuid `872088a4-95f4-472c-ae79-a29028bb3fbf`).

1. `GET /taxon/872088a4-.../` → `secSource.citation.titleCache` =
   `"Wisskirchen & Haeupler 1998: Standardliste der Farn- und Blütenpflanzen
   Deutschlands"`. The `sec.` reference is a fully structured, machine
   parseable object, not free text.

2. `GET /portal/taxon/872088a4-.../taxonRelationships` → **10 typed
   relations**, e.g.:

   ```json
   {
     "uuid": "7f0a3c0c-fcd0-48eb-b6f8-802745c647aa",
     "type": {
       "representation_L10n": "Congruent to",
       "symbol": "≜",
       "conceptRelationship": true
     },
     "class": "TaxonRelationship"
   }
   ```

   9 of the 10 relations are typed `"Congruent to"` (`conceptRelationship:
   true`); one is typed `"is misapplied name for"` (`conceptRelationship:
   false`) — i.e. the API distinguishes genuine Berendsohn-model concept
   relations (congruent/includes/excludes/overlaps) from misapplied-name
   relations, exactly the semantic distinction SP5's `sec_inference` needs.

3. **Gap found:** this endpoint gives the relation type + a relationship
   uuid, but does **not** embed the *partner* taxon concept (the other side
   of the relation) directly — likely a deliberate Jackson
   anti-recursion omission in this generic serializer, since a full nested
   `Taxon` object could recurse. No dedicated `/taxonRelationship/{uuid}`
   detail endpoint exists (`404`, unrouted).

4. **Resolved anyway, fully programmatically:** every other "Abies alba"
   concept (i.e. every concept sharing the same accepted name, found via
   the flat `/taxon` list, filterable by `titleCache` substring) that has
   exactly one `taxonRelationships` entry lets you match its relationship
   `uuid` back against the reference concept's list. We verified this for
   three siblings and it worked cleanly — 1:1 uuid matches, no ambiguity:

   | Sibling concept | uuid | matching relationship uuid |
   |---|---|---|
   | `... sec. EHRENDORFER: Liste der Gefäßpflanzen Mitteleuropas` | `b0d35335-...` | `fc35bfb1-...` |
   | `... sec. Greuter & al.: Med-Checklist` | `7a63f215-...` | `7f0a3c0c-...` |
   | `... sec. HEGI: Illustrierte Flora von Mitteleuropa` | `61c2bc4f-...` | `fe89d0ca-...` |

   So the full pair — concept A (with its `sec.`), relation type, concept B
   (with its `sec.`) — **is** reconstructable purely via the REST API, but
   requires a **two-hop join** (fetch relation types per concept, then
   cross-reference relationship uuids across name-sharing candidate
   concepts) rather than a single call returning the fully resolved edge.
   For a batch import (which is what SP5 needs, not a live per-request
   lookup) this is a straightforward, entirely scriptable graph-building
   job over the ~51k taxon concepts — not a blocker, but real engineering
   effort beyond "just call one endpoint."

## Licensing status

**No explicit license found anywhere probed** — not on the Drupal portal
pages, not on the CDM API landing/doc pages, not in any JSON response
(no `license`/`rights` field on any entity). This confirms and does not
resolve the ⚠️ already flagged in `docs/research/quellenregister.md`. Given
that the underlying data is `sec.`-attributed, copyrighted secondary literature
(Wisskirchen & Haeupler 1998, plus a dozen other cited floras/checklists
like HEGI, Flora Europaea, Med-Checklist, Rothmaler), redistributing the
*derived concept-relation graph* inside hostus's own API without written
clarification from BGBM/EDIT is a real legal risk, independent of the
technical retrievability question this PoC answers.

## Verdict: ⚠️ (partially — API exists, typed relations + sec. ARE machine-retrievable, but with real caveats)

Rationale:
- 🟢 **Structurally**, this is about as good as it gets: a real REST API
  (not scraping), JSON throughout, a fully structured `sec.`/citation
  object per concept, and genuine typed concept-relationship data
  (`Congruent to` etc., with a `conceptRelationship` flag separating true
  concept relations from misapplied-name relations) — exactly the
  Berendsohn (1995) model the spec assumes.
- ⚠️ But: (1) the relation payload requires a two-hop
  cross-reference join to resolve the partner concept — not a single-call
  lookup; (2) the one endpoint that *is* documented as a purpose-built flat
  export (`checklist/export`) returned empty result rows in every page we
  tried, so it cannot currently be relied on as a shortcut; (3) the license
  is completely unaddressed, which blocks redistribution regardless of
  technical feasibility; (4) this was only validated against one dataset/
  reference genus (Abies) — coverage across all ~51k concepts and rarer taxa
  (which matter more for a Red-List use case) is unverified at this scale.

## SP5 fallback / recommendation

Given the verdict is not a clean 🟢, but also not a 🔴:

1. **Preferred path (do this first):** treat this as *technically* feasible
   and build the SP5 import as a **batch/offline ETL job**, not a live
   per-request API call: page through all `/taxon` concepts, fetch
   `/portal/taxon/{uuid}/taxonRelationships` for each, and reconstruct the
   concept graph via the relationship-uuid cross-reference method
   demonstrated above. This is straightforward given the dataset is finite
   (~51k concepts) and stable, and it sidesteps having hostus depend on the
   CDM API's live latency/availability at request time.
2. **Before writing a single line of that ETL job:** get written licensing
   clarification from BGBM/EDIT (the spec's own fallback plan, §UC6/SP5,
   already anticipates this). Do not ship a public `/translate` endpoint
   built on this data without it — the underlying `sec.` sources are
   copyrighted flora literature, and "no license stated" is not permission.
3. **If licensing clarification is denied or stalls:** fall back to the
   spec's already-documented Plan B — manual/semi-automatic curation of
   concept relations from the printed *Standardliste* (Wisskirchen &
   Haeupler 1998) for a reduced taxon scope, and rescope SP5/`/translate`
   to that smaller, manually-curated set rather than the full ~51k-concept
   graph.
4. Either way, budget real engineering time for the two-hop
   relationship-resolution join — it is not a "call one endpoint and get
   an edge list" integration, contrary to what a first glance at
   `/taxon/{uuid}/taxonRelationships` might suggest (that undocumented,
   partial endpoint alone is a trap; the `portal/` prefixed variant plus
   cross-referencing is required for the typed relation, and even that
   still needs the sibling-matching step for the partner taxon).

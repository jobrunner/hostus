# Pipelines (xlsx/sqlite/REST → canonical CSV)

Three families of pipelines live here: three **name-list** pipelines
(GermanSL / EuroSL / FloraVeg, documented in their own section below) that
acquire additional backbone/checklist sources for local evaluation — the
EuroSL pipeline is also how **Euro+Med** is provided (see its section:
EuroSL.sqlite *is* the Euro+Med checklist; the former standalone `euromed`
REST-crawl pipeline was retired) — and one **xref bridge-hub** pipeline
(`wikidata`, documented
last) that harvests cross-references to other taxonomic authorities via
Wikidata, plus one **concept + relation** pipeline (`cdm`, documented at the
very end) that harvests the CDM `rl_standardliste` taxonomic-concept graph
for SP5's `/v1/translate`. All of them follow the same shape: pinned source →
download-or-reuse-cache → convert → canonical CSV in `output/`
(gitignored) → printed summary.

The three **trait** pipelines that used to live here (EIVE / Tichý / Midolo)
were removed and transferred to situs (Teilprojekt 2) along with the whole
hostus traits subsystem — see CHANGELOG.md "Traits-Subsystem entfernt" and
docs/superpowers/specs/2026-08-27-hostus-namensraum-redesign-design.md
Abschnitt 8.

**Language:** this collected README is English throughout and stays that way.
CLAUDE.md pins only the top-level `README.md`/`README.dev.md` to German, so a
per-pipeline README may be German — `pipelines/cdm/README.md` is. Both are
consistent with the project rules; do not "fix" either one to match the other.

## Name-list pipelines

Three further pipelines (`germansl`, `eurosl`, `floraveg`) acquire
additional backbone/checklist sources. (`eurosl` doubles as the Euro+Med
source — see its section; the former standalone `euromed` REST pipeline is
retired.) These are **the sources with no
findable license** (`redistribution: unknown`/`restricted` in `dataset.yaml`
terms — see Task 1's redistribution gate): the data is publicly offered by
its maintainers, but nobody has stated terms permitting redistribution.
Per the 2026-08-01 owner decision (`docs/superpowers/plans/2026-08-01-reality-check.md`),
that licenses **local, private evaluation** under German scientific-research
privilege (§60c/§87c UrhG) even though it does not license redistribution.

**These three pipelines are for local evaluation only.** Their `output/*.csv`
must never be exported in a served bundle — `ExportBundle` (Task 1's gate)
refuses by default to include any source not marked `redistribution:
allowed`, and none of these three are. Do not add them to `dataset.yaml` as
`allowed` sources; do not point any served endpoint at their `output/`.

### Canonical CSV contract (name lists)

Identical shape for all three sources, pipe-delimited (`|`), matching the
WCVP reader convention:

- Header: `taxon|rank|status|accepted_taxon|source_id`
- `taxon` — the scientific name string as the source provides it, as a bare
  canonical (GermanSL/EuroSL supply a separate name column; FloraVeg's table
  is already bare). (The retired Euro+Med REST pipeline instead emitted an
  author-laden `titleCache` with the `sec.` tail stripped — one reason it was
  retired in favour of EuroSL's structured `TaxonName`.)
- `rank` — as provided by the source, in the source's own vocabulary (e.g.
  GermanSL's `SPE`/`GAT`/`FAM`, EuroSL's `Species`/`Genus`/`Family`). Not
  normalized to a shared enum across sources — callers needing a uniform
  rank scheme must map per `source_id`. Empty when the source doesn't
  provide one at all (FloraVeg's Life_form table).
- `status` — `accepted` or `synonym` (EuroSL also distinguishes
  `synonymobjective`, kept as its own value rather than folded into
  `synonym`), lowercased. `accepted` when the source has no synonymy
  concept at all (FloraVeg).
- `accepted_taxon` — the accepted name string, only for `status != accepted`
  rows. Empty for accepted rows, and empty (not guessed) wherever the
  source doesn't expose the link (FloraVeg).
- `source_id` — the source's own stable identifier (GermanSL
  `TaxonUsageID`, EuroSL `TaxonUsageID`, FloraVeg `SeqID`).
- Empty field = not provided by the source (never guessed).

### Running a name-list pipeline

```bash
nix develop -c bash pipelines/germansl/build.sh
nix develop -c bash pipelines/eurosl/build.sh   # also the Euro+Med source (EuroSL.sqlite = Euro+Med)
nix develop -c bash pipelines/floraveg/build.sh
```

### GermanSL 1.5.5 (germansl.infinitenature.org)

Source: `GermanSL_1.5.5.zip` (the "GermanSL 1.5" download bundle), which
contains both a TURBOVEG export (`species.dbf`/`ecodbase.dbf`) and a
spreadsheet export (`GermanSL1.5.5.xlsx`, sheet `TCS`). The xlsx was chosen
per the task brief. It ships with a broken `<dimension ref="A1"/>` in its
sheet XML that makes openpyxl's read-only mode silently truncate every row
to column A (and non-read-only mode fails outright on a missing drawing
relationship) — `convert.py` parses the OOXML parts (shared strings +
sheet XML) directly to work around this.

```
version=1.5.5
rows=26129 taxa=26129 accepted=14656 synonym=11473
ranks=SPE:18231,SSP:2525,GAT:2072,VAR:1625,FAM:657,AGG:581,ORD:135,FOR:100,
      SEC:34,KLA:26,SER:25,ORA:21,CL5:19,ABT:17,SGE:13,SGR:12,SFA:7,UAB:6,
      SSE:5,CL1:5,AG3:3,ROOT:2,AG1:2,CL4:2,CL3:2,AG2:2
```

### EuroSL (eurosl.infinitenature.org)

Source: the single "latest version sqlite file" linked from the downloads
page (no versioned filename; the file's own `Version` table records its
build timestamp). Table `EuroPlusMed.Plantae` already has exactly the
canonical columns (`TaxonUsageID`, `TaxonName`, `TaxonRank`, `status`,
`TaxonConcept`), read directly via Python's stdlib `sqlite3`.

**This is the Euro+Med checklist, structured.** The single data table is
literally named `EuroPlusMed.Plantae`, and every row's `AccordingTo` column
reads `api.cybertaxonomy.org/euromed`. EuroSL is the *structured* view of the
same Euro+Med CDM dataset — bare `TaxonName`, `TaxonRank`, and an
accepted-name link — so hostus uses this pipeline as its Euro+Med source. The
former standalone `euromed` pipeline crawled the flat `/euromed/taxon` REST
listing of the same dataset, which exposes only an author-laden `titleCache`
with **no rank and no accepted-name link at all** (measured:
`docs/research/reality-check.md` M6 records the euromed canonical CSV as 0
rows with a rank and 0 with an accepted_taxon, against EuroSL's full rank plus
85 396 accepted-name links). So on every servable dimension EuroSL strictly
dominates and the flat-REST pipeline was retired as a degraded view (they are
different-vintage snapshots of the one CDM dataset, not a proven strict
superset — but the surplus REST rows are exactly the rank-less, link-less ones
that were unusable). See the retired section below.

```
version=Sun Nov  3 11:31:01 2024
rows=139039 taxa=139039
statuses=synonym:53842,accepted:53643,synonymobjective:31554
ranks=Species:88083,Subspecies:29330,Variety:11821,Genus:5242,Form:2262,
      Family:488,Unranked (infraspecific):296,Species Aggregate:287,
      Subvariety:252,Section:238,Coll. species:155,Order:106,Tribe:90,
      Subgenus:87,Subform:74,Proles:72,Subfamily:27,Subclass:24,Race:22,
      Unranked (infrageneric):19,Class:16,Grex (infraspec.):15,
      Superorder:12,Subsection bot.:8,Subdivision:4,Phylum:3,Division:2,
      Convar:2,Root:1,Suprageneric Taxon:1
```

### FloraVeg.EU (floraveg.eu)

Source: `Life_form.xlsx`. FloraVeg's `/download/` page offers only
per-topic Excel trait tables (life form, dispersal, disturbance indicator
values, parasitism, vegetation units, EUNIS habitats, ...) or the ESy expert
system (explicitly out of scope per the task brief) — there is no dedicated
taxon-checklist export. `Life_form.xlsx` was chosen as the simplest,
most complete per-taxon table (one row per taxon, a `SeqID`, and Raunkiaer
life-form flag columns not carried into this canonical list). It has no
rank or synonymy columns, so `rank` is always empty and `status` is always
`accepted`.

```
sheet=FloraVegEU-2023-01-03
rows=16402 taxa=16402
word_counts=2:15677,3:695,4:30   # binomial vs. (mostly) infraspecific names
```

### Euro+Med — CDM REST probe (api.cybertaxonomy.org/euromed) — RETIRED

> **Retired.** Euro+Med is now sourced from the **EuroSL** pipeline above:
> `EuroSL.sqlite`'s only data table is `EuroPlusMed.Plantae` (AccordingTo =
> `api.cybertaxonomy.org/euromed` on every row), i.e. the same CDM dataset,
> but structured — bare name, rank and accepted link. The flat `/euromed/
> taxon` listing this pipeline crawled carries only an author-laden
> `titleCache` with **no rank and no accepted-name link** (confirmed:
> `nameUsage`/`concept`/`conceptId` are all `null` on the record; and
> `docs/research/reality-check.md` M6 measures the euromed canonical CSV as
> 0 rows with a rank / 0 with an accepted_taxon, versus EuroSL's full rank
> plus 85 396 accepted links). So it contributed nothing servable that EuroSL
> does not, and the `build.sh`/`crawl.py` were removed; the probe finding is
> kept below for the record.

R1 found no bulk export for Euro+Med. This pipeline
re-probed the CDM REST API (the pattern PoC P8 validated against the
Wisskirchen instance) before giving up.

**Probe result — same failure pattern P8 found, on a different instance:**

- `GET /euromed/classification` → works: one classification, "Euro+Med
  2018" (`314a68f9-8449-495a-91c2-92fde8bcf344`).
- `GET /euromed/checklist/export?classification=<uuid>&pageSize=…` → reports
  a correct `count` (64815 for this classification) but `records: []` on
  every page tried. Broken exactly like P08's finding for
  `rl_standardliste/checklist/export`.
- `GET /euromed/checklist/exportCSV?classification=<uuid>` → HTTP 302
  redirecting to itself, 0 bytes. Also broken.
- `GET /euromed/taxon?pageSize=N&pageIndex=P` → **works.** Verified with
  distinct `pageIndex` values returning distinct, correctly-offset records.
  (The query parameter is `pageIndex`, not `pageNumber` — the latter is
  silently ignored and always serves page 0, which looks exactly like
  "pagination is broken" until the right parameter name is found.)
  167912 taxon concepts total at `pageSize=500` → 336 pages.

**Verdict: obtainable — but only via the flat `/taxon` listing, not the
documented bulk-export endpoints**, and with two real gaps relative to
GermanSL/EuroSL:

1. No `rank` field on this listing at all (only on the CDM name object,
   one extra HTTP call per record — infeasible at 167912 records in the
   time budget). Left empty, not guessed.
2. No accepted-name resolution for `synonym` rows without either a
   per-accepted-taxon `/synonyms` call (~65k extra requests) or PoC P8's
   two-hop relationship cross-reference walk. Left empty, not guessed.

`taxon` is derived from `titleCache` by stripping the `sec.`/`syn. sec.`
citation tail (regex); the author string is *not* further separated out
(no separate author field exists on this listing, unlike GermanSL/EuroSL).
`status` comes from the CDM `class` field (`Taxon` → accepted, `Synonym` →
synonym) — reliable and free, unlike rank/accepted-name.

```
total_count=167912 pages_available=336 page_size=500
rows=167888 taxa=156867
statuses=synonym:95846,accepted:72042
```

336 distinct pages (pageIndex 0..335) fetched, confirmed via
`pipelines/euromed/.cache/pages_fetched.log` (336 unique entries) — the
summary's printed `pages_fetched=337/336` is a cosmetic off-by-one in the
final log line (the loop counter is incremented once more before the break
that ends the crawl), not a double-fetched page or a data gap.

Wall-clock: ~20 minutes for the full crawl (336 sequential HTTP requests,
no concurrency; each 500-record page already takes ~5-6s given how verbose
the JSON is, which alone spaces requests out without an additional sleep).

**This corrects PoC P5's 🔴 "not obtainable" verdict for Euro+Med** — P5
tested for a bulk export (a ColDP/CDM archive via GBIF or ChecklistBank) and
found none; it did not test the CDM REST API directly. The REST API *is* a
viable path and yields a complete harvest of all 167912 taxon concepts, via
the flat paged listing rather than a single-file download. See
`poc/GATE.md` for the corrected P5 entry.

**What the REST records carry vs. what they don't** (observed, not assumed):
they carry a name+author+citation string (`titleCache`) and an
accepted/synonym flag (`class`) for every one of the 167912 concepts.
They do **not** carry, on this listing: a separate rank field, a resolved
accepted-name link for synonyms, distribution data, or parent/classification
links (`/taxon` returns flat records with no `IsChildTaxonOf`-equivalent
field) — a full ColDP-style export would carry all of these; this REST
harvest is a name-and-status list only, matching exactly the canonical
contract's scope and no further.

## Wikidata xref bridge-hub pipeline

`pipelines/wikidata/` harvests cross-references from Wikidata for hostus
2.0 SP4: our concept index already carries exactly one outbound join key,
`xref.powo` (the bare IPNI id from WCVP's `dynamicproperties.powoid`, e.g.
`396681-1`). Wikidata items carry that same id under **both** P961 (IPNI,
already bare) and P5037 (POWO, stored as a
`urn:lsid:ipni.org:names:...`-prefixed LSID) — and, on the same item,
identifiers for several other authorities. So Wikidata is used as a
**bridge hub**: join a concept to a Wikidata item by IPNI/POWO id, then
read every other authority off that item. See `poc/P07-findings.md` for
the PoC that verified these property numbers against live entity data.

### Property → `xref.authority` mapping

| Wikidata property | Label | maps to `authority` | Notes |
|---|---|---|---|
| P961 | IPNI plant ID | *(join key, not emitted as its own row)* | Already bare, e.g. `396681-1`. |
| P5037 | POWO ID | *(join key, not emitted as its own row)* | Stored as an LSID, `urn:lsid:ipni.org:names:396681-1` — the `urn:lsid:ipni.org:names:` prefix is stripped before use. |
| P846 | GBIF-species-ID (legacy) | `gbif` | Superseded by P14607; still queried and used as a fallback. |
| P14607 | GBIF taxon ID (new) | `gbif` | Preferred over P846 when both are present (PoC P7: a Wikidata migration from P846 → P14607 is in progress; querying only the new property would silently miss data still under the old one). |
| P10585 | Catalogue of Life ID | `colxr` | |
| P12380 | Euro+Med PlantBase taxon ID | `euromed` | |
| P12100 | FloraVeg.EU taxon ID | `floraveg` | **A NAME STRING, not an opaque id** (e.g. `"Corynephorus canescens"`) — see caveat below. |
| P7715 | World Flora Online ID | `wfo` | Not in the original spec's property list at all; discovered via PoC P7's `wbsearchentities` search. |
| P3151 | iNaturalist taxon ID | `inat` | UC2's ceiling metric — see task report for the measured fill rate. |
| *(the QID itself)* | — | `wikidata` | Always emitted for every item, so a consumer can resolve back to the source item. |

**P12100/FloraVeg.EU is a name, not an identifier.** Every other row in
this CSV carries an opaque id that is meaningless outside its issuing
authority's own database key space. P12100's value is the taxon's
*scientific name string* as FloraVeg.EU spells it — useful for a
best-effort cross-check or display, but **not safe to treat as a joinable
id** the way `gbif`/`colxr`/`wfo`/`inat` are. A consumer joining on
`authority=floraveg` must know it is joining on a name, with all the
usual name-matching caveats (synonymy, orthographic variants), not an id
lookup.

### Canonical CSV contract (xref)

Pipe-delimited, one row per (Wikidata item × authority):

```
join_authority|join_id|authority|ext_id|wikidata_qid
```

- `join_authority` is always `powo` — the key hostus's `xref` table
  already carries.
- `join_id` is the bare IPNI id: taken directly from P961, or from P5037
  with the LSID prefix stripped, so the ingest side never has to
  re-derive either. **When both P961 and stripped-P5037 are present and
  disagree**, `common.resolve_join_id` emits **whichever one is actually
  present in `xref.powo`** (checked when a joinable-id set is supplied —
  see the joinable-subset section below); P961 is used as a fallback only
  when both sides match, neither does, or no joinable-id set is supplied
  at all. Every disagreement is counted and printed by `convert.py`,
  never silently dropped; see the task report's "fix round 1" section for
  why this rule exists — an earlier version of this pipeline picked P961
  unconditionally, which emitted a **non-matching, dead `join_id`** for
  4.07% of the joinable population whenever P961 itself didn't happen to
  be the side that matched.
- `authority` — see the mapping table above.
- `ext_id` — the id (or, for `floraveg`, the name) in that authority's own
  key space.
- `wikidata_qid` — the source Wikidata item, repeated on every row (so a
  row is self-contained without a join back to a `wikidata`-authority row
  in the same file).

### Query + paging strategy

WDQS enforces a 60s query timeout. Scoping to taxa (`wdt:P31 wd:Q16521`)
that also carry P961 or P5037, live-measured while building this
pipeline: a single query combining that P31 type-join with several
`OPTIONAL` property lookups reliably times out (504/502/truncated JSON)
above a few hundred rows per page — the P31=Q16521 join alone already
costs ~30–45s regardless of page size, because Blazegraph must
materialize the intersection of two large sets before applying
`LIMIT`/`OFFSET`.

So the harvest is **two-phase** (`pipelines/wikidata/crawl.py`):

1. **Seed scan** — page P961 and P5037 *independently*, with no join and
   no `OPTIONAL`s at all: `SELECT ?item ?v WHERE { ?item wdt:P961 ?v }
   LIMIT 20000 OFFSET N`. This single-predicate shape consistently
   completes in 3–25s per page even at `N` in the hundreds of thousands,
   because WDQS can stream one predicate's index range without a join.
   The union of both scans is the seed set — every item we could
   possibly join.
2. **VALUES-batch enrichment** — re-visit the seed set in batches of 500
   QIDs via `VALUES ?item { wd:Q1 wd:Q2 ... }` with `OPTIONAL` for each of
   the other 7 properties. This is cheap (1–3s per batch) because each
   `OPTIONAL` is now a point-lookup against a small, explicit item list
   rather than a join against the whole graph.

**The live P31=Q16521 type filter is deliberately dropped from phase 1**
for cost reasons, on the strength of a direct measurement: a COUNT of
`?item wdt:P961 ?v ; wdt:P31 wd:Q16521` returned 907654, against a plain
`?item wdt:P961 ?v` COUNT of 908799 — i.e. 99.87% of P961-holders are
already typed as Wikidata's generic `taxon` class. The ~0.13% that are
IPNI/POWO-bearing but not typed as a taxon (a handful of disambiguation
edge cases) are accepted as noise rather than paying a ~30–45s join tax on
every page of a multi-hour crawl. This is a pipeline engineering
trade-off, not a hostus correctness requirement — hostus itself never
queries WDQS; it only reads the canonical CSV.

Both phases checkpoint to `.cache/` after every page/batch
(`crawl.py`'s docstring has the full resumability design), retry with
exponential backoff on 429/5xx/timeout/truncated-JSON (honoring
`Retry-After` on 429), and run strictly sequentially (no concurrency) —
politeness over a harvest expected to take well over an hour against the
public endpoint.

### Running

```bash
nix develop -c bash pipelines/wikidata/build.sh
```

If the shell running it is itself interrupted or time-limited, just
re-run the same command — progress is checkpointed under
`pipelines/wikidata/.cache/` and resumes rather than restarting.

### Licence

Wikidata is **CC0** → `redistribution: allowed` (no attribution
obligation, unlike the name-list pipelines above, none of which have a
findable license).

### Enrichment is restricted to the joinable subset

Phase 2 (enrichment) does not necessarily cover the whole seed union.
`build.sh` looks for `.cache/powo_ext_ids.txt` (one bare IPNI id per
line — the distinct `ext_id`s from the real concept DB's `xref` table
where `authority='powo'`); if present, only seed items whose P961 or
stripped-P5037 value is in that set are enriched at all. This is a
deliberate scope decision, not a shortcut: **a Wikidata item whose
IPNI/POWO id doesn't match one of our concepts can never be joined by
hostus's ingest, so spending crawl time enriching it buys nothing.** In
the run that produced the numbers below, this cut the population needing
enrichment from **928,129** (the full P961 ∪ stripped-P5037 seed union)
to **393,172** joinable items — roughly 42%, and it also means the
canonical CSV in `output/` only ever contains rows for items our current
index can actually use. If `.cache/powo_ext_ids.txt` is absent, the full
seed union is enriched instead (the pipeline's general-purpose mode).

### Observed summary (live run against the real WDQS, 2026-08-02)

**Join coverage — the headline:** of the 928,129 distinct Wikidata items
carrying P961 or P5037, **393,172 Wikidata items have an IPNI/POWO id
matching one of hostus's 440,534 `xref.powo` concepts, resolving to 392,218
distinct concepts = 89.03% of the index** (measured directly against `xref`
in the real concept DB, not estimated). The two numbers are different units
and must not be conflated: 393,172 counts QIDs, 392,218 counts concepts —
several Wikidata items can carry the same IPNI/POWO id. See the
reconciliation table in `docs/research/reality-check.md` (SP4 T4) for how the
two are derived from each other.

```
seed_union_total=928129
joinable_ids_total=440534
total_items=393172 rows=1709127
populated=wikidata:393172,gbif:384584,colxr:357922,euromed:95,floraveg:24278,wfo:366186,inat:182890
fill_rate_pct=wikidata:100.0,gbif:97.8,colxr:91.0,euromed:0.0,floraveg:6.2,wfo:93.1,inat:46.5
raw_property_counts=P961:369219,P5037:381020,P846:384480,P14607:114,P10585:357922,P12380:95,P12100:24278,P7715:366186,P3151:182890
p961_p5037_disagreements=16243 of items_with_both=357067 (4.549% of items carrying both P961 and P5037)
```

**Per-authority fill rate, population-level (393,172 joinable items) —
this is the first real evidence of how sparse each property actually is:**

| authority | property | populated | fill rate |
|---|---|---|---|
| wikidata | (the item itself) | 393,172 | 100.0% |
| gbif | P846/P14607 (legacy preferred-fallback) | 384,584 | 97.8% |
| wfo | P7715 | 366,186 | 93.1% |
| colxr | P10585 | 357,922 | 91.0% |
| inat | P3151 | 182,890 | **46.5%** |
| floraveg | P12100 (**name, not id**) | 24,278 | 6.2% |
| euromed | P12380 | 95 | 0.02% |

**PoC P7 generalises completely at population scale.** P7 saw P14607
("new" GBIF id) and P12380 (Euro+Med) empty on both reference taxa and
flagged that as inconclusive with n=2. At n=393,172 the picture is
unambiguous: **P14607 is populated on 114 items (0.03%)** and **P12380 on
95 items (0.02%)**. Querying P846 is not a fallback for the rare case
P14607 is missing — for practical purposes today, **P846 is the only
GBIF id Wikidata actually carries**, and the same holds for Euro+Med:
Wikidata is not a usable path to Euro+Med ids, full stop. This is a
finding about the source, not about this pipeline.

**Sampling bias, made explicit — inat is the case in point, but it is not
the whole picture.** Because enrichment processes the seed union in
ascending-QID order (older, more prominently-curated items first — see
the paging strategy above), a partial run's fill rate is a biased
estimate of the true population rate, not a random sample — but the bias
is not uniformly "the sample overstates the population" for every
property. At 173,500 items enriched (QID-ordered, unrestricted) vs. the
final 393,172-item joinable population:

| authority | sample (n=173,500) | population (n=393,172) | direction |
|---|---|---|---|
| inat | 53.3% | **46.5%** | fell 6.8pp, as the QID-order-bias hypothesis predicts |
| gbif | 99.1% | 97.8% | fell 1.3pp, same direction |
| wfo | 97.4% | 93.1% | fell 4.3pp, same direction |
| floraveg | 7.0% | 6.2% | fell 0.8pp, same direction |
| **colxr** | 90.4% | **91.0%** | **rose 0.6pp — the opposite direction** |
| **P14607 (gbif-new only)** | 0.002% | **0.03%** | **rose — opposite direction, but n is tiny (3 → 114) so the percentage move is not statistically meaningful** |

Four of six properties moved the way the QID-order bias predicts. **colxr
and P14607 moved the other way, and this pipeline does not have a
confirmed explanation for either.** A plausible but *unverified*
hypothesis for colxr is that Catalogue of Life coverage does not
correlate with Wikidata item age/prominence the same way GBIF/WFO/iNat
coverage does (COL is itself an aggregation of many regional checklists
added to Wikidata on their own schedule, not necessarily front-loaded onto
old/prominent items) — but this has not been tested against the data and
should be treated as speculation, not a finding. **Report population-level
fill rates, not partial-crawl samples, as the answer** regardless of
direction — the point stands even where the bias didn't run the expected
way: a partial-crawl sample is not a substitute for the full population
count, in either direction.

**P961 vs stripped-P5037 disagreement:** 16,243 items (4.5% of the
357,067 items carrying both properties) have a P961 value that does not
equal the LSID-stripped P5037 value on the same item. Sampled cases (see
`.superpowers/sdd/2026-08-02-sp4-xref/task-1-report.md`) show this is
usually a genuine data-quality issue in Wikidata (one property pointing
at a different IPNI record than the other, e.g. a homonym or a
subsequently-corrected identifier) rather than a formatting artifact. Per
the tie-break rule above, whichever side actually matches our concept
table is used for the CSV's `join_id` column (P961 only as a last-resort
fallback); both raw values are still queried and any item where *either*
raw value matches our concept table is treated as joinable, so a
disagreement never silently drops a genuinely reachable concept **and,
since fix round 1, never emits a dead join_id for one either** — see the
task report.

Wall-clock: seed phase (908,799 + 892,600 rows via single-predicate
`LIMIT 20000` pages) plus the joinable-restricted enrichment phase
(~530 `VALUES`-batches of 500 items, ~2.5-3.5s/batch) together ran to
completion across several bounded foreground invocations of
`build.sh <seconds>`, resuming from `.cache/` checkpoints each time (the
public WDQS endpoint does not stay up for one uninterrupted multi-hour
process). See the task report for the full timing breakdown.

## CDM concept + relation pipeline (`cdm`)

Source: `https://api.cybertaxonomy.org/rl_standardliste` (BGBM/EDIT CDM
server for the German "Standardliste der Farn- und Blütenpflanzen
Deutschlands"). hostus 2.0 SP5 (`POST /v1/translate`, UC6), Task 2.
Background: `poc/P08-findings.md` (probe), `poc/p08b_cdm_sample/`
(measurement), `docs/research/cdm-sample.md` (analysis + go/no-go),
`pipelines/cdm/README.md` (the pipeline's own, German, documentation).

Harvests **51,466 taxonomic concepts across 18 `sec.` reference spaces**
plus the **concept-relation graph** between them.

### Licence

**No licence statement exists.** None on the portal, none on the API, none
in the payloads — probed in PoC P8 and probed again while building this
pipeline. The data is derived from copyrighted flora literature. Therefore:

- `redistribution: unknown`,
- **local evaluation only**,
- shipping the derived relation graph through `/v1/translate` stays
  **blocked** until BGBM/EDIT clarify in writing.

This is the real go/no-go question for SP5, and it is not a technical one.

### Crawl etiquette actually used

Binding, an explicit owner decision; implemented in `pipelines/cdm/common.py`
and not bypassable:

- exactly one honest User-Agent on every request:
  `hostus/2.0 (+https://github.com/jobrunner/hostus; jo.brunner@mayflower.de) taxonomic-concept-research`
- **never a browser User-Agent.** 401/403 on the honest UA is a hard stop
  (`class Refused`, exit 2) and gets reported, never worked around.
- **≤ 1 request/second**, single threaded, exponential backoff on 429/5xx
  and on timeouts.
- everything cached under `.cache/` (gitignored): a re-run costs the server
  nothing. Measured on the validation slice: 411 requests in 7:06 =
  1.037 s/request, matching Task 1's `max(1s, latency)` cost model.

### Three phases, and why this is cheaper than Task 1 costed

| Phase | Requests | Endpoint | Yields |
|---|---:|---|---|
| A | 52 | `/portal/taxon?pageSize=1000&pageIndex=N` | every concept **with** name, raw rank, `secSource`, taxon nodes **and all outgoing relations inline** |
| C | ~5,000–10,000 **(estimated)** | `/classification/{c}/childNodes`, `/taxonNode/{n}/childNodes` | `parent_uuid` (walk of the 18 classification trees) |
| B | 51,466 | `/taxon/{u}/relationsToThisTaxon` | the partner (`to`) end of every edge — the long pole |

Task 1 costed the full crawl as `/portal/taxon/{uuid}/taxonRelationships` for
all 51,466 concepts plus a direction lookup for the ~55% that have relations:
~80,000 requests, 22–30 h. Probing further while building this pipeline
turned up a cheaper shape: the **flat portal listing** already carries, per
concept, `name.nameCache`/`titleCache`, `name.rank.representation_L10n` (the
raw CDM rank vocabulary), `secSource.citation.{uuid,titleCache}`,
`taxonNodes[]`, and — the important one — `relationsFromThisTaxon[]` with
each relation's uuid, type label, symbol and `conceptRelationship` flag.

Measured on page 0 (1,000 concepts): 492 distinct relationship uuids, holder
histogram `{1: 492}` — the listing emits each edge exactly **once**, at its
`from` end. So **52 requests replace 51,466** *and* supply relation direction
for free. New budget ≈ 51,518 + n_internal requests ≈ **17–22 h**, inside
Task 1's envelope rather than beyond it.

Note on Euro+Med (now sourced from EuroSL, its retired REST section above):
the plain `/euromed/taxon` listing has "no `rank` field at all", but
`/portal/taxon` **does** carry the full name object including `rank`. That
lead is moot while EuroSL.sqlite remains the Euro+Med source (it already has
rank + accepted links), but worth remembering if that mirror ever lapses.

Phase C exists only because the concepts CSV has a `parent_uuid` column.
There is no bulk taxon-node endpoint (`/taxonNode?pageSize=…` → 404,
`/checklist/export` → `records: []` on every page, unchanged since P8), so
the 18 classification trees are walked: one request per node that *has*
children; leaves arrive free inside their parent's response.

### Resumability — and where it is not record-exact

A 17–22 h run **will** be interrupted. Every phase checkpoints to disk, but
**the three phases are not equally clean about it**, which is worth stating
plainly rather than claiming uniform exactness.

**Phases B and C are exact.** They append one flushed NDJSON line per unit of
work and resume from the **set** of units already present, not from a
positional offset; a truncated trailing line (a kill between `write` and
`flush`) is dropped on read and the unit is simply re-fetched. No duplicates
arise.

**Phase A is HTTP-idempotent but not record-exact on resume.** Each raw page
is cached as gzipped JSON written to a temp file and renamed, so a kill
cannot leave a half-written page that reads back as truth. But the checkpoint
advances only after a **whole page** (1,000 concepts) has been distilled. A
kill 600 records into a page leaves the counter on the previous page, and the
resumed run replays that page in full — roughly 599 records land in
`concepts.ndjson` a second time.

That is **fixed at convert time, not in the crawler**:
`load_concepts_deduped()` de-duplicates on `concept_uuid` and reports
`duplicate_concept_records_dropped`. Keeping the crawler append-only and
cheap matters more than making it record-exact, and — decisively — a
**running** crawl must never have to be restarted to pick up this fix.
Relations are unaffected either way (`from_holders` is a set), so a phase-A
replay can never trip the falsifier.

Re-running `build.sh` is therefore free and cannot corrupt partial state; at
worst it duplicates phase-A records, which collapse again at convert time.
Verified with a real `SIGKILL` mid-phase-B, a deliberately torn trailing line,
and a simulated phase-A page replay (600 duplicate records injected → 600
dropped, 0 duplicate primary keys in the CSV).

### How both CDM CSVs are quoted — read this before parsing them

Both files are written by `csv.writer(delimiter="|")` with Python's default
`QUOTE_MINIMAL`, i.e. **RFC-4180 quoting with `"` as the quote character** —
the same convention as `pipelines/wikidata/convert.py`. A field containing a
double quote is quoted and its quotes are doubled. **237 of the 51,466
concepts hit this**, and the file really reads:

```
e18ac1cf-…|"Achillea millefolium ""Sammelart"""||Species Aggregate|…
```

A consumer **must** use a real CSV reader configured with `Comma = '|'`
(Go: `encoding/csv`, `r.Comma = '|'`) and **never**
`strings.Split(line, "|")`. On the line above the naive split yields
`"Achillea millefolium ""Sammelart"""` where the correct value is
`Achillea millefolium "Sammelart"` — the field *count* happens to come out
right, so the bug is silent. Task 3 reads these files and must not be misled.

What `_clean()` does on top is **not** escaping: newlines and carriage
returns become spaces, and a literal `|` is replaced by `/`. That
substitution is **lossy corruption**, not escaping — the original character
is gone. It affects **0 fields today** and is pure belt-and-braces. Should a
`|` ever appear in the data, the correct fix is to **delete** the
substitution and rely on the quoting the writer already performs, not to
keep silently mangling values.

### Canonical CSV contract (CDM concepts)

`output/cdm-concepts-canonical.csv`, pipe-delimited, one row per concept:

```
concept_uuid|scientific_name|authorship|rank|status|sec_uuid|sec_title|classification_uuid|parent_uuid
```

- `rank` carries the **raw CDM vocabulary** (`Species`, `Subspecies`,
  `Species Aggregate`, `Unranked (infraspecific)`, `Section bot.`, …).
  22 distinct values observed over the full 51,466 concepts. **Not** mapped
  onto hostus's vocabulary here — that is Task 3's job, where an unknown
  value must fail loudly. `domain.ParseRank` assumed 6 ranks, WCVP had 34,
  and the full ingest aborted after 5.4 s.
- `status` carries **only** the raw `TaxonNodeDto.taxonStatus`, and is
  **empty** where the tree walk has not yet reached the concept. Nothing is
  synthesised. An earlier version fell back to `Accepted`, which would have
  made 51,464 of 51,466 rows assert a value that was never measured — in the
  one CSV whose entire contract is "raw vocabulary, mapping happens in
  Task 3". Empty lets a reader distinguish *not observed* from *observed
  accepted*.
  The CDM `Taxon.doubtful` boolean is a **different field**, not a
  `taxonStatus` value, so it is not folded into this column; it is reported
  separately as `doubtful_concepts` in the summary (1 concept in 51,466).
- `classification_uuid` comes from the hand-curated, **uuid-keyed**
  `CROSSWALK` in `common.py`, lifted from `poc/p08b_cdm_sample/cdm_sample.py`
  together with its `assert_crosswalk()` gate, which still fails loudly.
  Coverage: 50,899/51,466 = **98.9%** of the dataset, matching Task 1.
  There is no machine link between `secSource` and the 18 classifications.
  A concept's `taxonNodes[].classification.uuid` *is* machine-readable but
  answers a different question — where the concept is **placed in a tree**,
  not which `sec.` space it belongs to (`Abies alba sec. Wisskirchen &
  Haeupler 1998` carries a node in the FloraWeb classification). Two
  diagnostics are printed, and the weaker one is labelled as such:
  `sec_space_among_taxon_node_classifications` only asks whether the
  crosswalked space is **among** the concept's placements — which is why
  `not_among` is 0, since a multi-placed concept counts as "among" for
  whichever space the crosswalk picked — while
  `concepts_also_placed_in_a_FOREIGN_classification` (5,875) measures the
  phenomenon that actually rules tree placement out. The crosswalk decides.
- `parent_uuid` is the parent **concept** uuid derived from the tree walk.
  A concept can sit in several trees (5,875 of 51,466 have more than one
  node); the node in the crosswalked `sec.` space wins, otherwise the
  uuid-lowest node. Empty where the walk has not reached the concept.

### Canonical CSV contract (CDM relations)

`output/cdm-relations-canonical.csv`, pipe-delimited, one row per
relationship uuid:

```
from_uuid|to_uuid|relation_type|relation_symbol|is_concept_relation|relationship_uuid
```

- `relation_type` and `relation_symbol` carry the **raw CDM vocabulary**
  (`Congruent to`/`≜`, `Includes`/`⊃`, `Overlaps`/`⊕`, `Included in or
  Includes or Overlaps`/`⊂⊃⊕`, `is pro parte synonym for`, `is misapplied
  name for`, `Not Congruent to`). No mapping here — Task 3.
- `is_concept_relation` is the CDM `type.conceptRelationship` flag, and is
  **empty** — not `false` — for an edge seen only from its `to` side, because
  phase A is the only source of the type object and the flag is then simply
  unknown. `false` is a meaningful value here (it marks a misapplied name as
  not belonging in the concept-relation table at all), so reporting an
  unobserved flag as `false` would be a fabricated measurement.
  `is misapplied name for` is the one type where it is genuinely `false`;
  such rows must be separated out downstream.
- `from_uuid` is the holder found in phase A (`relationsFromThisTaxon`),
  `to_uuid` the holder found in phase B (`relationsToThisTaxon`). Either is
  empty when that end is not (yet) crawled or is ambiguous.

### Relation resolution: the global edge map, and its falsifier

P8 found a relation's partner among the concepts sharing the same accepted
name — 75.9% resolved, with failures concentrated in genus transfers
(*Coronilla varia* ≜ *Securigera varia*) where the partner cannot share the
name. **The name restriction is dropped.** Every relationship uuid is looked
up in one global map over the whole crawl: `from_holders` from phase A,
`to_holders` from phase B. *resolved* = exactly one `from` and one `to`;
*dangling* = one holder total; *ambiguous* = two or more holders on the same
side.

Task 1's ~100% figure is a **projection** resting on one premise — that a
relationship uuid is a binary edge identity. The premise must be able to
fail, so:

- if **any** relationship uuid acquires a **third holder**, `convert.py`
  exits 3 and writes **no CSV at all** (the check runs before either file is
  opened). Verified by injecting a synthetic third holder.
- the **residual one-holder count** is always reported. On a full crawl it
  must go towards zero; a stubborn remainder means relations point at
  concepts outside the `/taxon` listing and `/translate`'s completeness must
  be capped accordingly.

### Running

```bash
bash pipelines/cdm/build.sh          # full crawl; re-run to continue after
                                     # an interrupt, nothing is re-fetched
bash pipelines/cdm/build.sh 3600     # bounded chunk: crawl at most 3600 s
CDM_CRAWL_ARGS="--max-pages 2 --max-concepts 400 --skip-tree" \
  bash pipelines/cdm/build.sh        # bounded validation slice
```

Exit codes:

| Code | Meaning |
|---:|---|
| `0` | done |
| `1` | crawl not yet complete — re-run, nothing is re-fetched |
| `2` | **honest User-Agent refused — stop and report**, never work around it |
| `3` | **falsifier tripped**: a relationship uuid acquired a third holder. No CSV written. |
| `4` | conversion failed for **any other** reason (crash, `assert_crosswalk()`, missing cache file). **Not** the falsifier. |

`3` and `4` are deliberately separate: `3` asserts one specific thing — that
the resolution model of `docs/research/cdm-sample.md` has been refuted — and
a crash must not be allowed to dilute that claim.

`output/`, `.cache/` and `cdm.summary.txt` are gitignored. **No bulk data is
committed** — what is committed is the scripts, the pipeline's own README,
and one named **de-minimis test fixture** under `fixtures/` (14 relations
covering all six resolved types plus their 18 concepts; 32 rows in total).

Why that fixture is in a public repository despite `redistribution: unknown`
— recording the decision rather than leaving it implicit: a 32-row slice
consists of identifiers, scientific names and controlled-vocabulary terms,
i.e. **facts, not creative expression**, and it exists solely so Task 3's Go
tests can run without network access. It is **not** the data base being
shipped with the software, which is where the owner's licensing frame
actually draws the line. **The fixture is not to be enlarged.**

The validation run's summary is reproduced verbatim below.

### Observed summary (bounded validation slice, 2026-08-02)

**This is not the full crawl.** It is `convert.py` run against a *snapshot*
of the cache taken while the full crawl was in progress. Phase A is complete
(all 51,466 concepts, all 26,346 edges' `from` ends). Phase B covers only
795 of 51,466 concepts (308 lifted at zero request cost from
`poc/p08b_cdm_sample/.cache/to`, the rest a 500-concept slice). Phase C had
reached 5,772 nodes at snapshot time and is still running.

So: `concepts_fetched`, the rank distribution and the relation-type
distribution are already **dataset-wide and final**. `resolved`, `dangling`,
`residual_one_holder_uuids`, `concepts_with_parent_uuid` and
`status_distribution` reflect the **slice** and will move substantially on
the full run.

```console
$ CDM_CRAWL_ARGS="--skip-tree --max-concepts 500" bash pipelines/cdm/build.sh
source=https://api.cybertaxonomy.org/rl_standardliste
crawl_etiquette=1 honest UA, <=1 req/s, single threaded, backoff on 429/5xx, disk cache
source=https://api.cybertaxonomy.org/rl_standardliste (/portal/taxon listing + /taxon/{u}/relationsToThisTaxon + classification tree walk)
license=NONE FOUND anywhere (portal, API, payloads) redistribution=unknown -- local evaluation only
concepts_fetched=51466
duplicate_concept_records_dropped=0  (phase A checkpoints per page, so an interrupted page replays in full on resume; deduped on concept_uuid here)
concepts_with_incoming_lookup=795
classifications=18
crosswalk assertion: 17 entries, all targets are real classification uuids; 17/18 classifications targeted, 1 explicitly unmapped
tree_nodes=5772 nodes_with_known_parent=3558
relations_found=26346
holders_per_relationship_uuid=1:25815, 2:531
falsifier=PASS no relationship uuid acquired a third holder (max holders seen: 2)
concepts_csv=output/cdm-concepts-canonical.csv rows=51466
sec_mapped_via_crosswalk=50899/51466 = 98.9%
concepts_with_parent_uuid=3558/51466 = 6.9%
concepts_with_multiple_taxon_nodes=5875
sec_space_among_taxon_node_classifications: among=35872 not_among=0 no_node_yet=15523  (diagnostic only, and a WEAK one: it asks whether the crosswalked sec. space is one of the concept's tree placements, NOT whether placement equals the sec. space)
concepts_also_placed_in_a_FOREIGN_classification=5875  (this is why taxonNodes cannot be used as the sec. space; the crosswalk decides)
rank_distribution(raw CDM)=Species:36146, Subspecies:6434, Genus:3711, Variety:2537, Species Aggregate:1088, Family:629, Species Group:374, Form:162, Section bot.:98, Order:90, Unranked (infraspecific):73, Subvariety:26, Subgenus:20, Class:16, Subclass:14, Unranked (infrageneric):11, Series:9, Subsection bot.:9, Subkingdom:7, Race:6, Phylum:5, Subform:1
status_distribution=(not observed):45696, Accepted:5770  (raw TaxonNodeDto.taxonStatus where the tree walk reached the concept, empty otherwise -- nothing is synthesised)
doubtful_concepts=1  (CDM Taxon.doubtful boolean -- a DIFFERENT field, deliberately NOT folded into the status column)
relations_csv=output/cdm-relations-canonical.csv rows=26346
resolved=531/26346 = 2.0%
ambiguous=0/26346 = 0.0%
dangling=25815/26346 = 98.0%
residual_one_holder_uuids=25815  (must go towards zero on a FULL crawl; a stubborn remainder means relations point at concepts outside the /taxon listing and /translate completeness must be capped)
orphan_to_end_only=0  (edge seen only from its `to` side -- its `from` concept is not in the /portal/taxon listing)
relation_type_distribution(raw CDM)=Congruent to:23971, Includes:1591, is misapplied name for:344, Included in or Includes or Overlaps:198, is pro parte synonym for:123, Overlaps:118, Not Congruent to:1
!! relation types NOT seen in Task 1's sample (new at full scope, must be handled by Task 3's mapper): Not Congruent to:1
resolved_relation_type_distribution(raw CDM)=Congruent to:473, Includes:26, Included in or Includes or Overlaps:12, is misapplied name for:10, Overlaps:6, is pro parte synonym for:4
reference_concept=872088a4-95f4-472c-ae79-a29028bb3fbf Abies alba Mill. | rank=Species | sec=Wisskirchen & Haeupler 1998: Standardliste der Farn- und Blütenpflanzen Deutschlands | cls=4ea7fe85-4a02-47a0-949f-5e623f0c6216
reference_concept_relations: from=0 to=10
  <--Congruent to(≜)-- Abies alba Mill. sec. Greuter & al.: Med-Checklist bisher Bd
  <--Congruent to(≜)-- Abies alba Mill. sec. HEGI: Illustrierte Flora von Mitteleur
  <--Congruent to(≜)-- Abies alba Mill. sec. EHRENDORFER: Liste der Gefäßpflanzen M
  <--Congruent to(≜)-- Abies alba Mill. sec. TUTIN et al.: Flora Europaea
  <--Congruent to(≜)-- Abies alba Mill. sec. SCHMEIL-FITSCHEN: Flora von Deutschlan
  <--Congruent to(≜)-- Abies alba Mill. sec. OBERDORFER: Pflanzensoziologische Exku
  <--Congruent to(≜)-- Abies alba Mill. sec. Schubert, R. & Vent, W. (eds.) 1990: E
  <--Congruent to(≜)-- Pinus abies L. sec. BfN: FloraWeb DB (fuer Synonyme mit Fa
  <--Congruent to(≜)-- Pinus abies L. 1753 sec. Andere Referenzen (fuer auct. Synonyme
  <--is misapplied name for(misapplied for)-- Pinus abies L. 1753 sec. Andere Referenzen (fuer auct. Synonyme
```

(The generated summary also carries a `wall_clock_seconds` line. It is
omitted here because on this re-run everything came from cache and it read
`1` — meaningless. The real measured figure is **411 requests in 7:06 =
1.037 s/request**, from the phase-B slice.)

Cross-checks against Task 1's sample measurement:

- **26,346 edges** dataset-wide. Task 1's sample implied ~27,400 — agreement
  within 4%.
- **Zero ambiguity, zero orphan `to`-ends** over 795 incoming lookups: every
  single `to` end found paired with exactly one already-known `from` end.
  Max holders per relationship uuid: **2**. The edge-identity premise holds
  so far.
- Relation-type shares dataset-wide: `Congruent to` 91.0%, `Includes` 6.0%,
  `is misapplied name for` 1.3%, `⊂⊃⊕` 0.8%, `pro parte` 0.5%, `Overlaps`
  0.4%. Task 1's re-weighted sample estimate was ~85% / 8.8% / 0.6% / 2.5% /
  0.2% / 2.5% — same ordering, `Congruent to` somewhat higher at full scope.
- **A seventh relation type appears that Task 1's sample never saw:
  `Not Congruent to` (1 occurrence).** `convert.py` flags every type outside
  Task 1's six explicitly. Task 3's mapper must handle it, and must abort on
  an unknown value rather than guess.
- The P8 reference concept `872088a4-95f4-472c-ae79-a29028bb3fbf`
  (*Abies alba* Mill. sec. Wisskirchen & Haeupler 1998) resolves as expected:
  0 outgoing, **10 incoming** relations, all 10 with a unique partner —
  congruences from *Abies alba* in seven other `sec.` spaces plus
  *Pinus abies* twice, and one `is misapplied name for`. It is a pure hub
  `to`-end, which is exactly why phase B cannot be skipped.

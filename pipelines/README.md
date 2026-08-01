# Pipelines (xlsx/sqlite/REST → canonical CSV)

Two families of pipelines live here: three **trait** pipelines (EIVE /
Tichý / Midolo, documented first below) and four **name-list** pipelines
(GermanSL / EuroSL / FloraVeg / Euro+Med, documented in their own section
further down) that acquire additional backbone/checklist sources for local
evaluation. Both families follow the same shape: pinned source → download-
or-reuse-cache → convert → canonical CSV in `output/` (gitignored) →
printed summary.

## Trait pipelines

Three per-vocabulary pipelines convert the raw EIVE / Tichý / Midolo
downloads (see `docs/research/quellenregister.md` for pinned Zenodo DOIs and
licenses, `poc/P06-findings.md` for the exact sheets/columns/join keys
discovered against the real data) into one shared canonical CSV format that
`internal/adapters/traits` reads.

These pipelines are shell + Python (openpyxl) tooling, run offline/ahead of
time to produce checked-in fixtures and, in a real deployment, the actual
ingest data. They are **not** part of `make verify` — hostus itself never
touches xlsx or the network at runtime; it only ever reads the canonical CSV
via `traits.Read`.

### Canonical CSV contract (traits)

Identical for all three vocabularies:

- Pipe-delimited (`|`), matching the WCVP reader convention.
- Header: `taxon|vocab|vocab_version|dim|value|niche_width|n_systems`
- One row per `(taxon, dim)` that the vocabulary actually provides a value
  for. A dimension the vocabulary doesn't cover for a given taxon (e.g. a
  Tichý "x"/indifferent or "NA" cell) is simply **not emitted as a row** —
  `dim`+`value` are mandatory once a row exists.
- `niche_width` / `n_systems` are empty strings when the vocabulary does not
  provide that concept at all (Tichý and Midolo never populate them; EIVE
  always does). Empty ≠ zero — `internal/adapters/traits.Read` decodes empty
  as a `nil` pointer, not `0.0`/`0`.
- `dim` values match `internal/domain.TraitDim` string spellings exactly:
  `M`, `N`, `R`, `L`, `T`, `S` (EIVE/Tichý indicator dims) and
  `disturbance_severity`, `disturbance_frequency`, `mowing_frequency`,
  `grazing_pressure`, `soil_disturbance` (Midolo).

### Running a trait pipeline

```bash
nix develop -c bash pipelines/eive/build.sh
nix develop -c bash pipelines/tichy/build.sh
nix develop -c bash pipelines/midolo/build.sh
```

Each `build.sh`:

1. Resolves its source file: reuses an already-downloaded copy under its own
   `.cache/` dir if present, else reuses the file already sitting in
   `poc/data/` from PoC P6 if present, else downloads it fresh from the
   pinned Zenodo URL. No `status=ACCEPTED`-style filtering or xlsx library in
   Go — conversion happens entirely in `python3`+`openpyxl` (already in the
   Nix devshell; Midolo's main table is already CSV, no openpyxl needed).
2. Emits the canonical CSV to `pipelines/<vocab>/output/<vocab>-canonical.csv`
   (gitignored — this is a generated artifact, not a fixture).
3. Prints a summary (rows, distinct taxa, dims, observed min/max per
   dimension) to stdout and to `pipelines/<vocab>/<vocab>.summary.txt`.

### Observed summaries (run 2026-08-01, against the real PoC P6 downloads)

### EIVE 1.0 (Zenodo 10.5281/zenodo.7534792, v1.0)

```
rows=71266 taxa=14835 dims=M,N,R,L,T
  dim M: min=0.0 max=10.0
  dim N: min=0.0 max=10.0
  dim R: min=0.0 max=10.0
  dim L: min=0.0 max=10.0
  dim T: min=0.0 max=10.0
```

Confirms the uniform 0-10 continuous scale for all 5 dims (poc/P06-findings.md).

### Tichý et al. 2023 (Zenodo 10.5281/zenodo.7427088, v2.0)

```
rows=45592 taxa=8908 dims=L,T,M,R,N,S
  dim L: min=1.0 max=9.0
  dim T: min=1.0 max=12.0
  dim M: min=1.0 max=12.0
  dim R: min=1.0 max=9.0
  dim N: min=1.0 max=9.0
  dim S: min=0.0 max=9.0
```

**Observed Salinity (S) range: 0.0–9.0.** `poc/P06-findings.md` only sampled
a narrow slice (~-0.02 to 0) and flagged the full range as unconfirmed;
`internal/domain.ScaleFor` deliberately does not hardcode a Salinity range
for this reason. This is that range, from the real data.

Note also: T (Temperature) and M (Moisture) both range up to 12, not the
classic Ellenberg 1-9 that `internal/domain.ScaleFor` currently documents for
all five L/T/M/R/N dims — some of Tichý's 13 source systems apparently use
an extended scale for these two dims. This is a real discrepancy between the
T1-committed `ScaleFor` comment and the actual data; flagged for follow-up,
not silently "fixed" here since correcting `ScaleFor`'s documented range is
T1/T4 territory, not this ingest pipeline's.

### Midolo et al. 2023 (Zenodo 10.5281/zenodo.7116957, v3)

```
rows=31910 taxa=6382 dims=disturbance_severity,disturbance_frequency,mowing_frequency,grazing_pressure,soil_disturbance
  dim disturbance_severity: min=0.10213 max=0.965
  dim disturbance_frequency: min=0.0 max=2.62976
  dim mowing_frequency: min=0.0 max=2.11841
  dim grazing_pressure: min=0.0 max=0.77826
  dim soil_disturbance: min=0.00217 max=0.935
```

Confirms these are genuinely continuous, unbounded-in-practice indicators
(no fixed min/max scale), matching `ScaleFor`'s `(0, 0, false)` sentinel for
all Midolo dims. `SD_*` columns (standard deviation) are intentionally never
mapped to `niche_width` — it is a different statistical concept, and Midolo
has no true niche-width column at all.

### Attribution (CC-BY-4.0 — required per vocabulary)

All three vocabularies are Zenodo-published under CC-BY-4.0. Any product
that serves data derived from these pipelines must retain attribution:

- **EIVE 1.0**: Dengler, J. et al. (2023). Ecological Indicator Values for
  Europe (EIVE) 1.0. *Vegetation Classification and Survey* 4: 7-29.
  https://doi.org/10.3897/VCS.98324 — data: Zenodo
  https://doi.org/10.5281/zenodo.7534792
- **Tichý et al. 2023**: Tichý, L. et al. (2023). Ellenberg-type indicator
  values for European vascular plant species. *Journal of Vegetation
  Science* 34: e13168. https://doi.org/10.1111/jvs.13168 — data: Zenodo
  https://doi.org/10.5281/zenodo.7427088 (v2.0)
- **Midolo et al. 2023**: Midolo, G. et al. (2023). Disturbance indicator
  values for European plants. *Global Ecology and Biogeography* 32(1):
  24-34. https://doi.org/10.1111/geb.13603 — data: Zenodo
  https://doi.org/10.5281/zenodo.7116957 (v3)

## Name-list pipelines

Four further pipelines (`germansl`, `eurosl`, `floraveg`, `euromed`) acquire
additional backbone/checklist sources. These are **the sources with no
findable license** (`redistribution: unknown`/`restricted` in `dataset.yaml`
terms — see Task 1's redistribution gate): the data is publicly offered by
its maintainers, but nobody has stated terms permitting redistribution.
Per the 2026-08-01 owner decision (`docs/superpowers/plans/2026-08-01-reality-check.md`),
that licenses **local, private evaluation** under German scientific-research
privilege (§60c/§87c UrhG) even though it does not license redistribution.

**These four pipelines are for local evaluation only.** Their `output/*.csv`
must never be exported in a served bundle — `ExportBundle` (Task 1's gate)
refuses by default to include any source not marked `redistribution:
allowed`, and none of these four are. Do not add them to `dataset.yaml` as
`allowed` sources; do not point any served endpoint at their `output/`.

### Canonical CSV contract (name lists)

Identical shape for all four sources, pipe-delimited like the trait CSVs:

- Header: `taxon|rank|status|accepted_taxon|source_id`
- `taxon` — the scientific name string as the source provides it. Where the
  source blends name+author+citation into one string (Euro+Med's
  `titleCache`), the `sec.`/`syn. sec.` citation tail is stripped but the
  author is left in place (no separate author column exists to draw on
  there, unlike GermanSL/EuroSL).
- `rank` — as provided by the source, in the source's own vocabulary (e.g.
  GermanSL's `SPE`/`GAT`/`FAM`, EuroSL's `Species`/`Genus`/`Family`). Not
  normalized to a shared enum across sources — callers needing a uniform
  rank scheme must map per `source_id`. Empty when the source doesn't
  provide one at all (FloraVeg's Life_form table; Euro+Med's flat listing).
- `status` — `accepted` or `synonym` (EuroSL also distinguishes
  `synonymobjective`, kept as its own value rather than folded into
  `synonym`), lowercased. `accepted` when the source has no synonymy
  concept at all (FloraVeg).
- `accepted_taxon` — the accepted name string, only for `status != accepted`
  rows. Empty for accepted rows, and empty (not guessed) wherever the
  source doesn't expose the link cheaply (Euro+Med — see its section below).
- `source_id` — the source's own stable identifier (GermanSL
  `TaxonUsageID`, EuroSL `TaxonUsageID`, FloraVeg `SeqID`, Euro+Med CDM
  `uuid`).
- Empty field = not provided by the source, same convention as the trait
  CSVs' empty `niche_width`/`n_systems`.

### Running a name-list pipeline

```bash
nix develop -c bash pipelines/germansl/build.sh
nix develop -c bash pipelines/eurosl/build.sh
nix develop -c bash pipelines/floraveg/build.sh
nix develop -c bash pipelines/euromed/build.sh   # slow: ~336 sequential HTTP pages, ~30-40 min
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
`TaxonConcept`), read directly via Python's stdlib `sqlite3`. **Surprise:**
every row's `AccordingTo` column reads `api.cybertaxonomy.org/euromed` —
EuroSL is itself built from the same underlying Euro+Med CDM dataset that
the `euromed` pipeline probes independently below.

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

### Euro+Med — CDM REST probe (api.cybertaxonomy.org/euromed)

R1 found no bulk export for Euro+Med. Per the task brief, this pipeline
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

# P02 — Findings: WCVP archive structure for the SP1 importer

**Question**: Is the WCVP bulk archive really a DwC-A (as the corrected
`docs/research/quellenregister.md` register found), not ColDP (as the
architecture spec's "ColDP importer" wording for WCVP implies)? What are the
exact filenames/delimiter, and how do the spec §4.3 schema fields
(`name.ipni_id`, `concept_name.role`/`homotypic`, `name.basionym_id`,
`name.rank`, `name.authorship`, `name.canonical`, `distribution.area_code`)
map onto real WCVP columns? (SP1's importer and SP2's distribution-based
ranking depend on this.)

**Probe**: [`p02_wcvp/inspect.sh`](./p02_wcvp/inspect.sh)

**Environment**: downloaded live via `nix develop -c bash poc/p02_wcvp/inspect.sh`
on 2026-07-31. Archive fetched directly from the primary URL
(`https://sftp.kew.org/pub/data-repositories/WCVP/wcvp_dwca.zip`, 85 MB,
`Last-Modified: 2026-06-04`) — no fallback needed.

## Command

```bash
nix develop -c bash poc/p02_wcvp/inspect.sh
```

Full captured output: [`p02_wcvp/inspect-output.txt`](./p02_wcvp/inspect-output.txt).

## 1. Archive format: DwC-A confirmed, NOT ColDP

`meta.xml`'s root element is `<archive xmlns="http://rs.tdwg.org/dwc/text/" metadata="eml.xml">`
— this is the canonical Darwin Core Archive descriptor format (same family as
GBIF occurrence/checklist DwC-A exports), structurally unrelated to ColDP's
`Name.tsv`/`Taxon.tsv`/`metadata.yaml` layout used by IPNI and WFO (see
register). The five files are:

| File | DwC-A role | rowType |
|---|---|---|
| `eml.xml` | dataset metadata (title, license, pubDate, GBIF/COL version) | — |
| `meta.xml` | DwC-A descriptor (defines core + 2 extensions, delimiter, field→term mapping) | — |
| `wcvp_taxon.csv` | **core** file | `http://rs.tdwg.org/dwc/terms/Taxon` |
| `wcvp_distribution.csv` | extension, joined via `coreid` | `http://rs.gbif.org/terms/1.0/Distribution` |
| `wcvp_replacementNames.csv` | extension, joined via `coreid` | `https://terms.catalogueoflife.org/NameRelation` (note: a **ColDP term URI** used as a DwC-A extension rowType — WCVP mixes vocabularies here, but the container format is still DwC-A) |

**Correction to the architecture spec**: wherever the spec or its subprojects
say "WCVP ColDP importer" this is factually wrong — the WCVP importer in
`internal/adapters/coldp/` (despite the package name `coldp`) must be a
**DwC-A parser** for WCVP specifically: pipe-delimited CSV + a `meta.xml`
descriptor, not tab-delimited ColDP TSVs. (IPNI and WFO, also consumed by
SP1/SP3, *are* genuine ColDP — so the importer package will need two distinct
parsing paths, or WCVP needs a small DwC-A→internal-row adapter that feeds the
same downstream pipeline. This should be reflected in SP1's task breakdown.)

## 2. Exact filenames, delimiter, encoding

- Delimiter: **pipe (`|`)**, confirmed both by `meta.xml`
  (`fieldsTerminatedBy="|"`) and empirically (header line: 17 pipes / 18
  columns in `wcvp_taxon.csv`, 0 tabs, 0 commas). `fieldsEnclosedBy=''`
  (**no quoting at all** — a naive CSV/TSV parser must not attempt RFC 4180
  quote-handling on this file; any literal `|` inside a data value would
  break the line, but no such case was observed in the reference-taxa rows).
  `linesTerminatedBy="\n"`, `encoding="UTF-8"`, `ignoreHeaderLines="1"`.
- Row counts (real, full archive): `wcvp_taxon.csv` 1,448,984 data rows,
  `wcvp_distribution.csv` 1,995,338 data rows, `wcvp_replacementNames.csv`
  44,041 data rows.
- **Header-name quirk (must be handled verbatim, not "fixed")**: the real
  `wcvp_taxon.csv` header contains a typo baked into the shipped data:
  `scientfiicname` and `scientfiicnameauthorship` (transposed letters, missing
  the correct "scientific" spelling). This is not a corruption from our
  download — it's present in the vendor file itself. **The SP1 importer must
  match column names positionally (per `meta.xml`'s `<field index=.. term=..>`
  mapping) rather than by parsing/matching the literal header string against
  the "correct" DWC term name**, or it must special-case this typo. The
  fixture preserves this typo verbatim for exactly this reason — an importer
  that assumes `scientificname` will silently fail to find the canonical-name
  column against this fixture (and the real file).

## 3. Schema field → WCVP column mapping

`wcvp_taxon.csv` (core, DwC `Taxon`), 18 columns by index:

| index | header (verbatim, incl. typo) | dwc term | → spec `name`/`taxon_concept`/`concept_name` field |
|---|---|---|---|
| 0 | `taxonid` | taxonID | primary key; **also serves as WCVP's own `name` id AND its `taxon_concept` id** — WCVP does not separate nomenclatural name identity from taxonomic concept identity the way the spec's `name`/`taxon_concept` split does (see §5) |
| 1 | `family` | family | denormalized, for filtering/backbone building |
| 2 | `genus` | genus | denormalized |
| 3 | `specificepithet` | specificEpithet | part of canonical, rank ≥ SPECIES |
| 4 | `infraspecificepithet` | infraspecificEpithet | part of canonical, rank ≥ SUBSPECIES |
| 5 | `scientfiicname` (typo) | scientificName | → `name.canonical` (full name **without** authorship, e.g. "Corynephorus canescens") |
| 6 | `scientfiicnameauthorship` (typo) | scientificNameAuthorship | → `name.authorship` (e.g. "(L.) P.Beauv." — parenthetical author present ⇒ recombination/basionym exists) |
| 7 | `taxonrank` | taxonRank | → `name.rank`, values seen: `Genus`, `Species`, `Subspecies`, `Variety`, `Form`, `Subvariety` — **mixed-case strings, not the spec's uppercase enum** (`FAMILY\|GENUS\|SPECIES\|SUBSPECIES`); importer must normalize case and map `Variety`/`Form`/`Subvariety` into the spec's rank set (spec only lists 4 ranks; WCVP has finer granularity that will need a decision — collapse into SUBSPECIES or extend the enum) |
| 8 | `taxonomicstatus` | taxonomicStatus | → `concept_name.role`: values seen `Accepted`, `Synonym`, `Illegitimate`, `Invalid` — **not just accepted/synonym as the spec's two-value enum assumes**; `Illegitimate`/`Invalid` are nomenclatural-status-adjacent statuses that still need an accepted-name target (both had non-empty `acceptednameusageid` in samples) but are neither cleanly "accepted" nor "synonym" |
| 9 | `acceptednameusageid` | acceptedNameUsageID | for `taxonomicstatus=Accepted` rows, **self-referential** (equals `taxonid`); for synonyms/illegitimate/invalid rows, points to the accepted taxon's `taxonid` → drives `concept_name` accepted/synonym grouping |
| 10 | `parentnameusageid` | parentNameUsageID | classification parent (e.g. genus row for a species) → `taxon_concept.parent_id` |
| 11 | `originalnameusageid` | originalNameUsageID | → `name.basionym_id` candidate: populated on recombinations/synonyms that have a parenthetical author, pointing to the basionym's `taxonid` (empirically confirmed, see §5) |
| 12 | `namepublishedin` | namePublishedIn | → `name.published_in` |
| 13 | `nomenclaturalstatus` | nomenclaturalStatus | → `name.nom_status`, free text (`, nom. illeg. superfl.`, `, not validly publ.`, `, nom. superfl.`) — needs normalization/parsing into the spec's `nom_nud\|nom_superfl\|pro_syn\|...` codes, not a 1:1 match |
| 14 | `taxonremarks` | taxonRemarks | free text, sometimes a short native-range summary (NOT the structured distribution — see §4) |
| 15 | `scientificnameid` | scientificNameID | **unreliable** IPNI-id source, see §5 |
| 16 | `dynamicproperties` | dynamicProperties | JSON blob: `{"powoid":"...","lifeform":"...","climate":"...","homotypicsynonym":"T"\|"","hybridformula":"...","reviewed":"Y"\|"N"}` — `powoid` is the **reliable** IPNI-id source (see §5); `homotypicsynonym` ("T"/empty) → `concept_name.homotypic` |
| 17 | `references` | dc:references | POWO taxon page URL, always ends in `urn:lsid:ipni.org:names:{powoid}` — third, cross-checkable source of the same IPNI id |

`wcvp_distribution.csv` (extension, DwC/GBIF `Distribution`), 6 columns:

| index | header | dwc term | → spec `distribution` field |
|---|---|---|---|
| 0 | `coreid` | (join key) | → `distribution.concept_id` (= `wcvp_taxon.taxonid`) |
| 1 | `locality` | locality | human-readable TDWG-L3 unit name (e.g. "Argentina Northeast") |
| 2 | `establishmentmeans` | establishmentMeans | `introduced` when non-native, else empty; **no "native" literal is written** — absence = native |
| 3 | `locationid` | locationID | **`TDWG:XXX`** where `XXX` is the WGSRPD level-3 area code → `distribution.area_code` (strip the `TDWG:` prefix), `distribution.area_scheme = "wgsrpd_l3"` |
| 4 | `occurrencestatus` | occurrenceStatus | empty in all sampled rows (one exception: literal `Extinct` seen for `Festuca ovina` × Netherlands) |
| 5 | `threatstatus` | iucn:threatStatus | empty in all sampled rows |

`wcvp_replacementNames.csv` (extension, rowType is a **ColDP** term
`NameRelation` despite the DwC-A container), 4 columns: `taxonid`,
`relatednameusageid`, `relationtype` (constant `"replacement name"`),
`remarks`. Small (44k rows), maps nomenclatural replacement-name relations —
relevant for `name.nom_status` disambiguation but not required for basic
accepted/synonym grouping.

## 4. WGSRPD level-3 presence — confirmed, but no continent/region column

The spec's schema (`distribution.area_scheme`) anticipates `wgsrpd_l3` as one
value. **Confirmed present**: `wcvp_distribution.csv.locationid` carries
exactly this (`TDWG:AUT`, `TDWG:GER`, `TDWG:CHS`, …) for every one of the
1,995,338 distribution rows sampled/spot-checked. WGSRPD is the standard TDWG
World Geographical Scheme for Recording Plant Distributions; level-3 = the
"botanical country/region" granularity the spec wants for `in_area` ranking.

**Gap found, not anticipated by the spec appendix**: there is **no
`continent` or `region` column** in `wcvp_distribution.csv` — `meta.xml`'s
field list for the Distribution extension is exactly `locality`,
`establishmentMeans`, `locationID`, `occurrenceStatus`, `threatStatus`,
`license`, `rightsHolder`, nothing else. If SP1/SP2 need a continent-level
rollup (e.g. for a coarser "in Europe" filter), it must be derived from the
WGSRPD level-3 code via an **external WGSRPD code→continent lookup table**
(the standard TDWG Level-1/2/3 code hierarchy, not shipped in this archive) —
not read directly off a WCVP column. This should be flagged for whichever
subproject (SP2) builds `in_area` ranking.

## 5. IPNI-id resolution — three sources, one unreliable

The spec's `name.ipni_id` field has **three** candidate WCVP sources per row,
and they disagree in reliability:

1. `scientificnameid` (e.g. `ipni:932629-1`) — **empty in ~30–40% of sampled
   rows** (confirmed empirically: e.g. `taxonid 519067`, "Paeonia delavayi
   var. lutea", has an empty `scientificnameid` field).
2. `dynamicproperties.powoid` (JSON field, e.g. `"519067-4"`) — **populated
   in every single row sampled**, including all rows where (1) was empty.
   This is the reliable source.
3. `references` URL suffix (`.../urn:lsid:ipni.org:names:{powoid}`) —
   also always populated, redundant with (2), useful as a cross-check /
   fallback if JSON parsing of `dynamicproperties` is skipped.

**Recommendation for SP1**: parse `dynamicproperties` as JSON and use
`powoid` as the primary `ipni_id` source; do not rely on `scientificnameid`
alone. This is a materially important, non-obvious finding — an importer
built against the DwC term name "scientificNameID" (which sounds like *the*
canonical IPNI-id field) would silently produce `NULL` `ipni_id` for a large
fraction of names.

Both reference taxa from the spec appendix resolve correctly through all
three sources:

| Taxon | `taxonid` | `powoid` / IPNI id | matches spec appendix? |
|---|---|---|---|
| *Corynephorus canescens* (L.) P.Beauv. | 405825 | `396681-1` | ✅ exact match to spec's `396681-1` |
| *Jacobaea vulgaris* Gaertn. | 3082777 | `226649-1` | ✅ exact match to spec's `226649-1` |

Note: WCVP also carries a **separate, illegitimate** `taxonid 3082280`
"Jacobaea vulgaris (L.) Claus" (IPNI `226650-1`, `taxonomicstatus=Illegitimate`,
a later homonym) — same canonical string, different author, different IPNI
id, both pointing at different things. Confirms the spec's warning that
canonical-name-only lookups are ambiguous; disambiguation needs
author+IPNI-id, not canonical alone.

*Festuca ovina* aggregate: accepted `taxonid 415853` (IPNI `403212-1`,
`L.`), with 167 rows carrying `acceptednameusageid=415853` in the full
archive (heterotypic + homotypic synonyms across many genera —
*Avena ovina*, *Bromus ovinus*, *Festuca duriuscula*, etc. — confirming the
"aggregate with many synonyms" premise from the spec appendix).

**Basionym/`originalnameusageid` spot check**: `taxonid 405826` ("Corynephorus
canescens f. pallidus", author `(Beckh.) Soó` — parenthetical, i.e. a
recombination) has `originalnameusageid=450134`, which resolves to `taxonid
450134` = "Weingaertneria canescens var. pallida" (Beckh., no parentheses —
the original combination). This is exactly the basionym relationship the
spec's `name.basionym_id` field wants, and it round-trips correctly through
`originalnameusageid`.

## 6. License

Confirms register footnote 2: `eml.xml`'s `<intellectualRights>` states
**CC BY 3.0** verbatim ("Creative Commons Attribution 3.0 Unported (CC BY 3.0)
License"), contradicting the GBIF dataset catalog page's CC BY 4.0 claim. The
archive-embedded license (CC BY 3.0) is the one to record in
`backbone_version.license` per the register's own recommendation, since it
travels with the actual bytes being ingested.

`eml.xml` additionally confirms: `pubDate` 2026-06-04, GBIF harvest
`dateStamp` 2026-06-04T15:07:11Z, and an embedded COL cross-reference block
(`<col><version>16.0</version><completeness>95</completeness></col>`) — useful
provenance for `backbone_version.version`/`manifest_sha` bookkeeping.

## 7. ID stability notes

- `taxonid` is WCVP/POWO's internal integer key and doubles as the IPNI
  numeric id in most but not all cases (e.g. `taxonid 405825` vs. its own
  `powoid 396681-1` — **they are different numbers for the same taxon**,
  because `taxonid` is a WCVP-internal id, and `powoid`/IPNI id is a
  separate authority's id embedded per-row). **Do not conflate `taxonid` with
  IPNI id** — they must be stored as two distinct identifiers
  (`taxon_concept.id` / internal foreign keys can use `taxonid`, but
  `name.ipni_id` must come from `dynamicproperties.powoid`, per §5).
- Accepted-row self-reference (`acceptednameusageid == taxonid`) is a cheap,
  reliable way to detect "this row is the accepted name" without depending
  solely on the `taxonomicstatus` string — useful as a defensive
  cross-check/invariant in the importer.

## Fixture

Cut into
[`internal/adapters/coldp/testdata/wcvp-sample/`](../internal/adapters/coldp/testdata/wcvp-sample/):

- `eml.xml` — trimmed (real content, ~150 reviewer `<associatedParty>`
  entries removed; license/pubDate/title/gbif+col version blocks kept
  verbatim).
- `meta.xml` — copied verbatim (3,190 bytes, unmodified).
- `wcvp_taxon.csv` — real header (typo preserved) + 20 real rows: 3 genus
  rows (`Corynephorus`, `Jacobaea`, `Festuca`), the 2 reference accepted
  species (*Corynephorus canescens*, *Jacobaea vulgaris*) plus 3–5 of their
  real synonyms each (including the basionym pair 405826/450134 and the
  illegitimate homonym 3082280), the *Festuca ovina* aggregate accepted row
  plus 5 real cross-genus synonyms (*Avena dura*, *Avena ovina*, *Bromus
  ovinus*, *Festuca duriuscula*, *Festuca ovina* var. *vulgaris*).
- `wcvp_distribution.csv` — real header + 27 real rows (9 each for the 3
  accepted taxa, WGSRPD-L3 codes, includes both native and `introduced`
  rows).
- `wcvp_replacementNames.csv` — real header + 2 real rows exercising the
  `taxonid`/`relatednameusageid` join back into `wcvp_taxon.csv`
  (477950→415853, 3082777→3082790).

All rows are byte-identical extracts from the real, live-downloaded archive
(no synthesized data) — a faithful miniature of the real file structure,
delimiter, and known data quirks (header typo, sparse `scientificnameid`).

## Verdict

**Verdict: 🟢 (WCVP structure supports the schema; fixture cut)**

The real archive is confirmed as a genuine DwC-A (not ColDP) with pipe
delimiters, exactly the 5 files the register predicted. Every spec §4.3
schema field the WCVP importer needs (`ipni_id`, accepted/synonym role,
basionym, rank, authorship, canonical, WGSRPD-L3 area codes) has a working
real-data mapping, verified against both reference taxa (*Corynephorus
canescens* IPNI `396681-1`, *Jacobaea vulgaris* IPNI `226649-1`) and the
*Festuca ovina* aggregate. Three findings need to flow back into SP1/SP2
task definitions rather than being silent surprises during implementation:

1. **Spec wording fix**: "WCVP ColDP importer" is wrong; it's a DwC-A parser.
   The `internal/adapters/coldp/` package needs a WCVP-specific DwC-A path
   distinct from the genuine ColDP path used for IPNI/WFO.
2. **IPNI id must come from `dynamicproperties.powoid`**, not
   `scientificnameid` (which is empty ~30–40% of the time) — a naive
   DWC-term-name-based importer would silently under-populate `name.ipni_id`.
3. **No continent/region column in WCVP distribution** — only WGSRPD-L3 codes
   and free-text locality names. A continent rollup needs an external
   WGSRPD code table, not a WCVP column; flag for SP2's `in_area` ranking
   design.

None of these are blockers — all are addressable in the importer/mapping
layer with the fixture now in place to write SP1's importer tests against.

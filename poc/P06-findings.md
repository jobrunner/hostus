# PoC P6 — Trait vocabulary formats + taxonomic join-keys (Phase 0, Task 0.6, Gate SP3)

## Goal

Verify the hostus 2.0 spec §UC1 assumptions about the three trait vocabularies
(EIVE 1.0, Tichý et al. 2023, Midolo et al. 2023): file format, dimensions,
scale, niche-width columns, and — critically — the taxonomic join-key column
each table keys on, given the known R1 blocker that the underlying backbones
(Euro+Med PlantBase, GermanSL, EuroSL) are ❌ (no findable license / no bulk
export; see `docs/research/quellenregister.md`).

## Method

Shell/curl + python3/openpyxl probe (`poc/p06_traits/inspect.sh`), run inside
the project's Nix dev shell (`nix develop -c bash poc/p06_traits/inspect.sh`).
Files fetched via the Zenodo record API (`GET /api/records/<id>`) to resolve
the current file keys/URLs, then downloaded to `poc/data/` (gitignored).
`openpyxl` (available in the nix devshell's `python3`) was used to dump sheet
names, header rows and sample rows for `.xlsx` files; the Midolo main table
(`.csv`) was read directly. No code touched outside `poc/p06_traits/` and this
file.

Files inspected:
- EIVE: `EIVE_Paper_1.0_SM_02.xlsx` (per-source-system long table, 31 EIV
  systems x taxon) and `EIVE_Paper_1.0_SM_08.xlsx` (aggregated species-level
  `mainTable` — the actual join target for SP3), Zenodo 10.5281/zenodo.7534792.
- Tichý: `Indicator.values-tables-2022-11-07-Zenodo.v2.xlsx`, Zenodo
  10.5281/zenodo.7427088 (v2.0).
- Midolo: `disturbance_indicator_values.csv` (main index) +
  `disturbance-habitat.xlsx` (EUNIS habitat crosswalk), Zenodo
  10.5281/zenodo.7116957.

## Results

| Vocabulary | File(s) / format | Dimensions | Scale | Niche-width column? | Join-key column | Taxonomy namespace it keys on |
|---|---|---|---|---|---|---|
| **EIVE 1.0** | `EIVE_Paper_1.0_SM_08.xlsx`, sheet `mainTable` (.xlsx, 14 835 taxa) | **M, N, R, L, T** (`EIVEres-M/N/R/L/T`) | **Uniform continuous 0–10** for all 5 dims (empirically confirmed: min=0, max=10 across all 14 835 rows for each dimension) | **Yes** — `EIVEres-{M,N,R,L,T}.nw3` (niche width, EIVE-specific) + `EIVEres-{M,N,R,L,T}.n` (count of contributing source systems, 0–31) | `TaxonConcept` (free-text scientific name string, e.g. `"×Aegilotriticum triticoides"`) — plus a `UUID` column that is an **EIVE-internal** taxon-concept ID, not an external backbone key | Euro+Med-aligned nomenclature (per EIVE methodology: names harmonised against Euro+Med PlantBase / WFO via the project's own concordance work in SM_02/SM_03, not via a resolvable EuroSL/Euro+Med ID) |
| **Tichý et al. 2023** | `Indicator.values-tables-2022-11-07-Zenodo.v2.xlsx` (.xlsx, 10 sheets; final values in `Tab-IVs-Tichy-et-al2023`, 8 909 taxa) | **L, T, M, R, N + S (Salinity)** — all 6 present (`LIGHT`, `TEMPERATURE`, `MOISTURE`, `REACTION`, `NUTRIENTS`, `SALINITY` sheets, plus a combined `Tab-IVs-Tichy-et-al2023` sheet) | **Varying, Ellenberg-compatible ordinal-ish scales**, not normalised to one range: Light/Temperature/Moisture/Reaction/Nutrients ≈ classic Ellenberg 1–9 (with `x`/`NA` for indifferent/missing), Salinity ≈ a distinct signed scale centred on 0 (observed range roughly −0.02 to 0 in the sample, needs full-range check before SP3 implementation) | No dedicated niche-width column found in any of the 10 sheets (only `Average/Median/Min/Max/Count` per per-database sheet, and a single `Average` in the final combined sheet) | `Taxon` (free-text scientific name, e.g. `"Abies alba"`) in every values sheet; `SeqID` is an internal sequential row ID, not a taxonomic identifier | Euro+Med-aligned nomenclature per-source-database, harmonised by the paper's own name-matching pipeline (13 national/regional Ellenberg-type source lists incl. Julve/France, Chytrý/Czechia, Landolt/Switzerland, etc. — described in sheet `Description of IVs`); no FloraVeg.EU/ESy ID column present in the delivered table itself |
| **Midolo et al. 2023** | `disturbance_indicator_values.csv` (CSV, main index, 3 000+ taxa) + `disturbance-habitat.xlsx` sheet `List1` (256 EUNIS habitat rows, the trait→habitat crosswalk) | **Disturbance.Severity, Disturbance.Frequency** (whole community + herb-layer variants) plus derived **Mowing.Frequency, Grazing.Pressure, Soil.Disturbance** — this is the "Disturbance" indicator set the spec references, confirmed present | Continuous indicator values (not integer-scaled); SD_* columns give per-taxon standard deviation, not a fixed min/max scale; habitat crosswalk sheet uses EUNIS 2021 codes with severity 0–1-ish and frequency in years | No EIVE-style niche-width column; only SD_* (standard deviation) per indicator, which is a different concept | `species` (free-text scientific name, e.g. `"Abies alba"`) in the main CSV, plus `family`; `EUNIS 2021 code` / `Red List code` in the crosswalk (habitat namespace, not taxon namespace) | Taxon names harmonised against **EUNIS habitat/vegetation-plot databases** (the paper computes disturbance indicator values from EVA vegetation-plot occurrence×habitat data) — no explicit Euro+Med/GermanSL/EuroSL ID column in either delivered file |

## Format/structure verdict vs. spec §UC1 claims

- **EIVE**: ✅ dimensions M,N,R,L,T confirmed; ✅ uniform 0–10 continuous scale
  confirmed empirically (all 5 dims, full table scan); ✅ niche-width columns
  present (`.nw3`) and EIVE-specific (not found in Tichý/Midolo); ✅ `n`
  source-systems count column present (`.n`, 0–31 range).
- **Tichý**: ✅ dimensions L,T,M,R,N confirmed; ✅ Salinity (S) present as a
  6th dimension, confirmed as spec claims (not part of EIVE); ✅ scales vary
  per dimension / are Ellenberg-compatible rather than uniformly rescaled
  (unlike EIVE); dedicated per-database sheets (Austria, Cantabrian range,
  Czech Republic, European mires, etc.) show the 13-source compilation
  structure the spec describes as "ESy/FloraVeg adaptations" — though no
  literal ESy/FloraVeg identifier column exists in the delivered table.
- **Midolo**: ✅ Disturbance severity/frequency indicators confirmed present
  (whole-community and herb-layer variants), plus the expected habitat
  crosswalk file (EUNIS-code keyed, 256 rows) as a **separate** file from the
  species-level main index — confirming the spec's "habitat crosswalk"
  claim as a join between two files, not one.

## The join blocker (concrete, per R1)

All three tables key on **free-text scientific-name strings** (`TaxonConcept`
in EIVE, `Taxon` in Tichý, `species` in Midolo) harmonised through each
project's own internal name-matching pipeline against **Euro+Med PlantBase
nomenclature** (EIVE, Tichý) or **EVA/EUNIS vegetation-plot taxonomy**
(Midolo) — **none of the three delivered files contain a resolvable external
ID column** (no GermanSL ID, no EuroSL ID, no Euro+Med taxon ID, no WCVP ID,
no GBIF/COL key). The join key is the bare name string itself.

This directly triggers the R1 blocker already documented in
`docs/research/quellenregister.md`:

- Euro+Med PlantBase (the taxonomy EIVE and Tichý harmonise their names
  against): ❌ no findable license, no bulk/API export.
- GermanSL: ❌ no findable license.
- EuroSL (re-exports Euro+Med + GermanSL): ❌ no findable license, inherits
  Euro+Med's risk.

**Impact on SP3**: hostus 2.0 cannot legally redistribute a EuroSL/Euro+Med
lookup table as the join layer between GBIF-served names and these three
trait tables' `TaxonConcept`/`Taxon`/`species` strings, because that lookup
table itself would be (or derive from) an unlicensed backbone. Since all
three trait vocabularies use free-text names rather than resolvable external
IDs anyway, this is not merely an inconvenience — there was never a licensed
ID-to-ID join path available even if EuroSL/Euro+Med were licensed; the
practical join has always had to be name-string matching.

**R1 fallback (unblocks SP3):** build **direct crosswalks between the three
trait tables and WCVP/COL-XR names** (both ✅ validated, open, bulk-exportable
per the source register) by fuzzy/exact name-matching each table's raw
`TaxonConcept`/`Taxon`/`species` string against WCVP's `wcvp_taxon.csv`
accepted+synonym name list (and/or COL-XR's `Name.tsv`), entirely bypassing
EuroSL/GermanSL/Euro+Med as an intermediate namespace. This is more
implementation work (a name-normalisation + fuzzy-match step per vocabulary,
with manual QA on ambiguous/hybrid names such as the `×Aegilotriticum` example
observed in the EIVE sample) but requires no third-party unlicensed
redistribution — matches the fallback already prescribed in the source
register's Blocker table for GermanSL/EuroSL under SP3.

## Verdict

**⚠️ (trait tables obtainable + formats verified; taxonomic join namespace
licensing-blocked — SP3 needs the crosswalk fallback)**

All three vocabularies are freely downloadable (CC-BY-4.0, open Zenodo
records, no auth), and their table structures match the spec's §UC1 claims
in every dimension checked (EIVE's 0–10 uniform scale + niche-width + n-count;
Tichý's 6-dimension Ellenberg-compatible varying scales including Salinity;
Midolo's disturbance severity/frequency + separate habitat crosswalk). But
none of the three tables carry a resolvable external taxonomic ID — they all
join on bare name strings harmonised against Euro+Med-family taxonomies that
are R1-blocked for redistribution. SP3 must implement the WCVP/COL-XR direct
name-crosswalk fallback rather than relying on a licensed EuroSL/Euro+Med
intermediate layer, which does not exist.

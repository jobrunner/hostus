# Trait pipelines (xlsx/csv → canonical CSV)

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

## Canonical CSV contract

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

## Running a pipeline

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

## Observed summaries (run 2026-08-01, against the real PoC P6 downloads)

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

## Attribution (CC-BY-4.0 — required per vocabulary)

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

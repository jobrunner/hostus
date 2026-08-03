# Fixture provenance

`eive-sample.csv`, `tichy-sample.csv`, `midolo-sample.csv` are a 5-taxon cut
of the real canonical CSVs produced by `pipelines/{eive,tichy,midolo}/build.sh`
against the actual downloads from PoC P6 (`poc/data/`), run 2026-08-01:

- `pipelines/eive/build.sh` → `pipelines/eive/output/eive-canonical.csv`
  (source: EIVE_Paper_1.0_SM_08.xlsx, EIVE 1.0, Zenodo 10.5281/zenodo.7534792)
- `pipelines/tichy/build.sh` → `pipelines/tichy/output/tichy-canonical.csv`
  (source: Indicator.values-tables-2022-11-07-Zenodo.v2.xlsx, Tichý et al.
  2023 v2.0, Zenodo 10.5281/zenodo.7427088)
- `pipelines/midolo/build.sh` → `pipelines/midolo/output/midolo-canonical.csv`
  (source: disturbance_indicator_values.csv, Midolo et al. 2023 v3, Zenodo
  10.5281/zenodo.7116957)

Each sample file is exactly the header row plus every row for 5 taxa,
selected from the full pipeline output with no other modification:

- **Matchable against the WCVP fixture** (`internal/adapters/wcvp/testdata/wcvp-sample`),
  the SP1 reference taxa: *Corynephorus canescens*, *Jacobaea vulgaris*,
  *Festuca ovina* — all three are present in every vocabulary's real output.
- **Deliberately NOT in the WCVP fixture** (to exercise T4's unmatched
  crosswalk path): *Abies alba*, *Quercus robur* — both genera do not appear
  in `wcvp_taxon.csv` of the WCVP fixture (which only covers Poaceae/
  Asteraceae genera: Avena, Bromus, Corynephorus, Festuca, Jacobaea, Senecio,
  Weingaertneria).

Row counts: 25 data rows per vocabulary (5 taxa x 5 EIVE dims; 5 taxa x
4-6 Tichý dims depending on data availability per taxon; 5 taxa x 5 Midolo
disturbance dims).

Attribution (CC-BY-4.0, all three vocabularies) is documented in
`pipelines/README.md`.

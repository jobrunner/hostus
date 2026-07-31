#!/usr/bin/env bash
# PoC P6 (Phase 0, Task 0.6): verify format + taxonomic join-keys of the three
# trait vocabularies EIVE 1.0, Tichy et al. 2023, Midolo et al. 2023 (Gate SP3).
#
# Downloads the primary data files from Zenodo (record API resolved file URLs)
# into poc/data/ (gitignored), then dumps sheet/file names, header row, and a
# few sample rows for each, using python3/openpyxl for xlsx and plain
# head/column inspection for csv.
set -euo pipefail

DATA_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)/data"
mkdir -p "$DATA_DIR"

download() {
  local label="$1" url="$2" out="$3"
  if [[ -f "${DATA_DIR}/${out}" ]]; then
    echo "=== ${label}: already downloaded (${out}) ==="
    return
  fi
  echo "=== ${label}: downloading ==="
  echo "GET ${url}"
  curl -sSL "${url}" -o "${DATA_DIR}/${out}"
  echo "  -> ${DATA_DIR}/${out} ($(stat -f%z "${DATA_DIR}/${out}" 2>/dev/null || stat -c%s "${DATA_DIR}/${out}") bytes)"
}

# --- 1) EIVE 1.0 (Zenodo 10.5281/zenodo.7534792) -----------------------------
# SM_02 = per-source-system long table (31 EIV systems x taxon, raw+rescaled).
# SM_08 = the final aggregated species-level "mainTable" (EIVEres-* + niche
#         width .nw3 + n-source-systems .n) - this is the join target for SP3.
download "EIVE-SM02" \
  "https://zenodo.org/api/records/7534792/files/EIVE_Paper_1.0_SM_02.xlsx/content" \
  "eive_sm02.xlsx"
download "EIVE-SM08" \
  "https://zenodo.org/api/records/7534792/files/EIVE_Paper_1.0_SM_08.xlsx/content" \
  "eive_sm08.xlsx"

# --- 2) Tichy et al. 2023 (Zenodo v2.0, 10.5281/zenodo.7427088) -------------
download "Tichy" \
  "https://zenodo.org/api/records/7427088/files/Indicator.values-tables-2022-11-07-Zenodo.v2.xlsx/content" \
  "tichy_indicator_values_v2.xlsx"

# --- 3) Midolo et al. 2023 (Zenodo 10.5281/zenodo.7116957) ------------------
download "Midolo-CSV" \
  "https://zenodo.org/api/records/7116957/files/disturbance_indicator_values.csv/content" \
  "midolo_disturbance_indicator_values.csv"
download "Midolo-XLSX" \
  "https://zenodo.org/api/records/7116957/files/disturbance-habitat.xlsx/content" \
  "midolo_disturbance_habitat_crosswalk.xlsx"

echo
echo "############################################################"
echo "# Inspecting downloaded files"
echo "############################################################"
echo

python3 - "$DATA_DIR" <<'PYEOF'
import sys, csv
from pathlib import Path
import openpyxl

data_dir = Path(sys.argv[1])

def dump_xlsx(path, max_rows=3, max_cols=25):
    print(f"--- {path.name} ---")
    wb = openpyxl.load_workbook(path, read_only=True, data_only=True)
    print("sheets:", wb.sheetnames)
    for sheet_name in wb.sheetnames:
        ws = wb[sheet_name]
        print(f"\n[sheet: {sheet_name}] rows~={ws.max_row} cols~={ws.max_column}")
        rows_iter = ws.iter_rows(values_only=True)
        header = next(rows_iter, None)
        if header is None:
            print("  (empty sheet)")
            continue
        header = header[:max_cols]
        print("  header:", header)
        for i, row in enumerate(rows_iter):
            if i >= max_rows:
                break
            print(f"  row{i+1}:", row[:max_cols])
    print()

def dump_csv(path, max_rows=3, max_cols=25):
    print(f"--- {path.name} ---")
    with path.open(newline='', encoding='utf-8-sig') as f:
        reader = csv.reader(f)
        header = next(reader, None)
        print("header:", header[:max_cols] if header else None)
        for i, row in enumerate(reader):
            if i >= max_rows:
                break
            print(f"row{i+1}:", row[:max_cols])
    print()

dump_xlsx(data_dir / "eive_sm02.xlsx")
dump_xlsx(data_dir / "eive_sm08.xlsx")
dump_xlsx(data_dir / "tichy_indicator_values_v2.xlsx")
dump_csv(data_dir / "midolo_disturbance_indicator_values.csv")
dump_xlsx(data_dir / "midolo_disturbance_habitat_crosswalk.xlsx")
PYEOF

echo "Inspection complete. Raw files retained under ${DATA_DIR} (gitignored)."

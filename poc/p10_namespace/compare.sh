#!/usr/bin/env bash
# PoC P10 — FloraVeg.EU / EUNIS-ESy namespace vs EIVE / Euro+Med namespace divergence probe.
#
# Gate: SP3/UC4. Spec §4.4 + §D.4 assume the two namespaces are genuinely
# different taxonomic concepts and must NOT be merged/conflated when storing
# trait values per taxon. This script pulls the actual name strings from both
# sides for two Sandtrockenrasen-critical genera (Festuca ovina group, Thymus)
# and prints them side by side so the divergence (or lack of it) is visible
# in plain text, not asserted from memory.
#
# Inputs (fetched into poc/data/, gitignored, reused across PoCs):
#   - poc/data/eive_sm08.xlsx        EIVE 1.0 SM_08, sheet "mainTable"
#                                     (Zenodo 10.5281/zenodo.7534792) — keys on
#                                     free-text `TaxonConcept` column, Euro+Med-
#                                     aligned nomenclature (see PoC P6).
#   - poc/data/esy_2021-06-01.txt    EUNIS-ESy expert system v2021-06-01
#                                     (Zenodo 10.5281/zenodo.4812736), SECTION 1
#                                     "Species aggregation": accepted concept
#                                     name followed by indented synonym/
#                                     segregate names folded under it.
#
# Run inside the project's Nix dev shell:
#   nix develop -c bash poc/p10_namespace/compare.sh
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
DATA_DIR="${REPO_ROOT}/poc/data"

EIVE_XLSX="${DATA_DIR}/eive_sm08.xlsx"
ESY_TXT="${DATA_DIR}/esy_2021-06-01.txt"
ESY_ZENODO_RECORD="4812736"

mkdir -p "${DATA_DIR}"

if [ ! -f "${EIVE_XLSX}" ]; then
  echo "ERROR: ${EIVE_XLSX} not found. Reuse the P6 download (poc/p06_traits/inspect.sh) first." >&2
  exit 1
fi

if [ ! -f "${ESY_TXT}" ]; then
  echo "Fetching EUNIS-ESy expert system from Zenodo record ${ESY_ZENODO_RECORD}..." >&2
  curl -sS -o "${ESY_TXT}" \
    "https://zenodo.org/api/records/${ESY_ZENODO_RECORD}/files/EUNIS-ESy-2021-06-01.txt/content"
fi

echo "################################################################"
echo "# 1. EIVE (Euro+Med-aligned) — TaxonConcept rows for the tested genera"
echo "#    Source: eive_sm08.xlsx, sheet mainTable, column TaxonConcept"
echo "#    Each row is an INDEPENDENT trait concept (own EIVEres-M/N/R/L/T values)."
echo "################################################################"
python3 - "${EIVE_XLSX}" <<'PYEOF'
import openpyxl, sys
wb = openpyxl.load_workbook(sys.argv[1], read_only=True, data_only=True)
ws = wb["mainTable"]
rows = ws.iter_rows(values_only=True)
header = next(rows)
tc_idx = header.index("TaxonConcept")

targets = [
    ("Festuca ovina group", ["Festuca ovina", "Festuca filiformis", "Festuca lemanii", "Festuca guestfalica"]),
    ("Thymus praecox group", ["Thymus praecox", "Thymus jankae"]),
]

all_rows = [r[tc_idx] for r in rows if r[tc_idx]]
for label, prefixes in targets:
    print(f"\n-- {label} --")
    for name in all_rows:
        n = str(name)
        if any(n == p or n.startswith(p + " ") for p in prefixes):
            print(f"  EIVE TaxonConcept: {n}")
PYEOF

echo
echo "################################################################"
echo "# 2. EUNIS-ESy / FloraVeg (Section 1: Species aggregation)"
echo "#    Source: esy_2021-06-01.txt"
echo "#    Format: accepted concept name, followed by indented lines listing"
echo "#    every name TREATED AS THE SAME CONCEPT (synonyms/segregates folded"
echo "#    into that one accepted concept for the vegetation-plot database)."
echo "################################################################"
echo
# esy_2021-06-01.txt has CRLF line endings; normalize to LF once so the
# blank-line block terminators below actually match. Extract SECTION 1 body
# once (avoids awk-piped-to-awk SIGPIPE aborting the script under `set -o
# pipefail` when the downstream extractor exits early).
ESY_LF="${DATA_DIR}/esy_2021-06-01.lf.txt"
tr -d '\r' < "${ESY_TXT}" > "${ESY_LF}"
ESY_SECTION1="${DATA_DIR}/esy_2021-06-01.section1.txt"
awk '/^SECTION 1:/{s=1; next} s && /^SECTION 1: End/{exit} s' "${ESY_LF}" > "${ESY_SECTION1}"

print_block() {
  # Print the accepted-concept block starting at a line matching $1
  # (name followed by whitespace/dash), up to the next blank line.
  awk -v pat="^${1}[ \t]" '$0 ~ pat {p=1} p && /^$/{p=0} p'  "${ESY_SECTION1}"
}

echo "-- Festuca ovina block (accepted concept + everything folded under it) --"
print_block "Festuca ovina"

echo
echo "-- Festuca filiformis block --"
print_block "Festuca filiformis"

echo
echo "-- Thymus praecox block --"
print_block "Thymus praecox"

echo
echo "################################################################"
echo "# 3. Divergence check: does ESy fold a name that EIVE lists as an"
echo "#    INDEPENDENT trait row into a DIFFERENT accepted concept?"
echo "################################################################"
echo
echo "'Festuca lemanii' appears in EIVE as its own TaxonConcept row (see 1)."
echo "In ESy Section 1 it appears here:"
grep -n "Festuca lemanii" "${ESY_TXT}" || echo "  (not found)"

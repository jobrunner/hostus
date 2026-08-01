#!/usr/bin/env bash
# FloraVeg.EU "Life_form" table -> canonical name-list CSV.
#
# Source: https://floraveg.eu/download/ -> Life_form.xlsx
# (https://files.ibot.cas.cz/cevs/downloads/floraveg/Life_form.xlsx).
# See convert.py's docstring for why this file was chosen over FloraVeg's
# other per-topic Excel downloads (no dedicated taxon-checklist export
# exists; this is the simplest complete per-taxon table; the ESy expert
# system is explicitly out of scope).
# No findable license -> redistribution: unknown/restricted. LOCAL
# EVALUATION USE ONLY, never redistributed in an exported bundle
# (enforced by the redistribution gate, Task 1).
#
# Emits pipe-delimited canonical CSV: taxon|rank|status|accepted_taxon|source_id
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

SOURCE_URL="https://files.ibot.cas.cz/cevs/downloads/floraveg/Life_form.xlsx"
SOURCE_FILE="Life_form.xlsx"

CACHE_DIR="${SCRIPT_DIR}/.cache"
OUT_DIR="${SCRIPT_DIR}/output"
mkdir -p "${CACHE_DIR}" "${OUT_DIR}"

SRC_PATH="${CACHE_DIR}/${SOURCE_FILE}"
OUT_PATH="${OUT_DIR}/floraveg-canonical.csv"
SUMMARY_PATH="${SCRIPT_DIR}/floraveg.summary.txt"

if [[ -f "${SRC_PATH}" ]]; then
  echo "FloraVeg: using cached ${SRC_PATH}"
else
  echo "FloraVeg: downloading ${SOURCE_URL}"
  curl -sSL -A "hostus-pipeline-floraveg/0.1 (research)" "${SOURCE_URL}" -o "${SRC_PATH}"
fi

{
  echo "source=${SOURCE_URL}"
  python3 "${SCRIPT_DIR}/convert.py" "${SRC_PATH}" "${OUT_PATH}"
} | tee "${SUMMARY_PATH}"

echo "FloraVeg: canonical CSV written to ${OUT_PATH}"
echo "FloraVeg: summary written to ${SUMMARY_PATH}"

#!/usr/bin/env bash
# EuroSL (infinitenature.org) -> canonical name-list CSV.
#
# Source: https://eurosl.infinitenature.org/downloads/ -> "latest version
# sqlite file" (a single EuroSL.sqlite, no versioned filename on the site;
# the file's own Version table records its build date, printed below).
# A single SQLite file, table "EuroPlusMed.Plantae" — read directly with
# python3's stdlib sqlite3 module (sqlite3 CLI is also present in the Nix
# devshell if a manual look is wanted).
# No findable license -> redistribution: unknown/restricted. LOCAL
# EVALUATION USE ONLY, never redistributed in an exported bundle
# (enforced by the redistribution gate, Task 1).
#
# Emits pipe-delimited canonical CSV: taxon|rank|status|accepted_taxon|source_id
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

SOURCE_URL="https://eurosl.infinitenature.org/EuroSL/latest/EuroSL.sqlite"
SOURCE_FILE="EuroSL.sqlite"

CACHE_DIR="${SCRIPT_DIR}/.cache"
OUT_DIR="${SCRIPT_DIR}/output"
mkdir -p "${CACHE_DIR}" "${OUT_DIR}"

SRC_PATH="${CACHE_DIR}/${SOURCE_FILE}"
OUT_PATH="${OUT_DIR}/eurosl-canonical.csv"
SUMMARY_PATH="${SCRIPT_DIR}/eurosl.summary.txt"

if [[ -f "${SRC_PATH}" ]]; then
  echo "EuroSL: using cached ${SRC_PATH}"
else
  echo "EuroSL: downloading ${SOURCE_URL}"
  curl -sSL -A "hostus-pipeline-eurosl/0.1 (research)" "${SOURCE_URL}" -o "${SRC_PATH}"
fi

VERSION="$(sqlite3 "${SRC_PATH}" 'SELECT * FROM Version LIMIT 1;' 2>/dev/null || echo unknown)"

{
  echo "source=${SOURCE_URL}"
  echo "version=${VERSION}"
  python3 "${SCRIPT_DIR}/convert.py" "${SRC_PATH}" "${OUT_PATH}"
} | tee "${SUMMARY_PATH}"

echo "EuroSL: canonical CSV written to ${OUT_PATH}"
echo "EuroSL: summary written to ${SUMMARY_PATH}"

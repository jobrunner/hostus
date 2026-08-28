#!/usr/bin/env bash
# GermanSL 1.5.6 (infinitenature.org) -> canonical name-list CSV.
#
# Source: https://germansl.infinitenature.org/downloads/ -> GermanSL 1.5 zip
# (unversioned "latest" link, GermanSL.zip, analogous to EuroSL's pattern),
# which bundles both the TURBOVEG export (species.dbf, ecodbase.dbf) and a
# spreadsheet export (GermanSL1.5.6.xlsx, sheet "TCS"). Re-pinned from the
# previously cached 1.5.5 build (verified via HEAD: different
# Content-Length, Last-Modified 2025-11-28; the zip's own xlsx member name
# confirms the 1.5.6 bump).
# The xlsx is preferred here per the task brief (simpler than the TURBOVEG
# dbf pair) even though it ships with a broken <dimension> declaration that
# openpyxl mis-reads (see convert.py's docstring for the workaround).
# No findable license -> redistribution: unknown/restricted. LOCAL
# EVALUATION USE ONLY, never redistributed in an exported bundle
# (enforced by the redistribution gate, Task 1).
#
# Emits pipe-delimited canonical CSV:
# taxon|rank|status|accepted_taxon|source_id|parent_id|parent_rank|vernacular_de
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

SOURCE_URL="https://germansl.infinitenature.org/GermanSL/1.5/GermanSL.zip"
SOURCE_ZIP="GermanSL.zip"
# The "latest" link at SOURCE_URL is unversioned like EuroSL's; its content
# no longer matches the pinned 1.5.5 build (verified: different
# Content-Length, Last-Modified 2025-11-28). The zip's own xlsx member name
# (GermanSL1.5.6.xlsx) carries the real version bump.
VERSION="1.5.6"

CACHE_DIR="${SCRIPT_DIR}/.cache"
OUT_DIR="${SCRIPT_DIR}/output"
mkdir -p "${CACHE_DIR}" "${OUT_DIR}"

ZIP_PATH="${CACHE_DIR}/${SOURCE_ZIP}"
XLSX_PATH="${CACHE_DIR}/GermanSL1.5.6.xlsx"
OUT_PATH="${OUT_DIR}/germansl-canonical.csv"
SUMMARY_PATH="${SCRIPT_DIR}/germansl.summary.txt"

if [[ -f "${ZIP_PATH}" ]]; then
  echo "GermanSL: using cached ${ZIP_PATH}"
else
  echo "GermanSL: downloading ${SOURCE_URL}"
  curl -sSL -A "hostus-pipeline-germansl/0.1 (research)" "${SOURCE_URL}" -o "${ZIP_PATH}"
fi

if [[ ! -f "${XLSX_PATH}" ]]; then
  echo "GermanSL: extracting xlsx from zip"
  unzip -p "${ZIP_PATH}" "GermanSL 1.5/GermanSL1.5.6.xlsx" > "${XLSX_PATH}"
fi

{
  echo "source=${SOURCE_URL}"
  echo "version=${VERSION}"
  python3 "${SCRIPT_DIR}/convert.py" "${XLSX_PATH}" "${OUT_PATH}"
} | tee "${SUMMARY_PATH}"

echo "GermanSL: canonical CSV written to ${OUT_PATH}"
echo "GermanSL: summary written to ${SUMMARY_PATH}"

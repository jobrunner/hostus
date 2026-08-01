#!/usr/bin/env bash
# Tichý et al. 2023 (Ellenberg-type indicator values) -> canonical trait CSV.
#
# Source: Zenodo record 10.5281/zenodo.7427088 (v2.0), file
# Indicator.values-tables-2022-11-07-Zenodo.v2.xlsx, sheet
# "Tab-IVs-Tichy-et-al2023" (combined final values) per poc/P06-findings.md.
# CC-BY-4.0 — any redistribution of derived data must retain attribution to
# Tichý et al. (2023), Journal of Vegetation Science 34: e13168,
# https://doi.org/10.1111/jvs.13168.
#
# Emits pipe-delimited canonical CSV: taxon|vocab|vocab_version|dim|value|niche_width|n_systems
# Tichý provides no niche-width or source-system-count column, so both
# fields are always empty.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"

VOCAB="tichy2023"
VOCAB_VERSION="2.0"
SOURCE_URL="https://zenodo.org/api/records/7427088/files/Indicator.values-tables-2022-11-07-Zenodo.v2.xlsx/content"
SOURCE_FILE="tichy_indicator_values_v2.xlsx"

CACHE_DIR="${SCRIPT_DIR}/.cache"
OUT_DIR="${SCRIPT_DIR}/output"
mkdir -p "${CACHE_DIR}" "${OUT_DIR}"

SRC_PATH="${CACHE_DIR}/${SOURCE_FILE}"
OUT_PATH="${OUT_DIR}/tichy-canonical.csv"
SUMMARY_PATH="${SCRIPT_DIR}/tichy.summary.txt"

if [[ -f "${SRC_PATH}" ]]; then
  echo "Tichy: using cached ${SRC_PATH}"
elif [[ -f "${REPO_ROOT}/poc/data/${SOURCE_FILE}" ]]; then
  echo "Tichy: reusing PoC P6 download ${REPO_ROOT}/poc/data/${SOURCE_FILE}"
  cp "${REPO_ROOT}/poc/data/${SOURCE_FILE}" "${SRC_PATH}"
else
  echo "Tichy: downloading ${SOURCE_URL}"
  curl -sSL "${SOURCE_URL}" -o "${SRC_PATH}"
fi

python3 "${SCRIPT_DIR}/convert.py" "${SRC_PATH}" "${OUT_PATH}" "${VOCAB}" "${VOCAB_VERSION}" | tee "${SUMMARY_PATH}"

echo "Tichy: canonical CSV written to ${OUT_PATH}"
echo "Tichy: summary written to ${SUMMARY_PATH}"

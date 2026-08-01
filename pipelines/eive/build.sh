#!/usr/bin/env bash
# EIVE 1.0 (Dengler et al. 2023) -> canonical trait CSV.
#
# Source: Zenodo record 10.5281/zenodo.7534792, file
# EIVE_Paper_1.0_SM_08.xlsx, sheet "mainTable" (aggregated species-level
# table, the actual SP3 join target per poc/P06-findings.md). CC-BY-4.0 —
# any redistribution of derived data must retain attribution to Dengler et
# al. (2023), Vegetation Classification and Survey 4: 7-29,
# https://doi.org/10.3897/VCS.98324.
#
# Emits pipe-delimited canonical CSV: taxon|vocab|vocab_version|dim|value|niche_width|n_systems
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"

VOCAB="eive"
VOCAB_VERSION="1.0"
SOURCE_URL="https://zenodo.org/api/records/7534792/files/EIVE_Paper_1.0_SM_08.xlsx/content"
SOURCE_FILE="eive_sm08.xlsx"

CACHE_DIR="${SCRIPT_DIR}/.cache"
OUT_DIR="${SCRIPT_DIR}/output"
mkdir -p "${CACHE_DIR}" "${OUT_DIR}"

SRC_PATH="${CACHE_DIR}/${SOURCE_FILE}"
OUT_PATH="${OUT_DIR}/eive-canonical.csv"
SUMMARY_PATH="${SCRIPT_DIR}/eive.summary.txt"

if [[ -f "${SRC_PATH}" ]]; then
  echo "EIVE: using cached ${SRC_PATH}"
elif [[ -f "${REPO_ROOT}/poc/data/${SOURCE_FILE}" ]]; then
  echo "EIVE: reusing PoC P6 download ${REPO_ROOT}/poc/data/${SOURCE_FILE}"
  cp "${REPO_ROOT}/poc/data/${SOURCE_FILE}" "${SRC_PATH}"
else
  echo "EIVE: downloading ${SOURCE_URL}"
  curl -sSL "${SOURCE_URL}" -o "${SRC_PATH}"
fi

python3 "${SCRIPT_DIR}/convert.py" "${SRC_PATH}" "${OUT_PATH}" "${VOCAB}" "${VOCAB_VERSION}" | tee "${SUMMARY_PATH}"

echo "EIVE: canonical CSV written to ${OUT_PATH}"
echo "EIVE: summary written to ${SUMMARY_PATH}"

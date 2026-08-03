#!/usr/bin/env bash
# Midolo et al. 2023 (disturbance indicator values) -> canonical trait CSV.
#
# Source: Zenodo record 10.5281/zenodo.7116957 (v3, Dec 2022), main index
# file disturbance_indicator_values.csv (already CSV -- no xlsx conversion
# needed) per poc/P06-findings.md. CC-BY-4.0 -- any redistribution of
# derived data must retain attribution to Midolo et al. (2023), Global
# Ecology and Biogeography 32(1): 24-34, https://doi.org/10.1111/geb.13603.
#
# Emits pipe-delimited canonical CSV: taxon|vocab|vocab_version|dim|value|niche_width|n_systems
# Midolo has no EIVE-style niche-width column, and its SD_* columns are
# per-indicator standard deviations, NOT a niche-width equivalent -- they
# are deliberately not mapped to niche_width. niche_width and n_systems are
# therefore always empty.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"

VOCAB="midolo2023"
VOCAB_VERSION="3"
SOURCE_URL="https://zenodo.org/api/records/7116957/files/disturbance_indicator_values.csv/content"
SOURCE_FILE="midolo_disturbance_indicator_values.csv"

CACHE_DIR="${SCRIPT_DIR}/.cache"
OUT_DIR="${SCRIPT_DIR}/output"
mkdir -p "${CACHE_DIR}" "${OUT_DIR}"

SRC_PATH="${CACHE_DIR}/${SOURCE_FILE}"
OUT_PATH="${OUT_DIR}/midolo-canonical.csv"
SUMMARY_PATH="${SCRIPT_DIR}/midolo.summary.txt"

if [[ -f "${SRC_PATH}" ]]; then
  echo "Midolo: using cached ${SRC_PATH}"
elif [[ -f "${REPO_ROOT}/poc/data/${SOURCE_FILE}" ]]; then
  echo "Midolo: reusing PoC P6 download ${REPO_ROOT}/poc/data/${SOURCE_FILE}"
  cp "${REPO_ROOT}/poc/data/${SOURCE_FILE}" "${SRC_PATH}"
else
  echo "Midolo: downloading ${SOURCE_URL}"
  curl -sSL "${SOURCE_URL}" -o "${SRC_PATH}"
fi

python3 "${SCRIPT_DIR}/convert.py" "${SRC_PATH}" "${OUT_PATH}" "${VOCAB}" "${VOCAB_VERSION}" | tee "${SUMMARY_PATH}"

echo "Midolo: canonical CSV written to ${OUT_PATH}"
echo "Midolo: summary written to ${SUMMARY_PATH}"

#!/usr/bin/env bash
# PoC P2 (Phase 0, Task 0.2): verify the real structure of the WCVP dwca zip.
#
# Downloads wcvp_dwca.zip into poc/data/ (gitignored), unzips it, and prints
# enough of the header/delimiter/sample-row/metadata structure to confirm (or
# refute) the spec's assumptions about the WCVP schema.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/../.." && pwd)"
DATA_DIR="${ROOT_DIR}/poc/data"
ZIP_PATH="${DATA_DIR}/wcvp_dwca.zip"
EXTRACT_DIR="${DATA_DIR}/wcvp_dwca"

PRIMARY_URL="https://sftp.kew.org/pub/data-repositories/WCVP/wcvp_dwca.zip"
FALLBACK_URL="https://hosted-datasets.gbif.org/datasets/wcvp.zip"

mkdir -p "${DATA_DIR}"

if [ ! -f "${ZIP_PATH}" ]; then
  echo "== Downloading WCVP archive ==" >&2
  if curl -fL --retry 3 --max-time 300 -o "${ZIP_PATH}" "${PRIMARY_URL}"; then
    echo "Downloaded from primary URL: ${PRIMARY_URL}" >&2
  else
    echo "Primary URL failed, trying fallback: ${FALLBACK_URL}" >&2
    rm -f "${ZIP_PATH}"
    curl -fL --retry 3 --max-time 300 -o "${ZIP_PATH}" "${FALLBACK_URL}"
    echo "Downloaded from fallback URL: ${FALLBACK_URL}" >&2
  fi
else
  echo "== Reusing cached archive: ${ZIP_PATH} ==" >&2
fi

echo "== Archive size ==" >&2
ls -lh "${ZIP_PATH}"

echo "== Archive contents ==" >&2
unzip -l "${ZIP_PATH}"

rm -rf "${EXTRACT_DIR}"
mkdir -p "${EXTRACT_DIR}"
unzip -o -q "${ZIP_PATH}" -d "${EXTRACT_DIR}"

echo
echo "== Extracted files =="
ls -la "${EXTRACT_DIR}"

for meta in eml.xml meta.xml; do
  path="${EXTRACT_DIR}/${meta}"
  if [ -f "${path}" ]; then
    echo
    echo "== ${meta} =="
    cat "${path}"
  else
    echo
    echo "== ${meta}: NOT FOUND =="
  fi
done

for csv in wcvp_taxon.csv wcvp_distribution.csv wcvp_replacementNames.csv; do
  path="${EXTRACT_DIR}/${csv}"
  if [ ! -f "${path}" ]; then
    echo
    echo "== ${csv}: NOT FOUND =="
    continue
  fi

  echo
  echo "== ${csv}: header row =="
  head -n 1 "${path}"

  echo
  echo "== ${csv}: delimiter detection (counts per candidate on header line) =="
  header_line="$(head -n 1 "${path}")"
  printf 'pipe(|): %s\n' "$(grep -o '|' <<<"${header_line}" | wc -l | tr -d ' ')"
  printf 'tab(\\t): %s\n' "$(grep -o $'\t' <<<"${header_line}" | wc -l | tr -d ' ')"
  printf 'comma(,): %s\n' "$(grep -o ',' <<<"${header_line}" | wc -l | tr -d ' ')"

  echo
  echo "== ${csv}: row count (incl. header) =="
  wc -l < "${path}"

  echo
  echo "== ${csv}: 5 sample data rows =="
  sed -n '2,6p' "${path}"
done

echo
echo "== Done =="

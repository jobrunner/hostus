#!/usr/bin/env bash
# Wikidata Query Service (query.wikidata.org/sparql) -> canonical xref CSV.
#
# hostus 2.0 SP4 bridge-hub harvest: our only outbound join key is `powo`
# in the `xref` table (the bare IPNI id, e.g. `396681-1`, from WCVP's
# `dynamicproperties.powoid`). Wikidata carries that same id under BOTH
# P961 (IPNI, already bare) and P5037 (POWO, stored as an
# `urn:lsid:ipni.org:names:...` LSID that must be stripped) -- and, on the
# same item, GBIF (P846 legacy / P14607 new), Catalogue of Life (P10585),
# Euro+Med (P12380), FloraVeg.EU (P12100 -- a NAME, not an id), WFO
# (P7715) and iNaturalist (P3151). So Wikidata is used as a bridge: join
# our concepts to a Wikidata item by IPNI/POWO id, then read every other
# authority off that same item. See poc/P07-findings.md for the PoC that
# verified these property numbers against the live entity data, and this
# script's crawl.py for the live-measured paging strategy (a single query
# combining the P31=Q16521 type filter with several OPTIONALs reliably
# exceeds WDQS's 60s timeout above a few hundred rows; a two-phase
# seed-scan + VALUES-batch-enrichment design does not).
#
# PYTHON DOES THE WORK (crawl.py + convert.py), same as every other
# pipeline in this repo -- no SPARQL/HTTP client in Go; hostus itself only
# ever reads the canonical CSV via a future internal/adapters/xref reader.
#
# Wikidata is CC0 -> redistribution: allowed (no attribution obligation,
# unlike the CC-BY-4.0 trait pipelines).
#
# Resumable: crawl.py checkpoints after every page (phase 1) and batch
# (phase 2) under .cache/, so if this script is interrupted (e.g. the
# calling shell's own timeout) it can simply be re-run to continue from
# where it left off -- it does not restart the harvest from scratch.
#
# Enrichment is restricted to the JOINABLE subset when
# .cache/powo_ext_ids.txt is present (one bare IPNI id per line, dumped
# from the real concept DB's `xref` table where authority='powo'): a
# Wikidata item whose IPNI/POWO id isn't one of ours can never be joined
# by hostus's ingest, so enriching it burns crawl time for no eventual
# benefit. See pipelines/README.md. If that file is absent, the full seed
# union is enriched instead (the general-purpose behaviour).
#
# USAGE:
#   bash build.sh              # run to completion or until the harness
#                               # kills the shell (progress is
#                               # checkpointed regardless -- just re-run)
#   bash build.sh <seconds>    # bounded foreground chunk: run for at most
#                               # <seconds> of enrichment, then stop
#                               # cleanly and report; re-run to continue
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TIME_BUDGET="${1:-}"

CACHE_DIR="${SCRIPT_DIR}/.cache"
OUT_DIR="${SCRIPT_DIR}/output"
mkdir -p "${CACHE_DIR}" "${OUT_DIR}"

OUT_PATH="${OUT_DIR}/wikidata-xref-canonical.csv"
SUMMARY_PATH="${SCRIPT_DIR}/wikidata.summary.txt"
JOINABLE_IDS_PATH="${CACHE_DIR}/powo_ext_ids.txt"

echo "Wikidata: two-phase harvest against https://query.wikidata.org/sparql"
echo "Wikidata: phase 1 = seed scan (P961/P5037, single-predicate, paged)"
echo "Wikidata: phase 2 = VALUES-batch enrichment (7 more xref properties)"
if [[ -f "${JOINABLE_IDS_PATH}" ]]; then
  echo "Wikidata: enrichment restricted to $(wc -l < "${JOINABLE_IDS_PATH}" | tr -d ' ') joinable ids from ${JOINABLE_IDS_PATH}"
fi
echo "Wikidata: licence CC0 -> redistribution: allowed"

START_TS=$(date +%s)
CRAWL_LOG="$(mktemp)"
if [[ -n "${TIME_BUDGET}" ]]; then
  python3 "${SCRIPT_DIR}/crawl.py" "${CACHE_DIR}" "${TIME_BUDGET}" | tee "${CRAWL_LOG}"
else
  python3 "${SCRIPT_DIR}/crawl.py" "${CACHE_DIR}" | tee "${CRAWL_LOG}"
fi
LAST_LINE="$(tail -n1 "${CRAWL_LOG}")"
END_TS=$(date +%s)
ELAPSED=$((END_TS - START_TS))

if [[ "${LAST_LINE}" != "DONE" ]]; then
  echo
  echo "Wikidata: crawl NOT yet complete after ${ELAPSED}s (${LAST_LINE})."
  echo "Wikidata: re-run 'bash pipelines/wikidata/build.sh' to continue -- progress is checkpointed under ${CACHE_DIR}."
  rm -f "${CRAWL_LOG}"
  exit 1
fi
rm -f "${CRAWL_LOG}"

echo "Wikidata: harvest complete in ${ELAPSED}s, converting to canonical CSV"

{
  echo "source=https://query.wikidata.org/sparql (two-phase: seed scan + VALUES-batch enrichment)"
  echo "license=CC0 redistribution=allowed"
  if [[ -f "${JOINABLE_IDS_PATH}" ]]; then
    python3 "${SCRIPT_DIR}/convert.py" "${CACHE_DIR}" "${OUT_PATH}" "${JOINABLE_IDS_PATH}"
  else
    python3 "${SCRIPT_DIR}/convert.py" "${CACHE_DIR}" "${OUT_PATH}"
  fi
  echo "wall_clock_seconds=${ELAPSED}"
} | tee "${SUMMARY_PATH}"

echo "Wikidata: canonical CSV written to ${OUT_PATH}"
echo "Wikidata: summary written to ${SUMMARY_PATH}"

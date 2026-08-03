#!/usr/bin/env bash
# CDM `rl_standardliste` (api.cybertaxonomy.org) -> canonical concept +
# relation CSVs.  hostus 2.0 SP5 (`POST /v1/translate`, UC6), Task 2.
#
# WHAT THIS HARVESTS
#   51466 taxonomic concepts across 18 `sec.` reference spaces, plus the
#   concept-relation graph between them (Congruent to, Includes, Overlaps,
#   the disjunctive uncertainty type "Included in or Includes or Overlaps",
#   pro parte and misapplied). See docs/research/cdm-sample.md for the
#   measurement that justified the full crawl and poc/P08-findings.md for
#   the original probe.
#
# THREE PHASES (crawl.py)
#   A  52 requests     /portal/taxon?pageSize=1000&pageIndex=N
#                      -- concepts WITH name, raw rank, secSource, taxon
#                         nodes AND every OUTGOING relation inline. This
#                         single listing replaces the 51466 per-concept
#                         relationship requests Task 1 costed, and supplies
#                         relation direction for free.
#   C  ~n_internal     classification tree walk, for `parent_uuid` only.
#                      One request per node that has children.
#   B  51466 requests  /taxon/{uuid}/relationsToThisTaxon
#                      -- the partner (`to`) end of every edge. The long pole.
#
# COST: ~51500 + n_internal requests. At the 1.139 s/request Task 1 measured
# (the limiter's 1 s floor is timed from request START, so the real cost is
# max(1 s, latency)) that is roughly 16-20 h -- inside Task 1's 22-30 h
# envelope for the endorsed scope, not beyond it.
#
# CRAWL ETIQUETTE -- binding, an explicit owner decision (see common.py):
#   * exactly one honest User-Agent:
#     hostus/2.0 (+https://github.com/jobrunner/hostus;
#     jo.brunner@mayflower.de) taxonomic-concept-research
#   * NEVER a browser User-Agent. 401/403 on the honest UA is a HARD STOP
#     (exit 2) and must be reported, never worked around.
#   * <= 1 request/second, single threaded, exponential backoff on 429/5xx.
#   * everything cached under .cache/ (gitignored): a re-run costs the
#     server nothing.
#
# RESUMABLE: a 16-20 h run WILL be interrupted. Every phase checkpoints to
# disk after each unit of work and resumes from what is already there, so
# re-running this script simply continues. An interrupt cannot corrupt
# partial state: pages are written to a temp file and renamed, and the
# append-only NDJSON logs tolerate (and re-fetch) a truncated final line.
#
# LICENCE: no licence statement was found anywhere -- not on the portal, not
# on the API, not in the payloads (probed in PoC P8 and again here). The data
# is derived from copyrighted flora literature. Therefore
# `redistribution: unknown`; LOCAL EVALUATION ONLY, and shipping the derived
# relation graph through /v1/translate stays blocked until BGBM/EDIT clarify
# in writing. See pipelines/README.md.
#
# EXIT CODES
#   0  done
#   1  crawl not yet complete -- re-run, nothing is re-fetched
#   2  the honest User-Agent was refused -- STOP and report
#   3  FALSIFIER tripped: a relationship uuid acquired a third holder
#   4  conversion failed for any other reason (crash, assert_crosswalk, ...)
#
# USAGE
#   bash pipelines/cdm/build.sh                # full crawl, run to completion
#                                              # (re-run to continue after an
#                                              #  interrupt -- nothing is lost)
#   bash pipelines/cdm/build.sh 3600           # bounded chunk: crawl at most
#                                              # 3600 s, then stop cleanly
#   CDM_CRAWL_ARGS="--max-concepts 400 ..." \
#     bash pipelines/cdm/build.sh              # bounded validation slice
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TIME_BUDGET="${1:-}"

CACHE_DIR="${SCRIPT_DIR}/.cache"
OUT_DIR="${SCRIPT_DIR}/output"
mkdir -p "${CACHE_DIR}" "${OUT_DIR}"

SUMMARY_PATH="${SCRIPT_DIR}/cdm.summary.txt"

echo "CDM: harvest against https://api.cybertaxonomy.org/rl_standardliste"
echo "CDM: phase A = flat /portal/taxon listing (concepts + outgoing relations)"
echo "CDM: phase C = classification tree walk (parent_uuid)"
echo "CDM: phase B = /taxon/{uuid}/relationsToThisTaxon (partner ends)"
echo "CDM: etiquette = 1 honest UA, <=1 req/s, single threaded, backoff, cached"
echo "CDM: licence NONE FOUND -> redistribution: unknown, local evaluation only"

START_TS=$(date +%s)
CRAWL_LOG="$(mktemp)"
CRAWL_ARGS=()
if [[ -n "${TIME_BUDGET}" ]]; then
  CRAWL_ARGS+=(--time-budget "${TIME_BUDGET}")
fi
# Deliberately unquoted: CDM_CRAWL_ARGS is a developer-supplied flag list for
# bounded validation slices, not user input.
# shellcheck disable=SC2086
if python3 "${SCRIPT_DIR}/crawl.py" "${CACHE_DIR}" "${CRAWL_ARGS[@]+"${CRAWL_ARGS[@]}"}" ${CDM_CRAWL_ARGS:-} | tee "${CRAWL_LOG}"; then
  CRAWL_RC=0
else
  CRAWL_RC=$?
fi
END_TS=$(date +%s)
ELAPSED=$((END_TS - START_TS))

if [[ ${CRAWL_RC} -eq 2 ]]; then
  echo
  echo "CDM: the honest User-Agent was REFUSED. STOPPING. Do not retry with a"
  echo "CDM: browser User-Agent -- report this to the data owner instead."
  rm -f "${CRAWL_LOG}"
  exit 2
fi

if [[ "$(tail -n1 "${CRAWL_LOG}")" != "DONE" ]]; then
  echo
  echo "CDM: crawl NOT yet complete after ${ELAPSED}s."
  echo "CDM: re-run 'bash pipelines/cdm/build.sh' to continue -- progress is"
  echo "CDM: checkpointed under ${CACHE_DIR} and nothing will be re-fetched."
  rm -f "${CRAWL_LOG}"
  exit 1
fi
rm -f "${CRAWL_LOG}"

echo "CDM: harvest complete in ${ELAPSED}s, converting to canonical CSVs"

CONVERT_LOG="$(mktemp)"
# 2>&1 INSIDE the group: the falsifier writes its per-uuid detail lines to
# stderr, and without this they bypass the tee and never reach a file --
# exactly the evidence you would want kept when the model breaks.
{
  echo "source=https://api.cybertaxonomy.org/rl_standardliste"
  echo "crawl_etiquette=1 honest UA, <=1 req/s, single threaded, backoff on 429/5xx, disk cache"
  if python3 "${SCRIPT_DIR}/convert.py" "${CACHE_DIR}" "${OUT_DIR}" 2>&1; then
    echo "wall_clock_seconds=${ELAPSED}"
  else
    echo "CONVERT_FAILED rc=$?"
  fi
} | tee "${CONVERT_LOG}"

CONVERT_RC="$(sed -n 's/^CONVERT_FAILED rc=//p' "${CONVERT_LOG}")"
if [[ -n "${CONVERT_RC}" ]]; then
  echo
  if [[ "${CONVERT_RC}" -eq 3 ]]; then
    # Exit 3 means one specific thing and must not be diluted: a relationship
    # uuid acquired a third holder, so it is not a binary edge identity and
    # the resolution model of docs/research/cdm-sample.md must be rethought.
    echo "CDM: FALSIFIER TRIPPED -- a relationship uuid acquired a third"
    echo "CDM: holder (see the FALSIFIED lines above for the uuids). The"
    echo "CDM: resolution model of docs/research/cdm-sample.md must be"
    echo "CDM: rethought. No CSV was written."
    rm -f "${CONVERT_LOG}"
    exit 3
  fi
  # Anything else -- a crash, a tripped assert_crosswalk(), a missing cache
  # file -- is a conversion error, NOT a falsification. Different code so the
  # two can never be confused by a caller or by CI.
  echo "CDM: conversion FAILED with rc=${CONVERT_RC} (this is NOT the"
  echo "CDM: falsifier; that is exit 3). A crosswalk assertion error means"
  echo "CDM: the sec. -> classification table no longer matches the 18 real"
  echo "CDM: classification uuids. See the traceback above."
  rm -f "${CONVERT_LOG}"
  exit 4
fi
mv "${CONVERT_LOG}" "${SUMMARY_PATH}"

echo "CDM: canonical CSVs written to ${OUT_DIR}"
echo "CDM: summary written to ${SUMMARY_PATH}"

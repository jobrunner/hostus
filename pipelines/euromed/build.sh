#!/usr/bin/env bash
# Euro+Med PlantBase -> canonical name-list CSV, via a CDM-REST probe.
#
# R1 found no bulk export for Euro+Med. This pipeline re-probes the CDM
# REST API at https://api.cybertaxonomy.org/euromed/ (the "euromed"
# datasource, confirmed reachable and confirmed to be the same underlying
# checklist EuroSL's SQLite mirrors -- see pipelines/eurosl, whose
# AccordingTo column literally reads "api.cybertaxonomy.org/euromed" on
# every row).
#
# PROBE RESULT: same pattern PoC P8 found for the Wisskirchen CDM instance.
#   - GET /euromed/classification -> works: exactly one classification,
#     "Euro+Med 2018" (uuid 314a68f9-8449-495a-91c2-92fde8bcf344).
#   - GET /euromed/checklist/export?classification=<uuid>&... -> reports a
#     correct `count` (64815 for this classification) but every page
#     returns an EMPTY `records` array. Broken, exactly like P08's finding
#     for rl_standardliste.
#   - GET /euromed/checklist/exportCSV?classification=<uuid> -> 302
#     redirect to itself, 0 bytes on the redirected URL. Also broken.
#   - GET /euromed/taxon?pageSize=N&pageIndex=P -> WORKS. Verified with
#     distinct page indices returning distinct, correctly-offset records
#     (note: the query param is `pageIndex`, NOT `pageNumber` -- the latter
#     is silently ignored and always serves page 0, which looks like
#     "pagination is broken" until you find the right param name).
#     167912 taxon concepts total at pageSize=500 -> 336 pages.
#
# VERDICT: obtainable, via the flat /taxon listing, NOT via the documented
# bulk-export endpoints. What's lost relative to GermanSL/EuroSL: no rank
# field on this listing, and no accepted-name resolution for synonyms
# (both would need one extra HTTP call per record at this dataset's scale
# -- infeasible in the time budget; see crawl.py's docstring). Both are
# left empty rather than guessed.
#
# No findable license -> redistribution: unknown/restricted. LOCAL
# EVALUATION USE ONLY, never redistributed in an exported bundle
# (enforced by the redistribution gate, Task 1).
#
# Politeness: sequential requests only (no concurrency), pageSize=500 (each
# page already takes ~5-6s given how verbose the JSON is, which alone
# spaces requests out without an additional sleep).
#
# Emits pipe-delimited canonical CSV: taxon|rank|status|accepted_taxon|source_id
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

CACHE_DIR="${SCRIPT_DIR}/.cache"
OUT_DIR="${SCRIPT_DIR}/output"
mkdir -p "${CACHE_DIR}" "${OUT_DIR}"

OUT_PATH="${OUT_DIR}/euromed-canonical.csv"
SUMMARY_PATH="${SCRIPT_DIR}/euromed.summary.txt"
RESUME_LOG="${CACHE_DIR}/pages_fetched.log"

echo "Euro+Med: crawling https://api.cybertaxonomy.org/euromed/taxon (paged, pageSize=500)"
echo "Euro+Med: this takes a while (~336 pages at ~5-6s each) -- see build.sh header for the probe result."

{
  echo "source=https://api.cybertaxonomy.org/euromed/taxon (paged; classification 'Euro+Med 2018')"
  python3 "${SCRIPT_DIR}/crawl.py" "${OUT_PATH}" "${RESUME_LOG}"
} | tee "${SUMMARY_PATH}"

echo "Euro+Med: canonical CSV written to ${OUT_PATH}"
echo "Euro+Med: summary written to ${SUMMARY_PATH}"

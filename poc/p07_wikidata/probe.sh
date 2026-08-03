#!/usr/bin/env bash
# PoC P7 (Phase 0, Task 0.7): verify Wikidata taxon-database properties
# (P14607/P846/P10585/P12380/P12100, + P961/P5037/WFO) resolve real xrefs
# for hostus 2.0 SP4 enrichment.
#
# WDQS requires a descriptive User-Agent (see
# https://www.mediawiki.org/wiki/Wikidata_Query_Service/User_Manual#Query_limits)
# or it will throttle/reject requests.
set -euo pipefail

UA="hostus-poc-P7/0.1 (https://github.com/jobrunner/hostus; investigation phase 0 task 0.7; contact: jo.brunner@mayflower.de)"
DATA_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)/data"
mkdir -p "$DATA_DIR"

fetch_json() {
  local label="$1" url="$2"
  echo "=== ${label} ==="
  echo "GET ${url}"
  curl -sS -H "User-Agent: ${UA}" -H "Accept: application/json" "${url}" \
    > "${DATA_DIR}/${label}.json"
  # Print a bounded preview without piping curl/jq into `head` (which SIGPIPEs
  # the upstream process under `set -o pipefail` and aborts the script).
  pretty="$(jq . "${DATA_DIR}/${label}.json")"
  printf '%s\n' "${pretty}" | cut -c1-4000
  echo
  echo
}

# --- Reference taxa -----------------------------------------------------
# Corynephorus canescens = Q159953 (verified in spec appendix)
# Jacobaea vulgaris = Q15630491 (verified via wbsearchentities during this
# probe; Q159749 does NOT exist / is not this taxon - do not reuse it)

# 1) Full entity dumps (ground truth: which statements actually exist)
fetch_json "entity_Q159953_corynephorus" \
  "https://www.wikidata.org/wiki/Special:EntityData/Q159953.json"

fetch_json "entity_Q15630491_jacobaea" \
  "https://www.wikidata.org/wiki/Special:EntityData/Q15630491.json"

# 2) SPARQL query.rq via WDQS: candidate xref properties for both taxa,
#    with English labels for both the property and (implicitly) its value.
QUERY="$(cat "$(dirname "${BASH_SOURCE[0]}")/query.rq")"
ENCODED_QUERY="$(python3 -c 'import sys,urllib.parse; print(urllib.parse.quote(sys.stdin.read()))' <<< "${QUERY}")"

fetch_json "sparql_xref_props" \
  "https://query.wikidata.org/sparql?format=json&query=${ENCODED_QUERY}"

# 3) Property metadata: confirm each Pxxxx's CURRENT label/description,
#    independent of the entity data, to catch deprecated/renamed IDs.
for PID in P14607 P846 P10585 P12380 P12100 P961 P5037; do
  fetch_json "propmeta_${PID}" \
    "https://www.wikidata.org/wiki/Special:EntityData/${PID}.json"
done

# 4) Search for a "World Flora Online ID" property by label, since we don't
#    have a confirmed Pxxxx for it going in (spec just says "the WFO id
#    property" without a number).
fetch_json "search_wfo_property" \
  "https://www.wikidata.org/w/api.php?action=wbsearchentities&search=World%20Flora%20Online%20ID&language=en&type=property&format=json"

echo "All probes complete. Raw JSON saved under ${DATA_DIR}"

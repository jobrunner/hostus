#!/usr/bin/env bash
# PoC P8b (SP5, Task 1): does P8's two-hop concept-relation method survive at
# scale? P8 validated it on ONE genus (Abies, 3 siblings). This probe measures
# it on a reproducible, stratified sample of 300-500 concepts out of the
# 51.466 taxon concepts in the CDM datasource `rl_standardliste`.
#
# Crawl etiquette (owner decision, BINDING):
#   * exactly one honest User-Agent, see $UA below -- verified to return 200 on
#     /classification, /taxon and /portal/taxon/{uuid}/taxonRelationships.
#     P8's 403 came from the Drupal *portal's* WAF, not from this API.
#   * NEVER substitute a browser User-Agent to get past a block. If the honest
#     UA is refused, the crawl STOPS and the refusal gets reported.
#   * <= 1 request/second, single threaded, backoff on 429/5xx.
#   * everything is cached under .cache/ (gitignored) so a re-run is free for
#     the server. Bulk data is never committed; sample.tsv (the drawn sample)
#     is small and IS committed so the run is exactly reproducible.
#
# Usage:
#   nix develop -c bash poc/p08b_cdm_sample/probe.sh all
#   nix develop -c bash poc/p08b_cdm_sample/probe.sh analyze
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PY="${PYTHON:-python3}"
API="https://api.cybertaxonomy.org/rl_standardliste"
UA="hostus/2.0 (+https://github.com/jobrunner/hostus; jo.brunner@mayflower.de) taxonomic-concept-research"

preflight() {
  echo "== preflight: does the honest User-Agent get served? ==" >&2
  local code
  for p in "/classification" "/taxon?pageSize=1&pageIndex=0" \
           "/portal/taxon/872088a4-95f4-472c-ae79-a29028bb3fbf/taxonRelationships"; do
    code=$(curl -sS -o /dev/null -w '%{http_code}' \
      -H "User-Agent: $UA" -H 'Accept: application/json' "$API$p")
    echo "  HTTP $code  $p" >&2
    if [ "$code" = "403" ] || [ "$code" = "401" ]; then
      echo "STOP: the honest User-Agent was refused ($code). Do NOT retry with" >&2
      echo "a browser UA. Report the refusal." >&2
      exit 2
    fi
    sleep 1
  done
}

step() { echo "== $* ==" >&2; "$PY" "$HERE/cdm_sample.py" "$@"; }

cmd="${1:-all}"
case "$cmd" in
  all)
    preflight
    step index    # ~52 requests: the flat /taxon list, pageSize=1000
    step draw     # 0 requests: deterministic, seed 20260802
    step crawl    # 1 request per sampled concept
    step direct   # 1 request per sampled concept that has >=1 relationship
    step probe    # bounded follow-up crawl for the dangling cases
    step deepdive # exhaustive crawl of 3 small genera, for the hand-analysis
    step crosscheck # genus-transfer crosscheck for the remaining dangles
    step analyze
    step latency
    ;;
  index|draw|crawl|direct|probe|deepdive|crosscheck|analyze|latency)
    step "$cmd"
    ;;
  preflight)
    preflight
    ;;
  *)
    echo "usage: probe.sh {all|preflight|index|draw|crawl|direct|probe|deepdive|crosscheck|analyze|latency}" >&2
    exit 1
    ;;
esac

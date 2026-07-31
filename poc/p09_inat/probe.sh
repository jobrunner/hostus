#!/usr/bin/env bash
# PoC P9: verify iNaturalist obscured-coordinate behavior for protected taxa.
# Confirms whether geoprivacy/taxon_geoprivacy=obscured is set for protected
# plants, and empirically measures the obscuring cell size, against the live
# iNaturalist API (no key required).
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
DATA="$ROOT/poc/data"
mkdir -p "$DATA"

UA="hostus-poc-p9/0.1 (+https://github.com/jobrunner/hostus; investigation PoC, contact: jo.brunner@mayflower.de)"
BASE="https://api.inaturalist.org/v1"

fetch() {
  local url="$1" out="$2"
  echo "GET $url" >&2
  curl -sS -H "User-Agent: $UA" -H "Accept: application/json" "$url" -o "$out"
  sleep 1
}

echo "== Resolving taxon IDs ==" >&2

fetch "$BASE/taxa?q=Cypripedium%20calceolus&rank=species" "$DATA/p09_taxa_cypripedium.json"
# NOTE: iNat's fuzzy `q=` search does NOT rank the exact name match first
# (results[0] came back as "Cypripedium parviflorum" during dev of this
# script) -- must select the exact scientific-name match explicitly.
CYPRIPEDIUM_ID=$(jq -r '[.results[] | select(.name == "Cypripedium calceolus")][0].id' "$DATA/p09_taxa_cypripedium.json")
CYPRIPEDIUM_NAME=$(jq -r '[.results[] | select(.name == "Cypripedium calceolus")][0].name' "$DATA/p09_taxa_cypripedium.json")
CYPRIPEDIUM_STATUSES=$(jq -c '[.results[] | select(.name == "Cypripedium calceolus")][0].conservation_statuses[]? | {place: .place_display_name, status: .status, iucn: .iucn}' "$DATA/p09_taxa_cypripedium.json")
echo "Cypripedium calceolus -> taxon_id=$CYPRIPEDIUM_ID name=$CYPRIPEDIUM_NAME statuses=$CYPRIPEDIUM_STATUSES" >&2

fetch "$BASE/taxa?q=Jacobaea%20vulgaris&rank=species" "$DATA/p09_taxa_jacobaea.json"
JACOBAEA_ID=$(jq -r '[.results[] | select(.name == "Jacobaea vulgaris")][0].id' "$DATA/p09_taxa_jacobaea.json")
JACOBAEA_NAME=$(jq -r '[.results[] | select(.name == "Jacobaea vulgaris")][0].name' "$DATA/p09_taxa_jacobaea.json")
echo "Jacobaea vulgaris -> taxon_id=$JACOBAEA_ID name=$JACOBAEA_NAME" >&2

EUROPE_PLACE_ID=97391

echo "== Fetching full taxon detail (per-place conservation_statuses incl. per-status geoprivacy) ==" >&2
fetch "$BASE/taxa/${CYPRIPEDIUM_ID}" "$DATA/p09_taxon_cypripedium_detail.json"
echo "conservation_statuses count: $(jq '.results[0].conservation_statuses | length' "$DATA/p09_taxon_cypripedium_detail.json")" >&2
jq -c '.results[0].conservation_statuses[] | {place: .place.display_name, status, iucn, geoprivacy}' \
  "$DATA/p09_taxon_cypripedium_detail.json" | tee "$DATA/p09_cypripedium_conservation_statuses.txt" >&2

echo "== Fetching observations (obscured case: Cypripedium calceolus, global) ==" >&2
fetch "$BASE/observations?taxon_id=${CYPRIPEDIUM_ID}&per_page=50&geo=true&order_by=observed_on&order=desc" \
  "$DATA/p09_obs_cypripedium.json"

echo "== Fetching observations (obscured case: Cypripedium calceolus, Europe only -- matches spec's Central-European use case) ==" >&2
fetch "$BASE/observations?taxon_id=${CYPRIPEDIUM_ID}&place_id=${EUROPE_PLACE_ID}&per_page=50&geo=true&order_by=observed_on&order=desc" \
  "$DATA/p09_obs_cypripedium_europe.json"

echo "== Fetching observations (control case: Jacobaea vulgaris) ==" >&2
fetch "$BASE/observations?taxon_id=${JACOBAEA_ID}&per_page=50&geo=true&order_by=observed_on&order=desc" \
  "$DATA/p09_obs_jacobaea.json"

echo "== quality_grade=research counts, with/without geoprivacy=open (Cypripedium) ==" >&2
fetch "$BASE/observations?taxon_id=${CYPRIPEDIUM_ID}&quality_grade=research&per_page=0" \
  "$DATA/p09_count_cypripedium_research_all.json"
fetch "$BASE/observations?taxon_id=${CYPRIPEDIUM_ID}&quality_grade=research&geoprivacy=open&per_page=0" \
  "$DATA/p09_count_cypripedium_research_open.json"
fetch "$BASE/observations?taxon_id=${CYPRIPEDIUM_ID}&geoprivacy=obscured&per_page=0" \
  "$DATA/p09_count_cypripedium_obscured.json"
fetch "$BASE/observations?taxon_id=${CYPRIPEDIUM_ID}&per_page=0" \
  "$DATA/p09_count_cypripedium_all.json"

echo "== Same counts for control (Jacobaea vulgaris) ==" >&2
fetch "$BASE/observations?taxon_id=${JACOBAEA_ID}&quality_grade=research&per_page=0" \
  "$DATA/p09_count_jacobaea_research_all.json"
fetch "$BASE/observations?taxon_id=${JACOBAEA_ID}&geoprivacy=obscured&per_page=0" \
  "$DATA/p09_count_jacobaea_obscured.json"
fetch "$BASE/observations?taxon_id=${JACOBAEA_ID}&per_page=0" \
  "$DATA/p09_count_jacobaea_all.json"

echo "" >&2
echo "======================================================" >&2
echo "SUMMARY" >&2
echo "======================================================" >&2

echo "--- Cypripedium calceolus (taxon_id=$CYPRIPEDIUM_ID) totals ---" >&2
echo "total observations (geo unfiltered): $(jq -r '.total_results' "$DATA/p09_count_cypripedium_all.json")" >&2
echo "geoprivacy=obscured count:           $(jq -r '.total_results' "$DATA/p09_count_cypripedium_obscured.json")" >&2
echo "quality_grade=research (all):        $(jq -r '.total_results' "$DATA/p09_count_cypripedium_research_all.json")" >&2
echo "quality_grade=research + open geo:   $(jq -r '.total_results' "$DATA/p09_count_cypripedium_research_open.json")" >&2

echo "--- Jacobaea vulgaris (taxon_id=$JACOBAEA_ID) totals ---" >&2
echo "total observations (geo unfiltered): $(jq -r '.total_results' "$DATA/p09_count_jacobaea_all.json")" >&2
echo "geoprivacy=obscured count:           $(jq -r '.total_results' "$DATA/p09_count_jacobaea_obscured.json")" >&2
echo "quality_grade=research (all):        $(jq -r '.total_results' "$DATA/p09_count_jacobaea_research_all.json")" >&2

echo "" >&2
echo "--- Cypripedium sample (50 most recent, geo=true): geoprivacy/taxon_geoprivacy field distribution ---" >&2
jq -r '[.results[] | (.geoprivacy // "null") + " / " + (.taxon_geoprivacy // "null")] | group_by(.) | map({key: .[0], count: length})' \
  "$DATA/p09_obs_cypripedium.json" | tee "$DATA/p09_cypripedium_geoprivacy_dist.json" >&2

echo "" >&2
echo "--- Jacobaea sample (50 most recent, geo=true): geoprivacy/taxon_geoprivacy field distribution ---" >&2
jq -r '[.results[] | (.geoprivacy // "null") + " / " + (.taxon_geoprivacy // "null")] | group_by(.) | map({key: .[0], count: length})' \
  "$DATA/p09_obs_jacobaea.json" | tee "$DATA/p09_jacobaea_geoprivacy_dist.json" >&2

echo "" >&2
echo "--- Cypripedium: for records where (taxon_)geoprivacy=obscured, dump location/geojson/accuracy fields ---" >&2
jq '[.results[] | select(.geoprivacy == "obscured" or .taxon_geoprivacy == "obscured") |
  {id, geoprivacy, taxon_geoprivacy, location, geojson, positional_accuracy, public_positional_accuracy, obscured, mappable}]' \
  "$DATA/p09_obs_cypripedium.json" | tee "$DATA/p09_cypripedium_obscured_records.json" >&2

echo "" >&2
echo "--- Cypripedium: distinct rounded 'location' values among obscured records (cell-size probe) ---" >&2
jq -r '[.results[] | select(.geoprivacy == "obscured" or .taxon_geoprivacy == "obscured") | .location] | unique | .[]' \
  "$DATA/p09_obs_cypripedium.json" | tee "$DATA/p09_cypripedium_obscured_locations.txt" >&2

echo "" >&2
echo "--- Cypripedium: sample of NON-obscured (open) records for comparison ---" >&2
jq '[.results[] | select(.geoprivacy != "obscured" and .taxon_geoprivacy != "obscured") |
  {id, geoprivacy, taxon_geoprivacy, location, positional_accuracy, public_positional_accuracy}] | .[0:5]' \
  "$DATA/p09_obs_cypripedium.json" | tee "$DATA/p09_cypripedium_open_sample.json" >&2

echo "" >&2
echo "--- Cypripedium calceolus, EUROPE ONLY: geoprivacy/taxon_geoprivacy field distribution ---" >&2
jq -r '[.results[] | (.geoprivacy // "null") + " / " + (.taxon_geoprivacy // "null")] | group_by(.) | map({key: .[0], count: length})' \
  "$DATA/p09_obs_cypripedium_europe.json" | tee "$DATA/p09_cypripedium_europe_geoprivacy_dist.json" >&2

echo "--- Cypripedium calceolus, EUROPE ONLY: obscured records dump ---" >&2
jq '[.results[] | select(.geoprivacy == "obscured" or .taxon_geoprivacy == "obscured") |
  {id, geoprivacy, taxon_geoprivacy, location, positional_accuracy, public_positional_accuracy, place_guess}]' \
  "$DATA/p09_obs_cypripedium_europe.json" | tee "$DATA/p09_cypripedium_europe_obscured_records.json" >&2

echo "" >&2
echo "Done. Raw JSON under poc/data/p09_*.json" >&2

#!/usr/bin/env bash
# PoC P3 (Phase 0, Task 0.3): verify GBIF v2/species/match honors checklistKey
# (COL-XR), while v1/species/suggest ignores it.
#
# NOTE: the GBIF v2 match contract uses `scientificName` as the query
# parameter, NOT `name`. Using `name` returns diagnostics.note="No name
# given" with matchType NONE - discovered during this probe.
set -euo pipefail

COL_XR_KEY="7ddf754f-d193-4cc9-b351-99906754a03b"
DATA_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)/data"
mkdir -p "$DATA_DIR"

fetch() {
  local label="$1" url="$2"
  echo "=== ${label} ==="
  echo "GET ${url}"
  curl -sS "${url}" | tee "${DATA_DIR}/${label}.json" | jq .
  echo
}

# 1) v2/species/match WITH checklistKey (COL-XR) - Corynephorus canescens
fetch "v2_match_corynephorus_colxr" \
  "https://api.gbif.org/v2/species/match?scientificName=Corynephorus%20canescens&checklistKey=${COL_XR_KEY}"

# 2) v2/species/match WITHOUT checklistKey (default GBIF backbone) - same taxon
fetch "v2_match_corynephorus_backbone" \
  "https://api.gbif.org/v2/species/match?scientificName=Corynephorus%20canescens"

# 3) v1/species/suggest WITH checklistKey - should be ignored
fetch "v1_suggest_corynephorus_with_checklistkey" \
  "https://api.gbif.org/v1/species/suggest?q=Corynephorus&checklistKey=${COL_XR_KEY}"

# 4) v1/species/suggest WITHOUT checklistKey - compare to (3)
fetch "v1_suggest_corynephorus_without_checklistkey" \
  "https://api.gbif.org/v1/species/suggest?q=Corynephorus"

# 5) v2/species/match WITH checklistKey - second data point: Jacobaea vulgaris
fetch "v2_match_jacobaea_colxr" \
  "https://api.gbif.org/v2/species/match?scientificName=Jacobaea%20vulgaris&checklistKey=${COL_XR_KEY}"

# 6) v2/species/match WITHOUT checklistKey - Jacobaea vulgaris backbone comparison
fetch "v2_match_jacobaea_backbone" \
  "https://api.gbif.org/v2/species/match?scientificName=Jacobaea%20vulgaris"

echo "All probes complete. Raw JSON saved under ${DATA_DIR}"

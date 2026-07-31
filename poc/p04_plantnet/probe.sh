#!/usr/bin/env bash
# PoC P4 (Phase 0, Task 0.4): verify PlantNet's identify API returns both
# gbif.id AND powo.id per result candidate, which UC1's PlantNet path relies
# on for matching via powo.id rather than fragile name-string matching.
#
# Requires PLANTNET_API_KEY and PLANTNET_API_ENDPOINT in the environment
# (loaded from .envrc.local via direnv/nix develop). The key is NEVER
# printed - all curl output that could contain the request URL with the key
# is redacted before being echoed.
set -euo pipefail

if [ -z "${PLANTNET_API_KEY:-}" ]; then
  echo "set PLANTNET_API_KEY in .envrc.local" >&2
  exit 1
fi

if [ -z "${PLANTNET_API_ENDPOINT:-}" ]; then
  echo "set PLANTNET_API_ENDPOINT in .envrc.local" >&2
  exit 1
fi

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DATA_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)/data"
mkdir -p "${DATA_DIR}"

IMAGE="${DATA_DIR}/p04_taraxacum_officinale.jpg"
if [ ! -f "${IMAGE}" ]; then
  echo "test image ${IMAGE} not found - see poc/P04-findings.md for the source URL" >&2
  exit 1
fi

# NOTE: the task's assumed project id "k-central-europe" does not exist.
# The /v2/projects?lat=&lon= helper (probed below) for a Central-European
# coordinate (49.8, 9.9) resolves to "k-middle-europe" instead - discovered
# during this probe. Using the correct id here.
PROJECT="k-middle-europe"

redact() {
  # Strip the api-key value out of anything that might echo the request URL.
  sed -E "s/(api-key=)[^&\"[:space:]]+/\1***/g"
}

echo "=== identify: ${PROJECT} (organ=flower) ==="
IDENTIFY_URL="${PLANTNET_API_ENDPOINT}/v2/identify/${PROJECT}?api-key=${PLANTNET_API_KEY}&include-related-images=false"
echo "POST $(printf '%s' "${IDENTIFY_URL}" | redact)"
curl -sS -X POST "${IDENTIFY_URL}" \
  -F "images=@${IMAGE}" \
  -F "organs=flower" \
  | tee "${DATA_DIR}/p04_identify_taraxacum.json" \
  | jq .
echo

echo "=== projects: coordinate -> project helper (lat=49.8, lon=9.9) ==="
PROJECTS_URL="${PLANTNET_API_ENDPOINT}/v2/projects?api-key=${PLANTNET_API_KEY}&lat=49.8&lon=9.9"
echo "GET $(printf '%s' "${PROJECTS_URL}" | redact)"
curl -sS "${PROJECTS_URL}" \
  | tee "${DATA_DIR}/p04_projects_lat49.8_lon9.9.json" \
  | jq .
echo

echo "=== field check: results[].gbif.id / results[].powo.id ==="
jq '[.results[]? | {score, scientificName: .species.scientificNameWithoutAuthor, gbifId: .gbif.id, powoId: .powo.id}]' \
  "${DATA_DIR}/p04_identify_taraxacum.json"

echo "All probes complete. Raw JSON saved under ${DATA_DIR} (gitignored)."

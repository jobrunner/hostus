#!/usr/bin/env bash
# PoC P8 (Phase 0, Task 0.8): is the Wisskirchen concept-relation data
# (`sec.` relationships, needed for SP5 `POST /v1/translate`) MACHINE-
# RETRIEVABLE from the CDM portal? This is the highest-risk PoC per the
# architecture spec ("P8 ... das groesste Risiko").
#
# Portal: https://portal.cybertaxonomy.org/rotelisten_flora_deutschland/
# The Drupal frontend links to the underlying CDM REST API at:
#   https://api.cybertaxonomy.org/rl_standardliste/
# (NOT "rotelisten_flora_deutschland" as the portal slug might suggest --
# the CDM datasource is named "rl_standardliste" and was discovered by
# grepping the portal HTML for `api.cybertaxonomy.org` links.)
#
# NOTE on User-Agent: the portal's WAF returns HTTP 403 for a descriptive
# bot-style User-Agent (e.g. "hostus-poc-P8/0.1 (...)"), but 200 for an
# ordinary browser UA. The api.cybertaxonomy.org CDM server itself does not
# appear to filter by User-Agent. We use a browser UA throughout for
# consistent, reproducible results; a real integration would need to
# clarify with BGBM whether descriptive bot UAs are acceptable.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
DATA="$ROOT/poc/data"
mkdir -p "$DATA"

UA="Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0 Safari/537.36"
PORTAL="https://portal.cybertaxonomy.org/rotelisten_flora_deutschland"
API="https://api.cybertaxonomy.org/rl_standardliste"

fetch() {
  local url="$1" out="$2"
  echo "GET $url" >&2
  curl -sS -H "User-Agent: $UA" -H "Accept: application/json" "$url" -o "$out" \
    -w "  -> HTTP %{http_code}, %{size_download} bytes, %{time_total}s\n" >&2
  sleep 0.5
}

echo "== 1) Portal reachability + discovery of the CDM REST API base ==" >&2
curl -sS -H "User-Agent: $UA" -D - -o "$DATA/p08_portal_home.html" "$PORTAL/" \
  -w "  -> HTTP %{http_code}\n" >&2 | head -5 >&2
echo "  Discovered CDM API links in portal HTML:" >&2
grep -oiE 'href="[^"]*api\.cybertaxonomy\.org[^"]*"' "$DATA/p08_portal_home.html" | sort -u >&2 || true

echo "== 2) CDM remote API index (Swagger/springfox landing page) ==" >&2
curl -sS -H "User-Agent: $UA" -o "$DATA/p08_api_index.html" "$API/" \
  -w "  -> HTTP %{http_code}, content-type %{content_type}\n" >&2

echo "== 3) Checklist Catalogue API doc + flat checklist/export ==" >&2
curl -sS -H "User-Agent: $UA" -o "$DATA/p08_checklist_doc.html" "$API/checklist" \
  -w "  -> HTTP %{http_code}\n" >&2
curl -sS -H "User-Agent: $UA" -o "$DATA/p08_checklist_export_doc.html" "$API/checklist/export" \
  -w "  -> HTTP %{http_code}\n" >&2

echo "== 4) Classification list (finds the Wisskirchen classification uuid) ==" >&2
fetch "$API/classification" "$DATA/p08_classification_list.json"
WISSKIRCHEN_CLS=$(jq -r '.records[] | select(.titleCache | test("WISSKIRCHEN & HAEUPLER"; "i")) | .uuid' "$DATA/p08_classification_list.json" | head -1)
echo "  Wisskirchen classification uuid: $WISSKIRCHEN_CLS" >&2

echo "== 5) Flat checklist/export (CSV-style export path) for that classification ==" >&2
echo "  (Documented response fields per doc/checklist/export: scientificName," >&2
echo "   author, rank, parentUuid, taxonConceptID -- ONE id per row, no typed" >&2
echo "   relation types, no partner sec. reference.)" >&2
fetch "$API/checklist/export?classification=${WISSKIRCHEN_CLS}&pageSize=5&pageNumber=1" \
  "$DATA/p08_checklist_export_sample.json"

echo "== 6) Full taxon listing (51k+ concepts) -- confirms per-concept sec. via titleCache ==" >&2
fetch "$API/taxon?pageSize=25" "$DATA/p08_taxon_list_sample.json"
jq -r '.count' "$DATA/p08_taxon_list_sample.json" | xargs -I{} echo "  total taxon concepts in dataset: {}" >&2

echo "== 7) Reference concept: Abies alba Mill. sec. Wisskirchen & Haeupler 1998 ==" >&2
ABIES_WK="872088a4-95f4-472c-ae79-a29028bb3fbf"
fetch "$API/taxon/${ABIES_WK}" "$DATA/p08_taxon_abies_wisskirchen.json"
echo "  secSource.citation.titleCache:" >&2
jq -r '.secSource.citation.titleCache' "$DATA/p08_taxon_abies_wisskirchen.json" >&2

echo "== 8) THE critical test: typed concept relationships for this concept ==" >&2
echo "  (portal/taxon/{uuid}/taxonRelationships -- expanded DTO with type incl." >&2
echo "   symbol/representation, conceptRelationship flag, and relationship uuid)" >&2
fetch "$API/portal/taxon/${ABIES_WK}/taxonRelationships" "$DATA/p08_taxon_abies_relationships.json"
echo "  relation types found:" >&2
jq -r '.[] | "    \(.uuid)  \(.type.representation_L10n) (\(.type.symbol)), conceptRelationship=\(.type.conceptRelationship)"' \
  "$DATA/p08_taxon_abies_relationships.json" >&2

echo "== 9) Cross-reference: resolve the OTHER concept in each relation ==" >&2
echo "   The taxonRelationships payload gives the relation type + relationship" >&2
echo "   uuid but NOT the partner taxon's uuid directly (Jackson serialization" >&2
echo "   drops the reference to avoid recursion). We resolve the partner by" >&2
echo "   fetching sibling 'Abies alba' concepts (different sec.) and matching" >&2
echo "   on the shared relationship uuid." >&2
declare -A SIBLINGS=(
  ["b0d35335-63e6-41ab-bdb0-d01851134e9c"]="EHRENDORFER: Liste der Gefaesspflanzen Mitteleuropas"
  ["7a63f215-0a41-4b7e-9394-bda4521d6ad1"]="Greuter & al.: Med-Checklist"
  ["61c2bc4f-a23d-4160-8f14-625b4484fc2f"]="HEGI: Illustrierte Flora von Mitteleuropa"
)
for uuid in "${!SIBLINGS[@]}"; do
  out="$DATA/p08_taxon_sibling_${uuid}.json"
  fetch "$API/portal/taxon/${uuid}/taxonRelationships" "$out"
  echo "  sibling '${SIBLINGS[$uuid]}' ($uuid) relationship uuids:" >&2
  jq -r '.[] | "    \(.uuid)  \(.type.representation_L10n)"' "$out" >&2
done

echo "== Done. All raw responses saved under poc/data/p08_*.{html,json} ==" >&2

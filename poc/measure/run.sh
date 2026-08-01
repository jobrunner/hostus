#!/usr/bin/env bash
# Mess-Harness fuer den Reality-Check (Task 3). Jede Kennzahl in
# docs/research/reality-check.md stammt aus genau einem dieser Schritte.
# Aufruf einzelner Schritte: run.sh m1 | m2 | m3 | m4 | m5
#
# Voraussetzungen: ./hostus gebaut (make build), WCVP unter
# poc/data/wcvp/{extracted,filtered}, Pipeline-Artefakte unter
# pipelines/*/output/.
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT"
OUT=poc/measure/out
mkdir -p "$OUT"

download() {
  mkdir -p poc/data/wcvp
  curl -sSL -o poc/data/wcvp/wcvp_dwca.zip https://sftp.kew.org/pub/data-repositories/WCVP/wcvp_dwca.zip
  unzip -o poc/data/wcvp/wcvp_dwca.zip -d poc/data/wcvp/extracted
  bash poc/measure/filter_ranks.sh poc/data/wcvp/extracted poc/data/wcvp/filtered
}

m1() {
  rm -f "$OUT"/m1.sqlite*
  /usr/bin/time -l ./hostus ingest --dataset poc/measure/dataset.wcvp.yaml --db "$OUT/m1.sqlite" 2>&1 | tee "$OUT/m1-ingest.log"
  ls -la "$OUT/m1.sqlite"
}

m2() {
  rm -f "$OUT"/m2.sqlite*
  # Leere DB mit Serienschema anlegen (bundle oeffnet die --db-Datei und
  # legt das Schema an), dann die FK-Kindspalten-Indizes ergaenzen. Ohne
  # sie laeuft der Volldaten-Ingest quadratisch (M1.1/M1.2).
  ./hostus bundle --db "$OUT/m2.sqlite" --out "$OUT/throwaway.sqlite" >/dev/null
  sqlite3 "$OUT/m2.sqlite" < poc/measure/fk_indexes.sql
  /usr/bin/time -l ./hostus ingest --dataset poc/measure/dataset.full.yaml --db "$OUT/m2.sqlite" 2>&1 | tee "$OUT/m2-ingest.log"
  ls -la "$OUT/m2.sqlite"
  sqlite3 "$OUT/m2.sqlite" < poc/measure/stats.sql | tee "$OUT/m2-stats.txt"
}

m3() {
  (cd poc && go build -o "$ROOT/$OUT/bridge" ./measure/bridge)
  "$OUT/bridge" --db "$OUT/m2.sqlite" \
    --vocab eive=pipelines/eive/output/eive-canonical.csv \
    --vocab tichy=pipelines/tichy/output/tichy-canonical.csv \
    --vocab midolo=pipelines/midolo/output/midolo-canonical.csv \
    --list euromed=pipelines/euromed/output/euromed-canonical.csv \
    --list eurosl=pipelines/eurosl/output/eurosl-canonical.csv \
    --list germansl=pipelines/germansl/output/germansl-canonical.csv \
    --list floraveg=pipelines/floraveg/output/floraveg-canonical.csv \
    | tee "$OUT/m3-bridge.txt"
}

m4() {
  (cd poc && go build -o "$ROOT/$OUT/latency" ./measure/latency)
  HOSTUS_SQLITE_PATH="$OUT/m2.sqlite" ./hostus serve --port 8099 --log-level warn &
  local pid=$!
  trap 'kill $pid 2>/dev/null || true' EXIT
  until curl -sf "http://127.0.0.1:8099/v1/suggest?q=ace" >/dev/null; do sleep 1; done
  "$OUT/latency" --base http://127.0.0.1:8099            | tee "$OUT/m4-latency-noarea.txt"
  "$OUT/latency" --base http://127.0.0.1:8099 --area GER | tee "$OUT/m4-latency-ger.txt"
  kill $pid 2>/dev/null || true
}

m5() {
  for area in GER AUT SWI CZE POL HUN FRA ITA NET BGM DEN; do
    rm -f "$OUT/bundle-$area.sqlite"
    ./hostus bundle --db "$OUT/m2.sqlite" --area "$area" --out "$OUT/bundle-$area.sqlite" --snapshot reality-check
  done
  rm -f "$OUT/bundle-full.sqlite"
  ./hostus bundle --db "$OUT/m2.sqlite" --out "$OUT/bundle-full.sqlite" --snapshot reality-check
  ls -la "$OUT"/bundle-*.sqlite
}

for step in "$@"; do "$step"; done

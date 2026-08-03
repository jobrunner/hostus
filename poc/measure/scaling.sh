#!/usr/bin/env bash
# Misst die Ingest-Dauer als Funktion der Zeilenzahl — einmal mit dem
# Serienschema und einmal mit den zusaetzlichen FK-Kindspalten-Indizes
# (poc/measure/fk_indexes.sql). Belegt den M1-Befund (quadratische
# Ingest-Kosten durch FK-Pruef-Scans bei `INSERT OR REPLACE`) mit echten
# Messpunkten statt mit einer Vermutung.
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT"
OUT=poc/measure/out
SRC=poc/data/wcvp/filtered

for n in "$@"; do
  d="poc/data/wcvp/sub-$n"
  mkdir -p "$d"
  cp "$SRC/meta.xml" "$SRC/eml.xml" "$SRC/wcvp_distribution.csv" "$SRC/wcvp_replacementNames.csv" "$d/"
  head -n "$((n + 1))" "$SRC/wcvp_taxon.csv" > "$d/wcvp_taxon.csv"

  sed "s#__PATH__#$ROOT/$d#" poc/measure/dataset.template.yaml > "$OUT/dataset-sub-$n.yaml"

  for mode in plain indexed; do
    db="$OUT/scale-$n-$mode.sqlite"
    rm -f "$db" "$db-wal" "$db-shm"
    ./hostus bundle --db "$db" --out "$OUT/throwaway.sqlite" >/dev/null
    if [ "$mode" = indexed ]; then
      sqlite3 "$db" < poc/measure/fk_indexes.sql
    fi
    start=$(date +%s)
    ./hostus ingest --dataset "$OUT/dataset-sub-$n.yaml" --db "$db" > "$OUT/scale-$n-$mode.log" 2>&1
    end=$(date +%s)
    printf '%s\t%s\t%ss\t%s bytes\n' "$n" "$mode" "$((end - start))" "$(wc -c < "$db" | tr -d ' ')"
  done
done

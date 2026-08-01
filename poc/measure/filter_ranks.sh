#!/usr/bin/env bash
# Erzeugt eine gefilterte Kopie des WCVP-DwC-A, in der nur Taxon-Zeilen mit
# einem von domain.ParseRank unterstuetzten Rang (FAMILY/GENUS/SPECIES/
# SUBSPECIES/VARIETY/FORM) verbleiben. Notwendig, weil `hostus ingest` auf
# dem ersten unbekannten Rang hart abbricht (M1-Befund).
set -euo pipefail
SRC="${1:?usage: filter_ranks.sh <src-dir> <dst-dir>}"
DST="${2:?usage: filter_ranks.sh <src-dir> <dst-dir>}"
mkdir -p "$DST"
cp "$SRC/meta.xml" "$SRC/eml.xml" "$SRC/wcvp_distribution.csv" "$SRC/wcvp_replacementNames.csv" "$DST/"
awk -F'|' 'NR==1{print; next}
  {r=toupper($8)}
  r=="SPECIES"||r=="VARIETY"||r=="SUBSPECIES"||r=="FORM"||r=="GENUS"||r=="FAMILY"{print}' \
  "$SRC/wcvp_taxon.csv" > "$DST/wcvp_taxon.csv"
echo "src rows: $(( $(wc -l < "$SRC/wcvp_taxon.csv") - 1 ))"
echo "dst rows: $(( $(wc -l < "$DST/wcvp_taxon.csv") - 1 ))"

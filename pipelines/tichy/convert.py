#!/usr/bin/env python3
"""Indicator.values-tables-2022-11-07-Zenodo.v2.xlsx (sheet
Tab-IVs-Tichy-et-al2023) -> canonical trait CSV.

The sheet has a two-row header: row 1 carries the dimension category name
(LIGHT/TEMPERATURE/MOISTURE/REACTION/NUTRIENTS/SALINITY) over the "Average"
columns in row 2, name column is "Taxon" (row 2, column index 1). Values are
either numeric, the literal "x" (indifferent, no numeric indicator), or
"NA" (not assessed) -- both are treated as "not provided" and the row is
skipped entirely (per the canonical CSV contract, one row per (taxon, dim)
that actually HAS a value; a vocabulary simply not providing a value for a
dim is the reader's job to represent as an absent row, not an empty value
field, since dim is mandatory).
"""
import csv
import sys

import openpyxl

CATEGORY_TO_DIM = {
    "LIGHT": "L",
    "TEMPERATURE": "T",
    "MOISTURE": "M",
    "REACTION": "R",
    "NUTRIENTS": "N",
    "SALINITY": "S",
}


def parse_value(raw):
    if raw is None:
        return None
    if isinstance(raw, (int, float)):
        return float(raw)
    s = str(raw).strip()
    if s == "" or s.lower() in ("x", "na"):
        return None
    try:
        return float(s)
    except ValueError:
        return None


def main():
    in_path, out_path, vocab, version = sys.argv[1:5]

    wb = openpyxl.load_workbook(in_path, read_only=True, data_only=True)
    ws = wb["Tab-IVs-Tichy-et-al2023"]
    rows = ws.iter_rows(values_only=True)
    category_row = next(rows)
    name_row = next(rows)

    taxon_col = None
    for i, name in enumerate(name_row):
        if name == "Taxon":
            taxon_col = i
            break
    if taxon_col is None:
        raise SystemExit("Tichy: could not find 'Taxon' column in header row 2")

    dim_cols = [(i, CATEGORY_TO_DIM[cat]) for i, cat in enumerate(category_row) if cat in CATEGORY_TO_DIM]
    dims = [d for _, d in dim_cols]

    stats = {d: [None, None] for d in dims}
    taxa = set()
    row_count = 0

    with open(out_path, "w", newline="", encoding="utf-8") as f:
        w = csv.writer(f, delimiter="|")
        w.writerow(["taxon", "vocab", "vocab_version", "dim", "value", "niche_width", "n_systems"])
        for r in rows:
            taxon = r[taxon_col]
            if not taxon:
                continue
            for col, dim in dim_cols:
                val = parse_value(r[col])
                if val is None:
                    continue
                w.writerow([taxon, vocab, version, dim, str(val), "", ""])
                row_count += 1
                lo, hi = stats[dim]
                stats[dim][0] = val if lo is None else min(lo, val)
                stats[dim][1] = val if hi is None else max(hi, val)
            taxa.add(taxon)

    print(f"vocab={vocab} version={version}")
    print(f"rows={row_count} taxa={len(taxa)} dims={','.join(dims)}")
    for d in dims:
        lo, hi = stats[d]
        print(f"  dim {d}: min={lo} max={hi}")


if __name__ == "__main__":
    main()

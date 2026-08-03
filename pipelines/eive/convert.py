#!/usr/bin/env python3
"""EIVE_Paper_1.0_SM_08.xlsx (sheet mainTable) -> canonical trait CSV.

Columns per PoC P6: name col TaxonConcept, dims EIVEres-{M,N,R,L,T}, niche
width EIVEres-{dim}.nw3, source-system count EIVEres-{dim}.n. Both niche
width and n are always present for EIVE (unlike Tichy/Midolo), so they are
never empty in the emitted rows.
"""
import csv
import sys

import openpyxl

DIMS = ["M", "N", "R", "L", "T"]


def fnum(x):
    if x is None or x == "":
        return None
    return float(x)


def main():
    in_path, out_path, vocab, version = sys.argv[1:5]

    wb = openpyxl.load_workbook(in_path, read_only=True, data_only=True)
    ws = wb["mainTable"]
    rows = ws.iter_rows(values_only=True)
    header = next(rows)
    idx = {name: i for i, name in enumerate(header)}

    stats = {d: [None, None] for d in DIMS}  # [min, max]
    taxa = set()
    row_count = 0

    with open(out_path, "w", newline="", encoding="utf-8") as f:
        w = csv.writer(f, delimiter="|")
        w.writerow(["taxon", "vocab", "vocab_version", "dim", "value", "niche_width", "n_systems"])
        for r in rows:
            taxon = r[idx["TaxonConcept"]]
            if not taxon:
                continue
            for d in DIMS:
                val = fnum(r[idx[f"EIVEres-{d}"]])
                if val is None:
                    continue
                nw = fnum(r[idx[f"EIVEres-{d}.nw3"]])
                n = r[idx[f"EIVEres-{d}.n"]]
                n_out = "" if n is None or n == "" else str(int(n))
                w.writerow([taxon, vocab, version, d, str(val), "" if nw is None else str(nw), n_out])
                row_count += 1
                lo, hi = stats[d]
                stats[d][0] = val if lo is None else min(lo, val)
                stats[d][1] = val if hi is None else max(hi, val)
            taxa.add(taxon)

    print(f"vocab={vocab} version={version}")
    print(f"rows={row_count} taxa={len(taxa)} dims={','.join(DIMS)}")
    for d in DIMS:
        lo, hi = stats[d]
        print(f"  dim {d}: min={lo} max={hi}")


if __name__ == "__main__":
    main()

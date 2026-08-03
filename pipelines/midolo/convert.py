#!/usr/bin/env python3
"""disturbance_indicator_values.csv -> canonical trait CSV.

Name col "species". Maps the whole-community disturbance dims to the
domain.TraitDim spellings; deliberately excludes the herblayer variants
(*.herblayer, not part of the domain dim set) and the SD_* columns (a
standard deviation, not a niche-width -- see domain.TraitValue doc comment).
"""
import csv
import sys

COLUMN_TO_DIM = {
    "Disturbance.Severity": "disturbance_severity",
    "Disturbance.Frequency": "disturbance_frequency",
    "Mowing.Frequency": "mowing_frequency",
    "Grazing.Pressure": "grazing_pressure",
    "Soil.Disturbance": "soil_disturbance",
}


def main():
    in_path, out_path, vocab, version = sys.argv[1:5]

    with open(in_path, newline="", encoding="utf-8-sig") as inf:
        reader = csv.DictReader(inf)
        dim_cols = [(col, dim) for col, dim in COLUMN_TO_DIM.items() if col in reader.fieldnames]
        dims = [d for _, d in dim_cols]

        stats = {d: [None, None] for d in dims}
        taxa = set()
        row_count = 0

        with open(out_path, "w", newline="", encoding="utf-8") as outf:
            w = csv.writer(outf, delimiter="|")
            w.writerow(["taxon", "vocab", "vocab_version", "dim", "value", "niche_width", "n_systems"])
            for rec in reader:
                taxon = rec.get("species")
                if not taxon:
                    continue
                for col, dim in dim_cols:
                    raw = rec.get(col)
                    if raw is None or raw == "":
                        continue
                    try:
                        val = float(raw)
                    except ValueError:
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

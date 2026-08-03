#!/usr/bin/env python3
"""EuroSL.sqlite (table "EuroPlusMed.Plantae") -> canonical name-list CSV.

Schema (verified against the real download, PRAGMA table_info):
TaxonUsageID, TaxonName, NameAuthor, status, TaxonConceptID, TaxonConcept,
TaxonRank, IsChildTaxonOfID, IsChildTaxonOf, fullname, nb.children, doubt,
secName, ambiguous, OriginalName, AccordingTo.

`status` is already a plain string ("Accepted", "Synonym", ...) — no boolean
flag to translate like GermanSL. `AccordingTo` on every observed row is
"api.cybertaxonomy.org/euromed": EuroSL is itself built from the Euro+Med
CDM dataset (see pipelines/euromed/ for the independent CDM-REST probe of
the same underlying data).

Canonical mapping:
  taxon          = TaxonName
  rank           = TaxonRank
  status         = status, lowercased
  accepted_taxon = TaxonConcept, only emitted when status != accepted
  source_id      = TaxonUsageID
"""
import csv
import sqlite3
import sys


def main():
    in_path, out_path = sys.argv[1:3]

    con = sqlite3.connect(in_path)
    cur = con.cursor()
    cur.execute(
        'SELECT TaxonUsageID, TaxonName, TaxonRank, status, TaxonConcept '
        'FROM "EuroPlusMed.Plantae"'
    )

    row_count = 0
    taxa = set()
    ranks = {}
    statuses = {}

    with open(out_path, "w", newline="", encoding="utf-8") as f:
        w = csv.writer(f, delimiter="|")
        w.writerow(["taxon", "rank", "status", "accepted_taxon", "source_id"])
        for source_id, taxon, rank, status, accepted_concept in cur:
            if not taxon:
                continue
            rank = rank or ""
            status_norm = (status or "").strip().lower()
            is_accepted = status_norm == "accepted"
            accepted_taxon = "" if is_accepted else (accepted_concept or "")
            w.writerow([taxon, rank, status_norm, accepted_taxon, source_id or ""])
            row_count += 1
            taxa.add(taxon)
            ranks[rank] = ranks.get(rank, 0) + 1
            statuses[status_norm] = statuses.get(status_norm, 0) + 1

    print(f"rows={row_count} taxa={len(taxa)}")
    print("statuses=" + ",".join(f"{k}:{v}" for k, v in sorted(statuses.items(), key=lambda kv: -kv[1])))
    print("ranks=" + ",".join(f"{k}:{v}" for k, v in sorted(ranks.items(), key=lambda kv: -kv[1])))


if __name__ == "__main__":
    main()

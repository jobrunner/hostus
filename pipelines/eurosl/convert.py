#!/usr/bin/env python3
"""EuroSL.sqlite (table "EuroPlusMed.Plantae") -> canonical name-list CSV.

Schema (verified against the real download, PRAGMA table_info):
TaxonUsageID, TaxonName, NameAuthor, status, TaxonConceptID, TaxonConcept,
TaxonRank, IsChildTaxonOfID, IsChildTaxonOf, fullname, nb.children, doubt,
secName, ambiguous, OriginalName, AccordingTo.

`status` is already a plain string ("Accepted", "Synonym", ...) — no boolean
flag to translate like GermanSL. `AccordingTo` on every observed row is
"api.cybertaxonomy.org/euromed", and the single data table is literally
`EuroPlusMed.Plantae`: EuroSL is the Euro+Med CDM dataset, structured. hostus
therefore uses THIS pipeline as its Euro+Med source. The former standalone
`euromed` pipeline crawled the flat /euromed/taxon REST listing of the same
dataset — author-laden names, no rank, no accepted link (reality-check M6:
0 rows with rank / 0 with accepted_taxon, vs EuroSL's full rank + 85,396
accepted links) — so it was retired (see pipelines/README.md, "RETIRED").

Canonical mapping:
  taxon          = TaxonName
  rank           = TaxonRank
  status         = status, lowercased
  accepted_taxon = TaxonConcept, only emitted when status != accepted
  source_id      = TaxonUsageID
  parent_id      = IsChildTaxonOfID
  parent_rank    = "" (EuroSL has no per-row parent-rank join; the Go
                   ingest resolves it from the already-read row map)
"""
import csv
import sqlite3
import sys


def convert(in_path, out_path):
    con = sqlite3.connect(in_path)
    cur = con.cursor()
    cur.execute(
        'SELECT TaxonUsageID, TaxonName, TaxonRank, status, TaxonConcept, '
        'IsChildTaxonOfID FROM "EuroPlusMed.Plantae"'
    )

    row_count = 0
    taxa = set()
    ranks = {}
    statuses = {}
    with_parent = 0

    with open(out_path, "w", newline="", encoding="utf-8") as f:
        w = csv.writer(f, delimiter="|")
        w.writerow(["taxon", "rank", "status", "accepted_taxon", "source_id",
                     "parent_id", "parent_rank"])
        for source_id, taxon, rank, status, accepted_concept, parent_id in cur:
            if not taxon:
                continue
            rank = rank or ""
            status_norm = (status or "").strip().lower()
            is_accepted = status_norm == "accepted"
            accepted_taxon = "" if is_accepted else (accepted_concept or "")
            parent_id = parent_id or ""
            # parent_rank stays empty here: EuroSL delivers only the OWN
            # rank per row, not the parent's, without a self-join. The Go
            # ingest resolves parent_rank via the already-read row map
            # (see internal/adapters/namelist, Task 5).
            w.writerow([taxon, rank, status_norm, accepted_taxon, source_id or "",
                        parent_id, ""])
            row_count += 1
            taxa.add(taxon)
            ranks[rank] = ranks.get(rank, 0) + 1
            statuses[status_norm] = statuses.get(status_norm, 0) + 1
            if parent_id:
                with_parent += 1

    print(f"rows={row_count} taxa={len(taxa)} with_parent_id={with_parent}")
    print("statuses=" + ",".join(f"{k}:{v}" for k, v in sorted(statuses.items(), key=lambda kv: -kv[1])))
    print("ranks=" + ",".join(f"{k}:{v}" for k, v in sorted(ranks.items(), key=lambda kv: -kv[1])))


def main():
    in_path, out_path = sys.argv[1:3]
    convert(in_path, out_path)


if __name__ == "__main__":
    main()

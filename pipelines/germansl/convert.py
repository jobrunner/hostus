#!/usr/bin/env python3
"""GermanSL1.5.5.xlsx (sheet TCS) -> canonical name-list CSV.

GermanSL ships its xlsx with a broken <dimension ref="A1"/> declaration in
sheet1.xml (verified against the real 1.5.5 download: the sheet actually has
17 columns and 26129 data rows, but the declared dimension is a single
cell). openpyxl's read_only mode trusts that declaration and silently
truncates every row to column A; non-read-only mode additionally fails
outright on this file (KeyError: missing xl/drawings/drawing1.xml, a
dangling relationship). So this parses the OOXML parts directly instead of
going through openpyxl: xl/sharedStrings.xml for the string table and
xl/worksheets/sheet1.xml for cell values, driven by the workbook.xml (+
.rels) sheet-name lookup so we do not hardcode a physical sheet file name.

Column layout (from row 1 of TCS, resolved via sharedStrings):
TaxonUsageID | TaxonName | NameAuthor | SYNONYM | TaxonConceptID |
TaxonConcept | VernacularName | TaxonRank | GRUPPE | IsChildTaxonOfID |
IsChildTaxonOf | NACHWEIS | AccordingTo | HYBRID | BEGRUEND | ETABSTATUS |
EDITSTATUS

Canonical mapping:
  taxon          = TaxonName
  rank           = TaxonRank
  status         = "synonym" if SYNONYM (boolean) else "accepted"
  accepted_taxon = TaxonConcept, but only emitted for synonym rows
  source_id      = TaxonUsageID
  parent_id      = IsChildTaxonOfID
  parent_rank    = "" (GermanSL has no per-row parent-rank join, analogous
                   to EuroSL; the Go ingest resolves it from the
                   already-read row map)
  vernacular_de  = VernacularName (German common name; empty if the source
                   row carries none — GermanSL is the ONLY pipeline that
                   emits this column, see internal/adapters/namelist's
                   reader doc comment)
"""
import csv
import re
import sys
import zipfile
import xml.etree.ElementTree as ET

NS = {"m": "http://schemas.openxmlformats.org/spreadsheetml/2006/main",
      "r": "http://schemas.openxmlformats.org/officeDocument/2006/relationships"}


def col_letters_to_index(ref):
    """'AB12' -> ('AB', 12) -> 0-based column index for 'AB'."""
    m = re.match(r"([A-Z]+)(\d+)", ref)
    letters = m.group(1)
    idx = 0
    for ch in letters:
        idx = idx * 26 + (ord(ch) - ord("A") + 1)
    return idx - 1


def load_shared_strings(zf):
    try:
        data = zf.read("xl/sharedStrings.xml")
    except KeyError:
        return []
    root = ET.fromstring(data)
    strings = []
    for si in root.findall("m:si", NS):
        texts = si.findall(".//m:t", NS)
        strings.append("".join(t.text or "" for t in texts))
    return strings


def find_sheet_path(zf, sheet_name):
    wb_xml = ET.fromstring(zf.read("xl/workbook.xml"))
    rid = None
    for sheet in wb_xml.findall(".//m:sheets/m:sheet", NS):
        if sheet.get("name") == sheet_name:
            rid = sheet.get("{http://schemas.openxmlformats.org/officeDocument/2006/relationships}id")
            break
    if rid is None:
        raise ValueError(f"sheet {sheet_name!r} not found in workbook.xml")
    rels = ET.fromstring(zf.read("xl/_rels/workbook.xml.rels"))
    for rel in rels:
        if rel.get("Id") == rid:
            return "xl/" + rel.get("Target")
    raise ValueError(f"relationship {rid!r} not found")


def iter_rows(xlsx_path, sheet_name):
    with zipfile.ZipFile(xlsx_path) as zf:
        shared = load_shared_strings(zf)
        sheet_path = find_sheet_path(zf, sheet_name)
        root = ET.fromstring(zf.read(sheet_path))
        for row in root.findall(".//m:sheetData/m:row", NS):
            cells = {}
            max_idx = -1
            for c in row.findall("m:c", NS):
                ref = c.get("r")
                idx = col_letters_to_index(ref)
                max_idx = max(max_idx, idx)
                t = c.get("t")
                v = c.find("m:v", NS)
                if v is None:
                    cells[idx] = None
                    continue
                raw = v.text
                if t == "s":
                    cells[idx] = shared[int(raw)]
                elif t == "b":
                    cells[idx] = raw == "1"
                else:
                    cells[idx] = raw
            yield [cells.get(i) for i in range(max_idx + 1)]


def convert(in_path, out_path):
    rows = iter_rows(in_path, "TCS")
    header = next(rows)
    idx = {name: i for i, name in enumerate(header)}

    row_count = 0
    taxa = set()
    ranks = {}
    accepted_n = 0
    synonym_n = 0
    with_parent = 0
    has_parent_col = "IsChildTaxonOfID" in idx

    with open(out_path, "w", newline="", encoding="utf-8") as f:
        w = csv.writer(f, delimiter="|")
        w.writerow(["taxon", "rank", "status", "accepted_taxon", "source_id",
                     "parent_id", "parent_rank", "vernacular_de"])
        for r in rows:
            if len(r) <= idx["TaxonName"] or not r[idx["TaxonName"]]:
                continue
            taxon = r[idx["TaxonName"]]
            rank = r[idx["TaxonRank"]] or ""
            is_synonym = bool(r[idx["SYNONYM"]]) if len(r) > idx["SYNONYM"] else False
            status = "synonym" if is_synonym else "accepted"
            accepted_taxon = ""
            if is_synonym and len(r) > idx["TaxonConcept"]:
                accepted_taxon = r[idx["TaxonConcept"]] or ""
            source_id = r[idx["TaxonUsageID"]]
            source_id = "" if source_id is None else str(source_id)
            parent_id = ""
            if has_parent_col and len(r) > idx["IsChildTaxonOfID"]:
                parent_id = r[idx["IsChildTaxonOfID"]]
                parent_id = "" if parent_id is None else str(parent_id)
            vernacular_de = ""
            if len(r) > idx["VernacularName"]:
                vernacular_de = r[idx["VernacularName"]] or ""

            # parent_rank stays empty here, analogous to EuroSL: GermanSL
            # gives only the OWN rank per row, not the parent's, without a
            # self-join. The Go ingest resolves parent_rank via the
            # already-read row map (see internal/adapters/namelist).
            w.writerow([taxon, rank, status, accepted_taxon, source_id,
                        parent_id, "", vernacular_de])
            row_count += 1
            taxa.add(taxon)
            ranks[rank] = ranks.get(rank, 0) + 1
            if is_synonym:
                synonym_n += 1
            else:
                accepted_n += 1
            if parent_id:
                with_parent += 1

    print(f"rows={row_count} taxa={len(taxa)} accepted={accepted_n} synonym={synonym_n} with_parent_id={with_parent}")
    print("ranks=" + ",".join(f"{k}:{v}" for k, v in sorted(ranks.items(), key=lambda kv: -kv[1])))


def main():
    in_path, out_path = sys.argv[1:3]
    convert(in_path, out_path)


if __name__ == "__main__":
    main()

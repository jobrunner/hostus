import unittest
import io
import os
import tempfile
import zipfile
import xml.etree.ElementTree as ET

from convert import convert

NS = {"m": "http://schemas.openxmlformats.org/spreadsheetml/2006/main",
      "r": "http://schemas.openxmlformats.org/officeDocument/2006/relationships"}


def make_xlsx(path, header, rows):
    """Builds a minimal single-sheet xlsx (sheet name TCS) via sharedStrings,
    the encoding convert.py's XML-parsing path actually supports."""
    workbook_xml = (
        '<?xml version="1.0" encoding="UTF-8" standalone="yes"?>'
        '<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" '
        'xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">'
        '<sheets><sheet name="TCS" sheetId="1" r:id="rId1"/></sheets>'
        '</workbook>'
    )
    rels_xml = (
        '<?xml version="1.0" encoding="UTF-8" standalone="yes"?>'
        '<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">'
        '<Relationship Id="rId1" '
        'Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" '
        'Target="worksheets/sheet1.xml"/>'
        '</Relationships>'
    )

    def col_letter(i):
        letters = ""
        i += 1
        while i > 0:
            i, rem = divmod(i - 1, 26)
            letters = chr(ord("A") + rem) + letters
        return letters

    all_rows = [header] + rows
    shared = []
    shared_index = {}

    def sid(v):
        if v not in shared_index:
            shared_index[v] = len(shared)
            shared.append(v)
        return shared_index[v]

    def row_xml(values, row_num):
        cells = []
        for i, v in enumerate(values):
            if v is None or v == "":
                continue
            ref = f"{col_letter(i)}{row_num}"
            cells.append(f'<c r="{ref}" t="s"><v>{sid(v)}</v></c>')
        return f'<row r="{row_num}">{"".join(cells)}</row>'

    rows_xml = "".join(row_xml(r, i + 1) for i, r in enumerate(all_rows))
    sheet_xml = (
        '<?xml version="1.0" encoding="UTF-8" standalone="yes"?>'
        '<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">'
        f'<sheetData>{rows_xml}</sheetData>'
        '</worksheet>'
    )
    shared_strings_xml = (
        '<?xml version="1.0" encoding="UTF-8" standalone="yes"?>'
        '<sst xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" '
        f'count="{len(shared)}" uniqueCount="{len(shared)}">'
        + "".join(f'<si><t>{s}</t></si>' for s in shared)
        + '</sst>'
    )

    with zipfile.ZipFile(path, "w") as zf:
        zf.writestr("xl/workbook.xml", workbook_xml)
        zf.writestr("xl/_rels/workbook.xml.rels", rels_xml)
        zf.writestr("xl/worksheets/sheet1.xml", sheet_xml)
        zf.writestr("xl/sharedStrings.xml", shared_strings_xml)


class TestConvertParentChain(unittest.TestCase):
    HEADER = ["TaxonUsageID", "TaxonName", "NameAuthor", "SYNONYM",
              "TaxonConceptID", "TaxonConcept", "VernacularName", "TaxonRank",
              "GRUPPE", "IsChildTaxonOfID", "IsChildTaxonOf", "NACHWEIS",
              "AccordingTo", "HYBRID", "BEGRUEND", "ETABSTATUS", "EDITSTATUS"]

    def test_parent_id_column_present_parent_rank_empty(self):
        with tempfile.TemporaryDirectory() as tmp:
            xlsx_path = os.path.join(tmp, "test.xlsx")
            row = ["id1", "Salsola kali", "L.", "", "id1", "Salsola kali",
                   "", "Species", "", "id0", "Salsola kali aggr.", "", "",
                   "", "", "", ""]
            make_xlsx(xlsx_path, self.HEADER, [row])

            out_path = os.path.join(tmp, "out.csv")
            convert(xlsx_path, out_path)

            with open(out_path, encoding="utf-8") as f:
                header = f.readline().strip().split("|")
                self.assertIn("parent_id", header)
                self.assertIn("parent_rank", header)
                data_row = f.readline().strip().split("|")
                row_dict = dict(zip(header, data_row))
                self.assertEqual(row_dict["parent_id"], "id0")
                self.assertEqual(row_dict["parent_rank"], "")


if __name__ == "__main__":
    unittest.main()

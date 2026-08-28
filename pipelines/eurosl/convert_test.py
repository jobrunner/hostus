import unittest
import sqlite3
import tempfile
import os
from convert import convert


class TestConvertParentChain(unittest.TestCase):
    def test_parent_id_column_present_parent_rank_empty(self):
        with tempfile.TemporaryDirectory() as tmp:
            db_path = os.path.join(tmp, "test.sqlite")
            conn = sqlite3.connect(db_path)
            conn.execute('CREATE TABLE "EuroPlusMed.Plantae" ('
                'TaxonUsageID TEXT, TaxonName TEXT, NameAuthor TEXT, status TEXT, '
                'TaxonConceptID TEXT, TaxonConcept TEXT, TaxonRank TEXT, '
                'IsChildTaxonOfID TEXT, IsChildTaxonOf TEXT, fullname TEXT, '
                '"nb.children" TEXT, doubt TEXT, secName TEXT, ambiguous INTEGER, '
                'OriginalName TEXT, AccordingTo TEXT)')
            conn.execute('INSERT INTO "EuroPlusMed.Plantae" VALUES '
                '("id1", "Salsola kali", "L.", "Accepted", "id1", "Salsola kali", '
                '"Species", "id0", "Salsola kali aggr.", "", "", "FALSE", "", 0, "", "")')
            conn.commit()
            conn.close()

            out_path = os.path.join(tmp, "out.csv")
            convert(db_path, out_path)

            with open(out_path, encoding="utf-8") as f:
                header = f.readline().strip().split("|")
                self.assertIn("parent_id", header)
                self.assertIn("parent_rank", header)
                row = f.readline().strip().split("|")
                row_dict = dict(zip(header, row))
                self.assertEqual(row_dict["parent_id"], "id0")
                # parent_rank stays empty here: EuroSL delivers only the
                # OWN rank per row, not the parent's, without a self-join.
                # The Go ingest resolves parent_rank via the already-read
                # row map (see reader.go / Task 5).
                self.assertEqual(row_dict["parent_rank"], "")


if __name__ == "__main__":
    unittest.main()

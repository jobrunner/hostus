# hostus Namensraum-/Klassifikations-/Aggregat-Redesign Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** hostus bekommt drei Namensräume (WCVP/EuroSL/GermanSL), ein erweitertes
Rang-Set (inkl. Sektionen/Aggregate/informelle Klade-Ebenen), ein
Aggregat-Modell mit vorberechnetem Namensraum-Vergleich, und `/v1/match`
liefert bei Sammel-Rängen eine Entscheidungsstruktur statt einer Sackgasse.
Traits wandern aus hostus heraus.

**Architecture:** Zwei Fälle werden strikt getrennt: **Fall A** (Anreicherung
bestehender WCVP-Konzepte per Namens-Crosswalk — erweitert die schon
existierende SP9-`IngestNameSpace`-Maschinerie) und **Fall B** (genuin neue
Konzepte für Ränge, die WCVP nicht kennt — nutzt den bestehenden
`BeginIngest`/`IngestTx`-Backbone-Pfad, mit `backbone_id = "eurosl"` bzw.
`"germansl"`). Eine neue `concept_aggregate`-Tabelle trägt die
Aggregat-Mitglieder-Relation, eine neue `concept_agreement`-Tabelle den
vorberechneten Namensraum-Vergleich.

**Tech Stack:** Go 1.26, `modernc.org/sqlite` (FTS5), bestehende
hexagonale Struktur (`internal/{domain,application,ports,adapters}`).

**Spec:** `docs/superpowers/specs/2026-08-27-hostus-namensraum-redesign-design.md`

**Korrektur gegenüber dem Spec, gefunden beim Plan-Schreiben:** Spec-Abschnitt
7 sagt, Fall A reiche `genuineBearerWinner` weiter. Der bestehende Code
(`internal/application/namespace_ingest.go:276`) verwendet für den
Namensraum-Crosswalk bewusst `policyRefuseAmbiguity`, nicht
`policyResolveGenuineBearer` — mit einer dokumentierten Begründung (ein still
tie-gebrochener Treffer könnte einem Konzept eine zweite Namensraum-Zeile
geben, was `domain.ResolveTargetSpace`s Auswahl auf ungemessene Weise
beeinflusst). Dieser Plan folgt der **bestehenden, bewussten Praxis**
(`policyRefuseAmbiguity`), nicht dem, was das Spec sagte.

## Global Constraints

- Go 1.26, nur die in CLAUDE.md gelistete Bibliothek (`modernc.org/sqlite`,
  `gorilla/mux`, `viper`, `cobra`, OTel, `go-sdk` MCP) — keine neue Dependency.
- Hexagon-Grenzen gelten (`internal/application` importiert nur
  `internal/domain` + `internal/ports`) — geprüft von `make arch`.
- **Nie von der Repository lesen, während eine Ingest-Transaktion offen ist**
  (`SetMaxOpenConns(1)` → echter Deadlock). `IngestNameSpace` und
  `IngestTraits` lösen das mit einer strikten Zwei-Phasen-Trennung
  (RESOLVE ohne offene Tx, dann WRITE in einer Tx) — jeder neue Ingest-Pfad
  hier muss dasselbe Muster übernehmen.
- Jede TDD-Task-DoD: `go build ./...`, `go vet ./...`, `go test -timeout 180s ./...`
  für die berührten Pakete, `make lint` (inkl. `_test.go`), `make mutation PKG=<pkg>`
  wo neue Verzweigungslogik entsteht.
- Docs Deutsch, Code-Kommentare sparsam Englisch (bestehende Konvention).
- Nie rohe Bulk-Daten committen. `pipelines/*/.cache/` bleibt ignoriert.
- **Eine Zahl, die nicht gemessen wurde, darf nicht behauptet werden** — jeder
  Task, der eine Kennzahl in einen Kommentar/Report schreibt, muss sie gegen
  echte Daten (`pipelines/{eurosl,germansl}/.cache/`) gemessen haben.
- Branch: `feature/namensraum-klassifikation-aggregat-redesign` (existiert
  bereits, Spec-Commit liegt dort).

---

## Task 1: Kanonisches Rang-Set erweitern

**Files:**
- Modify: `internal/domain/taxon.go:9-82` (Rank-Konstanten, `ParseRank`, `ParseRankLenient`)
- Test: `internal/domain/taxon_test.go`

**Interfaces:**
- Produces: `domain.Rank` mit den neuen Konstanten (siehe unten), `ParseRank`/`ParseRankLenient`
  unverändert in der Signatur (`func ParseRank(s string) (Rank, error)`,
  `func ParseRankLenient(s string) (Rank, string)`), aber mit erweiterter
  Mapping-Tabelle.

- [ ] **Step 1: Failing Test für jeden neuen kanonischen Rang schreiben**

```go
func TestParseRank_ExtendedVocabulary(t *testing.T) {
	cases := map[string]domain.Rank{
		"ORDER": domain.RankOrder, "CLASS": domain.RankClass,
		"SECTION": domain.RankSection, "SUBSECTION": domain.RankSubsection,
		"SUBGENUS": domain.RankSubgenus, "SERIES": domain.RankSeries,
		"SPECIES_AGGREGATE": domain.RankSpeciesAggregate,
		"GENUS_AGGREGATE": domain.RankGenusAggregate,
		"SUBFAMILY": domain.RankSubfamily, "TRIBE": domain.RankTribe,
		"PHYLUM": domain.RankPhylum, "SUBDIVISION": domain.RankSubdivision,
		"SUBCLASS": domain.RankSubclass, "SUPERORDER": domain.RankSuperorder,
		"COLL_SPECIES": domain.RankCollSpecies,
		"SUBSPECIES_GROUP": domain.RankSubspeciesGroup,
		"PROLES": domain.RankProles, "RACE": domain.RankRace,
		"CONVAR": domain.RankConvar, "GREX": domain.RankGrex,
		"UNRANKED_INFRAGENERIC": domain.RankUnrankedInfrageneric,
		"UNRANKED_INFRASPECIFIC": domain.RankUnrankedInfraspecific,
		"ROOT": domain.RankRoot,
	}
	for in, want := range cases {
		got, err := domain.ParseRank(in)
		if err != nil {
			t.Errorf("ParseRank(%q): unexpected error %v", in, err)
		}
		if got != want {
			t.Errorf("ParseRank(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseRank_InformalCladeCarriesTier(t *testing.T) {
	got, err := domain.ParseRank("INFORMAL_CLADE_5")
	if err != nil {
		t.Fatalf("ParseRank(%q): unexpected error %v", "INFORMAL_CLADE_5", err)
	}
	if got != domain.RankInformalClade {
		t.Errorf("ParseRank(%q) = %q, want %q", "INFORMAL_CLADE_5", got, domain.RankInformalClade)
	}
}
```

- [ ] **Step 2: Test ausführen, Fehlschlag bestätigen**

Run: `go test ./internal/domain/... -run TestParseRank_ExtendedVocabulary -v`
Erwartet: FAIL, `undefined: domain.RankOrder` etc.

- [ ] **Step 3: Rang-Konstanten + Mapping ergänzen**

In `internal/domain/taxon.go`, nach der bestehenden `const` Block
(`RankFamily`...`RankOther`) ergänzen:

```go
const (
	RankRoot             Rank = "ROOT"
	RankPhylum           Rank = "PHYLUM"
	RankSubdivision      Rank = "SUBDIVISION"
	RankInformalClade    Rank = "INFORMAL_CLADE"
	RankClass            Rank = "CLASS"
	RankSubclass         Rank = "SUBCLASS"
	RankSuperorder       Rank = "SUPERORDER"
	RankOrder            Rank = "ORDER"
	RankSubfamily        Rank = "SUBFAMILY"
	RankTribe            Rank = "TRIBE"
	RankSubgenus         Rank = "SUBGENUS"
	RankSection          Rank = "SECTION"
	RankSubsection       Rank = "SUBSECTION"
	RankSeries           Rank = "SERIES"
	RankSpeciesAggregate Rank = "SPECIES_AGGREGATE"
	RankGenusAggregate   Rank = "GENUS_AGGREGATE"
	RankCollSpecies      Rank = "COLL_SPECIES"
	RankSubspeciesGroup  Rank = "SUBSPECIES_GROUP"
	RankProles           Rank = "PROLES"
	RankRace             Rank = "RACE"
	RankConvar           Rank = "CONVAR"
	RankGrex             Rank = "GREX"
	RankUnrankedInfrageneric  Rank = "UNRANKED_INFRAGENERIC"
	RankUnrankedInfraspecific Rank = "UNRANKED_INFRASPECIFIC"
)
```

`ParseRank`'s Switch (Zeile ~54-82) um jeden neuen Wert erweitern (gleiches
Muster wie die bestehenden: `case RankOrder: return RankOrder, nil` usw.).
Für `INFORMAL_CLADE_<n>` (Tier-Suffix, z.B. GermanSL `CL1`-`CL5`) einen
Präfix-Check VOR dem Switch einbauen:

```go
if strings.HasPrefix(strings.ToUpper(strings.TrimSpace(s)), "INFORMAL_CLADE") {
	return RankInformalClade, nil
}
```

- [ ] **Step 4: Test ausführen, Erfolg bestätigen**

Run: `go test ./internal/domain/... -run TestParseRank -v`
Erwartet: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/domain/taxon.go internal/domain/taxon_test.go
git commit -m "feat(domain): erweitertes kanonisches Rang-Set (EuroSL+GermanSL)"
```

---

## Task 2: Schema erweitern — Klassifikation, Aggregat-Mitglieder, concept_agreement

**Files:**
- Modify: `internal/adapters/sqlite/schema.sql`
- Modify: `internal/adapters/sqlite/db.go` (Legacy-Migration, `PRAGMA table_info`-Guard, siehe bestehendes Muster für `trait_value.resolution`)
- Test: `internal/adapters/sqlite/db_internal_test.go`

**Interfaces:**
- Produces: drei neue Spalten auf `taxon_concept` (`family`, `order_name`, `class_name`
  — `order`/`class` sind SQLite-reservierte Wörter, daher `order_name`/`class_name`),
  zwei neue Tabellen `concept_aggregate`, `concept_agreement`.

- [ ] **Step 1: Failing Test — Schema-Migration ist idempotent und additiv**

```go
func TestSchema_ClassificationColumnsExist(t *testing.T) {
	db := openTestDB(t) // bestehender Test-Helper in db_internal_test.go
	rows, err := db.conn.Query(`PRAGMA table_info(taxon_concept)`)
	if err != nil {
		t.Fatalf("PRAGMA table_info: %v", err)
	}
	defer rows.Close()
	cols := map[string]bool{}
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dflt any
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			t.Fatalf("scan: %v", err)
		}
		cols[name] = true
	}
	for _, want := range []string{"family", "order_name", "class_name"} {
		if !cols[want] {
			t.Errorf("taxon_concept missing column %q", want)
		}
	}
}

func TestSchema_ConceptAggregateAndAgreementTablesExist(t *testing.T) {
	db := openTestDB(t)
	for _, table := range []string{"concept_aggregate", "concept_agreement"} {
		var name string
		err := db.conn.QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&name)
		if err != nil {
			t.Errorf("table %q not found: %v", table, err)
		}
	}
}
```

- [ ] **Step 2: Test ausführen, Fehlschlag bestätigen**

Run: `go test ./internal/adapters/sqlite/... -run TestSchema_ -v`
Erwartet: FAIL (Spalten/Tabellen fehlen)

- [ ] **Step 3: Schema ergänzen**

In `schema.sql`, `taxon_concept`-Definition erweitern (nach `rank_verbatim`):

```sql
  -- Klassifikation oberhalb der Familie (Herkunft: EuroSL/GermanSL Fall B,
  -- siehe docs/superpowers/specs/2026-08-27-hostus-namensraum-redesign-design.md
  -- Abschnitt 4). NULL wenn unbekannt — nie geraten. WCVP-Konzepte (backbone_id
  -- = "wcvp") haben diese Spalten aus Fall A per Crosswalk befüllt, nicht aus
  -- eigenen Daten (WCVP führt keine Ränge oberhalb FAMILY).
  family      TEXT,
  order_name  TEXT,
  class_name  TEXT
```

Neue Tabellen am Dateiende (vor `bundle_meta`) ergänzen:

```sql
-- Aggregat-/Sektions-Mitgliedschaft (Fall B). aggregate_concept_id ist immer
-- ein natives eurosl:/germansl:-Konzept (RankSpeciesAggregate/RankSection/...);
-- member_concept_id ist der WCVP-Sippen-Konzept, den das Aggregat umfasst.
-- Kein WCVP-Konzept ist je die aggregate-Seite (WCVP kennt keine Aggregate).
CREATE TABLE IF NOT EXISTS concept_aggregate (
  aggregate_concept_id TEXT NOT NULL REFERENCES taxon_concept(id),
  member_concept_id    TEXT NOT NULL REFERENCES taxon_concept(id),
  PRIMARY KEY (aggregate_concept_id, member_concept_id)
);
CREATE INDEX IF NOT EXISTS idx_concept_aggregate_member ON concept_aggregate(member_concept_id);

-- Vorberechneter Namensraum-Vergleich (Spec Abschnitt 5). Ein Eintrag pro
-- Paar (eurosl-Aggregat, germansl-Aggregat), das beim Ingest als
-- namensgleiches Gegenstück erkannt wurde. agreement ist einer von
-- identical|subset|superset|overlap|disjoint|one_sided (domain.Agreement).
CREATE TABLE IF NOT EXISTS concept_agreement (
  eurosl_concept_id   TEXT REFERENCES taxon_concept(id),   -- NULL bei one_sided (nur germansl)
  germansl_concept_id TEXT REFERENCES taxon_concept(id),   -- NULL bei one_sided (nur eurosl)
  agreement           TEXT NOT NULL,
  agreement_text      TEXT NOT NULL,
  only_in_eurosl       TEXT NOT NULL DEFAULT '',  -- komma-getrennte WCVP-Konzept-IDs
  only_in_germansl     TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (eurosl_concept_id, germansl_concept_id)
);
```

In `db.go`, im bestehenden Legacy-Migrationsblock (analog zum dortigen
`trait_value.resolution`-Guard) ergänzen:

```go
if err := addColumnIfMissing(db.conn, "taxon_concept", "family", "TEXT"); err != nil {
	return nil, fmt.Errorf("sqlite: migrating taxon_concept.family: %w", err)
}
if err := addColumnIfMissing(db.conn, "taxon_concept", "order_name", "TEXT"); err != nil {
	return nil, fmt.Errorf("sqlite: migrating taxon_concept.order_name: %w", err)
}
if err := addColumnIfMissing(db.conn, "taxon_concept", "class_name", "TEXT"); err != nil {
	return nil, fmt.Errorf("sqlite: migrating taxon_concept.class_name: %w", err)
}
```

(`addColumnIfMissing` folgt dem bestehenden `PRAGMA table_info`-Guard-Muster,
das schon für `trait_value.resolution` existiert — falls dort kein
generischer Helper existiert, diesen Task um das Extrahieren eines solchen
Helpers aus der bestehenden Migration erweitern, statt Code zu duplizieren.)

- [ ] **Step 4: Test ausführen, Erfolg bestätigen**

Run: `go test ./internal/adapters/sqlite/... -run TestSchema_ -v`
Erwartet: PASS

- [ ] **Step 5: DoD + Commit**

```bash
make lint && go test ./internal/adapters/sqlite/...
git add internal/adapters/sqlite/schema.sql internal/adapters/sqlite/db.go internal/adapters/sqlite/db_internal_test.go
git commit -m "feat(sqlite): Schema für Klassifikation, Aggregat-Mitglieder, concept_agreement"
```

---

## Task 3: EuroSL/GermanSL-Pipelines um Eltern-Kette + Rang erweitern

**Files:**
- Modify: `pipelines/eurosl/convert.py`
- Modify: `pipelines/germansl/convert.py`
- Test: `pipelines/eurosl/convert_test.py`, `pipelines/germansl/convert_test.py` (neu, `python3 -m unittest`, Muster wie bestehende Pipeline-Tests unter `pipelines/*/`)

**Interfaces:**
- Produces: erweiterte kanonische CSV-Form
  `taxon|rank|status|accepted_taxon|source_id|parent_id|parent_rank`
  (zwei neue Spalten gegenüber heute) für beide Pipelines. `parent_id` ist
  die Quelle-eigene `IsChildTaxonOfID`, `parent_rank` deren `TaxonRank`
  (roh, unkonvertiert — Konvertierung passiert erst im Go-Ingest via
  `domain.ParseRank`).

- [ ] **Step 1: Failing Test (Python, `unittest`) für die neue Spalte**

```python
# pipelines/eurosl/convert_test.py
import unittest
import sqlite3
import tempfile
import os
from convert import convert

class TestConvertParentChain(unittest.TestCase):
    def test_parent_id_and_rank_columns_present(self):
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
                self.assertEqual(row_dict["parent_rank"], "Species Aggregate")

if __name__ == "__main__":
    unittest.main()
```

- [ ] **Step 2: Test ausführen, Fehlschlag bestätigen**

Run: `cd pipelines/eurosl && python3 -m unittest convert_test.py -v`
Erwartet: FAIL (`parent_id`/`parent_rank` fehlen im Header)

- [ ] **Step 3: `convert.py` erweitern**

In `pipelines/eurosl/convert.py`, die SQL-Query bzw. den CSV-Writer-Header
und die Zeilen-Konstruktion um `IsChildTaxonOfID`/`TaxonRank` der aktuellen
Zeile selbst (nicht des Elternteils — die "eigene" Rang-Spalte gab es
implizit schon über `rank`, `parent_id`/`parent_rank` sind neu und
referenzieren den ELTERN-Eintrag) erweitern:

```python
writer.writerow(["taxon", "rank", "status", "accepted_taxon", "source_id",
                  "parent_id", "parent_rank"])
# ... in der Zeilen-Schleife:
writer.writerow([taxon_name, rank, status, accepted_taxon, source_id,
                  row["IsChildTaxonOfID"] or "", ""])
```

`parent_rank` bleibt zunächst leer in EuroSL (die Quelle liefert pro Zeile
nur den eigenen Rang, nicht den des Elternteils per Join) — der Go-Ingest
löst `parent_rank` später selbst über die bereits eingelesene Zeilen-Map
auf (siehe Task 5), daher genügt hier `parent_id`. **Kein zweiter Lookup
in Python nötig** — das vermeidet eine zweite, potenziell abweichende
Implementierung der Eltern-Auflösung.

Testanpassung: `self.assertEqual(row_dict["parent_rank"], "")` (leer, wie
oben begründet) statt `"Species Aggregate"`.

Gleiches Muster in `pipelines/germansl/convert.py` (dort schon
`IsChildTaxonOfID` im Header vorhanden, siehe `convert.py`s Docstring
Zeile 17-23) — nur den CSV-Output um die beiden Spalten erweitern, exakt
analog.

- [ ] **Step 4: Test ausführen, Erfolg bestätigen**

Run: `cd pipelines/eurosl && python3 -m unittest convert_test.py -v && cd ../germansl && python3 -m unittest convert_test.py -v`
Erwartet: PASS

- [ ] **Step 5: Pipelines neu laufen lassen, Kennzahlen messen**

```bash
cd pipelines/eurosl && bash build.sh    # neuer eurosl-canonical.csv mit parent_id
cd ../germansl
# GermanSL VORHER neu pinnen: SOURCE_URL in build.sh auf den 2025-09-29-Stand
# prüfen/aktualisieren (siehe Spec Abschnitt 0), dann:
rm -f .cache/GermanSL_1.5.5.zip .cache/GermanSL1.5.5.xlsx
bash build.sh
```

Gemessene Zeilenzahl beider `*-canonical.csv` gegen die im Spec genannten
Zahlen (139.039 EuroSL, 26.129 GermanSL) vergleichen und im Commit
dokumentieren — eine Abweichung ist ein Fund, kein Fehler, aber muss benannt
werden (Constraint "keine unbemerkte Drift").

- [ ] **Step 6: Commit**

```bash
git add pipelines/eurosl/convert.py pipelines/eurosl/convert_test.py \
        pipelines/germansl/convert.py pipelines/germansl/convert_test.py \
        pipelines/germansl/build.sh
git commit -m "feat(pipelines): Eltern-Kette in EuroSL/GermanSL-CSV, GermanSL neu gepinnt"
```

---

## Task 4: Fall A — Klassifikation + Vernakularnamen im Namensraum-Crosswalk

**Files:**
- Modify: `internal/application/namespace_ingest.go`
- Modify: `internal/ports/output/repository.go` (`IngestTx`-Interface)
- Modify: `internal/adapters/sqlite/` (neue `IngestTx`-Methode-Implementierung, Datei nach bestehendem Muster suchen — dieselbe Datei, die `AddNameSpaceEntry` implementiert)
- Test: `internal/application/namespace_ingest_test.go`

**Consumes:** `domain.NameSpaceEntry` (Task-unabhängig, bereits vorhanden),
`policyRefuseAmbiguity` (bereits vorhanden, `internal/application/traits_ingest.go:580`).

**Interfaces:**
- Produces: `NameRow` erweitert um `Family`, `OrderName`, `ClassName`,
  `VernacularDE string` (leer wenn nicht vorhanden); neue
  `IngestTx.UpsertClassification(conceptID string, family, orderName, className string) error`
  und `IngestTx.AddVernacularName(conceptID string, v domain.VernacularName) error`
  (`domain.VernacularName{Language, Name string}` — neuer, kleiner Domain-Typ,
  in `internal/domain/taxon.go` zu ergänzen).

- [ ] **Step 1: Failing Test — Klassifikation wird aus NameRow geschrieben**

```go
func TestIngestNameSpace_WritesClassificationOntoMatchedConcept(t *testing.T) {
	repo := seededNamespaceRepo(t) // Helper, seedet einen WCVP-Konzept "Salsola kali"
	src := staticNameRows{{
		Taxon: "Salsola kali", SourceID: "1408c0e8", Status: "accepted",
		Family: "Chenopodiaceae", OrderName: "Caryophyllales", ClassName: "Magnoliopsida",
	}}
	meta := domain.NameSpaceMeta{ID: "eurosl", Version: "2026-08-27", Redistribution: domain.RedistributionUnknown}

	report, err := application.IngestNameSpace(context.Background(), repo, src, meta)
	if err != nil {
		t.Fatalf("IngestNameSpace: unexpected error: %v", err)
	}
	if report.Matched != 1 {
		t.Fatalf("report.Matched = %d, want 1", report.Matched)
	}

	concept, _, _, _, err := repo.Concept(context.Background(), report.firstMatchedConceptID())
	if err != nil {
		t.Fatalf("Concept: unexpected error: %v", err)
	}
	if concept.Family != "Chenopodiaceae" {
		t.Errorf("concept.Family = %q, want %q", concept.Family, "Chenopodiaceae")
	}
	if concept.OrderName != "Caryophyllales" {
		t.Errorf("concept.OrderName = %q, want %q", concept.OrderName, "Caryophyllales")
	}
}
```

(`report.firstMatchedConceptID()` und `staticNameRows` sind Test-Helper,
im selben Testfile zu definieren — `staticNameRows` implementiert
`NameRowSource` mit den erweiterten `NameRow`-Feldern.)

- [ ] **Step 2: Test ausführen, Fehlschlag bestätigen**

Run: `go test ./internal/application/... -run TestIngestNameSpace_WritesClassification -v`
Erwartet: FAIL (`NameRow` hat kein Feld `Family`, `domain.Concept` kein Feld `Family`)

**Wichtiger Hinweis zur Herkunft von `Family`/`OrderName`/`ClassName`:** Diese
sind **keine** flachen CSV-Spalten — die kanonische CSV (Task 3) trägt pro
Zeile nur `parent_id`/`parent_rank`. Die Bridge-Schicht (`internal/app/ingest.go`,
`nameSpaceRowSource.Rows()`, Zeile 208-214) muss VOR dem Aufruf von
`application.IngestNameSpace` einmalig **alle** Zeilen derselben Quelle
(inkl. der Fall-B-Zeilen für Familie/Ordnung/Klasse aus Task 5) in eine
`map[sourceID]namelist.Row` laden und für jede Species-Zeile die
Eltern-Kette (`ParentID` → `ParentID` → …) hochlaufen, bis Zeilen mit
`Rank == "Family"/"Order"/"Class"` (bzw. GermanSL `FAM`/`ORD`/`KLA`)
gefunden sind — dieselbe Logik, die Task 5 für `aboveSpecies` bereits
kennt. Das Ergebnis (drei Strings pro Species-Zeile) füllt `NameRow.Family`
etc., BEVOR `IngestNameSpace` aufgerufen wird. Dieser Traversierungs-Helper
gehört nach `internal/app/ingest.go` (Bridge-Schicht), nicht in
`internal/application` — er kennt `namelist.Row`, das laut Hexagon-Regel
nur außerhalb von `internal/application` importiert werden darf.

- [ ] **Step 3: `domain.Concept` + `NameRow` + `IngestTx` erweitern, implementieren**

`internal/domain/taxon.go`, `Concept`-Struct um die drei Felder erweitern
(analog zu `RankVerbatim`):

```go
	Family     string
	OrderName  string
	ClassName  string
```

`internal/application/namespace_ingest.go`, `NameRow`-Struct erweitern:

```go
	Family    string
	OrderName string
	ClassName string
	VernacularDE string
```

`writeNameSpaceRow` (Zeile 189) nach dem `AddNameSpaceEntry`-Aufruf ergänzen:

```go
	if row.Family != "" || row.OrderName != "" || row.ClassName != "" {
		if err := tx.UpsertClassification(res.conceptID, row.Family, row.OrderName, row.ClassName); err != nil {
			return fmt.Errorf("application: writing classification for concept %q: %w", res.conceptID, err)
		}
	}
	if row.VernacularDE != "" {
		if err := tx.AddVernacularName(res.conceptID, domain.VernacularName{Language: "de", Name: row.VernacularDE}); err != nil {
			return fmt.Errorf("application: writing vernacular name for concept %q: %w", res.conceptID, err)
		}
	}
```

`internal/ports/output/repository.go`, `IngestTx`-Interface um die zwei
neuen Methoden ergänzen (nach `AddNameSpaceEntry`):

```go
	// UpsertClassification records family/order/class for conceptID — see
	// Task 2's schema (taxon_concept.family/order_name/class_name). Empty
	// strings are written as SQL NULL, never as "".
	UpsertClassification(conceptID string, family, orderName, className string) error
	// AddVernacularName writes one vernacular-name row (see the existing
	// `vernacular` table, schema.sql:168).
	AddVernacularName(conceptID string, v domain.VernacularName) error
```

Die SQLite-Implementierung (in der Datei, die `AddNameSpaceEntry`
implementiert — per `grep -n "func.*AddNameSpaceEntry" internal/adapters/sqlite/*.go`
zu finden) um beide Methoden ergänzen, exakt nach dem Muster der
Nachbarmethoden dort (vorbereitetes `UPDATE`/`INSERT OR IGNORE`-Statement,
Fehler mit `fmt.Errorf("sqlite: ...: %w", err)` gewrappt).

- [ ] **Step 4: Test ausführen, Erfolg bestätigen**

Run: `go test ./internal/application/... ./internal/adapters/sqlite/... -v`
Erwartet: PASS

- [ ] **Step 5: DoD + Commit**

```bash
make lint && make mutation PKG=./internal/application
git add internal/domain/taxon.go internal/application/namespace_ingest.go \
        internal/ports/output/repository.go internal/adapters/sqlite/
git commit -m "feat(namespace): Klassifikation + Vernakularnamen im Fall-A-Crosswalk"
```

---

## Task 5: Fall B — natives Konzept-Ingest für EuroSL/GermanSL (höhere Ränge, Aggregate)

**Files:**
- Create: `internal/application/nativespace_ingest.go`
- Test: `internal/application/nativespace_ingest_test.go`

**Consumes:** `output.Repository.BeginIngest(ctx, bv domain.BackboneVersion) (IngestTx, error)`
(bestehend), `IngestTx.UpsertName`/`UpsertConcept`/`LinkName`/`Finalize`
(bestehend), `domain.ParseRankLenient` (Task 1), `domain.RankSpeciesAggregate`
etc. (Task 1).

**Interfaces:**
- Produces:
  ```go
  type NativeRow struct {
  	Taxon        string
  	SourceID     string
  	Rank         string // roh, wird via domain.ParseRankLenient gemappt
  	Status       string
  	ParentID     string // Quelle-eigene IsChildTaxonOfID; "" = kein Elternteil
  }
  type NativeRowSource interface{ Rows() []NativeRow }
  func IngestNativeSpace(ctx context.Context, repo output.Repository, src NativeRowSource, bv domain.BackboneVersion, minRank domain.Rank) (NativeSpaceIngestReport, error)
  ```
  `minRank` filtert: nur Zeilen, deren Rang **oberhalb** von `SPECIES` liegt
  (Familie, Ordnung, Klasse, Sektion, Aggregat, Untergattung, …) werden als
  eigene Konzepte geschrieben — Species/Infraspecific-Zeilen werden
  übersprungen (die gehören zu Fall A, Task 4), damit keine doppelten
  Konzepte für dieselbe Art in zwei Backbones entstehen.

- [ ] **Step 1: Failing Test — ein Aggregat-Konzept wird als natives Konzept geschrieben**

```go
func TestIngestNativeSpace_WritesAggregateAsOwnConcept(t *testing.T) {
	repo := newInMemoryRepo(t) // bestehender Test-Helper (siehe match_test.go-Umfeld)
	src := staticNativeRows{
		{Taxon: "Salsola", SourceID: "genus1", Rank: "Genus", Status: "accepted"},
		{Taxon: "Salsola kali aggr.", SourceID: "agg1", Rank: "Species Aggregate", Status: "accepted", ParentID: "genus1"},
	}
	bv := domain.BackboneVersion{ID: "eurosl", Version: "2026-08-27", Redistribution: domain.RedistributionUnknown}

	report, err := application.IngestNativeSpace(context.Background(), repo, src, bv, domain.RankSpeciesAggregate)
	if err != nil {
		t.Fatalf("IngestNativeSpace: unexpected error: %v", err)
	}
	if report.Written != 1 {
		t.Fatalf("report.Written = %d, want 1 (Genus soll uebersprungen werden, da unterhalb minRank in der Hierarchie ausserhalb des Aggregat-Kontexts steht)", report.Written)
	}

	concept, _, _, _, err := repo.Concept(context.Background(), "eurosl:concept:agg1")
	if err != nil {
		t.Fatalf("Concept: unexpected error: %v", err)
	}
	if concept.Rank != domain.RankSpeciesAggregate {
		t.Errorf("concept.Rank = %q, want %q", concept.Rank, domain.RankSpeciesAggregate)
	}
	if concept.BackboneID != "eurosl" {
		t.Errorf("concept.BackboneID = %q, want %q", concept.BackboneID, "eurosl")
	}
}
```

- [ ] **Step 2: Test ausführen, Fehlschlag bestätigen**

Run: `go test ./internal/application/... -run TestIngestNativeSpace -v`
Erwartet: FAIL (`application.IngestNativeSpace` existiert nicht)

- [ ] **Step 3: `IngestNativeSpace` implementieren**

```go
package application

import (
	"context"
	"fmt"

	"github.com/jobrunner/hostus/internal/domain"
	"github.com/jobrunner/hostus/internal/ports/output"
)

type NativeRow struct {
	Taxon    string
	SourceID string
	Rank     string
	Status   string
	ParentID string
}

type NativeRowSource interface{ Rows() []NativeRow }

type NativeSpaceIngestReport struct {
	Space          string
	Rows           int
	Written        int
	Skipped        int // Rang <= SPECIES, gehört zu Fall A
	UnknownRank    int // domain.ParseRankLenient fiel auf RankOther zurück
	UnknownRankSample []string
}

// IngestNativeSpace schreibt jede Zeile von src, deren Rang STRIKT OBERHALB
// von SPECIES liegt (Familie, Ordnung, Klasse, Sektion, Aggregat,
// Untergattung, ...), als eigenes taxon_concept mit backbone_id = bv.ID.
// Species-/Infraspecific-Zeilen werden übersprungen — die sind Fall A
// (IngestNameSpace, Task 4), nicht Fall B.
//
// Rang-Mapping ist LENIENT (domain.ParseRankLenient), nicht strikt: ein
// unbekannter Rohwert darf den Ingest nicht abbrechen (siehe Spec Abschnitt
// 11, Korrektheits-Test 1) — er wird als RankOther gezählt und mit Beispiel
// gemeldet (report.UnknownRankSample), niemals geraten oder stillschweigend
// übersprungen.
func IngestNativeSpace(ctx context.Context, repo output.Repository, src NativeRowSource, bv domain.BackboneVersion, minRank domain.Rank) (NativeSpaceIngestReport, error) {
	report := NativeSpaceIngestReport{Space: bv.ID}
	rows := src.Rows()
	report.Rows = len(rows)

	tx, err := repo.BeginIngest(ctx, bv)
	if err != nil {
		return report, fmt.Errorf("application: starting native space ingest for %q: %w", bv.ID, err)
	}

	unknownSeen := map[string]bool{}
	for _, row := range rows {
		rank, verbatim := domain.ParseRankLenient(row.Rank)
		if rank == domain.RankOther {
			report.UnknownRank++
			if !unknownSeen[verbatim] {
				unknownSeen[verbatim] = true
				report.UnknownRankSample = append(report.UnknownRankSample, verbatim)
			}
		}
		if !aboveSpecies(rank) {
			report.Skipped++
			continue
		}

		name := domain.Name{
			ID: bv.ID + ":name:" + row.SourceID,
			Canonical: row.Taxon, Rank: rank, RankVerbatim: verbatim,
		}
		concept := domain.Concept{
			ID: bv.ID + ":concept:" + row.SourceID,
			BackboneID: bv.ID, AcceptedName: name, Rank: rank, RankVerbatim: verbatim,
			Status: domain.StatusAccepted,
		}
		if row.ParentID != "" {
			concept.ParentID = bv.ID + ":concept:" + row.ParentID
		}
		if err := tx.UpsertName(name); err != nil {
			_ = tx.Rollback()
			return report, fmt.Errorf("application: writing native name %q: %w", row.Taxon, err)
		}
		if err := tx.UpsertConcept(concept); err != nil {
			_ = tx.Rollback()
			return report, fmt.Errorf("application: writing native concept %q: %w", row.Taxon, err)
		}
		if err := tx.LinkName(concept.ID, name.ID, "accepted", nil); err != nil {
			_ = tx.Rollback()
			return report, fmt.Errorf("application: linking native name %q: %w", row.Taxon, err)
		}
		report.Written++
	}

	if err := tx.Finalize(); err != nil {
		_ = tx.Rollback()
		return report, fmt.Errorf("application: finalizing native space ingest for %q: %w", bv.ID, err)
	}
	if err := tx.Commit(); err != nil {
		return report, fmt.Errorf("application: committing native space ingest for %q: %w", bv.ID, err)
	}
	return report, nil
}

// aboveSpecies reports whether rank sits strictly above SPECIES in the
// classification hierarchy — the boundary between Fall A (enrichment of an
// existing WCVP concept) and Fall B (a genuinely new concept).
func aboveSpecies(rank domain.Rank) bool {
	switch rank {
	case domain.RankSpeciesAggregate, domain.RankGenusAggregate,
		domain.RankSection, domain.RankSubsection, domain.RankSeries,
		domain.RankSubgenus, domain.RankGenus, domain.RankTribe,
		domain.RankSubfamily, domain.RankFamily, domain.RankSuperorder,
		domain.RankOrder, domain.RankSubclass, domain.RankClass,
		domain.RankInformalClade, domain.RankSubdivision, domain.RankPhylum,
		domain.RankRoot:
		return true
	default:
		return false
	}
}
```

- [ ] **Step 4: Test ausführen, Erfolg bestätigen**

Run: `go test ./internal/application/... -run TestIngestNativeSpace -v`
Erwartet: PASS

- [ ] **Step 5: DoD + Commit**

```bash
make lint && make mutation PKG=./internal/application
git add internal/application/nativespace_ingest.go internal/application/nativespace_ingest_test.go
git commit -m "feat(namespace): natives Konzept-Ingest für Fall-B-Ränge (EuroSL/GermanSL)"
```

---

## Task 6: Aggregat-Mitglieder-Relation schreiben (`concept_aggregate`)

**Files:**
- Modify: `internal/application/nativespace_ingest.go` (Erweiterung von `IngestNativeSpace`)
- Modify: `internal/ports/output/repository.go` (`IngestTx.AddAggregateMember`)
- Modify: `internal/adapters/sqlite/` (Implementierung)
- Test: `internal/application/nativespace_ingest_test.go`

**Consumes:** `Task 5`'s `IngestNativeSpace`, Fall-A-Ergebnis aus Task 4 (WCVP-Konzept-IDs für Mitglieder).

**Interfaces:**
- Produces: `IngestTx.AddAggregateMember(aggregateConceptID, memberConceptID string) error`;
  `IngestNativeSpace` bekommt einen zusätzlichen Parameter
  `memberResolver func(parentID string) []string` (löst die native
  `ParentID` einer Species-Zeile — die in Fall A NICHT geschrieben wurde,
  weil `aboveSpecies` sie überspringt — auf die WCVP-Konzept-IDs auf, die
  Task 4 bereits verknüpft hat).

- [ ] **Step 1: Failing Test — Mitglieder werden verknüpft**

```go
func TestIngestNativeSpace_LinksAggregateMembers(t *testing.T) {
	repo := seededNamespaceRepo(t) // WCVP-Konzept "Salsola kali" existiert schon (wie Task 4)
	// Fall A (Task 4) hat "Salsola kali" bereits mit source_id "sk1" verknüpft:
	seedNameSpaceEntry(t, repo, "eurosl", "sk1", wcvpConceptID(t, repo, "Salsola kali"))

	src := staticNativeRows{
		{Taxon: "Salsola kali aggr.", SourceID: "agg1", Rank: "Species Aggregate", Status: "accepted"},
	}
	bv := domain.BackboneVersion{ID: "eurosl", Version: "2026-08-27"}
	memberLinks := map[string][]string{"agg1": {"sk1"}} // aggregat-source-id -> [mitglied-source-ids]

	report, err := application.IngestNativeSpace(context.Background(), repo, src, bv, domain.RankSpeciesAggregate, memberLinks)
	if err != nil {
		t.Fatalf("IngestNativeSpace: unexpected error: %v", err)
	}
	if report.MembersLinked != 1 {
		t.Errorf("report.MembersLinked = %d, want 1", report.MembersLinked)
	}

	members, err := repo.AggregateMembers(context.Background(), "eurosl:concept:agg1")
	if err != nil {
		t.Fatalf("AggregateMembers: unexpected error: %v", err)
	}
	if len(members) != 1 || members[0] != wcvpConceptID(t, repo, "Salsola kali") {
		t.Errorf("AggregateMembers = %v, want [%s]", members, wcvpConceptID(t, repo, "Salsola kali"))
	}
}
```

- [ ] **Step 2: Test ausführen, Fehlschlag bestätigen**

Run: `go test ./internal/application/... -run TestIngestNativeSpace_LinksAggregateMembers -v`
Erwartet: FAIL (Signaturänderung fehlt, `AggregateMembers` fehlt)

- [ ] **Step 3: Implementieren**

`internal/ports/output/repository.go`, `Repository`-Interface ergänzen:

```go
	// AggregateMembers returns the WCVP concept ids that aggregateConceptID
	// (a Fall-B native concept, rank SPECIES_AGGREGATE/GENUS_AGGREGATE)
	// includes, via concept_aggregate.
	AggregateMembers(ctx context.Context, aggregateConceptID string) ([]string, error)
```

`IngestTx`-Interface ergänzen:

```go
	// AddAggregateMember records one aggregate->member edge (Task 2's
	// concept_aggregate table). Both ids must already be written.
	AddAggregateMember(aggregateConceptID, memberConceptID string) error
```

`nativespace_ingest.go`, Signatur erweitern und im Schreib-Loop nach dem
`report.Written++` ergänzen:

```go
func IngestNativeSpace(ctx context.Context, repo output.Repository, src NativeRowSource, bv domain.BackboneVersion, minRank domain.Rank, memberLinks map[string][]string) (NativeSpaceIngestReport, error) {
	// ... unverändert bis zum Schreib-Loop ...
	for _, row := range rows {
		// ... unverändert bis report.Written++ ...
		report.Written++

		if memberSourceIDs, ok := memberLinks[row.SourceID]; ok {
			for _, memberSourceID := range memberSourceIDs {
				memberConceptID, err := tx.ResolveNameSpaceMember(bv.ID, memberSourceID) // s.u.
				if err != nil {
					_ = tx.Rollback()
					return report, fmt.Errorf("application: resolving aggregate member %q for %q: %w", memberSourceID, row.Taxon, err)
				}
				if memberConceptID == "" {
					continue // Fall-A-Crosswalk hat diese Zeile nicht aufgelöst — kein Fehler, siehe NameSpaceIngestReport.Unmatched
				}
				if err := tx.AddAggregateMember(concept.ID, memberConceptID); err != nil {
					_ = tx.Rollback()
					return report, fmt.Errorf("application: linking aggregate member %q -> %q: %w", concept.ID, memberConceptID, err)
				}
				report.MembersLinked++
			}
		}
	}
	// ... Finalize/Commit unverändert ...
}
```

`IngestTx.ResolveNameSpaceMember(space, sourceID string) (string, error)`
ist eine weitere neue, kleine `IngestTx`-Methode (liest `name_space_entry`
nach `(space, ext_id)`, gibt `concept_id` zurück oder `""` wenn nicht
gefunden — **liest innerhalb derselben offenen Transaktion**, was zulässig
ist, weil es dieselbe `IngestTx` ist, nicht ein zweiter `Repository`-Aufruf
während eine fremde Transaktion offen ist).

`AggregateMembers` (Repository, nicht IngestTx) mit einem einfachen
`SELECT member_concept_id FROM concept_aggregate WHERE aggregate_concept_id = ?`
implementieren.

- [ ] **Step 4: Test ausführen, Erfolg bestätigen**

Run: `go test ./internal/application/... -v`
Erwartet: PASS

- [ ] **Step 5: DoD + Commit**

```bash
make lint && make mutation PKG=./internal/application
git add internal/application/nativespace_ingest.go internal/ports/output/repository.go internal/adapters/sqlite/
git commit -m "feat(namespace): Aggregat-Mitglieder-Relation (concept_aggregate)"
```

---

## Task 7: `concept_agreement` — Vorberechnung des Namensraum-Vergleichs

**Files:**
- Create: `internal/application/concept_agreement.go`
- Test: `internal/application/concept_agreement_test.go`

**Consumes:** `Repository.AggregateMembers` (Task 6), `domain.Canonicalize`/
`domain.StripAggregateMarkers` (bestehend).

**Interfaces:**
- Produces:
  ```go
  type Agreement string
  const (
  	AgreementIdentical Agreement = "identical"
  	AgreementSubset    Agreement = "subset"
  	AgreementSuperset  Agreement = "superset"
  	AgreementOverlap   Agreement = "overlap"
  	AgreementDisjoint  Agreement = "disjoint"
  	AgreementOneSided  Agreement = "one_sided"
  )
  func ComputeConceptAgreement(ctx context.Context, repo output.Repository) (ConceptAgreementReport, error)
  ```
  (Domain-Typ `Agreement` lebt in `internal/domain/agreement.go`, nicht in
  `application`, damit spätere HTTP-Rendering-Schichten ihn ohne
  Application-Import nutzen können — Hexagon-Grenze.)

- [ ] **Step 1: Failing Test — identische Mitgliederliste → `identical`**

```go
func TestComputeConceptAgreement_IdenticalMembersYieldsIdentical(t *testing.T) {
	repo := newInMemoryRepo(t)
	seedAggregateWithMembers(t, repo, "eurosl:concept:agg1", "Salsola kali aggr.", []string{"wcvp:concept:1"})
	seedAggregateWithMembers(t, repo, "germansl:concept:agg2", "Salsola kali s. l.", []string{"wcvp:concept:1"})

	report, err := application.ComputeConceptAgreement(context.Background(), repo)
	if err != nil {
		t.Fatalf("ComputeConceptAgreement: unexpected error: %v", err)
	}
	if len(report.Pairs) != 1 {
		t.Fatalf("report.Pairs = %d, want 1", len(report.Pairs))
	}
	if report.Pairs[0].Agreement != domain.AgreementIdentical {
		t.Errorf("Agreement = %q, want %q", report.Pairs[0].Agreement, domain.AgreementIdentical)
	}
}

func TestComputeConceptAgreement_DifferingMembersYieldsOverlap(t *testing.T) {
	repo := newInMemoryRepo(t)
	seedAggregateWithMembers(t, repo, "eurosl:concept:agg1", "Salsola kali aggr.", []string{"wcvp:concept:1"})
	seedAggregateWithMembers(t, repo, "germansl:concept:agg2", "Salsola kali s. l.", []string{"wcvp:concept:1", "wcvp:concept:2"})

	report, err := application.ComputeConceptAgreement(context.Background(), repo)
	if err != nil {
		t.Fatalf("ComputeConceptAgreement: unexpected error: %v", err)
	}
	if report.Pairs[0].Agreement != domain.AgreementSuperset {
		t.Errorf("Agreement = %q, want %q (germansl ist Obermenge von eurosl)", report.Pairs[0].Agreement, domain.AgreementSuperset)
	}
	if len(report.Pairs[0].OnlyInGermansl) != 1 || report.Pairs[0].OnlyInGermansl[0] != "wcvp:concept:2" {
		t.Errorf("OnlyInGermansl = %v, want [wcvp:concept:2]", report.Pairs[0].OnlyInGermansl)
	}
}
```

- [ ] **Step 2: Test ausführen, Fehlschlag bestätigen**

Run: `go test ./internal/application/... -run TestComputeConceptAgreement -v`
Erwartet: FAIL

- [ ] **Step 3: Implementieren**

`internal/domain/agreement.go` (neu):

```go
package domain

type Agreement string

const (
	AgreementIdentical Agreement = "identical"
	AgreementSubset    Agreement = "subset"
	AgreementSuperset  Agreement = "superset"
	AgreementOverlap   Agreement = "overlap"
	AgreementDisjoint  Agreement = "disjoint"
	AgreementOneSided  Agreement = "one_sided"
)

// CompareAggregateMembers computes the Agreement between two member sets
// (a-side, b-side), given as sorted, deduplicated concept-id slices. It is a
// pure set comparison — the caller resolves which side is "eurosl" and which
// is "germansl".
func CompareAggregateMembers(a, b []string) (agreement Agreement, onlyA, onlyB []string) {
	setA, setB := toSet(a), toSet(b)
	for id := range setA {
		if !setB[id] {
			onlyA = append(onlyA, id)
		}
	}
	for id := range setB {
		if !setA[id] {
			onlyB = append(onlyB, id)
		}
	}
	sort.Strings(onlyA)
	sort.Strings(onlyB)

	switch {
	case len(onlyA) == 0 && len(onlyB) == 0:
		return AgreementIdentical, onlyA, onlyB
	case len(onlyA) == 0:
		return AgreementSubset, onlyA, onlyB // a ⊆ b
	case len(onlyB) == 0:
		return AgreementSuperset, onlyA, onlyB // a ⊇ b
	case len(setA)+len(setB)-len(onlyA)-len(onlyB) == 0: // kein gemeinsames Element
		return AgreementDisjoint, onlyA, onlyB
	default:
		return AgreementOverlap, onlyA, onlyB
	}
}

func toSet(ids []string) map[string]bool {
	s := make(map[string]bool, len(ids))
	for _, id := range ids {
		s[id] = true
	}
	return s
}
```

`internal/application/concept_agreement.go` (neu): iteriert über alle
`eurosl:`-Aggregat-Konzepte (Rank `SPECIES_AGGREGATE`/`GENUS_AGGREGATE`),
findet per `domain.Canonicalize(domain.StripAggregateMarkers(name))`
Gleichnamigkeit gegen alle `germansl:`-Aggregat-Konzepte, ruft
`Repository.AggregateMembers` für beide Seiten, ruft
`domain.CompareAggregateMembers`, generiert `agreement_text` (String-Template
je `Agreement`-Wert, siehe Spec Abschnitt 5 Beispiel-Satz), schreibt via
einer neuen `IngestTx`-losen Repository-Methode
`Repository.WriteConceptAgreement(ctx, pairs []domain.ConceptAgreementPair) error`
(kein `IngestTx` nötig — läuft NACH allen Ingests, kein FK-Konflikt-Risiko,
einfacher `INSERT OR REPLACE`-Batch).

Ein Aggregat ohne Gegenstück im anderen Namensraum bekommt `AgreementOneSided`
mit `germansl_concept_id = NULL` bzw. `eurosl_concept_id = NULL` (Schema aus
Task 2 erlaubt das explizit).

- [ ] **Step 4: Test ausführen, Erfolg bestätigen**

Run: `go test ./internal/application/... -run TestComputeConceptAgreement -v`
Erwartet: PASS

- [ ] **Step 5: DoD + Commit**

```bash
make lint && make mutation PKG=./internal/application && make mutation PKG=./internal/domain
git add internal/domain/agreement.go internal/application/concept_agreement.go internal/application/concept_agreement_test.go
git commit -m "feat(agreement): Vorberechnung des EuroSL/GermanSL-Aggregat-Vergleichs"
```

---

## Task 8: Suchmodus `match_mode` in `/v1/suggest`

**Files:**
- Modify: `internal/adapters/sqlite/suggest.go` (`ftsPrefixToken`, `Suggest`)
- Modify: `internal/ports/output/repository.go` (`SuggestOpts`)
- Modify: `internal/adapters/http/suggest.go` (Query-Parameter parsen)
- Test: `internal/adapters/sqlite/suggest_matchmode_internal_test.go`, `internal/adapters/http/suggest_test.go`

**Interfaces:**
- Produces: `SuggestOpts.MatchMode string` (`"name_start"` Default wenn leer,
  `"anywhere"` explizit), HTTP-Query-Parameter `match_mode`.

- [ ] **Step 1: Failing Test — `name_start` filtert Epithet-Treffer heraus**

```go
func TestSuggest_NameStartExcludesEpithetMatch(t *testing.T) {
	db := ingestWCVPFixture(t) // enthält u.a. Kunzea capitata, Carex spec.
	ctx := context.Background()

	got, err := db.Suggest(ctx, "ca", output.SuggestOpts{Limit: 50, MatchMode: "name_start"})
	if err != nil {
		t.Fatalf("Suggest: unexpected error: %v", err)
	}
	for _, item := range got {
		if strings.HasPrefix(strings.ToLower(item.Canonical), "kunzea") {
			t.Errorf("Suggest(%q, name_start) returned %q, want no epithet-only match", "ca", item.Canonical)
		}
	}
}

func TestSuggest_AnywhereStillMatchesEpithet(t *testing.T) {
	db := ingestWCVPFixture(t)
	ctx := context.Background()

	got, err := db.Suggest(ctx, "capitata", output.SuggestOpts{Limit: 50, MatchMode: "anywhere"})
	if err != nil {
		t.Fatalf("Suggest: unexpected error: %v", err)
	}
	if len(conceptIDs(got)) == 0 {
		t.Error("Suggest(\"capitata\", anywhere) = empty, want Kunzea capitata via epithet match")
	}
}
```

- [ ] **Step 2: Test ausführen, Fehlschlag bestätigen**

Run: `go test ./internal/adapters/sqlite/... -run TestSuggest_NameStart -v`
Erwartet: FAIL (`name_start` filtert heute nicht, `MatchMode`-Feld fehlt)

- [ ] **Step 3: Implementieren**

`SuggestOpts` um `MatchMode string` ergänzen (Kommentar: `""`/`"name_start"`
= Default, `"anywhere"` = heutiges Verhalten).

`suggest.go`, `Suggest`-Funktion: nach dem bestehenden `matches`-CTE-Aufbau
(Zeile ~143), bei `opts.MatchMode != "anywhere"` einen zusätzlichen Filter
gegen `name.canonical_fold` ergänzen — der einfachste korrekte Ansatz ist
ein `AND n.canonical_fold LIKE ? || '%'`-Zusatzbedingung auf das GEFALTETE
kanonische Präfix, angewendet NACH dem bestehenden FTS5-Match (der weiterhin
die Kandidatenmenge liefert, aber `name_start` filtert sie zusätzlich):

```go
if opts.MatchMode != "anywhere" {
	// name_start (Default): nur Namen, deren VOLLER kanonisierter String mit
	// dem Query-Präfix beginnt, nicht nur irgendein FTS5-Token darin. Löst
	// den SP7-Befund ("ca" traf Kunzea capitata über das Epithet-Token).
	prefix := domain.StripAggregateMarkers(domain.Canonicalize(q))
	cteClause += ` AND EXISTS (
		SELECT 1 FROM name nm JOIN concept_name cn ON cn.name_id = nm.id
		WHERE cn.concept_id = tc.id AND nm.canonical_fold LIKE ? || '%'
	)`
	// args entsprechend um `prefix` erweitern, an der richtigen Stelle in
	// der bereits dokumentierten args-Reihenfolge (siehe Suggest's
	// Kommentar Zeile 122-127) einfügen.
}
```

(Der Implementierer muss die exakte CTE-Struktur aus `suggest.go:143-233`
lesen, um die Platzierung korrekt zu treffen — dieser Schritt beschreibt
die Absicht, nicht die letzte Zeile SQL; die Platzierung muss gegen die
bestehende `args`-Reihenfolge-Dokumentation im Code verifiziert werden.)

`internal/adapters/http/suggest.go`, Query-Parameter-Parsing um
`match_mode` ergänzen, validiert gegen `{"", "name_start", "anywhere"}`,
sonst `400 INVALID_QUERY`.

- [ ] **Step 4: Test ausführen, Erfolg bestätigen**

Run: `go test ./internal/adapters/sqlite/... ./internal/adapters/http/... -v`
Erwartet: PASS

- [ ] **Step 5: DoD + Commit**

```bash
make lint && make mutation PKG=./internal/adapters/sqlite
git add internal/adapters/sqlite/suggest.go internal/ports/output/repository.go internal/adapters/http/suggest.go
git commit -m "feat(suggest): match_mode Parameter (name_start Default, anywhere optional)"
```

---

## Task 9: `GET /v1/concept/{id}` — Klassifikation, Aggregat-Felder, Vernakularnamen

**Files:**
- Modify: `internal/adapters/http/taxa.go` (`handleConcept`, DTO)
- Test: `internal/adapters/http/taxa_test.go`

**Consumes:** `domain.Concept.Family/OrderName/ClassName` (Task 4),
`Repository.AggregateMembers` (Task 6), `Repository.NameSpaceEntries`
(bestehend, für `aggregate_memberships`-Rückverweis).

- [ ] **Step 1: Failing Test — Klassifikation + Aggregat-Mitglieder im Response**

```go
func TestHandleConcept_IncludesClassificationAndAggregateMembers(t *testing.T) {
	repo := seededFullRepo(t) // Salsola kali (WCVP) + Salsola kali aggr. (eurosl) verlinkt
	req := httptest.NewRequest(http.MethodGet, "/v1/concept/eurosl:concept:agg1", nil)
	req = mux.SetURLVars(req, map[string]string{"id": "eurosl:concept:agg1"})
	w := httptest.NewRecorder()

	handleConcept(repo)(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body: %s", w.Code, w.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	members, ok := body["members"].([]any)
	if !ok || len(members) != 1 {
		t.Fatalf("body[\"members\"] = %v, want 1 entry", body["members"])
	}
}
```

- [ ] **Step 2: Test ausführen, Fehlschlag bestätigen**

Run: `go test ./internal/adapters/http/... -run TestHandleConcept_IncludesClassification -v`
Erwartet: FAIL

- [ ] **Step 3: DTO + Handler erweitern**

`taxa.go`s Concept-Response-DTO um `classification`, `members` (nur wenn
`concept.Rank` ein Sammel-Rang ist, siehe `aboveSpeciesAggregateRank`-Helper
analog Task 5), `aggregate_memberships` (nur wenn `concept.Rank == SPECIES`
und `NameSpaceEntries` mind. einen `Aggregate`-Eintrag liefert),
`vernacular_names` erweitern. Feldnamen exakt wie Spec Abschnitt 4/5.

- [ ] **Step 4: Test ausführen, Erfolg bestätigen**

Run: `go test ./internal/adapters/http/... -v`
Erwartet: PASS

- [ ] **Step 5: OpenAPI + DoD + Commit**

`internal/adapters/http/openapi.yaml` um die neuen Felder ergänzen (Contract-Test
`openapi_contract_test.go` hält Route+Methode ab, Schema-Test `openapi_schema_test.go`
das Response-Schema — beide müssen grün bleiben).

```bash
make lint && go test ./internal/adapters/http/...
git add internal/adapters/http/taxa.go internal/adapters/http/taxa_test.go internal/adapters/http/openapi.yaml
git commit -m "feat(concept): Klassifikation, Aggregat-Mitglieder/-schaften im Response"
```

---

## Task 10: `POST /v1/match` — Klassifikation immer, `aggregate_resolution` als Pflichtfeld

**Files:**
- Modify: `internal/application/match.go` (`MatchResult`, `MatchInSpace`/`MatchNames`)
- Modify: `internal/adapters/http/taxa.go` (`handleMatch`, DTO)
- Test: `internal/application/match_test.go`, `internal/adapters/http/taxa_test.go`

**Consumes:** Task 7's `domain.CompareAggregateMembers`/`Agreement`,
Task 6's `Repository.AggregateMembers`, Task 5's `aboveSpecies`.

**Interfaces:**
- Produces: `MatchResult` erweitert um `Classification domain.Classification`
  (neuer kleiner Struct `{Family, OrderName, ClassName string}` in
  `internal/domain/taxon.go`) und `AggregateResolution *domain.AggregateResolution`
  (`nil` wenn kein Sammel-Rang betroffen — Pointer, damit JSON-Absenz
  eindeutig von einem leeren Objekt unterscheidbar ist).

- [ ] **Step 1: Failing Test — `classification` immer, `aggregate_resolution` bei Aggregat-Treffer**

```go
func TestMatchNames_AlwaysIncludesClassification(t *testing.T) {
	repo := seededMatchRepo(t)
	conceptID := seedSileneOtites(t, repo) // mit Family "Caryophyllaceae" geseedet
	results, err := application.MatchNames(context.Background(), repo, []application.MatchRequest{
		{ID: "1", Verbatim: "Silene otites"},
	})
	if err != nil {
		t.Fatalf("MatchNames: unexpected error: %v", err)
	}
	if results[0].Classification.Family != "Caryophyllaceae" {
		t.Errorf("Classification.Family = %q, want %q", results[0].Classification.Family, "Caryophyllaceae")
	}
}

func TestMatchNames_AggregateHitCarriesResolutionAcrossSpaces(t *testing.T) {
	repo := seededAggregateRepo(t) // "Salsola kali aggr." in eurosl UND germansl bekannt, agreement=identical vorberechnet
	results, err := application.MatchNames(context.Background(), repo, []application.MatchRequest{
		{ID: "1", Verbatim: "Salsola kali agg."},
	})
	if err != nil {
		t.Fatalf("MatchNames: unexpected error: %v", err)
	}
	res := results[0].AggregateResolution
	if res == nil {
		t.Fatal("AggregateResolution = nil, want non-nil for an aggregate match")
	}
	if len(res.Options) != 3 {
		t.Fatalf("len(Options) = %d, want 3 (eurosl, germansl, wcvp)", len(res.Options))
	}
	if res.Agreement != domain.AgreementIdentical {
		t.Errorf("Agreement = %q, want %q", res.Agreement, domain.AgreementIdentical)
	}
}
```

- [ ] **Step 2: Test ausführen, Fehlschlag bestätigen**

Run: `go test ./internal/application/... -run "TestMatchNames_AlwaysIncludesClassification|TestMatchNames_AggregateHitCarriesResolution" -v`
Erwartet: FAIL

- [ ] **Step 3: Implementieren**

`internal/domain/taxon.go`, neue kleine Typen:

```go
type Classification struct {
	Family    string
	OrderName string
	ClassName string
}

type AggregateResolutionOption struct {
	NameSpace          string
	Status             AggregatePolicy // "known" | "unresolvable" | "" (absent)
	AggregateConceptID string
	MemberCount        int
}

type AggregateResolution struct {
	RequestedNameSpace string
	Status             AggregatePolicy
	MemberCount        int
	Options            []AggregateResolutionOption
	Agreement          Agreement // "" wenn nur 0/1 Namensraum das Aggregat kennt
}
```

`match.go`, `MatchResult` um `Classification Classification` und
`AggregateResolution *AggregateResolution` ergänzen. In `matchOne`
(bzw. der Stelle, die `ConceptID` final setzt) nach erfolgreicher
Konzept-Ermittlung IMMER `repo`-Klassifikation nachladen (kleiner
zusätzlicher Repository-Call, `Repository.Concept` liefert `domain.Concept`
bereits inkl. `Family`/`OrderName`/`ClassName` aus Task 4 — einfach die
Felder übernehmen, kein neuer Repository-Endpunkt nötig).

Wenn `domain.IsAggregateName(req.Verbatim)` **oder** das aufgelöste Konzept
selbst einen Sammel-Rang trägt (`aboveSpecies`-artiger Check aus Task 5, auf
die konkreten Sammel-Ränge `SPECIES_AGGREGATE/GENUS_AGGREGATE/SECTION/
SUBSECTION/SUBGENUS` beschränkt, siehe Spec Abschnitt 6): für jeden
ingestierten Namensraum (`eurosl`, `germansl`, `wcvp` — Liste über
`Repository.NameSpaces(ctx)`, falls diese Methode noch nicht existiert, als
Teil dieses Tasks ergänzen) `AggregateMembers` abfragen, `AggregateResolutionOption`
aufbauen; wenn zwei Namensräume beide `known` sind, `Repository.ConceptAgreement`
(neue, kleine Methode, liest `concept_agreement` nach Konzept-ID) für das
`Agreement`-Feld abfragen.

`internal/adapters/http/taxa.go`, `handleMatch`-DTO um `classification` und
`aggregate_resolution` (nur wenn `!= nil`) erweitern.

- [ ] **Step 4: Test ausführen, Erfolg bestätigen**

Run: `go test ./internal/application/... ./internal/adapters/http/... -v`
Erwartet: PASS

- [ ] **Step 5: DoD + Commit**

```bash
make lint && make mutation PKG=./internal/application
git add internal/domain/taxon.go internal/application/match.go internal/adapters/http/taxa.go internal/adapters/http/openapi.yaml
git commit -m "feat(match): classification immer, aggregate_resolution als Pflichtfeld bei Sammel-Raengen

BREAKING: /v1/match ist nicht mehr byte-identisch ohne target_space —
jedes Ergebnis traegt jetzt classification. Spec Abschnitt 6, bewusste
Entscheidung."
```

---

## Task 11: `POST /v1/translate` — `target_space` vereinheitlichen

**Files:**
- Modify: `internal/application/translate.go`
- Modify: `internal/adapters/http/translate.go`
- Test: `internal/application/translate_test.go`

**Consumes:** `Repository.NameSpaces` (Task 10), bestehende `sec_reference`-Lookup-Logik.

- [ ] **Step 1: Failing Test — `target_space=germansl` funktioniert wie eine sec.-UUID**

```go
func TestTranslate_AcceptsNameSpaceAsTargetSpace(t *testing.T) {
	repo := seededTranslateRepo(t) // WCVP-Konzept mit germansl name_space_entry
	req := application.TranslateRequest{
		ConceptID: wcvpConceptID(t, repo, "Salsola kali"),
		TargetSec: "germansl",
	}
	result, err := application.Translate(context.Background(), repo, req)
	if err != nil {
		t.Fatalf("Translate: unexpected error: %v", err)
	}
	if result.Entry.Mode != "concept_id" {
		t.Errorf("Entry.Mode = %q, want %q", result.Entry.Mode, "concept_id")
	}
	// Keine CDM-Relation erwartet (das ist ein Namensraum, keine
	// concept_relation-Kante) — stattdessen die name_space_entry-Spellings.
}
```

- [ ] **Step 2: Test ausführen, Fehlschlag bestätigen**

Run: `go test ./internal/application/... -run TestTranslate_AcceptsNameSpaceAsTargetSpace -v`
Erwartet: FAIL (heute nur CDM-`sec_reference`-UUIDs akzeptiert)

- [ ] **Step 3: Implementieren**

In `Translate` (`translate.go`), vor der bestehenden `sec_reference`-Auflösung
prüfen, ob `req.TargetSec` einer der ingestierten `name_space`-IDs entspricht
(`Repository.NameSpaces(ctx)`, Task 10). Falls ja: die
`domain.ResolveTargetSpace`-Logik (bereits vorhanden, `namespace.go:142`)
verwenden, um Spelling + `AggregatePolicy` zu ermitteln, und das Ergebnis in
`TranslateResult` unter einem Zweig rendern, der klar von der
`concept_relation`-Form unterschieden ist (kein `is_equality`/`relation_from_source`
— das sind CDM-spezifische Konzepte, die für einen Namensraum-Vergleich keinen
Sinn ergeben). Falls `req.TargetSec` weder Namensraum noch bekannte
`sec_reference`-ID ist: bestehendes Fehlerverhalten (`400`, Raum benannt)
unverändert.

- [ ] **Step 4: Test ausführen, Erfolg bestätigen**

Run: `go test ./internal/application/... -v`
Erwartet: PASS

- [ ] **Step 5: DoD + Commit**

```bash
make lint && make mutation PKG=./internal/application
git add internal/application/translate.go internal/adapters/http/translate.go internal/adapters/http/openapi.yaml
git commit -m "feat(translate): target_space akzeptiert Namensräume (eurosl/germansl/wcvp) neben CDM sec.-UUIDs"
```

---

## Task 12: Traits-Subsystem entfernen

**Files:**
- Delete: `internal/application/traits_ingest.go`, `internal/application/traits_ingest_test.go` (und alle `traits_*_test.go`-Geschwister)
- Delete: `internal/adapters/http/traits.go`, `internal/adapters/http/traits_test.go`
- Modify: `internal/adapters/sqlite/schema.sql` (Tabellen `trait_value`/`trait_vocabulary` entfernen — als NEUE Datei-Version, nicht rückwirkend migrieren, siehe Hinweis unten)
- Modify: `internal/adapters/http/router.go` (Route-Registrierung entfernen)
- Modify: `internal/ports/output/repository.go` (`AddTraitValue`/`UpsertTraitVocabulary` aus `IngestTx` entfernen — ACHTUNG: `BeginTraitIngest` wird von `IngestNameSpace`/`IngestNativeSpace` NICHT mehr exklusiv für Traits gebraucht, aber der Name bleibt sinnvoll für "kein Backbone"-Ingests; NICHT entfernen, nur die zwei Trait-spezifischen Methoden)
- Delete: `pipelines/{eive,tichy,midolo}/` (nach Transfer-Kopie für situs, siehe Hinweis)
- Test: `internal/adapters/http/router_test.go`, `internal/adapters/http/openapi_contract_test.go`

**Wichtiger Hinweis vor dem Löschen:** `pipelines/{eive,tichy,midolo}/` NICHT
einfach löschen, sondern per `git mv` oder Kopie zuerst in ein Transfer-Verzeichnis
außerhalb des hostus-Repos sichern (z.B. `/tmp/hostus-traits-transfer/` oder
direkt als erster Commit im situs-Repo, falls Teilprojekt 2 zu diesem
Zeitpunkt schon existiert) — Spec Abschnitt 8 verlangt Transfer, kein
Datenverlust.

- [ ] **Step 1: Failing Test — `/v1/concept/{id}/traits` ist nicht mehr registriert**

```go
func TestRouter_TraitsRouteRemoved(t *testing.T) {
	router := newTestRouter(t) // bestehender Helper in router_test.go
	req := httptest.NewRequest(http.MethodGet, "/v1/concept/wcvp:concept:1/traits", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404 (Route soll entfernt sein)", w.Code)
	}
}
```

- [ ] **Step 2: Test ausführen, Fehlschlag bestätigen**

Run: `go test ./internal/adapters/http/... -run TestRouter_TraitsRouteRemoved -v`
Erwartet: FAIL (Route liefert heute 200/andere Response)

- [ ] **Step 3: Löschen + Referenzen entfernen**

```bash
mkdir -p /tmp/hostus-traits-transfer
cp -r pipelines/{eive,tichy,midolo} /tmp/hostus-traits-transfer/
git rm -r pipelines/eive pipelines/tichy pipelines/midolo
git rm internal/application/traits_ingest.go internal/adapters/http/traits.go
git rm internal/application/traits_ingest_test.go internal/adapters/http/traits_test.go
# weitere traits_*_test.go Geschwister-Dateien analog (per find internal -iname "*traits*_test.go" identifizieren)
```

`router.go`: Zeile mit `r.HandleFunc("/v1/concept/{id}/traits", ...)` entfernen.
`schema.sql`: `trait_value`/`trait_vocabulary`-`CREATE TABLE`-Blöcke entfernen
(bestehende Datenbanken behalten die Tabellen als Altlast — kein
Migrations-`DROP TABLE` nötig, da lokale PoC-Datenbanken ohnehin neu
ingestiert werden; siehe Spec Abschnitt 10, Lizenz-/PoC-Haltung).
`repository.go`: `AddTraitValue`/`UpsertTraitVocabulary` aus `IngestTx` entfernen,
zugehörige DTOs (`domain.TraitValue`, `domain.TraitVocabMeta`) NUR entfernen,
wenn kein anderer Konsument mehr existiert (per `grep -rn "domain.TraitValue\|domain.TraitVocabMeta"` prüfen).
`openapi.yaml`: `/v1/concept/{id}/traits`-Pfad entfernen.

- [ ] **Step 4: Test ausführen, Erfolg bestätigen; volle Suite grün**

Run: `go build ./... && go test ./... 2>&1 | tail -50`
Erwartet: Build grün, keine Kompilierfehler durch tote Trait-Referenzen,
`TestRouter_TraitsRouteRemoved` PASS.

- [ ] **Step 5: CHANGELOG + DoD + Commit**

`CHANGELOG.md`, `[Unreleased]` → `### Removed`:
```markdown
- **Traits-Subsystem entfernt** (`GET /v1/concept/{id}/traits`, EIVE/Tichý/
  Midolo-Pipelines). BREAKING. Transfer nach situs (Teilprojekt 2), siehe
  docs/superpowers/specs/2026-08-27-hostus-namensraum-redesign-design.md
  Abschnitt 8.
```

```bash
make verify
git add -A
git commit -m "feat(traits)!: Traits-Subsystem entfernt, Transfer nach situs

BREAKING CHANGE: GET /v1/concept/{id}/traits entfernt. EIVE/Tichý/Midolo-
Pipelines nach situs transferiert (Teilprojekt 2)."
```

---

## Task 13: Fachliche Korrektheits-Tests

**Files:**
- Create: `internal/domain/rank_golden_test.go`
- Create: `internal/application/crosswalk_regression_test.go`
- Create: `docs/research/2026-08-27-drift-check.md` (gemessene Baseline-Kennzahlen)
- Test: alle oben genannten sind selbst die Tests

**Consumes:** alles aus Task 1–7.

- [ ] **Step 1: Rang-Golden-Liste (Korrektheits-Test 1, Spec Abschnitt 11)**

```go
// internal/domain/rank_golden_test.go
// Golden-Liste JEDES gemessenen Rohwerts aus EuroSL (29) und GermanSL (27
// Kuerzel) — siehe docs/superpowers/specs/2026-08-27-hostus-namensraum-
// redesign-design.md Abschnitt 2. Ein neuer, hier nicht gelisteter Rohwert
// muss ParseRank/ParseRankLenient auffallen (Fehler bzw. RankOther), nie
// automatisch geraten werden.
func TestParseRankLenient_GoldenVocabulary(t *testing.T) {
	golden := map[string]domain.Rank{
		"Root": domain.RankRoot, "Phylum": domain.RankPhylum,
		"Suprageneric Taxon": domain.RankOther, // absichtlich NICHT gemappt (siehe Spec)
		"Subdivision": domain.RankSubdivision, "Division": domain.RankPhylum,
		"Class": domain.RankClass, "Subclass": domain.RankSubclass,
		"Superorder": domain.RankSuperorder, "Order": domain.RankOrder,
		"Family": domain.RankFamily, "Genus": domain.RankGenus,
		"Tribe": domain.RankTribe, "Subfamily": domain.RankSubfamily,
		"Species": domain.RankSpecies, "Section": domain.RankSection,
		"Unranked (infrageneric)": domain.RankUnrankedInfrageneric,
		"Subgenus": domain.RankSubgenus, "Subspecies": domain.RankSubspecies,
		"Species Aggregate": domain.RankSpeciesAggregate,
		"Subsection bot.": domain.RankSubsection, "Variety": domain.RankVariety,
		"Form": domain.RankForm, "Unranked (infraspecific)": domain.RankUnrankedInfraspecific,
		"Subvariety": domain.RankSubvariety, "Coll. species": domain.RankCollSpecies,
		"Proles": domain.RankProles, "Race": domain.RankRace,
		"Subform": domain.RankSubform, "Grex (infraspec.)": domain.RankGrex,
		"Convar": domain.RankConvar,
		// GermanSL-Kuerzel:
		"SPE": domain.RankSpecies, "SSP": domain.RankSubspecies, "GAT": domain.RankGenus,
		"VAR": domain.RankVariety, "FAM": domain.RankFamily, "AGG": domain.RankSpeciesAggregate,
		"ORD": domain.RankOrder, "FOR": domain.RankForm, "SEC": domain.RankSection,
		"KLA": domain.RankClass, "SER": domain.RankSeries, "ORA": domain.RankUnrankedInfraspecific,
		"ABT": domain.RankPhylum, "SGE": domain.RankSubgenus, "SGR": domain.RankSubspeciesGroup,
		"SFA": domain.RankSubfamily, "UAB": domain.RankOther, // bewusst ungemappt, siehe Spec
		"SSE": domain.RankSubsection, "CL1": domain.RankInformalClade,
		"AG1": domain.RankSpeciesAggregate, "AG2": domain.RankGenusAggregate,
		"AG3": domain.RankOther, // bewusst ungemappt, siehe Spec (Domaenen-Knoten, kein Rang)
	}
	for raw, want := range golden {
		got, _ := domain.ParseRankLenient(raw)
		if got != want {
			t.Errorf("ParseRankLenient(%q) = %q, want %q", raw, got, want)
		}
	}
}

func TestParseRankLenient_TrulyUnknownValueFallsBackToOther(t *testing.T) {
	got, verbatim := domain.ParseRankLenient("ein-nie-gesehener-rang")
	if got != domain.RankOther {
		t.Errorf("ParseRankLenient(unbekannt) = %q, want %q", got, domain.RankOther)
	}
	if verbatim != "ein-nie-gesehener-rang" {
		t.Errorf("verbatim = %q, want %q", verbatim, "ein-nie-gesehener-rang")
	}
}
```

- [ ] **Step 2: Crosswalk-Regression auf echten Fällen (Korrektheits-Test 2)**

```go
// internal/application/crosswalk_regression_test.go
func TestCrosswalk_InulaHirtaResolvesToPentanemaViaTier2(t *testing.T) {
	repo := seededHomonymRepo(t) // Inula hirta homotypisches Synonym von Pentanema hirtum,
	                              // heterotypisch von Pentanema britannicum, in keinem Konzept accepted
	results, err := application.MatchNames(context.Background(), repo, []application.MatchRequest{
		{ID: "1", Verbatim: "Inula hirta"},
	})
	if err != nil {
		t.Fatalf("MatchNames: unexpected error: %v", err)
	}
	if results[0].RequiresReview {
		t.Error("RequiresReview = true, want false (Tier-2-Homonym-Aufloesung ist eindeutig)")
	}
	if results[0].ConceptID != pentanemaHirtumConceptID(t, repo) {
		t.Errorf("ConceptID = %q, want Pentanema hirtum's concept", results[0].ConceptID)
	}
}

func TestCrosswalk_SalsolaKaliAggregateYieldsOverlapAgreement(t *testing.T) {
	repo := seededSalsolaKaliRepo(t) // eurosl: 1 Mitglied, germansl: 2 Mitglieder (S. tragus subsp. tragus zusaetzlich)
	results, err := application.MatchNames(context.Background(), repo, []application.MatchRequest{
		{ID: "1", Verbatim: "Salsola kali agg."},
	})
	if err != nil {
		t.Fatalf("MatchNames: unexpected error: %v", err)
	}
	if results[0].AggregateResolution.Agreement != domain.AgreementSuperset {
		t.Errorf("Agreement = %q, want %q (germansl fuehrt eine zusaetzliche Sippe)", results[0].AggregateResolution.Agreement, domain.AgreementSuperset)
	}
}

func TestCrosswalk_RubusSectRubusIsOneSided(t *testing.T) {
	repo := seededRubusSectionRepo(t) // nur germansl kennt "Rubus sect. Rubus", eurosl fuehrt Rubus flach
	results, err := application.MatchNames(context.Background(), repo, []application.MatchRequest{
		{ID: "1", Verbatim: "Rubus sect. Rubus"},
	})
	if err != nil {
		t.Fatalf("MatchNames: unexpected error: %v", err)
	}
	if results[0].AggregateResolution.Agreement != domain.AgreementOneSided {
		t.Errorf("Agreement = %q, want %q", results[0].AggregateResolution.Agreement, domain.AgreementOneSided)
	}
}
```

- [ ] **Step 3: Kein-Fabrikat-Stichprobe (Korrektheits-Test 3)**

```go
func TestClassification_EveryValueTracesToASourceRow(t *testing.T) {
	// Stichprobe: fuer jedes Konzept mit gesetzter Family/OrderName/ClassName
	// muss eine name_space_entry ODER ein Fall-B-Konzept existieren, das die
	// Zeile als Quelle nennt — kein Klassifikations-Wert ohne Herkunftszeile.
	repo := seededFullRepo(t)
	concepts := allConceptsWithClassification(t, repo) // Test-Helper: SELECT id FROM taxon_concept WHERE family IS NOT NULL
	for _, id := range concepts {
		if !hasProvenance(t, repo, id) { // Test-Helper: prueft name_space_entry ODER backbone_id in {eurosl,germansl}
			t.Errorf("concept %q has classification but no traceable source_id", id)
		}
	}
}
```

- [ ] **Step 4: Alle Tests ausführen, Erfolg bestätigen**

Run: `go test ./internal/domain/... ./internal/application/... -v`
Erwartet: PASS

- [ ] **Step 5: Drift-Check-Dokument + DoD + Commit**

`docs/research/2026-08-27-drift-check.md`:

```markdown
# Gemessene Baseline-Kennzahlen (Drift-Check)

Diese Zahlen sind gegen den Datenstand vom 2026-08-27 gemessen. Eine
Abweichung nach einem Quellen-Update ist ein Fund, kein automatischer Fehler
— aber sie muss auffallen (siehe Spec Abschnitt 11, Korrektheits-Test 4).

| Kennzahl | Wert | Quelle |
|---|---|---|
| EuroSL-Zeilen gesamt | 139.039 | pipelines/eurosl/.cache/EuroSL.sqlite |
| EuroSL Species Aggregate | 287 | SELECT COUNT(*) WHERE TaxonRank='Species Aggregate' |
| GermanSL-Zeilen gesamt | 26.129 | pipelines/germansl/.cache/GermanSL1.5.5.xlsx (VOR Neu-Pinnen — nach Task 3 aktualisieren) |
| GermanSL AGG (Species-Aggregat) | 581 | siehe Task-3-Messung |
```

(Werte NACH Task 3's Neu-Pinnen mit den dann aktuellen Zahlen überschreiben
— dieser Task schreibt die Struktur/Methodik fest, nicht notwendigerweise
die finalen Zahlen, falls Task 3 vorher läuft und andere Zahlen misst.)

```bash
make verify
git add internal/domain/rank_golden_test.go internal/application/crosswalk_regression_test.go docs/research/2026-08-27-drift-check.md
git commit -m "test: fachliche Korrektheits-Tests (Rang-Golden-Liste, Crosswalk-Regression, Drift-Check)"
```

---

## Self-Review Notes

- **Task-Reihenfolge folgt Spec Abschnitt 12** (Rang → Schema → Pipelines →
  Fall A → Fall B → Aggregat-Relation → Agreement → API-Schicht → Traits-
  Entfernung → Tests), mit einer Korrektur: Traits-Entfernung (Task 12) ist
  unabhängig und könnte auch früher laufen — hier bewusst spät platziert,
  damit die API-Tasks (9-11) nicht durch parallele Löschungen gestört werden.
- **Spec-Korrektur dokumentiert:** `policyRefuseAmbiguity` statt
  `genuineBearerWinner` für Fall A (siehe Kopf dieses Plans) — das ist eine
  Faktenkorrektur aus dem bestehenden Code, keine neue Design-Entscheidung.
- **Platzhalter-Scan:** Jeder Task hat konkreten Code in den Steps, keine
  "TBD"/"add appropriate handling". Zwei Stellen bleiben bewusst als
  Implementierungs-Hinweis statt exaktem Code (Task 8 Step 3 CTE-Platzierung,
  Task 12 traits_*_test.go-Geschwister) — beide sind so benannt, weil sie von
  der exakten, erst beim Lesen der Datei sichtbaren Struktur abhängen, nicht
  weil Inhalt fehlt; beide Stellen sagen dem Bearbeiter genau, was zu prüfen
  und wonach zu suchen ist.
- **Typkonsistenz geprüft:** `domain.Agreement` (Task 7) wird in Task 10
  (`AggregateResolution.Agreement`) und Task 13 (Testfälle) identisch
  verwendet. `NativeRowSource`/`IngestNativeSpace` (Task 5) Signatur wird in
  Task 6 um `memberLinks` erweitert — Task 6 zitiert die volle neue Signatur,
  nicht nur den Diff, damit ein Bearbeiter, der nur Task 6 liest, sie
  vollständig sieht.
- **Spec-Abdeckung:** Abschnitt 1 (Datenquellen) → Task 3/4/5.
  Abschnitt 2 (Rang-Set) → Task 1/13. Abschnitt 3 (Suchmodus) → Task 8.
  Abschnitt 4 (Concept-API) → Task 9. Abschnitt 5 (Aggregat-Modell) →
  Task 6/7. Abschnitt 6 (Match) → Task 10. Abschnitt 7 (Crosswalk) →
  Task 4 (mit der dokumentierten Korrektur). Abschnitt 8 (Traits) →
  Task 12. Abschnitt 9 (Translate) → Task 11. Abschnitt 10 (Lizenz) →
  kein Code-Task nötig (Haltung, keine Implementierung). Abschnitt 11
  (Tests) → Task 13. Abschnitt 12 (Reihenfolge) → Task-Anordnung selbst.
  Abschnitt 13 (Offene Punkte) → absichtlich nicht in Tasks übersetzt,
  da sie auf Teilprojekt 2-4 bzw. eine externe Klärung verweisen.

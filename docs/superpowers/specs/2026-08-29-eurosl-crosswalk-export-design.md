# EuroSL-Crosswalk-Export: Design-Spec

Stand: 2026-08-29.

**Ziel:** hostus bekommt einen neuen CLI-Befehl `hostus export-crosswalk`,
der zwei kanonische CSVs schreibt, die situs' Species-Ingest als
Voraussetzung braucht (siehe situs-Spec
`docs/superpowers/specs/2026-08-29-situs-aggregat-mitgliedsarten-design.md`,
situs-Repo): eine deterministische Name→Concept-ID-Übersetzungstabelle für
eurosl-kompatible Namen, und die Aggregat-Mitgliederlisten aus dem
Namensraum-/Klassifikations-/Aggregat-Redesign (`concept_aggregate`).

**Anlass.** situs' Referenzdaten (`species_roles.csv`, EuroVeg/EUNIS-
Herkunft) führen bereits eurosl-kontrollierte Namen — situs braucht dafür
keine hostus-Matching-Intelligenz (`/v1/match`), nur eine 1:1-Nachschlage-
Tabelle. situs will diesen Ingest-Schritt außerdem bewusst OHNE Live-
Abhängigkeit von einer laufenden hostus-Instanz bauen (beide Dienste sind
noch nicht durchgängig deployed) — ein einmaliger Datei-Export/Kopiervorgang
statt eines Netzwerkaufrufs.

**Kein Redistributions-Gate.** Anders als `hostus bundle` (das für
`redistribution: unknown`-Quellen wie EuroSL standardmäßig verweigert und
`--force-include-restricted` verlangt) bekommt dieser Export KEIN Gate: es
ist ein lokaler Pipeline-Handoff zwischen zwei vom selben Betreiber
lokal ausgeführten Diensten (PoC-Status, Spec Abschnitt 10 des
Namensraum-Redesigns), keine Weitergabe an Dritte im lizenzrechtlichen
Sinne. Owner-Entscheidung, 2026-08-29.

## 1. CLI

```
hostus export-crosswalk --db hostus.sqlite --out-dir ./out/
  → out/eurosl_crosswalk.csv    (name|concept_id)
  → out/aggregate_members.csv   (aggregate_concept_id|member_concept_id|member_name)
```

Neuer Befehl, nicht Teil von `bundle` — semantisch anders (flache
CSV-Paare statt eines SQLite-Bundles), immer zusammen gebraucht (situs'
Ingest braucht beide Dateien gemeinsam), daher ein Aufruf, zwei Dateien.

```go
// cmd/hostus/export_crosswalk.go — analog zu bundle.go
func newExportCrosswalkCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "export-crosswalk",
		Short: "Export eurosl_crosswalk.csv + aggregate_members.csv for situs' ingest",
		RunE:  runExportCrosswalk,
	}
	cmd.Flags().String("db", "", "path to the source SQLite database")
	cmd.Flags().String("out-dir", "", "output directory for both CSVs")
	return cmd
}
```

## 2. Architektur

`internal/app/export_crosswalk.go` (Composition Root, analog zu `bundle.go`):

```go
type ExportCrosswalkReport struct {
	CrosswalkRows int
	MemberRows    int
	// NameCollisions counts names present in BOTH the Fall-A
	// (name_space_entry) and Fall-B (native eurosl concept) source — this
	// should structurally never happen (Fall A never targets an already-
	// native eurosl name), but is measured, not assumed. A collision is
	// reported with both concept ids, never silently resolved to one side.
	NameCollisions []CrosswalkCollision
}

func ExportCrosswalk(ctx context.Context, repo *sqlite.DB, outDir string) (ExportCrosswalkReport, error)
```

**`eurosl_crosswalk.csv` (name|concept_id):** `UNION` aus zwei Quellen:

1. **Fall A** — `name_space_entry` gefiltert auf `space='eurosl'`:
   `SELECT name, concept_id FROM name_space_entry WHERE space = 'eurosl'`
   (WCVP-Konzepte, per Namensraum-Crosswalk erreicht — Task 4 des
   Namensraum-Redesigns).
2. **Fall B** — native eurosl-Konzepte, ihr EIGENER akzeptierter Name:
   `SELECT n.canonical, tc.id FROM taxon_concept tc JOIN name n ON n.id = tc.accepted_name WHERE tc.backbone_id = 'eurosl'`
   (Aggregate, Sektionen, Familien, ... — Task 5 des Namensraum-Redesigns).

Eine Namens-Kollision zwischen beiden Quellen wird gezählt und mit beiden
Concept-IDs gemeldet (`NameCollisions`), NIE stillschweigend eine Seite
bevorzugt — situs' eigener Ingest behandelt eine mehrdeutige Crosswalk-Zeile
ohnehin als Fund, nicht als Ratefall (siehe situs-Spec).

**`aggregate_members.csv` (aggregate_concept_id|member_concept_id|member_name):**

```sql
SELECT ca.aggregate_concept_id, ca.member_concept_id, n.canonical
FROM concept_aggregate ca
JOIN taxon_concept tc ON tc.id = ca.member_concept_id
JOIN name n ON n.id = tc.accepted_name
```

Derselbe Join, den `GET /v1/concept/{id}`s `aggregateMembers`-Handler
bereits nutzt (Task 9 des Namensraum-Redesigns) — keine neue Join-Logik,
nur eine Batch-Variante davon.

## 3. Fehlerbehandlung & Tests

| Fall | Verhalten |
|---|---|
| `--db`/`--out-dir` fehlt | Fehler, Befehl bricht ab (wie bei `bundle`) |
| DB nicht lesbar | Fehler, Befehl bricht ab |
| Namens-Kollision Fall A/Fall B | gezählt + gemeldet, Export läuft weiter (beide Zeilen landen in der CSV — situs entscheidet, wie es damit umgeht) |
| `concept_aggregate` leer (kein Fall-B-Ingest gelaufen) | `aggregate_members.csv` mit nur der Kopfzeile, kein Fehler |

Tests: `internal/app/export_crosswalk_test.go` (Report-Zahlen, Kollisions-
Fund, beide Quellen korrekt vereinigt), `cmd/hostus/export_crosswalk_test.go`
(CLI-Flags, Fehlerfälle).

## Prüfbare Zusagen

- `eurosl_crosswalk.csv` enthält für jeden Namen HÖCHSTENS eine Zeile pro
  Quelle (Fall A ODER Fall B) — eine Kollision wird gemeldet, nie
  automatisch aufgelöst.
- `aggregate_members.csv`s `member_name` ist immer der AKZEPTIERTE Name des
  Mitglieds-Konzepts (nicht dessen Verbatim-Schreibweise aus irgendeiner
  Quelle) — konsistent mit `/v1/concept/{id}`s `members[]`-Feld.
- Kein Redistributions-Gate — dieser Export läuft unabhängig vom
  `redistribution`-Status der beteiligten Quellen.

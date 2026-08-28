# Gemessene Baseline-Kennzahlen (Drift-Check)

Diese Zahlen sind gegen den Datenstand vom 2026-08-27 gemessen (Task 3s
Neu-Pinnen, siehe `pipelines/eurosl/eurosl.summary.txt` und
`pipelines/germansl/germansl.summary.txt`). Eine Abweichung nach einem
Quellen-Update ist ein Fund, kein automatischer Fehler — aber sie muss
auffallen (siehe Spec Abschnitt 11, Korrektheits-Test 4). Diese Datei ist
selbst der methodische Fixpunkt: eine künftige Neuvermessung überschreibt
die Werte in der Tabelle, nicht die Methodik.

| Kennzahl | Wert | Quelle |
|---|---|---|
| EuroSL-Zeilen gesamt | 139.039 | `pipelines/eurosl/eurosl.summary.txt` (`rows=`) |
| EuroSL Species Aggregate (Rohwert `Species Aggregate`) | 287 | `pipelines/eurosl/eurosl.summary.txt` (`ranks=`) |
| EuroSL Version | Sun Nov 3 11:31:01 2024 | `pipelines/eurosl/eurosl.summary.txt` (`version=`) |
| EuroSL distinkte Rohwerte in `ranks=` | **30** | `pipelines/eurosl/eurosl.summary.txt` — der Plan/Brief nannte 29; siehe Fund 1 unten |
| GermanSL-Zeilen gesamt | 26.599 | `pipelines/germansl/germansl.summary.txt` (`rows=`) |
| GermanSL Version | **1.5.6** | `pipelines/germansl/germansl.summary.txt` (`version=`) — Task 3 hat neu gepinnt; Plan/Brief nannten 1.5.5 |
| GermanSL AGG (Species-Aggregat, Rohwert `AGG`) | 625 | `pipelines/germansl/germansl.summary.txt` (`ranks=`) |
| GermanSL distinkte Rohwerte in `ranks=` | 26 | `pipelines/germansl/germansl.summary.txt` — der Brief nannte 27 |

## Wie diese Zahlen erhoben wurden

```bash
grep '^ranks=' pipelines/eurosl/eurosl.summary.txt | sed 's/ranks=//' | tr ',' '\n' | wc -l
grep '^ranks=' pipelines/germansl/germansl.summary.txt | sed 's/ranks=//' | tr ',' '\n' | wc -l
```

Jeder distinkte Rohwert ist Teil der Golden-Liste in
`internal/domain/rank_golden_test.go`
(`TestParseRankLenient_EuroSLGoldenVocabulary`) bzw. bereits in
`internal/domain/taxon_test.go`
(`TestParseRankLenient_GermanSLRankCodes` +
`TestParseRankLenient_GermanSLDeliberatelyUnmappedCodesStayOther` +
`TestParseRankLenient_GermanSLRootCode`). Ein neuer, dort nicht gelisteter
Rohwert lässt den jeweiligen Test fehlschlagen (falscher `len(golden)`- bzw.
fehlender Testfall), statt automatisch geraten zu werden.

## Funde (Task 13)

Task 13s Auftrag war ausdrücklich, jeden Rohwert **gegen den echten Code**
zu verifizieren statt die Plan-Vorgaben blind zu übernehmen. Dabei kamen
vier reale Abweichungen zu Tage — dokumentiert hier, nicht stillschweigend
"korrigiert":

1. **EuroSL hat 30, nicht 29, distinkte Rohwerte.** Der Plan und dieser
   Task-Brief zählten 29; `eurosl.summary.txt`s `ranks=`-Histogramm listet
   tatsächlich 30 (`Species`, `Subspecies`, `Variety`, `Genus`, `Form`,
   `Family`, `Unranked (infraspecific)`, `Species Aggregate`, `Subvariety`,
   `Section`, `Coll. species`, `Order`, `Tribe`, `Subgenus`, `Subform`,
   `Proles`, `Subfamily`, `Subclass`, `Race`, `Unranked (infrageneric)`,
   `Class`, `Grex (infraspec.)`, `Superorder`, `Subsection bot.`,
   `Subdivision`, `Phylum`, `Division`, `Convar`, `Root`,
   `Suprageneric Taxon`). Der Golden-Test zählt jetzt 30 und bricht ab,
   falls diese Zahl je wieder driftet.

2. **Acht EuroSL-Rohwerte degradieren real zu `RankOther`, obwohl ein
   passender kanonischer Rang bereits existiert.** `canonicalRanks`
   (`internal/domain/taxon.go`) kennt nur die unterstrich-getrennte
   Enum-Schreibweise (z. B. `"SPECIES_AGGREGATE"`), nicht EuroSLs wörtliche,
   leerzeichen-/klammerhaltige Spaltenwerte (z. B. `"Species Aggregate"`).
   Anders als bei GermanSL (`germanSLRankCodes`) existiert für EuroSL keine
   eigene Alias-Tabelle, die diese Lücke schließt. Betroffen (Rohwert →
   tatsächliches Ergebnis, Zeilenzahl aus dem Histogramm):
   - `"Unranked (infraspecific)"` → `RankOther` statt
     `RankUnrankedInfraspecific` (296 Zeilen)
   - `"Species Aggregate"` → `RankOther` statt `RankSpeciesAggregate`
     (287 Zeilen — **exakt die Zahl, die als "Species Aggregate"-Kennzahl
     in der Spec/diesem Dokument steht**; die Aggregat-ERKENNUNG in
     Match/Agreement beruht aber auf `concept_aggregate`-Verknüpfungen,
     nicht auf `Rank`, daher bricht dadurch keine bereits gebaute Task
     4–10-Funktionalität — nur `Rank`/`RankVerbatim` selbst tragen für
     diese 287 Zeilen die falsche Information)
   - `"Coll. species"` → `RankOther` statt `RankCollSpecies` (155 Zeilen)
   - `"Unranked (infrageneric)"` → `RankOther` statt
     `RankUnrankedInfrageneric` (19 Zeilen)
   - `"Grex (infraspec.)"` → `RankOther` statt `RankGrex` (15 Zeilen)
   - `"Subsection bot."` → `RankOther` statt `RankSubsection` (8 Zeilen)
   - `"Division"` → `RankOther`, NICHT `RankPhylum` (2 Zeilen) — ein
     Vorab-Hinweis dieses Tasks nahm an, `"Division"` sei bereits als
     EuroSL-Synonym für Phylum gemappt; das stimmt nicht, es existiert kein
     Alias-Eintrag dafür.
   - `"Suprageneric Taxon"` (1 Zeile) bleibt korrekt/absichtlich
     `RankOther` (Domänen-Bookkeeping-Knoten, siehe Task 1s Design).
   Golden-Test: `TestParseRankLenient_EuroSLGoldenVocabulary` in
   `internal/domain/rank_golden_test.go` pinnt genau dieses reale Verhalten
   (inkl. ausführlicher "KNOWN DEFECT"-Kommentare pro Zeile) — kein
   stillschweigender Fix in diesem Task.

3. **`AggregateResolution.Agreement` zeigt live nie `"one_sided"`.**
   `buildAggregateResolution` (`internal/application/match.go`) ruft
   `Repository.ConceptAgreement` nur auf, wenn BEIDE Namensräume
   (eurosl UND germansl) `AggregatePolicyKnown` sind. Ein echtes
   One-Sided-Aggregat hat aber per Definition genau einen bekannten und
   einen unauflösbaren Namensraum — dieses Gate feuert dort nie, obwohl
   `ComputeConceptAgreement`/`WriteConceptAgreement` den `one_sided`-Wert
   für dasselbe Aggregat bereits korrekt in `concept_agreement` abgelegt
   haben. Die Einseitigkeit bleibt live sichtbar (über
   `AggregateResolution.Options[i].Status` pro Namensraum), nur eben nicht
   über `Agreement`. Siehe
   `TestCrosswalk_RubusFruticosusAggregateIsOneSidedInGermanSL` in
   `internal/application/crosswalk_regression_test.go`, die diesen Fund
   direkt gegen die echten Daten beweist (der Batch-Report liefert
   `AgreementOneSided` für dieselben Daten, der Live-Pfad liefert `""`).

4. **Der "Fall-B-Konzept"-Nachweispfad in Korrektheits-Test 3 ist
   aktuell tote Logik.** `ingestTx.UpsertConcept`s INSERT-Statement
   (`internal/adapters/sqlite/db.go`) enthält die Spalten
   `family`/`order_name`/`class_name` gar nicht; nur
   `UpsertClassification` schreibt sie, und das ruft ausschließlich
   `namespace_ingest.go`s Fall-A-Crosswalk auf — immer im selben aufruf wie
   `AddNameSpaceEntry` für dasselbe Konzept
   (siehe `writeNameSpaceRow`). `nativespace_ingest.go` (Fall B) ruft
   `UpsertClassification` nirgends auf. Ergebnis: nach heutigem Stand hat
   JEDES Konzept mit gesetzter Klassifikation garantiert auch einen
   `name_space_entry` — ein natives eurosl/germansl-Konzept kann derzeit gar
   keine Family/OrderName/ClassName tragen. Das ist eine STÄRKERE
   Invariante als der Plan annahm, keine Lücke. Siehe
   `TestClassification_EveryValueTracesToASourceRow` in
   `internal/application/crosswalk_regression_test.go`.

Keiner dieser vier Funde wurde in diesem Task durch Produktionscode-Änderung
"repariert" — das war ausdrücklich nicht der Auftrag von Task 13. Sie sind
hier und in den jeweiligen Testkommentaren dokumentiert, zur Entscheidung
durch die abschließende Review bzw. einen Folge-Task.

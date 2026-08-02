# CDM `rl_standardliste` — Konzept- und Relationsernte

Quelle: `https://api.cybertaxonomy.org/rl_standardliste` (BGBM/EDIT, CDM
Server der „Standardliste der Farn- und Blütenpflanzen Deutschlands").
Kontext: hostus 2.0 **SP5** (`POST /v1/translate`, UC6), Task 2.
Vorarbeit: `poc/P08-findings.md` (Sonde), `poc/p08b_cdm_sample/` (Messung),
`docs/research/cdm-sample.md` (Auswertung und Go/No-Go).

Geerntet werden **51.466 taxonomische Konzepte** in **18 `sec.`-Referenz­
räumen** plus der **Konzeptrelationsgraph** zwischen ihnen.

## Lizenz — vor allem anderen

**Es gibt keine Lizenzangabe.** Weder auf dem Portal noch auf der API noch
in den Payloads (in P8 geprüft und beim Bau dieser Pipeline erneut). Die
Daten sind aus urheberrechtlich geschützter Florenliteratur abgeleitet.
Daraus folgt:

* `redistribution: unknown`,
* **nur lokale Auswertung**,
* die Weitergabe des abgeleiteten Relationsgraphen über `/v1/translate`
  bleibt gesperrt, bis BGBM/EDIT das schriftlich klären.

Das ist die eigentliche Go/No-Go-Frage für SP5, und sie ist keine technische.

## Crawl-Etikette — verbindlich

Festgelegt vom Auftraggeber, implementiert in `common.py`, nicht umgehbar:

* Genau **ein ehrlicher User-Agent** auf jedem Request:
  `hostus/2.0 (+https://github.com/jobrunner/hostus; jo.brunner@mayflower.de) taxonomic-concept-research`
* **Niemals ein Browser-User-Agent.** 401/403 auf den ehrlichen UA ist ein
  **harter Stopp** (`class Refused`, Exit-Code 2) und wird gemeldet, nicht
  umgangen.
* **≤ 1 Request/Sekunde**, single-threaded, exponentieller Backoff bei
  429/5xx und bei Timeouts.
* Alles unter `.cache/` (gitignoriert) zwischengespeichert: ein
  Wiederholungslauf kostet den Server nichts.

## Drei Phasen

| Phase | Requests | Endpunkt | Liefert |
|---|---:|---|---|
| **A** | 52 | `/portal/taxon?pageSize=1000&pageIndex=N` | alle Konzepte **mit** Name, Rang (roh), `secSource`, Taxon-Nodes **und allen ausgehenden Relationen inline** |
| **C** | ≈ 5.000–10.000 **(geschätzt)** | `/classification/{c}/childNodes`, `/taxonNode/{n}/childNodes` | `parent_uuid` (Baumlauf über die 18 Klassifikationen) |
| **B** | 51.466 | `/taxon/{u}/relationsToThisTaxon` | das Partner-Ende (`to`) jeder Kante — die lange Stange |

### Warum diese Form billiger ist als die von Task 1 kalkulierte

Task 1 kalkulierte `/portal/taxon/{uuid}/taxonRelationships` für alle 51.466
Konzepte plus eine Richtungsabfrage für die ≈ 55 % mit Relationen: rund
80.000 Requests, 22–30 h. Beim Bau dieser Pipeline stellte sich heraus, dass
die **flache Portal-Liste** `/portal/taxon?pageSize=1000&pageIndex=N` pro
Konzept bereits mitliefert:

* `name.nameCache` / `name.titleCache` (Name + Autor),
* `name.rank.representation_L10n` (das **rohe** CDM-Rangvokabular),
* `secSource.citation.{uuid,titleCache}` (Task-1-Befund, bestätigt),
* `taxonNodes[]` (Node-UUID + Klassifikations-UUID des Baums),
* **`relationsFromThisTaxon[]`** — jede ausgehende Relation mit
  Relations-UUID, Typ, Symbol und `conceptRelationship`.

Gemessen auf Seite 0 (1.000 Konzepte): 492 verschiedene Relations-UUIDs,
Halter-Histogramm `{1: 492}`. Die Liste gibt jede Kante also **genau einmal**
aus, an ihrem `from`-Ende — **52 Requests ersetzen 51.466** und liefern die
Richtung gratis mit.

Neues Budget: 52 + ≈ 5.000–10.000 **(geschätzt, nicht gemessen)** + 51.466
Requests. Bei den von Task 1 gemessenen 1,139 s/Request (`max(1 s, Latenz)`,
weil der Limiter ab Request-*Start* misst) sind das rund **17–22 h** — eine
Spanne, deren Untergrenze auf einer **Schätzung** beruht und die deshalb
nicht als gemessene Zahl zu lesen ist. Sie liegt innerhalb von Task 1s
22–30-h-Rahmen, nicht darüber hinaus.

Phase C existiert nur wegen der Spalte `parent_uuid`. Ein Bulk-Endpunkt für
Taxon-Nodes gibt es nicht (`/taxonNode?pageSize=…` → 404,
`/checklist/export` → `records: []` auf jeder Seite, unverändert seit P8).
Deshalb werden die 18 Klassifikationsbäume gelaufen: ein Request je Knoten
**mit** Kindern; Blätter kosten nichts, sie kommen in der Antwort ihres
Elternknotens mit.

**Grundlage der Schätzung 5.000–10.000** (die unsicherste Zahl dieser
Pipeline): Nach 250 Expansionen waren 2.911 Knoten bekannt, davon 476 mit
Kindern — 16 %. Der Anteil interner Knoten sinkt mit der Baumtiefe, und
gelaufen war zu dem Zeitpunkt erst die Baumspitze. Hochgerechnet auf
≈ 57.000 Knoten ergibt das eine Spanne, keine Messung. Wer sie nicht bezahlen
will, setzt `--skip-tree`; dann bleibt `parent_uuid` leer, alles andere in
der Konzept-CSV ist unverändert vollständig.

## Resumierbarkeit

Ein 17–22-h-Lauf **wird** unterbrochen. Jede Phase schreibt nach jeder
Arbeitseinheit auf Platte und setzt dort wieder an — **die drei Phasen sind
dabei aber nicht gleich sauber**, und das gehört gesagt:

* **Phase B und C: exakt.** Sie hängen je Arbeitseinheit **eine geflushte
  NDJSON-Zeile** an und setzen über die **Menge** der bereits vorhandenen
  Einheiten fort, nicht über einen Positions-Offset. Eine abgeschnittene
  letzte Zeile (Kill zwischen `write` und `flush`) wird beim Lesen verworfen
  und die Einheit schlicht neu geholt. Es entstehen keine Dubletten.
* **Phase A: idempotent auf HTTP-Ebene, aber nicht satzgenau beim
  Fortsetzen.** Jede Rohseite liegt als gzip-JSON vor, geschrieben in eine
  Temp-Datei und per `os.replace` umbenannt — ein Kill kann keine halbe
  Seite hinterlassen, die ein späterer Lauf als Wahrheit liest. Der
  Checkpoint wird jedoch **erst nach der ganzen Seite** gesetzt (1.000
  Konzepte), der Abschluss per Sentinel `-1`. Ein Kill nach 600 von 1.000
  destillierten Sätzen lässt den Zähler auf der vorigen Seite stehen; beim
  Fortsetzen wird die Seite **komplett wiederholt** und rund 599 Sätze
  landen ein zweites Mal in `concepts.ndjson`.

  **Das ist bewusst nicht im Crawler repariert, sondern in `convert.py`:**
  `load_concepts_deduped()` dedupliziert auf `concept_uuid` und meldet die
  Zahl der verworfenen Dubletten (`duplicate_concept_records_dropped`). So
  bleibt der Crawler append-only und billig, und ein **laufender** Crawl
  muss für diesen Fix nicht neu gestartet werden. Die Relationen sind ohnehin
  nicht betroffen (`from_holders` ist eine Menge) — eine Wiederholung kann
  den Falsifikator nicht auslösen.

Ein erneuter Aufruf von `build.sh` ist damit kostenlos und beschädigt keinen
Teilzustand; er kann in Phase A lediglich Sätze doppeln, die beim Konvertieren
wieder zusammenfallen. Getestet mit einem echten `SIGKILL` mitten in Phase B,
einer künstlich abgeschnittenen Zeile und einer simulierten
Phase-A-Seitenwiederholung (600 Dubletten → 0 doppelte Primärschlüssel in der
CSV); siehe
`.superpowers/sdd/2026-08-02-sp5-sec-translate/task-2-report.md`.

## Auflösung der Relationen — die globale Kantenkarte

P8 suchte den Partner einer Relation unter den Konzepten **mit demselben
Namen**. Task 1 hat das mit 75,9 % gemessen; die Fehlschläge sind
überwiegend Gattungswechsel (*Coronilla varia* ≜ *Securigera varia*), bei
denen der Partner den Namen naturgemäß nicht teilt. **Die Namensrestriktion
ist ersatzlos gestrichen.** Stattdessen wird jede Relations-UUID in einer
globalen Karte über den ganzen Crawl nachgeschlagen:

```
from_holders[rel_uuid]  Konzepte, die rel_uuid in Phase A führen  (from-Ende)
to_holders[rel_uuid]    Konzepte, die rel_uuid in Phase B führen  (to-Ende)
```

* **resolved** = genau ein `from` und genau ein `to`
* **dangling** = insgesamt genau ein Halter (Partner noch nicht gecrawlt
  oder gar nicht im `/taxon`-Listing)
* **ambiguous** = zwei oder mehr Halter auf **derselben** Seite

## Falsifikator — verbindlich

Task 1s ≈ 100 %-Auflösung ist eine **Projektion**. Sie ruht auf einer
Prämisse: eine Relations-UUID ist eine **binäre Kantenidentität**. Belegt
ist sie auf 602 UUIDs über 782 Konzepte (Histogramm `{1: 346, 2: 256}`); von
den 256 Zwei-Halter-UUIDs hatten 202 auf beiden Seiten Richtungsdaten und
alle 202 zerfielen in genau ein `from` und ein `to`, null Anomalien. Die
Prämisse muss **scheitern können**:

* Bekommt **irgendeine** Relations-UUID einen **dritten Halter**, bricht
  `convert.py` mit Exit-Code 3 ab und schreibt **keine** CSV. Die UUID ist
  dann keine Kantenidentität und das gesamte Auflösungsmodell muss neu
  gedacht werden.
* Die **verbleibende Zahl der Ein-Halter-UUIDs** wird immer gemeldet. Bei
  einem Vollcrawl muss sie gegen null gehen; tut sie das nicht, gibt es
  Relationen zu Konzepten außerhalb des Listings, und die Vollständigkeit
  von `/translate` ist entsprechend zu deckeln.

Ebenfalls hart: `assert_crosswalk()` (aus `cdm_sample.py` übernommen) prüft
bei jedem Lauf, dass jedes Crosswalk-Ziel eine der 18 echten
Klassifikations-UUIDs ist und dass jede Klassifikation entweder getroffen
oder ausdrücklich als unabgebildet deklariert ist.

## Rohvokabular, kein Mapping

`relation_type` und `rank` tragen das **rohe CDM-Vokabular**
(`Congruent to`, `Included in or Includes or Overlaps`, `Species`,
`Species Aggregate`, `Unranked (infraspecific)`, …). Es wird hier
**absichtlich nicht** auf das hostus-Vokabular abgebildet — Mapping ist eine
Domänenentscheidung und gehört in **Task 3**, wo es testbar ist und wo ein
unbekannter Wert laut abbricht. Das ist die `domain.ParseRank`-Lektion: SP1
nahm 6 Ränge an, WCVP hatte 34, der Vollimport brach nach 5,4 s ab.

Der Validierungslauf hat **22 rohe Ränge** und **7 Relationstypen** gesehen —
einen mehr als Task 1s Stichprobe (`Not Congruent to`, 1×). `convert.py`
meldet jeden Typ, den Task 1 nicht gesehen hat, ausdrücklich.

Aus demselben Grund ist **`status` leer**, wo der Baumlauf das Konzept nicht
erreicht hat. Die Spalte trägt ausschließlich das rohe
`TaxonNodeDto.taxonStatus`; nichts wird synthetisiert. Eine frühere Fassung
setzte ersatzweise `Accepted` — damit hätten 51.464 von 51.466 Zeilen einen
nie gemessenen Wert behauptet, in genau der CSV, deren Vertrag „rohes
Vokabular, Mapping in Task 3" lautet. Das boolesche CDM-Feld
`Taxon.doubtful` ist ein **anderes** Feld und wird nicht eingemischt; es
erscheint als eigener Zähler `doubtful_concepts` in der Zusammenfassung.

## Der CSV-Vertrag — bitte genau lesen

Beide Dateien werden von `csv.writer(delimiter="|")` mit Pythons
Voreinstellung `QUOTE_MINIMAL` geschrieben, also **mit RFC-4180-Quoting** und
`"` als Quote-Zeichen. Das ist dieselbe Konvention wie in
`pipelines/wikidata/convert.py`. Ein Feld, das ein Anführungszeichen
enthält, wird gequotet und seine Anführungszeichen werden verdoppelt.
**237 der 51.466 Konzepte sind betroffen**, z. B.:

```
e18ac1cf-…|"Achillea millefolium ""Sammelart"""||Species Aggregate|…
```

Ein Konsument **muss** deshalb einen echten CSV-Reader mit `Comma = '|'`
verwenden (Go: `encoding/csv`, `r.Comma = '|'`) und **niemals**
`strings.Split(line, "|")`. Der naive Split liefert für obige Zeile
`"Achillea millefolium ""Sammelart"""` statt
`Achillea millefolium "Sammelart"` — die Feldzahl stimmt zufällig, der Wert
nicht. Das ist die Falle, in die Task 3 nicht laufen darf.

Was `_clean()` zusätzlich tut, ist **kein** Escaping: Zeilenumbrüche werden
zu Leerzeichen, und ein literales `|` wird durch `/` ersetzt. Letzteres ist
**verlustbehaftete Korruption**, nicht Maskierung — das Original ist weg.
Heute betrifft es **0 Felder**, es ist reine Vorsichtsmaßnahme. Sollte je ein
`|` in den Daten auftauchen, ist die Substitution zu **entfernen** und auf
das ohnehin vorhandene Quoting zu vertrauen, nicht beizubehalten.

## Aufruf

```bash
# Vollcrawl, bis fertig (nach einem Abbruch einfach erneut aufrufen —
# es geht nichts verloren und nichts wird doppelt geholt)
bash pipelines/cdm/build.sh

# Begrenzter Abschnitt: höchstens 3600 s crawlen, dann sauber stoppen
bash pipelines/cdm/build.sh 3600

# Begrenzte Validierungsscheibe
CDM_CRAWL_ARGS="--max-pages 2 --max-concepts 400 --skip-tree" \
  bash pipelines/cdm/build.sh
```

Exit-Codes:

| Code | Bedeutung |
|---:|---|
| `0` | fertig |
| `1` | Crawl noch nicht vollständig — erneut aufrufen, es wird nichts doppelt geholt |
| `2` | **ehrlicher User-Agent abgelehnt — stoppen und melden**, nicht umgehen |
| `3` | **Falsifikator ausgelöst**: eine Relations-UUID hat einen dritten Halter bekommen. Keine CSV geschrieben. |
| `4` | Konvertierung aus einem **anderen** Grund fehlgeschlagen (Absturz, `assert_crosswalk()`, fehlende Cache-Datei). **Nicht** der Falsifikator. |

`3` und `4` sind bewusst getrennt: `3` heißt genau eine Sache, nämlich dass
das Auflösungsmodell aus `docs/research/cdm-sample.md` widerlegt ist. Ein
Absturz darf diese Aussage nicht verwässern.

## Artefakte

`output/`, `.cache/` und `cdm.summary.txt` sind gitignoriert. **Es werden
keine Bulk-Daten committet** — committet sind die Skripte, diese
Dokumentation und **eine namentlich benannte De-minimis-Testfixture** unter
`fixtures/` (14 Relationen, die alle sechs aufgelösten Typen abdecken, plus
ihre 18 Konzepte; 32 Zeilen insgesamt).

Warum diese Fixture trotz `redistribution: unknown` im Repository liegt —
die Entscheidung gehört festgehalten, nicht stillschweigend getroffen: Eine
32-Zeilen-Scheibe besteht aus Identifikatoren, wissenschaftlichen Namen und
Begriffen eines kontrollierten Vokabulars — **Fakten, keine schöpferische
Leistung**. Sie existiert einzig, damit die Go-Tests in Task 3 ohne
Netzzugriff laufen können. Sie ist **nicht** der Datenbestand, der mit der
Software ausgeliefert wird — und genau dort verläuft die Linie, die die
Lizenzlage zieht. **Die Fixture wird nicht vergrößert.**

Die Zusammenfassung des Validierungslaufs ist wörtlich in
`pipelines/README.md` festgehalten.

Die beiden kanonischen CSV-Verträge sind in `pipelines/README.md`
dokumentiert.

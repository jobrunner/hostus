# FloraVeg-Namensraum: Ingest und gemessener Crosswalk (SP9, Task 1)

Stand: 2026-08-04. Gemessen gegen den **vollständigen, realen WCVP-Index**
(`wcvp 2026-06-04`, 440.534 `taxon_concept`, 1.448.984 `name`) und die reale
Pipeline-Ausgabe `pipelines/floraveg/output/floraveg-canonical.csv`.

## Ausgangsbefund: FloraVeg war auf `master` nirgends ingestiert

Vor dieser Aufgabe existierte das Pipeline-Artefakt, aber **kein Pfad, der es
in den Index bringt**:

- `pipelines/floraveg/output/floraveg-canonical.csv` lag auf Platte (16.402
  Datenzeilen), erzeugt von `pipelines/floraveg/convert.py`.
- `dataset.example.yaml` führte `floraveg` unter **`backbones:`** mit
  `path: ./backbones/floraveg`. Das war nicht bloß der falsche Abschnitt,
  sondern **nicht ingestierbar**: `internal/app.readerFor` liest *jeden*
  `backbones:`-Eintrag durch den WCVP-DwC-A-Reader, und die Pipeline liefert
  gar kein DwC-A-Verzeichnis, sondern eine einzelne kanonische CSV.
- Kein Reader las den Namenslisten-Vertrag (`taxon|rank|status|
  accepted_taxon|source_id`), obwohl `pipelines/README.md` ihn für vier
  Quellen (floraveg, germansl, eurosl, euromed) verbindlich beschreibt.
- Die Treffer auf „floraveg" im Go-Code waren durchweg **Etiketten, keine
  Daten**: `trait_vocabulary.taxonomy = "floraveg-eunis-aligned"` (der
  Namensraum, gegen den Tichý/Midolo harmonisiert sind) und ein
  `floraveg`-Eintrag in der Kommentar-Aufzählung von `xref.authority`.
  Weder `xref` noch sonst eine Tabelle enthielt je eine FloraVeg-Zeile.

Ergebnis: FloraVeg war dokumentiert, gepinnt und geerntet — aber nie
ingestiert.

## Ingest-Design

Ein **Namensraum** ist eine Checkliste, die Namen beiträgt und keine
Taxonomie: keine Synonymie, keine Elternkette, keine externe ID zum Joinen.
Er ist deshalb weder ein Backbone (er erzeugt keine `taxon_concept`-Zeilen
und darf nicht in `backbone_version` landen, das die
`backbone_versions`-Provenienz jeder API-Antwort und `/health/ready` speist)
noch ein Trait-Vokabular.

| Schicht | Neu |
| --- | --- |
| `internal/adapters/namelist` | Reader für den geteilten Namenslisten-CSV-Vertrag (nach dem Vertrag benannt, nicht nach FloraVeg — alle vier Pipelines emittieren denselben Header) |
| `internal/domain` | `NameSpaceMeta`, `NameSpaceEntry`, `IsAggregateName` |
| `internal/adapters/sqlite` | Tabellen `name_space` + `name_space_entry`, Schreib-/Leseweg |
| `internal/application` | `IngestNameSpace` (zweiphasig, siehe unten) |
| `internal/adapters/manifest` | Manifest-Abschnitt `name_spaces:` (Schema + strikter Decode) |
| `internal/app` | Adapter→Application-DTO-Brücke + Verdrahtung in `hostus ingest` |

`name_space_entry` ist auf **`(space, ext_id)`** geschlüsselt, nicht auf
`(space, concept_id)`. Das ist der Kern für UC4: FloraVeg schreibt *Festuca
ovina* unter drei SeqIDs dreifach — `Festuca ovina` (5647), `Festuca ovina
aggr.` (5648), `Festuca ovina s. l.` (5649) — und alle drei fallen auf
dasselbe WCVP-Konzept, weil WCVP überhaupt keine aggregatmarkierten Namen
führt. Auf das Konzept zu schlüsseln würde genau die Unterscheidung
wegwerfen, die `aggregate_policy` treffen muss.

### Crosswalk: SP3-Maschinerie, nicht ein zweiter Pfad

`IngestNameSpace` löst jeden Namen über **`resolveTraitName`** auf — dieselbe
unveränderte Funktion, die der Trait-Ingest benutzt, also dieselbe
`domain.NameCandidates`-Leiter (exakter Schlüssel zuerst, dann
Hybrid-/Genitiv-Schreibweisen, dann die zwei markierten
Circumscriptions-Urteile) mit denselben drei Ausgängen und derselben
Weigerung zu raten. Es gibt bewusst **keinen zweiten Namensauflösungspfad**.

Der Ablauf ist strikt zweiphasig: Phase 1 löst alle *distinkten* Namen auf,
**ohne offene Transaktion**; Phase 2 öffnet eine Transaktion und schreibt
nur. Der SQLite-Adapter läuft mit `SetMaxOpenConns(1)` — ein Lesezugriff bei
offener Ingest-Transaktion ist ein echter Deadlock in `hostus ingest`. Ein
Test pinnt das (`readsAfterBegin == 0`).

### Verlust ist sichtbar, nie still

`hostus ingest` gibt pro Namensraum aus:

```
Name spaces:
  floraveg: rows=16402 matched=14050 unmatched=357 ambiguous=1995 concepts=13473
    aggregates: 246 of 309 resolved
    dropped: duplicate ext_ids=0 reader errors=0
    normalized aggregate_to_nominate: rows=246 taxa=246 [flagged: circumscriptions equated, not identical]
    ...
    unmatched sample: …
    ambiguous sample: …
  hinweis: floraveg (redistribution=unknown) — lokal genutzt, nicht redistribuierbar
```

`matched + unmatched + ambiguous == rows` gilt immer; jede Verlustart hat
zusätzlich eine begrenzte, deterministische Stichprobe. Ein doppelter
`ext_id` wird **gezählt statt überschrieben** (`INSERT OR REPLACE` würde die
erste Zeile still verdrängen); FloraVeg hat davon null.

## Die drei gemessenen Zahlen

Kommando (Wegwerf-Harness gegen eine schreibbare Kopie des realen Index,
nicht committet):

```console
$ cp /tmp/full-real.sqlite "$SCRATCH/measure.sqlite"
$ nix develop -c go run ./measure_tmp "$SCRATCH/measure.sqlite" \
    pipelines/floraveg/output/floraveg-canonical.csv
csv rows=16402 reader_errors=0
backbones=[{ID:wcvp Version:2026-06-04 …}]
rows=16402 matched=14050 unmatched=357 ambiguous=1995
aggregates=309 aggregates_matched=246
concepts=13473 duplicate_ext_ids=0
normalized aggregate_to_nominate rows=246 taxa=246 flagged=true
normalized hybrid_spacing        rows=236 taxa=236 flagged=false
normalized hybrid_marker_added   rows=77  taxa=77  flagged=false
normalized orthography_genitive  rows=20  taxa=20  flagged=false
normalized hybrid_marker_dropped rows=16  taxa=16  flagged=false
aggregates_unmatched=2 aggregates_ambiguous=61
```

### 1. FloraVeg-Namen auf einem WCVP-Konzept

| | Zeilen | Anteil |
| --- | ---: | ---: |
| FloraVeg-Namen gesamt | 16.402 | 100 % |
| **matched** | **14.050** | **85,7 %** |
| unmatched | 357 | 2,2 % |
| **ambiguous** | **1.995** | **12,2 %** |

Die Zeilenzahl ist zugleich die Taxazahl: alle 16.402 `taxon`-Werte sind
distinkt. (Der Aufgabentext nennt 16.403 — das ist die *Zeilenzahl der
Datei inklusive Kopfzeile*; Datenzeilen sind 16.402.)

Nur 13.455 der geschriebenen Einträge trafen exakt; 595 brauchten eine
Normalisierungsregel, davon 246 die **markierte**
`aggregate_to_nominate`-Regel.

### 2. Aggregate

| | Zeilen | Anteil |
| --- | ---: | ---: |
| Aggregat-Namen gesamt | **309** | 100 % |
| **matched** | **246** | **79,6 %** |
| ambiguous | 61 | 19,7 % |
| unmatched | 2 | 0,6 % |

**Korrektur zum Aufgabentext:** es sind **309**, nicht 308. 305 tragen
`aggr.`, 3 tragen `s. l.` — und eine, `Dryopteris affinis s. lat.`, trägt die
Langform, die eine `s\. l\.`-Suche nicht findet. `domain.AggregateBases`
kennt sie (der Aggregat-Marker-Satz stammt aus SP3), weshalb der gemessene
Wert die Handzählung korrigiert statt ihr zu folgen.

Jeder dieser 246 Treffer ist ein **markierter** Treffer und keiner ist ein
Aggregat-Konzept: WCVP führt null aggregatmarkierte Namen, also löst ein
Aggregat immer auf seine Nominatart auf — eine Circumscription, die *enger*
ist als die des Aggregats. Genau deshalb steht die Regel in
`name_space_entry.resolution` und wird im Report getrennt ausgewiesen.

### 3. WCVP-Konzepte mit FloraVeg-Gegenstück

| | Konzepte | Anteil |
| --- | ---: | ---: |
| **mit FloraVeg-Eintrag** | **13.473** | — |
| von allen `taxon_concept` | 440.534 | **3,06 %** |
| davon Rang SPECIES | 13.123 von 368.928 | 3,56 % |

13.473 < 14.050, weil ein Konzept mehrere FloraVeg-Schreibweisen trägt
(577 Mehrfachzuordnungen, überwiegend die Aggregat-/`s. l.`-Dubletten).

## Befund: die Mehrdeutigkeit ist die Auflage, nicht die Nichttreffer

Mit 12,2 % ist **ambiguous fünfmal so groß wie unmatched**. Das ist keine
Anomalie dieses Namensraums, sondern dieselbe Auflage, die SP3 für die
Trait-Vokabulare gemessen hat (EIVE: 8.961 ambiguous von 71.266 Zeilen =
12,6 %, `docs/research/reality-check.md`). Beispiel `Abies alba`: WCVP führt
zwei Namen dieser Schreibweise — `Abies alba Mill.` (akzeptiert) und `Abies
alba (Castigl.) Michx.` (Synonym eines anderen Konzepts). Der Crosswalk
weigert sich zu raten und zählt die Zeile.

Der nächste Hebel für Abdeckung liegt damit **nicht** in weiteren
Schreibweisen-Regeln — die sind ausgereizt (595 Zeilen Gesamtertrag) —
sondern in **Disambiguierung**: bei genau einem `accepted`-Kandidaten unter
mehreren Namen wäre die Wahl nicht geraten. Das ist bewusst *nicht* Teil
dieser Aufgabe: `resolveTraitName` ist geteilt, eine Änderung dort verschiebt
auch den SP3-Trait-Crosswalk und braucht ihre eigene Messung.

## Redistribution-Gate: verifiziert, nicht angenommen

FloraVegs Lizenz ist ungeklärt (`redistribution: unknown`,
`pipelines/README.md`), lokale Auswertung ist zulässig, Weitergabe nicht.

Das Gate (`findRestrictedSources` in `internal/adapters/sqlite/bundle.go`)
fragte vor dieser Aufgabe **nur** `backbone_version`, `trait_vocabulary` und
`xref_source` ab. Ein Namensraum war ihm unbekannt — der Test dafür wurde
zuerst geschrieben und war **rot**:

```
--- FAIL: TestExportBundle_RefusesByDefaultWhenNameSpaceNotAllowed
--- FAIL: TestExportBundle_ForceIncludeRestrictedNameSpace_SucceedsAndRecordsSource
```

Das ist exakt die Lücke, die der SP4-Review beschrieben hat: jede Aufgabe
war lokal korrekt, und das Gate leckte trotzdem, weil eine *neue Art* von
Quelle schlicht nicht in der Abfrage stand. Behoben durch eine vierte
Abfrage über `name_space_entry → name_space`, plus **konzept-gescopte**
Kopie beider Tabellen (dieselbe Begründung wie bei `sec_reference`:
`name_space_entry.name` ist geernteter Inhalt, kein Quellen-Metadatum). Ein
Bundle kann so nur einen Namen tragen, den das Gate bereits gesehen hat.

Gepinnt auf zwei Ebenen — Adapter (`bundle_namespace_test.go`) und
**Kompositionswurzel** (`internal/app`, `TestBundle_RefusesNameSpaceByDefault`:
echter `app.Ingest` gefolgt von echtem `app.Bundle`), damit „lokal korrekt,
global leck" nicht wieder möglich ist.

## Was diese Aufgabe NICHT liefert

`esy_diagnostic_relevance` bleibt offen. Das **ESy-Regelwerk wurde nie
geerntet** — SP3 hat es explizit ausgeklammert, und die FloraVeg-Pipeline
zieht ihre Namensliste aus `Life_form.xlsx`, einer Merkmalstabelle ohne jeden
Bezug zum Expertensystem. Das ist eine **Datenbeschaffungs-, keine
Implementierungslücke** (das ESy-Regelwerk selbst ist auf Zenodo CC BY 4.0
und damit lizenzrechtlich unproblematisch — im Gegensatz zu den
floraveg.eu-Downloads). Task 3 dokumentiert sie.

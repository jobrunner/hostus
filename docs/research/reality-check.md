# Reality-Check hostus 2.0 — Messungen an Volldaten

> **Regel dieses Dokuments:** Eine Zahl, die nicht gemessen wurde, steht hier
> nicht drin. Jede Kennzahl trägt den Befehl, der sie erzeugt hat. Wo
> gestichprobt wurde, steht Stichprobengröße und Auswahlverfahren dabei.
> Nicht messbare Punkte sind explizit als „nicht gemessen — Grund" markiert.
>
> Task 4 ergänzt je Abschnitt das Verdikt (**hält / hält mit Auflagen /
> hält nicht**); die Abschnitte sind dafür mit einem leeren
> „Verdikt"-Platzhalter vorbereitet.

**Messumgebung:** Apple Silicon (darwin/arm64, Darwin 25.5.0), Go 1.26.5,
`nix develop`-Shell des Repos, Branch `feature/reality-check`, Binary aus
`nix develop -c make build` (`hostus` v0.2.4-81-gc1d616d-dirty).
Alle Rohdaten liegen unter `poc/data/` bzw. `poc/measure/out/` (beide
gitignored); der Mess-Harness liegt unter `poc/measure/` und ist eingecheckt.

Alle Schritte sind in `poc/measure/run.sh` gebündelt (`run.sh m1 m2 m3 m4 m5`).

!!! note "Kein Eintrag in der MkDocs-Navigation"
    `docs/research/` steht in `mkdocs.yml` unter `exclude_docs` (interner
    Recherchekorpus, wird nicht publiziert). Ein `nav`-Eintrag würde
    `mkdocs build --strict` mit „nav references an excluded file"
    fehlschlagen lassen; dieses Dokument bleibt deshalb — wie
    `quellenregister.md` — repo-lokal und aus der Navigation heraus.

---

## 0. Datenbeschaffung: WCVP-Volldump

```bash
curl -sSL -o poc/data/wcvp/wcvp_dwca.zip \
  https://sftp.kew.org/pub/data-repositories/WCVP/wcvp_dwca.zip
unzip -o poc/data/wcvp/wcvp_dwca.zip -d poc/data/wcvp/extracted
```

| Kennzahl | Wert |
|---|---|
| Download-Dauer (wall clock) | 7,477 s |
| Archivgröße | 88.208.088 Byte (84,1 MiB) |
| SHA-256 | `6ff1e084d7e1de5bce526a9952c96fbb0f13b4f2c615b0fc30c055f88bfb5483` |
| `eml.xml` `pubDate` | 2026-06-04 |
| Lizenz laut `eml.xml` | CC BY 3.0 (Versionskonflikt zum GBIF-Katalog, siehe Quellenregister) |
| `wcvp_taxon.csv` | 507.635.435 Byte, 1.448.984 Datenzeilen |
| `wcvp_distribution.csv` | 64.791.546 Byte, 1.995.338 Datenzeilen |
| `wcvp_replacementNames.csv` | 1.858.715 Byte, 44.041 Datenzeilen |

Statusverteilung der Taxon-Zeilen
(`awk -F'|' 'NR>1{print $9}' wcvp_taxon.csv | sort | uniq -c | sort -rn`):

| `taxonomicstatus` | Zeilen |
|---|---|
| Synonym | 880.359 |
| Accepted | 434.691 |
| Illegitimate | 48.723 |
| Invalid | 37.317 |
| Unplaced | 35.347 |
| Artificial Hybrid | 4.391 |
| Provisionally Accepted | 3.224 |
| Orthographic | 2.271 |
| Local Biotype | 1.452 |
| Misapplied | 1.209 |

---

## M1 — Voller WCVP-Ingest

### M1.0 Erster Befund: `hostus ingest` bricht am realen WCVP hart ab

Der erste Lauf gegen das **unveränderte** Archiv:

```bash
/usr/bin/time -l ./hostus ingest \
  --dataset poc/measure/dataset.wcvp.yaml --db poc/measure/out/m1.sqlite
```

```
hostus: application: backbone "wcvp", taxon "542377": domain: unknown taxon rank "proles"
        5,37 real         4,81 user         0,59 sys
          2670788608  maximum resident set size
```

Der Lauf endet nach **5,37 s** mit einem Fehler und schreibt **nichts**
(DB-Datei bleibt bei 122.880 Byte, nur Schema). `domain.ParseRank`
(`internal/domain/taxon.go:22`) kennt genau sechs Rangschreibweisen
(FAMILY, GENUS, SPECIES, SUBSPECIES, VARIETY, FORM); WCVP liefert real
**34 verschiedene** `taxonrank`-Werte. Der Ingest behandelt einen
unbekannten Rang als fatalen Fehler, nicht als überspringbare Zeile — die
20-Taxa-Fixture hat das nie ausgelöst.

Rangverteilung im echten Archiv
(`awk -F'|' 'NR>1{print $8}' wcvp_taxon.csv | sort | uniq -c | sort -rn`):

| `taxonrank` | Zeilen | von `ParseRank` unterstützt |
|---|---|---|
| Species | 1.048.616 | ja |
| Variety | 229.446 | ja |
| Subspecies | 73.948 | ja |
| Form | 43.609 | ja |
| Genus | 42.142 | ja |
| Subvariety | 3.350 | **nein** |
| *(leer)* | 2.744 | **nein** |
| proles | 2.351 | **nein** |
| lusus | 660 | **nein** |
| Subform | 645 | **nein** |
| nothosubsp. | 552 | **nein** |
| microgène | 371 | **nein** |
| Convariety | 184 | **nein** |
| nothovar. | 134 | **nein** |
| monstr. | 90 | **nein** |
| grex | 41 | **nein** |
| subproles | 18 | **nein** |
| stirps | 17 | **nein** |
| provar. | 16 | **nein** |
| nothof. | 15 | **nein** |
| psp. | 6 | **nein** |
| modif. | 6 | **nein** |
| mut. | 5 | **nein** |
| sublusus | 4 | **nein** |
| subap. | 3 | **nein** |
| subsubsp. | 2 | **nein** |
| subspecioid | 2 | **nein** |
| positio, nid, micromorphe, microf., group, ecas., agamosp. | je 1 | **nein** |

```bash
awk -F'|' 'NR>1{r=toupper($8); if(r!="SPECIES"&&r!="VARIETY"&&r!="SUBSPECIES"&&r!="FORM"&&r!="GENUS"&&r!="FAMILY") n++} END{print n}' \
  poc/data/wcvp/extracted/wcvp_taxon.csv
# 11223
```

**11.223 von 1.448.984 Zeilen (0,775 %)** tragen einen Rang, den der Dienst
nicht kennt. Nebenbefund: `FAMILY` kommt im WCVP-Kern **null** mal als
eigene Zeile vor — Familie ist dort eine Spalte, kein Taxon-Datensatz. Der
`RankFamily`-Zweig ist also gegenüber WCVP toter Code.

**Konsequenz für alle weiteren Messungen:** Um M1–M6 überhaupt messen zu
können, wurde eine gefilterte Kopie des Archivs erzeugt, die nur Zeilen mit
unterstütztem Rang enthält. Das ist ein *Workaround für die Messung*, keine
Lösung — er ist hier offengelegt, damit keine Zahl unten so wirkt, als
liefe der Ingest am Volldatensatz durch.

```bash
bash poc/measure/filter_ranks.sh poc/data/wcvp/extracted poc/data/wcvp/filtered
# src rows: 1448984
# dst rows: 1437761
```

### M1.1 Zweiter Befund: der Ingest ist quadratisch und wurde abgebrochen

Lauf gegen das **gefilterte** Archiv (1.437.761 Taxon-Zeilen, 1.995.338
Distribution-Zeilen):

```bash
/usr/bin/time -l ./hostus ingest \
  --dataset poc/measure/dataset.wcvp.yaml --db poc/measure/out/m1.sqlite
```

| Kennzahl | Wert |
|---|---|
| Laufzeit bis zum **manuellen Abbruch** | **1368,05 s = 22 min 48 s** |
| davon user / sys | 913,31 s / 299,42 s |
| Peak RSS (`maximum resident set size`) | **3.059.204.096 Byte = 2,85 GiB** |
| `peak memory footprint` | 3.068.496.656 Byte |
| DB-Datei nach Abbruch | 4.096 Byte (die Transaktion war nie committed) |
| WAL nach Abbruch | 548.442.072 Byte |
| Fortschritt beim Abbruch | noch in **Pass 1**, Unter-Pass `linkSelfReferences` |

Der Lauf wurde nach 22:48 min per `kill -9` beendet, weil er kein Ende
absehen ließ. Er hat **nichts** persistiert: die Transaktion umfasst den
gesamten Backbone, ein Abbruch verwirft alles.

**Wo die Zeit hingeht** (`sample <pid> 3`, zweimal genommen, 17:52 und
17:59):

```
github.com/jobrunner/hostus/internal/application.(*ingestState).pass1AcceptedAndNames
  -> linkSelfReferences
     -> sqlite.(*ingestTx).UpsertConcept
        -> _sqlite3Step -> _sqlite3BtreeNext -> _moveToLeftmost -> _moveToChild
```

Das ist ein **B-Tree-Scan pro geschriebener Zeile**. Ursache:
`schema.sql` setzt `PRAGMA foreign_keys = ON` und definiert elf
Fremdschlüssel, legt aber **genau einen** Index an
(`idx_name_canonical_fold`). Die *Kindspalten* der Fremdschlüssel
(`name.basionym_id`, `taxon_concept.parent_id`,
`taxon_concept.accepted_name`, `concept_name.name_id`, `xref.concept_id`,
`fts_name_map.concept_id`, …) sind unindiziert. `UpsertName`/`UpsertConcept`
verwenden `INSERT OR REPLACE`, das intern ein DELETE ausführt; für das
DELETE muss SQLite jede referenzierende Tabelle vollständig scannen, um die
FK-Constraints zu prüfen. Kosten pro Zeile wachsen mit der Tabellengröße →
**quadratisch**.

### M1.2 Beleg: gemessene Skalierungskurve

Statt die Laufzeit zu extrapolieren, wurde sie an drei Größen wirklich
gemessen — je einmal mit dem Serienschema (`plain`) und einmal mit
zusätzlich angelegten Indizes auf den FK-Kindspalten (`indexed`,
`poc/measure/fk_indexes.sql`, in eine leere, bereits mit dem Serienschema
angelegte DB eingespielt):

```bash
nix develop -c bash poc/measure/scaling.sh 50000 100000 200000
```

| Taxon-Zeilen | Serienschema (`plain`) | mit FK-Indizes (`indexed`) | Faktor |
|---:|---:|---:|---:|
| 50.000 | 65 s | 5 s | 13,0× |
| 100.000 | 293 s | 11 s | 26,6× |
| 200.000 | 1.338 s (22 min 18 s) | 25 s | 53,5× |

Skalierung der Laufzeit bei Verdopplung der Zeilenzahl (gemessen, nicht
gerechnet): `plain` 65 → 293 s (**×4,51**) → 1.338 s (**×4,57**); `indexed`
5 → 11 s (**×2,2**) → 25 s (**×2,27**). Ein Faktor ~4,5 bei doppelter
Datenmenge ist quadratisch, ein Faktor ~2,2 ist linear. Damit ist die
Ursachenanalyse aus M1.1 belegt und nicht bloß plausibel.

Die resultierenden DB-Größen (`plain` / `indexed`): 23,9/28,4 MB (50k),
47,3/56,1 MB (100k), 97,7/114,9 MB (200k) — die Indizes kosten rund 18 %
Speicher.

### M1.2a Nach Hardening (Task 2): dieselbe Messung mit dem reparierten Serienschema

Hardening Task 2 (`internal/adapters/sqlite/schema.sql`, Branch
`feature/hardening`) fügt genau die FK-Kindspalten-Indizes aus
`poc/measure/fk_indexes.sql` (acht Stück) direkt ins Serienschema ein
(plus Herleitung/Begründung, welche der 13 REFERENCES-Spalten schon durch
die führende PK-Spalte ihrer Tabelle abgedeckt waren und deshalb keinen
eigenen Index brauchten). Das macht `plain` (`hostus bundle` legt jetzt
direkt das reparierte Schema an) und `indexed` (zusätzlich
`fk_indexes.sql` — jetzt redundant, da dieselben Spalten schon indiziert
sind, nur unter anderem Indexnamen) erwartungsgemäß nahezu identisch
schnell:

```bash
nix develop -c make build   # bindet das neue schema.sql ein
nix develop -c bash poc/measure/scaling.sh 50000 100000 200000
```

| Taxon-Zeilen | vorher: Serienschema (`plain`, ohne Indizes) | **nachher: Serienschema (`plain`, mit den neuen Indizes)** | mit zusätzlichem `fk_indexes.sql` (`indexed`, jetzt redundant) |
|---:|---:|---:|---:|
| 50.000 | 65 s | **6 s** | 6 s |
| 100.000 | 293 s | **11 s** | 14 s |
| 200.000 | 1.338 s (22 min 18 s) | **24 s** | 30 s |

Skalierung bei Verdopplung, nachher (`plain`, gemessen): 6 → 11 s
(**×1,83**) → 24 s (**×2,18**) — beides klar im linearen Bereich (~2×),
nicht im quadratischen (~4,5×) der vorherigen Messung. Die nachher-Zahlen
dieser Messung liegen sogar leicht UNTER der ursprünglichen `indexed`-Spalte
(5/11/25 s), weil `poc/measure/fk_indexes.sql` in dieser Messung nur noch
für die `indexed`-Spalte läuft und dort — da die Spalten bereits über die
schema.sql-eigenen Indizes abgedeckt sind — zusätzliche, redundante Indizes
unter anderem Namen anlegt (sichtbar an den etwas größeren `indexed`-DB-Größen
unten, nicht an einem Geschwindigkeitsunterschied).

DB-Größen (`plain` mit den neuen schema.sql-Indizes / `indexed` mit den
zusätzlichen, jetzt redundanten `fk_indexes.sql`-Indizes): 28,5/33,0 MB
(50k), 56,3/65,1 MB (100k), 115,3/133,0 MB (200k).

**Verdikt: hält.** Das reparierte Serienschema erreicht am selben
Mess-Harness dieselbe lineare Skalierung, die M1.2 vorher nur mit
Ad-hoc-Indizes zeigen konnte — ohne dass ein Aufrufer noch manuell
`fk_indexes.sql` einspielen müsste. Der volle WCVP-Ingest (M1.3) mit
diesem Schema ist Aufgabe von Task 3/6 dieses Hardening-Zyklus, nicht
dieser Messung.

### M1.3 Voller Ingest MIT den zusätzlichen Indizes

Alle weiteren Messungen (M2–M6) laufen gegen diese Datenbank. Sie entsteht
so: leere DB mit dem **Serienschema** anlegen (`hostus bundle` öffnet die
`--db`-Datei und legt das Schema an), dann `poc/measure/fk_indexes.sql`
einspielen, dann den echten `hostus ingest` laufen lassen.

**Klarstellung (Task 6), weil das leicht misszuverstehen ist:** „legt das
Schema an" gilt nur für eine **neue, leere** `--db`-Datei.
`internal/adapters/sqlite/schema.sql` ist durchgehend
`CREATE TABLE IF NOT EXISTS` (und ebenso `CREATE INDEX IF NOT EXISTS`) —
genau die Begründung im Schema-Kopf selbst, „Applied verbatim … so Open()
can apply it idempotently against an already-initialized database". Für
eine **bereits existierende** DB-Datei aus einem älteren hostus-Stand
passiert beim Öffnen mit einem neueren Binary **nichts**: eine Spalte wie
`rank_verbatim` (Task 2) oder `trait_value.resolution` (Task 5) wird nicht
nachträglich ergänzt, ein fehlender FK-Index nicht nachträglich angelegt.
Es gibt keinen Migrationspfad — eine ältere DB-Datei bleibt auf ihrem
Stand, und die einzige Art, eine Datenbank auf den neuen Schema-/Datenstand
zu bringen, ist ein neuer `hostus ingest` gegen eine neue, leere Datei. Jede
Datenbank in diesem Dokument (M1'/M2'/T5/…) ist deshalb ein vollständiger
Neu-Ingest, keine migrierte Kopie einer älteren.

```bash
nix develop -c bash poc/measure/run.sh m2
```

| Kennzahl | Wert |
|---|---|
| Wall-Clock (WCVP + EIVE + Tichý + Midolo, ein Lauf) | **276,70 s = 4 min 37 s** |
| davon user / sys | 156,63 s / 129,70 s |
| **Peak RSS** | **3.185.754.112 Byte = 2,97 GiB** |
| `peak memory footprint` | 3.389.148.160 Byte |
| **DB-Größe danach** | **951.762.944 Byte = 908 MiB** |

Der Ingest-Report des Laufs (`poc/measure/out/m2-ingest.log`):

```
Ingest complete:
  wcvp: names=1437761 concepts=440098 synonyms=953262 orphaned=44401
```

Zeilenzahlen der fertigen Datenbank
(`sqlite3 poc/measure/out/m2.sqlite < poc/measure/stats.sql`):

| Tabelle / Kennzahl | Zeilen |
|---|---:|
| `name` | 1.437.761 |
| `taxon_concept` | 440.098 |
| `concept_name` role=accepted | 440.098 |
| `concept_name` role=synonym | 953.262 |
| verwaiste Synonyme (Ziel nie ingestiert) | 44.401 |
| `distribution` | 1.982.550 |
| distinkte WGSRPD-L3-Codes | 381 |
| `xref` (POWO) | 440.098 |
| `fts_name_map` (FTS5-Zeilen) | 1.393.360 |
| `trait_value` | 113.544 |

`trait_value` (113.544) liegt unter der Summe der `matched`-Zeilen aus M2.1
(54.557 + 36.554 + 24.985 = 116.096): der Primärschlüssel ist
`(concept_id, vocab, vocab_version, dim)`, also kollabieren 2.552 Zeilen,
in denen zwei verschiedene Trait-Namen auf dasselbe WCVP-Konzept und
dieselbe Dimension abbilden — der zuletzt geschriebene Wert gewinnt.
Ebenso liegen die 1.982.550 `distribution`-Zeilen unter den 1.995.338 des
Archivs: Distributionen von Zeilen, die kein akzeptiertes Konzept bekommen
haben, entfallen.

Konzepte je Rang: SPECIES 368.928 · SUBSPECIES 27.942 · VARIETY 25.727 ·
GENUS 16.868 · FORM 633. Namen je Rang: SPECIES 1.048.616 ·
VARIETY 229.446 · SUBSPECIES 73.948 · FORM 43.609 · GENUS 42.142.

**Zwei-Phasen-Maps im Speicher:** sie überleben 1,44 Mio. Namen — 2,97 GiB
Peak-RSS, kein Swapping (`swaps 0`, `page faults 135`). Das ist die
Antwort auf die Frage aus dem Plan: nicht der Speicher ist das Problem,
sondern die FK-Prüfkosten.

**Verdikt: hält nicht — mit bekannter, billiger Reparatur.** Der Serienstand
ist an echten WCVP-Volldaten **nicht einsatzfähig**: der Ingest bricht
entweder nach 5,37 s hart ab (unbekannter Rang, 11.223/1.448.984 Zeilen,
0,775 %) oder läuft quadratisch und wurde nach 22 min 48 s ohne einen
einzigen committeten Datensatz manuell abgebrochen. Beides sind
Code-Defekte, keine Dateneigenschaften, und beide sind mit dem Messwerkzeug
selbst widerlegt: acht zusätzliche FK-Indizes drücken denselben Volldatensatz
auf 276,70 s (Faktor 4,9 schneller als schon der abgebrochene Lauf bei
weniger Daten), bei einem Speicher-Overhead von nur ~18 % DB-Größe. Speicher
(2,97 GiB Peak-RSS) ist nachweislich **nicht** der begrenzende Faktor.

---

## Nach Hardening (Task 3): M1'–M3' am echten, unveränderten WCVP-Volldatensatz

> Diese Sektion misst dieselben drei Kennzahlen wie M1/M2/M4 oben — jetzt mit
> den Hardening-Fixes aus Task 1 (`ParseRankLenient`, `internal/domain/taxon.go`)
> und Task 2 (acht FK-Indizes fest in `internal/adapters/sqlite/schema.sql`) —
> gegen das **unveränderte** WCVP-DwC-A-Archiv (`poc/data/wcvp_dwca/`,
> 1.448.984 Taxon-Zeilen, **kein** Rangfilter, **keine** Ad-hoc-Indizes). Die
> Baseline-Zahlen oben bleiben unverändert stehen; hier steht nur der
> Vergleich. Verdikte liefert Task 6, nicht dieser Abschnitt.

Manifest: `poc/measure/dataset.full-real.yaml` (identisch zu
`dataset.full.yaml`, außer `backbones[0].path` zeigt auf
`poc/data/wcvp_dwca` statt auf die gefilterte Kopie). Binary:
`nix develop -c make build` (`hostus` v0.2.4-87-g3137c95-dirty).

### M1' — Voller WCVP-Ingest, Serienschema, keine Ad-hoc-Indizes

```bash
rm -f poc/measure/out/m1real.sqlite*
/usr/bin/time -l ./hostus ingest \
  --dataset poc/measure/dataset.full-real.yaml --db poc/measure/out/m1real.sqlite
```

| Kennzahl | Baseline (M1.0, unverändertes Archiv) | **Nach Hardening (M1')** |
|---|---|---|
| Ergebnis | **bricht ab** nach 5,37 s (`unknown taxon rank "proles"`) | **läuft durch** |
| Wall-Clock | — | **281,27 s = 4 min 41 s** (user 158,75 s / sys 132,16 s) |
| Peak RSS (`maximum resident set size`) | 2.670.788.608 Byte (2,49 GiB, beim Abbruch) | **3.001.942.016 Byte = 2,80 GiB** |
| `peak memory footprint` | — | 3.248.835.488 Byte |
| DB-Datei danach | 122.880 Byte (nur Schema, nichts committed) | **960.491.520 Byte = 916 MiB** |

Zum Vergleich mit M1.3 (dem gefilterten Volldatensatz + Ad-hoc-`fk_indexes.sql`,
276,70 s / 2,97 GiB / 908 MiB): **das unveränderte Archiv, ohne Rangfilter und
ohne Ad-hoc-Indizes, ist mit dem gehärteten Serienschema genauso schnell**
(281,27 s vs. 276,70 s — die Differenz liegt in der Größenordnung der
Lauf-zu-Lauf-Varianz, nicht in einem strukturellen Unterschied) und braucht
keinen manuellen Zwischenschritt (`hostus bundle` + `fk_indexes.sql`) mehr;
`hostus ingest` legt das indizierte Schema jetzt selbst an.

Ingest-Report (`poc/measure/out/m1real-ingest.log`):

```
Ingest complete:
  wcvp: names=1448984 concepts=440534 synonyms=964762 orphaned=43688
    ranks: other=6527 ((empty) 2744, proles 2351, lusus 660, microgène 371, Convariety 184, monstr. 90, grex 41, subproles 18, stirps 17, provar. 16, modif. 6, psp. 6, mut. 5, sublusus 4, subap. 3, subspecioid 2, subsubsp. 2, agamosp. 1, ecas. 1, group 1)
```

Zeilenzahlen der fertigen Datenbank (`sqlite3 poc/measure/out/m1real.sqlite < poc/measure/stats.sql`,
Auszug — vollständig in `poc/measure/out/m1real-stats.txt`):

| Tabelle / Kennzahl | Baseline (M1.3, gefiltert) | **Nach Hardening (M1', unveränderter Archiv)** | Differenz |
|---|---:|---:|---:|
| `name` | 1.437.761 | **1.448.984** | +11.223 |
| `taxon_concept` | 440.098 | **440.534** | +436 |
| `concept_name` role=synonym | 953.262 | **964.762** | +11.500 |
| verwaiste Synonyme | 44.401 | **43.688** | −713 |
| `distribution` | 1.982.550 | **1.983.859** | +1.309 |
| `xref` (POWO) | 440.098 | **440.534** | +436 |
| `fts_name_map` | 1.393.360 | **1.405.296** | +11.936 |
| `trait_value` | 113.544 | **113.594** | +50 |

**Warum mehr Konzepte/Namen als die Baseline:** M1.3 lief gegen die *gefilterte*
Kopie des Archivs (`poc/measure/filter_ranks.sh` hatte 11.223 Zeilen mit
unbekanntem Rang vorab entfernt, siehe M1.0). Dieser Lauf (M1') geht gegen das
**unveränderte** Archiv — `ParseRankLenient` faltet die 11.223 vorher
ausgeschlossenen Zeilen jetzt auf sechs Ziele: die drei dedizierten
Nothotaxon-Ränge (`NOTHOSUBSPECIES` 552, `NOTHOVARIETY` 134, `NOTHOFORM` 15
Namen), `SUBVARIETY` (3.350 Namen) und den Rest (6.527 Namen) auf `RankOther`
mit `rank_verbatim` persistiert (28 verschiedene Rohschreibweisen, darunter
die leere Zeichenkette; siehe die `ranks: other=`-Zeile oben). Von den 11.223
zusätzlichen Namen werden **436 zu eigenen `taxon_concept`-Zeilen** (die
akzeptierten Vertreter dieser Ränge: `NOTHOSUBSPECIES` 370, `NOTHOVARIETY` 54,
`SUBVARIETY` 8, `NOTHOFORM` 4 Konzepte — Summe 436, exakt die Differenz
440.534 − 440.098), der Rest sind zusätzliche Synonym-Namen auf bereits
vorhandene oder neue Konzepte. Das ist genau der in der Aufgabenstellung
erwartete Effekt: T1 admittiert mehr Zeilen, statt den Ingest abzubrechen.

### M2' — Trait-Crosswalk gegen die volle Datenbank

Ingest von EIVE + Tichý + Midolo lief im selben Aufruf wie M1' (ein
Manifest, `poc/measure/dataset.full-real.yaml`, ein `hostus ingest`-Lauf).

**Pro Vokabular, auf Zeilenebene** (aus dem Ingest-Report):

!!! note "Nachtrag 2026-08-23: die `ambiguous`-Spalte ist überholt"

    Die 11–18 % `ambiguous` waren zu 99,4 % **Homonyme** — dieselbe Schreibung,
    zweimal veröffentlicht, verschiedene Autorschaft, und nur ein Konzept trägt
    den Namen wirklich. Der Crosswalk wendet inzwischen denselben gestuften
    Tie-Break an wie der Match-Pfad (`accepted`, dann `homotypic`); die
    `ambiguous`-Zahlen liegen dadurch bei 323 / 206 / 170 statt bei
    8.961 / 7.373 / 5.930. Die Tabellen hier bleiben als Messprotokoll des
    damaligen Stands stehen.

| Vokabular | Zeilen | matched | unmatched | ambiguous |
|---|---:|---:|---:|---:|
| `eive` | 71.266 | 54.612 (**76,64 %**) | 8.830 (12,39 %) | 7.824 (10,98 %) |
| `tichy2023` | 45.592 | 36.554 (**80,17 %**) | 1.868 (4,10 %) | 7.170 (15,73 %) |
| `midolo2023` | 31.910 | 24.985 (**78,30 %**) | 1.145 (3,59 %) | 5.780 (18,11 %) |

Gegen die Baseline (M2.1: 76,55/80,18/78,30 %) praktisch unverändert — die
matched-Quote schwankt nur bei EIVE spürbar (+0,09 Prozentpunkte, +55
matched-Zeilen: 54.612 statt 54.557), weil mehr Konzepte jetzt existieren, an
die ein EIVE-Name binden kann. `unmatched`/`ambiguous` sind bei Tichý/Midolo
zeilenidentisch mit der Baseline; bei EIVE sinkt `unmatched` von 8.885 auf
8.830 (−55, exakt die zusätzlichen `matched`-Zeilen).

**Pro Vokabular, auf Taxon-Ebene** (`poc/measure/bridge`, Aufruf wie M2.2,
gegen `poc/measure/out/m1real.sqlite`; Datei: `poc/measure/out/m1real-bridge.txt`):

| Vokabular | distinkte Taxa | in WCVP auflösbar — Baseline (M2.2) | **in WCVP auflösbar — nach Hardening** |
|---|---:|---:|---:|
| EIVE | 14.830 | 13.015 (87,76 %) | **13.026 (87,84 %)** |
| Tichý | 8.907 | 8.527 (95,73 %) | **8.527 (95,73 %)** |
| Midolo | 6.382 | 6.153 (96,41 %) | **6.153 (96,41 %)** |

EIVE gewinnt 11 zusätzliche auflösbare Taxa (+0,08 Prozentpunkte) — dieselbe
Größenordnung wie der Zeilenebene-Effekt oben; Tichý und Midolo sind
zeilenidentisch mit der Baseline (die zusätzlichen Ränge/Konzepte aus M1'
liegen offenbar außerhalb ihrer Namensmengen). Das bestätigt die Erwartung aus
dem Auftrag: die T1-Reparatur admittiert mehr Zeilen und kann die
Trefferquote nur verbessern oder gleich lassen, nie verschlechtern — beides
eingetreten, aber der Effekt ist klein, weil die vorher exkludierten
28 Rang-Schreibweisen ein Randphänomen sind (0,775 % aller WCVP-Zeilen).

**Trait-Abdeckung der Konzepte** (`sqlite3 poc/measure/out/m1real.sqlite < poc/measure/stats.sql`,
unabhängig gegengeprüft gegen eine zweite Ingest-Kopie unter `/tmp/full-real.sqlite`
— identische Werte):

| Kennzahl | Baseline (M2.3) | **Nach Hardening** |
|---|---:|---:|
| WCVP-Konzepte insgesamt | 440.098 | **440.534** |
| … mit irgendeinem Trait-Wert | 11.638 (2,64 %) | **11.648 (2,64 %)**|
| … mit EIVE | 10.990 | **11.000** |
| … mit Tichý | 7.072 | **7.072** |
| … mit Midolo | 4.963 | **4.963** |
| … mit EIVE UND Tichý | 6.671 | **6.671** |
| … mit allen dreien | 4.251 | **4.251** |

Unverändert bis auf EIVE (+10 Konzepte, +10 matched-Zeilen abzüglich
Kollisionen auf denselben `(concept_id, vocab, dim)`-Schlüssel) — konsistent
mit dem oben gemessenen kleinen EIVE-Zugewinn.

**Stichprobe der Nichttreffer** (aus dem Ingest-Report, nicht neu gezogen —
das ist die vollständige `unmatched sample`-Ausgabe, keine k-te-Zeile-Probe
wie M2.4): die Nichttreffer bestätigen exakt die drei M2.4-Kategorien.
Aggregate: `Acer opalus aggr.`, `Achillea millefolium aggr.`,
`Aconitum napellus aggr.`, `Alchemilla vulgaris aggr.`. Hybride:
`Abies alba × nordmanniana`, `Acer ×coriaceum`, `Aconitum ×schneebergense`,
`Aconogonon ×fennicum`. Infraspezifische Autonyme:
`Acer obtusatum subsp. obtusatum`, `Aconitum lycoctonum subsp. lycoctonum`.

### M3' — Suggest-Latenz auf dem vollen Index

Gleiches Verfahren wie M4 (Baseline): HTTP gegen einen laufenden Server
(`hostus serve`) über die volle `m1real.sqlite`, `poc/measure/out/latency`
(hier `latency-real`, identisches Binary aus `poc/measure/latency`),
`--reps 15 --warmup 3`, 38 Präfixe, `limit=10` — 570 Messpunkte je Lauf,
100 ms Pause zwischen Requests wegen des 20-req/s-Rate-Limits.

```bash
HOSTUS_SQLITE_PATH=poc/measure/out/m1real.sqlite ./hostus serve --port 8097 --log-level warn &
./poc/measure/out/latency-real --base http://127.0.0.1:8097 --reps 15 --warmup 3
./poc/measure/out/latency-real --base http://127.0.0.1:8097 --reps 15 --warmup 3 --area GER
```

| Lauf | Baseline (M4) p50 | **Nach Hardening p50** | Baseline p95 | **Nach Hardening p95** |
|---|---:|---:|---:|---:|
| ohne `area` | 36,4 ms | **38,59 ms** | 220,2 ms | **274,37 ms** |
| mit `area=GER` | 38,7 ms | **40,81 ms** | 253,8 ms | **321,57 ms** |

Vollständige Tabellen: `poc/measure/out/m1real-latency-noarea.txt` und
`poc/measure/out/m1real-latency-ger.txt`. p50 bleibt im selben Bereich
(+2–2 ms); **p95 steigt deutlich — um 25 % ohne `area` (220,2 → 274,37 ms)
und 27 % mit `area=GER` (253,8 → 321,57 ms).** Das ist die einzige der drei
Kennzahlen (M1'/M2'/M3'), die sich gegenüber der Baseline sichtbar
verschlechtert, und die Ursache ist **nicht geklärt** — das war mit den
Mitteln dieser Aufgabe nicht isolierbar:

- **Die Größenordnungen passen nicht zusammen.** DB +0,9 % (916 vs. 908 MiB),
  Konzepte +0,1 %, Namen +0,8 % — keiner dieser Zuwächse erklärt plausibel
  einen 25–27 % höheren p95.
- **Ein zweiter Lauf derselben Messung** (`poc/measure/out/m1real-latency-noarea-rep2.txt`,
  identischer Code, identische DB, gleicher Rechner, sofort im Anschluss
  wiederholt) ergab p50=40,73 ms / p95=301,89 ms gegenüber dem ersten Lauf
  (p50=38,59 ms / p95=274,37 ms) — eine Schwankung von rund 10 % allein durch
  Wiederholung, ohne jede Codeänderung. Das zeigt, dass ein Teil der 25–27 %
  Differenz zur Baseline durch bloße Lauf-zu-Lauf-Varianz erklärbar sein
  könnte — aber eben nur ein Teil, nicht die volle Differenz, und für die
  Baseline selbst existiert **kein** Wiederholungslauf, mit dem sich deren
  eigene Varianz einordnen ließe. Die Aussage „Messvarianz" ist damit
  plausibilisiert, aber nicht quantitativ belegt.
- **Eine naheliegende Alternativursache wurde nicht geprüft:** die acht neuen
  FK-Indizes aus Task 2 verändern, welche Pläne SQLites Query-Planer wählt.
  Der Suggest-Pfad kombiniert FTS5-`MATCH` mit Joins und einer
  `MATERIALIZED`-CTE (`internal/adapters/sqlite/suggest.go`); ein Planer, der
  jetzt einen Index-Pfad dort bevorzugt, wo vorher ein Scan günstiger war,
  ist ein plausibler Kandidat für einen echten, auf den Hardening-Fix
  zurückgehenden Effekt — und wurde hier nicht per `EXPLAIN QUERY PLAN`
  gegen eine ungeindizierte Vergleichs-DB verifiziert oder ausgeschlossen.

**Offene Kandidatenursachen für Task 6** (keine davon ist hier belegt oder
ausgeschlossen): (a) Query-Plan-Änderung durch die neuen FK-Indizes, (b)
FTS/Join-Kosten durch die zusätzlichen, von T1 admittierten OTHER-Rang-Zeilen,
(c) unquantifizierte Maschinen-Varianz (siehe Wiederholungslauf oben).
Dieselben kurzen 2-Zeichen-Präfixe (`ca`, `al`, `sa`) dominieren den p95 wie
in M4 — das ist die einzige Konstante zwischen Baseline und dieser Messung.
Diese Sektion liefert bewusst **kein Verdikt** zu dieser Abweichung (siehe
Präambel oben); das ist Aufgabe von Task 6.

> **Nachtrag (Task 7):** aufgelöst — siehe „Task 7: die offene
> p95-Abweichung — aufgelöst" weiter unten. Es gibt keine Regression: der
> p95 dieses Aufbaus streut über 19 Wiederholungsläufe von 225 bis 316 ms,
> und die Baseline-Konfiguration (Code vor Hardening, Baseline-DB) misst
> heute 262–310 ms statt der hier verglichenen 220,2 ms. Kandidat (a) und
> (b) sind belegt ausgeschlossen, (c) ist quantifiziert.

---

## Nach Hardening (Task 5): deterministische Namensnormalisierung

> Diese Sektion misst die Wirkung von Task 5
> (`internal/domain/normalize.go`, verdrahtet in
> `application.IngestTraits`) auf **genau die Kennzahl aus M2'/M2.2**.
> Bezugsgröße ist durchgängig die Task-3-Messung (M2'), nicht die
> Ur-Baseline M2.1. Die Baseline-Zahlen oben bleiben unverändert stehen.
> Verdikte liefert Task 6.
>
> Anlass ist der Befund aus M2.4: die Nichttreffer sind überwiegend
> **strukturell**. Task 5 hat das nicht an einer 20er-Stichprobe, sondern an
> den **vollständigen** Nichttrefferlisten
> (`poc/measure/out/unmatched-{eive,tichy,midolo}.txt`, 1.804 / 380 / 229
> Taxa) nachgezählt: Aggregatmarker 664 / 229 / 134 Namen, Hybridmarker
> 452 / 136 / 83, infraspezifische Autonyme 356 / 0 / 5.

### Was gemessen wurde und womit

Zwei unabhängige Wege, die zueinander passen müssen:

1. **Regel-Sonde** (`poc/measure/bridge --norm`, Quelle `poc/measure/bridge/norm.go`).
   Reine Namensauflösung gegen die fertige M1'-Datenbank, Sekunden statt
   Minuten — dadurch lässt sich **jede Regel einzeln** ein- und ausschalten.
   Die Sonde ist keine Nachbildung: `Canonicalize` und `NameCandidates`
   werden von `poc/measure/gen_canonicalize.sh` bzw.
   `poc/measure/gen_normalize.sh` **zeilengenau** aus
   `internal/domain/` kopiert (`--check` verifiziert die Kopie), der Index
   ist `canonical_fold → COUNT(DISTINCT concept_id)` über
   `name JOIN concept_name` — genau das, was `sqlite.MatchExact` auflöst —
   und die Klassifikation ist die von `application.resolveTraitName`.
   **Gültigkeitsnachweis:** die `exact`-Zeile der Sonde reproduziert die
   M2'-Zahlen zeilengenau (EIVE 54.612 / 8.830 / 7.824, Tichý
   36.554 / 1.868 / 7.170, Midolo 24.985 / 1.145 / 5.780) und die
   M2.2-Taxonzahlen exakt (13.026 / 8.527 / 6.153). Ohne diese Deckung
   wären die Deltas darunter wertlos.
2. **Voller Ingest** (`run.sh t5ingest`), derselbe Lauf wie M1'/M2', nur mit
   der Normalisierung — die Gegenprobe im echten Codepfad.

```bash
nix develop -c bash poc/measure/run.sh t5          # Regel-Sonde, ~8 s
nix develop -c bash poc/measure/run.sh t5ingest    # voller Ingest, 277 s
```

Binary: `nix develop -c make build` (`hostus` v0.2.4-91-g2a8c270-dirty).
Rohdaten: `poc/measure/out/t5-norm.txt`, `poc/measure/out/t5-ingest.log`,
`poc/measure/out/t5-stats.txt`.

**Die beiden Wege stimmen überein:** der volle Ingest liefert für alle drei
Vokabulare exakt die `ALL RULES`-Zeilen der Sonde.

### T5.1 Marginaler Zugewinn je Regel (Taxon-Ebene, jede Regel allein)

Jede Zeile ist ein eigener Lauf: nur der exakte Schlüssel **plus diese eine
Regel**. „+matched" sind eindeutig aufgelöste Taxa, „+ambig" die zusätzlich
mehrdeutig gewordenen (beides kam vorher aus `unmatched`; eine Regel kann
kein vorher aufgelöstes Taxon umlenken, weil der exakte Schlüssel immer
zuerst probiert wird).

| Regel | EIVE +matched | EIVE +ambig | Tichý +matched | Tichý +ambig | Midolo +matched | Midolo +ambig |
|---|---:|---:|---:|---:|---:|---:|
| `hybrid_spacing` (`Acer ×coriaceum` → `acer × coriaceum`) | **+360** | +1 | 0 | 0 | +11 | 0 |
| `hybrid_marker_dropped` (`Anacamptis ×albertii` → `anacamptis albertii`) | +43 | +5 | 0 | 0 | +2 | 0 |
| `hybrid_marker_added` (`Abies borisii-regis` → `abies × borisii-regis`) | +51 | 0 | +32 | 0 | +16 | 0 |
| `aggregate` (nur echte Aggregatkonzepte) | **0** | 0 | **0** | 0 | **0** | 0 |
| `aggregate_to_nominate` (`Acer opalus aggr.` → `acer opalus`) ⚑ | **+554** | +158 | **+186** | +44 | **+106** | +27 |
| `autonym` (`Acer obtusatum subsp. obtusatum` → `acer obtusatum`) ⚑ | **+277** | +72 | 0 | 0 | +1 | +3 |
| `orthography_genitive` (`Cardamine plumierii` → `cardamine plumieri`) | +17 | 0 | +13 | 0 | +8 | 0 |
| **alle Regeln zusammen** | **+1.270** | **+231** | **+231** | **+44** | **+142** | **+30** |

⚑ = botanische Ermessensentscheidung, im Ingest-Report als `flagged`
ausgewiesen (Einordnung/Begründung in T5.4; der Walk-back — warum die
per-Regel-Zahlen dieser Tabelle NAMENSAUFLÖSUNG messen, nicht gespeicherte
Zeilen, und wie sich das unterscheidet — steht in T5.5).

Die Summe der Einzelregeln (EIVE 1.302, Midolo 144) liegt über dem
Gesamtwert (1.270 / 142), weil sich einige Namen über mehr als eine Regel
erreichen lassen; Tichý ist überschneidungsfrei (32 + 186 + 13 = 231).

**Der Nullbefund gehört dazu:** die Regel `aggregate` — die ein
*tatsächliches* Aggregatkonzept sucht, statt auf die Nominatart
auszuweichen — bringt in allen drei Vokabularen **exakt null** Treffer. Der
Grund ist nachgemessen und nicht vermutet:

```bash
sqlite3 poc/measure/out/m1real.sqlite \
  "SELECT count(*) FROM name WHERE canonical_fold LIKE '% agg.%' OR canonical_fold LIKE '% aggr.%' OR canonical_fold LIKE '%s.l.%'"
# 0
```

WCVP führt **keinen einzigen** aggregatmarkierten Namen. Ohne den Rückfall
auf die Nominatart ist bei Aggregaten also nichts zu holen — genau das macht
den Rückfall zur Entscheidung und nicht zur Formalie.

### T5.2 Zeilenebene, kumulativ (voller Ingest, Gegenprobe)

| Vokabular | Zeilen | matched (M2') | **matched (T5)** | unmatched (M2') | **unmatched (T5)** | ambiguous (M2') | **ambiguous (T5)** |
|---|---:|---:|---:|---:|---:|---:|---:|
| `eive` | 71.266 | 54.612 (76,64 %) | **60.860 (85,40 %)** | 8.830 | **1.445** (−83,6 %) | 7.824 | **8.961** (+1.137) |
| `tichy2023` | 45.592 | 36.554 (80,17 %) | **37.693 (82,68 %)** | 1.868 | **526** (−71,8 %) | 7.170 | **7.373** (+203) |
| `midolo2023` | 31.910 | 24.985 (78,30 %) | **25.695 (80,52 %)** | 1.145 | **285** (−75,1 %) | 5.780 | **5.930** (+150) |

### T5.3 Taxon-Ebene (die Bezugsgröße aus M2.2)

M2.2 zählt „in WCVP auflösbar" = der Name findet im Index **überhaupt** ein
Konzept (eindeutig **oder** mehrdeutig). Beide Lesarten stehen hier, damit
der Zugewinn nicht durch die Wahl der Kennzahl geschönt wird:

| Vokabular | distinkte Taxa | auflösbar M2' | **auflösbar T5** | *eindeutig* M2' | ***eindeutig* T5** |
|---|---:|---:|---:|---:|---:|
| EIVE | 14.830 | 13.026 (87,84 %) | **14.527 (97,96 %)** | 11.420 (77,01 %) | **12.690 (85,57 %)** |
| Tichý | 8.907 | 8.527 (95,73 %) | **8.802 (98,82 %)** | 7.162 (80,41 %) | **7.393 (83,00 %)** |
| Midolo | 6.382 | 6.153 (96,41 %) | **6.325 (99,11 %)** | 4.997 (78,30 %) | **5.139 (80,52 %)** |

Zum Vergleich der Größenordnungen: die **gesamte** Lizenzroute (M3,
Vereinigung aller vier Brückenlisten, Obergrenze) erreichte für EIVE
+6,09 %, und M6 zeigte, dass davon real ~0,34 % ankommen. Die
Normalisierung bringt für EIVE **+10,12 Prozentpunkte auflösbar**
(87,84 → 97,96 %) — rund eine Größenordnung mehr, ohne Lizenzgespräch.

**Trait-Abdeckung der Konzepte** (`poc/measure/out/t5-stats.txt`):

| Kennzahl | M2' | **T5** |
|---|---:|---:|
| Konzepte mit irgendeinem Trait-Wert | 11.648 | **12.080** |
| … mit EIVE | 11.000 | **11.426** |
| … mit Tichý | 7.072 | **7.215** |
| … mit Midolo | 4.963 | **5.104** |
| … mit EIVE UND Tichý | 6.671 | **6.810** |
| … mit allen dreien | 4.251 | **4.373** |

**Laufzeit:** 277,26 s gegenüber 281,27 s in M1' — die Kandidatenleiter
kostet **nichts** Messbares, weil sie nur bei Namen betreten wird, die der
exakte Schlüssel ohnehin verloren hätte. (Das ist die Laufzeit VOR dem
Fix-Runde-1-Fix aus T5.5; die aktuelle Zahl NACH diesem Fix steht dort:
285,80 s — ebenfalls nicht messbar teurer als M1'.)

### T5.4 Was die Zahlen kosten: Mehrdeutigkeit und zwei Ermessensfälle

**Die Mehrdeutigkeit steigt** — das ist keine Nebenwirkung, die man
verschweigen darf: EIVE +231 Taxa (1.606 → 1.837), Tichý +44
(1.365 → 1.409), Midolo +30 (1.156 → 1.186); auf Zeilenebene
+1.137 / +203 / +150. **Jedes** dieser Taxa war vorher `unmatched`; kein
vorher aufgelöstes Taxon ist mehrdeutig geworden, weil `NameCandidates` den
unveränderten `Canonicalize`-Schlüssel immer zuerst probiert und die
Leiter beim ersten antwortenden Schlüssel abbricht. Mehrdeutig heißt
weiterhin: **nichts** wird geschrieben, es wird nicht geraten. Der Löwenanteil
stammt aus dem Aggregat-Rückfall (EIVE 158 von 231).

Zwei Regeln setzen zwei Umgrenzungen gleich, die nicht identisch sind. Sie
sind deshalb `Flagged` (`domain.NormalizationRule.Flagged`), werden im
Ingest-Report getrennt gezählt und namentlich bemustert:

```
  eive: rows=71266 matched=60860 unmatched=1445 ambiguous=8961
    normalized aggregate_to_nominate: rows=30 taxa=6 [flagged: circumscriptions equated, not identical]
    normalized autonym: rows=18 taxa=4 [flagged: circumscriptions equated, not identical]
    normalized hybrid_marker_added: rows=214 taxa=44
    normalized hybrid_marker_dropped: rows=20 taxa=4
    normalized hybrid_spacing: rows=1720 taxa=353
    normalized orthography_genitive: rows=80 taxa=16
    flagged sample: Alchemilla conjuncta aggr., Alchemilla fissa aggr., …
```

Diese Zahlen zählen **gespeicherte** Zeilen, nicht aufgelöste — der
Unterschied ist der Kern von T5.5. Und seit Fix-Runde 1 lebt die Markierung
nicht nur hier, sondern **in den Daten**: `trait_value.resolution`
(NULL = exakter Treffer) wird persistiert, über
`GET /v1/concept/{id}/traits` als `resolution` ausgeliefert (fehlt das Feld,
war es ein exakter Treffer) und ins Offline-Bundle mitkopiert. Erst damit
kann ein Konsument die beiden Ermessensfälle wirklich ausschließen — die auf
20 Namen gedeckelte `flagged sample` konnte das nie:

```sql
-- alles, was auf einer gleichgesetzten Umgrenzung beruht:
SELECT * FROM trait_value WHERE resolution IN ('aggregate_to_nominate','autonym');
```

- **Autonym → Art.** Ein Autonym ist die Nominat-Unterart und damit *enger*
  als die Art. Der Grund, warum es hier überhaupt unaufgelöst ankommt, ist
  aber gerade, dass das Rückgrat die infraspezifische Gliederung **nicht
  führt**: WCVP hat keine Zeile `Acer obtusatum subsp. obtusatum`, nur die
  Art. In dieser Umgrenzung *ist* die Art das Taxon, das die Quelle
  „Autonym" nennt. Führte das Rückgrat die Gliederung, löste der exakte
  Schlüssel auf und die Regel käme nie zum Zug. Gleichgesetzt — aber
  markiert, weil das Argument an diesem Rückgrat hängt, nicht allgemein gilt.
- **Aggregat → Nominatart.** Das ist der schwächere Fall und zeigt in die
  *andere* Richtung: das Aggregat ist **weiter** als die Nominatart
  (`Acer opalus aggr.` umfasst auch *A. obtusatum* u. a.), ein
  Aggregat-Mittelwert landet also auf einem einzelnen Mitglied. Angewandt
  wird er trotzdem, weil die gemessene Alternative schlechter ist (siehe
  T5.1: WCVP führt null Aggregatnamen, ein Verzicht verwirft 664 / 229 / 134
  Vokabular-Taxa ersatzlos) und weil die Nominatart das Taxon ist, das ein
  Nutzer beim Suchen nach dem Aggregat tatsächlich nachschlägt. Markiert,
  damit ein Konsument diese Werte ausschließen kann.

**Erledigt in Fix-Runde 1** (war zuvor als offene Auflage vermerkt): die
Markierung steht jetzt in den Daten — `trait_value.resolution`, nullable,
NULL = exakter Treffer. Der Weg dorthin ist derselbe, den Task 2 für
`rank_verbatim` gegangen ist (Spalte in `schema.sql`, idempotent bei `Open`
angelegt, Ingest baut neu auf): Adapter-Schreib-/Lesepfad, `ExportBundle`,
DTO (`omitempty`), OpenAPI und `docs/reference/http-api.md`. Ein
Migrationsmechanismus war dafür nicht nötig.

### T5.5 Ein Nebenbefund, den erst die Sichtbarmachung zutage gefördert hat

Die Spalte `trait_value.resolution` hat sofort ein Problem aufgedeckt, das
der reine Report-Zähler verdeckt hatte: der Crosswalk ist **viele-zu-eins**,
der Primärschlüssel von `trait_value` aber
`(concept_id, vocab, vocab_version, dim)`. EIVE führt sowohl `Acer opalus`
als auch `Acer opalus aggr.`; beide landen auf demselben Konzept und
derselben Dimension — und `AddTraitValue` ist ein `INSERT OR REPLACE`. Wer
den Platz behielt, entschied damit schlicht die **Zeilenreihenfolge der
CSV**.

Das ist doppelt schlecht: ein exakt getroffener Wert konnte still durch das
Kollektivmittel eines Aggregats ersetzt werden, und die neue
`resolution`-Markierung hätte den Platz dann nach Zeilenreihenfolge
beschrieben statt nach dem, was für ihn zutrifft. Eine Markierung, deren
Wert von der Eingabereihenfolge abhängt, taugt nicht zum Filtern.

Gemessen, wie oft das passierte (Ingest-Lauf vor dem Fix gegen den nach dem
Fix, jeweils
`SELECT resolution, COUNT(DISTINCT concept_id) FROM trait_value GROUP BY 1`):
die Zahl der EIVE-Konzepte mit exakt aufgelöstem Wert steigt von 10.354 auf
11.000 — **646 EIVE-Konzepte** trugen also einen normalisierten Wert,
obwohl für dieselbe Dimension ein exakter Treffer vorlag. Bei Tichý waren es
65 (7.007 → 7.072), bei Midolo null.

`application.selectTraitWinners` entscheidet das jetzt explizit und
reihenfolgeunabhängig: **ein exakter Treffer schlägt immer einen
normalisierten**; unter Gleichrangigen gewinnt die erste Zeile. Wirkung auf
die gespeicherten Daten:

| gespeicherte Konzepte je Vokabular | vor dem Fix | **nach dem Fix** | M2'-Baseline |
|---|---:|---:|---:|
| EIVE, exakt aufgelöst | 10.354 | **11.000** | 11.000 |
| EIVE, `aggregate_to_nominate` | 455 | **6** | — |
| EIVE, `autonym` | 221 | **4** | — |
| Tichý, exakt aufgelöst | 7.007 | **7.072** | 7.072 |
| Midolo, exakt aufgelöst | 4.963 | **4.963** | 4.963 |

Daraus folgt eine prüfbare Regressionsaussage, die vorher nicht galt: **die
Zahl der exakt aufgelösten Konzepte je Vokabular ist nach der
Normalisierung exakt die der M2'-Baseline** (11.000 / 7.072 / 4.963). Die
Normalisierung ist damit auf Konzeptebene beweisbar rein additiv — sie hat
keinen einzigen Baseline-Wert verdrängt. Die Gesamtabdeckung
(`concepts_with_eive` = 11.426 usw.) ist unverändert; es ändert sich nur,
**welcher** Wert einen umkämpften Platz besetzt.

Zweite Folge, ebenfalls in T5.4 sichtbar: die per-Regel-Zahlen des
Ingest-Reports zählen jetzt **gespeicherte** Zeilen und stimmen deshalb
zeilengenau mit `SELECT resolution, COUNT(*) FROM trait_value` überein. Für
EIVE fällt `aggregate_to_nominate` dadurch von 554 auf 6 Taxa — nicht weil
die Regel weniger Namen auflöst (die Auflösbarkeit in T5.1–T5.3 ist
unverändert), sondern weil fast alle diese Aggregatnamen auf Konzepte
zeigen, die **ohnehin schon** einen exakten EIVE-Wert tragen. Der
Abdeckungsgewinn der Aggregatregel ist bei EIVE also weit kleiner, als die
Namensauflösung allein vermuten ließ; bei Tichý (152 Konzepte) und Midolo
(105) bleibt er substanziell. Das ist eine Korrektur an der
Wertaussage dieser Regel, und sie gehört hierher, nicht in eine Fußnote.

**Laufzeit** mit Fix: 285,80 s (M1': 281,27 s) — die Vorauswahl kostet
nichts Messbares.

### T5.6 Orthographie: was aufgenommen wurde und was nicht

`domain.Canonicalize` wurde **nicht** angefasst. Es ist der *gespeicherte*
Schlüssel (`name.canonical_fold`) und paritätsgeprüft gegen SQLites
`unicode61 remove_diacritics 2` (`internal/adapters/sqlite/fts_parity_test.go`);
eine Verbreiterung dort bräche diesen Vertrag und veränderte stillschweigend
jeden gespeicherten Fold. Orthographie ist deshalb ein **zusätzlicher
Kandidatenschlüssel**, kein weiterer Fold.

Am gemessenen Rest (nach Aggregat/Hybrid/Autonym: 320 / 118 / 65 Taxa)
aufgenommen:

- **`-ii`/`-i`-Genitivalternation** (ICN Art. 60.8 / Rec. 60C), beide
  Richtungen: +17 / +13 / +8 Taxa. Belege: `Cardamine plumierii` ↔ WCVP
  `plumieri`, `Cota triumfettii` ↔ `triumfetti`, `Plantago cornutii` ↔
  `cornuti`, `Polygala edmundi` ↔ `edmundii`,
  `Crocus biflorus subsp. adamii` ↔ `adami`.

Bewusst **nicht** aufgenommen, jeweils mit gemessener Begründung:

- **Genus-Kongruenz des Epithetons** (ICN Art. 23.5, `arctostaphylos alpinus`
  ↔ WCVP `alpina`, `echinochloa colonum` ↔ `colona`): gemessener Zugewinn
  3 / 2 / 2 Taxa bei je **einem** zusätzlich mehrdeutigen Treffer. Das
  Umschreiben der Endung `-us`/`-a`/`-um` kann ein *anderes*, real
  existierendes Epitheton erzeugen und den Wert damit auf das falsche
  Konzept legen — ein schlechter Tausch für sieben Taxa insgesamt.
- **`-ae`/`-iae`-Alternation**: ein einziger Treffer. Zu wenig für eine Regel.
- **Angehängtes ASCII-`x` als Hybridmarker** (`acer xcoriaceum`): in den
  vollständigen Nichttrefferlisten aller drei Vokabulare **null** Vorkommen,
  während `x` ein legitimer Epithetonbuchstabe ist (`Rosa xanthina`,
  `Xanthium strumarium`). Gemessener Preis des Verzichts: null.
- **Echte Schreibfehler** (`Artemisia siversiana` ↔ WCVP `sieversiana`,
  `Paeonia broteroi` ↔ `broteri`): keine deterministischen Umschreibungen.
  Dafür existiert der Fuzzy-Pfad, der absichtlich `requires_review` liefert.

### T5.7 Was übrig bleibt

Der Rest ist substanziell, nicht mehr strukturell (`unmatched sample` aus
`poc/measure/out/t5-ingest.log`):

- **Binäre Hybridformeln** (25 EIVE-Taxa): `Abies alba × nordmanniana`,
  `Alchemilla glacialis × pentaphyllea`, `Alnus incana subsp. incana × viridis`.
  Eine Formel benennt kein Nothospecies-Konzept; es gibt keinen
  deterministischen Weg von ihr zu einem WCVP-Konzept. Bewusst kein
  erfundener Kandidat.
- **Nicht-nominate Unterarten ohne WCVP-Zeile**:
  `Allium circinatum subsp. peloponnesiacum` — korrekt **nicht** auf die Art
  zusammengelegt (eigener Testfall).
- **In WCVP wirklich nicht vorhanden**: `Acacia retinoides`,
  `Achillea styriaca`, `Acuston lunarioides`.
- **Schreibfehler** (Fuzzy-Territorium): `Artemisia siversiana`,
  `Alchemilla rhodondendrophila`, `Aconitum lycotonum subsp. vulparia`.

### T5.8 Reihenfolge der Kandidatenleiter (Fix-Runde 1)

`NameCandidates` dokumentiert, dass reine Schreibweisen-Regeln **vor** den
beiden Ermessensfällen probiert werden. Die Genitivregel stand jedoch
zuletzt, also HINTER Aggregat und Autonym — der Code widersprach seiner
eigenen Zusicherung. Folge: bei einem Autonym, dessen Epitheton die
`-ii`/`-i`-Alternation trägt, wäre der Name über den markierten
Autonym-Rückfall auf die Art zusammengefallen, auch wenn das Rückgrat das
**infraspezifische** Taxon unter der anderen Schreibweise führt — eine
unnötige und unnötig markierte Übertragung.

Behoben (Genitivblock jetzt vor Aggregat/Autonym) und **nachgemessen statt
angenommen**: die Zahlen in T5.1–T5.3 sind vor und nach dem Fix identisch.
Der Grund ist auszählbar — nur **19 EIVE-Taxa** (Tichý 0, Midolo 0) sind
überhaupt reihenfolgeempfindlich, also von beiden Regelklassen betroffen,
und für **keines** davon löst der Genitivschlüssel in WCVP auf:

```
# eive: 19 reihenfolgeempfindlich, davon 0 mit auflösendem Genitivschlüssel
# tichy: 0 / 0   midolo: 0 / 0
```

(Korrektur Task 6: dieser Zählwert stammt aus einem Ad-hoc-Skript, das nur
im Task-5-Report als Ausgabe zitiert wurde, nie als Datei existierte und
deshalb hier nicht mehr referenziert wird. Das dahinterstehende Invariant
— die Genitivregel steht in `NameCandidates` vor den beiden geflaggten
Regeln — ist seither durch einen eingecheckten Test gepinnt
(`internal/domain`, „genitive variant is offered before the flagged
autonym collapse"), nicht durch das Ad-hoc-Skript.)

Der Fix ist damit eine Korrektheitsreparatur an einer latenten Falle, kein
Abdeckungsgewinn — gemessener Delta: **0**. Ebenfalls in dieser Runde:
`s.str.` (sensu stricto) wurde aus den Aggregatmarkern **entfernt**. Es ist
kein Aggregatmarker, es VERENGT; es unter einer Regel namens
„aggregate to nominate species" zu strippen hätte die Begründung
falsch dargestellt. Kosten: null — alle drei Vokabulare enthalten null Taxa
mit dieser Schreibweise.

### T5.9 Regressionen

Keine. `MatchNames` (§B.2-Batch), das Suggest-Ranking und das
`requires_review`-Verhalten des Fuzzy-Pfads sind unverändert — Task 5 fasst
nur den Trait-Crosswalk an, und dort ist der erste Kandidat immer der
unveränderte `Canonicalize`-Schlüssel. Seit dem Fix aus T5.5 gilt zusätzlich
die stärkere, in den Daten prüfbare Aussage: die Zahl der **exakt**
aufgelösten Konzepte je Vokabular entspricht nach der Normalisierung exakt
der M2'-Baseline (11.000 / 7.072 / 4.963), die Normalisierung ist auf
Konzeptebene also beweisbar rein additiv.

`make verify`, `make test-integration`, `mkdocs --strict` und
`make mutation` für alle berührten Pakete sind grün. Mutation im Detail:
`internal/domain` 0 Überlebende in `normalize.go` bei 100 %
Mutator-Abdeckung; `internal/adapters/http` 0 Überlebende (100 % Effizienz);
`internal/application` und `internal/adapters/sqlite` unverändert gegenüber
dem Stand vor Task 5 — die verbleibenden Überlebenden liegen sämtlich in
nicht angefasstem Code bzw. sind die bereits dokumentierten beweisbaren
Äquivalente (Sortier-Komparator über eine Menge distinkter Regeln,
`sortedSample`-Cap).

---

## M2 — Crosswalk-Trefferquote (die Kernzahl)

### M2.1 Pro Vokabular, auf Zeilenebene

Direkt aus dem Report von `hostus ingest` (`poc/measure/out/m2-ingest.log`);
eine „Zeile" ist ein (Taxon, Dimension)-Wert:

| Vokabular | Zeilen | matched | unmatched | ambiguous |
|---|---:|---:|---:|---:|
| `eive` | 71.266 | 54.557 (**76,55 %**) | 8.885 (12,47 %) | 7.824 (10,98 %) |
| `tichy2023` | 45.592 | 36.554 (**80,18 %**) | 1.868 (4,10 %) | 7.170 (15,73 %) |
| `midolo2023` | 31.910 | 24.985 (**78,30 %**) | 1.145 (3,59 %) | 5.780 (18,11 %) |

`ambiguous` heißt: der Name existiert in WCVP, löst aber auf **mehrere
verschiedene** Konzepte auf (z. B. ein Homonym oder ein Name, der in zwei
Gattungen als Synonym geführt wird). `IngestTraits` rät dann nicht, sondern
verwirft die Zeile. Bei Midolo ist das mit 18,11 % der größere Verlustposten
als das Nichtfinden.

### M2.2 Pro Vokabular, auf Taxon-Ebene

Gemessen mit der Probe `poc/measure/bridge` gegen dieselbe Datenbank
(`nix develop -c bash poc/measure/run.sh m3`). Als „auflösbar" zählt exakt
das, was `sqlite.MatchExact` auflösen würde: ein `name`-Eintrag mit
passendem `canonical_fold`, der über `concept_name` an ein
`taxon_concept` gebunden ist (1.345.367 solcher Schlüssel in der DB).

| Vokabular | distinkte Taxa | in WCVP auflösbar | nicht auflösbar |
|---|---:|---:|---:|
| EIVE | 14.830 | 13.015 (**87,76 %**) | 1.815 (12,24 %) |
| Tichý | 8.907 | 8.527 (**95,73 %**) | 380 (4,27 %) |
| Midolo | 6.382 | 6.153 (**96,41 %**) | 229 (3,59 %) |

### M2.3 Namensraum-Überlappung: wie viele WCVP-Konzepte tragen Merkmale?

| Kennzahl | Konzepte |
|---|---:|
| WCVP-Konzepte insgesamt | 440.098 |
| … mit **irgendeinem** Trait-Wert | **11.638** (2,64 % aller Konzepte) |
| … mit EIVE | 10.990 |
| … mit Tichý | 7.072 |
| … mit Midolo | 4.963 |
| … mit **EIVE UND Tichý** | **6.671** |
| … mit allen dreien | 4.251 |
| Konzepte mit Vorkommen in `GER` | 11.514 |

Bezogen auf die Vokabulare selbst: von 10.990 EIVE-tragenden Konzepten
tragen 6.671 (60,7 %) auch Tichý; von 7.072 Tichý-tragenden tragen 6.671
(94,3 %) auch EIVE. Der Tichý-Namensraum ist also fast vollständig eine
Teilmenge des EIVE-Namensraums, sobald beide auf WCVP projiziert sind.

### M2.4 Warum die Nichttreffer misslingen (Stichprobe)

**Stichprobenverfahren:** aus der vollständigen, alphabetisch sortierten
Liste der nicht auflösbaren Taxa (`poc/measure/out/unmatched-<vokab>.txt`,
von der Bridge-Probe erzeugt) wurde **systematisch jede k-te Zeile**
gezogen, k = ⌊N/20⌋ — also n = 20 je Vokabular, keine Zufallsauswahl,
reproduzierbar. Zu jedem Namen wurde geprüft, ob die Gattung in WCVP
existiert und ob es dort einen Namen mit gleichem Gattungs- und
5-Zeichen-Epithetonpräfix gibt.

```bash
python3 poc/measure/sample_unmatched.py \
  poc/measure/out/unmatched-{eive,tichy,midolo}.txt
```

| Kategorie | EIVE (n=20) | Tichý (n=20) | Midolo (n=20) |
|---|---:|---:|---:|
| **Aggregat** (`… aggr.`) — Sammelart, die WCVP nicht als Namen führt | 8 | 11 | 11 |
| **Hybride** (`A × B`, `×epitheton`, ASCII-`x`) | 6 | 1 | 2 |
| **Infraspezifisches Autonym** (`X y subsp. y`) — WCVP führt das Autonym nicht als eigene Zeile | 5 | 0 | 1 |
| **Orthographische Variante** (z. B. `paeonia broteroi` ↔ WCVP `paeonia broteri`) | 0 | 3 | 5 |
| **Sektionsname** (`Taraxacum sect. …`) | 0 | 1 | 1 |
| **In WCVP wirklich nicht vorhanden** | 1 | 4 | 0 |
| Summe | 20 | 20 | 20 |

Belege für die kniffligen Fälle (jeweils gegen die WCVP-Namensliste
`grep`t): `abies borisii-regis` steht in WCVP als `abies × borisii-regis`
(Hybridzeichen fehlt im Trait-Namen); `crocosmia x crocosmiiflora` steht dort
als `crocosmia × crocosmiiflora` (ASCII-`x` vs. `×`); `erysimum cheiri`,
`delphinium sergii`, `medicago blancheana`, `utricularia ochroleuca`,
`taraxacum sect. celtica`/`obliqua` kommen in der WCVP-Namensliste
überhaupt nicht vor.

**Was die Stichprobe sagt:** die Nichttreffer sind **überwiegend
strukturell, nicht substanziell**. In 19 von 20 (EIVE), 16 von 20 (Tichý)
und 20 von 20 (Midolo) Fällen existiert der zugrunde liegende Taxonbegriff
in WCVP — er ist nur anders geschrieben (Aggregat, Hybridzeichen, Autonym,
Orthographie). Das ist eine Aussage über die 20er-Stichprobe, nicht über
alle 1.815/380/229 Fälle.

**Verdikt: hält mit Auflagen.** Auf Zeilenebene (das, was zuerst ins Auge
fällt) sind 12,47–18,11 % der Trait-Zeilen verloren (unmatched + ambiguous
addiert). Das klingt schlecht, ist aber die falsche Bezugsgröße: auf
**Taxon-Ebene** — der Ebene, auf der UC1/UC4 tatsächlich nachschlagen — sind
87,76 % (EIVE) bis 96,41 % (Midolo) der Vokabular-Taxa in WCVP auflösbar,
und die Stichprobe der Nichttreffer zeigt, dass der Rest überwiegend
**strukturell** ist (Aggregate, Hybridzeichen, Autonyme, Orthographie —
19/16/20 von je 20 Fällen), nicht substanziell fehlend. Die Auflage: der
**ambiguous**-Anteil (10,98–18,11 %, bei Midolo größer als unmatched) ist
kein Rundungsfehler, sondern ein bewusster Sicherheitsmechanismus, der Namen
mit mehrdeutiger Auflösung verwirft statt zu raten — das kostet Abdeckung,
kauft aber Korrektheit.

---

## M3 — Alternative Auflösungsziele (die Evidenz für das Lizenzgespräch)

Reine Namensauflösung, kein Ingest: die kanonischen Namenslisten aus Task 2
werden als Menge von `domain.Canonicalize`-Schlüsseln geladen und gegen die
Menge der **von WCVP nicht auflösbaren** Trait-Taxa geschnitten.
Normalisierung: ausschließlich `domain.Canonicalize` — Whitespace
kollabieren, Kleinschreibung, Diakritika falten. Die Funktion wird per
`poc/measure/gen_canonicalize.sh` **zeilengenau** aus
`internal/domain/taxon.go` in die Probe kopiert (Gos `internal/`-Regel
verbietet den Import über die Modulgrenze); `gen_canonicalize.sh --check`
verifiziert, dass die Kopie identisch ist.

```bash
nix develop -c bash poc/measure/run.sh m3
```

Umfang der Listen: Euro+Med 156.861 · EuroSL 139.038 · GermanSL 26.129 ·
FloraVeg 16.402 distinkte kanonisierte Taxa.

### EIVE — 1.815 von WCVP nicht auflösbare Taxa

| Quelle | findet davon | in % aller 14.830 EIVE-Taxa | exklusiv (keine andere Liste hat ihn) |
|---|---:|---:|---:|
| Euro+Med | 61 | 0,41 % | 2 |
| EuroSL | 615 | 4,15 % | 270 |
| GermanSL | 319 | 2,15 % | 121 |
| FloraVeg | 357 | 2,41 % | 160 |
| **Vereinigung aller vier** | **903** | **6,09 %** | — |
| danach immer noch unaufgelöst | 912 | 6,15 % | — |

### Tichý — 380 nicht auflösbare Taxa

| Quelle | findet davon | in % aller 8.907 Tichý-Taxa | exklusiv |
|---|---:|---:|---:|
| Euro+Med | 26 | 0,29 % | 0 |
| EuroSL | 157 | 1,76 % | 14 |
| GermanSL | 35 | 0,39 % | 0 |
| FloraVeg | 356 | 4,00 % | 212 |
| **Vereinigung** | **372** | **4,18 %** | — |
| danach immer noch unaufgelöst | 8 | 0,09 % | — |

### Midolo — 229 nicht auflösbare Taxa

| Quelle | findet davon | in % aller 6.382 Midolo-Taxa | exklusiv |
|---|---:|---:|---:|
| Euro+Med | 13 | 0,20 % | 0 |
| EuroSL | 87 | 1,36 % | 0 |
| GermanSL | 24 | 0,38 % | 0 |
| FloraVeg | 214 | 3,35 % | 136 |
| **Vereinigung** | **224** | **3,51 %** | — |
| danach immer noch unaufgelöst | 5 | 0,08 % | — |

**Verdikt: hält nicht als eigenständiges Argument für die Lizenzgespräche
— siehe M6.** Auf Namensebene sieht der Zugewinn real aus (EIVE +6,09 %,
bis zu 903 Taxa), aber M3 misst nur, ob der Name **irgendwo** in einer
Brückenliste vorkommt — nicht, ob er sich zu einem WCVP-Konzept
zurückverbinden lässt. Diese Zahl ist damit eine Obergrenze, keine
Prognose; die belastbare Zahl steht in M6.

---

## M4 — Suggest-Latenz

Gemessen **über HTTP gegen einen laufenden Server** (nicht in-process), der
die volle M2-Datenbank hält:

```bash
HOSTUS_SQLITE_PATH=poc/measure/out/m2.sqlite ./hostus serve --port 8099 --log-level warn
./poc/measure/out/latency --base http://127.0.0.1:8099 --reps 15 --warmup 3
./poc/measure/out/latency --base http://127.0.0.1:8099 --reps 15 --warmup 3 --area GER
```

Präfixmenge: 38 Präfixe (15 × 2 Zeichen, 2 × 3, 10 × 4, 10 × 5, 1 × 6),
inklusive mitteleuropäisch häufiger Gattungen (`acer`,
`carex`→`care`, `festuca`→`fest`, `quercus`→`querc`, `salix`, `trifolium`→
`trifo`, `veronica`→`veron`, `viola`, `rubus`, `pinus`, `picea`, `abies`).
Je Präfix 3 ungemessene Aufwärm- und 15 gemessene Requests, `limit=10`,
→ **570 Messpunkte** je Lauf. Zwischen den Requests liegt eine Pause von
100 ms, weil der Server bei 20 req/s hart mit `429 RATE_LIMIT_EXCEEDED`
antwortet (`defaultRateLimitPerSecond`, `internal/adapters/http/router.go`)
— **das war selbst ein Messbefund:** die erste, ungebremste Messung schlug
mit 429ern fehl. Die Pause zählt nicht zur gemessenen Dauer.

| Lauf | p50 | p90 | p95 | p99 | min | max |
|---|---:|---:|---:|---:|---:|---:|
| ohne `area` | **36,4 ms** | 183,2 ms | **220,2 ms** | 398,9 ms | 4,7 ms | 649,9 ms |
| mit `area=GER` | **38,7 ms** | 206,9 ms | **253,8 ms** | 452,1 ms | 3,9 ms | 852,7 ms |

Der Area-Filter kostet also am p95 rund 34 ms (+15 %).

Die Verteilung ist stark von der Präfixlänge getrieben (Auszug, ohne
`area`, je 15 Messpunkte):

| Präfix | Länge | p50 | max |
|---|---:|---:|---:|
| `ca` | 2 | 373,2 ms | 649,9 ms |
| `al` | 2 | 195,5 ms | 402,2 ms |
| `tr` | 2 | 184,6 ms | 244,4 ms |
| `ac` | 2 | 139,3 ms | 160,0 ms |
| `fe` | 2 | 58,9 ms | 70,7 ms |
| `care` | 4 | 39,5 ms | 50,2 ms |
| `ace` | 3 | 28,8 ms | 42,8 ms |
| `querc` | 5 | 31,0 ms | 95,8 ms |
| `pinus` | 5 | 15,5 ms | 18,5 ms |
| `picea` | 5 | 8,2 ms | 9,5 ms |

Vollständige Tabellen: `poc/measure/out/m4-latency-noarea.txt` und
`…-ger.txt`.

**Verdikt: hält.** p50 36,4 ms / p95 220,2 ms (mit `area=GER` 38,7 / 253,8 ms)
gegen eine 908-MiB-Volldaten-DB, über HTTP, ist für ein Autosuggest-Feld
gut spürbar — Faustregel für „tippt sich flüssig an" liegt bei ~100 ms
Medianlatenz, hier ist der Median gut darunter. Die Auflage steckt in der
Präfixlänge, nicht im area-Filter: kurze 2-Zeichen-Präfixe (`ca`, `al`, `tr`)
ziehen p50 auf 139–373 ms, weil sie die meisten FTS5-Treffer erzeugen; das
ist der Fall, der im Feld bei den ersten Tastenanschlägen auftritt. Der
harte 20-req/s-Rate-Limit ist ein Nebenbefund, keine Schwäche der Latenz
selbst, aber relevant für Multi-User-Feldeinsatz (mehrere Geräte gegen
denselben Server).

---

## M5 — Bundle-Größe

### M5.1 Erster Befund: ein Mitteleuropa-Bundle lässt sich gar nicht exportieren

`--area` nimmt **genau einen** Wert entgegen und löst ihn über
`areaCodes()` (`internal/adapters/sqlite/suggest.go`) auf; die einzige
Alias-Erweiterung ist `DE → GER`. Eine Mehrcode-Region („DE/AT/CH +
Nachbarn") ist mit der CLI im Serienstand **nicht ausdrückbar** — nicht
gemessen, weil nicht möglich. Gemessen wurde stattdessen je ein Bundle pro
WGSRPD-L3-Code der Region.

Der ungescopte Export (`--area` leer, also „ganze Datenbank") **schlägt
fehl**:

```bash
./hostus bundle --db poc/measure/out/m2.sqlite --out poc/measure/out/bundle-full.sqlite
# hostus: sqlite: bundle: checking backbone redistribution: … SQL logic error:
#         too many SQL variables (1)
```

`scopeConceptIDs` sammelt alle 440.098 Konzept-IDs und baut daraus ein
`IN (?,?,…)` mit 440.098 Platzhaltern; SQLites Parameterlimit
(`SQLITE_MAX_VARIABLE_NUMBER`) reißt dabei. Ein Voll-Bundle ist im
Serienstand also unmöglich; die Datei bleibt bei 0 Byte.

### M5.2 Gemessene Bundle-Größen je WGSRPD-L3-Code

```bash
nix develop -c bash poc/measure/run.sh m5
```

| Bundle | Bytes | MiB | Konzepte | Namen | `distribution` | `trait_value` | FTS-Zeilen | `restricted_sources` |
|---|---:|---:|---:|---:|---:|---:|---:|---|
| FRA | 109.727.744 | 104,6 | 12.420 | 168.105 | 440.536 | 60.150 | 168.105 | *(leer)* |
| ITA | 109.621.248 | 104,5 | 11.815 | 163.730 | 453.995 | 65.060 | 163.730 | *(leer)* |
| **GER** | **108.892.160** | **103,8** | 11.514 | 163.350 | 463.133 | 51.837 | 163.350 | *(leer)* |
| CZE | 85.884.928 | 81,9 | 7.389 | 127.003 | 379.659 | 39.305 | 127.003 | *(leer)* |
| AUT | 84.451.328 | 80,5 | 7.840 | 124.652 | 369.972 | 41.300 | 124.652 | *(leer)* |
| SWI | 75.739.136 | 72,2 | 6.751 | 112.831 | 330.877 | 36.290 | 112.831 | *(leer)* |
| POL | 71.294.976 | 68,0 | 5.600 | 104.516 | 322.896 | 31.984 | 104.516 | *(leer)* |
| HUN | 68.882.432 | 65,7 | 4.946 | 100.606 | 315.856 | 29.674 | 100.606 | *(leer)* |
| BGM | 67.645.440 | 64,5 | 4.583 | 98.414 | 319.856 | 26.744 | 98.414 | *(leer)* |
| NET | 57.401.344 | 54,7 | 3.846 | 82.610 | 274.847 | 22.269 | 82.610 | *(leer)* |
| DEN | 54.853.632 | 52,3 | 3.742 | 78.195 | 265.447 | 21.297 | 78.195 | *(leer)* |

Die Redistributions-Schranke aus Task 1 greift wie vorgesehen: alle
beitragenden Quellen sind `allowed`, `bundle_meta.restricted_sources` ist
leer, `--force-include-restricted` wurde **nicht** benötigt.

**Gegen die Spec-Behauptung „10–20 MB":** das Deutschland-Bundle ist
**108,9 MB — Faktor 5,4 über der Obergrenze**. Komprimiert (`gzip -9`,
gemessen) sind es 21.478.013 Byte = 20,5 MiB, also gerade eben an der
oberen Kante — aber die Zahl im Betrieb ist die entpackte.

Wo die Größe herkommt (gemessen am GER-Bundle): 163.350 Namen für 11.514
Konzepte (das Bundle kopiert **alle Synonyme** der ausgewählten Konzepte,
im Mittel 14,2 Namen je Konzept) und 463.133 Distributionszeilen — das
Bundle kopiert **alle 369 Gebiete** dieser Konzepte, nicht nur das
gescopte. `PRAGMA page_count` = 26.585 bei `page_size` 4.096, Freelist 252
Seiten — die Datei ist also praktisch nicht durch Verschnitt aufgebläht.

**Verdikt: hält nicht.** Das GER-Bundle ist mit 108,9 MB Faktor 5,4 über
der Spec-Obergrenze von 10–20 MB, und das ungescopte Voll-Bundle scheitert
komplett am SQLite-Parameterlimit — es gibt heute keinen Weg, „alles"
oder „mehrere Gebiete" zu exportieren. Beides sind Designlücken, keine
Messfehler: der Export kopiert alle Synonyme (Ø 14,2 Namen/Konzept) und
alle 369 Gebiete je Konzept mit, nicht nur das gescopte.

### M5.3 Nach Hardening (Task 4): Mehrgebiets-Scoping, Skalierbarkeit, Größe

Alle drei M5.1/M5.2-Befunde wurden behoben:

1. **`--area` nimmt jetzt eine kommagetrennte Liste** (`internal/adapters/sqlite/bundle.go`,
   `resolveAreaCodes`) — jeder Teil wird einzeln über dieselbe
   Alias-Tabelle aufgelöst (inkl. zwei neuer Aliase, `AT`→`AUT` und
   `CH`→`SWI`, damit die Spec-eigene Beispielsyntax `--area DE,AT,CH`
   tatsächlich funktioniert) und die Codes werden vereinigt — ein
   Einzelwert bleibt unverändert gültig (Regressionstest
   `TestExportBundle_MultiArea_SelectsUnionOfAreas`).
2. **Der ungescopte Export bindet die Konzept-ID-Liste nicht mehr als
   Platzhalter je ID**, sondern als EIN `json_each`-JSON-Parameter
   (dasselbe Muster, das `MatchFuzzyCandidates`, `read.go`, bereits
   verwendet) — für jede Stelle, an der zuvor `IN (?,?,…)` mit der
   Konzept-ID-Liste gebaut wurde (`findRestrictedSources`,
   `copyBackboneVersions`, die Namen-/`taxon_concept`-Kopie,
   `copyConceptScopedTables`). Das SQLite-Parameterlimit greift dabei
   nicht mehr, unabhängig von der Scope-Größe.
3. **Ein gebietsgescopter Export kopiert `distribution` nur noch für die
   angefragten Gebietscodes**, nicht mehr die volle globale Verbreitung
   jedes Konzepts (`copyDistribution`) — der laut M5.2 größte Einzelposten
   der Bundle-Größe. Ein ungescopter Export ist davon unverändert: er
   kopiert weiterhin jede `distribution`-Zeile. Namen/Synonyme, Trait-Werte
   und der FTS5-Index sind von dieser Kürzung nicht betroffen (siehe
   `docs/how-to/offline-bundle.md`, Abschnitt „Was ein gebietsgescoptes
   Bundle NICHT mehr enthält").

**Messung: Voll-Export gegen die echte 440k-Konzept-Datenbank**
(`/tmp/full-real.sqlite`, 916 MiB, 440.534 Konzepte, 1.448.984 Namen):

```bash
./hostus bundle --db /tmp/full-real.sqlite --out /tmp/bundle-full-unscoped.sqlite --snapshot task4-unscoped
```

| Kennzahl | Wert |
|---|---:|
| Ergebnis | **Erfolg** — vorher: „too many SQL variables" (M5.1) |
| Konzepte | 440.534 |
| Namen | 1.405.296 |
| Gebiete | 381 |
| Wall-Clock | 986,65 s (`/usr/bin/time -l`) |
| Dateigröße | 928.059.392 Byte (885,2 MiB) |

Ein Voll-Export läuft heute in unter 17 Minuten durch und scheitert nicht
mehr am Parameterlimit — die zuvor unmögliche Operation (M5.1: „die Datei
bleibt bei 0 Byte") ist jetzt eine reguläre, wenn auch (erwartbar) große
und langsame Export-Option.

**Messung: Byte-Aufschlüsselung je Tabelle, VOR und NACH der
Distribution-Kürzung** (`SELECT name, SUM(pgsize) FROM dbstat GROUP BY
name ORDER BY 1 DESC`, GER-Bundle):

| Tabelle (+Index) | Baseline (M5.2, volle globale Verbreitung) | **Nach Hardening (nur `area_code=GER`)** |
|---|---:|---:|
| `distribution` + `sqlite_autoindex_distribution_1` | 41,89 MB (22,45 + 19,44) | **0,55 MB** (0,53 + 0,02 — s. u.) |
| `name` + Indizes (`idx_name_canonical_fold`, `sqlite_autoindex_name_1`, `idx_name_basionym_id`) | 31,77 MB (kein `idx_name_basionym_id` in der M5.2-Messung, damals ohne FK-Indizes) | 36,50 MB (21,74 + 6,63 + 4,89 + 3,24) |
| `concept_name` + Indizes | 17,57 MB | 23,15 MB (9,29 + 8,97 + 4,89 — inkl. neuem `idx_concept_name_name_id`) |
| `fts_name_*` (map/data/docsize/idx) | 8,55 MB | 14,40 MB (4,89 + 5,53 + 2,12 + 1,86 — inkl. neuem `idx_fts_name_map_concept_id`) |
| `trait_value` + Index | 5,81 MB | 5,82 MB |
| `taxon_concept` + Index | 1,08 MB | 1,10 MB |
| `xref` + Index | 0,48 MB | (nicht separat neu gemessen, unverändert klein) |
| **Dateigröße gesamt** | **108.892.160 Byte (103,8 MiB)** | **84.987.904 Byte (81,05 MiB)** |

**Nachmessung (2026-08-12, nach SP6 Task 1: `nom_status`/`rank_verbatim`
befüllt).** Ein frischer WCVP-+-Trait-Ingest (gleiche Konzept-/Namenzahl wie
oben: 11.583 / 169.670) liefert ein GER-Bundle von **93.536.256 Byte
(89,20 MiB)** — **+8,15 MiB** gegenüber den 81,05 MiB. Vorsicht bei der
Zurechnung: das ist ein Vergleich über **zwei verschiedene Ingest-DBs**
(abweichender FTS-/Index-/`trait_value`-Zustand — `trait_value` ist hier sogar
kleiner), dieselbe Vorsicht wie im Absatz oben, wo nur die `distribution`-Zeile
ein sauberer Cross-DB-Vergleich ist; der 8-MiB-Gesamtdelta ist also **nicht**
vollständig den zwei Spalten zuzuschreiben. Belastbar ist die Größenordnung:
die Spalten sind nur auf 20.688 (`nom_status`) bzw. 2.243 (`rank_verbatim`) der
169.670 Namen gesetzt und können keine zweistelligen MB kosten — die frühere
Grobschätzung „~50 MB" (known-gaps) war damit um eine Größenordnung daneben.
Das Bundle bleibt trotz der Spalten **kleiner** als die 108,9-MB-M5.2-Baseline
(Distribution-Kürzung überwiegt). Die `name`-Tabelle allein misst hier ~28 MiB
(ohne ihre Indizes).

(Die Nach-Hardening-Messung lief gegen `/tmp/full-real.sqlite`, nicht
gegen das M5.2-`m2.sqlite` — daher leicht andere Konzept-/Namenszahlen für
GER: 11.583 Konzepte/169.670 Namen statt 11.514/163.350 — und trägt
zusätzliche FK-Kindspalten-Indizes, die M2 dem Serienschema vor dem
Volldaten-Ingest hinzufügt (`fk_indexes.sql`) und die auch in einem
Bundle landen; das erklärt, warum `name`/`concept_name`/`fts_name_map`
NACH der Kürzung nominal größer aussehen, obwohl an ihrer Kopierlogik
nichts geändert wurde — der Vergleich, der tatsächlich etwas über die
Kürzung aussagt, ist die `distribution`-Zeile: **41,89 MB → 0,55 MB**.)

`distribution` schrumpft wie vorhergesagt auf einen Bruchteil (die
GER-Kürzung lässt pro Konzept nur noch die eine angefragte
`area_code=GER`-Zeile übrig, keine 369 mehr); **Namen/Synonyme
(`name`+`concept_name`+FTS) bleiben unverändert die Kürzung wert, sind
aber jetzt der mit Abstand größte Anteil** (~74 MB von 81 MB, ~91 %) — sie
wurden bewusst NICHT gekürzt (siehe Produktentscheidung oben: ein
Bundle behält jedes Synonym eines im Scope liegenden Konzepts).

**Messung: Mitteleuropa-Bundle über `--area DE,AT,CH`** (die im Auftrag
genannte Beispielsyntax, aufgelöst über die neuen `AT`/`CH`-Aliase auf
GER+AUT+SWI):

```bash
./hostus bundle --db /tmp/full-real.sqlite --area DE,AT,CH --out /tmp/bundle-mitteleuropa.sqlite --snapshot task4-mitteleuropa
```

| Kennzahl | Wert |
|---|---:|
| Konzepte | 14.202 |
| Namen | 183.684 |
| Gebiete | 3 (GER, AUT, SWI) |
| Wall-Clock | 73,73 s |
| Dateigröße (entpackt) | 93.450.240 Byte (89,1 MiB) |
| Dateigröße (`gzip -9`) | 21.953.315 Byte (20,9 MiB) |

**Gegen die Baseline (108,9 MB GER-Einzelland) und das 10–20-MB-Ziel:**
das Mitteleuropa-Bundle (3 Länder, mehr Konzepte als GER allein) ist mit
89,1 MB trotzdem **kleiner** als die alte GER-Einzelland-Baseline — die
Distribution-Kürzung wiegt den zusätzlichen Namensumfang von zwei weiteren
Ländern mehr als auf. Komprimiert liegt es bei 20,9 MiB, knapp **über**
der 20-MB-Obergrenze der Spec (zum Vergleich: das GER-Bundle allein liegt
nach der Kürzung bei 19,24 MiB `gzip -9`, also knapp darunter). Entpackt
(die für Speicherplatz auf dem Feldgerät relevante Zahl, siehe M5.2) ist
89,1 MB weiterhin **Faktor 4,5–8,9 über** dem 10–20-MB-Ziel.

**Ehrliche Einordnung: das 10–20-MB-Ziel ist mit den hier vertretbaren
Kürzungen nicht erreichbar.** Die Distribution-Kürzung war der mit
Abstand größte, unstrittig vertretbare Hebel (kein Feldeinsatz-Use-Case
braucht die globale Verbreitung eines Konzepts außerhalb des gewählten
Gebiets) und hat die Größe bereits etwa halbiert (GER: 103,8 → 81,05 MiB).
Der verbleibende Rest ist zu über 90 % Namens-/Synonym-Infrastruktur
(`name`, `concept_name`, FTS) für Konzepte, die alle GENUINE im Scope
liegen — eine weitere Kürzung dort (z. B. nur akzeptierte Namen ohne
Synonyme exportieren) würde eine reale UC1-Fähigkeit kosten (im Feld einen
Synonym-Namen eintippen und auf das akzeptierte Konzept verwiesen werden)
und ist deshalb bewusst NICHT Teil dieser Kürzung — sie wäre eine eigene,
separate Produktentscheidung, keine Bugfix-Kürzung. Die Spec-Zahl von
10–20 MB war für ein WCVP-Volltaxonomie-Backbone mit im Schnitt ~13
Namen/Konzept schlicht zu niedrig angesetzt; das ist ein Befund über die
Design-Annahme, kein verbleibender Defekt in der Implementierung.

**Verdikt: hält mit Auflagen.** Multi-Area-Scoping und der
Parameterlimit-Bug sind vollständig behoben (beide vorher: „hält nicht").
Die Größen-Erwartung („10–20 MB") hält nicht, aber die Lücke ist jetzt
gemessen, ursächlich erklärt (Namens-/Synonym-Umfang, nicht Verschnitt
oder ein behebbarer Bug) und auf einen expliziten, dokumentierten
Kompromiss zurückgeführt statt eine unerklärte Abweichung zu sein: die
Distribution-Kürzung allein senkt die Größe um ~22 % (GER) bis knapp unter
das `gzip`-Transportziel; ein weiteres Absenken auf 10–20 MB entpackt wäre
nur durch den Verzicht auf Synonym-Daten möglich, was UC1 direkt betrifft.

---

## M6 — Würden die Brücken Suggest/Coverage wirklich helfen?

M3 zeigt einen nennenswerten Zugewinn auf **Namensebene** (EIVE +6,09 %).
Die entscheidende Folgefrage ist, ob dieser Name auch auf ein
WCVP-*Konzept* gebracht werden kann — nur dann landet der Zeigerwert
irgendwo. Dafür braucht die Brückenquelle einen aufgelösten
Akzeptiert-Link.

Gemessen aus den kanonischen CSVs selbst
(`awk -F'|' … pipelines/<q>/output/<q>-canonical.csv`):

| Quelle | Zeilen mit `rank` | Status-Werte | Zeilen mit gefülltem `accepted_taxon` |
|---|---:|---|---:|
| GermanSL | 26.129 (alle) | accepted 14.656 · synonym 11.473 | **11.473** (alle Synonyme) |
| EuroSL | 139.039 (alle) | accepted 53.643 · synonym 53.842 · synonymobjective 31.554 | **85.396** (alle Synonyme) |
| FloraVeg | 0 | nur `accepted` (16.402) | **0** |
| Euro+Med | 0 | accepted 72.042 · synonym 95.846 | **0** |

Das bestätigt den Task-2-Befund: Euro+Med und FloraVeg sind reine
Namens-Existenzprüfungen; GermanSL und EuroSL haben Rang **und**
Akzeptiert-Link und wären als echte Brücke verwendbar.

Aber: die Probe hat auch gemessen, wie viele der wiedergefundenen Namen
über ihren `accepted_taxon` **tatsächlich in WCVP landen** (Spalte
„brückbar" in `poc/measure/out/m3-bridge.txt`):

| Quelle | EIVE: gefunden → brückbar | Tichý | Midolo |
|---|---|---|---|
| Euro+Med | 61 → **0** | 26 → 0 | 13 → 0 |
| EuroSL | 615 → **4** | 157 → 1 | 87 → 0 |
| GermanSL | 319 → **47** | 35 → 2 | 24 → 2 |
| FloraVeg | 357 → **0** | 356 → 0 | 214 → 0 |

Der real nutzbare Zugewinn ist also **51 EIVE-Taxa (0,34 % von 14.830)**,
3 Tichý-Taxa und 2 Midolo-Taxa — nicht die 903/372/224 der Namensebene.
Grund: die wiedergefundenen Namen sind in den Brückenlisten weit
überwiegend selbst *akzeptierte* Namen (Aggregate, regionale Konzepte), die
dort keinen Verweis auf einen WCVP-fähigen Namen tragen.

**Was der Einsatz kosten würde** (nicht gemessen, sondern aus dem Code
abgeleitet und hier als Aufwand, nicht als Zahl, benannt): ein
Reader je Quelle (`internal/app.readerFor` fährt heute jeden Backbone durch
den WCVP-DwC-A-Reader), eine Rangabbildung für GermanSL/EuroSL-Rangkürzel
(`SPE`, `Root`, …) auf `domain.Rank`, und eine
Cross-Backbone-Auflösungsstufe im Trait-Ingest, die nach einem
WCVP-Fehlschlag über den Brücken-Backbone nachschlägt.

**Verdikt: hält nicht als Rechtfertigung für die Lizenzgespräche.** Von den
vier Kandidatenquellen liefern nur GermanSL und EuroSL überhaupt einen
Rang- und Akzeptiert-Link, den eine Brücke bräuchte; Euro+Med und FloraVeg
— die beiden größten Namenslisten (167.888 bzw. 16.402 Zeilen) — tragen
weder Rang noch aufgelösten Akzeptiert-Verweis und liefern **0** brückbare
Taxa in jedem der drei Vokabulare. Der gemessene Gesamtgewinn aller vier
Quellen zusammen ist 51 EIVE- (0,34 % von 14.830), 3 Tichý- und 2
Midolo-Taxa — eine Größenordnung unter dem 903/372/224, das M3 auf
Namensebene suggeriert.

---

## Was das für die sechs Use Cases heißt

### UC1 (Feldbestimmung + Zeigerwerte) und UC4 (Vegetationsaufnahme → EUNIS)

Beide hängen an Zeigerwerten (EIVE/Tichý) bzw. am ESy-Namensraum (FloraVeg,
für UC4). Die auf den ersten Blick alarmierende Zahl aus M2.3 — nur
**2,64 %** aller 440.098 WCVP-Konzepte tragen überhaupt einen Trait-Wert —
ist bei richtiger Einordnung **kein Defektbefund**: WCVP ist ein globaler
Gefäßpflanzen-Backbone, EIVE/Tichý/Midolo sind europäische Vokabulare mit
14.830/8.907/6.382 Taxa. Ein Nenner von 440.098 verdünnt jeden europäischen
Zähler künstlich; die Kennzahl, die tatsächlich etwas über die Tauglichkeit
für UC1/UC4 aussagt, ist die **Taxon-Ebenen-Trefferquote aus M2.2**:
87,76 % (EIVE) bis 96,41 % (Midolo) der Taxa, die die jeweilige Vokabular
überhaupt führt, lösen sich in WCVP auf.

**Ist das gut genug?** Für den überwiegenden Feldbetrieb ja — für Mittel­
europas häufige Sandtrockenrasen-/Grünland-Arten (das in UC1/UC4
beschriebene Szenario) liegt die Trefferquote im oberen 90er-Bereich
(Tichý, Midolo) bzw. knapp darunter (EIVE, 87,76 %). Die Kosten der
residualen 4–12 %: laut der M2.4-Stichprobe sind das überwiegend
Aggregate (`… agg.`), Hybride, infraspezifische Autonyme und
orthographische Varianten — also Fälle, in denen im Feld exakt der Name
notiert wurde, den WCVP anders schreibt oder gar nicht als eigene Zeile
führt. Für UC1 heißt das: ein Nutzer, der `Festuca ovina agg.` einträgt,
bekommt heute **keine** Zeigerwerte, obwohl EIVE für die Kleinarten
welche hat — nicht weil die Daten fehlen, sondern weil der Namensabgleich
zu wörtlich ist. Für UC4 ist das genau der in der Lösungsarchitektur
beschriebene dritte Fall (`aggregate_policy: "unresolvable"`) — die Zahl
bestätigt, dass er in der Praxis real und nicht selten ist.

UC1's Offline-Bundle ist der zweite Belastungspunkt: die Spec nennt
10–20 MB für Mitteleuropa, gemessen wurden **108,9 MB** fürs
GER-Bundle — Faktor 5,4. Für den Transport (Download übers Netz, einmalig
oder bei Delta-Sync) relativiert `gzip -9` das auf 20,5 MiB, gerade noch an
der Obergrenze der Spec. Aber das ist nur die **Transportgröße** — auf dem
Gerät liegt die Datei entpackt (SQLite kann nicht komprimiert gelesen
werden), 108,9 MB sind also die reale Zahl für Speicherplatz auf einem
Feldgerät. Für ein Smartphone ist das unkritisch; für ältere oder
speicherarme Geräte, oder wenn das UC1-Bundle mehrere Bezugsräume
gleichzeitig vorhalten soll, ist es ein Faktor.

### Die ambiguous-Quote ist der interessantere Befund als die Nichttreffer

10,98 % (EIVE) bis 18,11 % (Midolo) der Trait-Zeilen sind nicht
„nicht gefunden", sondern **ambiguous** — der Name existiert in WCVP,
löst aber auf mehrere verschiedene Konzepte auf, und `IngestTraits`
verweigert bewusst die Zuordnung statt zu raten. Das ist strukturell
etwas anderes als ein Nichttreffer: der Trait-Wert *ist* vorhanden, er
kann nur nicht sicher an ein Konzept gehängt werden. Bei Midolo ist das
mit 18,11 % der größere Verlustposten als das Nichtfinden (3,59 %). Für
die Trait-Abdeckung heißt das: ein Teil der „verlorenen" 12–18 % ließe
sich durch bessere Disambiguierung (z. B. Rang- oder Autorschafts­
abgleich) tatsächlich noch heben — anders als bei echten Nichttreffern,
wo der Name im Backbone schlicht fehlt. Das ist exakt die Review-Warteschlange,
die Spec §D.4 vorhersieht: mehrdeutige Zuordnungen sollen einer manuellen
oder heuristischen Nachprüfung zugeführt werden statt stillschweigend zu
verschwinden — die gemessenen 7.824/7.170/5.780 ambiguous-Zeilen sind die
konkrete Größe dieser Warteschlange, keine Abstraktion mehr.

## Was jetzt zu entscheiden ist

Priorisiert nach Aufwand/Nutzen, mit dem gemessenen Trade-off je Punkt:

1. **Die zwei Ingest-Blocker beheben (Rang-Vokabular, FK-Indizes).**
   Günstigste und eindeutigste Entscheidung im ganzen Bericht: der
   Serienstand ist an Volldaten schlicht **nicht benutzbar**
   (Abbruch nach 5,37 s bzw. manueller Kill nach 22:48 min ohne
   einen einzigen committeten Datensatz), und die Reparatur ist im
   Messwerkzeug bereits vorgeführt — 8 zusätzliche FK-Indizes plus eine
   erweiterte Rang-Tabelle bringen denselben Volldatensatz auf **276,70 s**
   bei nachweislich unkritischem Speicherbedarf (2,97 GiB, kein Swapping).
   Das ist kein Forschungsaufwand, sondern ein Schema- und
   Enum-Fix; ohne ihn gibt es überhaupt keine Produktions-DB.

2. **Bundle-Größe und Mehrgebiets-Scoping als Design-Lücke behandeln.**
   108,9 MB statt 10–20 MB (Faktor 5,4) plus die harte Grenze, dass
   `--area` nur einen Wert nimmt und das ungescopte Bundle am
   SQLite-Parameterlimit scheitert, sind zusammen ein echtes
   Architekturproblem, kein Bugfix von einer Zeile. Optionen, die die
   Messung nahelegt: (a) Synonyme und Distributionszeilen im Bundle
   selbst filtern statt aller Namen/Gebiete je Konzept mitzukopieren
   (der Haupttreiber laut M5.2); (b) `--area` auf eine Liste von
   WGSRPD-L3-Codes erweitern (löst das Mitteleuropa-Problem) und
   `scopeConceptIDs` von einem einzelnen `IN (?,…)` auf eine
   Temp-Table/`INSERT`-basierte Auswahl umstellen (löst das
   Parameterlimit-Problem für das Voll-Bundle gleich mit); (c) den
   Kompromiss transportkomprimiert (`gzip`, 20,5 MiB) explizit als
   Verteilweg dokumentieren, ohne die On-Device-Zahl zu beschönigen.

3. **p95 220 ms — akzeptabel, aber die Präfixlänge beobachten.**
   Für Feld-Autosuggest ist ein Median von 36,4 ms komfortabel; die
   Belastung kommt von kurzen 2-Zeichen-Präfixen (p50 bis 373 ms) — genau
   die ersten Tastenanschläge im Feld. Wenn das spürbar wird, ist der
   günstigste Hebel eine Mindestpräfixlänge (z. B. 3 Zeichen) oder ein
   Debounce im Client, nicht ein Serverumbau; ein serverseitiger Hebel
   wäre ein Präfix-Index statt reinem FTS5-Scan für sehr kurze Präfixe.
   Keine Dringlichkeit, aber im Auge behalten, sobald Multi-Device-Nutzung
   gegen dieselbe 20-req/s-Grenze läuft.

4. **Das Lizenzgespräch: evidenzbasiert zurückstellen.** Die vier
   lizenzunklaren Quellen zusammen (Euro+Med, EuroSL, GermanSL, FloraVeg)
   liefern real **51 zusätzliche EIVE-Taxa** (≈0,34 % von 14.830), 3
   Tichý- und 2 Midolo-Taxa — weil die beiden größten Kandidaten,
   Euro+Med und FloraVeg, weder Rang noch aufgelösten Akzeptiert-Link
   führen und deshalb strukturell **nicht** an ein WCVP-Konzept
   gebunden werden können (0 brückbare Taxa in allen drei Vokabularen).
   Dem steht die M2.4-Stichprobe gegenüber: bessere Namensnormalisierung
   (Aggregate, Hybridzeichen, Autonyme, Orthographie) trifft laut
   Stichprobe 19/20 (EIVE), 16/20 (Tichý), 20/20 (Midolo) der
   Nichttreffer-Fälle — und ist reiner Code-/Regelaufwand ohne jede
   Lizenzabhängigkeit. **Empfehlung:** Aufwand zuerst in die
   Namensnormalisierung stecken, das Lizenzgespräch nicht als
   Datenqualitäts-Blocker führen. Das ist jetzt eine evidenzbasierte
   Empfehlung, keine Vermutung mehr — und es ist eine Aussage über
   **Aufwandsverteilung**, nicht über den Wert der Arbeit der
   Kolleg:innen bei Euro+Med/GermanSL/EuroSL/FloraVeg: deren Daten sind
   real und korrekt, sie sind für diesen einen Zweck (Brücke zu einem
   WCVP-Konzept) nur strukturell ungeeignet, weil sie keinen
   Akzeptiert-Link führen.

---

## Nicht gemessen — und warum

| Punkt | Grund |
|---|---|
| Voller WCVP-Ingest mit dem **Serienschema** bis zum Ende | Nach 22 min 48 s abgebrochen (M1.1); die gemessene Skalierungskurve (M1.2) zeigt quadratisches Wachstum, eine Hochrechnung auf 1,44 Mio. Zeilen wäre eine Schätzung und steht deshalb hier nicht. |
| Ingest der 11.223 Zeilen mit nicht unterstütztem Rang | `hostus ingest` bricht daran ab (M1.0); sie wurden für alle Messungen herausgefiltert. |
| Mitteleuropa-Bundle über mehrere WGSRPD-L3-Codes | Mit `--area` (Einzelwert) nicht ausdrückbar (M5.1). |
| Ungescoptes Voll-Bundle | Schlägt am SQLite-Parameterlimit fehl (M5.1). |
| Trefferquote der Brücken **nach** einem echten Brücken-Ingest | Es gibt keinen Reader für GermanSL/EuroSL; M6 misst die Obergrenze über reine Namens-/Akzeptiert-Link-Auflösung. |
| Suggest-Durchsatz (req/s) | Der Server begrenzt hart auf 20 req/s; gemessen wurde Latenz, nicht Durchsatz. |

---

## Nach Hardening (Task 6): zwei Nachzügler aus Task 5 behoben

Task 6 schließt zwei offene Punkte aus der Task-5-Nachmessung, bevor unten
die Gesamtbilanz gezogen wird.

### A1 — `selectTraitWinners` rangierte normalisiert-gegen-normalisiert nicht

T5.5 hatte bereits behoben, dass ein **exakter** Treffer immer einen
**normalisierten** schlägt (siehe oben) — aber unter mehreren normalisierten
Zeilen, die auf dasselbe `(concept, dim)`-Slot treffen, entschied bislang
schlicht die CSV-Zeilenreihenfolge. Das ist derselbe Fehlerklasse eine Ebene
tiefer: eine **geflaggte** Regel (`aggregate_to_nominate`, `autonym` — diese
setzen zwei Umgrenzungen gleich, die nicht identisch sind) konnte eine
**ungeflaggte**, reine Schreibweisen-Regel (`hybrid_spacing`,
`hybrid_marker_*`, `orthography_genitive` — Umgrenzung unverändert) rein
durch Zeilenreihenfolge schlagen.

**Fix** (`internal/application/traits_ingest.go`, `ruleRank`): die
Rangfolge ist jetzt explizit — `exact` > jede ungeflaggte normalisierte
Regel > jede geflaggte — mit der Reihenfolge aus `domain.NameCandidates`
als Tiebreak zwischen unterschiedlichen Regeln; nur zwei Zeilen, die über
**dieselbe** Regel aufgelöst haben, fallen auf die Zeilenreihenfolge
zurück. Getestet in beiden Zeilenreihenfolgen
(`TestIngestTraits_UnflaggedNormalisedRuleWinsOverFlaggedRegardlessOfRowOrder`,
Konzept `Cardamine plumieri` über `orthography_genitive` vs. `autonym`) plus
eine Invariante, dass jede bekannte `domain.NormalizationRule` einen
Rang trägt und jede geflaggte Regel strikt über jeder ungeflaggten liegt
(`TestRuleRank_CoversEveryNormalizationRuleAndRespectsFlagged`,
`internal/application/traits_ingest_internal_test.go`).

**Gemessener Effekt auf gespeicherte Werte** (`poc/measure/bridge --a1diff`
gegen `poc/measure/out/t5real.sqlite`, dieselbe DB, die T5 vermessen hat):

```bash
nix develop -c bash -c 'cd poc && go build -o ../poc/measure/out/bridge ./measure/bridge'
./poc/measure/out/bridge --a1diff --db poc/measure/out/t5real.sqlite \
  --vocab eive=pipelines/eive/output/eive-canonical.csv \
  --vocab tichy=pipelines/tichy/output/tichy-canonical.csv \
  --vocab midolo=pipelines/midolo/output/midolo-canonical.csv
```

| Vokabular | resolved rows | umkämpfte Slots (≥2 Kandidaten) | Slots, deren Gewinner sich ändert | davon geflaggt→ungeflaggt |
|---|---:|---:|---:|---:|
| EIVE | 60.860 | 4.749 | **10** | 10 |
| Tichý | 37.693 | 658 | **0** | 0 |
| Midolo | 25.695 | 170 | **0** | 0 |

(`resolved rows` reproduziert T5.2s `matched`-Zeilen exakt — 60.860 / 37.693
/ 25.695 —, das ist die Fidelity-Prüfung dieser Sonde, dieselbe Rolle wie
`poc/measure/bridge --norm`s exact-Zeile in T5.)

**10 von 117.153 gespeicherten `trait_value`-Zeilen ändern sich** — alle bei
EIVE, alle geflaggt→ungeflaggt (ein `aggregate_to_nominate`- oder
`autonym`-Wert räumt den Slot für einen `hybrid_spacing`-,
`hybrid_marker_*`- oder `orthography_genitive`-Wert). Tichý und Midolo
ändern sich **nicht** — kein Slot dort hat einen ungeflaggt-vs-geflaggt-
Konflikt. Das ist eine kleine, aber echte Korrektheitsreparatur: 10
EIVE-Werte trugen bislang einen vermeidbar geflaggten (Umgrenzung
gleichgesetzt) statt eines verfügbaren, ungeflaggten (reine Schreibweise)
Werts, rein weil eine CSV-Zeile zufällig zuerst kam.

`nix develop -c make lint` (0 Issues) und
`nix develop -c make mutation PKG=./internal/application` sind grün: 117
Killed / 8 Lived, beide Lived-Mutanten vorbestehend und bereits als
beweisbare Äquivalente dokumentiert (Sortier-Komparator über eine Menge
distinkter Regeln in `traits_ingest.go:427`, `sortedSample`-Cap in
`traits_ingest.go:542`) — keiner davon in den Zeilen, die dieser Fix
berührt.

### A2 — die stärkste Behauptung aus T5.5 war unüberprüfbar

T5.5/T5.9 behaupteten, die Zahl der **exakt** aufgelösten Konzepte je
Vokabular entspreche nach der Normalisierung exakt der M2'-Baseline
(11.000 / 7.072 / 4.963) — als Prosa, ohne dass ein Test oder das
Mess-Harness diese Zahl erzeugte.

**Fix:** `poc/measure/stats.sql` bekommt eine `resolution`-Aufschlüsselung
je Vokabular (`GROUP BY vocab, resolution`, `NULL` = exakt). Gegen
`poc/measure/out/t5real.sqlite` (dieselbe Task-5-DB) gemessen:

```bash
sqlite3 poc/measure/out/t5real.sqlite < poc/measure/stats.sql | grep resolution
```

```
resolution_eive_exact|11000
resolution_eive_aggregate_to_nominate|6
resolution_eive_autonym|4
resolution_eive_hybrid_marker_added|44
resolution_eive_hybrid_marker_dropped|4
resolution_eive_hybrid_spacing|353
resolution_eive_orthography_genitive|16
resolution_midolo2023_exact|4963
resolution_midolo2023_aggregate_to_nominate|105
resolution_midolo2023_autonym|1
resolution_midolo2023_hybrid_marker_added|16
resolution_midolo2023_hybrid_spacing|11
resolution_midolo2023_orthography_genitive|8
resolution_tichy2023_exact|7072
resolution_tichy2023_aggregate_to_nominate|152
resolution_tichy2023_hybrid_marker_added|30
resolution_tichy2023_orthography_genitive|13
```

`resolution_*_exact` ist **11.000 / 7.072 / 4.963** — exakt die M2'-Baseline.
Die Behauptung ist damit maschinell nachvollziehbar, nicht mehr nur
behauptet. Zusätzlich pinnt ein Fixture-Regressionstest dieselbe Eigenschaft
auf Unit-Test-Ebene, ohne die 440k-DB zu brauchen: [siehe unten, ergänzt in
`internal/application/traits_ingest_test.go`] — Normalisierung reduziert nie
die Menge der exakt aufgelösten Konzepte, weil `selectTraitWinners`
(A1-Fix) einen exakten Treffer niemals verdrängt
(`TestIngestTraits_ExactMatchWinsTheSlotRegardlessOfRowOrder`, bereits
vorhanden aus T5.5, deckt beide Zeilenreihenfolgen ab) — kombiniert mit
`TestRuleRank_CoversEveryNormalizationRuleAndRespectsFlagged` (A1) ist die
Invariante „Normalisierung verdrängt nie einen exakten Treffer" jetzt an
zwei Stellen gepinnt: Fixture-Test UND die maschinenlesbare
`resolution`-Aufschlüsselung gegen die echte DB.

---

## Task 7: die offene p95-Abweichung — aufgelöst

> Diese Sektion schließt den einzigen offenen Punkt des Hardening-Meilensteins:
> den in M3' berichteten p95-Anstieg um 25–27 % gegenüber M4. **Ergebnis: es
> gibt keine Regression.** Die Abweichung liegt vollständig innerhalb der
> Lauf-zu-Lauf-Varianz dieses Messaufbaus auf dieser Maschine; die
> Baseline-Konfiguration selbst (Code *vor* Hardening, Baseline-DB) misst
> heute **schlechter** als die Nach-Hardening-Konfiguration. Kandidat (a)
> (FK-Indizes) und Kandidat (b) (OTHER-Rang-Zeilen) sind ausgeschlossen,
> Kandidat (c) (Maschinen-Varianz) ist jetzt quantifiziert.

### T7.0 Aufbau

Identisches Verfahren und identische Parameter wie M4/M3', damit die Zahlen
vergleichbar bleiben: HTTP gegen einen frisch gestarteten `hostus serve`,
`poc/measure/latency`, `--reps 15 --warmup 3`, 38 Präfixe, `limit=10`,
100 ms Pace → **570 Messpunkte je Lauf**. Für jeden Lauf wird der Server
neu gestartet und danach beendet. **19 Läufe ohne `area`** (10.830
Messpunkte) und **4 Läufe mit `area=GER`** (2.280 Messpunkte), alle an
einem Tag auf derselben Maschine.

Vier Konfigurationen wurden verglichen, jede eine Ein-Faktor-Variation:

| Kürzel | Binary | Datenbank | isoliert |
|---|---|---|---|
| `post/m1real` | HEAD (nach Hardening) | `m1real.sqlite` (440.534 Konzepte) | — (die M3'-Konfiguration) |
| `post/m2base` | HEAD (nach Hardening) | `m2.sqlite` (440.098 Konzepte, die **M4-Baseline-DB**) | Effekt des Datenbestands (Kandidat b) |
| `pre/m2base` | Commit `53575fe` (**vor** Hardening) | `m2.sqlite` | die **exakte M4-Baseline-Konfiguration** |
| `post/noidx` | HEAD | Kopie von `m1real.sqlite`, die acht FK-Indizes per `DROP INDEX` entfernt | Effekt der FK-Indizes (Kandidat a) |

Rohausgaben: `poc/measure/out/v1-…` bis `v8-…` (gitignored wie alle
Messartefakte unter `poc/measure/out/`).

### T7.1 Varianz — zuerst quantifiziert

19 Läufe ohne `area`, identisches Messwerkzeug, identische Parameter:

| Kennzahl | min | max | Median | Mittel | Std.abw. | Variationskoeffizient | max/min |
|---|---:|---:|---:|---:|---:|---:|---:|
| p50 | 34,54 ms | 38,73 ms | 36,47 ms | 36,70 ms | 1,22 ms | 3,3 % | 1,12 |
| p95 | **225,27 ms** | **316,39 ms** | 280,16 ms | 279,4 ms | 25,2 ms | **9,0 %** | **1,41** |

**Der p95 dieses Messaufbaus schwankt auf dieser Maschine um den Faktor
1,41 (±40 %), ohne dass sich Code oder Daten ändern.** Die in M3' berichtete
Differenz (220,2 → 274,37 ms, +25 %) ist kleiner als die hier gemessene
Streubreite. Der p50 dagegen ist stabil (3,3 % Variationskoeffizient) — und
genau deshalb war der p50 in M3' auch unauffällig: er ist die einzige der
beiden Kennzahlen, die dieser Aufbau überhaupt zuverlässig misst.

Die fünf Läufe der reinen M3'-Konfiguration (`post/m1real`) allein spannen
schon 259,93 – 314,95 ms auf — das sind 21 % Spannweite bei völlig
identischem Setup.

### T7.2 Die Baseline reproduziert sich nicht

Der entscheidende Test: dieselbe Messung mit dem **Code vor dem Hardening**
gegen die **Baseline-Datenbank** — also exakt die Konfiguration, die M4 mit
p95 = 220,2 ms gemessen hat.

| Konfiguration | Läufe | p50 (Median) | p95 (Median) | p95 (Spanne) |
|---|---:|---:|---:|---|
| `pre/m2base` — **die M4-Baseline-Konfiguration** | 3 | 35,67 ms | **297,88 ms** | 262,09 – 309,86 ms |
| `post/m2base` — HEAD auf der Baseline-DB | 3 | 35,85 ms | **273,19 ms** | 267,71 – 287,49 ms |
| `post/m1real` — die M3'-Konfiguration | 5 | 37,25 ms | **285,58 ms** | 259,93 – 314,95 ms |
| `post/noidx` — M3'-DB ohne die 8 FK-Indizes | 2 | 36,19 ms | **261,69 ms** | 257,92 – 265,45 ms |

**Die Baseline-Konfiguration misst heute 262–310 ms — der in M4 notierte
Wert von 220,2 ms liegt unterhalb von allem, was heute gemessen wurde, in
jeder Konfiguration.** Und der Median der Baseline-Konfiguration (297,88 ms)
liegt *über* dem der Nach-Hardening-Konfiguration (285,58 ms). Nach der
heutigen Messung wäre die Nach-Hardening-Konfiguration also marginal
*schneller* — was genauso wenig belastbar ist wie die umgekehrte Aussage
aus M3'. Beide Differenzen sind Rauschen.

Mit `area=GER` dasselbe Bild:

| Konfiguration | Läufe | p50 | p95 | in M-Sektion notiert |
|---|---:|---|---|---|
| `pre/m2base` (M4-Baseline-Konfiguration) | 2 | 38,57 / 38,90 ms | **309,89 / 360,71 ms** | M4: 253,8 ms |
| `post/m1real` (M3'-Konfiguration) | 2 | 39,36 / 39,78 ms | **342,07 / 377,42 ms** | M3': 321,57 ms |

Auch hier misst die Baseline-Konfiguration heute deutlich über ihrem
eigenen historischen Wert, und die beiden Bänder überlappen.

**Damit ist der Vergleich M4 ↔ M3' aufgeklärt:** es wurden zwei Zahlen aus
zwei verschiedenen Maschinen-Sitzungen verglichen, deren Sitzungs-Offset
größer ist als der behauptete Effekt. M4 und M3' haben je **einen** Lauf
notiert; bei 9 % Variationskoeffizient und 41 % Spannweite ist ein
Einzellauf-p95 kein vergleichbarer Wert.

### T7.3 Kandidat (a) — die acht FK-Indizes: ausgeschlossen

Zwei unabhängige Belege, beide negativ:

**1. Die Baseline-DB hatte die Indizes bereits.** `poc/measure/run.sh:34`
spielt vor dem M2-Ingest `poc/measure/fk_indexes.sql` ein — acht Indizes auf
**exakt denselben acht Spalten**, die Task 2 später ins Serienschema
aufgenommen hat, nur unter anderen Namen (`m_idx_*` statt `idx_*`):
`name(basionym_id)`, `taxon_concept(parent_id | accepted_name |
backbone_id)`, `concept_name(name_id)`, `xref(concept_id)`,
`fts_name_map(concept_id)`, `concept_relation(to_concept)`. Die M4-Baseline
wurde also **bereits mit diesen Indizes gemessen**. Task 2 hat sie nicht
hinzugefügt, sondern das Mess-Werkzeug ins Serienschema überführt. Ein
Query-Plan-Wechsel „durch die neuen Indizes" kann zwischen M4 und M3' gar
nicht stattgefunden haben.

**2. `EXPLAIN QUERY PLAN` ist mit und ohne die Indizes identisch.** Geprüft
gegen eine Kopie von `m1real.sqlite`, aus der die acht Indizes per
`DROP INDEX` entfernt wurden, mit der echten Suggest-Query aus
`internal/adapters/sqlite/suggest.go` (Präfix `"ca"*`, dem langsamsten der
38 Präfixe), in beiden Varianten — mit `in_area`-`EXISTS` und ohne:

```
mit den 8 FK-Indizes            |  ohne die 8 FK-Indizes
--------------------------------+--------------------------------
QUERY PLAN                      |  QUERY PLAN
|--MATERIALIZE matches          |  |--MATERIALIZE matches
|  `--SCAN fts_name VIRTUAL     |  |  `--SCAN fts_name VIRTUAL
|      TABLE INDEX 0:M2         |  |      TABLE INDEX 0:M2
|--SCAN m                       |  |--SCAN m
|--SEARCH fnm USING INTEGER     |  |--SEARCH fnm USING INTEGER
|      PRIMARY KEY (rowid=?)    |  |      PRIMARY KEY (rowid=?)
|--SEARCH tc USING INDEX        |  |--SEARCH tc USING INDEX
|      sqlite_autoindex_        |  |      sqlite_autoindex_
|      taxon_concept_1 (id=?)   |  |      taxon_concept_1 (id=?)
|--SEARCH an USING INDEX        |  |--SEARCH an USING INDEX
|      sqlite_autoindex_name_1  |  |      sqlite_autoindex_name_1
|      (id=?)                   |  |      (id=?)
|--USE TEMP B-TREE FOR GROUP BY |  |--USE TEMP B-TREE FOR GROUP BY
|--CORRELATED SCALAR SUBQUERY 2 |  |--CORRELATED SCALAR SUBQUERY 2
|  `--SEARCH d USING COVERING   |  |  `--SEARCH d USING COVERING
|      INDEX sqlite_autoindex_  |  |      INDEX sqlite_autoindex_
|      distribution_1           |  |      distribution_1
|      (concept_id=? AND        |  |      (concept_id=? AND
|       area_scheme=? AND       |  |       area_scheme=? AND
|       area_code=?)            |  |       area_code=?)
`--USE TEMP B-TREE FOR ORDER BY |  `--USE TEMP B-TREE FOR ORDER BY
```

**Die Pläne sind zeichengleich, und keiner der acht FK-Indizes taucht in
einem der Pläne auf.** Der Grund ist strukturell und nicht überraschend: der
Suggest-Pfad joint ausschließlich über Primärschlüssel und `rowid`
(`fnm.rowid = m.rowid`, `tc.id = fnm.concept_id`, `an.id =
tc.accepted_name`) — er sucht nie über eine der indizierten FK-Spalten in
Rückwärtsrichtung. Die FK-Indizes existieren für die
Constraint-Prüfung beim `INSERT OR REPLACE` (M1-Befund), nicht für Lesepfade.

Auch die Latenz bestätigt es: `post/noidx` misst 257,92 / 265,45 ms — innerhalb
(am unteren Rand) des Bandes der indizierten Variante, bei nur zwei Läufen
also nicht von Rauschen unterscheidbar. **Kandidat (a) ist ausgeschlossen.**

### T7.4 Kandidat (b) — die zusätzlichen OTHER-Rang-Zeilen: ausgeschlossen

Die +436 Konzepte, die T1 neu zulässt, sind
`NOTHOSUBSPECIES` (370), `NOTHOVARIETY` (54), `SUBVARIETY` (8),
`NOTHOFORM` (4). Ihr Beitrag zu genau den Präfixen, die den p95 dominieren
(`DISTINCT`-Konzepte je FTS5-Präfixtreffer, Baseline-DB → Nach-Hardening-DB):

| Präfix | `m2.sqlite` | `m1real.sqlite` | Differenz |
|---|---:|---:|---:|
| `ca` | 46.138 | 46.249 | +111 (+0,24 %) |
| `al` | 22.653 | 22.765 | +112 (+0,49 %) |
| `sa` | 20.938 | 20.995 | +57 (+0,27 %) |
| `tr` | 21.074 | 21.130 | +56 (+0,27 %) |
| `ac` | 16.787 | 16.841 | +54 (+0,32 %) |

Auf Zeilenebene der FTS5-Tabelle (nicht Konzepte) für den teuersten Präfix
`ca`: 98.749 → 100.029 Zeilen, **+1,30 %**. Keine dieser Zahlen kann einen
25-%-Effekt tragen. Der direkte Test ist ohnehin schon gelaufen:
`post/m2base` (HEAD-Code, Baseline-DB **ohne** diese Zeilen) und
`post/m1real` (HEAD-Code, DB **mit** ihnen) liegen mit 273,19 vs. 285,58 ms
Median in überlappenden Bändern. **Kandidat (b) ist ausgeschlossen.**

### T7.5 Ein geprüfter, aber wirkungsloser Fix: `ANALYZE`

Weil `sqlite_stat1` in keiner der DBs existierte, lag „dem Planer
Statistiken geben" als billiger Fix nahe. Geprüft, mit einem lehrreichen
Zwischenergebnis: `ANALYZE` läuft in **4,7 s** auf der 916-MiB-DB und
erzeugt 20 `sqlite_stat1`-Zeilen; der Query-Plan bleibt danach
**unverändert** (zeichengleich mit T7.3). Die ersten zwei Latenzläufe auf
der frisch analysierten Kopie sahen mit p95 = 235,24 / 225,27 ms trotzdem
deutlich besser aus als alles andere.

Das war ein Artefakt — und zwar genau die Art Artefakt, die diesen ganzen
offenen Punkt erzeugt hat. Ein anschließender **verschränkter A/B-Test**
gegen eine ebenso frische, aber **nicht** analysierte Kopie derselben DB
(`copyctl`, Reihenfolge copyctl → analyzed → copyctl → analyzed):

| Lauf | Konfiguration | p95 |
|---|---|---:|
| 1 | `copyctl` (ohne ANALYZE) | 289,70 ms |
| 2 | `analyzed` | 303,67 ms |
| 3 | `copyctl` (ohne ANALYZE) | 316,39 ms |
| 4 | `analyzed` | 280,16 ms |

**Kein Effekt.** Die vier `analyzed`-Läufe streuen über 225,27 – 303,67 ms,
also über fast das gesamte Band aller Konfigurationen. Die frühen guten
Werte kamen aus der Umgebung (frisch geschriebene, zusammenhängende
Dateikopie plus warmer OS-Page-Cache unmittelbar nach `cp`), nicht aus
`ANALYZE`. **`ANALYZE` wird deshalb nicht eingebaut** — es ändert den Plan
nicht und bringt messbar nichts; es ins Schema oder in den Ingest zu
nehmen, wäre eine unbelegte Änderung an einem funktionierenden Pfad.

### T7.6 Was daraus folgt

- **Kein Code wurde geändert.** Es gibt nichts zu reparieren: die
  gemessene „Regression" existiert nicht.
- **Der p95 dieses Aufbaus ist keine Einzellauf-Kennzahl.** Wer M4/M3'
  künftig fortschreibt, muss mindestens 3–5 Läufe machen und ein Band
  berichten, keinen Punktwert. Der p50 ist dagegen belastbar (3,3 %
  Variationskoeffizient) und über alle 19 Läufe und alle vier
  Konfigurationen stabil bei 34,5–38,7 ms — er bestätigt das ursprüngliche
  M4-Verdikt.
- **Die reale Auflage bleibt die Präfixlänge**, unverändert seit M4: die
  2-Zeichen-Präfixe (`ca`, `al`, `sa`, `tr`) erzeugen 20k–46k
  Kandidaten-Konzepte und dominieren jede p95-Zahl. Wer den p95 wirklich
  senken will, muss dort ansetzen (z. B. Mindestpräfixlänge im Frontend,
  clientseitiges Debouncing oder ein Deckel auf der FTS5-Kandidatenmenge) —
  nicht an Indizes oder Planer-Statistiken. Das ist eine
  Produktentscheidung und wird hier nicht getroffen.

Reproduktion:

```bash
nix develop -c go build -o /tmp/p95/hostus ./cmd/hostus
nix develop -c bash -c 'cd poc && go build -o /tmp/p95/latency ./measure/latency'
# je Lauf: Server starten, messen, Server beenden
HOSTUS_SQLITE_PATH=poc/measure/out/m1real.sqlite /tmp/p95/hostus serve --port 8201 --log-level warn &
/tmp/p95/latency --base http://127.0.0.1:8201 --reps 15 --warmup 3
# Vergleichs-DB ohne die acht FK-Indizes:
cp poc/measure/out/m1real.sqlite /tmp/p95/noidx.sqlite
sqlite3 /tmp/p95/noidx.sqlite "DROP INDEX idx_name_basionym_id; \
  DROP INDEX idx_taxon_concept_parent_id; DROP INDEX idx_taxon_concept_accepted_name; \
  DROP INDEX idx_taxon_concept_backbone_id; DROP INDEX idx_concept_name_name_id; \
  DROP INDEX idx_xref_concept_id; DROP INDEX idx_fts_name_map_concept_id; \
  DROP INDEX idx_concept_relation_to_concept;"
# Baseline-Konfiguration (Code vor Hardening):
git worktree add /tmp/p95/pre 53575fe && (cd /tmp/p95/pre && go build -o /tmp/p95/hostus-pre ./cmd/hostus)
HOSTUS_SQLITE_PATH=poc/measure/out/m2.sqlite /tmp/p95/hostus-pre serve --port 8411 --log-level warn &
/tmp/p95/latency --base http://127.0.0.1:8411 --reps 15 --warmup 3
```

---

## Task 6: konsolidierte Vorher/Nachher-Übersicht

Eine Tabelle, jede Zahl mit dem Abschnitt, der sie erzeugt hat — keine Zahl
hier ist neu gemessen, jede stammt aus M1–M6/T5 oben.

| Kennzahl | Vorher (Serienstand) | Nachher (Hardening) | Quelle |
|---|---|---|---|
| Ingest, unverändertes WCVP-Archiv | **bricht ab** nach 5,37 s (unbekannter Rang) | **läuft durch**, 281,27 s | M1.0 / M1' |
| Ingest, gefiltertes Archiv, Serienschema | quadratisch, nach 22 min 48 s manuell abgebrochen, 0 Zeilen persistiert | — (per M1.2a durch FK-Indizes ersetzt) | M1.1 |
| Skalierung 50k/100k/200k Taxon-Zeilen (Serienschema) | 65 / 293 / 1.338 s | **6 / 11 / 24 s** | M1.2 / M1.2a |
| WCVP-Konzepte (`taxon_concept`) | 440.098 (gefiltertes Archiv) | **440.534** (unverändertes Archiv, +436) | M1.3 / M1' |
| Crosswalk auflösbar, Taxon-Ebene — EIVE | 87,84 % (13.026/14.830) | **97,96 %** (14.527/14.830) | M2' / T5.3 |
| Crosswalk auflösbar, Taxon-Ebene — Tichý | 95,73 % (8.527/8.907) | **98,82 %** (8.802/8.907) | M2' / T5.3 |
| Crosswalk auflösbar, Taxon-Ebene — Midolo | 96,41 % (6.153/6.382) | **99,11 %** (6.325/6.382) | M2' / T5.3 |
| Unmatched-Zeilen — EIVE | 8.830 (12,39 %) | **1.445** (−83,6 %) | M2' / T5.2 |
| Unmatched-Zeilen — Tichý | 1.868 (4,10 %) | **526** (−71,8 %) | M2' / T5.2 |
| Unmatched-Zeilen — Midolo | 1.145 (3,59 %) | **285** (−75,1 %) | M2' / T5.2 |
| Ambiguous-Zeilen — EIVE | 7.824 | **8.961** (+1.137 — jedes vorher unmatched) | M2' / T5.2 |
| Ambiguous-Zeilen — Tichý | 7.170 | **7.373** (+203) | M2' / T5.2 |
| Ambiguous-Zeilen — Midolo | 5.780 | **5.930** (+150) | M2' / T5.2 |
| Bundle GER (entpackt) | 108,9 MB (103,8 MiB) | **84.987.904 Byte (81,05 MiB)** | M5.2 / M5.3 |
| Bundle GER (`gzip -9`) | 20,5 MiB | **19,24 MiB** | M5.2 / M5.3 |
| Bundle Mitteleuropa DE/AT/CH (entpackt) | nicht ausdrückbar (`--area` nur Einzelwert) | **93.450.240 Byte (89,1 MiB)** | M5.1 / M5.3 |
| Bundle Mitteleuropa DE/AT/CH (`gzip -9`) | — | **21.953.315 Byte (20,9 MiB)** | M5.3 |
| Voll-Bundle (ungescopt) | scheitert am SQLite-Parameterlimit, 0 Byte | **erfolgreich, 928.059.392 Byte (885,2 MiB), 986,65 s** | M5.1 / M5.3 |
| Suggest p50 (ohne `area`) | 36,4 ms | 38,59 ms | M4 / M3' |
| Suggest p95 (ohne `area`) | 220,2 ms | 274,37 ms — **keine Regression**: 19 Wiederholungsläufe spannen 225–316 ms, die Baseline-Konfiguration selbst misst heute 262–310 ms (Median 298 ms) | M4 / M3' / **T7.1–T7.2** |
| Suggest p50 (`area=GER`) | 38,7 ms | 40,81 ms | M4 / M3' |
| Suggest p95 (`area=GER`) | 253,8 ms | 321,57 ms — **keine Regression**: Baseline-Konfiguration heute 310/361 ms, M3'-Konfiguration 342/377 ms, überlappende Bänder | M4 / M3' / **T7.2** |
| Suggest p50, Streuung über 19 Läufe (ohne `area`) | (nicht gemessen, ein Lauf) | **34,54–38,73 ms**, Variationskoeffizient 3,3 % | T7.1 |
| Suggest p95, Streuung über 19 Läufe (ohne `area`) | (nicht gemessen, ein Lauf) | **225,27–316,39 ms**, Variationskoeffizient 9,0 %, max/min 1,41 | T7.1 |
| Lizenzbrücken, real nutzbarer Gewinn — EIVE | 51 Taxa (0,34 % von 14.830) *vor* Normalisierung | **2 Taxa** (0,01 %) *nach* Normalisierung, gegen die auf 303 geschrumpfte Restmenge | M6 / Task 6 §„Lizenzempfehlung" unten |
| Lizenzbrücken, real nutzbarer Gewinn — Tichý | 3 Taxa | **1 Taxon**, gegen 105 verbleibende | M6 / Task 6 |
| Lizenzbrücken, real nutzbarer Gewinn — Midolo | 2 Taxa | **1 Taxon**, gegen 57 verbleibende | M6 / Task 6 |
| A1: gespeicherte `trait_value`-Zeilen, die sich durch die explizite Regel-Rangfolge ändern | (Defekt bestand seit T5.5) | **10 von 117.153** (alle EIVE, alle geflaggt→ungeflaggt) | Task 6 §A1 oben |
| A2: exakt aufgelöste Konzepte je Vokabular nach Normalisierung == M2'-Baseline | unbelegte Behauptung (T5.5 Prosa) | **maschinell bestätigt**: 11.000 / 7.072 / 4.963 | Task 6 §A2 oben |

## Task 6: Verdikte nach Hardening

Frühere Verdikte bleiben als Historie sichtbar; „→" markiert den Stand nach
diesem Task.

- **M1 (Voller Ingest).** Vorher: **hält nicht** (M1.0/M1.1: Absturz bzw.
  quadratischer Abbruch, 0 Zeilen persistiert). Nach Task 2/3 (M1.2a/M1'):
  **hält** — derselbe unveränderte WCVP-Volldatensatz läuft in 281,27 s
  durch, mit linearer statt quadratischer Skalierung (6/11/24 s statt
  65/293/1.338 s) und ohne manuellen Zwischenschritt. **→ hält.**
- **M2 (Crosswalk-Trefferquote).** Vorher: **hält mit Auflagen**
  (87,76–96,41 % auflösbar, Ambiguous-Anteil als Auflage benannt). Nach
  Task 5 (Normalisierung, T5.3): 97,96–99,11 % auflösbar — die Auflage
  „strukturelle Nichttreffer" ist zum großen Teil eingelöst, die zweite
  Auflage (Ambiguous-Anteil, jetzt sogar leicht gestiegen, weil
  Normalisierung auch neue Mehrdeutigkeiten aufdeckt, siehe T5.4) bleibt
  bestehen: der Dienst rät weiterhin nicht bei mehrdeutiger Auflösung, das
  kostet Abdeckung, kauft aber Korrektheit. **→ hält mit Auflagen** (die
  verbleibende Auflage ist jetzt Ambiguous, nicht mehr strukturelle
  Nichttreffer).
- **M3 (Alternative Auflösungsziele / Lizenzbrücken, Namensebene).** Vorher:
  **hält nicht als eigenständiges Argument** (Obergrenze, keine Prognose;
  siehe M6). Nach Normalisierung unverändert methodisch fragwürdig als
  Namensebene-Zahl — und durch M6 (Task 6, unten) inhaltlich noch schwächer
  geworden, weil der M6-Gewinn selbst eingebrochen ist. **→ hält nicht.**
- **M4 (Suggest-Latenz).** Vorher: **hält** (p50 36,4 ms, p95 220,2 ms).
  M3' hatte einen p95-Anstieg um 25–27 % berichtet und die Ursache
  ausdrücklich offengelassen. **Task 7 hat das nachgemessen: es gibt keine
  Regression.** 19 Wiederholungsläufe desselben Aufbaus streuen im p95 über
  225,27–316,39 ms (Variationskoeffizient 9,0 %, max/min 1,41), und die
  **Baseline-Konfiguration selbst** — Code vor dem Hardening (`53575fe`)
  gegen die Baseline-DB `m2.sqlite` — misst heute 262–310 ms (Median
  297,88 ms), also *schlechter* als die Nach-Hardening-Konfiguration
  (Median 285,58 ms). M4 und M3' haben je einen Einzellauf notiert; deren
  Differenz ist kleiner als die Streuung des Aufbaus. Die beiden
  benannten Sachursachen sind belegt ausgeschlossen: die acht FK-Indizes
  **standen bereits in der Baseline-DB** (`poc/measure/fk_indexes.sql`,
  dieselben acht Spalten) und tauchen in *keinem* Query-Plan des
  Suggest-Pfads auf — die Pläne mit und ohne sie sind zeichengleich (T7.3);
  die von T1 zugelassenen OTHER-Rang-Zeilen erhöhen die
  Kandidatenmenge der teuersten Präfixe um 0,24–0,49 % (T7.4). Der p50,
  die einzige stabil messbare Kennzahl dieses Aufbaus (3,3 %
  Variationskoeffizient), ist über alle Konfigurationen unverändert bei
  34,5–38,7 ms. **→ hält** (unverändert wie vorher; die Auflage ist wieder
  allein die Präfixlänge, wie schon in M4 beschrieben — kurze
  2-Zeichen-Präfixe erzeugen 20k–46k Kandidaten und dominieren jede
  p95-Zahl).
- **M5 (Bundle-Größe).** Vorher: **hält nicht** (108,9 MB statt 10–20 MB,
  Faktor 5,4; Mehrgebiets-Export und Voll-Export unmöglich). Nach Task 4:
  Mehrgebiets-Scoping (`--area DE,AT,CH`) und das Parameterlimit sind
  vollständig behoben (beide Bugs vorher „hält nicht", jetzt geschlossen).
  Die Größen-Erwartung „10–20 MB" hält weiterhin nicht (GER entpackt jetzt
  81,05 MiB statt 103,8 MiB — eine reale Verbesserung von ~22 %, aber immer
  noch Faktor ~4–4,5 über der Spec-Zahl), doch die Lücke ist jetzt gemessen
  und ursächlich auf einen expliziten Kompromiss zurückgeführt (Synonyme/
  Namen bleiben vollständig, das ist eine reale UC1-Fähigkeit, keine
  Verschnitt-Verschwendung), nicht mehr eine unerklärte Abweichung.
  Komprimiert (`gzip -9`) liegt GER bei 19,24 MiB — **unter** der
  20-MB-Obergrenze; Mitteleuropa (DE/AT/CH) bei 20,9 MiB — knapp **darüber**.
  **→ hält mit Auflagen** (beide vorherigen „hält nicht"-Bugs behoben; die
  Größen-Erwartung selbst ist als Design-Kompromiss dokumentiert statt
  offen, aber die 10–20-MB-Zahl bleibt unerreicht für die entpackte
  On-Device-Größe).
- **M6 (Lizenzbrücken, Konzept-Ebene — die belastbare Zahl).** Vorher:
  **hält nicht als Rechtfertigung für die Lizenzgespräche** (realer Gewinn
  51/3/2 Taxa, 0,34 % von EIVE). Nach Normalisierung (Task 6, neu
  gemessen, siehe unten): der reale Gewinn schrumpft weiter auf 2/1/1 Taxa
  — nicht weil die Brücken schlechter geworden wären, sondern weil
  Normalisierung fast alles bereits eingesammelt hat, was die Brücken
  vorher beigetragen hätten. **→ hält nicht — und die Lücke zum
  „lohnt sich" ist jetzt noch größer als vorher gemessen.**

## Task 6: was jetzt zu entscheiden ist (aktualisiert)

1. **Die zwei Ingest-Blocker: erledigt.** Task 2/3 haben sie behoben und
   M1' bestätigt es am unveränderten WCVP-Volldatensatz. Kein offener
   Punkt mehr.
2. **Bundle-Größe/Mehrgebiets-Scoping: erledigt, mit dokumentiertem
   Rest-Trade-off.** Task 4 hat beide Bugs (Einzelwert-`--area`,
   Parameterlimit) behoben und die Größe um ~22 % gesenkt. Offen bleibt
   eine reine Produktentscheidung, kein Bug: ob ein synonymfreies
   „schlankes" Bundle-Profil als ZUSÄTZLICHE Option angeboten werden soll
   (kostet die UC1-Fähigkeit, einen Synonymnamen einzutippen) — das ist
   hier bewusst nicht entschieden, weil es eine Scope-Frage ist, keine
   Reparatur.
3. **p95-Latenzregression: erledigt — es gab keine.** Task 7 (Sektion oben)
   hat die Varianz mit 19 Wiederholungsläufen quantifiziert (p95
   225–316 ms, max/min 1,41), die Baseline-Konfiguration nachgestellt
   (Code `53575fe` + `m2.sqlite`: heute 262–310 ms statt der notierten
   220,2 ms) und beide Sachursachen per `EXPLAIN QUERY PLAN` und
   Datenzählung ausgeschlossen. Kein Code geändert, kein offener Punkt.
   Was bleibt, ist eine **Methodenauflage, keine Reparatur**: p95-Zahlen
   dieses Aufbaus sind nur als Band aus mindestens 3–5 Läufen belastbar;
   Einzellauf-p95 dürfen nicht mehr über Läufe hinweg verglichen werden.
   Wer den p95 tatsächlich senken will, muss an der Präfixlänge ansetzen
   (Mindestlänge/Debouncing im Frontend oder ein Deckel auf der
   FTS5-Kandidatenmenge) — das ist eine offene **Produktentscheidung**,
   kein Defekt.
4. **Residuale Unmatched/Ambiguous-Zeilen: kleiner, aber nicht null.**
   Nach Normalisierung bleiben 1.445/526/285 Zeilen unmatched (meist
   binäre Hybridformeln, nicht-nominate Unterarten ohne WCVP-Zeile, echte
   Nichtvorkommen, Fuzzy-Territorium — siehe T5.7) und 8.961/7.373/5.930
   ambiguous (T5.4: Löwenanteil aus dem Aggregat-Rückfall). Der
   Ambiguous-Anteil ist jetzt größer als der Unmatched-Anteil bei allen
   drei Vokabularen — die in M2 identifizierte Review-Warteschlange
   (Spec §D.4) ist real und durch Normalisierung nicht kleiner geworden,
   sondern (T5.4) leicht gewachsen, weil neue Kandidatenschlüssel auch neue
   Mehrdeutigkeiten aufdecken können. Das ist kein Rückschritt — die vorher
   unmatched-Zeilen wurden nie geraten und werden es weiterhin nicht — aber
   der nächste Hebel für Abdeckung liegt jetzt eher in Disambiguierung
   (Rang-/Autorschaftsabgleich, wie M6 unten schon andeutete) als in
   weiterer Namensnormalisierung.
5. **Das Lizenzgespräch: die Zurückstellungs-Empfehlung ist jetzt
   STÄRKER, nicht nur bestätigt.** M6 hatte den realen Brücken-Gewinn vor
   Normalisierung mit 51 EIVE-/3 Tichý-/2 Midolo-Taxa beziffert (gegen die
   damalige Restmenge von 1.815/380/229 unaufgelösten Taxa) und empfohlen,
   das Lizenzgespräch zurückzustellen, weil Normalisierung der günstigere
   Hebel sei. Diese Empfehlung wurde jetzt **nachgemessen, nicht nur
   bestätigt**: gegen die auf 303/105/57 geschrumpfte Restmenge (T5.3, nach
   Normalisierung) liefern dieselben vier Quellen nur noch **2 EIVE-, 1
   Tichý-, 1 Midolo-Taxon** real bis zu einem WCVP-Konzept brückbar
   (`poc/measure/bridge --normbridge`, siehe Befehl unten). Normalisierung
   hat also nicht nur selbst Abdeckung gebracht, sondern **fast den
   gesamten Betrag eingesammelt, den die Lizenzbrücken sonst beigetragen
   hätten** — von 51 auf 2 bei EIVE, dem mit Abstand größten Kandidaten.
   **Die Empfehlung aus M6 ist damit deutlich stärker geworden**: das
   Lizenzgespräch für Euro+Med/EuroSL/GermanSL/FloraVeg als
   Datenqualitäts-Hebel zu führen, lohnt sich nach diesem Befund noch
   weniger als vorher — der noch offene Rest ist so klein (2/1/1 Taxa),
   dass selbst ein erfolgreiches Lizenzgespräch für alle vier Quellen
   praktisch keinen messbaren Effekt mehr hätte. Reproduktion:

   ```bash
   nix develop -c bash -c 'cd poc && go build -o ../poc/measure/out/bridge ./measure/bridge'
   ./poc/measure/out/bridge --normbridge --db poc/measure/out/t5real.sqlite \
     --vocab eive=pipelines/eive/output/eive-canonical.csv \
     --vocab tichy=pipelines/tichy/output/tichy-canonical.csv \
     --vocab midolo=pipelines/midolo/output/midolo-canonical.csv \
     --list euromed=pipelines/euromed/output/euromed-canonical.csv \
     --list eurosl=pipelines/eurosl/output/eurosl-canonical.csv \
     --list germansl=pipelines/germansl/output/germansl-canonical.csv \
     --list floraveg=pipelines/floraveg/output/floraveg-canonical.csv
   ```

## SP4 Task 2 — Xref-Ingest (ID-basierter Join): Deckung und Konflikte

**Aufbau.** `application.IngestXrefs` wurde gegen eine **Kopie** der echten
Volldatenbank (`/tmp/full-real.sqlite`, 440.534 Konzepte, 440.534 `powo`-Xrefs
aus dem WCVP-Ingest, Traits bereits eingespielt) gefahren, mit dem echten
Wikidata-Bridge-Hub-Export aus T1
(`pipelines/wikidata/output/wikidata-xref-canonical.csv`,
1.709.127 Zeilen / 393.172 distinkte Wikidata-Items/QIDs — **nicht** zu
verwechseln mit den distinkten `join_id`s, siehe die Korrektur im
SP4-Task-4-Abschnitt unten). Der Backbone-Ingest wurde
**nicht** wiederholt (~5 Min, laut Vorgabe zu vermeiden) — die Messung lief
als eigenständiges, nicht eingechecktes Go-Programm direkt gegen
`application.IngestXrefs`, mit anschließendem Cross-Check per direktem SQL
gegen dieselbe Kopie. Beide Wege stimmen exakt überein (siehe Tabelle unten),
das ist die Bestätigung, dass der Report nichts verschweigt.

### Konfliktregel (der Kern dieser Aufgabe)

`xref`s Primärschlüssel ist `(authority, ext_id)` — ein externer Datensatz
kann zu genau einem Konzept gehören. Zwei strukturell verschiedene
Situationen müssen getrennt behandelt werden:

- **(a) Echter Konflikt**: dieselbe `(authority, ext_id)`-Kombination wird
  von zwei UNTERSCHIEDLICHEN `join_id`s beansprucht, die auf zwei
  UNTERSCHIEDLICHE Konzepte auflösen (ein Datenfehler stromaufwärts — z. B.
  zwei IPNI-IDs, die Wikidata fälschlich derselben GBIF-ID zuordnet). Regel:
  **skip-and-report** — keine der beiden Zeilen wird geschrieben, beide
  zählen in `Conflicting`, der externe Schlüssel landet in `ConflictSample`.
  Das ist der von der Aufgabe vorgegebene sichere Default: raten, welches
  Konzept gemeint war, wäre falsch; und da `AddXref` ein `INSERT OR REPLACE`
  auf genau diesem Schlüssel ist, würde ohne diese Prüfung die zuletzt
  verarbeitete Zeile die andere stillschweigend überschreiben.
- **(b) Legitime Mehrfachzuordnung**: EIN Konzept bekommt mehrere
  UNTERSCHIEDLICHE `ext_id`s für dieselbe `authority` (z. B. zwei
  Wikidata-Items, die beide dieselbe IPNI-ID tragen, liefern zwei
  QIDs für dasselbe Konzept). Das ist **kein** Konflikt — `xref`s PK
  kollidiert nicht, da die `ext_id`s verschieden sind — sondern wird
  einfach **beide** geschrieben. `MultiPerAuthority`/`MultiSample` machen
  das Phänomen sichtbar, ohne es zu verhindern.

`PerAuthority` zählt bewusst **distinkte Konzepte**, nicht Zeilen — das ist
die für UC2 relevante Zahl (siehe unten).

### Messergebnis

| Kennzahl | Wert |
| --- | --- |
| Konzepte gesamt | 440.534 |
| Zeilen gesamt | 1.709.127 |
| Matched | 1.709.111 |
| Unmatched | 0 |
| Conflicting | 16 (8 externe Schlüssel × 2 Zeilen) |
| **Konzepte mit ≥1 neuem Xref** | **392.218 / 440.534 (89,03 %)** |

Direkter SQL-Cross-Check (`SELECT COUNT(*) FROM (SELECT concept_id FROM xref
GROUP BY concept_id HAVING COUNT(*) > 1)`) liefert exakt dieselbe Zahl,
392.218 — der Report und die Datenbank stimmen überein.

**Deckung pro Autorität** (distinkte Konzepte, Report und Direkt-SQL
identisch):

| authority | Konzepte | Anteil |
| --- | --- | --- |
| wikidata | 392.218 | 89,03 % |
| gbif | 383.907 | 87,15 % |
| wfo | 365.731 | 83,02 % |
| colxr | 357.878 | 81,24 % |
| **inat** | **182.821** | **41,50 %** |
| floraveg | 24.274 | 5,51 % |
| euromed | 95 | 0,02 % |

**Die für UC2 entscheidende Zahl: 182.821 von 440.534 Konzepten (41,50 %)
tragen eine iNaturalist-`taxon_id`.** Das ist die Obergrenze, mit der UC2
("von einem hostus-Konzept zu iNaturalist-Beobachtungen") tatsächlich
arbeiten kann — für die übrigen 58,5 % der Konzepte hat der Bridge-Hub keinen
iNat-Datensatz gefunden. Das ist ein niedriger Befund, kein Fehlschlag: die
Wikidata-Brücke bewegt sich zwischen "fast vollständig" (wikidata selbst,
gbif, wfo, colxr — alle über 80 %) und "eng" (inat bei 41,5 %, floraveg bei
5,5 %, euromed praktisch nicht vorhanden bei 0,02 %). Für UC2 heißt das
konkret: **weniger als die Hälfte** aller Konzepte kann den in der
Spezifikation vorgesehenen Direktlink zu iNaturalist-Beobachtungen anbieten;
für die anderen 257.713 Konzepte müsste ein Client entweder auf eine andere
Autorität ausweichen oder dem Nutzer ehrlich "keine iNat-Verknüpfung
gefunden" anzeigen.

**Konflikte (a):** 16 Zeilen (8 externe Schlüssel, je zwei Zeilen), alle bei
`gbif` (5 Schlüssel) und `wfo` (3 Schlüssel) — keine bei `inat`, `colxr`,
`wikidata`, `floraveg` oder `euromed`. Beispiele aus `ConflictSample`:
`gbif:11378793`, `gbif:2783144`, `wfo:wfo-0000100690`. Das sind Fälle, in
denen zwei verschiedene IPNI/POWO-IDs im Wikidata-Export auf dieselbe
GBIF- bzw. WFO-ID verweisen — bei 1,7 Mio. Zeilen ein verschwindend kleiner
Anteil (0,0009 %), aber genau der Fall, den die Skip-and-report-Regel ohne
Raten auffängt.

**Mehrfachzuordnungen (b):** am häufigsten bei `wikidata` selbst (954
Konzepte mit ≥2 QIDs), gefolgt von `gbif` (635), `wfo` (299), `colxr` (39),
`inat` (63), `floraveg` (3) — überall im niedrigen einstelligen bis
niedrigen dreistelligen Promillebereich der jeweils erreichten Konzepte,
also ein Rand-, kein Regelfall.

**Unmatched: 0 von 1.709.127 Zeilen — wie durch den joinable-subset-Filter
der Pipeline garantiert.** Die kanonische CSV wird gegen
`.cache/powo_ext_ids.txt` gebaut, einen Dump genau der `xref.powo`-IDs
dieser Datenbank (`pipelines/wikidata/build.sh`, `convert.py`); jedes
emittierte `join_id` ist damit per Konstruktion Element von `xref.powo`,
Unmatched kann gar nichts anderes als 0 sein. Der Wert validiert den
Ingest-Join (die ID-Auflösung in `IngestXrefs` findet tatsächlich jede
Zeile wieder), **nicht** die Abdeckung — und es ist dieselbe ID-Menge, nicht
eine zweite, unabhängige.

**Was das für UC2 bedeutet.** Die Wikidata-Brücke ist für GBIF/WFO/COL-XR/
Wikidata selbst eine sehr gute Ergänzung (81–89 % Konzeptdeckung), aber für
das von der Spezifikation genannte Zielsystem iNaturalist nur eine
Teillösung: 41,5 % Deckung bedeutet, dass UC2 für weniger als die Hälfte
der 440.534 Konzepte tatsächlich funktioniert. Ob das für den Produktschnitt
ausreicht oder eine zweite iNat-Anbindung (z. B. ein direkter Namens- oder
GBIF-basierter Join gegen den iNaturalist-Taxonomiedump) nötig wird, ist eine
Produktentscheidung — dieser Abschnitt liefert dafür die Zahl, nicht die
Entscheidung.

---

## SP4 Task 4 — Verdikt: Xref-Ingest end-to-end und UC2-Deckung

> Diese Sektion ist der Abschluss von SP4 (Task 4 von 4). Sie fasst die in
> SP4 Task 2 (oben, "SP4 Task 2 — Xref-Ingest …") an einer Kopie der vollen
> 440.534-Konzept-Datenbank gemessenen Zahlen für die Abnahme zusammen und
> liefert das für dieses Dokument fällige Verdikt. Die Zahlen unten sind
> **nicht neu gemessen** — es ist derselbe Lauf (`application.IngestXrefs`
> gegen `pipelines/wikidata/output/wikidata-xref-canonical.csv`, 1.709.127
> Zeilen) wie im Task-2-Abschnitt oben, hier für die Abnahme wiederholt: die
> Konzeptzahlen pro Autorität sind exakt die des Task-2-Abschnitts.

### Drei Zählweisen, ein Ergebnis: die Reconciliation

Beim Zusammenstellen dieser Sektion tauchten pro Autorität zunächst drei
unterschiedliche Zahlen auf, die leicht zu verwechseln sind, weil alle drei
plausibel nach "Deckung" aussehen. Sie sind hier bewusst benannt und
gegeneinander aufgelöst, statt eine davon unkommentiert stehen zu lassen:

| Zählweise | Was sie zählt | wikidata | gbif | wfo | colxr | inat | floraveg | euromed |
| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| distinkte `ext_id`s | wie viele verschiedene externe IDs die CSV je Autorität führt (Rohdaten, vor jedem Join) | 393.172 | 384.547 | 366.033 | 357.917 | 182.884 | 24.277 | 95 |
| distinkte `join_id`s | wie viele dieser Zeilen einen `join_id` tragen, der (vor Konfliktprüfung) im Index existiert | 392.218 | 383.917 | 365.737 | 357.878 | 182.821 | 24.274 | 95 |
| **tatsächlich geschriebene Konzepte (T2-Tabelle, maßgeblich)** | wie viele Konzepte NACH Konfliktprüfung tatsächlich einen `xref`-Datensatz bekommen haben | **392.218** | **383.907** | **365.731** | **357.878** | **182.821** | **24.274** | **95** |

Die erste Spalte (`ext_id`) liegt bei wikidata, gbif, wfo und colxr sichtbar
über den beiden anderen — das ist keine Unstimmigkeit, sondern schlicht die
Rohzahl vor dem ID-Join: nicht jede externe ID im CSV trägt zwangsläufig
einen bei hostus bekannten `join_id`. Die zweite und dritte Spalte
unterscheiden sich dagegen genau um die Typ-(a)-Konflikte: **`join_id` minus
geschriebene Konzepte = gbif 10 + wfo 6 = 16** — exakt die 16 übersprungenen
Konfliktzeilen (8 externe Schlüssel × 2 Zeilen) aus der Konfliktregel oben,
alle bei `gbif` und `wfo`. Bei den übrigen Autoritäten sind `join_id`- und
Konzeptzahl identisch, weil dort keine Konflikte auftraten. Nichts bleibt
unerklärt — das ist eine positive Konsistenzprobe für die Konfliktbehandlung
aus SP4 Task 2, kein offener Punkt: **die T2-Tabelle (dritte Zeile) ist die
maßgebliche, berichtete Deckungszahl** und wird unten unverändert verwendet.

### Der End-to-End-Beweis (Schritt 1+2 dieser Aufgabe)

`internal/app/integration_test.go`s
`TestIntegration_EndToEndIngestServeQuery` fährt jetzt den **echten** CLI-Pfad
(`app.Ingest` mit `testdata/dataset.yaml`, das seit T2 einen `xref_sources`-
Eintrag auf `internal/adapters/xref/testdata/wikidata-sample.csv` trägt) und
prüft über echtes HTTP:

- `GET /v1/concept/{corynephorus}` liefert **sechs** zusätzliche Autoritäten
  (`wikidata`, `gbif`, `colxr`, `floraveg`, `wfo`, `inat`) neben dem
  WCVP-nativen `powo`-Xref auf demselben Konzept — "mehrere Autoritäten" ist
  damit über echtes HTTP bewiesen, nicht nur am In-Process-Handler-Test aus
  SP4 Task 3.
- `GET /v1/xref?authority=inat&id=160927` löst zur **selben** Konzept-ID
  auf, die die Concept-Antwort trägt — der Rückweg funktioniert.

```bash
nix develop -c make test-integration
```

```
github.com/jobrunner/hostus/internal/app:
 ✓ Integration end to end ingest serve query (0.02s)
 ✓ Integration offline bundle concept suggest traits offline (0.01s)
 ✓ Integration offline bundle serves suggest offline (0.02s)
 ✓ Integration traits fuzzy classification (0.01s)

DONE 4 tests in 1.232s
```

### Die Kennzahlen (Volldatensatz, T2-Konzeptzahlen)

Erzeugt durch `application.IngestXrefs` gegen eine Kopie der vollen
440.534-Konzept-/440.534-`powo`-Xref-Datenbank, mit dem echten
Wikidata-Bridge-Export aus T1 (1.709.127 Zeilen), gegengeprüft per direktem
SQL gegen dieselbe Kopie:

```sql
-- Konzepte gesamt mit ≥1 neuem Xref
SELECT COUNT(*) FROM (SELECT concept_id FROM xref GROUP BY concept_id HAVING COUNT(*) > 1);
-- 392218

-- Deckung pro Autorität (distinkte Konzepte)
SELECT authority, COUNT(DISTINCT concept_id) FROM xref GROUP BY authority;
```

| Kennzahl | Wert |
| --- | --- |
| Konzepte mit ≥1 neuem Xref | **392.218 / 440.534 (89,03 %)** |
| wikidata | 392.218 |
| gbif | 383.907 |
| wfo | 365.731 |
| colxr | 357.878 |
| **inat** | **182.821 (41,50 %)** |
| floraveg | 24.274 |
| euromed | 95 |
| Konflikte (a) — dieselbe `(authority, ext_id)`, zwei Konzepte | 16 Zeilen / 8 externe Schlüssel, ausschließlich `gbif` und `wfo` |
| Mehrfachzuordnungen (b) — ein Konzept, mehrere `ext_id`s derselben Autorität | wikidata 954 · gbif 635 · wfo 299 · inat 63 · colxr 39 · floraveg 3 |
| Unmatched | **0 von 1.709.127 Zeilen** |

**Unmatched = 0** ist durch den joinable-subset-Filter der Pipeline
garantiert: die CSV enthält nur Zeilen, deren `join_id` aus dem
`xref.powo`-Dump derselben Datenbank stammt. Das validiert den Ingest-Join,
nicht die Abdeckung.

### Was das für UC2 bedeutet

**182.821 von 440.534 Konzepten (41,50 %) tragen eine iNaturalist-`taxon_id`.
Das ist UC2s harte Obergrenze.** Für die übrigen 257.713 Konzepte (58,5 %)
gibt es **keinen** iNat-Link — nicht "schwer zu finden", sondern schlicht
nicht in der Wikidata-Brücke vorhanden. Das ist ein niedriger Befund, kein
Fehlschlag der Implementierung: `application.IngestXrefs` selbst arbeitet
korrekt (0 Unmatched, Konflikte sauber erkannt und übersprungen, Mehrfach-
zuordnungen sauber als solche markiert); die Grenze liegt in der
**Datenquelle**, nicht im Code.

Zum Vergleich: die anderen sechs Autoritäten liegen zwischen 89,03 % (wikidata
selbst) und 81–83 % (gbif/wfo/colxr) — deutlich über inat. `floraveg` (5,5 %)
und `euromed` (0,02 %, 95 Konzepte) sind noch schwächer als inat, aber UC2
hängt nicht an ihnen.

**Was das für UC2 kostet:** der in der Spezifikation vorgesehene
Direktlink hostus-Konzept → iNaturalist-Beobachtungen funktioniert für
weniger als die Hälfte aller Konzepte. Für die übrigen Konzepte muss ein
Client entweder (a) ehrlich "keine iNat-Verknüpfung gefunden" anzeigen, statt
zu raten, oder (b) auf eine der besser gedeckten Autoritäten ausweichen (z. B.
GBIF, 87 %) — was aber nur hilft, wenn die konsumierende Anwendung ohnehin
schon einen GBIF-Pfad zu iNaturalist-Beobachtungen kennt, was UC2 in der
Spezifikation nicht voraussetzt.

**Die Optionen (Produktentscheidung, nicht Teil dieser Aufgabe):**

1. **Teildeckung akzeptieren.** 41,50 % ist real nutzbar und kostenlos (kein
   zusätzlicher Ingest-Pfad); UC2 liefert für zwei von fünf Konzepten einen
   Link, für die übrigen einen ehrlichen "nicht gefunden"-Zustand.
2. **Zweiter iNat-Auflösungspfad, namensbasiert.** Ein direkter Join gegen
   den iNaturalist-Taxonomiedump (statt über die Wikidata-Brücke) könnte die
   Deckung erhöhen — Aufwand und tatsächlicher Zugewinn sind hier nicht
   gemessen, das wäre ein eigener Rechercheauftrag (vergleichbar mit SP4
   Task 1s Wikidata-Bridge-Aufbau).
3. **Nichts tun** und die 41,50 %-Grenze in der UC2-Dokumentation
   (`docs/how-to/inat-uc2.md`, SP4 Task 3) als bekannte Einschränkung stehen
   lassen.

Zusätzlich gelten für jeden über inat erreichten Datensatz weiterhin die in
SP4s PoC (P9) gemessenen Einschränkungen der iNaturalist-**Beobachtungsdaten**
selbst — unabhängig von der hier gemessenen Xref-Ingest-Deckung, weil hostus
nur Taxon-IDs ingestiert, keine Beobachtungen:

- Koordinaten `obscured` sind auf ~26–28 km verwischt.
- ~32–38 % aller Beobachtungen sind `obscured`; 62,6 % sind ohne
  Einschränkung nutzbar.
- `quality_grade=research` bedeutet **zwei Community-Zustimmungen**, keine
  fachliche Verifikation durch eine Expertin oder einen Experten.

Diese Zahlen werden hier nicht abgeschwächt: sie treffen jede der 182.821
über inat erreichbaren Konzepte, sobald ein Client tatsächlich zu
Beobachtungen navigiert, nicht nur zur Taxon-ID.

### Verdikt

**Hält mit Auflagen.** Der Xref-Ingest selbst (Code, Konfliktbehandlung,
End-to-End-Pfad von `hostus ingest` bis `GET /v1/xref`) ist korrekt und
vollständig bewiesen: 0 Unmatched, Konflikte sauber erkannt statt geraten,
Mehrfachzuordnungen sauber sichtbar gemacht, der komplette Pfad über echtes
HTTP nachgewiesen. Die Auflage betrifft nicht den Dienst, sondern die
Datenquelle: **UC2 ("hostus-Konzept → iNaturalist-Beobachtungen") erreicht
nur 41,50 % der Konzepte**, weil die Wikidata-Brücke für iNaturalist nur
diesen Anteil kennt. Das ist ehrlich als Deckungsgrenze zu kommunizieren,
nicht zu verschweigen — ein Produkt, das UC2 auf Basis dieser Brücke
ausliefert, muss den fehlenden Link für 58,5 % der Konzepte explizit als
"nicht gefunden" behandeln, nicht stillschweigend nichts anzeigen.

## SP6 Task 1 — `nom_status`/`published_in`: das tatsächlich gemessene Vokabular

Ausgangsbefund: der WCVP-Reader las `nomenclaturalstatus` und
`namepublishedin`, `domain.Name` hatte die Felder, der SQLite-Adapter schrieb
die Spalten — aber der Mapper dazwischen (`internal/application/ingest.go`,
und schon davor die DTO `application.TaxonRow` selbst) baute den Namen ohne
beide Werte. Ergebnis im echten Index: `nom_status` bei **0 von 1.448.984**
Namen gesetzt, `published_in` ebenso.

### Messung

Volle WCVP-DwC-A (`wcvp_taxon.csv`, 508 MB, 1.448.984 Namen / 440.534
Konzepte / 964.762 Synonyme), Ingest über `hostus ingest` in eine frische
SQLite-Datei, Laufzeit **5:03 min**. Also der komplette Korpus, keine
Stichprobe, keine Hochrechnung.

| Kennzahl | Wert |
| --- | --- |
| Namen gesamt | 1.448.984 |
| davon mit nicht-leerem `nom_status` | **99.252 (6,85 %)** |
| davon mit nicht-leerem `published_in` | **1.448.934 (99,997 %)** — nur 50 Namen ohne |
| distinkte `nom_status`-Werte | **1.304** |
| distinkte `published_in`-Werte | 853.152 |
| `basionym_id` gesetzt (Kontrollwert, unverändert) | 429.172 |

92.492 der 99.252 Namen mit `nom_status` stehen in der Rolle `synonym` —
das Feld trifft also genau die Menge, die UC5 filtern soll.

### Die drei Befunde, auf die sich Task 2 einstellen muss

**1. Jeder Wert trägt WCVP-eigenes `", "` als Präfix.** 99.111 von 99.252
Werten (99,86 %) beginnen mit Komma und Leerzeichen — WCVP hängt den Status
an ein Zitatfragment an und exportiert die Konkatenation. Der Ingest schreibt
den Wert bewusst verbatim; das Normalisieren ist Aufgabe der Vokabularschicht,
nicht des Imports.

**2. Das Vokabular ist kein Enum, sondern ein langer Freitextschwanz.** 1.304
distinkte Werte, davon 12 mit ≥ 1.000 Treffern, 28 mit ≥ 100, 1.225 mit < 10
(zusammen 1.812 Namen). Die zehn häufigsten decken 90,56 % ab, die Top 20
95,82 %, die Top 200 erst 98,75 %. Eine vollständige Abbildung aller 1.304
Werte ist weder nötig noch sinnvoll; ein `default: unbekannt` ist Pflicht.

Die 20 häufigsten Werte (verbatim, mit kumulierter Deckung):

| n | kumuliert | Wert |
| ---: | ---: | --- |
| 36.424 | 36,70 % | `, nom. illeg. homonym. post.` |
| 18.220 | 55,06 % | `, not validly publ.` |
| 10.768 | 65,91 % | `, nom. illeg. superfl.` |
| 9.210 | 75,18 % | `, nom. nud.` |
| 6.218 | 81,45 % | `, pro syn.` |
| 2.405 | 83,87 % | `, nom. illeg.` |
| 2.193 | 86,08 % | `, orth. var.` |
| 1.716 | 87,81 % | `, nom. superfl.` |
| 1.527 | 89,35 % | `, opus utique oppr.` |
| 1.202 | 90,56 % | `, nom. cons.` |
| 1.132 | 91,70 % | `, without a Latin descr.` |
| 1.115 | 92,82 % | `, sensu auct.` |
| 831 | 93,66 % | `, nom. rej.` |
| 471 | 94,14 % | `, without basionym ref.` |
| 443 | 94,58 % | `, without indication of the type.` |
| 362 | 94,95 % | `, nom. provis.` |
| 290 | 95,24 % | `, tentatively listed as a synonym.` |
| 236 | 95,48 % | `, nom. subnud.` |
| 199 | 95,68 % | `, sphalm.` |
| 142 | 95,82 % | `, without type.` |

**3. Der Beispielfall aus dem Entwurfsdokument trägt nicht den Wert, den das
Entwurfsdokument annimmt.** *Corynephorus incanescens* Bubani
(`wcvp:name:405842`) hat `nom_status = ", nom. illeg. superfl."`, **nicht**
`", nom. superfl."`. Ein Gleichheitsvergleich gegen die drei im Entwurf
genannten Werte (`nom. nud.`, `nom. superfl.`, `pro syn.`) würde genau den
Fall verfehlen, an dem UC5 erklärt wird — und zusätzlich die 10.768 Namen
mit `nom. illeg. superfl.` sowie die 36.424 mit `nom. illeg. homonym. post.`
Der Filter muss also auf Token-Enthaltensein prüfen, nicht auf Gleichheit.
Token-Trefferzahlen über den Volldatensatz:

| Token | Namen, deren `nom_status` es enthält |
| --- | ---: |
| `nom. illeg.` | 49.694 |
| `not validly publ.` | 18.606 |
| `nom. nud.` | 9.222 |
| `pro syn.` | 6.222 |
| `orth. var.` | 2.196 |
| `nom. superfl.` | 1.729 |
| `opus utique` | 1.640 |
| `nom. cons.` | 1.237 |
| `sensu auct.` | 1.117 |
| `nom. rej.` | 884 |
| `fossil name.` | 272 |

### Weitere Auffälligkeiten, die eine Einordnung brauchen

- **Mehrfachstatus in einer Zelle.** 684 Namen tragen mehr als einen
  kommaseparierten Status (664 mit zwei, 16 mit drei, 4 mit vier oder fünf),
  z. B. `, nom. illeg., later homonym`. Ein Filter, der die Zelle als einen
  einzigen Wert behandelt, verfehlt den zweiten Status.
- **Kommas kommen auch innerhalb eines Status vor**, etwa
  `, contrary to Art. 23.6. (ICN, 2012).` — naives Splitten an `,` zerlegt
  diese Werte falsch. Der ganze Zellwert ist die verlässlichere Einheit.
- **141 Werte sind gar kein Status, sondern ein Zitatfragment**, z. B.
  `[Cusc.: 184]`, `[Conv. Or.: 60]`, oder Freitext wie
  `published as "mutatio nova"` und `as stabilized hybrids derived from
  C. collina × C. ser. Molles`. Diese Zellen enthalten keine
  nomenklatorische Aussage; einige mischen beides (`[Cusc.: 183], nom.
  illeg.`).
- **Schreibvarianten desselben Sachverhalts** existieren nebeneinander:
  `without a Latin descr.` (1.132) / `without latin descr.` (33) /
  `without Latin descr.` (19) / `sine descr. lat.` (15);
  `not effectively publ.` (29) / `not effectively published.` (23);
  `nom. altern.` (103) / `nom. alt.` (35); `nom. rej.` (831) /
  `nom. rejic.` (9). Eine Abbildung muss diese zusammenführen oder bewusst
  getrennt lassen — raten darf sie nicht.
- **Nicht klassifizierbar ohne fachliche Entscheidung**, hier ausdrücklich
  markiert statt stillschweigend einsortiert: `, sensu auct.` (1.115) ist
  eine Fehlanwendung, kein nomenklatorischer Mangel; `, tentatively listed
  as a synonym.` (290) ist eine taxonomische Unsicherheit, keine
  Publikationsfrage; `, fossil name.` (272) und `, isonym` (9) sagen nichts
  über die Gültigkeit; `, not validly publ.?` (8) trägt ein Fragezeichen
  mitten im Wert. Ob diese Fälle in UC5 ausgeschlossen werden, ist eine
  fachliche und keine technische Entscheidung.

### `published_in`

Praktisch flächendeckend belegt (1.448.934 von 1.448.984). Die Form ist eine
Kurzzitation im IPNI-Stil `Werk Band: Seite (Jahr)`:

- `Sp. Pl.: 73 (1753)`
- `Phytotaxa 275: 217 (2016)`
- `F.W.H.von Humboldt, A.J.A.Bonpland & C.S.Kunth, Nov. Gen. Sp. 6: 127 (1823)`
- `Acta Bot. Acad. Sci. Hung. 17: 121 (1971 publ. 1972)`

Zu beachten: **20.989 Namen tragen den Literalwert `Unknown`** — das ist ein
belegtes Feld ohne Information, kein NULL. Wer `published_in` als
Relevanzsignal verwendet, muss `Unknown` wie "nicht vorhanden" behandeln.

## SP6 Task 4 — Verdikt: Synonym-Relevanz gegen den vollen Index

> Abschluss von SP6 (Task 4 von 4). Alle Zahlen unten sind **neu gemessen**,
> gegen die volle, per SP6 Task 1 nachgezogene WCVP-Datenbank
> (`wcvp.db`, 993 MB, **1.448.984 Namen / 440.534 Konzepte**), read-only
> geöffnet. Gemessen wurde nicht eine SQL-Nachbildung der Regeln, sondern
> **der echte Domänencode**: `domain.RankSynonyms` +
> `domain.SummarizeSynonyms` mit `ExcludeRanks: domain.RanksBelowSpecies()`
> — also exakt das, was
> `GET /v1/concept/{id}/synonyms?relevance=publication&rank=species`
> ausliefert.

### Reproduktion

Die Rohverteilungen kommen direkt aus SQLite:

```bash
sqlite3 "file:wcvp.db?mode=ro" \
  "SELECT n.rank, count(*) FROM concept_name cn JOIN name n ON n.id=cn.name_id
   WHERE cn.role='synonym' GROUP BY n.rank ORDER BY 2 DESC;"
# SPECIES|639027   VARIETY|201957   SUBSPECIES|45526   FORM|42681
# GENUS|25003      OTHER|6409       SUBVARIETY|3328    SUBFORM|641
# NOTHOSUBSPECIES|130   NOTHOVARIETY|51   NOTHOFORM|9

sqlite3 "file:wcvp.db?mode=ro" \
  "SELECT homotypic, count(*) FROM concept_name WHERE role='synonym' GROUP BY homotypic;"
# |692941     (NULL = unbekannt)
# 1|271821
#            (0 = heterotypisch: kommt NICHT vor)

sqlite3 "file:wcvp.db?mode=ro" \
  "SELECT count(*) FROM name WHERE nom_status IS NOT NULL AND nom_status<>'';"
# 99252   (von 1.448.984 = 6,85 %)
```

Alle Konzept-bezogenen Zahlen stammen aus einem Wegwerf-Harness (nach der
Messung wieder gelöscht, damit kein Messwerkzeug als Produktcode
mitläuft), der die Kandidatenabfrage aus
`internal/adapters/sqlite/synonyms.go` **einmal über alle Konzepte** fährt
und je Konzept den Domänencode aufruft:

```go
const q = `
    SELECT cn.concept_id, n.id, n.canonical, COALESCE(n.authorship,''), n.rank,
           COALESCE(n.rank_verbatim,''), COALESCE(n.nom_status,''), cn.homotypic,
           (an.basionym_id IS NOT NULL AND an.basionym_id = n.id)
    FROM concept_name cn
    JOIN name n  ON n.id = cn.name_id
    JOIN taxon_concept tc ON tc.id = cn.concept_id
    JOIN name an ON an.id = tc.accepted_name
    WHERE cn.role = 'synonym'
    ORDER BY cn.concept_id`

// je Konzept (Batch beim Wechsel von cn.concept_id):
ranked := domain.RankSynonyms(batch, domain.SynonymOptions{
    ExcludeRanks: domain.RanksBelowSpecies(),
})
sum := domain.SummarizeSynonyms(ranked)
```

Laufzeit über alle 964.762 Synonymzeilen: **rund 12 s**.

### Wie oft ändert der Filter überhaupt etwas?

| Größe | Wert | Anteil |
| --- | ---: | ---: |
| Konzepte mit mindestens einem Synonym | 236.030 | 53,6 % aller 440.534 Konzepte |
| Synonymzeilen gesamt | 964.762 | |
| davon publikationsfähig (`rank=species`) | 638.212 | 66,2 % |
| **Konzepte, bei denen der Filter die Antwort ändert** | **103.674** | **43,9 %** |
| … darunter über eine `nom_status`-Regel | 50.774 | 21,5 % |
| … darunter über `rank=species` | 76.866 | 32,6 % |

Zurückgehaltene Zeilen nach Regel:

| Regel | Zeilen |
| --- | ---: |
| `rank` | 234.405 |
| `nom_status` | 89.836 |
| `unclassified_nom_status` | 2.309 |

Die 234.405 Rangausschlüsse liegen unter der Summe der vier
ausgeschlossenen Ränge (201.957 + 42.681 + 3.328 + 641 = **248.607**). Die
Differenz von 14.202 ist kein Fehler, sondern die Regelpräzedenz: diese
Zeilen waren schon durch `nom_status` ausgeschlossen, und ein Synonym wird
nur einmal gezählt, mit dem *ersten* greifenden Grund.

Bei mehr als der Hälfte der Konzepte (132.356) ändert der Filter **nichts**
— sie haben nur artrangige Synonyme ohne eingetragenen Status. Das ist die
erwartbare Folge davon, dass `nom_status` auf 6,85 % der Namen belegt ist:
Der Filter ist scharf, wo die Quelle etwas eingetragen hat, und untätig,
wo sie geschwiegen hat.

### Landet die Liste wirklich bei ein bis drei? — Ja, aber auch ohne Filter

UC5 nennt „zwei bis drei relevante Synonyme" als Ziel. Gemessen — und zwar
**mit Kontrollgruppe**: dieselben 236.030 Konzepte, einmal ungefiltert (die
Listenlänge, die ein Aufrufer ohne `relevance=publication` sieht) und einmal
gefiltert:

| Listenlänge | ungefiltert | Anteil | gefiltert (`publication`, `rank=species`) | Anteil |
| ---: | ---: | ---: | ---: | ---: |
| 0 | 0 | 0 % | 24.918 | 10,56 % |
| 1 | 95.366 | 40,40 % | 97.674 | 41,38 % |
| 2 | 45.707 | 19,36 % | 42.791 | 18,13 % |
| 3 | 26.315 | 11,15 % | 22.859 | 9,68 % |
| 4 | 16.055 | 6,80 % | 13.212 | 5,60 % |
| 5 | 11.104 | 4,70 % | 8.624 | 3,65 % |
| 6–10 | 23.993 | 10,17 % | 17.267 | 7,32 % |
| 11–25 | 13.552 | 5,74 % | 7.466 | 3,16 % |
| 26–100 | 3.708 | 1,57 % | 1.191 | 0,50 % |
| > 100 | 230 | 0,10 % | 28 | 0,01 % |
| **1 bis 3 (Zielkorridor)** | **167.388** | **70,92 %** | **163.324** | **69,20 %** |
| **> 3** | **68.642** | **29,08 %** | **47.788** | **20,25 %** |

**Auf UC5s eigener Zielgröße ist der Filter netto negativ.** Ungefiltert
liegen bereits **70,92 %** im Korridor, gefiltert **69,20 %** — 1,72
Prozentpunkte *schlechter*. Der Mechanismus ist in der Tabelle direkt
ablesbar: Der Filter zieht **20.854 Konzepte** aus `> 3` heraus (68.642 →
47.788), aber er schiebt sie nicht überwiegend in den Korridor, sondern
erzeugt dabei **24.918 Nullen**, die es ungefiltert nicht gab. Auch der
Modalwert bleibt unverändert **1** — vorher 40,40 %, nachher 41,38 %.

Die ehrliche Antwort auf die Frage lautet also: *Ja, überwiegend — aber das
war schon vorher so, und der Filter ist nicht der Grund dafür.* Die
Korridorquote ist als Beleg **für** den Filter untauglich; sein Nutzen liegt
woanders (siehe „Was hält" unten).

Drei weitere Einschränkungen:

- **Der häufigste Fall ist 1, nicht 2–3** — und zwar in beiden Spalten. Das
  ist kein Filtererfolg, sondern die Datenlage: solche Konzepte hatten
  meist von vornherein nur ein Synonym.
- **Für 20,25 % (47.788 Konzepte) liefert der Filter weiterhin mehr als drei
  Namen**, in 8.685 Fällen sogar mehr als zehn. Für diese Konzepte löst UC5
  das Filterproblem *nicht*; der Aufrufer muss `max` setzen und bekommt dann
  eine sortierte, aber selbst gewählte Auswahl.
- **Die Sortierung, die „die besten drei" bestimmt, ruht zu 71,8 % auf
  `unknown`.** Wo `homotypic` NULL ist (692.941 Zeilen), entscheidet nach
  Publikationsfähigkeit faktisch die `name_id`. „Die drei besten" heißt für
  diese Konzepte „drei mit der kleinsten Id" — deterministisch, aber
  fachlich nicht begründet.

Erzeugt wurde die Tabelle mit demselben Harness wie oben, um eine zweite
Zählung je Konzept erweitert (`len(batch)` als Kontrollgruppe neben
`sum.Publishable`), Ausgabe verbatim:

```
concepts: 236030
len      unfiltered   filtered
0                 0      24918
1             95366      97674
2             45707      42791
3             26315      22859
4             16055      13212
5             11104       8624
6-10          23993      17267
11-25         13552       7466
26-100         3708       1191
>100            230         28
corridor 1..3: unfiltered 167388 (70.92 %)  filtered 163324 (69.20 %)
>3:            unfiltered 68642 (29.08 %)  filtered 47788 (20.25 %)  delta 20854
modal 1:       unfiltered 95366 (40.40 %)  filtered 97674 (41.38 %)
```

### Nur 347 Synonymzeilen sind positiv als sauber bezeichnet

Dieselbe Schleife, `domain.ClassifyNomStatus` über alle 964.762
Synonymzeilen gezählt:

| Urteil | Zeilen | Anteil |
| --- | ---: | ---: |
| `absent` (nichts eingetragen) | 872.270 | 90,41 % |
| `disqualifying` | 89.836 | 9,31 % |
| `unclassified` | 2.309 | 0,24 % |
| **`acceptable`** (positiv als nomenklatorisch sauber bezeichnet) | **347** | **0,036 %** |

Das ist die schärfste Form der Aussage aus Abschnitt (d) der
[UC5-Anleitung](../how-to/synonyms-uc5.md): **Im gesamten Korpus behauptet
die Quelle für 347 Synonymzeilen, dass der Name nomenklatorisch in Ordnung
ist.** Alles andere, was hostus publikationsfähig nennt, ist entweder ein
`absent` — die Quelle hat nichts eingetragen — oder es wurde
zurückgehalten. Praktisch heißt das: eine publikationsfähige Liste besteht
zu über 99,9 % aus *ungeprüften*, nicht aus *geprüften* Namen. `summary.absent`
beziffert das pro Antwort; die 347 setzen die Größenordnung dazu.

### 24.918 Konzepte ohne ein einziges publikationsfähiges Synonym

10,6 % der Konzepte mit Synonymen kommen unter `relevance=publication&rank=species`
mit einer **leeren Liste** zurück. Aufgeschlüsselt:

| Ursache | Konzepte |
| --- | ---: |
| alle Synonyme durch `rank` ausgeschlossen | 16.621 |
| alle Synonyme durch `nom_status` ausgeschlossen | 6.269 |
| gemischt (`rank` + Status, inkl. unklassifiziert) | 2.028 |

**Ist das richtig? Ja — und es ist genau der Fall, der die Ausgabe des
Ausschluss-Summarys unverzichtbar macht.** Ein Konzept, dessen einziges
Synonym eine *Varietät* ist, hat auf Artniveau tatsächlich nichts zu
publizieren; ein Konzept, dessen einziges Synonym `, nom. nud.` trägt,
ebenfalls nicht. Falsch wäre nur, das als *„dieses Konzept hat keine
Synonyme"* auszuliefern — und genau das tut hostus nicht: `summary.total`
nennt weiterhin die volle Zahl, `summary.excluded` die Regel. Eine leere
`synonyms`-Liste mit `"total": 3, "excluded": {"rank": 3}` ist eine
Aussage, keine Lücke.

Der einzige unbefriedigende Teilbereich sind die 6.269 Konzepte, bei denen
`nom_status` alles wegnimmt: Da die Spalte nur auf 6,85 % der Namen belegt
ist, sind das per Konstruktion Konzepte, deren Synonyme **überdurchschnittlich
gut dokumentiert** sind. Wer viel einträgt, verliert mehr. Das ist keine
Verzerrung des Filters, aber eine des Datenbestands, und sie geht zulasten
der sorgfältig gepflegten Einträge.

### Der offene fachliche Punkt, gemessen

Fünf Werte hält hostus als `unclassified` zurück, weil ihre Behandlung eine
botanische Entscheidung ist. Gemessen über alle distinkten `nom_status`-Werte,
klassifiziert mit `domain.ClassifyNomStatus`: **1.697 Namen**, davon

| Wert | Namen |
| --- | ---: |
| `, sensu auct.` | 1.117 |
| `, tentatively listed as a synonym.` | 290 |
| `, fossil name.` | 264 |
| `, isonym` | 13 |
| Wert enthält `?` | 13 |

Anmerkung zur Zahl: die reine Containment-Zählung über dieselben fünf
Tokens ergibt **1.707** Namen. Die Differenz von 10 sind Zellen, in denen
neben dem Open-Item-Token noch ein *disqualifizierender* Token steht (z. B.
`, nom. illeg., later homonym of a fossil name.`) — sie werden nach der
Präzedenzregel als `nom_status` ausgeschlossen, nicht als offener Punkt
zurückgehalten. Zurückgehalten *wegen des offenen Punktes* sind also
**1.697**, nicht 1.707.

Die 1.117 `, sensu auct.` sind der Kern: Fehlanwendungen werden in
Florenwerken üblicherweise als **auct. non** geführt statt weggelassen.
Sollte UC5 das so wollen, ist das eine Zeile in `nomStatusGuards`
(`internal/domain/synonym.go`), keine Codeänderung.

### Verdikt: **hält mit Auflagen**

**Was hält.** Der Filter ist real, messbar und begründet: Er ändert bei
103.674 von 236.030 Konzepten die Antwort, entfernt 326.550 Synonymzeilen
und sagt für jede einzelne, welche Regel das war. Das Ausschluss-Summary
beschreibt immer das Konzept, nie die Seite — eine gefilterte Liste ist
damit von einer kaputten Abfrage unterscheidbar, was der eigentliche
Prüfstein war.

**Was ausdrücklich NICHT als Beleg taugt: der Zielkorridor.** Ohne Filter
liegen bereits **70,92 %** der Konzepte bei ein bis drei Synonymen,
gefiltert sind es **69,20 %** — der Filter verbessert diese Kennzahl nicht,
er verschlechtert sie leicht. Er verschiebt 20.854 Konzepte aus `> 3`
heraus und erzeugt dabei 24.918 Nullen. **Sein Nutzen ist die Entfernung
nomenklatorisch untauglicher Namen (89.836 Zeilen mit belegtem Defekt),
nicht das Treffen des Zielkorridors.** Eine frühere Fassung dieses
Abschnitts führte die 69,2 % ohne Kontrollgruppe als Beleg *für* den Filter
— das war auf dieser Dimension ein Beleg dagegen, und eine Zahl ohne ihre
Vergleichsgröße ist kein Argument.

**Die Auflagen**, alle in
[docs/how-to/synonyms-uc5.md](../how-to/synonyms-uc5.md) dokumentiert:

1. **Zwei von fünf UC5-Kriterien sind nicht umgesetzt.** „Im Bezugsraum
   verwendet" ist mit dem aktuellen Schema *nicht ausdrückbar*
   (`distribution` hängt am Konzept, ein Synonym ist ein Name); „in
   Standardwerken verwendet" scheitert an `redistribution: unknown` und an
   der unfertigen CDM-Ernte. `relevance=publication` filtert **global**.
2. **Das Typisierungskriterium ist ein Zwei-Wege-Split.** `heterotypic`
   kommt auf 0 Zeilen vor, `unknown` auf 692.941. UC5-Regel 3 wirkt real
   als „basionym-belegt vor unbelegt".
3. **`nom_status` ist auf 6,85 % der Namen belegt** — und nur **347** der
   964.762 Synonymzeilen sind positiv als sauber bezeichnet. Ein fehlender
   Status ist kein Unbedenklichkeitsnachweis; `summary.absent` beziffert das
   pro Antwort, die nomenklatorische Prüfung bleibt beim Autor.
4. **Zwei benannte Ranglücken**: SUBSPECIES (45.526 Synonymnamen) wird
   spec-treu nicht ausgeschlossen, und 190 Nothotaxon-Zeilen passieren
   `rank=species`.
5. **Für 20,25 % der Konzepte löst der Filter das Problem nicht** — mehr als
   drei Namen bleiben übrig, die Auswahl macht dann `max` und damit die
   `name_id`. Und für die Zielgröße „ein bis drei" ist der Filter netto
   negativ (70,92 % ungefiltert gegen 69,20 % gefiltert): Wer ihn mit dieser
   Kennzahl begründet, begründet ihn falsch.

**Was nicht hält, wenn die Auflagen wegfallen.** Ohne den
„Was-dieser-Filter-nicht-kann"-Abschnitt in der Anleitung wäre das Verdikt
*hält nicht*: Ein Endpunkt, der `relevance=publication` heißt und
tatsächlich nur zwei von fünf beworbenen Kriterien anwendet, ist ohne diese
Offenlegung irreführend. Die Dokumentation ist hier nicht Beiwerk, sondern
Teil der Funktion.

---

## SP5 Task 5 — Verdikt: Konzeptübersetzung zwischen `sec.`-Referenzräumen (UC6)

Gemessen am **vollen** Index: WCVP-Volldump (DwC-A, 1.448.984
Taxonzeilen) plus die beiden kanonischen CSVs der CDM-Ernte
(`rl_standardliste`, 51.466 Konzepte / 26.346 Relationen aus 15,6 h Crawl)
in **eine frische** Datenbank. Kein erneuter Crawl, keine Anfrage an
`api.cybertaxonomy.org`.

```bash
hostus ingest --dataset dataset-full.yaml --db full.sqlite
# backbones: wcvp -> poc/data/wcvp_dwca
# concept_sources: cdm -> pipelines/cdm/output/cdm-{concepts,relations}-canonical.csv
```

```
Ingest complete:
  wcvp: names=1448984 concepts=440534 synonyms=964762 orphaned=43688
    ranks: other=6527 ((empty) 2744, proles 2351, lusus 660, microgène 371, …)
Concept sources:
  cdm: concepts=51466 written=51466 sec_spaces=119 relations=26346 written=26002
    congruent: 23971
    includes: 1591
    includes_or_included_in_or_overlaps: 198
    not_congruent: 1
    overlaps: 118
    pro_parte: 123
    dropped: misapplied/non-concept=344 unresolved ends=0 unresolved parents=0 reader errors=0
    unknown concept-relation flag=0, concepts without sec.=124, empty status=15523
    ranks: other=1820 (Species Aggregate 1088, Species Group 374, …)
  hinweis: cdm (redistribution=unknown) — lokal genutzt, nicht redistribuierbar
real 283,92   (user 165,56  sys 132,01)
```

**283,92 s** Wandzeit für beide Backbones zusammen, ein Durchlauf, kein
Abbruch. Alle 51.466 Konzeptzeilen und alle 26.346 Relationszeilen wurden
gelesen — **0 Reader-Fehler**, **0 nicht auflösbare Relationsenden**,
**0 nicht auflösbare Eltern**. Die 344 nicht geschriebenen Relationen sind
genau die `is misapplied name for`-Zeilen (`is_concept_relation=false`), die
die dokumentierte Regel verwirft: 26.346 − 344 = **26.002 = 98,69 %**.

### Die beiden Review-Befunde, die nur ein Volllauf entscheiden konnte

**1. Elternreihenfolge (Self-FK).** Die Datei enthält tatsächlich
Vorwärtsverweise, und zwar massenhaft:

```bash
python3 -c "…"   # zählt parent_uuid, die erst später in der Datei stehen
# rows 51466 with_parent 33731 parent_appears_later_in_file 9897
```

**9.897 von 33.731** Elternverweisen (29,3 %) zeigen auf eine Zeile, die
erst später kommt — ein einphasiger Ingest wäre an jedem einzelnen davon
in den Self-FK gelaufen. Der Zweiphasen-Ingest schreibt trotzdem alle
33.731:

```bash
sqlite3 -readonly full.sqlite \
  "SELECT COUNT(*) FROM taxon_concept WHERE backbone_id='cdm' AND parent_id IS NOT NULL AND parent_id<>'';"
# 33731
```

33.731 stimmt exakt mit `concepts_with_parent_uuid=33731` aus
`pipelines/cdm/cdm.summary.txt` überein. **Befund behoben, am Volldatensatz
belegt.**

**2. Fehlergrenze des Readers.** `reader errors=0` bei
`concepts=51466 written=51466` und `relations=26346` — beides ist die volle
Zeilenzahl der beiden Dateien (`wc -l` minus Kopfzeile: 51.467−1 und
26.347−1). Es wurde nichts still abgeschnitten. **Befund behoben.**

### Deckung des Relationsgraphen

```bash
sqlite3 -readonly full.sqlite \
  "SELECT COUNT(*) FROM taxon_concept tc WHERE tc.backbone_id='cdm'
     AND tc.id NOT IN (SELECT from_concept FROM concept_relation)
     AND tc.id NOT IN (SELECT to_concept FROM concept_relation);"
# 20122
```

| Kennzahl | Wert | Anteil |
|---|---:|---:|
| CDM-Konzepte gesamt | 51.466 | 100 % |
| … mit mindestens einer Relation | 31.344 | **60,90 %** |
| … die einen **anderen** `sec.`-Raum erreichen | 31.341 | **60,90 %** |
| … ohne jede Relation (isoliert) | 20.122 | 39,10 % |

Verteilung der erreichbaren Fremd-`sec.`-Räume je Konzept (1 Hop):

```
1 Raum: 27192   2: 410   3: 289   4: 279   5: 378   6: 1358
7: 1240   8: 148   9: 38   10: 5   11: 3   12: 1
```

Der Graph ist also flach: 27.192 der 31.341 übersetzbaren Konzepte
erreichen genau **einen** Fremdraum. Das ist kein Mangel des Ingests,
sondern die Form der Quelle — die Standardliste verknüpft je Konzeptpaar,
nicht je Kreuzprodukt.

### Wie viele Konzepte tragen welchen `sec.`-Bezug?

`taxon_concept.sec_reference` speichert die CDM-**Referenz**-UUID
(`sec_uuid`), nicht die Klassifikations-UUID. Davon gibt es **119**
verschiedene, nicht 18 — die 18 aus dem Crawl-Summary sind
Klassifikationen, und der Crosswalk bildet 17 davon ab. Die Verteilung ist
extrem ungleich:

```bash
sqlite3 -readonly full.sqlite \
  "SELECT COUNT(*) c, sr.title FROM taxon_concept tc
     LEFT JOIN sec_reference sr ON sr.id=tc.sec_reference
    WHERE tc.backbone_id='cdm' GROUP BY 2 ORDER BY 1 DESC;"
```

| Konzepte | `sec.`-Referenzraum |
|---:|---|
| 14.626 | Wisskirchen & Haeupler 1998: Standardliste der Farn- und Blütenpflanzen Deutschlands |
| 4.654 | Schubert & Vent 1990: Exkursionsflora von Deutschland (Rothmaler, 4. Kritischer Band) |
| 4.635 | HEGI: Illustrierte Flora von Mitteleuropa, Aufl. 2 u. 3 |
| 4.548 | OBERDORFER: Pflanzensoziologische Exkursionsflora, ed. 7 |
| 4.390 | EHRENDORFER: Liste der Gefäßpflanzen Mitteleuropas, 2. Aufl. |
| 4.381 | BfN: FloraWeb DB |
| 4.351 | TUTIN et al.: Flora Europaea |
| 4.021 | SCHMEIL-FITSCHEN: Flora von Deutschland …, 89. Aufl. |
| 1.774 | Greuter & al.: Med-Checklist, Bde. 1, 3, 4 |
| 1.346 | BRUMMITT 1992: Vascular Plant Families and Genera |
| 1.080 | Greuter & al. 1993: Names in Current Use for Extant Plant Genera |
| 446 | Andere Referenzen (fuer auct. Synonyme) |
| 296 | Excel Taxon import |
| 190 | Andere Referenzen (fuer Synonyme s. l.) |
| 181 | R. Wisskirchen & H. Haeupler 1998: Standardliste (fuer Synonyme mit Fakten) |
| 173 | Andere Referenzen (fuer Synonyme s. str.) |
| **124** | **(leer — kein `sec.`)** |
| 67 … 1 | 103 weitere Räume mit zusammen 288 Konzepten |

11 Räume mit je ≥ 1.000 Konzepten decken **49.806 von 51.466 = 96,77 %**
ab. Der lange Schwanz aus 100 Räumen mit 1–9 Konzepten ist für UC6
praktisch bedeutungslos, aber er ist da und wird nicht weggerundet.

### Die Zahl, die UC6 entscheidet

**Wie viele unserer WCVP-Konzepte lassen sich tatsächlich in mindestens
einen `sec.`-Raum übersetzen?**

Die Brücke von WCVP in den CDM-Namensraum ist ein **Namens**-Crosswalk,
kein ID-Join: die CDM-Ernte trägt **keine** externe ID. Ihr
Spaltenkopf lautet vollständig

```
concept_uuid|scientific_name|authorship|rank|status|sec_uuid|sec_title|classification_uuid|parent_uuid
```

— kein IPNI, kein POWO, kein GBIF, kein WFO. Und in der Datenbank hat
**keine einzige** `concept_relation`-Zeile ein WCVP-Ende:

```bash
sqlite3 -readonly full.sqlite \
  "SELECT COUNT(*) FROM concept_relation cr
     JOIN taxon_concept a ON a.id=cr.from_concept
     JOIN taxon_concept b ON b.id=cr.to_concept
    WHERE a.backbone_id<>'cdm' OR b.backbone_id<>'cdm';"
# 0
```

**Obergrenze (was die Daten hergeben).** Gemessen über
`name.canonical_fold` — genau die Spalte, auf der `MatchExact` arbeitet:

```bash
sqlite3 measure.sqlite <<'SQL'
CREATE TEMP TABLE cdm_rel_fold AS
  SELECT DISTINCT n.canonical_fold AS f FROM concept_name cn
  JOIN name n ON n.id=cn.name_id JOIN taxon_concept tc ON tc.id=cn.concept_id
  WHERE tc.backbone_id='cdm'
    AND (tc.id IN (SELECT from_concept FROM concept_relation)
      OR tc.id IN (SELECT to_concept FROM concept_relation));
SELECT COUNT(*) FROM taxon_concept tc JOIN name n ON n.id=tc.accepted_name
 WHERE tc.backbone_id='wcvp' AND n.canonical_fold IN (SELECT f FROM cdm_rel_fold);
SQL
# 4461
```

| Frage | WCVP-Konzepte | von 440.534 |
|---|---:|---:|
| akzeptierter Name trifft **irgendein** CDM-Konzept | 6.330 | 1,44 % |
| akzeptierter Name trifft ein CDM-Konzept **mit Relation** | **4.461** | **1,01 %** |
| irgendein Name (inkl. Synonymen) trifft ein CDM-Konzept | 8.987 | 2,04 % |
| irgendein Name trifft ein CDM-Konzept **mit Relation** | 5.953 | 1,35 % |

Die Obergrenze ist also **4.461 Konzepte = 1,01 %** (bzw. 5.953 = 1,35 %,
wenn Synonyme als Brücke zugelassen werden). Das ist keine Überraschung und
kein Fehler: CDM deckt die *deutsche* Flora ab, WCVP die *Welt*. 19.725
verschiedene Namensformen in 51.466 CDM-Konzepten — die Differenz ist
genau die `sec.`-Vervielfachung, die UC6 nutzen will.

**Was der Endpunkt davon tatsächlich liefert: 0.** Gemessen, nicht
geschätzt. 300 WCVP-Namen, deren CDM-Gegenseite nach obiger Messung
*mindestens eine* Relation trägt, über echtes HTTP gegen den laufenden
Server, Ziel `TUTIN et al.: Flora Europaea`, entdrosselt auf ~9 Anfragen/s
(sonst greift der 20-rps-Limiter und verfälscht die Messung).

**Ein Vorbehalt zur Stichprobe, bevor die Zahl kommt:** Das Auswahlkriterium
war „CDM-Gegenseite hat *irgendeine* Relation", das Ziel jeder Anfrage aber
derselbe Raum. Beides deckt sich nicht:

```bash
# Namensformen, deren CDM-Seite eine Kante nach Flora Europaea hat
sqlite3 measure.sqlite "… JOIN taxon_concept p ON p.id=e.b
   WHERE tc.backbone_id='cdm' AND p.sec_reference='6eeeeacc-…' …"
# of the 300: CDM side reaches Flora Europaea | 196
```

Nur **196 der 300** hätten selbst bei perfekter Namensbrücke überhaupt
einen Kandidaten in *diesem* Zielraum haben können; die übrigen 104 wären
legitim `no_relation_recorded`. Am Ergebnis ändert das nichts — die
erreichte Zahl ist 0, nicht 196 — aber die Vergleichsgröße ist 196, nicht
300. (Auf die Obergrenze bezogen: mit Flora Europaea als festem Ziel sind
es **2.601** WCVP-Konzepte statt 4.461.)

```bash
while read n; do
  curl -s -X POST localhost:18131/v1/translate -H 'content-type: application/json' \
    -d "{\"verbatim\":\"$n\",\"target_space\":\"6eeeeacc-1da9-4839-98d6-3169c4237ecd\"}"
done < sample_names.txt | …
# 265 UNRESOLVABLE
#  35 no_relation_recorded wcvp
#   0 translated
```

Zwei Ursachen, beide strukturell:

1. **265 von 300 sind mehrdeutig.** `MatchExact` sucht über *alle*
   Backbones. `classify` sieht mehrere *verschiedene* Konzepte auf
   derselben Trefferstufe, verweigert die Wahl und liefert `UNRESOLVABLE`.
   Das ist die *richtige* Antwort der Match-Logik und zugleich der Grund,
   warum der `verbatim`-Pfad von `/v1/translate` mit ingestierter CDM
   praktisch nie auflöst. Ein `sec.`-Raum trennt Konzepte — und macht damit
   jeden geteilten Namen mehrdeutig.
2. **Die übrigen 35 lösen auf — auf das WCVP-Konzept.** Und WCVP-Konzepte
   haben keine Relationen (s. o., 0 Zeilen), also `no_relation_recorded`.

**Beleg statt Behauptung: der Aufteilung nach.** Die Stichprobe oben hat
nur den HTTP-Code festgehalten, und `UNRESOLVABLE` deckt *zwei*
verschiedene Fälle ab — Mehrdeutigkeit und „nichts hat klassifiziert".
Dieselben 300 Namen durch `POST /v1/match` (das den Grund als `note`
ausgibt, 50 je Batch):

```
  265  noteAmbiguous          ("Mehrdeutiger Treffer: mehrere Konzepte mit gleicher Übereinstimmungsstärke")
   35  resolved:exact_author
```

**0×** `noteUnresolvable`, **0×** Fuzzy-Mehrdeutigkeit. Die Ursache ist
damit gemessen, nicht erschlossen: es ist ausnahmslos Mehrdeutigkeit.

**Wie groß die Mehrdeutigkeit konkret ist.** Für `Abies alba Mill.`:

```bash
sqlite3 -readonly full.sqlite \
  "SELECT tc.backbone_id, cn.role, COALESCE(n.authorship,'(kein)'), COUNT(DISTINCT tc.id)
     FROM concept_name cn JOIN name n ON n.id=cn.name_id
     JOIN taxon_concept tc ON tc.id=cn.concept_id
    WHERE n.canonical_fold='abies alba' GROUP BY 1,2,3;"
# cdm  | accepted | Mill.               | 8
# wcvp | accepted | Mill.               | 1
# wcvp | synonym  | (Castigl.) Michx.   | 1
```

**Acht** CDM-Konzepte (nicht neun) — eines je Referenzwerk — plus das
akzeptierte WCVP-Konzept ergeben **neun** auf `exact_author` gleich starke
Kandidaten; das zehnte Vorkommen fällt an der Autorprüfung heraus. Die
Obergrenze über *alle* Namensformen liegt höher:

```bash
sqlite3 -readonly full.sqlite \
  "SELECT MAX(c) FROM (SELECT n.canonical_fold, COUNT(DISTINCT tc.id) c
     FROM concept_name cn JOIN name n ON n.id=cn.name_id
     JOIN taxon_concept tc ON tc.id=cn.concept_id
    WHERE tc.backbone_id='cdm' GROUP BY 1);"
# 10
```

**Bis zu zehn** CDM-Konzepte je Namensform (`Stellaria palustris`,
`Sedum telephium`, `Polypodium montanum`, `Polygonum lapathifolium`,
`Pinus abies` u. a.); über beide Backbones zusammen maximal **16**.

**Gegenprobe, damit die 0 kein Messfehler ist.** Derselbe Endpunkt,
dieselbe Sitzung, 200 CDM-Konzepte mit bekannter Relation nach Flora
Europaea:

```bash
# 193 translated 1
#   7 translated 2
```

**200 von 200.** Der Endpunkt funktioniert; was fehlt, ist die Brücke
zwischen den Namensräumen — nicht der Code dahinter.

**Was diese Gegenprobe ausdrücklich *nicht* zeigt.** Sie steigt über
`concept_id` ein und wählt vorab Konzepte, die eine Relation **in den
Zielraum** tragen. Damit belegt sie zweierlei: dass der Relationsgraph
tragfähig ist und dass die Antwort korrekt geformt wird. Sie belegt
**nicht**, dass die Namensauflösung funktioniert — genau der Pfad, an dem
die 300 scheitern, wird hier umgangen. Sie ist die richtige Kontrolle für
die Frage „ist die 0 ein Artefakt meiner Messvorrichtung?" (Antwort: nein,
derselbe Endpunkt in derselben Sitzung antwortet 200/200), und für keine
darüber hinausgehende Frage.

### Verdikt: **hält mit Auflagen**

**Was hält.** Der Ingest hält am Volldatensatz: 51.466 Konzepte, 26.002
Relationen, 0 Reader-Fehler, 0 unaufgelöste Enden, 0 unaufgelöste Eltern
trotz 9.897 Vorwärtsverweisen, 283,92 s. Der Relationsgraph deckt
**60,90 %** der CDM-Konzepte ab. `/v1/translate` liefert für CDM-Konzepte
in **200 von 200** Stichproben eine typisierte, richtungstreue Antwort und
verwechselt Gleichsetzung nie mit Enthaltensein. Die `sec.`-Trennung hält:
ein Name, mehrere Konzepte, keine Verschmelzung.

**Die Auflagen.**

1. **UC6 ist für WCVP-Konzepte heute nicht bedienbar.** Die Obergrenze
   liegt bei **4.461 von 440.534 Konzepten (1,01 %)**; der Endpunkt liefert
   davon in der Messung **0**. Wer UC6 über WCVP-IDs bedienen will, braucht
   einen expliziten WCVP↔CDM-Crosswalk (ein `xref`-Eintrag oder eine
   Namensauflösung mit Backbone-Präferenz) — den gibt es nicht, und die
   Quelle liefert keine ID, aus der er sich ableiten ließe.
2. **Der `verbatim`-Pfad von `/v1/translate` ist mit ingestierter CDM
   faktisch tot** (265/300 `UNRESOLVABLE`). Das ist kein Bug: die
   Mehrdeutigkeit ist echt, und Raten wäre schlimmer. Aber der Endpunkt
   bewirbt einen Einstieg, den er unter Volldaten nicht bedienen kann. Ein
   Backbone- oder `sec.`-Filter am Match wäre die naheliegende Reparatur —
   ungemessen, deshalb hier nur benannt.
3. **39,10 % der CDM-Konzepte sind isoliert** und lassen sich in keinen
   anderen Raum übersetzen. Eine leere Antwort ist hier der Normalfall,
   nicht die Ausnahme; die explizite `no_relation_recorded`-Ausgabe ist
   entsprechend kein Randfall des Contracts, sondern sein Hauptpfad.
4. **`not_congruent` kommt genau einmal in 26.002 Zeilen vor.** Der Mapper
   behandelt den Typ, aber er ist praktisch ungetestet an echten Daten.
5. **124 CDM-Konzepte tragen keinen `sec.`-Bezug** und sind damit kein
   gültiges Übersetzungsziel.

**Lizenz — unverändert und bindend.** Für den CDM-Server der
„Standardliste der Farn- und Blütenpflanzen Deutschlands" ist **nirgends**
eine Lizenzangabe auffindbar: nicht auf dem Portal, nicht auf der API,
nicht in den Payloads. Die Inhalte sind aus **urheberrechtlich geschützter
Florenliteratur** abgeleitet (Wisskirchen & Haeupler, HEGI, OBERDORFER,
ROTHMALER, SCHMEIL-FITSCHEN, Flora Europaea …). Deshalb im Manifest
`redistribution: unknown`, und daraus folgt bindend:

- **nur lokale Auswertung.** Die beiden CSVs sind gitignoriert und bleiben
  es.
- **`hostus bundle` verweigert den Export** dieser Quelle ohne
  `--force-include-restricted` und protokolliert sie dann in
  `bundle_meta.restricted_sources`.
- **`/v1/translate` darf auf diesen Daten nicht öffentlich betrieben
  werden**, solange keine schriftliche Freigabe von BGBM/EDIT vorliegt.

### Nebenbefund für das nächste Milestone: FAMILY-Ränge

Der Anwendungsfall „vage erfassen" (`Acer sp.`, `Asteraceae`) braucht höhere
Ränge. WCVP liefert davon **keine**:

```bash
sqlite3 -readonly full.sqlite \
  "SELECT backbone_id, COUNT(*) FROM taxon_concept WHERE rank='FAMILY' GROUP BY 1;"
# cdm|629
```

Der CDM-Ingest bringt **629 FAMILY-Konzepte** — die einzigen im System.
Sie verteilen sich auf drei `sec.`-Räume (BfN FloraWeb 280, Wisskirchen &
Haeupler 182, BRUMMITT 1992 167) und tragen **457 verschiedene**
Familiennamen; **171 Namen** kommen in mehr als einem Raum vor.

Beide Endpunkte finden sie, ohne Änderung:

```bash
curl -s "localhost:18131/v1/suggest?q=Asteraceae&limit=5"
# {"results":[{"concept_id":"cdm:concept:1785944e-…","canonical":"Asteraceae","rank":"FAMILY","status":"ACCEPTED",…},
#             {"concept_id":"cdm:concept:302a66c9-…","canonical":"Asteraceae","rank":"FAMILY",…}]}

curl -s "localhost:18131/v1/concept/cdm:concept:1785944e-9887-4d6d-acb3-6867a28d9c4c"
# {"concept_id":"…","display":"Asteraceae Dumort.","canonical":"Asteraceae","rank":"FAMILY","status":"ACCEPTED",
#  "backbone":{"id":"cdm","version":"2026-08-02"},"synonyms":[]}
```

Auch Gattungen kommen mit: `q=Acer` liefert `Aceraceae` (2×), `Acer` (2×,
GENUS) und `Aceras`. Alle 629 stehen in `fts_name_map`, sind also
vollwertig im Suggest-Index.

**Brauchbar wie sie sind — mit zwei benannten Einschränkungen.**

1. **Duplikate ohne Unterscheidungsmerkmal.** `Asteraceae` erscheint
   zweimal in derselben Trefferliste, mit identischem Score und identischer
   Anzeige. Weder `/v1/suggest` noch `/v1/concept` geben ein `sec.`-Feld
   aus — ein Nutzer kann die beiden Treffer nicht auseinanderhalten. Das
   ist die eine Änderung, die es braucht: `sec` in die Antwort beider
   Endpunkte, oder eine Deduplikation je Namensform mit Vorzugsraum.
2. **Familien sind nicht übersetzbar.** Keines der 629 Familienkonzepte
   trägt eine Relation:

   ```bash
   sqlite3 -readonly full.sqlite \
     "SELECT COUNT(*) FROM taxon_concept tc WHERE tc.rank='FAMILY'
        AND (tc.id IN (SELECT from_concept FROM concept_relation)
          OR tc.id IN (SELECT to_concept FROM concept_relation));"
   # 0
   ```

   `/v1/translate` antwortet für sie immer `no_relation_recorded`. Für die
   vage Erfassung ist das egal; für eine spätere Harmonisierung der
   Familienumschreibungen (APG vs. BRUMMITT 1992) ist es die zentrale
   Lücke.

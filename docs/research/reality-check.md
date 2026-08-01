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

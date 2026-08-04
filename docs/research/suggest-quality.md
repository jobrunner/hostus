# Suggest-Qualität: Latenz und Zusammensetzung gemessen (SP7, Task 1)

Diese Messung verlängert `docs/research/reality-check.md` (M4, T7.1/T7.2) um
genau die Frage, die dort benannt und nicht beantwortet wurde: Kurze
Präfixe treffen Zehntausende Konzepte, und `GET /v1/suggest` bezahlt das mit
p50 bis 373 ms. Gemessen wird, **was die beiden freigegebenen Hebel
(Gebietseinschränkung, Kandidaten-Cap) kosten und bringen** — und wie die
Ergebnisliste heute tatsächlich zusammengesetzt ist.

Nichts wird in diesem Task implementiert. Die Zahlen entscheiden die Form
von Task 2 (Ranking) und Task 3 (Query-/API-Form).

## Aufbau

| | |
|---|---|
| Index | `/tmp/full-real.sqlite`, 440.534 Konzepte, 1.448.984 Namen, 1.983.859 Distributionszeilen, 381 WGSRPD-L3-Gebiete |
| Öffnung | strikt lesend (`file:…?mode=ro&_pragma=query_only(1)`) — `sqlite.Open` der Produktion würde Migrationen anwenden |
| Harness | `poc/measure/suggestquality` (neu), `poc/measure/latency` (erweitert um `--runs`) |
| Rohausgaben | `poc/measure/out/sp7-t1-{latency,composition,http-baseline}.txt` — **nicht versioniert** (`poc/.gitignore`); die vollständigen Tabellen stehen darum im [Anhang](#anhang-vollstandige-tabellen) dieses Dokuments |
| Präfixmenge | 38 Präfixe, identisch mit der aus M4: 15 × 2 Zeichen (inkl. `ca`, `al`, `sa`, `tr`, `ac`), 2 × 3, 10 × 4, 10 × 5, 1 × 6 |
| Gebietsmenge | `GER,AUT,SWI,CZE,POL,HUN,FRA,ITA,NET,BGM,DEN` — **die Liste aus `poc/measure/run.sh`, Schritt `m5`**, unverändert übernommen |
| Läufe | 5 vollständige Läufe, je 1 Aufwärm- + 3 gemessene Wiederholungen pro Präfix → 114 Messpunkte pro Lauf und Szenario |

**Was diese p95 nicht sind:** sie sind über eine *künstliche* Präfixmischung
gepoolt — 15 der 38 Präfixe sind die pathologischen 2-Zeichen-Fälle. Die
Zahlen charakterisieren diese gewählte Verteilung, **nicht** den
Produktionsverkehr; eine reale Tippsequenz enthält weniger 2-Zeichen-Anfragen
pro Sitzung. Vergleichbar sind die Szenarien nur untereinander, weil sie
dieselbe Mischung fahren. `quantile` rechnet mit abgerundetem Nearest-Rank
(`s[int(p*(n-1))]`), nicht interpolierend.

Ein einzelner p95 ist hier **keine belastbare Zahl** — T7.1 hat das teuer
gelernt (19 Läufe spannen 225–316 ms, CV 9,0 %). Alle Latenzen stehen darum
als Band über 5 Läufe.

### Warum auf SQL-Ebene gemessen wird

Keiner der beiden Hebel ist heute über HTTP ausdrückbar: `area` löst über
`areaCodes()` (`internal/adapters/sqlite/suggest.go`) auf **höchstens einen**
WGSRPD-L3-Code auf, und einen Cap-Parameter gibt es überhaupt nicht. Die
vier Szenarien laufen darum gegen dieselbe Query, die
`internal/adapters/sqlite/suggest.go` absetzt, in
`poc/measure/suggestquality/main.go` reproduziert.

**Kontrolle dazu:** derselbe Baseline-Fall zusätzlich über den echten
HTTP-Pfad, gegen eine Kopie des Index (die Kopie wurde nach der Messung
gelöscht — der Server hätte sonst Migrationen auf den echten Index
geschrieben):

```bash
cp /tmp/full-real.sqlite /tmp/sp7-t1-http.sqlite
HOSTUS_SQLITE_PATH=/tmp/sp7-t1-http.sqlite ./hostus serve --port 8099 --log-level warn &
./poc/measure/out/latency --base http://127.0.0.1:8099 --runs 3 --reps 8 --warmup 2
```

```
p50 per run: 34.488ms 33.389ms 34.525ms
p95 per run: 219.95ms 251.678ms 236.819ms
p50 band: 33.389ms .. 34.525ms (median 34.488ms)
p95 band: 219.95ms .. 251.678ms (median 236.819ms)
```

Das HTTP-Band (p95 220–252 ms) liegt im SQL-Band (p95 211–294 ms) und im
heutigen M4-Band des Reality-Checks, **225–316 ms**
(`docs/research/reality-check.md:1964`, 19 Läufe, CV 9,0 %). Die oft zitierten
220,2 ms aus `reality-check.md:514` sind dagegen ein **Einzellauf** von vor
der Härtung und taugen nicht als Band.

Die SQL-Messung bildet den Serving-Pfad also ohne systematischen Versatz ab.
Der HTTP-Overhead ist **nicht konstant**: im p50 sind es +14 ms (34 ms HTTP
vs. 20 ms SQL), **am p95 ist er praktisch null** (236,8 vs. 237,1 ms) — die
Tail-Latenz wird vollständig von der Abfrage bestimmt, nicht vom Transport.

## Die vier Szenarien

```bash
(cd poc && go build -o ../poc/measure/out/suggestquality ./measure/suggestquality)
./poc/measure/out/suggestquality --runs 5 --reps 3 --warmup 1 --only latency
```

```
db=/tmp/full-real.sqlite runs=5 reps/prefix=3 warmup=1 limit=10 fetch-budget=40 cap=2000
areas=AUT,BGM,CZE,DEN,FRA,GER,HUN,ITA,NET,POL,SWI (11 codes)
prefixes=38

## S1 baseline (no area, no cap)
p50 per run: 19.94ms 19.392ms 19.115ms 20.572ms 20.861ms
p95 per run: 211.118ms 265.465ms 294.487ms 237.124ms 226.563ms
p50 band: 19.115ms .. 20.861ms (median 19.94ms, CV 3.7%)
p95 band: 211.118ms .. 294.487ms (median 237.124ms, CV 13.4%)

## S2 area-scoped (Mitteleuropa, no cap)
p50 per run: 42.135ms 43.19ms 44.334ms 43.531ms 43.2ms
p95 per run: 471.927ms 429.474ms 403.727ms 408.575ms 376.974ms
p50 band: 42.135ms .. 44.334ms (median 43.2ms, CV 1.8%)
p95 band: 376.974ms .. 471.927ms (median 408.575ms, CV 8.5%)

## S3 cap 2000 (no area)
p50 per run: 13.598ms 13.221ms 13.369ms 13.032ms 13.016ms
p95 per run: 59.541ms 58.983ms 65.101ms 64.556ms 69.702ms
p50 band: 13.016ms .. 13.598ms (median 13.221ms, CV 1.8%)
p95 band: 58.983ms .. 69.702ms (median 64.556ms, CV 7.0%)

## S4 area-scoped + cap 2000
p50 per run: 21.967ms 21.702ms 22.218ms 21.928ms 22.079ms
p95 per run: 71.989ms 68.666ms 74.087ms 67.589ms 75.36ms
p50 band: 21.702ms .. 22.218ms (median 21.967ms, CV 0.9%)
p95 band: 67.589ms .. 75.36ms (median 71.989ms, CV 4.7%)
```

Zusammengefasst, jeweils Median des Bandes, mit dem Band in Klammern:

| Szenario | p50 | p95 | p95 gegen S1 |
|---|---:|---:|---:|
| **S1** Baseline, ohne Gebiet, ohne Cap | 19,9 ms (19,1–20,9) | **237,1 ms** (211,1–294,5) | — |
| **S2** Mitteleuropa-Gebiet, ohne Cap | 43,2 ms (42,1–44,3) | **408,6 ms** (377,0–471,9) | **+72 %** |
| **S3** Cap 2000, ohne Gebiet | 13,2 ms (13,0–13,6) | **64,6 ms** (59,0–69,7) | **−73 %** |
| **S4** Gebiet **und** Cap 2000 | 22,0 ms (21,7–22,2) | **72,0 ms** (67,6–75,4) | **−70 %** |

Über **diese** fünf Läufe sind die Bänder von S1 und S2 getrennt, ebenso die
von S3/S4 und S1. Das ist eine Eigenschaft dieser zehn Stichproben, kein
dauerhafter Fakt: ein unabhängiger S1-Lauf im Review erreichte **365,2 ms**
und lag damit 3 % unter der S2-Untergrenze — bei einem p95 mit CV 13,4 %
ist Nichtüberlappung kein Beweis. Die *Richtung* bleibt abgesichert, weil
sich auch die unabhängig gemessenen Bänder des Reviews trennten (siehe
„Kontrollen zur Query-Form" unten).

#### S2 driftet: das Band ist nicht stationär

Die fünf S2-Läufe sind streng fast fallend — `471,9 → 429,5 → 403,7 →
408,6 → 377,0 ms`. S2 ist das erste Szenario, das `distribution`
(1,98 Mio. Zeilen) überhaupt anfasst, und zahlt damit die Kaltseiten, die S4
später warm erbt. Der Median eines driftenden Laufs ist die falsche
Kennzahl; **die belastbare Zahl ist die Asymptote, ~377 ms** — immer noch
**+59 %** gegenüber S1. Die Drift macht den Vergleich S4-gegen-S2 zudem
leicht günstig für S4. Für die nächste Messung: Szenarienreihenfolge
randomisieren oder `distribution` vorwärmen.

### S2 ist der wichtigste Befund und er widerspricht der Erwartung

Die Gebietseinschränkung macht die Abfrage in der heutigen Query-Form
**langsamer, nicht schneller** — p95 408,6 ms (Asymptote 377 ms) gegenüber
237,1 ms in der Baseline. Das ist kein Widerspruch zur Auftragsentscheidung
(„Gebiet ist der primäre Mechanismus"), sondern eine Aussage über die
*Reihenfolge* der Operationen: die Einschränkung ist ein korreliertes
`EXISTS` auf `distribution`, und `EXPLAIN QUERY PLAN` zeigt, dass es
**zweimal in verschiedener Granularität** anfällt. Das *restringierende*
`EXISTS` steht in `WHERE`, also **vor** `GROUP BY tc.id`, und läuft damit
**pro FTS-Namenszeile** — bei `ca` 100.029-mal. Das `in_area`-`EXISTS` in
der SELECT-Liste läuft pro Gruppe, also 46.249-mal. Zusammen rund **146.000
Indexsonden**, gut das Dreifache der Zahl, die eine erste Fassung dieses
Dokuments nannte. Der Filter läuft **nachdem** die Kandidaten gebildet
wurden: er reduziert die Ausgabe, nicht die Arbeit.

Für die schlimmsten Präfixe (je 15 Messpunkte; alle 38 Präfixe im
[Anhang](#anhang-vollstandige-tabellen)):

| Präfix | S1 p50 / p95 | S2 p50 / p95 | S3 p50 / p95 | S4 p50 / p95 |
|---|---:|---:|---:|---:|
| `ca` | 435,5 / 539,8 ms | 977,7 / **1.089,9 ms** | 118,5 / 163,3 ms | 124,3 / 134,7 ms |
| `al` | 212,7 / 354,3 ms | 403,7 / 478,7 ms | 59,5 / 77,6 ms | 67,7 / 80,7 ms |
| `sa` | 191,3 / 379,5 ms | 374,7 / 473,8 ms | 58,7 / 65,1 ms | 66,8 / 71,9 ms |
| `po` | 183,5 / 380,2 ms | 356,3 / 491,3 ms | 59,1 / 60,2 ms | 68,0 / 78,0 ms |
| `tr` | 173,5 / 223,1 ms | 352,3 / 453,6 ms | 56,1 / 84,9 ms | 65,4 / 81,3 ms |
| `ac` | 148,4 / 207,2 ms | 282,2 / 414,2 ms | 44,9 / 45,7 ms | 53,5 / 69,1 ms |

`ca` überschreitet mit Gebiet allein die Sekunde. Mit Cap **und** Gebiet
(S4) fällt dasselbe `ca` auf 134,7 ms — das Gebiet ist also bezahlbar,
sobald es nicht mehr über die volle Kandidatenmenge läuft.

Der Index fehlt nicht: `distribution` hat
`PRIMARY KEY (concept_id, area_scheme, area_code)`
(`sqlite3 -readonly /tmp/full-real.sqlite ".schema distribution"`), die
Sonde ist indexgestützt. Teuer ist ihre **Anzahl**.

#### Kontrollen zur Query-Form: S2 misst eine Form, die die Produktion nicht fährt

Das ist die offene Flanke dieser Messung, und sie wurde im Review
geschlossen. S2 baut die Abfrage als restringierendes `EXISTS` **plus**
`in_area`-`EXISTS` (`poc/measure/suggestquality/main.go`), während die
Produktion (`internal/adapters/sqlite/suggest.go`) **nur** `in_area` hat,
mit Inline-Platzhaltern statt `json_each`. Sobald restringiert wird, ist
`in_area` konstant 1 — die zweite Sonde ist reine Verschwendung. S2
überzeichnet also.

Drei unabhängig gemessene, schlankere Varianten (je 3 Läufe, p95-Median,
Baseline in derselben Session 233,5 ms):

| Variante | p95 | gegen Baseline |
|---|---:|---:|
| nur Restriktion, `json_each` | 388,6 ms | **+66 %** |
| nur Restriktion, Inline-`IN` | 417,2 ms | **+79 %** |
| **Produktionsform: nur `in_area`, keine Restriktion** | 383,5 ms | **+64 %** |
| (zum Vergleich) S2 dieser Messung | 408,6 ms | +72 % |

Der Befund ist damit robuster als die ursprüngliche Evidenz: **schon das
bloße Berechnen von `in_area` für die Sortierung — was die Produktion heute
tut — kostet +64 % p95.** Keine schlankere Form kommt unter +64 %.

Ein zweiter Abweichungspunkt des Harness wurde im Review gefunden und
behoben: `rankSuggestions` verglich den Status gegen das Kleinbuchstaben-
Literal `"accepted"`, während die Spalte `ACCEPTED` (434.691) bzw. `UNKNOWN`
(5.843) speichert — Schritt 3 von `domain.RankSuggestions` war damit toter
Code. **Auf die hier berichteten Zahlen wirkt das nicht** (alle 40
Kandidaten in den `ac`/`ca`/`al`-Fenstern sind `ACCEPTED`, weshalb die
HTTP-Gegenprobe zeichengleich war); der Vergleich ist jetzt
`strings.EqualFold`, und die Composition-Ausgabe ist danach byte-identisch.

Unabhängige Vorevidenz aus einem anderen Harness und Codepfad:
`docs/research/reality-check.md:1962` hat für `area=GER` (ein einziger Code,
über HTTP) bereits p95 321,57 ms gegen 274,37 ms ohne Gebiet gemessen —
dieselbe Richtung, vor dieser Messung und ohne Kenntnis von ihr.

### Nebenbefund: „DE/AT/CH und Nachbarn" ist über die API nicht ausdrückbar

Die gemessene Gebietsmenge stammt aus `poc/measure/run.sh` (`m5`). Der
Produktionscode kennt sie in zwei verschiedenen Ausbaustufen:
`resolveAreaCodes` (`internal/adapters/sqlite/bundle.go`) versteht für
`hostus bundle --area` eine **kommagetrennte Liste**, `areaCodes`
(`internal/adapters/sqlite/suggest.go`) für `GET /v1/suggest?area=` dagegen
**genau einen** Wert. Die Alias-Tabelle kennt nur `DE`, `AT`, `CH`; die
Nachbarn müssten ohnehin als rohe L3-Codes kommen. Wenn die
Gebietseinschränkung der primäre Mechanismus wird, ist die Mehrcode-Form
von `?area=` in Task 3 eine Voraussetzung, kein Extra.

## Kandidatenmengen, Cap-Wirkung und Gattungsverlust

```bash
./poc/measure/out/suggestquality --only composition
```

Auszug; alle 38 Präfixe im [Anhang](#anhang-vollstandige-tabellen):

| Präfix | FTS-Namenszeilen | Kandidaten global | davon Gattungen | Kandidaten in-area | davon Gattungen | Cap greift | Kandidaten nach Cap | Gattungen nach Cap | Gattungen verloren |
|---|---:|---:|---:|---:|---:|:--:|---:|---:|---:|
| `ac` | 32.936 | 16.841 | 396 | **1.454** | 99 | ja | 1.538 | 396 | **0** |
| `al` | 46.032 | 22.765 | 357 | 3.414 | 96 | ja | 1.507 | 357 | **0** |
| `ca` | 100.029 | 46.249 | 1.032 | 4.491 | 237 | ja | 1.755 | 793 | **239** |
| `po` | 46.786 | 20.957 | 537 | 1.972 | 131 | ja | 1.524 | 537 | **0** |
| `sa` | 45.390 | 20.995 | 454 | 2.276 | 106 | ja | 1.520 | 454 | **0** |
| `tr` | 42.109 | 21.130 | 686 | 2.222 | 177 | ja | 1.569 | 686 | **0** |
| `betu` | 1.306 | 231 | 2 | 31 | 2 | nein | 231 | 2 | 0 |
| `pinus` | 1.824 | 334 | 1 | 75 | 1 | nein | 334 | 1 | 0 |

Der Cap ist auf **FTS-Namenszeilen** angesetzt (`LIMIT 2000` in der
`matches`-CTE), weil dort die Arbeit entsteht; die Zeilen kollabieren
anschließend über `GROUP BY tc.id` auf weniger Konzepte — daher liegt
„Kandidaten nach Cap" durchgängig unter 2000.

Die vier bestellten Zahlen:

1. **Wie oft greift der Cap?** Bei **34 von 38 Präfixen** (89,5 %). Nicht
   gekappt werden nur `betu`, `abies`, `picea`, `pinus` — also genau die
   langen, engen Präfixe, die auch heute schon schnell sind. Der Cap ist
   damit kein Randfall, sondern der Normalfall.
2. **Wie viele Kandidaten bleiben für `ac` in-area?** **1.454 von 16.841
   global — 8,6 %.** Die Gebietseinschränkung ist mit Abstand die
   wirksamste *inhaltliche* Reduktion; nur ist sie in der heutigen
   Query-Form die teuerste.
3. **Was kostet der Cap an Gattungen, wenn er vor der Rangdiversität
   greift?** Bei **37 von 38 Präfixen: null.** Ausnahme ist genau ein
   Präfix, `ca`: 239 von 1.032 Gattungen (23,2 %) fallen weg.
4. **Gilt das auch für die Präfixe des Auftrags?** `ac`, `al`, `sa`, `tr`,
   `po` verlieren null Gattungen, obwohl der Cap bei allen greift.

**Damit ist das Ordnungsargument aus dem Plan messbar schwach.** Der Auftrag
hat ausdrücklich verlangt, das offen zu sagen, falls die Zahl null ist: sie
ist es in 37 von 38 Fällen. „Cap nach Rangdiversität" lässt sich mit diesen
Zahlen **nicht** über den Gattungsverlust rechtfertigen — nur über den einen
Fall `ca`. Der Grund dafür steht im nächsten Abschnitt: der Cap schneidet so
grob, dass er die Gattungen gar nicht erreicht, weil bm25 sie ohnehin nicht
sortiert.

## Zusammensetzung: das Flooding ist real und schlimmer als angenommen

Rang-Histogramm der heutigen Top 10, ohne `area`, `limit=10`
(Fetch-Budget 40, danach `domain.RankSuggestions`):

```
`ac`: SPECIES=10
`ca`: SPECIES=10
`al`: SPECIES=10
```

Die Listen im Wortlaut (Ausgabe von `--only composition`; über HTTP mit
`curl -s "http://127.0.0.1:8099/v1/suggest?q=ac&limit=10"` **zeichengleich**
nachgeprüft):

```
`ac`:  Lucuma multiflora · Acalypha acapulcensis · Acalypha accedens ·
       Acalypha acmophylla · Acalypha acrogyna · Acalypha acuminata ·
       Pleurothallis acestrophylla · Pleurothallis acutilabia ·
       Sciodaphyllum acuminatum · Acidocroton acunae
`ca`:  Kunzea capitata · Ladenbergia carua · Ladenbergia macrocarpa ·
       Ladenbergia moritziana · Ladenbergia oblongifolia ·
       Landolphia calabarica · Landolphia camptoloba · Lantana canescens ·
       Macbridea caroliniana · Maxillaria cacaoensis
`al`:  Lepechinia schiedeana · Xanthosoma sagittifolium ·
       Adelmeria alpinum · Alpinia calcarata · Alpinia galanga ·
       Alpinia nigra · Kernera saxatilis · Lomatocarpa albomarginata ·
       Myosotis albiflora · Nonea echioides
```

Drei Befunde, alle drei relevant für Task 2:

**(a) Null Gattungen in 30 von 30 Plätzen.** Obwohl `domain.RankSuggestions`
GENUS (Ordinal 1) explizit vor SPECIES (2) einsortiert, taucht keine Gattung
auf. Die Rangpriorität greift ins Leere, weil sie erst **nach** dem
SQL-`LIMIT 40` läuft und unter den 40 geholten Zeilen keine Gattung ist. Die
Forderung „für `Ac` muss *Acer* neben den Arten stehen" ist heute nicht
näherungsweise erfüllt.

**(b) Nur 9 von 30 Treffern beginnen überhaupt mit dem Präfix.** `ac` 6/10,
`al` 3/10, `ca` **0/10**. `Kunzea capitata` steht auf Platz 1 für `ca`, weil
FTS5 **tokenweise** matcht: das Epitheton zählt wie der Gattungsname. Für
ein Autosuggest-Feld ist das der eigentliche Qualitätsmangel — noch vor der
Rangverteilung.

**(c) Die Ursache ist bm25, und die ist quantifizierbar.** `bm25(fts_name)`
liefert für eine Präfixanfrage fast keine Auflösung:

```bash
sqlite3 -readonly /tmp/full-real.sqlite "
WITH m AS MATERIALIZED (SELECT bm25(fts_name) s FROM fts_name WHERE fts_name MATCH '\"ac\"*')
SELECT COUNT(*), COUNT(DISTINCT s), MIN(s),
 SUM(CASE WHEN s=(SELECT MIN(s) FROM m) THEN 1 ELSE 0 END) FROM m;"
```

| Präfix | FTS-Zeilen | **verschiedene bm25-Werte** | bester Wert | Zeilen auf dem besten Wert |
|---|---:|---:|---:|---:|
| `ac` | 32.936 | **11** | −5,4242 | 137 |
| `ca` | 100.029 | **12** | −3,7357 | 1.001 |
| `al` | 46.032 | **12** | −4,9234 | 204 |

33.000 bis 100.000 Zeilen verteilen sich auf **11–12 Score-Werte**. Innerhalb
eines Wertes ist die Reihenfolge willkürlich (rowid). Konkret:

```bash
# Positionen von Acer/Acalypha/Achillea in der heutigen Kandidatenordnung für "ac"
sqlite3 -readonly /tmp/full-real.sqlite "…ROW_NUMBER() OVER (ORDER BY s ASC)…"
Acalypha|GENUS|-4.93488315377987|137
Acer    |GENUS|-4.93488315377987|272
Achillea|GENUS|-4.93488315377987|410
```

Alle drei haben **denselben** Score; die Positionen 137/272/410 sind reines
Tie-Break-Rauschen. Entsprechend nutzlos wäre eine Gattungsquote, die
weiterhin auf bm25 aufsetzt — sie holt die *falschen* Gattungen:

```
`ac` global   erste Gattung an Position 133 von 16841; Top-5: Malaxis, Mallotus, Manilkara, Masdevallia, Acalypha
`ac` in-area  erste Gattung an Position 12 von 1454; Top-5: Malaxis, Acalypha, Aralia, × Orchimantoglossum, Orchis
`ca` global   erste Gattung an Position 963 von 46249; Top-5: Isertia, Ixora, Jatropha, Koilodepas, Korthalsia
`ca` in-area  erste Gattung an Position 103 von 4491; Top-5: Lantana, Leonurus, Melaleuca, Acalypha, Neottia
`al` global   erste Gattung an Position 194 von 22765; Top-5: Landolphia, Lepechinia, Liparis, Machaonia, Magnolia
`al` in-area  erste Gattung an Position 46 von 3414; Top-5: Liparis, Magnolia, Vitex, Commelina, Laserpitium
```

*Acer* ist in keiner dieser Listen. Eine Quote von drei GENUS-Plätzen für
`ac` liefert heute **Malaxis, Mallotus, Manilkara**.

**Kein Bug, aber hier zu nennen:** FAMILY-Konzepte existieren im Index nicht
(0 von 440.534). `Aceraceae` ist aus WCVP nicht vorschlagbar. Ein
Rangordinal 0 für FAMILY ist in `domain.rankOrder` vorhanden, aber tot.
`parent_id` ist auf 423.631 / 440.534 Konzepten (96,2 %) gesetzt, die
Hierarchie GENUS→SPECIES ist also begehbar.

## Empfehlung für Task 2

Die Messungen tragen **keine** Empfehlung für eine Rangquote als primären
Hebel. Eine Quote verteilt Plätze innerhalb einer Ordnung; die Ordnung
selbst ist hier das Defekte (11–12 Score-Werte, Gattungen zufällig auf
Position 133/272/410, 0/10 präfixbeginnende Treffer bei `ca`). Eine Quote
auf dieser Grundlage macht die Liste rangmäßig bunter, nicht richtiger.

Empfohlene Reihenfolge für Task 2, in dieser Priorität:

1. **Zuerst ein präfixverankertes Relevanzsignal statt bm25.** Sortierschlüssel
   vor allem anderen: (i) beginnt der *akzeptierte* Name mit dem Präfix,
   (ii) beginnt das erste Token (der Gattungsname) mit dem Präfix, (iii)
   Länge/Tokenzahl. Das ist der Schritt, der `Kunzea capitata` von Platz 1
   für `ca` entfernt. Ohne ihn ist jeder weitere Schritt Kosmetik.

   **Abnahmekriterien, nach Scope getrennt — das ist wichtig, weil dieser
   Schlüssel allein die Hierarchieforderung nicht erfüllt:**

   - *ohne `area`:* Anteil präfixbeginnender Treffer in den Top 10 von
     heute **9/30** auf ≥ 27/30. Ehrlicherweise: dieses Kriterium ist mit
     Schritt 1 **nahezu trivial erfüllt** und damit kein echtes Gate — es
     dokumentiert nur, dass der Schlüssel überhaupt greift.
   - *mit Mitteleuropa-Scoping:* `Acer` in den Top 10 für `ac`.

   Warum die Trennung nötig ist: **142 verschiedene GENUS-Canonicals der
   `ac`-Menge beginnen mit `Ac`** (*Acacia, Acaciella, Acaena, Acalypha,
   Acampe, …*). Unter dem obigen Schlüssel — alle 142 sind gleich lang
   qualifiziert, Restsortierung nach Länge — landet *Acer* etwa auf Rang
   30–40 unter den Gattungen, also **nicht** in den Top 10. Erst mit
   Gebietsscoping wird das Kriterium erreichbar: in Mitteleuropa gibt es
   nur **18** `Ac*`-Gattungen, und *Acer* ist die siebte alphabetisch
   (`Acacia, Acaena, Acalypha, Acanthoprasium, Acanthospermum, Acanthus,
   Acer, Achillea, …`). Das Kriterium „*Acer* in den Top 10" setzt also
   **genau den Hebel voraus, den dieses Dokument als den teuren
   klassifiziert.** Wer es ohne `area` erreichen will, braucht ein
   *zusätzliches* Signal — Popularität/Nutzungshäufigkeit oder `in_area`
   als Rangschlüssel statt als Filter. Das ist eine Entwurfsentscheidung
   für Task 2, keine Messfrage.

   ```bash
   # 142 global / 18 in Mitteleuropa
   sqlite3 -readonly /tmp/full-real.sqlite "
   WITH matches AS MATERIALIZED (SELECT rowid FROM fts_name WHERE fts_name MATCH '\"ac\"*')
   SELECT COUNT(DISTINCT an.canonical) FROM matches m
     JOIN fts_name_map f ON f.rowid=m.rowid
     JOIN taxon_concept tc ON tc.id=f.concept_id
     JOIN name an ON an.id=tc.accepted_name
   WHERE an.rank='GENUS' AND lower(an.canonical) LIKE 'ac%';"
   ```
2. **Danach eine kleine, gedeckelte Gattungsquote** (Größenordnung 2–3
   Plätze von 10), damit die Hierarchieforderung auch dann hält, wenn viele
   Arten derselben Gattung präfixbeginnend sind (`Acalypha …` × 5 im
   heutigen `ac`-Ergebnis). Erst nach Schritt 1 sinnvoll, weil erst dann
   die richtigen Gattungen zur Verfügung stehen. Eine Familienquote hat
   keinen Gegenstand (FAMILY = 0).
3. **Cap vor Rangdiversität ist zulässig** — und zwar aus Latenzgründen,
   nicht aus Diversitätsgründen. Der gemessene Gattungsverlust ist bei 37
   von 38 Präfixen null; nur `ca` verliert 239 von 1.032 Gattungen. Das ist
   zu wenig, um die Reihenfolge „Diversität vor Cap" zu erzwingen, aber
   genug, um den Cap **nicht blind auf bm25-Reihenfolge** zu setzen.
   *Prognose, nicht gemessen:* sobald Schritt 1 umgesetzt ist, sortiert der
   Cap nach einem sinnvollen Schlüssel, und der `ca`-Verlust dürfte als
   Argument verschwinden — das ist billig nachzumessen und sollte in Task 2
   nachgemessen statt geglaubt werden. **Wenn Task 2
   die Reihenfolge „Diversität zuerst" beibehält, dann als Design-Prinzip,
   nicht mit dieser Zahl begründet.**

Für Task 3 (Query-/API-Form) folgt aus S2/S4 zwingend:

- Der Cap gehört **in die `matches`-CTE**, nicht hinter den Join. S3 zeigt
  den vollen Gewinn (p95 −73 %) genau dort.
- Die Gebietseinschränkung ist **nur zusammen mit dem Cap** bezahlbar: allein
  kostet sie +72 % p95 (`ca` > 1 s), zusammen mit dem Cap liegt sie bei
  72,0 ms. Die Auftragsentscheidung „Gebiet ist der primäre Mechanismus"
  bleibt richtig — sie braucht aber den Cap als Voraussetzung, nicht als
  Alternative.
- `?area=` muss mehrere WGSRPD-L3-Codes annehmen (`resolveAreaCodes` aus
  `bundle.go` kann das bereits, `areaCodes` aus `suggest.go` nicht).

## Anhang: vollständige Tabellen

Die Rohausgaben unter `poc/measure/out/` sind über `poc/.gitignore`
nicht versioniert. Weil diese Messung das einzige Artefakt des
Meilensteins und der Einstiegspunkt für Task 2/3 ist, stehen die
vollständigen Tabellen hier im Dokument.

### S1 — Baseline (ohne Gebiet, ohne Cap)

p50/p95 je Präfix über alle 5 Läufe (15 Messpunkte je Präfix).

| Prefix | p50 | p95 | max |
|---|---:|---:|---:|
| `ac` | 148.372ms | 207.15ms | 400.895ms |
| `ag` | 38.931ms | 46.635ms | 56.713ms |
| `al` | 212.69ms | 354.305ms | 363.658ms |
| `be` | 82.924ms | 108.613ms | 110.218ms |
| `ca` | 435.457ms | 539.77ms | 910.15ms |
| `ce` | 90.37ms | 132.913ms | 133.968ms |
| `fe` | 46.195ms | 67.989ms | 78.202ms |
| `ga` | 84.968ms | 128.801ms | 186.122ms |
| `po` | 183.547ms | 380.154ms | 387.876ms |
| `qu` | 43.324ms | 43.788ms | 68.162ms |
| `ra` | 99.031ms | 131.731ms | 164.24ms |
| `sa` | 191.294ms | 379.503ms | 416.305ms |
| `th` | 72.764ms | 121.325ms | 202.312ms |
| `tr` | 173.523ms | 223.104ms | 253.392ms |
| `ve` | 128.812ms | 195.027ms | 219.915ms |
| `ace` | 11.972ms | 13.489ms | 14.576ms |
| `ach` | 10.874ms | 14.002ms | 16.167ms |
| `acer` | 9.063ms | 9.427ms | 10.21ms |
| `arte` | 7.513ms | 7.766ms | 7.786ms |
| `betu` | 3.922ms | 6.227ms | 8.118ms |
| `cala` | 20.455ms | 26.694ms | 32.153ms |
| `care` | 26.142ms | 28.141ms | 28.154ms |
| `cent` | 19.542ms | 27.678ms | 28.198ms |
| `fest` | 11.664ms | 15.719ms | 18.461ms |
| `gali` | 10.881ms | 11.492ms | 15.073ms |
| `hier` | 54.891ms | 59.772ms | 62.136ms |
| `ranu` | 16.17ms | 19.864ms | 21.564ms |
| `abies` | 2.089ms | 2.225ms | 2.267ms |
| `picea` | 1.832ms | 1.864ms | 1.883ms |
| `pinus` | 4.616ms | 4.695ms | 4.763ms |
| `querc` | 14.634ms | 17.165ms | 18.874ms |
| `rubus` | 27.514ms | 28.592ms | 32.18ms |
| `salix` | 10.489ms | 12.508ms | 12.703ms |
| `thymu` | 5.612ms | 6.625ms | 7.683ms |
| `trifo` | 9.978ms | 10.761ms | 12.462ms |
| `veron` | 7.611ms | 8.178ms | 8.231ms |
| `viola` | 19.872ms | 20.789ms | 29.816ms |
| `potent` | 12.619ms | 13.448ms | 15.226ms |

### S2 — Gebiet Mitteleuropa (ohne Cap)

p50/p95 je Präfix über alle 5 Läufe (15 Messpunkte je Präfix).

| Prefix | p50 | p95 | max |
|---|---:|---:|---:|
| `ac` | 282.241ms | 414.211ms | 424.011ms |
| `ag` | 76.841ms | 92.567ms | 94.222ms |
| `al` | 403.727ms | 478.682ms | 486.922ms |
| `be` | 160.683ms | 181.088ms | 207.179ms |
| `ca` | 977.693ms | 1.089879s | 1.095914s |
| `ce` | 180.761ms | 205.266ms | 220.667ms |
| `fe` | 89.527ms | 142.414ms | 147.609ms |
| `ga` | 140.486ms | 162.732ms | 254.185ms |
| `po` | 356.267ms | 491.303ms | 576.015ms |
| `qu` | 85.239ms | 100.186ms | 170.541ms |
| `ra` | 188.546ms | 275.51ms | 288.131ms |
| `sa` | 374.731ms | 473.824ms | 537.841ms |
| `th` | 144.257ms | 207.412ms | 253.042ms |
| `tr` | 352.253ms | 453.635ms | 554.88ms |
| `ve` | 195.857ms | 274.566ms | 308.083ms |
| `ace` | 24.774ms | 29.128ms | 29.73ms |
| `ach` | 21.26ms | 22.096ms | 22.384ms |
| `acer` | 19.069ms | 24.279ms | 31.468ms |
| `arte` | 17.302ms | 17.569ms | 17.71ms |
| `betu` | 8.371ms | 10.861ms | 11.852ms |
| `cala` | 43.367ms | 44.072ms | 47.471ms |
| `care` | 62.939ms | 66.992ms | 97.019ms |
| `cent` | 41.979ms | 42.292ms | 43.964ms |
| `fest` | 23.104ms | 52.671ms | 59.773ms |
| `gali` | 21.988ms | 22.25ms | 22.347ms |
| `hier` | 123.872ms | 125.988ms | 127.092ms |
| `ranu` | 38.39ms | 46.436ms | 49.852ms |
| `abies` | 4.711ms | 5.015ms | 7.513ms |
| `picea` | 3.96ms | 4.017ms | 4.076ms |
| `pinus` | 10.915ms | 11.013ms | 11.057ms |
| `querc` | 33.973ms | 38.95ms | 39.105ms |
| `rubus` | 59.328ms | 77.057ms | 80.65ms |
| `salix` | 24.532ms | 24.821ms | 24.901ms |
| `thymu` | 12.29ms | 12.434ms | 13.351ms |
| `trifo` | 20.167ms | 23.837ms | 24.898ms |
| `veron` | 15.329ms | 15.459ms | 15.715ms |
| `viola` | 40.283ms | 59.765ms | 63.062ms |
| `potent` | 26.264ms | 27.764ms | 64.445ms |

### S3 — Cap 2000 (ohne Gebiet)

p50/p95 je Präfix über alle 5 Läufe (15 Messpunkte je Präfix).

| Prefix | p50 | p95 | max |
|---|---:|---:|---:|
| `ac` | 44.856ms | 45.654ms | 67.458ms |
| `ag` | 19.541ms | 20.648ms | 24.138ms |
| `al` | 59.544ms | 77.563ms | 82.089ms |
| `be` | 32.308ms | 41.645ms | 42.411ms |
| `ca` | 118.522ms | 163.253ms | 200.188ms |
| `ce` | 33.88ms | 52.189ms | 83.003ms |
| `fe` | 22.03ms | 23.184ms | 23.5ms |
| `ga` | 28.692ms | 29.47ms | 29.767ms |
| `po` | 59.13ms | 60.166ms | 96.769ms |
| `qu` | 22.007ms | 24.025ms | 29.472ms |
| `ra` | 34.945ms | 64.436ms | 69.702ms |
| `sa` | 58.691ms | 65.101ms | 95.689ms |
| `th` | 28.817ms | 31.033ms | 34.964ms |
| `tr` | 56.107ms | 84.929ms | 116.098ms |
| `ve` | 37.882ms | 42.699ms | 79.978ms |
| `ace` | 11.453ms | 11.742ms | 15.328ms |
| `ach` | 11.074ms | 14.092ms | 15.439ms |
| `acer` | 9.488ms | 10.053ms | 10.09ms |
| `arte` | 8.36ms | 9.241ms | 32.94ms |
| `betu` | 4.792ms | 6.814ms | 7.195ms |
| `cala` | 13.322ms | 14.213ms | 19.716ms |
| `care` | 14.914ms | 15.32ms | 15.756ms |
| `cent` | 12.423ms | 12.56ms | 12.642ms |
| `fest` | 10.643ms | 11.844ms | 13.221ms |
| `gali` | 10.205ms | 10.992ms | 12.666ms |
| `hier` | 23.277ms | 45.668ms | 57.642ms |
| `ranu` | 11.158ms | 11.477ms | 11.671ms |
| `abies` | 2.642ms | 2.676ms | 2.724ms |
| `picea` | 2.267ms | 2.332ms | 2.351ms |
| `pinus` | 5.803ms | 5.851ms | 5.864ms |
| `querc` | 11.305ms | 11.575ms | 11.697ms |
| `rubus` | 14.894ms | 15.175ms | 15.482ms |
| `salix` | 9.138ms | 9.727ms | 9.9ms |
| `thymu` | 6.825ms | 7.039ms | 7.066ms |
| `trifo` | 9.966ms | 10.226ms | 10.483ms |
| `veron` | 8.127ms | 8.46ms | 8.504ms |
| `viola` | 12.411ms | 12.714ms | 12.934ms |
| `potent` | 10.836ms | 11.351ms | 13.741ms |

### S4 — Gebiet + Cap 2000

p50/p95 je Präfix über alle 5 Läufe (15 Messpunkte je Präfix).

| Prefix | p50 | p95 | max |
|---|---:|---:|---:|
| `ac` | 53.512ms | 69.053ms | 75.36ms |
| `ag` | 28.078ms | 33.893ms | 80.859ms |
| `al` | 67.684ms | 80.678ms | 85.691ms |
| `be` | 40.419ms | 50.02ms | 56.424ms |
| `ca` | 124.328ms | 134.703ms | 235.653ms |
| `ce` | 42.396ms | 53.344ms | 67.605ms |
| `fe` | 30.348ms | 44.519ms | 53.299ms |
| `ga` | 36.76ms | 50.817ms | 56.701ms |
| `po` | 68.014ms | 78.031ms | 80.804ms |
| `qu` | 29.371ms | 29.993ms | 30.994ms |
| `ra` | 43.007ms | 44.573ms | 55.82ms |
| `sa` | 66.819ms | 71.933ms | 77.03ms |
| `th` | 37.493ms | 41.969ms | 89.604ms |
| `tr` | 65.356ms | 81.277ms | 85.619ms |
| `ve` | 45.503ms | 46.482ms | 66.056ms |
| `ace` | 19.164ms | 19.514ms | 19.806ms |
| `ach` | 19.203ms | 19.421ms | 19.618ms |
| `acer` | 16.958ms | 18.409ms | 21.333ms |
| `arte` | 16.231ms | 20.632ms | 40.497ms |
| `betu` | 9.131ms | 9.354ms | 9.536ms |
| `cala` | 22.43ms | 29.356ms | 53.398ms |
| `care` | 22.741ms | 24.926ms | 25.511ms |
| `cent` | 20.945ms | 23.538ms | 30.206ms |
| `fest` | 17.991ms | 19.738ms | 21.641ms |
| `gali` | 17.449ms | 17.918ms | 17.985ms |
| `hier` | 31.71ms | 33.937ms | 65.616ms |
| `ranu` | 18.968ms | 19.825ms | 21.119ms |
| `abies` | 5.232ms | 5.291ms | 5.438ms |
| `picea` | 4.338ms | 4.402ms | 4.464ms |
| `pinus` | 12.031ms | 12.14ms | 12.151ms |
| `querc` | 19.183ms | 22.583ms | 22.594ms |
| `rubus` | 22.063ms | 23.394ms | 29.378ms |
| `salix` | 16.274ms | 20.504ms | 31.084ms |
| `thymu` | 13.193ms | 14.351ms | 15.126ms |
| `trifo` | 17.558ms | 19.442ms | 59.844ms |
| `veron` | 14.364ms | 16.703ms | 54.986ms |
| `viola` | 20.757ms | 21.995ms | 22.056ms |
| `potent` | 17.459ms | 18.633ms | 22.511ms |

### Zusammensetzung, alle 38 Präfixe

| Prefix | FTS-Namenszeilen | Kandidaten global | davon Gattungen | Kandidaten in-area | davon Gattungen | Cap greift | Kandidaten nach Cap | Gattungen nach Cap | Gattungen verloren |
|---|---:|---:|---:|---:|---:|:--:|---:|---:|---:|
| `ac` | 32936 | 16841 | 396 | 1454 | 99 | ja | 1538 | 396 | 0 |
| `ag` | 10200 | 4474 | 135 | 509 | 48 | ja | 1303 | 135 | 0 |
| `al` | 46032 | 22765 | 357 | 3414 | 96 | ja | 1507 | 357 | 0 |
| `be` | 19855 | 11420 | 286 | 875 | 69 | ja | 1616 | 286 | 0 |
| `ca` | 100029 | 46249 | 1032 | 4491 | 237 | ja | 1755 | 793 | 239 |
| `ce` | 23552 | 11095 | 313 | 1254 | 97 | ja | 1440 | 313 | 0 |
| `fe` | 11111 | 5361 | 81 | 655 | 22 | ja | 1402 | 81 | 0 |
| `ga` | 18063 | 9294 | 270 | 957 | 71 | ja | 1327 | 270 | 0 |
| `po` | 46786 | 20957 | 537 | 1972 | 131 | ja | 1524 | 537 | 0 |
| `qu` | 10855 | 4497 | 83 | 360 | 15 | ja | 1206 | 83 | 0 |
| `ra` | 22147 | 11902 | 208 | 1530 | 52 | ja | 1436 | 208 | 0 |
| `sa` | 45390 | 20995 | 454 | 2276 | 106 | ja | 1520 | 454 | 0 |
| `th` | 18255 | 9633 | 303 | 845 | 81 | ja | 1428 | 303 | 0 |
| `tr` | 42109 | 21130 | 686 | 2222 | 177 | ja | 1569 | 686 | 0 |
| `ve` | 24957 | 12782 | 125 | 1464 | 42 | ja | 1405 | 125 | 0 |
| `ace` | 3584 | 1006 | 17 | 131 | 9 | ja | 937 | 17 | 0 |
| `ach` | 2654 | 1379 | 61 | 184 | 17 | ja | 1293 | 61 | 0 |
| `arte` | 2428 | 874 | 3 | 103 | 1 | ja | 815 | 3 | 0 |
| `betu` | 1306 | 231 | 2 | 31 | 2 | nein | 231 | 2 | 0 |
| `cala` | 5758 | 2820 | 43 | 192 | 11 | ja | 1287 | 43 | 0 |
| `care` | 10076 | 2916 | 7 | 370 | 3 | ja | 1009 | 7 | 0 |
| `cent` | 5741 | 2663 | 59 | 445 | 18 | ja | 1346 | 59 | 0 |
| `fest` | 3974 | 1092 | 5 | 310 | 3 | ja | 1035 | 5 | 0 |
| `gali` | 3603 | 1268 | 14 | 182 | 5 | ja | 1131 | 14 | 0 |
| `hier` | 19461 | 9525 | 17 | 3165 | 8 | ja | 1523 | 17 | 0 |
| `potent` | 4723 | 983 | 1 | 138 | 1 | ja | 878 | 1 | 0 |
| `querc` | 5305 | 1166 | 2 | 105 | 1 | ja | 813 | 2 | 0 |
| `ranu` | 5815 | 2170 | 2 | 455 | 2 | ja | 1314 | 2 | 0 |
| `salix` | 4140 | 902 | 1 | 178 | 1 | ja | 730 | 1 | 0 |
| `thymu` | 2089 | 551 | 1 | 82 | 1 | ja | 544 | 1 | 0 |
| `trifo` | 2882 | 944 | 3 | 204 | 3 | ja | 794 | 3 | 0 |
| `veron` | 2611 | 805 | 2 | 105 | 1 | ja | 734 | 2 | 0 |
| `viola` | 5449 | 1946 | 2 | 356 | 1 | ja | 1076 | 2 | 0 |
| `abies` | 843 | 164 | 1 | 38 | 1 | nein | 164 | 1 | 0 |
| `acer` | 2844 | 606 | 10 | 71 | 5 | ja | 562 | 10 | 0 |
| `picea` | 738 | 127 | 1 | 35 | 1 | nein | 127 | 1 | 0 |
| `pinus` | 1824 | 334 | 1 | 75 | 1 | nein | 334 | 1 | 0 |
| `rubus` | 10889 | 2479 | 1 | 1394 | 1 | ja | 898 | 1 | 0 |

## Reproduktion

```bash
(cd poc && go build -o ../poc/measure/out/suggestquality ./measure/suggestquality)
(cd poc && go build -o ../poc/measure/out/latency        ./measure/latency)

./poc/measure/out/suggestquality --runs 5 --reps 3 --warmup 1 --only latency \
  > poc/measure/out/sp7-t1-latency.txt
./poc/measure/out/suggestquality --only composition \
  > poc/measure/out/sp7-t1-composition.txt

# HTTP-Kontrolle gegen eine Kopie (die Kopie danach loeschen!)
cp /tmp/full-real.sqlite /tmp/sp7-t1-http.sqlite
HOSTUS_SQLITE_PATH=/tmp/sp7-t1-http.sqlite ./hostus serve --port 8099 --log-level warn &
./poc/measure/out/latency --base http://127.0.0.1:8099 --runs 3 --reps 8 --warmup 2 \
  > poc/measure/out/sp7-t1-http-baseline.txt
pkill -f "hostus serve --port 8099"; rm -f /tmp/sp7-t1-http.sqlite*
```

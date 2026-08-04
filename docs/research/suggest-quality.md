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
| Rohausgaben | `poc/measure/out/sp7-t1-latency.txt`, `…-composition.txt`, `…-http-baseline.txt` |
| Präfixmenge | 38 Präfixe, identisch mit der aus M4: 15 × 2 Zeichen (inkl. `ca`, `al`, `sa`, `tr`, `ac`), 2 × 3, 10 × 4, 10 × 5, 1 × 6 |
| Gebietsmenge | `GER,AUT,SWI,CZE,POL,HUN,FRA,ITA,NET,BGM,DEN` — **die Liste aus `poc/measure/run.sh`, Schritt `m5`**, unverändert übernommen |
| Läufe | 5 vollständige Läufe, je 1 Aufwärm- + 3 gemessene Wiederholungen pro Präfix → 114 Messpunkte pro Lauf und Szenario |

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
M4-Band aus dem Reality-Check (220,2 ms). Die SQL-Messung bildet den
Serving-Pfad also ohne systematischen Versatz ab; der HTTP-Overhead steckt
im p50 (34 ms HTTP vs. 20 ms SQL) und ist konstant.

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

Die Bänder von S1 und S2 überlappen nicht, die von S3/S4 und S1 ebenfalls
nicht. Beide Richtungsaussagen sind also belastbar.

### S2 ist der wichtigste Befund und er widerspricht der Erwartung

Die Gebietseinschränkung macht die Abfrage in der heutigen Query-Form
**langsamer, nicht schneller** — p95 408,6 ms gegenüber 237,1 ms in der
Baseline. Das ist kein Widerspruch zur Auftragsentscheidung („Gebiet ist der
primäre Mechanismus"), sondern eine Aussage über die *Reihenfolge* der
Operationen: die Einschränkung ist heute ein korreliertes `EXISTS` auf
`distribution`, das **pro Kandidatenkonzept einmal** ausgewertet wird. Bei
`ca` sind das 46.249 Indexsonden, und der Filter läuft **nachdem** die
Kandidaten schon gebildet wurden. Er reduziert also die Ausgabe, nicht die
Arbeit.

Für die schlimmsten Präfixe (Auszug aus den Volltabellen in
`poc/measure/out/sp7-t1-latency.txt`, je 15 Messpunkte):

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

Vollständige Tabelle in `poc/measure/out/sp7-t1-composition.txt`. Auszug:

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
   Länge/Tokenzahl. Das ist der Schritt, der `Acer` für `ac` überhaupt in
   Reichweite bringt und `Kunzea capitata` von Platz 1 für `ca` entfernt.
   Ohne ihn ist jeder weitere Schritt Kosmetik. Messbares Abnahmekriterium:
   Anteil präfixbeginnender Treffer in den Top 10 von heute **9/30** auf
   ≥ 27/30, und `Acer` in den Top 10 für `ac`.
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
   genug, um den Cap **nicht blind auf bm25-Reihenfolge** zu setzen: sobald
   Schritt 1 umgesetzt ist, sortiert der Cap ohnehin nach einem sinnvollen
   Schlüssel und der `ca`-Verlust verschwindet als Argument. **Wenn Task 2
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

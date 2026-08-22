# Fuzzy-Prefilter: greift der Vorfilter am echten Index überhaupt?

Stand: 2026-08-22. Messung, keine Ableitung. Grundlage: ein frisch aus
`build-full.yaml` gebauter Index (WCVP 2026-06-15 + CDM + FloraVeg + EuroSL +
Wikidata + drei Trait-Vokabulare, 2,13 GB), das Messwerkzeug
`poc/measure/fuzzyrecall`, und als Kontrollgruppe die 13.791 Artenrollen-Zeilen
des ESy-Datensatzes aus `situs`. Anlass:
[Issue #67](https://github.com/jobrunner/hostus/issues/67), Fehlerklasse 3
(„Keine Fuzzy-Auflösung", 56 Namen).

Alle Abfragen mit `entry_backbone = wcvp`, wie die Reproduktion im Ticket.

## Die Frage — und warum die Pool-Größe sie nicht beantwortet

Der Vorfilter in `internal/adapters/sqlite/read.go`
(`fuzzyCandidateRows`) wählt Namen per `GLOB '<erste Rune>*'` plus
Längenfenster ±3 Runen, ordnet nach `ABS(Längendifferenz), canonical_fold`
— also **alphabetisch** — und nimmt `LIMIT 20`.

Die naheliegende Metrik wäre „wie groß ist der Kandidatenpool". Die ist
irreführend: ein kleinerer Pool kann das Ziel genauso zuverlässig verfehlen.
Gemessen wird deshalb **„ist das Ziel in der zurückgegebenen Menge"**
(`found`) und **„ist es der bestbewertete Kandidat oberhalb des
Schwellwerts"** (`top`) — Letzteres ist die Bedingung, unter der die
Anwendungsschicht überhaupt auflöst.

## Aufbau

Zwei Query-Mengen, weil beide je eine Schwäche haben:

- **Synthetisch** (366 Fälle aus 60 Basisnamen): echte binomiale
  `SPECIES`-Namen aus dem Index, deterministisch mutiert. Das Ziel ist damit
  **per Konstruktion bekannt**, Recall also exakt messbar. Fünf Klassen:
  Vertauschung im Epitheton, fehlender Buchstabe, a/e-Verwechslung,
  Vertauschung in der Gattung an Position 2 bzw. 5, sowie zwei
  Gattungstausch-Klassen (gleicher/anderer Anfangsbuchstabe).
- **Real** (418 Fälle): ESy-`verbatim_name`-Werte **ohne** Exact-Match im
  `wcvp`-Backbone — genau die Zeilen, die in Produktion auf den Fuzzy-Pfad
  laufen. Ihr korrektes Ziel ist *nicht* bekannt, sie werden daher auf
  „löst überhaupt etwas aus" gemessen und der Spitzenkandidat von Hand
  geprüft.

Die Kanonisierung und die Similarity-Metrik werden per
`poc/measure/gen_fuzzy_domain.sh` **verbatim** aus `internal/domain`
kopiert (Gos `internal/`-Regel ist pfadbasiert, das poc-Modul darf nicht
importieren) — nachgebaut würde eine andere Metrik gemessen als der Dienst
verwendet.

## Befund 1: Der Vorfilter ist bei 0,0 %. Überall.

| Klasse | Fälle | `found` | `top` | p95 |
|---|---|---|---|---|
| Vertauschung im Epitheton | 59 | **0,0 %** | 0,0 % | 1,195 s |
| Buchstabe fehlt | 59 | **0,0 %** | 0,0 % | 769 ms |
| a/e-Verwechslung | 59 | **0,0 %** | 0,0 % | 651 ms |
| Vertauschung Gattung Pos. 2 | 51 | **0,0 %** | 0,0 % | 991 ms |
| Vertauschung Gattung Pos. 5 | 51 | **0,0 %** | 0,0 % | 936 ms |
| ESy, real | 418 | — | — | 880 ms |

Nicht „schlecht bei Gattungs-Synonymie" (die Rahmung von Issue #67), sondern
**wirkungslos bei einem einzigen vertauschten Buchstaben** — dem Fall, gegen
den die Spec ihren Schwellwert austariert hat. Und der Fehlschlag kostet
**0,65–1,2 s p95** pro Versuch.

Ursache: bei ~41.000 Zeilen im Fenster gehen die 20 Slots an alphabetisch
frühe Namen; das Ziel wird nie gescort. Die alphabetische Ordnung war als
*Determinismus*-Maßnahme gedacht (ohne `ORDER BY` liefert SQLite eine
beliebige Teilmenge) — sie macht die Auswahl deterministisch **falsch**.

## Befund 2: Das LIMIT ist der Fehler, nicht die Prefixlänge

Eine frühere Sondierung hatte einen 4-Zeichen-Prefix verworfen, weil er das
Ziel „trotzdem nicht" liefert. Das war **mit** dem `LIMIT 20` gemessen. Ohne
das LIMIT — den verbleibenden Pool vollständig in Go scoren — kippt das
Ergebnis:

| Strategie | Vertauschung | Buchstabe fehlt | a/e | ⌀ Pool | p95 |
|---|---|---|---|---|---|
| heute (1 Rune, alpha, LIMIT 20) | 0,0 % | 0,0 % | 0,0 % | 20 | 651–1195 ms |
| 1 Rune, kein LIMIT | 96,6 % | 100 % | 100 % | 41.545 | 743–789 ms |
| 2 Runen, kein LIMIT | 96,6 % | 100 % | 100 % | 5.966 | 99–122 ms |
| 3 Runen, kein LIMIT | 96,6 % | 100 % | 100 % | 1.292 | 29–40 ms |
| **4 Runen, kein LIMIT** | **96,6 %** | **100 %** | **100 %** | **597** | **14–20 ms** |
| 5 Runen, kein LIMIT | 96,6 % | 100 % | 100 % | 520 | 11–14 ms |

(Spalten = `top`, also tatsächlich aufgelöst, nicht nur enthalten.)

Der 4-Runen-Prefix ohne LIMIT ist damit **gleichzeitig korrekt und ~50×
schneller als der heutige kaputte Zustand** — der Pool, der sortiert werden
muss, fällt von ~41.000 auf ~600 Zeilen. Die 600 Levenshtein-Vergleiche in Go
sind gegenüber der Abfrage nicht messbar.

## Befund 3: Die Prefixlänge ist ein echter Trade — und `fts_name` deckt die Lücke

Ein Tippfehler **innerhalb** des Prefixes ist für einen Prefix-Filter
unsichtbar. Gemessen, statt vermutet:

| Strategie | Fehler Gattung Pos. 2 | Fehler Gattung Pos. 5 |
|---|---|---|
| 2 Runen | 2,0 % | 100 % |
| 3 Runen | 2,0 % | 100 % |
| 4 Runen | 2,0 % | 100 % |
| 5 Runen | 2,0 % | 3,9 % |
| `fts_name MATCH <Epitheton>` | **100 %** | **100 %** |
| 4 Runen ∪ FTS-Epitheton | **100 %** | **100 %** |

`fts_name` ist mit `unicode61` tokenisiert, jedes **Wort** eines kanonischen
Namens ist also ein indexierter Token — das Epitheton ist damit ohne
Suffix-Scan erreichbar. Das ist die einzige Route, die einen Fehler in der
Gattung überlebt. Preis: p95 56–74 ms für die Vereinigung statt 14 ms, und
ein ⌀ Pool von 712–1.353.

## Befund 4: Gattungs-Synonymie ist mit Fuzzy grundsätzlich nicht lösbar

Bei vollständigem Gattungstausch liegt das Ziel über die FTS-Route in
**100 %** der Fälle in der Kandidatenmenge — und ist in **0 %** der beste
Kandidat. Der Grund ist nicht der Vorfilter, sondern die Metrik: die
Ganzstring-Ähnlichkeit eines Gattungstauschs liegt unter dem Schwellwert
`domain.FuzzyThreshold` (0,85). An den Beispielen des Tickets selbst:

| Abfrage | Ziel | Similarity |
|---|---|---|
| `astracantha diphtherites` | `astragalus diphtherites` | 0,792 |
| `astracantha diphtherites` | `tragacantha diphtherites` | 0,833 |
| `arctostaphylos alpinus` | `arctous alpina` | 0,545 |
| `bellidiastrum michelii` | `aster bellidiastrum` | 0,318 |

Zum Vergleich der Fall, den die Spec als Fuzzy-Beispiel führt:
`corynephorus canascens` → `corynephorus canescens` = 0,955.

Das ist eine **Datenfrage** (Gattungs-Synonyme), keine Vorfilterfrage. Den
Schwellwert zu senken wäre der falsche Ausweg — nötig wären 0,79, und was
dabei mit hereinkommt, zeigt Befund 5.

## Befund 5: Ohne einen Guard bringt der Fix 30,6 % Fehltreffer mit

Von den 418 realen ESy-Namen erreichen mit `4 Runen ∪ FTS-Epitheton` **62**
den Schwellwert. Von Hand geprüft: **43 plausibel, 19 nachweislich falsch =
30,6 %.** Alle 19 haben dieselbe Ursache — die Gattung ist ein Moos- oder
Flechtengattungsname, den WCVP überhaupt nicht führt, während das Epitheton
zufällig auf eine Blütenpflanze passt:

```
sphagnum platyphyllum  ->  solanum platyphyllum    0,857
cladonia fimbriata     ->  caladenia fimbriata     0,895   (eine Orchidee)
kurzia pauciflora      ->  kunzea pauciflora       0,882
thuidium tamariscinum  ->  thesium tamariscinum    0,857
climacium dendroides   ->  limonium dendroides     0,850
```

Das identische Epitheton ist die **längere** Hälfte des Strings und trägt den
Score über den Schwellwert. Das ist kein Randfall: Moose und Flechten sind
laut Issue #67 rund 8 % der ESy-Zeilen und damit ein großer Teil dessen, was
in der Praxis überhaupt auf den Fuzzy-Pfad läuft. Fuzzy „anzuschalten" würde
sie stillschweigend auf falsche Konzepte auflösen — schlechter als
`unresolvable`.

Richtig aufgelöst werden umgekehrt mehrere Klasse-3-Beispiele des Tickets:

```
arctostaphylos alpinus              ->  arctostaphylos alpina         0,909
astracantha diphtherites            ->  astracantha diphterites       0,958
bellidiastrum michelii              ->  bellidastrum michelii         0,955
abies borisii-regis                 ->  abies × borisii-regis         0,905
artemisia lerchiana                 ->  artemisia lercheana           0,947
festuca marginata subsp. andres-molinae -> ... subsp. andresmolinae    0,974
```

## Befund 6: Ein Guard pro Wort trennt die beiden Gruppen — gemessen

Zusätzlich zur Ganzstring-Ähnlichkeit eine Schwelle auf das **Gattungs-Token
allein** (≥ 0,85):

| | Zahl |
|---|---|
| entfernte Fehltreffer | **18 von 19** |
| verlorene richtige Treffer | **1 von 43** |
| Restleck | 1 |

Das Leck ist `buellia punctata` → `ruellia punctata` (Gattung 0,857) — eine
Flechtengattung, die sich von einer Blütenpflanzengattung in genau einem
Buchstaben unterscheidet. Dagegen hilft kein String-Maß.

Der Grund, dass der Guard so scharf trennt: bei einem Schreibfehler weicht
die Gattung um höchstens einen Buchstaben ab (`cochleria`/`cochlearia` =
0,900, `dorystoechas`/`dorystaechas` = 0,917, `bellidiastrum`/`bellidastrum`
= 0,923), bei einer verwechselten Gattung dagegen um zwei bis vier
(`cladonia`/`caladenia` = 0,778, `sphagnum`/`solanum` = 0,625,
`mylia`/`myrcia` = 0,667).

## Empfehlung

Drei Änderungen, in dieser Reihenfolge — die zweite ist die Bedingung dafür,
dass die erste keinen Schaden anrichtet:

1. **`LIMIT` streichen, GLOB-Prefix auf 4 Runen verengen.** Recall 0 → 96,6–100 %
   bei Schreibfehlern, p95 von 651–1195 ms auf 14–20 ms.
2. **Guard pro Wort im Scoring** (Gattungs-Token ≥ Schwellwert, nicht nur der
   Gesamtstring). Ohne den holt Änderung 1 gemessene 30,6 % Fehltreffer ins
   Ergebnis.
3. **FTS-Epitheton als zweite Vorfilter-Route** — kauft die Tippfehler
   *innerhalb* der Gattung (2 % → 100 %), kostet p95 14 → 56–74 ms. Eigene
   Scheibe, unabhängig entscheidbar.

**Nicht** gelöst und ausdrücklich offen: die Gattungs-Synonymie aus
Fehlerklasse 3 (Befund 4). Die braucht eine Entscheidung über eine
Datenquelle, keinen Vorfilter.

## Einschränkungen dieser Messung

- Die 43 „plausiblen" Treffer aus Befund 5 sind eine **Handklassifikation**,
  nicht gegen eine nomenklatorische Autorität verifiziert. Die 19 falschen
  sind dagegen belastbar: WCVP führt diese Gattungen nicht, ein Treffer ist
  dort per Konstruktion falsch.
- Die synthetische Klasse „Gattungstausch, gleicher Anfangsbuchstabe" setzt
  eine **zufällige** andere Gattung ein und ist damit härter als reale
  Gattungs-Synonymie (wo die Namen meist verwandt sind). Ihre 0 % sind kein
  Urteil über reale Fälle.
- Alle Latenzen sind Einzelläufe auf einer Maschine. Nach der
  Messlehre aus PR #15 ist p95 hier nur als Größenordnung zu lesen — die
  Aussage „50× schneller" trägt das, eine Aussage über 14 vs. 20 ms nicht.
- Gemessen wurde ausschließlich mit `entry_backbone = wcvp`. Ohne den Filter
  ist der Pool größer (CDM trägt denselben Namen mehrfach), die
  Recall-Aussagen ändern sich dadurch nicht, die Latenzen schon.

## Reproduktion

```bash
hostus ingest --dataset build-full.yaml --db /tmp/hostus-fuzzy-index.db
cd poc && go build -o /tmp/fuzzyrecall ./measure/fuzzyrecall/
/tmp/fuzzyrecall -db /tmp/hostus-fuzzy-index.db -n 60 \
  -esy ../situs/pipelines/eunis/out/species_roles.csv
# Präzisions-Check von Hand:
/tmp/fuzzyrecall -db /tmp/hostus-fuzzy-index.db -n 1 -esy <...> \
  -only "prefix4+fts" -dump "prefix4+fts"
```

---

## Nachmessung nach der Umsetzung (2026-08-22)

Umgesetzt wurden alle drei empfohlenen Änderungen plus ein vierter Guard, den
erst diese Nachmessung sichtbar gemacht hat. Gemessen über `POST /v1/match`
gegen dieselben Daten, also inklusive Kandidatenleiter, Autor-Abtrennung und
Konzept-Eindeutigkeit — Dinge, die das Harness nicht modelliert.

| | vorher | nachher |
|---|---|---|
| ESy-Namen (distinkt) | 3.587 | 3.587 |
| davon aufgelöst | 91,7 % | **91,6 %** |
| davon per Fuzzy | **0** | **40** |
| Fehltreffer unter den Fuzzy-Treffern | — | **2 von 40** |

Die Auflösungsquote sinkt um 0,1 Punkte, weil die Guards fünf vorher *falsche*
Treffer entfernen — die Quote allein wäre hier die irreführende Zahl.

**Das Harness hat eine Fehlerklasse nicht vorhergesagt**, und das ist der
Grund, diese Nachmessung überhaupt zu machen: fünf ESy-Zeilen, die eine
**Sektion** benennen (`Taraxacum sect. Alpina`), lösten auf die **Art**
`Taraxacum sectum` auf (0,875). Sichtbar war das nur am Endpunkt, weil dort die
Autor-Abtrennung den großgeschriebenen Sektionsnamen als Autorschaft entfernt
— der Suchschlüssel ist `taraxacum sect.`, und der ist von `taraxacum sectum`
zwei Zeichen entfernt. Die Gattung ist identisch, der Gattungs-Guard also
blind. Dagegen der vierte Guard: **ein Rangkürzel auf nur einer Seite
verhindert die Auflösung.**

**Was von den 40 falsch bleibt, und warum:**

- `Buellia punctata` → `Ruellia punctata` (0,938): Flechtengattung, die sich in
  genau einem Buchstaben von einer Blütenpflanzengattung unterscheidet. Das ist
  das im Befund 6 vorhergesagte Restleck, gegen das kein Zeichenmaß hilft.
- `Juniperus communis subsp. communis` → `subsp. eucommunis` (0,944): andere
  Ursache. `Juniperus communis` liegt im Index **zweimal** (accepted +
  synonym), der Autonym-Schlüssel ist damit mehrdeutig, die Kandidatenleiter
  fällt auf Fuzzy durch, und Fuzzy landet auf einer anderen Unterart. Ein
  mehrdeutiger Exact-Treffer ist aussagekräftiger als ein Fuzzy-Treffer auf ein
  anderes Taxon — ob die Leiter das so behandeln soll, ist eine offene
  Entscheidung, kein Vorfilterproblem.

**Korrektur an Befund 3:** die `top`-Spalte des Harness überschätzt, was der
Endpunkt tatsächlich auflöst. Der Endpunkt verlangt zusätzlich ein
**eindeutiges Konzept**; wo derselbe Name mehrfach im Index liegt, kommt trotz
bestem Score `unresolvable` zurück (`Arctostaphylos alpinus` → zwei Konzepte
mit `Arctostaphylos alpina`, beide `synonym`; `Artemisia lercheana` → drei
Konzepte, eines davon `accepted`). Letzteres zeigt eine eigene Lücke: der
`role=accepted`-Vorrang aus Issue #67 Klasse 2 existiert nur auf dem
Exact-Pfad, nicht auf dem Fuzzy-Pfad.

**Latenzen am Endpunkt** (Einzelmessungen, als Größenordnung zu lesen):
0,003 s für einen Exact-Treffer, 0,016–0,18 s für einen Fuzzy-Treffer,
0,32–0,37 s im schlechtesten beobachteten Fall — gegenüber 0,65–1,2 s p95 für
den alten, ergebnislosen Vorfilter.

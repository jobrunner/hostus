# CDM `rl_standardliste`: die Zwei-Hop-Methode an einer geschichteten Stichprobe gemessen

**Datum:** 2026-08-02 · **Kontext:** SP5 (`POST /v1/translate`, UC6), Task 1 ·
**Vorarbeit:** `poc/P08-findings.md` · **Sonde:** `poc/p08b_cdm_sample/`

PoC P8 hat die Zwei-Hop-Methode an **einer Gattung** (*Abies*, 3 Geschwister­
konzepten) belegt. Diese Messung prüft, ob sie 51.466 Konzepte überlebt.
Alle Zahlen unten stammen aus einem realen, protokollierten Crawl gegen
`https://api.cybertaxonomy.org/rl_standardliste`; jede Zahl steht mit dem
Kommando, das sie erzeugt hat.

## Kurzfassung

| Messgröße | Ergebnis |
|---|---|
| Relationsdichte (Stichprobe) | **61,6 %** der Konzepte haben ≥ 1 Relation |
| Relationsdichte (auf den Datensatz hochgerechnet) | **≈ 55 %** (grob **50–63 %**, 95-%-Cluster-Bootstrap 48–63 %) |
| Zwei-Hop-Auflösung *mit* P8s Namensrestriktion (gemessen) | **75,9 % eindeutig**, 0 % mehrdeutig, 24,1 % ins Leere |
| Zwei-Hop-Auflösung *ohne* Namensrestriktion (gemessen, auf 782 gecrawlten Konzepten) | **79,1 % eindeutig, 0 % mehrdeutig** |
| Zwei-Hop-Auflösung bei Vollcrawl — **projiziert (Vollcrawl-Modell)** | **≈ 100 % eindeutig, 0 % mehrdeutig**, gestützt auf 202/202 anomaliefreie Kanten (siehe §2) |
| Relationstypen | 6 Werte, **3 davon nicht im SP1-Vokabular** |
| `sec.`-Auflösbarkeit | 100 % strukturierter `secSource`; Zuordnung zu den 18 Klassifikationen nur über eine **handgepflegte Crosswalk-Tabelle** |
| Crawl-Kosten Vollcrawl | **14–20 h** (nur Relationen) · **22–30 h** (Relationen + Richtung nur für die ≈ 55 % mit Relationen — der empfohlene Umfang) · 29–39 h (Richtung für alle) |

**Empfehlung: GO** für einen Vollcrawl in Task 2 — mit einer Korrektur an der
Methode (siehe [Go/No-Go](#gono-go-empfehlung-für-task-2)).

## Methode und Reproduzierbarkeit

### Crawl-Etikette

* Genau ein ehrlicher User-Agent:
  `hostus/2.0 (+https://github.com/jobrunner/hostus; jo.brunner@mayflower.de) taxonomic-concept-research`.
  Er wird von der API mit HTTP 200 beantwortet — P8s 403 kam von der WAF des
  Drupal-*Portals*, nicht von dieser API. Ein Browser-UA wurde **nicht**
  eingesetzt; die Sonde bricht bei 401/403 hart ab (`class Refused`), statt
  auszuweichen.
* ≤ 1 Request/Sekunde, single-threaded, Backoff bei 429/5xx.
* Alles unter `poc/p08b_cdm_sample/.cache/` (gitignoriert) zwischen­gespeichert:
  ein Wiederholungslauf kostet den Server nichts. Bulk-Daten werden nicht
  committet; committet ist nur `sample.tsv` (500 Zeilen).
* Verbrauchtes Budget dieser Messung: **909 Requests**, ca. 21 Minuten
  Netto-Verkehr.

### Ziehung der Stichprobe

Reproduzierbar über `probe.sh draw` (deterministisch, Seed `20260802`,
Kandidaten aus dem lokal aufgebauten Index der 51.466 Konzepte). Gezogen
werden **ganze Namensgruppen**, nicht Einzelkonzepte — sonst wäre der
Partner eines Konzepts systematisch außerhalb der Stichprobe und die
Zwei-Hop-Rate künstlich null.

Schichten (Schicht = Zahl unterschiedlicher `sec.`-Referenzen pro
kanonischem Namen; die Homonym-Schicht sticht die anderen):

```console
$ nix develop -c bash poc/p08b_cdm_sample/probe.sh draw
== draw ==
sample: 500 concepts in 159 name groups, 123 distinct genera
  A_many_sec      152 concepts
  B_mid_sec       156 concepts
  C_two_sec        76 concepts
  D_single_sec     50 concepts
  E_homonym        66 concepts
```

* **A** = Name in ≥ 8 `sec.`-Räumen (harter Fall: viele Partnerkandidaten),
* **B** = 3–7, **C** = 2, **D** = 1 (Kontrollgruppe),
* **E** = Homonyme im operativen Sinn: derselbe kanonische Name mit ≥ 2
  verschiedenen Autorenzeichenketten — genau der Fall, in dem „Partner über
  den Namen finden" schiefgehen kann. 66 Konzepte, also deutlich mehr als das
  vom Auftrag geforderte „mindestens ein bekanntes Homonym".
* Höchstens 3 Namensgruppen pro Gattung ⇒ **123 verschiedene Gattungen**.
  Die flache `/taxon`-Liste führt kein Familienfeld; die taxonomische Streuung
  wird deshalb über Gattungen erzwungen und nicht über Familien behauptet.

Die gezogene Liste liegt als `poc/p08b_cdm_sample/sample.tsv` im Repository.

### Ablauf

```bash
nix develop -c bash poc/p08b_cdm_sample/probe.sh all
# preflight -> index (52 Requests) -> draw (0) -> crawl (500)
# -> direct (308) -> probe (160) -> deepdive (128) -> crosscheck (9)
# -> analyze -> latency
```

## 1. Relationsdichte

```console
$ nix develop -c bash poc/p08b_cdm_sample/probe.sh analyze
== 1. relation density ==
concepts with >=1 relationship: 308/500 = 61.6%
relationships total: 532, mean per concept: 1.06, mean per related concept: 1.73
relations-per-concept histogram: 0:192, 1:260, 2:11, 3:4, 4:1, 5:3, 6:9, 7:11, 8:5, 9:1, 10:1, 14:1, 15:1
per stratum:
  A_many_sec      96/152 =  63.2%
  B_mid_sec      121/156 =  77.6%
  C_two_sec       49/ 76 =  64.5%
  D_single_sec     0/ 50 =   0.0%
  E_homonym       42/ 66 =  63.6%
```

Der Graph ist **nicht dünn**. Zwei Befunde sind wichtig:

* **Namen, die nur in *einem* `sec.`-Raum vorkommen, haben nie eine
  Konzeptrelation** (0/50). Das ist plausibel — es gibt schlicht kein
  Gegenüber — und heißt, dass rund ein Fünftel des Datensatzes für
  `/translate` strukturell leer bleibt.
* Die Stichprobe überrepräsentiert absichtlich Mehrfach-`sec.`-Namen. Mit den
  echten Schichtanteilen des Gesamtdatensatzes zurückgewichtet (Punktschätzung
  ≈ 56 %, belastbar nur als Größenordnung — siehe Konfidenzintervall unten):

```console
== 6. index-wide context ==
stratum shares in the full dataset vs. sampled density:
  A_many_sec       8312 concepts ( 16.2% of dataset), sampled density  63.2%
  B_mid_sec       20883 concepts ( 40.6% of dataset), sampled density  77.6%
  C_two_sec        2348 concepts (  4.6% of dataset), sampled density  64.5%
  D_single_sec    10691 concepts ( 20.8% of dataset), sampled density   0.0%
  E_homonym        9232 concepts ( 17.9% of dataset), sampled density  63.6%
  re-weighted dataset-wide relation density estimate: 56.0% (~28836 of 51466 concepts)
  stratified cluster bootstrap (5000 resamples over name groups, seed 20260802): 95% CI 48% - 63%
```

Die Punktschätzung 56,0 % ist **scheingenau**: die 500 Konzepte sind nur 159
unabhängige Cluster (Namensgruppen), und ob ein Konzept Relationen hat, ist
innerhalb einer Gruppe fast konstant. Ein Cluster-Bootstrap über Namensgruppen
(stratifiziert, 5000 Resamples) liefert ein 95-%-Intervall von **48–63 %**.
Im Text wird deshalb durchgängig **„≈ 55 % (grob 50–63 %)"** verwendet, nicht
„56,0 %".

## 2. Zwei-Hop-Auflösungsrate

P8s Methode wörtlich nachgebaut: Partnerkandidaten sind die anderen Konzepte
*mit demselben Namen*, ein Treffer ist eine übereinstimmende Relations-UUID.

```console
== 2. two-hop resolution ==
  exactly one partner          404 / 532 =  75.9%
  ambiguous (>1 candidate)       0 / 532 =   0.0%
  dangling (0 candidates)      128 / 532 =  24.1%
  MEASURED over the whole crawled set (name restriction dropped):
    exactly one 421, ambiguous 0, dangling 111  (= 79.1% resolved)
```

**Gemessen sind zwei Zahlen: 75,9 % mit Namensrestriktion, 79,1 % ohne.**
Alles darüber ist Projektion und wird hier auch so bezeichnet.

**Null Mehrdeutigkeit** — auch in der Homonym-Schicht (`ambiguous cases:`
bleibt leer). Der strukturelle Grund ist wichtiger als die Prozentzahl:

```console
  holders per relationship uuid over 782 crawled concepts: 1:346, 2:256
  two-holder uuids with a direction lookup on BOTH ends: 202 (of 256; 54 not both looked up)
    exactly one `from` + one `to`: 202, anomalous: 0
  one-holder uuids: 53 `from`, 58 `to`, 235 unchecked -- a mix is the signature of an UNCRAWLED partner, not of a one-ended edge
  => PROJECTION (not a measurement): a full crawl of all 51.466 concepts resolves ~100% with 0% ambiguity, provided no uuid ever acquires a third holder. Task 2 must abort the import if one does, and report the residual one-holder count.
```

Drei voneinander unabhängige Belege dafür, dass eine Relations-UUID eine
**binäre Kantenidentität** ist und kein Typ- oder Gruppenschlüssel:

1. Von 602 gefundenen Relations-UUIDs trägt **keine mehr als zwei** Konzepte
   (Histogramm `{1: 346, 2: 256}`) — kein Gegenbeispiel in 782 Konzepten.
2. Von den 256 Zwei-Halter-UUIDs haben **202 auf beiden Seiten** eine
   Richtungsabfrage. **Alle 202 zerfallen in genau ein `from` und genau ein
   `to`; null Anomalien** — keine UUID mit zwei `to`- oder zwei `from`-Enden.
   Das ist positive Bestätigung, nicht nur „kein Gegenbeispiel gesehen".
3. Die Ein-Halter-UUIDs teilen sich in **53 `from` / 58 `to`** — genau die
   Signatur eines *nicht mitgecrawlten Partners*. Wären es echte einseitige
   Kanten, müsste die Richtung systematisch auf einer Seite liegen.

Daraus folgt die **Projektion (Vollcrawl-Modell)**: wer alle 51.466 Konzepte
crawlt, löst annähernd 100 % der Relationen eindeutig auf. Die 24,1 % „ins
Leere" sind Folge von P8s Namensrestriktion bzw. der begrenzten Stichprobe,
kein Datenmangel. Die Prämisse ist auf den 43 % der UUIDs geprüft, bei denen
beide Enden zufällig im Crawl liegen — sie ist gut gestützt, aber nicht
bewiesen.

**Falsifikator für Task 2** (verbindlich in den Vollcrawl aufzunehmen):

* Der Import **bricht ab**, sobald irgendeine Relations-UUID einen **dritten**
  Halter bekommt. Dann ist die UUID keine Kantenidentität und das gesamte
  Auflösungsmodell muss neu gedacht werden.
* Der Import **meldet am Ende die verbleibende Zahl der Ein-Halter-UUIDs**.
  Bei einem Vollcrawl muss sie gegen null gehen; tut sie das nicht, gibt es
  Relationen zu Konzepten außerhalb des `/taxon`-Listings, und die
  Vollständigkeit von `/translate` ist entsprechend zu deckeln.

## 3. Warum die Fehlschläge fehlschlagen

Aufschlüsselung der 128 offenen Relationen nach Typ:

| Typ | gesamt | offen | Anteil offen |
|---|---:|---:|---:|
| Congruent to | 472 | 80 | 16,9 % |
| Includes | 38 | 34 | 89,5 % |
| Overlaps | 9 | 7 | 77,8 % |
| Included in or Includes or Overlaps | 9 | 3 | 33,3 % |
| is pro parte synonym for | 2 | 2 | 100 % |
| is misapplied name for | 2 | 2 | 100 % |

74 der 128 offenen Fälle hängen an Konzepten `sec. Wisskirchen & Haeupler
1998` — der Standardliste, die als Nabe des Netzes fungiert.

Für die Handanalyse wurden **drei kleine Gattungen vollständig** gecrawlt
(alle Konzepte des Datensatzes in *Coronilla*, *Dorycnium*, *Persicaria*,
128 Konzepte). Damit wird aus „Partner nicht gefunden" ein Urteil:

```console
$ nix develop -c bash poc/p08b_cdm_sample/probe.sh deepdive
deep-dive over 128 concepts in ['Coronilla', 'Dorycnium', 'Persicaria']

dangling relationships of sampled concepts in these genera, resolved against the exhaustive genus crawl:
  STILL MISSING  Persicaria maculosa Gray           Congruent to           (sec. Wisskirchen & Haeupler 1998)
  … (6x)
  FOUND  Persicaria maculosa Gray           Congruent to           -> Persicaria mitis Delarbre          R. Wisskirchen & H. Haeupler 1998
  STILL MISSING  Coronilla varia L.                 Congruent to           (sec. TUTIN et al.: Flora Europaea)
  … (6x, je ein sec.-Raum)
  FOUND  Dorycnium pentaphyllum subsp. germ Congruent to           -> Dorycnium germanicum (Gremli) Rikl Wisskirchen & Haeupler 1998
  … (3x)

  partner inside the genus: 4, partner outside the genus: 12
```

Die verbleibenden 12 wurden gezielt gegengeprüft (9 zusätzliche Konzepte):

```console
$ nix develop -c bash poc/p08b_cdm_sample/probe.sh crosscheck
crosscheck over 9 concepts: ['Securigera varia', 'Polygonum persicaria']
  RESOLVED ACROSS GENERA  Persicaria maculosa Gray  (sec. Wisskirchen & Haeupler 1998) --Congruent to--> Polygonum persicaria L. (sec. HEGI …)
  … 6x für Persicaria/Polygonum, 6x für Coronilla/Securigera …
  previously dangling relationships now resolved: 12
```

**Alle 16 handanalysierten Fälle sind damit aufgeklärt, keiner blieb offen.**
Kategorien:

| # | Kategorie | Beispiel |
|---:|---|---|
| 12 | **Gattungswechsel** — Partner steht unter einem anderen Gattungsnamen | *Coronilla varia* sec. Flora Europaea ≜ *Securigera varia* sec. W&H; *Persicaria maculosa* sec. W&H ≜ *Polygonum persicaria* sec. HEGI |
| 3 | **Rangwechsel / anderes Epitheton in derselben Gattung** | *Dorycnium pentaphyllum* subsp. *germanicum* ≜ *Dorycnium germanicum* |
| 1 | **anderer Artname in derselben Gattung** | *Persicaria maculosa* ≜ *Persicaria mitis* |

**Reichweite dieser Handanalyse:** Die 16 Fälle stammen aus drei Gattungen,
die ausgewählt wurden, *weil sie klein genug für einen vollständigen Crawl
waren* — sie sind **keine Zufallsstichprobe** aus den 128 offenen Fällen. Für
die übrigen 112 ist die Erklärung deshalb **Inferenz, keine Messung**: die
Typverteilung legt sie nahe (`Includes`, `Overlaps` und `pro parte` verbinden
per Definition Konzepte *unterschiedlichen Umfangs*, die typischerweise auch
unterschiedlich heißen — daher deren Ausfallquote von 78–100 % unter einer
Namensrestriktion), belegt ist sie nicht. Der Vollcrawl in Task 2 entscheidet
das ohnehin abschließend.

**Fazit dieses Abschnitts:** Der einzige *gemessene* Fehlermodus ist P8s
Heuristik „Partner teilt den Namen", nicht die Datenqualität. Genau diese
Heuristik ist bei `Includes`/`Overlaps` — den taxonomisch *interessantesten*
Relationen für UC6 — fast immer falsch.

## 4. Verteilung der Relationstypen

```console
== 4. relation-type distribution ==
  conceptRel | representation_L10n | symbol | symmetric | count
  True       | Congruent to                 | ≜                | True      | 472
  True       | Includes                     | ⊃                | False     | 38
  True       | Overlaps                     | ⊕                | True      | 9
  True       | Included in or Includes or Overlaps | ⊂⊃⊕       | True      | 9
  True       | is pro parte synonym for     | p.p. for         | False     | 2
  False      | is misapplied name for       | misapplied for   | False     | 2
  values NOT in the SP1 vocabulary (congruent|includes|included_in|overlaps|disjoint):
  ['Included in or Includes or Overlaps', 'is misapplied name for', 'is pro parte synonym for']
  re-weighted by stratum (dataset-wide share of relations):
    Congruent to                          85.5%
    Includes                               8.8%
    Included in or Includes or Overlaps    2.5%
    Overlaps                               2.5%
    is misapplied name for                 0.6%
    is pro parte synonym for               0.2%
```

Die Rohzahlen sind **ungewichtete Stichprobenzahlen**; die Stichprobe
überrepräsentiert Mehrfach-`sec.`-Namen. Nach derselben Schichtgewichtung wie
bei der Dichte ergeben sich datensatzweit **≈ 85 % `Congruent to`** und
**≈ 14 %** informativere Relationen (`Includes` 8,8 %, `⊂⊃⊕` 2,5 %,
`Overlaps` 2,5 %).

Für das SP1-Schema, das `congruent|includes|included_in|overlaps|disjoint`
annimmt, heißt das konkret:

* **`Included in or Includes or Overlaps` (`⊂⊃⊕`)** — ein *disjunktiver*
  Unschärfetyp („irgendeine Überlappung, welche genau ist unbekannt"). Er
  passt auf **keinen** der fünf Schemawerte und darf nicht auf `overlaps`
  gemappt werden; das würde eine Präzision behaupten, die die Quelle
  ausdrücklich verweigert. Braucht einen eigenen Wert.
* **`is pro parte synonym for`** — Berendsohn-nah, aber kein Mengenrelations­
  typ. Eigener Wert oder bewusster Ausschluss.
* **`is misapplied name for`** — das einzige `conceptRelationship: false`;
  gehört semantisch nicht in dieselbe Relationstabelle.
* **`included_in` kommt als eigener Typ überhaupt nicht vor.** Die Quelle
  modelliert nur `Includes` und dreht die Richtung um (`symmetric: false`,
  `inverseRepresentation_L10n`). Ohne Richtung ist `Includes` unbrauchbar.
* **`disjoint`/`Excludes` kommt in der Stichprobe nicht vor** (0 Treffer).

Das ist derselbe Fehlermodus wie bei `domain.ParseRank` (6 bekannte vs. 34
reale Ränge): **das angenommene Vokabular ist unvollständig und würde beim
Vollimport abbrechen oder still falsch mappen.**

### Richtung

Die Richtung ist **nicht** in `/portal/taxon/{uuid}/taxonRelationships`
enthalten, wohl aber über einen zweiten Endpunkt ableitbar
(`/taxon/{uuid}/relationsToThisTaxon` ⇒ das Konzept ist Ziel der Kante):

```console
== 4b. direction (relationsToThisTaxon) ==
  classified 532 relationship ends (0 without a direction lookup)
  Congruent to                 from  232 / to  240
  Includes                     from   25 / to   13
  Overlaps                     from    3 / to    6
  Included in or Includes or Overlaps  from 3 / to 6
  is misapplied name for       from    1 / to    1
  is pro parte synonym for     to      2
```

Das verdoppelt die Request-Kosten eines Vollcrawls, ist für die asymmetrischen
Typen aber unverzichtbar.

## 5. `sec.`-Auflösbarkeit

```console
== 5. sec. resolvability ==
sampled concepts without a structured secSource citation: 0/500
classifications in the dataset: 18
  crosswalk assertion: 17 entries, all targets are real classification uuids; 17/18 classifications targeted, 1 explicitly unmapped
sample concepts whose sec. title is EXACTLY a classification titleCache: 4/500
index-wide exact title matches: 448/51466
distinct sec. references in the sample: 19 (whole dataset: 120)
sample concepts whose sec. maps onto a classification via the hand crosswalk: 494/500 = 98.8%
index-wide: 50899/51466 = 98.9%
```

* **Jedes** Konzept trägt ein strukturiertes `secSource` mit
  `citation.uuid` + `titleCache` — und zwar bereits **in der flachen
  `/taxon`-Liste**, ohne Einzelabruf. Das war in P8 nicht bekannt und spart
  im Vollcrawl 51.466 Requests.
* **Aber:** Es gibt **keine maschinelle Verbindung zwischen `secSource` und
  den 18 Klassifikationen.** Die `Classification`-Objekte führen nur einen
  `titleCache` und *keine* Referenz-UUID (`/classification/{uuid}` → 404,
  `/portal/taxon/{uuid}`-`taxonNodes` ohne Klassifikations-UUID), und die
  Titel sind anders geschrieben als die der Referenzen — exakte
  Titelgleichheit trifft nur 448 von 51.466 Konzepten (0,9 %).
* Mit einer **handgepflegten Crosswalk-Tabelle von 17 Einträgen**
  (`CROSSWALK` in `cdm_sample.py`, z. B. `Schubert & Vent 1990` →
  `ROTHMALER, Exkursionsflora …`) sind **98,9 % des Datensatzes** einem der
  18 `sec.`-Räume zugeordnet. Der Rest sind Einzelfallreferenzen
  (`Excel Taxon import`, `Sell & West in Fl. Europ.`, …) aus einem Schwanz von
  120 verschiedenen Referenzen.
* Die Tabelle ist auf **Klassifikations-UUIDs** abgebildet, nicht auf einen
  zweiten Anzeigetext, und `assert_crosswalk()` prüft bei jedem Lauf, dass
  **jedes Ziel eine der 18 echten UUIDs ist** und dass jede Klassifikation
  entweder getroffen oder ausdrücklich als unabgebildet deklariert ist. Eine
  frühere Fassung bildete auf Titel ab und enthielt zwei gekürzte Zeichen­
  ketten (`"… Standardliste ... 1998"`), die auf gar nichts passten —
  ausgerechnet bei WISSKIRCHEN & HAEUPLER, der Nabe des Datensatzes. Die
  Deckungszahl war davon nicht betroffen (sie prüfte nur die Schlüsselseite),
  der Fehler wäre aber mit der Tabelle nach Task 2 gewandert. **Die
  Abbildungsseite ist jetzt zugesichert.**
* Genau **eine** der 18 Klassifikationen — `Andere Referenzen (fuer Synonyme
  p. p.)` — wird von keiner einzigen `secSource`-Referenz im Datensatz
  angesteuert und ist deshalb bewusst unabgebildet (`CLS_UNMAPPED`).
* Bewertung: unkritisch, aber **Kuration, keine Daten** — 17 Zeilen, die
  versioniert und zugesichert gehören.

## 6. Crawl-Kosten

```console
$ nix develop -c bash poc/p08b_cdm_sample/probe.sh latency
n=909 mean=0.353s p50=0.159s p95=2.992s max=4.249s
observed cost per request (max(1s, latency)): 1.139s
  1 req/s floor            51466 concepts                 14.3 h
  1 req/s floor            51466 x2 (rel + direction)     28.6 h
  observed max(1s,latency) 51466 concepts                 16.3 h
  observed max(1s,latency) 51466 x2 (rel + direction)     32.6 h
  pessimistic 1s + mean lat 51466 concepts                 19.3 h
  pessimistic 1s + mean lat 51466 x2 (rel + direction)     38.7 h
  worst case 1s + p95 lat  51466 concepts                 57.1 h
  worst case 1s + p95 lat  51466 x2 (rel + direction)    114.1 h
```

Der Limiter misst das 1-Sekunden-Intervall ab Request-*Start*; die reale
Kosten je Request sind also `max(1 s, Latenz)`. Die beobachteten Teilläufe
bestätigen das (500 Konzepte in 8:21 min, 308 in 5:09 min).

**Projektion Vollcrawl, als Spanne:**

| Umfang | Requests | Wall-Clock |
|---|---:|---|
| Nur Relationen | 51.518 | **14–20 h** |
| Relationen + Richtung (nur für Konzepte mit Relationen, ≈ 55 %) — **empfohlener Umfang** | ≈ 80.000 (± 4.000 je nach echter Dichte) | **22–30 h** |
| Relationen + Richtung für alle (nicht nötig) | 103.000 | 29–39 h |

Die Zeilen sind unterschiedlich weit gefasst; die Kurzfassung oben nennt
dieselben drei Umfänge. Die Obergrenze „1 s + p95" (57 h / 114 h) ist
unrealistisch pessimistisch —
p95 = 3 s tritt in Schüben auf und geht im 1-s-Fenster größtenteils unter.
Realistisch ist **ein Wochenende**, resumierbar, mit Cache.

## Go/No-Go-Empfehlung für Task 2

**GO — Vollcrawl aller 51.466 Konzepte, mit einer Methodenkorrektur.**

Begründung:

1. **Die Zwei-Hop-Methode skaliert — aber nicht in P8s Formulierung.** P8s
   Kandidatensuche „andere Konzepte mit demselben Namen" erreicht 75,9 %
   und verfehlt systematisch genau die Relationen, die UC6 am meisten
   interessieren (`Includes`/`Overlaps`: 78–100 % Ausfall). Die Korrektur ist
   billig und beseitigt das Problem aller Voraussicht nach vollständig: **die
   Namensrestriktion ersatzlos streichen** und stattdessen im Vollcrawl eine globale Map
   `relationshipUuid → {Konzept A, Konzept B}` aufbauen. Gemessen trägt keine
   Relations-UUID mehr als zwei Endpunkte (602 UUIDs, Histogramm `{1: 346,
   2: 256}`), und alle 202 beidseitig geprüften Kanten haben genau ein `from`-
   und ein `to`-Ende (null Anomalien). Die UUID ist damit sehr gut gestützt
   eine echte Kantenidentität ⇒ **projizierte Auflösung ≈ 100 %,
   Mehrdeutigkeit 0 %**, ohne Namensheuristik und ohne Homonymrisiko. Ohne
   Namensrestriktion *gemessen* sind 79,1 %. Der Vollcrawl muss den in §2
   formulierten Falsifikator mitführen (Abbruch bei einem dritten Halter,
   Meldung der Rest-Ein-Halter).
2. **Der Graph trägt.** ≈ 55 % der Konzepte (grob 50–63 %) haben mindestens
   eine Relation;
   das Netz ist um Wisskirchen & Haeupler 1998 als Nabe organisiert, also
   genau um den `sec.`-Raum, den UC6 als Referenz nimmt.
3. **Die Kosten sind vertretbar:** 14–20 h für die Relationen allein;
   **22–30 h im empfohlenen Umfang** (Relationen für alle 51.466 Konzepte plus
   Richtungsabfrage nur für die ≈ 55 %, die überhaupt Relationen haben);
   29–39 h, wenn die Richtung unnötigerweise für alle geholt wird. Einmalig,
   resumierbar, gegen einen statischen Datensatz (`created` 2013,
   `updated` 2021).
4. **Keine Teilmenge von Klassifikationen empfehlen.** Ein Crawl auf z. B. nur
   W&H + Rothmaler spart wenig (die Nabe hängt an allen 18 Räumen) und würde
   die globale Kanten-Map wieder unvollständig machen — also genau den Fehler
   reproduzieren, den diese Messung gerade aufgedeckt hat. Entweder ganz oder
   gar nicht.

**Kein Rescope von SP5 aus technischen Gründen.** Zwei Auflagen bleiben aber
bestehen:

* **Blocker (unverändert aus P8): die Lizenz.** Es gibt weiterhin keine
  Lizenzangabe. Der Vollcrawl darf laufen (Forschungszweck, 1 req/s), aber die
  **Redistribution des abgeleiteten Relationsgraphen über `/v1/translate`
  bleibt bis zur schriftlichen Klärung mit BGBM/EDIT gesperrt.** Das ist die
  eigentliche Go/No-Go-Frage für SP5, und sie ist keine technische.
* **Schemakorrektur vor dem Import.** Das SP1-Vokabular
  `congruent|includes|included_in|overlaps|disjoint` ist gemessen
  unvollständig. Task 2 muss aufnehmen: `⊂⊃⊕` als eigenständigen
  Unschärfetyp, `pro parte`, die Trennung
  `conceptRelationship: true|false` (Misapplied Names gehören nicht in die
  Konzeptrelationstabelle) und eine **Richtungsspalte** — `included_in` ist in
  der Quelle kein Typ, sondern die Umkehrung von `Includes`. Ein Importer, der
  auf einen unbekannten Typ trifft, muss laut abbrechen und nicht raten.

### Was das UC6 kostet

Realistisch bekommt `/translate` Antworten für **rund 55 % der Konzepte (grob
50–63 %)**; der Rest sind überwiegend Namen, die nur in einem `sec.`-Raum
existieren, also strukturell nichts zu übersetzen haben. Von den gelieferten
Kanten sind — schichtgewichtet — rund **85 % `Congruent to`**, die glatte,
unspannende Antwort, und nur ≈ **14 %** tragen die taxonomisch interessante
Information (`Includes`, `Overlaps`, `⊂⊃⊕`). Ein `/translate`, das nur Kongruenzen liefern könnte, wäre kaum mehr
als ein Synonymlookup; die Korrektur aus Punkt 1 ist deshalb nicht kosmetisch,
sondern das, was UC6 überhaupt erst rechtfertigt.

## Nebenbefunde

* **Es gibt keinen Endpunkt, der den Partner direkt liefert.** Geprüft und
  jeweils 404 bzw. ohne Partner: `/taxonRelationship/{uuid}`,
  `/portal/taxonRelationship/{uuid}`, `/taxon/{uuid}/taxonRelationships`,
  `/portal/taxon/{uuid}` (Felder `relationsToThisTaxon` /
  `relationsFromThisTaxon` enthalten nur Typ + UUID). Die Swagger-Beschreibung
  der Installation ist defekt (`/v2/api-docs` → 400, `/swagger-resources` →
  500), eine vollständige Endpunktliste ließ sich nicht abrufen.
* Die flache `/taxon`-Liste liefert `secSource` **mit**, ohne Einzelabruf —
  52 Requests statt 51.466 für die `sec.`-Zuordnung.
* Der Datensatz enthält 120 verschiedene `sec.`-Referenzen bei nur 18
  Klassifikationen; der Schwanz sind Einzelautorenzitate für Synonyme und
  Misapplied Names.

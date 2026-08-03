# Changelog

Alle wesentlichen Änderungen an diesem Projekt werden in dieser Datei dokumentiert.

Das Format basiert auf [Keep a Changelog](https://keepachangelog.com/de/1.0.0/),
und dieses Projekt folgt [Semantic Versioning](https://semver.org/lang/de/).

## [Unreleased]

### Added (SP6, Task 4 — Offenlegung, End-to-End-Beweis und Verdikt)
- **Neue Anleitung
  [„Publikationsfähige Synonyme filtern (UC5)"](docs/how-to/synonyms-uc5.md)**
  mit dem durchgerechneten *Corynephorus-canescens*-Beispiel und einem
  ausdrücklichen Abschnitt **„Was dieser Filter nicht kann"**. UC5 nennt
  fünf Relevanzkriterien; SP6 liefert zwei vollständig, eines teilweise
  und **zwei überhaupt nicht**. Das steht jetzt in der Anleitung des
  Endpunkts selbst und nicht in einer Forschungsdatei: Wer
  `relevance=publication` liest und regionale Filterung annimmt,
  veröffentlicht die falsche Synonymliste.
  - **Keine regionale Filterung** („im Bezugsraum verwendet"):
    `distribution` hängt am *Konzept*, ein Synonym ist ein *Name* — mit
    dem aktuellen Schema nicht ausdrückbar. Die Anleitung nennt, was es
    bräuchte (eine Name-×-Gebiet-Relation mit Quelle je Zeile).
  - **Keine Filterung nach Standardwerken**: ROTHMALER, OBERDORFER, HEGI
    und SCHMEIL-FITSCHEN sind vier der 18 CDM-Klassifikationen aus SP5,
    aber `redistribution: unknown` und die Ernte ist unfertig.
  - **Die Typisierung ist ein Zwei-Wege-Split**: `homotypic = 1` auf
    271.821 Synonymzeilen, NULL (= *unbekannt*) auf 692.941,
    `heterotypic` auf **0**.
  - **Ein fehlender `nom_status` ist kein Unbedenklichkeitsnachweis** —
    die Spalte ist auf 99.252 von 1.448.984 Namen (6,85 %) belegt.
  - Zwei benannte Ranglücken: **SUBSPECIES wird spec-treu nicht
    ausgeschlossen** (45.526 Synonymnamen) und 190 Nothotaxon-Zeilen
    passieren `rank=species`.
  - Offener **fachlicher** Punkt, gemessen: fünf Werte halten **1.697
    Namen** zurück, davon 1.117 mit `, sensu auct.`. Sollen
    Fehlanwendungen als *auct. non* geführt werden, ist das eine Zeile in
    `nomStatusGuards`, keine Codeänderung.
- **End-to-End-Beweis über echtes HTTP** in
  `internal/app/integration_test.go`: `ingest -> serve -> GET
  /v1/concept/{id}/synonyms`, gefiltert gegen ungefiltert, mit
  Namensprüfungen statt bloßer Zählungen. *Festuca ovina* verliert genau
  *Avena dura* (`", nom. illeg. superfl."`) und *Festuca ovina* var.
  *vulgaris* (`", not validly publ."`) und behält *Avena ovina*, *Bromus
  ovinus* und *Festuca duriuscula*; bei *Corynephorus canescens* fallen
  drei Rangausschlüsse weg, während subsp. *maritimus* bleibt — die
  SUBSPECIES-Lücke ist damit als Test festgeschrieben, nicht nur
  dokumentiert. Das Ausschluss-Summary muss die Differenz erklären.
- **SP6-Verdikt in `docs/research/reality-check.md`**, gemessen mit dem
  echten Domänencode über alle 964.762 Synonymzeilen: Der Filter ändert
  bei **103.674 von 236.030** Konzepten die Antwort und **24.918 Konzepte
  (10,6 %)** bleiben ohne ein einziges publikationsfähiges Synonym.
  Verdikt: **hält mit Auflagen**.
- **Der Zielkorridor ist mit Kontrollgruppe gemessen — und taugt nicht als
  Beleg für den Filter.** Ungefiltert liegen bereits **70,92 %** der
  Konzepte bei ein bis drei Synonymen, gefiltert **69,20 %**: Auf UC5s
  eigener Zielgröße ist der Filter **netto negativ**. Er zieht 20.854
  Konzepte aus `> 3` heraus (68.642 → 47.788) und erzeugt dabei 24.918
  Nullen; der Modalwert bleibt **1** (40,40 % → 41,38 %). Sein Nutzen ist
  die Entfernung nomenklatorisch untauglicher Namen (89.836 Zeilen mit
  belegtem Defekt), nicht das Treffen des Korridors. Die frühere Fassung
  führte die 69,2 % ohne Vergleichsgröße als Beleg *für* den Filter — eine
  Zahl ohne Kontrollgruppe ist kein Argument.
- **Die Urteilsverteilung über alle 964.762 Synonymzeilen** steht jetzt in
  Anleitung und Reality-Check: `absent` 872.270 / `disqualifying` 89.836 /
  `unclassified` 2.309 / **`acceptable` 347**. Für ganze **347 Zeilen** im
  Korpus behauptet die Quelle, der Name sei nomenklatorisch in Ordnung —
  jede publikationsfähige Liste besteht zu über 99,9 % aus Namen, über die
  niemand etwas gesagt hat.
- **Die vollständige 36-Zeilen-`nom_status`-Regeltabelle** steht jetzt in
  `docs/reference/http-api.md` — bisher versprach die Anleitung sie dort
  zweimal, ohne dass es sie gab. Sie ist gegen `domain.NomStatusRules()`
  festgenagelt (`internal/domain/nomstatus_doc_test.go`, Token/Urteil/
  Trefferzahl zeilenweise), womit auch die bis dahin unbelegte Begründung
  des exportierten Accessors („für den Doku-Generator", den es nicht gibt)
  eingelöst ist.
- **Doppelt ausgeschlossene Synonyme** sind jetzt im clientseitigen Vertrag
  erklärt, nicht nur im Reality-Check: Präzedenz bucht sie unter
  `nom_status`, weshalb `excluded.rank` die Zahl der *nur* rangbedingt
  zurückgehaltenen Synonyme ist — im Beispiel `rank: 16` bei 17
  infraspezifischen Synonymen, korpusweit 14.202 Zeilen.
- **`docs/explanation/known-gaps.md`** (neu, in der Navigation): fünf
  bewusst nicht behobene Befunde mit Auswirkung und nächstem Schritt —
  kein Drift-Signal für das `nom_status`-Vokabular, fehlender
  Mutanten-Mindestboden im Gate, die überholte 108,9-MB-Bundle-Angabe, der
  vierfach kopierte `TaxonRow`-Mapper und 316 Zeilen handgepflegtes
  OpenAPI ohne Drift-Prüfung. Sie standen bisher nur im SDD-Ledger, das
  das nächste Teilprojekt nicht liest.

### Fixed (SP6, Task 4 — Review-Nachlauf)
- **Das neue `Not covered`-Gate lief in der CI auf keinem einzigen
  SP6-Paket.** `.github/workflows/mutation.yml` fuhr `config`, `httperr`,
  `adapters/telemetry`, `adapters/http`, `app`, `adapters/mcp` und
  `cmd/hostus` — die gesamte SP6-Logik liegt aber in `internal/domain`,
  `internal/application` und `internal/adapters/sqlite`, und `make verify`
  ruft `mutation` gar nicht auf. Das Gate war also manuell nachgewiesen und
  nirgends verdrahtet: Ein Rückfall bei den hochgezogenen Bedingungen in
  `internal/domain/synonym.go` wäre unbemerkt geblieben. Die drei Pakete
  laufen jetzt mit (gemessene Laufzeit lokal: 30 s / 62 s / 154 s, rund
  vier Minuten zusätzlich bei 60 Minuten Job-Timeout).
- **`internal/domain/synonym.go`, Regel `type`:** Die Notiz behauptete,
  alle 1.099 Zellen mit `type` seien Mangelaussagen. `", type variety."`
  (1 Name, *Helichrysum bracteatum* var. *chrysanthum*) ist eine
  taxonomische Anmerkung. Notiz auf 1.098 von 1.099 korrigiert, der
  Einzelfall in Code und Referenz benannt; der Name wird weiterhin
  zurückgehalten (die sichere Richtung), die saubere Auflösung wäre ein
  Guard.

### Changed (SP6, Task 4 — Mutations-Gate)
- **`make mutation` erzwingt jetzt `Not covered: 0`.** Ein überlebender
  Mutant heißt „ein Test läuft durch die Zeile, prüft aber zu lasch" und
  ist begründbar; ein NOT-COVERED-Mutant heißt „kein Test führt diesen
  Code aus" und ist es nie. Der harte Fehlschlag hängt deshalb genau an
  `Not covered` und nicht an einer Efficacy-Schwelle, die gegen die
  bestehenden dokumentiert-gerechtfertigten Überlebenden spröde wäre.
  Fehlt die Zeile `Not covered: N` im Report, bricht das Target ebenfalls
  ab — ein Gate ohne Signal darf nicht grün melden. Genau eine Ausnahme:
  `No results to report.` bei gesetztem `PKG` (ein Paket ohne mutierbare
  Stelle, z. B. `./internal/httperr`) ist eine gültige Antwort; ohne `PKG`
  ist es die bekannte gremlins-Grenze bei `./...` und damit ein Fehlschlag.
- **Zwei Altlasten, die das neue Gate aufgedeckt hat, sind behoben** —
  beide waren „kein Test führt diesen Code aus", keiner davon neu:
  - `internal/app/bundle.go`: `app.Bundle` war nur über `cmd/hostus`
    getestet, also aus Sicht von `make mutation PKG=./internal/app` gar
    nicht. Neues `internal/app/bundle_test.go` deckt Erfolgs- und
    Fehlerpfad im eigenen Paket ab (`Not covered: 1 -> 0`).
  - `internal/adapters/telemetry/telemetry.go`: der `otlpEnabled`-Zweig von
    `Setup` wurde von keinem Test betreten. Neuer Test mit aktiviertem OTLP
    gegen einen toten Endpunkt (hermetisch — die Exporter bauen ihren
    Client lazy und wählen nichts) (`Not covered: 2 -> 0`).
- **`internal/domain/synonym.go`: zwei Bedingungspaare aus den `case`-Armen
  hochgezogen.** Gos Coverage-Modell beendet den gezählten Block am `{`
  eines tag-losen `switch`; eine Bedingung *im* `case`-Arm liegt damit in
  keinem gezählten Block, und `make mutation` meldete sechs Mutanten in
  `ClassifyNomStatus`/`judgeSynonym` dauerhaft als NOT COVERED — kein Test
  hätte das beheben können. Als eigene Zuweisungen (`anyDisqualifying`,
  `anyAcceptable`, `disqualifying`, `unclassified`) werden sie mutiert und
  von der bestehenden Testsuite **gekillt**: `./internal/domain` steht
  jetzt bei `Killed: 123, Lived: 8, Not covered: 0`.

### Added (SP6, Task 3 — `GET /v1/concept/{id}/synonyms`)
- **Neuer Endpunkt `GET /v1/concept/{id}/synonyms?relevance=&rank=&max=`**
  (UC5): „Das Problem ist Filterung, nicht Beschaffung." Er wendet das
  Relevanzmodell aus Task 2 auf die Synonyme eines Concepts an. Am realen
  Index liefert *Corynephorus canescens* (`wcvp:concept:405825`) mit
  `relevance=publication&rank=species&max=3` genau die drei erwarteten
  Namen — *Aira canescens* L. führt als Basionym, *Corynephorus
  incanescens* Bubani (`", nom. illeg. superfl."`) ist ausgeschlossen.
- **Standard ist die UNGEFILTERTE Liste** (`relevance=all`). Der Filter
  hält bei *Corynephorus canescens* 20 von 26 Synonymen zurück und muss
  deshalb ausdrücklich angefordert werden; `/v1/concept/{id}` liefert
  dieselben Synonyme ungefiltert, und zwei Endpunkte mit
  unterschiedlicher Zeilenzahl auf dieselbe Frage lesen sich als Fehler.
  Beide Modi tragen dieselbe Begründung pro Synonym und dieselbe Bilanz.
- **Jeder Ausschluss ist sichtbar.** `summary` beschreibt immer das
  Concept, nie die ausgelieferte Seite: `total`, `publishable`, `absent`,
  `excluded` (Anzahl je Regel) und die unklassifizierten Rohwerte. Ein
  Filter, der 20 von 26 Synonymen entfernt, ohne das zu sagen, ist von
  einer kaputten Abfrage nicht zu unterscheiden.
- **`max` kappt IMMER nach dem Ranking**, nie davor — `max=3` liefert die
  drei besten Synonyme, nie drei beliebige. Bereich `[0, 2000]`; `0` und
  ein fehlender Parameter bedeuten „keine Kappung". Die Obergrenze liegt
  über dem gemessenen Maximum von 1.127 Synonymen pro Concept.
- **Ein gültiger, aber nicht unterstützter `rank` wird abgelehnt**
  (`400`, unter Nennung des Werts), nicht still ignoriert: UC5 definiert
  nur für `species` eine Ausschlussmenge, und eine unbeabsichtigt
  ungefilterte Antwort wäre die gefährlichere Variante.
- **`output.Repository.SynonymCandidates`** (neu) trägt `nom_status`, das
  dreiwertige `homotypic` und `is_basionym` pro Synonym. `is_basionym`
  wird im SQLite-Adapter aufgelöst (`accepted_name.basionym_id = name.id`,
  113.642 Synonymzeilen im gemessenen Index) und durch einen
  Adapter-Test festgenagelt — bliebe das Flag überall `false`, würde
  UC5-Regel 4 still zum No-op, und nichts in `internal/domain` könnte das
  bemerken. `Concept()` und damit `/v1/concept/{id}` bleiben unverändert.
- **`rank_verbatim` wird durchgereicht.** 6.409 Synonymzeilen ranken als
  `OTHER`, 3.731 davon mit einer erfassten Schreibweise (`proles` 2.338,
  `lusus` 658, `microgène` 336, `Convariety` 184, `grex` 41). Keine davon
  wird von `rank=species` ausgeschlossen — sie landen also in
  Publikationslisten, wo ein blankes `OTHER` nichts aussagt. Gleiche
  Begründung wie bei `conceptDTO.rank_verbatim`.
- Dokumentation: `docs/reference/http-api.md` und
  `api/openapi/openapi.yaml` (handgepflegt, dokumentierte Abweichung seit
  S14). Beide benennen ausdrücklich, dass `typification: heterotypic` auf
  dem aktuellen Index nicht auftreten kann (`concept_name.homotypic` ist
  1 oder NULL, nie 0).

### Fixed (SP6, Task 3)
- **Zwei von drei Test-Row-Sources ließen `nom_status`/`published_in`
  fallen.** `internal/adapters/http`, `internal/adapters/sqlite` und
  `internal/application` besitzen je einen eigenen Mapper von
  `wcvp.TaxonRow` auf `application.TaxonRow`; zwei davon kannten die in
  Task 1 ergänzten Felder nicht, so dass jedes Fixture-Synonym
  nomenklatorisch sauber aussah. Alle drei spiegeln jetzt wieder das echte
  Mapping aus `internal/app/ingest.go` (alle zwölf Felder).

### Added (SP6, Task 2 — Publikations-Relevanzmodell für Synonyme)
- **`internal/domain/synonym.go`**: das reine Entscheidungsmodell für
  `GET /v1/concept/{id}/synonyms` (UC5) — ohne I/O, damit die Regeln
  isoliert prüfbar bleiben. `domain.RankSynonyms` liefert **jeden**
  Kandidaten zurück (auch die ausgeschlossenen), jeweils mit dem Grund.
- **`domain.ClassifyNomStatus` bewertet per Token-Containment, nicht per
  Gleichheit.** Das in Task 1 gemessene Vokabular (1.304 distinkte Werte,
  1.225 davon mit < 10 Treffern) lässt weder ein geschlossenes Enum noch
  fail-loud-Parsing zu. Das Lehrbeispiel aus UC5, *Corynephorus
  incanescens* Bubani (`wcvp:name:405842`), trägt `", nom. illeg.
  superfl."` und **nicht** `", nom. superfl."` — ein Gleichheitsvergleich
  würde genau den Fall verfehlen, mit dem UC5 erklärt wird.
- **`unclassified` ist ein eigenes Urteil und wird zurückgehalten**, nicht
  stillschweigend publiziert: ein erfasster, aber nicht klassifizierbarer
  Status wird aus der Publikationsliste genommen, behält seinen Rohwert und
  wird in `domain.SummarizeSynonyms` gezählt. Ein *fehlender* Status
  (`absent`, 1.349.732 Namen) ist davon strikt getrennt und bleibt
  publizierbar.
- **`domain.Typification` ist dreiwertig.** `concept_name.homotypic` ist
  auf 692.941 Zeilen `NULL` — das heißt *unbekannt*, nicht „heterotypisch“.
  Sortierung: bekannt homotypisch → unbekannt → bekannt heterotypisch, und
  welcher der drei Fälle vorlag, steht im Ergebnis. **Hinweis:** im
  aktuellen Index gibt es keine einzige `homotypic = 0`-Zeile, `heterotypic`
  ist heute also unerreichbar (siehe Task-2-Report §4).
- **Ein literales Fragezeichen schlägt jede andere Regel.** Ist die Quelle
  selbst unsicher, löst hostus die Unsicherheit nicht auf: alle 13 Namen mit
  `?` im `nom_status` (`", not validly publ.?"` 8, `", an nom. valid.?"` 4,
  `", nom. superfl. ?"` 1) werden `unclassified`. Die Regel ist bewusst
  generisch — eine wertspezifische Variante stufte `", nom. superfl. ?"`
  über das blanke `superfl` als `disqualifying` ein und bewertete damit
  dieselbe erkenntnistheoretische Lage gegenteilig.
- **`domain.BotanicalOpenItems`** benennt die fünf Werte, die eine
  botanische statt einer technischen Entscheidung brauchen (Fragezeichen 13,
  `sensu auct.` 1.117, `tentatively listed as a synonym` 290, `fossil name`
  274, `isonym` 13) — sie werden ausgewiesen, nicht geraten.
- `SummarizeSynonyms` zählt zusätzlich `Absent` (wie viele publizierbare
  Synonyme allein auf einem *fehlenden* Status beruhen); `Excluded` ist
  immer allokiert.
- Rang-Ausschluss (`VARIETY`, `SUBVARIETY`, `FORM`, `SUBFORM`) ist
  **Aufrufer-gesteuert** (`domain.RanksBelowSpecies()`), nicht fest
  verdrahtet.

### Fixed (SP6, Task 1 — nomenklatorischer Status und Publikation)
- **`nom_status` und `published_in` gingen beim Ingest verloren.** Der
  WCVP-Reader las `nomenclaturalstatus`/`namepublishedin`, `domain.Name`
  hatte beide Felder, der SQLite-Adapter schrieb beide Spalten — aber die
  DTO `application.TaxonRow` und der Mapper in `pass1AcceptedAndNames`
  kannten sie nicht. Im echten Index waren beide Spalten deshalb bei **0
  von 1.448.984** Namen belegt. Jetzt: 99.252 Namen (6,85 %) mit
  `nom_status`, 1.448.934 (99,997 %) mit `published_in`. Das ist die
  Voraussetzung für den Publikations-Relevanzfilter von
  `GET /v1/concept/{id}/synonyms` (UC5).
- Beide Spalten werden als SQL `NULL` gespeichert, wenn die Quelle nichts
  liefert — ein leerer Quellwert wird nicht zu einem Platzhalter.
- Das tatsächlich gemessene `nom_status`-Vokabular (1.304 distinkte Werte,
  Verteilung, Präfix-Artefakt `", "`, Mehrfachstatus je Zelle) ist in
  `docs/research/reality-check.md`, Abschnitt „SP6 Task 1", dokumentiert.

### Added (SP5, Task 4 — `POST /v1/translate`)
- **`POST /v1/translate`**: Übersetzung eines Konzepts zwischen
  `sec.`-Referenzräumen (UC6). Einstieg über `concept_id` **oder**
  `verbatim` (dieselbe Auflösung wie `POST /v1/match`, inklusive der Regel,
  dass ein Fuzzy-Treffer **immer** `requires_review: true` setzt) plus
  `target_space`. Die Antwort ist die **abgeleitete** `sec_inference`-
  Struktur (Architektur-Spec §4.3) — keine persistierte Tabelle.
- **Nur `congruent` ist eine Gleichsetzung.** `domain.Relation.IsEquality`
  hält diese Regel an genau einer Stelle; jeder Kandidat trägt
  `is_equality` explizit (auch `false` — ein fehlendes Feld läse sich wie
  „unbekannt") plus eine deutschsprachige Begründung. `overlaps` und das
  bewusst unbestimmte ⊂⊃⊕ (`includes_or_included_in_or_overlaps`) bleiben
  als das sichtbar, was sie sind, und werden nicht eingeebnet.
- **Ehrliche Richtung**: hostus speichert Relationen nur in der
  Quellrichtung, deshalb liefert jeder Kandidat die gespeicherte Aussage
  (`statement`), deren Relation (`stored_relation`), die Richtung
  (`direction`) und die richtungssichere quellenseitige Lesart
  (`relation_from_source` + `has_inverse`). Ein Feld namens `relation` gibt
  es **bewusst nicht**: CDM emittiert ausschließlich die
  `Includes`-Richtung, eingehende Kanten sind also häufig, und ein Client
  mit `if c.relation == "includes"` läse eine eingehende Kante genau
  verkehrt herum. `relation_from_source` ist immer vorhanden und
  ausdrücklich `null` (nicht weggelassen), wenn keine sinnvolle Umkehrung
  existiert (eingehende `pro_parte`-Kante) — hostus erfindet keine.
- **Genau ein Hop**, ohne Konfigurationsmöglichkeit: eine transitive Kette
  ist über dieses Vokabular nicht allgemein gültig (`overlaps ∘ overlaps`
  sagt nichts, `⊂⊃⊕ ∘ irgendwas` ist undefiniert). `max_hops` steht auf
  jeder Antwort; `max_hops != 1` wird mit `400 INVALID_QUERY` **benannt**
  abgelehnt statt still zu einer Ein-Hop-Antwort zu degradieren.
- **Keine Relation ist eine Antwort, kein Fehler**: `200` mit
  `result: "no_relation_recorded"` und leerem (nie weggelassenem)
  `candidates`. Ein Namenstreffer wird **nie** ersatzweise als Übersetzung
  ausgeliefert. Optional (`include_name_candidates: true`) erscheinen
  namensgleiche Konzepte unter dem eigenen Schlüssel
  `unrelated_name_candidates`, ohne Relationsfeld und mit
  `requires_review: true`.
- Fehlerabbildung: unbekannte `concept_id` **oder** unbekannter
  `target_space` → `404 NOT_FOUND` (ein Tippfehler im Zielraum darf nicht
  wie „keine Relation erfasst" aussehen); nicht auflösbares `verbatim` →
  `422 UNRESOLVABLE`.
- Neue Repository-Methoden `SecReferenceByID` und `ConceptRelationsInSec`
  (Ein-Hop-Kanten in **beiden** gespeicherten Richtungen, Selbstkanten
  ausgeschlossen, deterministisch sortiert; liefert das Quellkonzept mit,
  damit „Id unbekannt" und „keine Relationen" von einer Abfrage entschieden
  werden).
- Doku: `docs/how-to/sec-translate-uc6.md` (deutsch — Lizenzlage der
  CDM-Daten, Ein-Hop-Grenze, „keine Antwort heißt keine erfasste Relation")
  und `docs/reference/http-api.md`; OpenAPI-Baseline um `/v1/translate`
  erweitert.

### Added (SP5, Task 3 — `sec.` als erstklassige Konzeptdimension)
- `domain.SecReference` und `domain.Relation` mit strikter `ParseRelation`:
  Das Relationsvokabular ist jetzt **gemessen statt angenommen**. Die fünf
  Werte aus SP1 (`congruent|includes|included_in|overlaps|disjoint`) waren
  eine Annahme; die Vollernte über 26.346 Relationen korrigierte sie in
  beide Richtungen — `disjoint` kommt **nie** vor und ist entfallen, dafür
  kamen `pro_parte`, `misapplied`, das genuin **unsichere** ⊂⊃⊕
  (`includes_or_included_in_or_overlaps`, wird **nicht** auf `overlaps`
  eingeebnet) und `not_congruent` (genau 1 Zeile von 26.346) hinzu. Ein
  unbekannter Wert bricht laut ab und nennt den Wert — die
  `ParseRank`-Lektion, diesmal ohne lenienten Zwilling, weil eine Relation
  eine wissenschaftliche Aussage ist und kein beschreibendes Metadatum.
- `internal/adapters/cdm`: `ReadConcepts`/`ReadRelations` für die beiden
  kanonischen CDM-CSVs (Pipe-CSV mit RFC-4180-Quoting, gesammelte Fehler mit
  Zeilennummern, keine Panics). `is_concept_relation` ist ein **Tri-State**
  (`true`/`false`/leer = unbekannt), leerer `status` bleibt leer.
- `application.IngestCDM`: strikt zweiphasiger Ingest der CDM-Konzepte als
  **zweiter Backbone** (`cdm:concept:<uuid>`, `sec_reference` gefüllt) —
  bewusst eigene Zeilen neben den WCVP-Konzepten desselben Namens.
  Misapplied-Name-Zeilen (`conceptRelationship: false`) sind **keine**
  Konzeptrelationen und werden verworfen, gezählt und bemustert;
  Relationen werden **nur in der Quellrichtung** gespeichert (Inversion ist
  über `domain.Relation.Inverse` eine Abfragezeit-Frage), nicht auflösbare
  Enden werden gezählt und bemustert, brechen den Ingest aber nie ab.
- Schema: neue Tabelle `sec_reference`, Index auf
  `taxon_concept(sec_reference)`, erweitertes `relation`-Vokabular und ein
  auf `(from_concept, to_concept, relation, source)` verbreiterter
  Primärschlüssel von `concept_relation` — vorher überschrieben sich zwei
  verschiedene Relationstypen desselben Paares still. Altbestände werden
  beim `Open` per `PRAGMA table_info`-geprüfter Migration umgebaut.
- Manifest: `concept_sources:` mit `redistribution: unknown` (CDM hat
  **keine** auffindbare Lizenz), schema-validiert wie jede andere Quelle;
  `hostus ingest` gibt Konzeptquellen samt aller Verlustzähler aus.
- Offline-Bundle: `sec_reference` (auf die Referenzräume der kopierten
  Konzepte gescopet, weil `title` geernteter Inhalt ist) und
  `concept_relation` (beidseitig gescopet) werden mitkopiert; das
  Redistributionsgatter verweigert einen Bundle-Export mit CDM-Daten per
  Default und protokolliert die Quelle unter `--force-include-restricted`.

### Fixed (SP5, Task 3 — Review)
- CDM-Ingest schreibt `parent_id` in einem **zweiten Unterlauf** (wie der
  WCVP-Pfad). Vorher brach der Ingest ab, sobald ein Kind vor seinem Elter
  in der Datei stand — gemessen am echten Artefakt betrifft das 312 der 697
  Zeilen mit `parent_uuid`. Die Fixture kann das nicht zeigen, daher ein
  SQLite-Test mit umgekehrter Reihenfolge.
- Der Umbau des `concept_relation`-Primärschlüssels läuft jetzt in **einer
  Transaktion**, prüft die Bedingung darin erneut, räumt eine
  Scratch-Tabelle vorher weg und verifiziert das Ergebnis per
  `PRAGMA foreign_key_check`. Beide Abbruchfenster der vorigen Fassung
  (unöffenbare Datenbank bzw. stiller Datenverlust) sind zusätzlich
  rückwärts reparierbar: eine liegengebliebene Scratch-Tabelle wird beim
  `Open` zurückgeführt.
- `internal/adapters/cdm`: Zeilennummern kommen aus `csv.Reader.FieldPos`
  bzw. `csv.ParseError` statt aus einem Satzzähler (der bei mehrzeiligen
  gequoteten Feldern driftet). Beschädigte Sätze brechen den Durchlauf
  **nicht** mehr ab — ein `csv.ParseError` verbraucht Eingabe, also wird
  jede spätere Zeile weiter gelesen; begrenzt (und dann ein **harter**
  Fehler) ist nur noch der Lesevorgang, der **gar keine Eingabe
  verbraucht** (klebriger I/O-Fehler). Vorher hätten 20 beschädigte Zeilen
  den Rest der Datei still verworfen und trotzdem Erfolg gemeldet.
- Die Wiederherstellung einer abgebrochenen `concept_relation`-Migration
  läuft jetzt **innerhalb** des `foreign_keys=OFF`-Fensters und prüft
  danach `PRAGMA foreign_key_check`. Vorher machte eine Scratch-Zeile mit
  nicht auflösbarem Ende die Datenbank dauerhaft unöffenbar — mit einem
  opaken Treiberfehler statt der benannten Meldung.
- Beide Migrationstransaktionen nutzen `BEGIN IMMEDIATE` (sie lesen und
  schreiben; unter WAL scheitert die Aufwertung eines einfachen `BEGIN`
  sonst mit `SQLITE_BUSY_SNAPSHOT`, das `busy_timeout` nicht wiederholt),
  und ein fehlgeschlagenes `PRAGMA foreign_keys = ON` verdeckt den
  Migrationsfehler nicht mehr (`errors.Join`).
- `internal/app/integration_test.go` an die neue `app.Ingest`-Signatur
  angepasst; `make test-integration` kompiliert und läuft wieder.

### Added (SP5, Vorarbeit)
- Pipeline `pipelines/cdm/` (`build.sh`, `crawl.py`, `convert.py`,
  `common.py`, README): resumierbare Ernte der 51.466 CDM-Konzepte aus
  `rl_standardliste` in 18 `sec.`-Referenzräumen plus des Konzept­
  relationsgraphen, ausgegeben als zwei kanonische, pipe-getrennte CSVs
  (`cdm-concepts-canonical.csv`, `cdm-relations-canonical.csv`). Trägt
  bewusst das **rohe** CDM-Vokabular in `rank` und `relation_type` (22 Ränge,
  7 Relationstypen gemessen — einer mehr als Task 1s Stichprobe sah); das
  Mapping gehört nach Task 3, wo ein unbekannter Wert laut abbricht
  (`ParseRank`-Lektion). Auflösung der Relationen über eine globale
  Kanten-Map ohne P8s Namensrestriktion, mit dem verbindlichen Falsifikator
  aus `docs/research/cdm-sample.md`: Abbruch ohne CSV, sobald eine
  Relations-UUID einen dritten Halter bekommt, plus Meldung der
  verbleibenden Ein-Halter-UUIDs. Crawl-Etikette (ein ehrlicher User-Agent,
  ≤ 1 req/s, harter Stopp statt Browser-UA bei 401/403, Backoff, Plattencache)
  ist verbindlich implementiert. `status` trägt ausschließlich das rohe
  `TaxonNodeDto.taxonStatus` und bleibt leer, wo der Baumlauf das Konzept
  nicht erreicht hat; nichts wird synthetisiert. Beide CSVs sind
  RFC-4180-gequotet (`csv`-Reader mit `Comma='|'` nötig, kein `split('|')`).
  Exit-Codes trennen Falsifikator (`3`) von jedem anderen Konvertierungs­
  fehler (`4`). Lizenzlage unverändert:
  **keine Lizenzangabe auffindbar → `redistribution: unknown`, nur lokale
  Auswertung**; committet ist neben den Skripten nur eine
  De-minimis-Testfixture von 32 Zeilen, damit die Task-3-Tests offline laufen
- Messsonde `poc/p08b_cdm_sample/` (`probe.sh` + `cdm_sample.py`, reproduzierbar
  über Seed `20260802` und die committete `sample.tsv`) und der Bericht
  `docs/research/cdm-sample.md`: die Zwei-Hop-Methode aus PoC P8 an einer
  geschichteten Stichprobe von 500 CDM-Konzepten gemessen. Kernbefunde:
  Relationsdichte ≈ 55 % datensatzweit (95-%-Cluster-Bootstrap 48–63 %),
  Relations-UUIDs sind sehr gut gestützt echte Kantenidentitäten (gemessen
  75,9 % Auflösung unter P8s Namensrestriktion, 79,1 % ohne sie; ≈ 100 % bei
  Vollcrawl projiziert, gestützt auf 202/202 anomaliefreie Kanten und mit
  einem Falsifikator für Task 2), drei Relationstypen außerhalb des
  angenommenen SP1-Vokabulars, `sec.`→Klassifikation nur über eine
  handgepflegte Crosswalk-Tabelle, Vollcrawl-Kosten 14–30 h bei 1 req/s

hostus wird vom zustandslosen GBIF-Autosuggest-Proxy zum lokalen
Multi-Backbone-Namens- und Merkmalsservice umgebaut (siehe
`docs/superpowers/specs/2026-07-31-hostus-2.0-architecture.md`). Dieser
Abschnitt sammelt den Abschluss von **SP0 (Harness & Skelett)**, **SP1
(Foundation)**, **SP2 (Suggest + Offline-Bundle)**, **SP3 (Traits +
Fuzzy-Match)** und **SP4 (Xref-Enrichment über eine Wikidata-Brücke)**: SP1
liefert das lokale SQLite/FTS5-Rückgrat selbst — Ingest eines
WCVP/POWO-DwC-A-Manifests
(`hostus ingest`) in eine versionierte lokale Datenbank, gruppiert nach
akzeptierten Concepts mit ihren Synonymen, sowie die ersten drei
`/v1`-Leseendpunkte (`GET /v1/concept/{id}`, `GET /v1/xref`, `POST
/v1/match`) darauf, an die `/health/ready` jetzt gekoppelt ist. SP2 baut
darauf den Frontend-Autosuggest-Endpunkt (`GET /v1/suggest`, gebiets- und
rangfiltert, priorisiert) sowie den gebietsgescopten Offline-Export
(`hostus bundle`) für feldeinsatztaugliches, vollständig
netzwerkunabhängiges Serving. **SP3 (Traits + Fuzzy-Match)** ergänzt
ökologische Merkmalswerte (EIVE, Tichý et al. 2023, Midolo et al. 2023) und
ein zweites Klassifikationsmerkmal — Namens-Ähnlichkeitsabgleich für
Vegetationsaufnahme-Importe mit Tippfehlern:

- Drei Trait-Pipelines (`pipelines/{eive,tichy,midolo}/build.sh`,
  xlsx/csv → gemeinsames kanonisches CSV-Format) plus Reader
  (`internal/adapters/traits`) und Fixtures, gegen die echten
  Zenodo-Downloads gezogen; `internal/domain.ScaleFor` dokumentiert pro
  (Vokabular, Dimension) Min/Max/Normalisierungsstatus anhand der
  tatsächlich gemessenen Wertebereiche (Tichý T/M reichen bis 12, nicht
  wie beim „klassischen" Ellenberg-Schema bis 9)
- Namens-Crosswalk-Ingest (`application.IngestTraits`,
  `trait_value`/`trait_vocabulary`-Tabellen): jede Trait-Tabellenzeile trägt
  nur einen rohen Taxon-Namensstring (keine WCVP-/POWO-ID), der Abgleich
  gegen den ingestierten Backbone ist daher verlustbehaftet — `hostus
  ingest` gibt `matched`/`unmatched`/`ambiguous` je Vokabular UND eine
  Stichprobe der nicht zugeordneten Namen aus, statt den Verlust
  stillschweigend zu verschlucken
- `GET /v1/concept/{id}/traits?vocab=` — Merkmalswerte gruppiert PRO
  Vokabular (nie zusammengeführt, da deren Taxonomie-Namensräume
  nachweislich divergieren), jeder Wert trägt seine eigene `scale`
  (nicht das ganze Set), da selbst innerhalb eines Vokabulars die Skalen
  zwischen Dimensionen abweichen (Tichý T: 1–12 vs. L: 1–9)
- `POST /v1/match`: neue Klassifikation `fuzzy` (normalisierte
  Levenshtein-Ähnlichkeit, Schwelle 0.85) als Auffangnetz, nachdem
  `exact`/`exact_author`/`aggregate_alias` nichts fanden — `fuzzy` setzt
  `requires_review` IMMER auf `true`, unabhängig von der Ähnlichkeits-Score
- `GET /v1/concept/{id}`: `classification` (Eltern-Kette, root-first,
  tiefenbegrenzt) und `synonyms[].homotypic` (nur `true`, wenn die
  Basionym-Verknüpfung ein gemeinsames Basionym beweist, sonst fehlt das
  Feld statt fälschlich `false` zu behaupten)
- `hostus bundle`: die im "Bekannte Einschränkungen"-Abschnitt (unten)
  angekündigte Gefahrenstelle — ein gebietsgescoptes Bundle, dessen
  Concept/Name auf ein NICHT mitkopiertes Parent/Basionym außerhalb des
  Gebiets verweist — ist jetzt eingetreten (SP3 befüllt `parent_id`
  erstmals) und behoben: `ExportBundle` NULLt gebietsfremde
  Selbstreferenzen, statt mit einem FK-Fehler abzubrechen;
  `trait_value`/`trait_vocabulary` werden ebenfalls gebietsgescopt mit
  exportiert, ein Bundle bleibt also vollständig offline abfragbar
  (Concept, Suggest, Traits) — end-to-end belegt in
  `internal/app/integration_test.go`
- **Deterministische Namensnormalisierung im Trait-Crosswalk**
  (`internal/domain/normalize.go`, `NameCandidates`): ein Trait-Name wird
  jetzt gegen eine geordnete Kandidatenleiter aufgelöst — zuerst
  unverändert `domain.Canonicalize` (identisches Verhalten wie bisher),
  danach je eine Regel für Hybridmarker (`Acer ×coriaceum` →
  `acer × coriaceum`, Marker ergänzen/entfernen, ASCII-`x`),
  Aggregatmarker (`Acer opalus aggr.`, `… s. l.`), infraspezifische
  Autonyme (`Acer obtusatum subsp. obtusatum`) und die
  `-ii`/`-i`-Genitivalternation (ICN Art. 60.8). Kein Fuzzy-Matching: jede
  Regel ist eine endliche, nomenklatorisch begründete Umschreibung, nichts
  wird bewertet. Gemessen an den Volldaten steigt die Taxon-Auflösbarkeit
  von 87,84 / 95,73 / 96,41 % auf **97,96 / 98,82 / 99,11 %** (EIVE /
  Tichý / Midolo), die nicht auflösbaren Trait-Zeilen sinken um
  84 / 72 / 75 % — vollständige Messung inkl. marginalem Zugewinn je Regel
  in `docs/research/reality-check.md`, Abschnitt „Nach Hardening (Task 5)".
  Zwei der Regeln setzen Umgrenzungen gleich, die nicht identisch sind
  (Aggregat → Nominatart, Autonym → Art). Sie sind deshalb **in den Daten**
  gekennzeichnet, nicht nur im Ingest-Report: neue nullable Spalte
  `trait_value.resolution` (NULL = exakter Treffer, sonst der Regelname),
  ausgeliefert als `resolution` auf `GET /v1/concept/{id}/traits`
  (`omitempty` — fehlt das Feld, war es ein exakter Treffer) und in jedes
  Offline-Bundle mitkopiert. Ein Konsument, der die Näherung nicht
  akzeptieren kann, filtert auf `resolution IN
  ('aggregate_to_nominate','autonym')`. Die Mehrdeutigkeitsquote steigt
  durch die Normalisierung (EIVE +231 Taxa) — ausschließlich aus vorher
  unauflösbaren Namen; mehrdeutig heißt weiterhin: es wird nichts
  geschrieben.
- **Exakte Treffer gewinnen den `trait_value`-Platz** (`application.
  selectTraitWinners`). Der Crosswalk ist viele-zu-eins (EIVE führt
  `Acer opalus` UND `Acer opalus aggr.`, beide lösen auf dasselbe Konzept
  auf), der Primärschlüssel von `trait_value` ist aber
  `(concept_id, vocab, vocab_version, dim)` und `AddTraitValue` ein
  `INSERT OR REPLACE` — bis hierher entschied damit die Zeilenreihenfolge
  der CSV, ob ein exakt getroffener Wert oder das Kollektivmittel eines
  Aggregats gespeichert wurde. Jetzt schlägt ein exakter Treffer immer
  einen normalisierten, unter Gleichrangigen gewinnt die erste Zeile.
  Messbare Folge: 646 EIVE- und 65 Tichý-Konzepte behalten ihren exakten
  Wert, den sie zuvor an einen normalisierten verloren hatten; die Zahl der
  exakt aufgelösten Konzepte je Vokabular entspricht damit wieder exakt der
  Baseline (11.000 / 7.072 / 4.963), die Normalisierung ist auf
  Konzeptebene beweisbar rein additiv.
- **`selectTraitWinners` rangiert normalisiert-gegen-normalisiert jetzt
  explizit** (`application.ruleRank`), statt bei einem Slot-Konflikt
  zwischen zwei normalisierten Zeilen einfach die CSV-Zeilenreihenfolge
  entscheiden zu lassen — derselbe Defekt eine Ebene unter dem oben
  beschriebenen exakt-vs-normalisiert-Fix: exakt > jede ungeflaggte
  Regel (reine Schreibweise) > jede geflaggte Regel (Aggregat→Nominatart,
  Autonym→Art), mit der `NameCandidates`-Reihenfolge als Tiebreak.
  Gemessen (`poc/measure/bridge --a1diff` gegen die Task-5-DB): 10 von
  117.153 gespeicherten `trait_value`-Zeilen ändern den Gewinner, alle bei
  EIVE, alle geflaggt→ungeflaggt.
- **Die Behauptung „exakt aufgelöste Konzepte == M2'-Baseline" ist jetzt
  maschinell nachvollziehbar statt nur behauptet**: `poc/measure/stats.sql`
  bekommt eine `resolution`-Aufschlüsselung je Vokabular
  (`GROUP BY vocab, resolution`), bestätigt gegen die echte Datenbank
  11.000/7.072/4.963 exakt.
- **Bewusster Lizenz-Scope-Schnitt (PoC R1)**: Euro+Med PlantBase,
  GermanSL, EuroSL und die FloraVeg.EU-Downloads werden NICHT ingestiert —
  für keine der vier Quellen ließ sich eine belastbare
  Weiterverbreitungslizenz feststellen (`docs/research/quellenregister.md`).
  Der taxonomische Anschluss für EIVE/Tichý/Midolo läuft deshalb direkt
  gegen den WCVP/POWO-Backbone, nicht über eine dieser vier Quellen als
  Vermittlungsschicht — dokumentiert in
  `docs/how-to/trait-ingest.md`, nicht stillschweigend übergangen.

- **Hardening-Zyklus (Tasks 1–6) abgeschlossen**: die in der
  Reality-Check-Volldaten-Messkampagne (siehe „Reality-Check T2–T4" oben)
  gefundenen Defekte sind behoben und am selben unveränderten
  WCVP-Volldatensatz (1.448.984 Zeilen) nachgemessen — konsolidierte
  Vorher/Nachher-Tabelle und aktualisierte Verdikte in
  `docs/research/reality-check.md`, Abschnitt „Task 6: konsolidierte
  Vorher/Nachher-Übersicht". Kernzahlen: Ingest läuft jetzt durch
  (vorher: Absturz nach 5,37 s bzw. quadratischer Abbruch nach 22:48 min)
  in 281,27 s mit linearer Skalierung (6/11/24 s statt 65/293/1.338 s bei
  50k/100k/200k Zeilen); Taxon-Ebenen-Crosswalk-Auflösbarkeit steigt durch
  Namensnormalisierung auf 97,96/98,82/99,11 % (vorher 87,84/95,73/96,41 %);
  Mehrgebiets-Bundle-Export und der Parameterlimit-Bug beim Voll-Export
  sind behoben, das Deutschland-Bundle schrumpft von 103,8 auf 81,05 MiB
  (gzip 19,24 MiB, unter der 20-MB-Spec-Obergrenze). Die Lizenz-Zurückstellungs-
  Empfehlung aus M6 ist nach der Normalisierung STÄRKER geworden, nicht nur
  bestätigt: der real brückbare Gewinn der vier lizenzunklaren Quellen
  schrumpft von 51/3/2 auf **2/1/1 Taxa** (EIVE/Tichý/Midolo), weil
  Normalisierung fast alles bereits einsammelt, was die Brücken sonst
  beigetragen hätten (`poc/measure/bridge --normbridge`).

- **Der letzte offene Punkt des Hardening-Zyklus (Suggest-p95) ist
  nachgemessen und geschlossen — es gab keine Regression** (Task 7,
  `docs/research/reality-check.md`, Abschnitt „Task 7: die offene
  p95-Abweichung — aufgelöst"). M3' hatte gegenüber M4 einen p95-Anstieg
  von 25–27 % berichtet und die Ursache offengelassen. 19
  Wiederholungsläufe desselben Messaufbaus (je 570 Messpunkte, `--reps 15
  --warmup 3`) streuen im p95 über **225,27–316,39 ms**
  (Variationskoeffizient 9,0 %, max/min 1,41) — die berichtete Differenz
  ist kleiner als die Streuung des Aufbaus. Die **Baseline-Konfiguration
  selbst** (Code vor dem Hardening, Commit `53575fe`, gegen die
  Baseline-DB `m2.sqlite`) misst heute 262–310 ms statt der notierten
  220,2 ms, also *schlechter* als die Nach-Hardening-Konfiguration. Beide
  Sachursachen sind belegt ausgeschlossen: die acht FK-Indizes standen
  bereits in der Baseline-DB (`poc/measure/fk_indexes.sql`, dieselben acht
  Spalten) und tauchen in keinem Query-Plan des Suggest-Pfads auf — die
  `EXPLAIN QUERY PLAN`-Ausgaben mit und ohne sie sind zeichengleich; die
  von T1 zugelassenen OTHER-Rang-Zeilen erhöhen die Kandidatenmenge der
  teuersten Präfixe um 0,24–0,49 %. Ein geprüfter Fix (`ANALYZE`) ändert
  den Plan nicht und bringt im verschränkten A/B-Test nichts und wurde
  deshalb **nicht** eingebaut. Kein Code geändert; der p50 (34,5–38,7 ms
  über alle Läufe, Variationskoeffizient 3,3 %) bestätigt das
  ursprüngliche M4-Verdikt **hält**.

- **SP4 (Xref-Enrichment über eine Wikidata-Brücke) abgeschlossen.** hostus
  kennt bislang nur den `powo`-Xref aus dem WCVP-Ingest selbst; SP4 bringt
  echte Cross-References zu sechs weiteren Autoritäten hinzu, ohne
  Wikidata direkt zur Laufzeit abzufragen:
  - `pipelines/wikidata/build.sh` erzeugt einmalig, offline, einen
    kanonischen Bridge-Hub-Export (`join_authority|join_id|authority|
    ext_id|wikidata_qid`, gegen den echten POWO-Xref-Bestand gejoint,
    1.709.127 Zeilen, 0 tote `join_id`s — siehe
    `.superpowers/sdd/2026-08-02-sp4-xref/task-1-report.md`).
  - `internal/adapters/xref` (Reader) und `application.IngestXrefs`
    (zweiphasiger, ID-basierter Join, `xref_sources` im Manifest, über
    `hostus ingest` wie die Trait-Vokabulare berichtet) schreiben die neuen
    Xrefs in einer einzigen Transaktion. Zwei Situationen werden getrennt
    behandelt: dieselbe `(authority, ext_id)`-Kombination, von zwei
    verschiedenen Konzepten beansprucht, wird **übersprungen und
    gemeldet** (kein Raten); ein Konzept mit mehreren `ext_id`s derselben
    Autorität ist **kein** Konflikt und wird vollständig geschrieben.
  - **BREAKING:** `xrefs.<authority>` in `GET /v1/concept/{id}` und
    `GET /v1/xref` ist jetzt ein sortiertes Array (`{"inat": ["160927"]}`)
    statt eines einzelnen Strings (`{"inat": "160927"}`) — jeder Client,
    der den Wert als String liest, muss angepasst werden. Die
    vorherige `map[string]string` hätte bei mehreren `ext_id`s pro
    Autorität stillschweigend nur die zuletzt geschriebene behalten, was
    real bei 63 Konzepten (`inat` allein) aufgetreten wäre.
  - Xref-Provenienz (`xref_source`-Tabelle + `xref.source`-Spalte, wie
    `backbone_version`/`trait_vocabulary`): eine ingestierte Datenbank
    beantwortet jetzt, aus welcher Ernte ihre Xrefs stammen (Version,
    Lizenz, `manifest_sha`), und der Redistributions-Gate von `hostus
    bundle` deckt damit auch Xref-Quellen ab — ein Export mit einer nicht
    freigegebenen Xref-Quelle im Scope wird per Default verweigert,
    `--force-include-restricted` protokolliert sie in
    `bundle_meta.restricted_sources`. Lokaler Ingest bleibt ungegated.
    Bestandsdatenbanken (SP1–SP3) migrieren beim nächsten `Open`
    automatisch (`ALTER TABLE xref ADD COLUMN source`, bestehende Zeilen
    behalten `source = NULL`).
  - Neue deutsche UC2-Anleitung `docs/how-to/inat-uc2.md` (Suggest →
    Concept → `xrefs.inat` → iNaturalist-Beobachtungen) inklusive der
    gemessenen PoC-Einschränkungen (Koordinatenverwischung ~26–28 km,
    32–38 % `obscured`, `quality_grade=research` = zwei
    Community-Zustimmungen statt Fachverifikation).
  - Volldaten-Messung (`docs/research/reality-check.md`, Abschnitte „SP4
    Task 2" und „SP4 Task 4 — Verdikt"): **392.218 / 440.534 Konzepte
    (89,03 %)** gewinnen mindestens einen neuen Xref; pro Autorität
    wikidata 392.218, gbif 383.907, wfo 365.731, colxr 357.878, **inat
    182.821 (41,50 %)**, floraveg 24.274, euromed 95; 16 echte Konflikte
    (8 externe Schlüssel, alle gbif/wfo); 0 von 1.709.127 Zeilen
    unmatched — Letzteres ist durch den joinable-subset-Filter der
    Pipeline garantiert und validiert den Ingest-Join, nicht die
    Abdeckung. **Verdikt: hält mit Auflagen** — der Ingest selbst ist
    vollständig korrekt und end-to-end bewiesen
    (`internal/app/integration_test.go`), aber UC2 erreicht wegen der
    Datenlage bei iNaturalist nur 41,50 % der Konzepte; für die übrigen
    58,5 % muss ein Client "keine iNat-Verknüpfung gefunden" ehrlich
    anzeigen statt zu raten.

Weitere Backbones (COL XR) sowie `translate` und `/openapi` als generierte
Spezifikation folgen in SP5+. `release-please` cuttet daraus das nächste
`2.0.0-alpha.N`-Release; bis dahin akkumulieren PRs hier.

### Fixed
- Xref-Konflikterkennung greift jetzt auch über Läufe hinweg: bisher
  gruppierte `application.IngestXrefs` nur die Zeilen der GERADE
  ingestierten Quelle, sodass eine `(authority, ext_id)`, die bereits in
  der Datenbank auf ein anderes Konzept zeigte (früherer Lauf, ältere CSV,
  zweite Quelle), von `INSERT OR REPLACE` stillschweigend umgehängt wurde —
  ohne Zählung, ohne Stichprobe. Phase 1 löst den bestehenden Eigentümer
  jedes externen Schlüssels jetzt mit auf (weiterhin ein Lesevorgang VOR
  der Transaktion) und behandelt eine Abweichung wie einen quellinternen
  Konflikt: übersprungen, gezählt, gesampelt. Ein unveränderter Re-Ingest
  bleibt idempotent.
- Trait-Ingest schreibt keine `backbone_version`-Zeilen mehr: er lief zuvor
  über `BeginIngest` mit einer synthetischen `trait:<vokabular>`-Version,
  die im `backbone_versions`-Provenienzblock JEDER `/v1/suggest`- und
  `/v1/match`-Antwort auftauchte (ein Trait-Vokabular als taxonomischer
  Backbone ausgewiesen) und `/health/ready` verfälschte. Neuer Port
  `output.Repository.BeginTraitIngest` öffnet die Transaktion ohne
  Backbone-Eintrag.
- Trait-Ingest gleicht die `(vocab, version)`-Identität aus Manifest und
  kanonischer CSV jetzt zeilenweise ab und bricht bei Abweichung mit einer
  Fehlermeldung ab, die beide Seiten nennt, statt Werte unter fremder
  Identität zu speichern (ein `id: eive`, das auf Tichýs CSV zeigt, hätte
  dessen 1–12-Werte auf EIVEs normalisierter 0–10-Skala ausgeliefert);
  persistiert wird die im Manifest gepinnte Version. `dataset.example.yaml`
  pinnte für Tichý/Midolo Versionen, die die Pipelines gar nicht emittieren
  (`1.0` statt `2.0`/`3`) — korrigiert.
- `dataset.example.yaml` verweist für Tichý/Midolo nicht mehr auf
  `floraveg.eu/download/...` (Lizenz ungeklärt, PoC-R1-Scope-Schnitt),
  sondern auf die Zenodo-DOIs der CC-BY-4.0-Originaldatensätze — der Wert
  wird in `trait_vocabulary.source_url` persistiert, über die API
  ausgeliefert und in jedes Offline-Bundle kopiert.
- Trait-CSV-Reader: die Kurzzeilen-Prüfung ging von genau sieben Spalten
  aus und verursachte einen Panic bei einem Header mit ZUSÄTZLICHEN Spalten (die der Reader
  mit `FieldsPerRecord = -1` ausdrücklich zulässt); sie orientiert sich
  jetzt an der tatsächlichen Position der gesuchten Spalten.
- `POST /v1/match`: der Aggregat-Pfad wählte bei mehreren Treffern
  `candidates[0]`, auch wenn diese auf VERSCHIEDENE Concepts zeigten. Er
  wendet jetzt dieselbe Prüfung an wie der exact- und der fuzzy-Pfad:
  `requires_review: true`, alle Kandidaten gelistet, kein geratenes
  `concept_id`.
- Fuzzy-Prefilter der SQLite-Adapter: das GLOB-Präfix aus dem ersten
  Zeichen der Anfrage wird escaped — ein führendes `[` erzeugte zuvor eine
  unterminierte Zeichenklasse (fand nichts), ein führendes `*`/`?` machte
  den Prefilter zum No-op (voller Tabellenscan).
- `docs/how-to/trait-ingest.md` nannte einen Pfad
  (`pipelines/tichy/output/tichy2023-canonical.csv`), den die Pipeline nicht
  erzeugt.
- `google.golang.org/grpc` auf v1.82.1 angehoben (behebt GO-2026-6061 in der
  OTLP-gRPC-Exporter-Kette von `internal/adapters/telemetry`).
- Go-Toolchain auf 1.26.5 angehoben (`go.mod` toolchain-Direktive,
  Dockerfile-Builder-Image) — behebt drei aufrufbare Standardbibliotheks-CVEs
  (GO-2026-5856 `crypto/tls`, GO-2026-5039 `net/textproto`, GO-2026-5037
  `crypto/x509`). `govulncheck ./...` meldet danach 0 aufrufbare
  Vulnerabilities; `make security-check` ist grün.
- v1-GBIF-Proxy-Wortlaut aus operator-/user-facing Oberflächen entfernt
  (CLI-Hilfetext, CORS-Kommentar, README/OpenAPI/Docs, gomodguard-
  Begründungen), die dem SP1-SQLite-Plan widersprachen.
- `example.env`/`docker-compose.yml` auf die tatsächlichen `HOSTUS_*`-Keys
  umgestellt; `docker-compose.yml`s Port-Mapping (`HOSTUS_SERVER_PORT`)
  reparlert, das zuvor mit dem falschen `PORT`-Var auseinanderlief.
- Tote v1-Prometheus-Metriken (`hostus_cache_hits_total`,
  `hostus_cache_misses_total`, `hostus_gbif_errors_total`) entfernt, die nie
  inkrementiert wurden; `hostus_rate_limit_rejects_total` und
  `hostus_load_shedding_active` an die aktive Rate-Limit-/Load-Shed-
  Middleware angeschlossen, statt dauerhaft bei Null zu stehen.
- Totes `internal/cache` (kein Importer, ungestoppter Cleanup-Ticker/
  Goroutine-Leak) entfernt — passte nicht zum SP1-SQLite-Design.
- `hostus serve` loggt jetzt zusätzlich zum RingLog (MCP) auch auf stderr,
  inkl. einer Startzeile mit der Listen-Adresse.
- `internal/adapters/sqlite/suggest.go`: der SQL-seitige Fetch-Budget-Query
  (`ORDER BY score ASC LIMIT ?`, bm25-only) konnte in_area-Kandidaten
  (Ranking-Priorität 2 laut Spezifikation §B.1, also ÜBER dem bm25-Score)
  aus dem Kandidaten-Set werfen, bevor `domain.RankSuggestions` überhaupt
  lief — bei einem breiten Prefix mit mehr Treffern als das Budget
  (`max(20, 4×limit)`) konnten dadurch gebietsfremde Treffer vor
  gebietseigenen landen, genau die §B.1-Inversion, die die
  In-Area-Priorisierung verhindern soll. Fix: `ORDER BY in_area DESC, score
  ASC LIMIT ?`, damit in_area-Zeilen das Budget-Fenster überleben; die
  vollständige Mehrschlüssel-Sortierung bleibt weiterhin
  `domain.RankSuggestions`s Aufgabe.
- `GET /v1/suggest`: die 400-Fehlermeldung bei einem unbekannten
  `rank`-Token verkettete `"unknown rank: "` mit `domain.ParseRank`s
  eigenem Fehlertext (`domain: unknown taxon rank "foo"`) und leakte so
  interne Domain-Fehlerformulierungen in die HTTP-Antwort; die Meldung
  benennt das Token jetzt direkt (`unknown rank "foo"`).

### Bekannte Einschränkungen (SP1/SP2, behoben in SP3)
- ~~`ExportBundle` (`internal/adapters/sqlite/bundle.go`) kopiert Zeilen 1:1
  (`copyRows`), ohne Fremdschlüssel-Ziele auf Gebietszugehörigkeit zu
  prüfen: Ein gebietsgescoptes Bundle kopiert nur `taxon_concept`/`name`-
  Zeilen der im Gebiet liegenden Concepts. Solange `taxon_concept.parent_id`
  und `name.basionym_id` (SP1/SP2) für jedes ausgewählte Backbone durchweg
  NULL sind, bleibt das folgenlos. Sobald eine künftige SP diese Spalten
  erstmals befüllt, kann ein Concept/Name außerhalb des Gebiets auf einen
  NICHT mitkopierten Parent/Basionym verweisen — das gebietsgescopte Bundle
  würde dann mit einem FK-Fehler abbrechen. Wer diese Spalten zuerst befüllt,
  muss `ExportBundle` gleichzeitig anpassen (gebietsfremde Selbstreferenzen
  auf NULL setzen, oder die referenzierten Ancestor-Zeilen mit ins Bundle
  ziehen), statt das als Feld-Crash neu zu entdecken.~~ **Eingetreten und
  behoben in SP3**: `taxon_concept.parent_id`/`name.basionym_id` werden jetzt
  befüllt (Classification/Homotypic); `ExportBundle` NULLt gebietsfremde
  Selbstreferenzen statt einen FK-Fehler zu werfen — siehe SP3-Abschnitt oben
  und `internal/app/integration_test.go`s
  `TestIntegration_OfflineBundleConceptSuggestTraitsOffline`.

### Added
- Hexagonales Skelett (`internal/domain`, `application`, `ports/{input,output}`,
  `adapters/{http,mcp,telemetry}`, `app`, `config`) mit `depguard`/`gomodguard`-
  erzwungenen Schichtgrenzen
- OTel-Setup (Traces + Metrics, OTLP-Exporter) inkl. In-Memory-Span-/Log-
  Ringpuffer-Exporter, `otelmux`-instrumentierte Middleware-Kette
  (Request-ID → Logging → Rate-Limiting → Load-Shedding → Timeouts → CORS →
  Metrics)
- Stdio-Debug-MCP (`hostus mcp`, `modelcontextprotocol/go-sdk`): `get_recent_logs`,
  `tail_errors`, `get_trace`, `list_spans` — read-only Logs/Spans für Claude Code
- Cobra-CLI (`serve` als Default, `version`, sowie Stubs für `ingest`,
  `validate`, `bundle` — Implementierung folgt in SP1/SP2)
- viper-Konfiguration mit `HOSTUS_`-Präfix, Priorität `config.yaml` <
  `HOSTUS_*`-Env-Var < CLI-Flag (kein Dotenv-Loader im Binary)
- Fehler-Envelope um `NOT_FOUND` und `UNRESOLVABLE` erweitert (für die
  künftigen `/v1/concept`, `/v1/match`, `/v1/xref`-Endpunkte)
- `verify`-Gate (`fmt-check vet lint test arch debt-guard` + Compile-Check) als
  kanonischer Grün-Check, Tech-Debt-Ratchet-Trio (`.debt-budget`,
  `.coverage-floors`, `scripts/{debt-guard,coverage-gate}.sh`)
- golangci-lint v2 mit hexagonalen `depguard`-Grenzen, govulncheck, gosec,
  go-licenses-Allowlist
- Nix-Flake + direnv (`go_1_26`, projektlokaler GOPATH/GOCACHE), CGO-frei
- CI-Workflows portiert aus ortus (ohne Last-Test-Stack): `ci`, `docker-release`,
  `release-please`, `mutation`, `fuzz`, `codecharta`, `openapi-diff`,
  `commitlint`, `vuln-scan`, `actions-security` (zizmor), `dependabot-auto-merge`,
  `update-skills-submodule`
- `third_party/claude-skills`-Submodul (`jobrunner/claude-skills`, Branch `main`)
  + `.claude/hooks`/`.githooks` (pre-commit: fmt-check → build → debt-guard)
- MkDocs-Material/Diátaxis-Dokumentationsgerüst (Tutorials/How-to/Referenz/
  Erklärung) + OpenAPI-Baseline für den Doc-Drift-Check
- Sechs neue ADRs (`docs/explanation/decisions/0009`–`0014`): lokaler
  Multi-Backbone-Index, SQLite/FTS5-Persistenz, versionierter Artefaktvertrag
  (`dataset.yaml`), hexagonale Architektur, OpenTelemetry von Tag 1,
  stdio-Debug-MCP
- `.goreleaser.yml` (portiert aus ortus, `release.mode: append`, CGO-frei,
  linux/darwin × amd64/arm64)
- **SP1**: lokales SQLite/FTS5-Rückgrat (`internal/adapters/sqlite`):
  `taxon_concept`/`taxon_name`/`concept_name`/`xref`/`distribution`/
  `backbone_version`-Schema, `output.Repository`-Port (`Concept`,
  `ConceptByXref`, `MatchExact`, `BackboneVersions`, `BeginIngest`/`IngestTx`)
- **SP1**: versionierter Dataset-Manifest-Adapter (`internal/adapters/manifest`,
  `dataset.yaml`, schema-validiert, SHA-inhaltsadressiert) und WCVP/POWO-
  DwC-A-Reader (`internal/adapters/wcvp`)
- **SP1**: `application.Ingest` — liest ein Manifest-Backbone in zwei Durchgängen
  ein (Namen + akzeptierte Concepts, dann Synonym-Verknüpfung unter ihr
  akzeptiertes Concept), meldet `names`/`concepts`/`synonyms`/`orphaned`
  je Backbone; `hostus ingest --dataset <manifest> --db <file>` als CLI-Einstieg
- **SP1**: `application.MatchNames` — klassifiziert verbatime Namen als
  `exact`, `exact_author`, `aggregate_alias` oder `unresolvable`
  (Autoren-Mehrdeutigkeit liefert Kandidaten statt Fehlklassifikation)
- **SP1**: `GET /v1/concept/{id}`, `GET /v1/xref`, `POST /v1/match` auf dem
  neuen Repository; `GET /health/ready` jetzt an das Vorhandensein
  mindestens eines eingelesenen Backbones gekoppelt (vorher immer 200)
- **SP1**: `internal/app/integration_test.go` (`integration`-Build-Tag,
  `make test-integration`) — treibt den kompletten Ingest→Serve→Query-Fluss
  über einen echten `httptest.Server` end-to-end
- **SP1**: OpenAPI-Baseline (`api/openapi/openapi.yaml`) und
  `docs/reference/http-api.md` um die drei neuen `/v1`-Endpunkte ergänzt
  (Concept-/Match-DTOs, Fehler-Envelope-Schema)
- **SP2**: `GET /v1/suggest?q=&area=&rank=&limit=` — FTS5-Präfix-Autosuggest
  über den lokalen Index (`internal/adapters/sqlite`: `fts_name`/
  `fts_name_map`, bei `hostus ingest` befüllt), priorisiert nach §B.1
  (Präfix-Treffer vor Nicht-Treffer, im Gebiet vor nicht im Gebiet, akzeptiert
  vor Synonym, breitere vor feineren Rängen, bm25-Score aufsteigend) via
  `application.Suggest`/`domain.RankSuggestions`; fehlendes/leeres `q` liefert
  `400 INVALID_QUERY`
- **SP2**: `hostus bundle --db <db> --area <code> --out <bundle> [--snapshot
  v1]` — exportiert einen gebietsgescopten, eigenständigen SQLite/FTS5-
  Auszug (`sqlite.ExportBundle`) inkl. neu aufgebautem FTS-Index und
  `bundle_meta`-Provenienz; das Bundle ist danach vollständig offline per
  `hostus serve` bedienbar (kein Zugriff auf die Quell-Datenbank oder ein
  Netzwerk nötig) — siehe `docs/how-to/offline-bundle.md`
- **SP2**: SQLite-Verbindung auf `journal_mode=WAL` + `busy_timeout=5000`
  umgestellt (`internal/adapters/sqlite/db.go`), damit ein laufender
  `serve`-Reader nicht von einem parallelen `ingest`/`bundle`-Writer
  blockiert wird
- **SP2**: `application.MatchNames`-Autoren-Mehrdeutigkeit markiert
  mehrdeutige Treffer jetzt mit `requires_review: true` statt sie
  stillschweigend als `exact_author` durchzuwinken
- **SP2**: `internal/app/integration_test.go` um `GET /v1/suggest`
  (area-priorisiert, `q` fehlt → 400) sowie einen eigenen
  Offline-Bundle-Test erweitert: `sqlite.ExportBundle` scoped auf `area=AUT`,
  danach `application.Suggest` ausschließlich gegen die exportierte
  Bundle-Datei (keine Quell-Datenbank, kein HTTP-Server) — belegt den
  Feldeinsatz-Anspruch end-to-end
- **SP2**: OpenAPI (`/v1/suggest`) und `docs/reference/http-api.md` auf den
  tatsächlichen Handler-Stand reconciled; neue Anleitung
  `docs/how-to/offline-bundle.md` für den `hostus bundle`-CLI-Workflow
- **Reality-Check T1**: `redistribution`-Gate — jeder Backbone- und
  Trait-Vokabular-Eintrag im Manifest trägt jetzt ein Pflichtfeld
  `redistribution: allowed|restricted|unknown` (`internal/domain.Redistribution`,
  schema-validiert). Lokales `hostus ingest` bleibt für jede Quelle
  uneingeschränkt möglich (mit `hinweis:`-Zeile für nicht-`allowed`-Quellen);
  `hostus bundle` verweigert den Export dagegen standardmäßig, sobald eine
  nicht-`allowed`-Quelle zum Export-Scope beiträgt (Fehlermeldung nennt
  Quelle + Wert), und `--force-include-restricted` übersteuert das bewusst,
  protokolliert die betroffenen Quell-IDs aber unauslöschlich in
  `bundle_meta.restricted_sources` — ein Dokumentationsversprechen wird so
  durch eine maschinelle Prüfung ersetzt (siehe
  `docs/how-to/trait-ingest.md`, `docs/how-to/offline-bundle.md`)
- **Reality-Check T2–T4**: Volldaten-Messkampagne gegen den echten
  WCVP/POWO-Dump (1.448.984 Zeilen) plus EIVE/Tichý/Midolo und die vier
  Kandidaten-Brückenquellen Euro+Med, EuroSL, GermanSL, FloraVeg
  (`poc/measure/`, Ergebnisse + Verdikte in `docs/research/reality-check.md`,
  repo-lokal, nicht in der MkDocs-Navigation). Kernbefunde: der
  Serienstand-Ingest ist an Volldaten **nicht einsatzfähig** (bricht nach
  5,37 s an unbekannten WCVP-Rängen ab, oder läuft quadratisch und wurde
  nach 22:48 min ohne einen einzigen committeten Datensatz abgebrochen) —
  mit acht zusätzlichen FK-Indizes und erweiterter Rangunterstützung sinkt
  derselbe Volldaten-Ingest auf 276,70 s; die Zeigerwert-Abdeckung ist auf
  Taxon-Ebene 87,76–96,41 % für die Taxa, die EIVE/Tichý/Midolo tatsächlich
  führen (die oft zitierte 2,64 %-„Abdeckung" bezieht sich auf den globalen
  WCVP-Nenner und ist kein Defektbefund); das gebietsgescopte Offline-Bundle
  ist mit 108,9 MB Faktor 5,4 über der Spec-Annahme von 10–20 MB, und ein
  Mehrgebiets- oder Voll-Bundle ist mit der heutigen CLI nicht exportierbar;
  die vier lizenzunklaren Brückenquellen liefern zusammen real nur 51
  zusätzliche EIVE-Taxa (≈0,34 %), weil Euro+Med und FloraVeg keinen Rang
  und keinen aufgelösten Akzeptiert-Link führen — bessere
  Namensnormalisierung (Aggregate, Hybridzeichen, Autonyme, Orthographie)
  ist laut Stichprobe der lohnendere Hebel als die Lizenzklärung

### Changed
- `Dockerfile`: Build-Stage injiziert `main.Version`/`main.Commit`/`main.BuildDate` per Ldflags (statt `main.version` aus `VERSION`-Datei) — identische Variablenpfade wie im Makefile, damit `hostus version` im Image echte Build-Infos zeigt
- `Dockerfile`: `USER` auf numerische UID:GID (`65532:65532`, distroless "nonroot") statt Namen umgestellt (hadolint DL3066)
- `flake.nix`: `hadolint` zum devShell hinzugefügt (Dockerfile-Linting reproduzierbar verfügbar, auch für CI in S12)
- `docker-compose.yml`: Kommentar zum Healthcheck ergänzt (Exec-Form-Prozess-Check, kein HTTP-Probe — Grund siehe unten)
- `CLAUDE.md`: Reconciled auf hostus 2.0 — Projektbeschreibung, erlaubte
  Bibliotheken, API-Endpunkte, Fehlercodes und Architektur-Abschnitt spiegeln
  jetzt den Multi-Backbone-Service statt des v1-GBIF-Proxys; Git-/CI/CD-
  Workflow-Regeln unverändert
- `architecture/adrs.md`: ADR-001, ADR-003, ADR-008 als **Superseded**
  markiert (Verweis auf ADR-0009/-0010); ADR-004 (Go + Minimal-Dependencies)
  im Wortlaut auf den hostus-2.0-Stack reconciled

### Removed
- `internal/gbif`, GBIF-gestützter `suggest`-Handler und der GBIF-spezifische
  Taxonomie-Mapper (Architektur-Inversion laut Spec Abschnitt 6)
- `Dockerfile`: `HEALTHCHECK`/`COPY VERSION` entfernt. `gcr.io/distroless/static` hat weder Shell noch HTTP-Client (kein curl/wget) — ein In-Container-Probe gegen `GET /health/live` ist technisch nicht möglich. Liveness wird stattdessen extern vom Orchestrator gegen `GET /health/live` geprüft. Ein `hostus health`-Self-Probe-Subcommand wäre eine Option, aber ungetestete Go-Logik außerhalb dieses Tasks — bewusst nicht umgesetzt

## [0.2.4] - 2026-06-16

### Added
- **CI**: Trivy-Filesystem-Scan im Security-Job (`go.sum`, Dockerfile, IaC). Schwellwert: HIGH/CRITICAL, ignoriert unfixable Findings, blockt PR bei Treffern
- **Release**: Trivy-Image-Scan post-build (HIGH/CRITICAL → Release kippt)
- **Release**: Automatische SBOM-Generierung im CycloneDX- und SPDX-Format, angehängt als Release-Asset
- **Release**: BuildKit-Attestationen (`sbom: true`, `provenance: mode=max`) als OCI-Manifest am Image — abrufbar via `docker buildx imagetools inspect`
- **CI/Release**: SARIF-Upload der Trivy-Findings nach GitHub Security ("Code scanning alerts")
- `.trivyignore`-Datei (leer) für künftige bewusst akzeptierte Findings

### Changed
- Permissions `security-events: write` in `ci.yml` und `release.yml` (SARIF-Upload)

## [0.2.3] - 2026-06-13

### Fixed
- Port-Mapping in docker-compose.yml korrigiert (intern und extern dynamisch)

### Changed
- `docker-compose.yml` verwendet jetzt Registry-Image (`ghcr.io/jobrunner/hostus:latest`)

### Added
- `docker-compose.local.yml` als Override für lokale Image-Builds
- Make-Targets: `start` (Registry-Image), `start-local` (lokaler Build), `stop`

## [0.2.2] - 2026-06-13

### Changed
- GitHub Actions auf Node-24-fähige Major-Versionen angehoben (GitHub erzwingt Node 24 ab 2026-06-16):
  - `actions/checkout`: v4 → v6
  - `actions/setup-go`: v5 → v6
  - `actions/upload-artifact`: v4 → v7
  - `golangci/golangci-lint-action`: v7 → v9
  - `docker/setup-qemu-action`: v3 → v4
  - `docker/setup-buildx-action`: v3 → v4
  - `docker/login-action`: v3 → v4
  - `docker/build-push-action`: v6 → v7
  - `softprops/action-gh-release`: v2 → v3

## [0.2.1] - 2026-06-13

### Fixed
- Release-Workflow (`release.yml`): Permission `contents: write` ergänzt, damit `softprops/action-gh-release` GitHub Releases anlegen darf (vorher 403 "Resource not accessible by integration")

## [0.2.0] - 2026-06-13

### Changed
- Go-Toolchain von 1.24 auf 1.26 angehoben (längeres Security-Support-Fenster bis ~Feb 2027)
- `flake.nix`: `pkgs.go_1_24` → `pkgs.go_1_26` (liefert aktuell 1.26.3; zieht automatisch auf 1.26.4 nach, sobald nixpkgs-unstable nachzieht)
- `flake.lock` auf nixpkgs nixos-unstable @ 2026-06-10 aktualisiert
- `go.mod`: `go 1.24.11` → `go 1.26.0` (Mindestanforderung)
- `Dockerfile`: Build-Image `golang:1.24-alpine` → `golang:1.26.4-alpine` (enthält Stdlib-Fixes für GO-2026-5037 und GO-2026-5039)
- CI (`.github/workflows/ci.yml`): `go-version` 1.24 → 1.26.4 in allen Jobs
- CI: `golangci-lint-action` Version v2.7.2 → v2.12.2 (synchron zur Nix-Toolchain)
- Doku in `CLAUDE.md` und `README.dev.md` auf Go 1.26 aktualisiert

### Fixed
- `Makefile`-Target `fmt` formatiert nur noch Projektquellen (`cmd`, `internal`), nicht den GOMODCACHE-Pfad `.go/mod`
- `.golangci.yml`: `goconst` und `noctx` für Test-Dateien ausgeschlossen, `goconst` zusätzlich für `internal/api/openapi.go` (OpenAPI-Datenliteral)

## [0.1.1] - 2025-01-14

### Fixed
- golangci-lint v2 Konfigurationsformat korrigiert
- CI-Pipeline auf golangci-lint-action v7 aktualisiert

### Changed
- Claude Code lokale Einstellungen in .gitignore aufgenommen

## [0.1.0] - 2025-01-13

### Added
- Initiale Projektstruktur
- Go-Modul mit Abhängigkeiten (gorilla/mux, viper, prometheus)
- Konfiguration via CLI, Environment und .env
- GBIF-Client für Taxonomie-Abfragen
- In-Memory Cache mit TTL
- REST-API Endpoint `/api/v1/taxa/suggest`
- OpenAPI-Spezifikation (Code-first mit swaggo)
- Middleware-Chain: Request-ID, Logging, Rate-Limiting, Load-Shedding, Timeouts, CORS, Metrics
- Prometheus Metrics unter `/metrics`
- Dockerfile (Multi-Arch, Distroless)
- docker-compose für lokale Entwicklung
- GitHub Actions für CI/CD

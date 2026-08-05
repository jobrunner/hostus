# Bekannte Lücken und offene Schulden

Diese Seite führt Befunde, die **bewusst nicht behoben** wurden, dort auf,
wo die nächste Person sie findet — nicht in einem Prozess-Ledger, das beim
nächsten Teilprojekt niemand aufschlägt. Jeder Eintrag nennt den Befund, die
Auswirkung und den nächsten sinnvollen Schritt — also das, was die Lücke
wirklich schließen würde, nicht nur die Notiz, dass es sie gibt.

Behobene Punkte werden hier gelöscht, nicht abgehakt; der Verlauf steht im
`CHANGELOG.md`.

## Der `verbatim`-Einstieg von `/v1/translate` ist mit CDM praktisch tot (SP5)

`POST /v1/translate` bietet zwei Einstiege, `concept_id` und `verbatim`.
Der zweite löst am vollen Index praktisch nie auf: von 300 WCVP-Namen, die
nachweislich eine CDM-Gegenseite **mit** Relation haben, kamen **265 als
`UNRESOLVABLE`** zurück, 35 lösten auf das relationslose WCVP-Konzept auf,
**0** wurden übersetzt. `POST /v1/match` auf denselben 300 Namen zeigt die
Ursache eindeutig: **265× „Mehrdeutiger Treffer"**, **0×** „kein
eindeutiger Treffer".

Die Ursache ist die Bauart, nicht ein Defekt: Ein `sec.`-Raum trennt
Konzepte gleichen Namens, `MatchExact` sucht über **alle** Backbones, und
`Abies alba Mill.` ist deshalb acht CDM-Konzepte plus ein WCVP-Konzept —
neun gleich starke Treffer, bei denen die Auflösung korrekt nicht rät. Der
Endpunkt selbst ist in Ordnung: über `concept_id` liefert er in 200 von 200
Stichproben eine typisierte Antwort.

*Nächster Schritt:* Einen Backbone- oder `sec.`-Filter für die Auflösung
(etwa `entry_backbone`/`entry_sec` am Request), damit `verbatim` gegen
**einen** Namensraum auflösen kann statt gegen alle. Vorher messen, wie
viele der 265 damit eindeutig würden — die Zahl ist unbekannt, und ein
Filter, der die Mehrdeutigkeit nur verschiebt, wäre keine Verbesserung.
Bis dahin: `/v1/translate` mit `concept_id` benutzen, wie es
[die Anleitung](../how-to/sec-translate-uc6.md) jetzt sagt.

## `/v1/suggest` und `/v1/concept` geben kein `sec.`-Feld aus (SP5)

Seit CDM als zweiter Backbone ingestiert wird, liegen mehrere Konzepte
**desselben Namens** im Index — eines je Referenzwerk. Beide Endpunkte
geben aber kein `sec.`-Feld aus, sodass diese Treffer in der Antwort
**nicht unterscheidbar** sind:

```bash
curl -s "…/v1/suggest?q=Asteraceae&limit=5"
# {"concept_id":"cdm:concept:1785944e-…","display":"Asteraceae","canonical":"Asteraceae",
#  "rank":"FAMILY","status":"ACCEPTED","in_area":false,"score":-17.556039197970815}
# {"concept_id":"cdm:concept:302a66c9-…","display":"Asteraceae","canonical":"Asteraceae",
#  "rank":"FAMILY","status":"ACCEPTED","in_area":false,"score":-17.556039197970815}
```

Identische Anzeige, identischer Kanonischer, **identischer Score** — die
einzige Differenz ist die undurchsichtige UUID. Dasselbe gilt für
`GET /v1/concept/{id}`, dessen Antwort `backbone` nennt, aber nicht den
`sec.`-Bezug. Das trifft nicht nur Familien: 457 verschiedene
Familiennamen verteilen sich auf 629 Konzepte, und auf Artebene ist die
Vervielfachung weit größer (bis zu **zehn** CDM-Konzepte je Namensform).

*Nächster Schritt:* `sec` (id + title) in die Antwort beider Endpunkte
aufnehmen — `/v1/translate` führt das Feld bereits und zeigt die Form. Eine
Deduplikation je Namensform mit Vorzugsraum wäre die Alternative, verliert
aber Information, die UC6 ausdrücklich braucht.

## Kein Drift-Signal für das `nom_status`-Vokabular (SP6)

Die Regeltabelle in
[der HTTP-Referenz](../reference/http-api.md) ist gegen **einen gemessenen
Stand** des WCVP-Artefakts geschrieben: 1.304 distinkte Werte, 99.252
belegte Namen, 36 Token mit Trefferzahlen. `internal/domain/nomstatus_doc_test.go`
hält Code und Doku zusammen — aber **nichts merkt, wenn sich die Quelle
ändert**. Beim nächsten Artefakt-Bump kann WCVP neue Statuswerte einführen,
und die einzige Folge wäre, dass sie still in `unclassified` landen und
lautlos zurückgehalten werden.

*Nächster Schritt:* `hostus ingest` gibt die vier Urteilszahlen
(`absent`/`disqualifying`/`unclassified`/`acceptable`) plus die Top-N der
unklassifizierten Rohwerte aus — dann ist der heutige Stand
(872.270 / 89.836 / 2.309 / 347 über 964.762 Synonymzeilen) eine
Vergleichsbasis statt einer Momentaufnahme. Dieselbe Disziplin wie bei
`matched/unmatched/ambiguous`.

## `docs/how-to/offline-bundle.md` nennt eine überholte Bundle-Größe (SP6)

Die Größenangabe **108,9 MB** für das GER-Bundle stammt aus dem
Reality-Check M5.2 und ist seit SP6 Task 1 überholt: Der Task hat zwei
Spalten (`nom_status`, `rank_verbatim`) befüllt, was das Bundle um rund
50 MB wachsen lässt. Der Text argumentiert weiter gegen die alte Zahl.

*Nächster Schritt:* Bundle neu messen und Zahl plus Faktor-Aussage
aktualisieren — nicht schätzen.

## Der vierfach kopierte `TaxonRow`-Mapper (SP6)

Der Befund, der SP6 Task 1 ausgelöst hat: Eine Quellspalte wird an vier
Stellen auf dieselbe Struktur abgebildet, und Task 1 war genau die Reparatur
einer Stelle, an der eine davon vergessen worden war — geparst, dann von
einer Mapping-Zeile fallen gelassen, Spalte leer, niemand hat sich
beschwert. Der Hazard ist bislang nur im SDD-Ledger von SP6 festgehalten,
das SP7 nicht liest.

*Nächster Schritt:* die vier Abbildungen auf eine zusammenführen oder
mindestens einen Test, der für jede Quellspalte prüft, dass sie in der
Zieltabelle ankommt. Die Fehlerklasse ist „stiller Verlust", und die tritt
in diesem Projekt wiederholt auf.

## 316 Zeilen handgepflegtes OpenAPI ohne Drift-Prüfung (SP6)

`make doc-drift` prüft die OpenAPI-Baseline gegen die Doku, überspringt
aber genau die neu hinzugekommenen Pfade (`skip`-Zweige in
`scripts/doc-drift-check.sh`, solange es keine eingebettete Spec und keinen
Routen-Contract-Test gibt). Der Endpunktvertrag von SP5/SP6 ist damit
Handarbeit ohne Netz — die Projektregel „OpenAPI muss codegeneriert sein"
ist an dieser Stelle nicht eingelöst.

*Nächster Schritt:* Routen-Contract-Test aktivieren (Router-Routen gegen
die Spec-Pfade), damit die `skip`-Zweige entfallen.
## Mutationstest für `internal/adapters/telemetry` blockiert in CI nicht

**Stand:** 2026-08-03 · **Betrifft:** `.github/workflows/mutation.yml`

Das Paket meldet sein Ergebnis, lässt den Job aber nicht rot werden
(`continue-on-error` nur für diesen Matrix-Eintrag).

**Warum.** gremlins kompiliert das Paket je Mutant neu, und dieses eine zieht das
komplette OpenTelemetry-SDK. Ein einzelner Mutant auf einer Kapazitätsangabe
(`memory.go:173`, `make(map[string]string, rec.NumAttrs()+len(r.attrs))`) sprengt
die 7 GB eines `ubuntu-latest`-Runners: im Log ticken die Mutanten in ~1,75 s
durch, dann 60 s Stille, dann „the runner has received a shutdown signal" und
exit 143. Nacheinander erfolglos versucht: eigener Runner je Paket (Matrix),
`GOFLAGS=-p=1`, `GOMEMLIMIT=4GiB`, `--workers 1`. Jede Maßnahme half, keine
reichte.

**Das Paket ist nicht ungeprüft.** Lokal läuft es vollständig durch:
53 killed / 1 lived / 2 not covered, **98,15 % efficacy**. Der eine Überlebende
ist genau jene Kapazitätsangabe — ein äquivalenter Mutant, den kein Test töten
kann, weil `make(m, n)` nur die Vorab-Allokation betrifft.

**Was es wirklich lösen würde**, in absteigender Präferenz: einen Runner mit mehr
RAM (`ubuntu-latest-4-core` o. ä.); oder die Kapazitätsangabe streichen
(`make(map[string]string)`), was den Mutanten ersatzlos entfernt und messbar
nichts kostet — dieselbe Lösung, die SP6 für ein `1+len()+len()` gewählt hat.

**Bis dahin:** vor einer Änderung an `internal/adapters/telemetry` lokal
`make mutation PKG=./internal/adapters/telemetry` laufen lassen.

## `poc/` wird von `make verify` weder kompiliert noch gelintet

**Stand:** 2026-08-04 · **Betrifft:** `poc/go.mod`, `scripts/debt-guard.sh:39,65`

`poc/` ist ein eigenes Go-Modul (`github.com/jobrunner/hostus-poc`) ohne
`go.work`, und der Debt-Guard nimmt `^\./poc/` ausdrücklich aus. Damit
läuft über die Messharnesse **kein** Schritt von `make verify`: kein
`go build`, kein `go vet`, kein Linter, kein Test.

**Warum das zählt.** Die Harnesse unter `poc/measure/` sind keine
Wegwerfskripte — sie produzieren die Zahlen, auf denen Architektur-
entscheidungen beruhen (`docs/research/reality-check.md`,
`docs/research/suggest-quality.md`). SP7/Task 1 hat den strukturellen
Preis sichtbar gemacht: `poc/measure/suggestquality` verglich den
Konzeptstatus gegen `"accepted"`, während die Spalte `ACCEPTED` speichert.
Der Tiebreak war toter Code und fiel erst im Review auf, nicht in der CI.
Auf das Ergebnis wirkte es dort nicht — beim nächsten Mal kann es das.

**Was es lösen würde:** ein `go.work` mit `.` und `./poc`, oder ein
`verify`-Schritt, der `cd poc && go build ./... && go vet ./...` ausführt.
Das Lint-Regelwerk der Hexagon-Grenzen soll für `poc/` **nicht** gelten —
gefordert ist nur, dass der Code kompiliert und `vet`-sauber ist.

**Bis dahin:** nach einer Änderung an `poc/**` von Hand
`nix develop -c bash -c 'cd poc && gofmt -l . && go vet ./...'` laufen lassen.

## `/v1/concept/{id}/traits` liefert 500, während `/health/ready` 200 sagt (SP8)

**Stand:** 2026-08-04 · **Betrifft:** `internal/adapters/sqlite/traits.go:107`,
`internal/adapters/sqlite/db.go:63,67`

**Befund.** Beim Hand-Test der Testkonsole gegen den vollen Index
(`/tmp/full-real.sqlite`) antwortete `GET /v1/concept/{id}/traits` auf einem
Konzept mit fünf vorhandenen EIVE-Zeilen mit `500 INTERNAL_ERROR`. Ursache
ist eine fehlende Spalte:

```
sqlite> select resolution from trait_value limit 1;
Error: no such column: resolution
```

Die Abfrage in `traits.go:107` selektiert `tv.resolution` unbedingt, aber
`sqlite.Open` legt die Spalte auf einem älteren Index nicht an: es gibt
genau **zwei** Ad-hoc-Migrationen — `migrateXrefSourceColumn` und
`migrateConceptRelationPK` — und keine für `trait_value`. Jeder Index, der
vor SP3s `resolution`-Spalte gebaut wurde, liefert damit auf **jedem**
Konzept einen 500 auf `/traits`.

**Auswirkung.** Der Dienst startet, `/health/ready` meldet **200**, und erst
die erste echte Anfrage an einen von sieben Endpunkten scheitert — pro
Anfrage neu, still, mit einer Fehlermeldung, die die Ursache nicht nennt.
Ein Dienst, der sich bereit meldet und dann dauerhaft einen Endpunkt nicht
bedienen kann, ist schlechter als einer, der beim Start ehrlich abbricht:
die Bereitschaftsprüfung deckt Schema-Drift heute überhaupt nicht ab.

**Was es lösen würde.** Vorzugsweise ein **Schema-Check in `sqlite.Open`,
der laut scheitert** — die erwarteten Spalten je Tabelle gegen
`PRAGMA table_info` prüfen und mit einer Meldung abbrechen, die Tabelle,
Spalte und den nötigen Ingest/Migrationsschritt nennt. Ein fehlendes Stück
Schema ist ein Startproblem, kein Laufzeitproblem, und gehört dorthin, wo
es genau einmal auffällt. Eine dritte Ad-hoc-Migration analog
`migrateXrefSourceColumn` behebt zwar diesen einen Fall, lässt die
strukturelle Lücke (die nächste Spalte) aber offen — sinnvoll nur
zusätzlich, nicht statt dessen.

**Bis dahin:** gegen einen Index arbeiten, der mit dem aktuellen Ingest
gebaut wurde; oder die Spalte von Hand nachziehen
(`ALTER TABLE trait_value ADD COLUMN resolution TEXT`) — eine fehlende
`resolution` bedeutet „exakter Treffer" und ist als NULL korrekt.

## Kein Endpunkt listet die verfügbaren `sec.`-Referenzräume (SP8)

**Stand:** 2026-08-04 · **Betrifft:** `POST /v1/translate`,
`internal/adapters/http/assets/index.html`

**Befund.** `POST /v1/translate` verlangt ein `target_space`, aber keine API
sagt, welche Räume es gibt: `sec_reference` ist nirgends abfragbar. In der
Testkonsole ist das Feld deshalb ein **Freitextfeld** — man muss den Namen
kennen oder raten.

**Auswirkung.** Ein geratener Raum ist von einem leeren Ergebnis nicht zu
unterscheiden, ohne die Antwort genau zu lesen: ein unbekannter Zielraum
liefert `404 NOT_FOUND`, ein bekannter ohne Relation
`200 no_relation_recorded`. Beides ist für sich korrekt, aber wer den
Raumnamen nicht kennt, kann den Endpunkt praktisch nicht bedienen.

**Was es lösen würde:** ein `GET /v1/sec` (id + title, wie
`/v1/translate` das Feld bereits führt), oder `sec_reference` in der
Manifest-Antwort. Dann kann die Konsole eine Auswahlliste statt eines
Freitextfelds anbieten.

**Bis dahin:** die Räume aus dem Index lesen
(`select id, title from sec_reference`) und den gewünschten Wert eintippen.

## Das ESy-Regelwerk ist nicht geerntet — `esy_diagnostic_relevance` bleibt unentscheidbar (SP9)

**Stand:** 2026-08-05 · **Betrifft:** `POST /v1/match` mit `target_space`
(UC4), `esy_diagnostic_relevance`

**Befund.** UC4 verlangt pro Treffer eine ESy-diagnostische Relevanz. hostus
kann sie nicht bestimmen: Die FloraVeg-Pipeline hat nur eine **Namensliste**
aus `Life_form.xlsx` geerntet, nicht das **ESy-Expertensystem** selbst (das
Regelwerk, das entscheidet, welche Art in welcher Regel differenzierend ist).
SP3 hatte das Regelwerk bereits explizit ausgeklammert. Ohne die Regeln ist
„ist dieser Name in einer Regel differenzierend?" schlicht nicht
beantwortbar.

**Auswirkung.** Das Feld ist auf dem `target_space`-Pfad **immer present** mit
dem Sentinel `not_determinable` — bewusst kein `null` und nicht fehlend, damit
seine Abwesenheit nie als „nicht relevant" gelesen wird. Solange das Regelwerk
fehlt, kann hostus den vom Quelldokument als wichtigsten genannten Fall nicht
liefern: Ruht eine Regel auf einer im Feld nicht bestimmbaren Kleinart, ist die
richtige Antwort **„nicht entscheidbar", nicht „Habitat nicht erfüllt"** — und
genau diese Unterscheidung fällt derzeit aus.

**Was es lösen würde:** das ESy-Regelwerk beschaffen und ingestieren. Es liegt
auf **Zenodo unter CC BY 4.0** und ist damit — anders als die
floraveg.eu-Downloads — lizenzrechtlich unproblematisch. Zu prüfen wäre, ob es
maschinell in einen Regelvertrag überführbar ist (analog zur P8-Sondierung für
Wisskirchen), inklusive der Frage, gegen welchen Namensraum die Regeln
geschlüsselt sind und ob dieser mit dem hier ingestierten FloraVeg-Namensraum
zusammenfällt.

**Bis dahin:** `esy_diagnostic_relevance` als „hier nicht entscheidbar"
behandeln, nicht als negatives Urteil. `aggregate_policy` (baubar und
gemessen) ist die verwertbare Hälfte von UC4 — siehe
[SP9-Verdikt](../research/sp9-uc4-verdict.md).

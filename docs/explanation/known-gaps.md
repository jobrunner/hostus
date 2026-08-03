# Bekannte Lücken und offene Schulden

Diese Seite führt Befunde, die **bewusst nicht behoben** wurden, dort auf,
wo die nächste Person sie findet — nicht in einem Prozess-Ledger, das beim
nächsten Teilprojekt niemand aufschlägt. Jeder Eintrag nennt den Befund, die
Auswirkung und den nächsten sinnvollen Schritt — also das, was die Lücke
wirklich schließen würde, nicht nur die Notiz, dass es sie gibt.

Behobene Punkte werden hier gelöscht, nicht abgehakt; der Verlauf steht im
`CHANGELOG.md`.

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

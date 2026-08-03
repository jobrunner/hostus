# Bekannte Lücken und offene Schulden

Diese Seite führt Befunde, die **bewusst nicht behoben** wurden, dort auf,
wo die nächste Person sie findet — nicht in einem Prozess-Ledger, das beim
nächsten Teilprojekt niemand aufschlägt. Jeder Eintrag nennt den Befund, die
Auswirkung und den nächsten sinnvollen Schritt.

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

## Das Mutations-Gate hat keinen positiven Mutantenboden (SP6)

`make mutation` erzwingt `Not covered: 0`. Die Ausnahme „`No results to
report.` ist in Ordnung, **wenn `PKG` gesetzt ist**" trifft genau den Fall,
den die CI immer herstellt: `.github/workflows/mutation.yml` ruft
ausschließlich mit `PKG` auf. Ein Paket, das aus irgendeinem Grund keinen
einzigen Mutanten mehr erzeugt — falscher Pfad, kaputte Build-Tags,
umbenanntes Verzeichnis —, meldet damit grün.

*Nächster Schritt:* pro Paket eine erwartete Mindestzahl an Mutanten (ein
grober Boden, kein Ratchet) und Fehlschlag, wenn sie unterschritten wird.
Erst das macht die Ausnahme sicher.

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

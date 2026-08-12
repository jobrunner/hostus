# Bekannte Lücken und offene Schulden

Diese Seite führt Befunde, die **bewusst nicht behoben** wurden, dort auf,
wo die nächste Person sie findet — nicht in einem Prozess-Ledger, das beim
nächsten Teilprojekt niemand aufschlägt. Jeder Eintrag nennt den Befund, die
Auswirkung und den nächsten sinnvollen Schritt — also das, was die Lücke
wirklich schließen würde, nicht nur die Notiz, dass es sie gibt.

Behobene Punkte werden hier gelöscht, nicht abgehakt; der Verlauf steht im
`CHANGELOG.md`.

## `docs/how-to/offline-bundle.md` nennt eine überholte Bundle-Größe (SP6)

Die Größenangabe **108,9 MB** für das GER-Bundle stammt aus dem
Reality-Check M5.2 und ist seit SP6 Task 1 überholt: Der Task hat zwei
Spalten (`nom_status`, `rank_verbatim`) befüllt, was das Bundle um rund
50 MB wachsen lässt. Der Text argumentiert weiter gegen die alte Zahl.

*Nächster Schritt:* Bundle neu messen und Zahl plus Faktor-Aussage
aktualisieren — nicht schätzen.

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

**Stand:** 2026-08-12 · **Betrifft:** `.github/workflows/mutation.yml`

Das Paket meldet sein Mutationsergebnis, lässt den Job aber nicht rot werden
(`continue-on-error` nur für diesen Matrix-Eintrag), weil es auf dem 7-GB-
`ubuntu-latest`-Runner OOM-gekillt wird (exit 143): gremlins kompiliert das
Paket je Mutant neu, und dieses eine zieht das komplette OpenTelemetry-SDK.

**Was seit 2026-08-12 anders ist.** Die früher als Ursache vermutete
`make()`-Kapazitätsangabe (`RingLog.Handle`) wurde gestrichen
(`make(map[string]string)`) — das entfernte einen äquivalenten LIVED-Mutanten
und hebt die lokale Efficacy auf **100 % (56 killed / 0 lived / 0 not
covered)**. Aber der OOM blieb: PR #33 reproduzierte ihn in CI nach ~2 min
**ohne** diesen Mutanten. Der Engpass ist also die Neukompilierung des SDK je
Mutant, nicht eine einzelne Allokation.

**Was es wirklich lösen würde:** ein Runner mit mehr RAM
(`ubuntu-latest-4-core` o. ä.). Das Repo hat aktuell **keinen** größeren
Runner konfiguriert (alle Jobs auf `ubuntu-latest`), und größere GitHub-Runner
sind eine Org-/Kosten-Entscheidung — daher bleibt `continue-on-error`.

**Bis dahin:** vor einer Änderung an `internal/adapters/telemetry` lokal
`make mutation PKG=./internal/adapters/telemetry` laufen lassen (läuft dort
vollständig durch).

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

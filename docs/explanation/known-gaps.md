# Bekannte Lücken und offene Schulden

Diese Seite führt Befunde, die **bewusst nicht behoben** wurden, dort auf,
wo die nächste Person sie findet — nicht in einem Prozess-Ledger, das beim
nächsten Teilprojekt niemand aufschlägt. Jeder Eintrag nennt den Befund, die
Auswirkung und den nächsten sinnvollen Schritt — also das, was die Lücke
wirklich schließen würde, nicht nur die Notiz, dass es sie gibt.

Behobene Punkte werden hier gelöscht, nicht abgehakt; der Verlauf steht im
`CHANGELOG.md`.

## OpenAPI wird handgepflegt statt codegeneriert (SP6)

**Drift-Risiko: geschlossen (2026-08-12).** Der Routen-Contract-Test
`TestRoutesMatchOpenAPISpec` (`internal/adapters/http`) gleicht jede vom Router
gemountete Route beidseitig gegen einen Pfad+Methode in
`api/openapi/openapi.yaml` ab — eine neue Route ohne Spec-Eintrag (oder
umgekehrt) lässt CI rot werden. `scripts/doc-drift-check.sh` führt ihn als
Check 3 aus (der frühere `skip`-Zweig ist entfallen). Die handgepflegte Spec
kann damit **nicht mehr still von den Routen driften**.

**Schema-*Inhalt* ebenfalls gepinnt (2026-08-12).** `TestOpenAPISchemasMatchDTOs`
(`internal/adapters/http`) reflektiert über jeden Request/Response-DTO und
gleicht rekursiv jedes Component-Schema ab: Property-Namen, required-Status (Go
`omitempty` ⇔ Schema-`required`), Skalartypen, Array-Element- und Map-Wert-Typen
sowie die `$ref`/Inline-Objekt-Verschachtelung. Ein Feld, das umbenannt,
(nicht-)optional gemacht oder umtypisiert wird, ohne die Spec anzupassen, lässt
CI rot werden. doc-drift führt beide Tests in Check 3 aus.

**Weiter offen (bewusst zurückgestellt):** die Projektregel „OpenAPI muss
codegeneriert sein" ist damit nicht *wörtlich* eingelöst — die Spec wird
weiterhin von Hand geschrieben, nur jetzt in **Pfad+Methode UND Schema-Inhalt**
verifiziert. Volle Codegenerierung (Spec aus Routen/Handlern) hätte einen
bespoke Generator oder eine zusätzliche Abhängigkeit gebraucht, was mit der
„keine schweren Deps"-Regel kollidiert; sie bliebe ein eigener SP, falls über
die Drift-Sicherung hinaus gewünscht. Nicht geprüft (bewusst, kein Struktur-
Drift): Enum-Wertlisten, Descriptions/Examples und Format-Hinweise — Prosa, kein
Vertrag.

## `telemetry`- und `sqlite`-Mutation laufen nicht im Per-PR-Gate (7-GB-Runner-OOM)

**Stand:** 2026-08-12 · **Betrifft:** `.github/workflows/mutation.yml`

Zwei Pakete lassen sich auf dem 7-GB-`ubuntu-latest`-Runner nicht verlässlich
mutationstesten: `internal/adapters/telemetry` (zieht je Mutant das komplette
OpenTelemetry-SDK) und `internal/adapters/sqlite` (den `modernc.org/sqlite`-
Treiber). gremlins rekompiliert das Paket je Mutant, und der `go build`-
Subprozess spitzt dabei auf ~6+ GB RSS — Ergebnis: `exit 143` (OOM-Kill) oder
Thrashing bis zum 60-min-Timeout. Lokal laufen beide vollständig durch
(telemetry 56/0/0, sqlite `Not covered: 0`, 302/4).

**Was PR #35 geklärt hat.** Ein Swap-Headroom-Schritt behebt es **nicht**
deterministisch: telemetry wird trotz 12 GB Swap OOM-gekillt (der OTel-Compile-
Peak ist aktives Working-Set, das der OOM-Killer vor dem Auslagern trifft), und
sqlite thrasht auf Swap 15–60 min statt der ~154 s lokal. Deshalb sind beide
Pakete aus dem **Per-PR-Job** entfernt und laufen in einem separaten
`mutation-heavy`-Job **nur** auf wöchentlichem Cron/`workflow_dispatch` — so ist
das Per-PR-Gate schnell, deterministisch und vollständig blockierend, und kein
OOM kann je einen PR flaky machen. Auf dem Cron sind **beide** schweren Beine
report-only (`continue-on-error`): ein `workflow_dispatch`-Messlauf zeigte
sqlite mit Swap noch bei 15 min+ laufen, telemetry OOMt weiterhin — ein rotes
Ergebnis dort hieße „thrashte/Timeout", nicht „echte Regression", also wäre
Gating nur Alert-Fatigue. Das hochgeladene `mutation.log`-Artefakt trägt das
Ergebnis, wenn ein Bein doch durchläuft.

**Was es wirklich lösen würde:** ein Runner mit mehr RAM
(`ubuntu-latest-4-core`, 16 GB). Dann fielen beide Pakete zurück in die
Per-PR-Matrix und der ganze `mutation-heavy`-Job entfiele. Größere
GitHub-Runner sind eine Org-/Kosten-Entscheidung — daher die Zwei-Stufen-
Trennung als runnerneutrale Lösung.

**Bis dahin:** vor einer Änderung an `internal/adapters/telemetry` oder
`internal/adapters/sqlite` lokal `make mutation PKG=…` laufen lassen (läuft
dort vollständig durch) — das ist das eigentliche Entwickler-Gate für diese
beiden Pakete.

## Das ESy-Regelwerk ist nicht ingestiert — `esy_diagnostic_relevance` bleibt `not_determinable` (SP9)

**Stand:** 2026-08-05, Sondierung 2026-08-12 · **Betrifft:** `POST /v1/match`
mit `target_space` (UC4), `esy_diagnostic_relevance`

**Befund.** UC4 verlangt pro Treffer eine ESy-diagnostische Relevanz. hostus
kann sie nicht bestimmen: Die FloraVeg-Pipeline hat nur eine **Namensliste**
aus `Life_form.xlsx` geerntet, nicht das **ESy-Expertensystem** selbst (das
Regelwerk, das entscheidet, welche Art in welcher Regel differenzierend ist).
SP3 hatte das Regelwerk bereits explizit ausgeklammert. **Ohne die ingestierten
Regeln** ist „ist dieser Name in einer Regel differenzierend?" nicht
beantwortbar — mit ihnen ist es das (siehe Sondierung unten); es fehlt der
Ingest, nicht die grundsätzliche Beantwortbarkeit.

**Auswirkung.** Das Feld ist auf dem `target_space`-Pfad **immer present** mit
dem Sentinel `not_determinable` — bewusst kein `null` und nicht fehlend, damit
seine Abwesenheit nie als „nicht relevant" gelesen wird. Solange das Regelwerk
fehlt, kann hostus den vom Quelldokument als wichtigsten genannten Fall nicht
liefern: Ruht eine Regel auf einer im Feld nicht bestimmbaren Kleinart, ist die
richtige Antwort **„nicht entscheidbar", nicht „Habitat nicht erfüllt"** — und
genau diese Unterscheidung fällt derzeit aus.

**Beschaffungs-/Machbarkeitsfrage: sondiert und geklärt (2026-08-12).** Die
Sondierung [SP9-ESy-Spike](../research/sp9-esy-spike.md) hat alle drei offenen
Punkte beantwortet — das war der Blocker, nicht das Feature selbst:
- **Beschaffbar & Lizenz:** ein einzelner 1,6-MB-Textfile
  (`EUNIS-ESy-2020-06-08.txt`) auf **Zenodo, DOI 10.5281/zenodo.3841729,
  CC BY 4.0** — redistribuierbar, direkt per HTTP ladbar.
- **Maschinell parsbar:** eine formale Grammatik in vier Abschnitten
  (169 Artengruppen, 304 Habitat-Regeln), mit R-Referenzparser (Bruelheide et
  al. 2021) — ein Parser-Task, kein Reverse-Engineering.
- **Namensraum:** schlichte Binomiale, **66,4 % der ESy-Art-Namen liegen
  verbatim** im ingestierten FloraVeg-Namensraum (exakter Vergleich, also ein
  Boden; End-to-End bis zum WCVP-Konzept ≈ 57 %). Die nicht getroffenen ~34 %
  sind qualitativ Moose/Flechten (Scope) und Schreibvarianten; wie viele der
  SP3-Crosswalk auffinge, ist ungetestet und Sache des Ingest-SP.

**Wichtige Scope-Grenze aus dem Spike:** eine volle ESy-Klassifikation
(Aufnahme → Habitat) ist für einen Namensdienst **systembedingt unmöglich** —
alle 304 Regeln brauchen Deckungswerte, 168 zusätzlich Standortdaten, die
hostus (kennt einen Namen, keine Aufnahme) nicht hat. Die *beantwortbare*
Frage „ist dieser Name im Regelwerk eine diagnostische Art?" hängt aber allein
am Regelwerk.

**Was es jetzt noch lösen würde (eigener SP, nicht mehr Sondierung):** die
Datei als CC-BY-4.0-Quelle pinnen, SECTION 1–3 parsen, ESy-Arten per
SP3-Crosswalk auf Konzepte abbilden und `esy_diagnostic_relevance` dreiwertig
machen (`diagnostic`/`not_diagnostic`/`not_determinable`) — Umsetzungspfad im
Spike-Dokument.

**Bis dahin:** `esy_diagnostic_relevance` bleibt korrekt `not_determinable`
(nicht „Habitat nicht erfüllt"). `aggregate_policy` (baubar und gemessen) ist
die verwertbare Hälfte von UC4 — siehe
[SP9-Verdikt](../research/sp9-uc4-verdict.md).

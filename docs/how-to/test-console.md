# Die eingebettete Testkonsole benutzen

`hostus serve` liefert unter `/` eine Testkonsole aus — eine einzelne
HTML-Seite, die im Binary steckt. Sie ist da, damit man den Index **von Hand
beurteilen** kann: mit REST allein sieht man nicht, ob eine Trefferliste gut
ist.

## Was die Konsole nicht ist

Zuerst das, damit niemand sie für etwas anderes hält:

- **Kein Produkt-UI.** Sie ist ein Messinstrument. Es gibt keine
  Bedienerführung, keine Fehlerbehandlung für Endnutzer, keine
  Barrierefreiheits- oder Browserkompatibilitätszusage.
- **Nicht authentifiziert.** Es gibt keinen Login, keine Rollen, keine
  Sitzung. Wer die Seite erreicht, erreicht sie ganz.
- **Nicht für Exposition gehärtet.** Sie ist für `localhost` und ein
  vertrauenswürdiges Netz gebaut, nicht für das offene Internet. Es gibt
  kein CSRF-Token, keine Rate-Limit-Trennung von der API und keine
  Angriffsflächenanalyse.
- **Kein zweiter Zugang zu den Daten.** Sie spricht ausschließlich dieselbe
  öffentliche HTTP-API wie jeder andere Client. Was die Konsole sehen kann,
  kann `curl` auch sehen.

Sie ist **standardmäßig an**. Das ist eine bewusste Entscheidung für einen
lokal betriebenen, unauthentifizierten Forschungsdienst — und genau deshalb
steht hier ausdrücklich, wie man sie für ein Deployment abschaltet.

## Starten

```bash
hostus serve                       # Konsole an (Standard), http://localhost:8080/
```

Öffnen: `http://localhost:8080/`. Es gibt keinen Build-Schritt, kein
`npm install`, kein separates Frontend — die Seite ist einkompiliert.

## Den Schalter umlegen

Der Schlüssel heißt `ui.enabled` und folgt der normalen Prioritätsleiter
(Datei < Umgebung < Flag):

```yaml
# config.yaml — unterste Stufe
ui:
  enabled: false
```

```bash
HOSTUS_UI_ENABLED=false hostus serve   # mittlere Stufe: schlägt die Datei
hostus serve --ui=false                # oberste Stufe: schlägt die Umgebung
hostus serve --ui=true                 # …in beide Richtungen
```

`--ui` ist bewusst ein Wert-Flag: ohne `--ui=false` könnte die oberste Stufe
die Konsole nur ein-, aber nie ausschalten.

**Für ein Deployment:** `HOSTUS_UI_ENABLED=false` setzen (in
`docker-compose.yml` per `env_file:`/`environment:`) oder `--ui=false` an den
Startbefehl hängen. Ist die Konsole aus, registriert der Router **gar
nichts** unter `/`: weder `/` noch `/assets/*` noch ein Deep Link
antworten — alles ist 404. Die API (`/v1/*`, `/health/*`, `/metrics`,
`/openapi`) ist in beiden Schalterstellungen Byte für Byte identisch; ein
Integrationstest vergleicht sie über echtes HTTP.

Prüfen:

```bash
curl -s -o /dev/null -w '%{http_code}\n' http://localhost:8080/            # 404
curl -s -o /dev/null -w '%{http_code}\n' http://localhost:8080/health/live # 200
```

## Die vier Panels

| Panel | Ruft | Wofür es da ist |
| ----- | ---- | --------------- |
| **1 Suggest** | `GET /v1/suggest` beim Tippen | Beurteilen, ob die Trefferliste taugt. Zeigt je Treffer Rang, Akzeptanz, `in_area` und Score — und rechnet über der Liste aus, **wie viele Treffer überhaupt mit dem getippten Präfix beginnen** und wie der Rangmix aussieht. |
| **2 Konzept** | `GET /v1/concept/{id}` + `…/traits` + `…/synonyms?relevance=publication` | Einen Treffer aufklappen: Klassifikation, Xrefs (alle IDs je Autorität), Verbreitung, Synonyme, Indikatorwerte mit ihrer Skala. |
| **3 Match** | `POST /v1/match` | Eine Liste von Verbatim-Namen auflösen und je Zeile Einstufung, Konfidenz und `requires_review` sehen. |
| **4 Translate** | `POST /v1/translate` | Das gezeigte Konzept in einen `sec.`-Raum übersetzen. |

Ein Treffer in Panel 1 oder eine `concept_id` in Panel 3 führt per Klick nach
Panel 2; Panel 4 arbeitet mit dem dort gezeigten Konzept.

Jedes Panel schreibt die tatsächlich gerufene URL, den HTTP-Status und die
gemessene Dauer über das Ergebnis. **Nichts wird zwischengespeichert** —
jede Anzeige ist die Antwort, die der Dienst gerade gegeben hat.

Die Seite lädt **nichts** aus dem Netz: kein CDN, keine Web-Schrift, kein
externes Bild. Sie funktioniert in einem Feldeinsatz ohne Verbindung genauso
wie am Schreibtisch, und ein `Content-Security-Policy: default-src 'self'`
sorgt dafür, dass das so bleibt.

## Was du erwarten solltest

Die folgenden Beobachtungen sind **bekannte Eigenschaften des heutigen
Standes**, keine kaputte Installation. Sie stehen hier, damit niemand eine
Stunde mit der Suche nach einem Fehler verbringt, den es nicht gibt.

### Suggest liefert Treffer, die nicht mit dem Präfix beginnen

Tippt man `ca`, steht **_Kunzea capitata_ auf Platz 1**, und von 30
Treffern beginnt **kein einziger** mit „ca". Über die Präfixe `ac`, `ca`
und `al` zusammen sind es 17 von 90, und es sind **0 Gattungen auf 90
Plätzen**. Alle Treffer teilen denselben Score; gematcht wird das
Epitheton, nicht der Namensanfang, sodass die Reihenfolge faktisch das
Alphabet ist.

Die Konsole färbt genau deshalb jede Zeile rot, die nicht mit dem Präfix
beginnt, und schreibt die Quote und den Rangmix über die Liste. **Das rote
Panel ist das erwartete Bild, nicht der Fehler.** Details:
[Suggest-Qualität](../research/suggest-quality.md).

Rechne außerdem mit **Antwortzeiten um zwei Sekunden** bei zweistelligen
Präfixen gegen den vollen Index.

### Keine deutschen Trivialnamen

`vernacular_de` ist Teil der API, aber der Index enthält **null** deutsche
Trivialnamen. Das Feld fehlt deshalb in jeder Antwort (`omitempty`), und die
Konsole zeigt nirgends einen deutschen Namen an.

### `/v1/translate` antwortet für WCVP-Konzepte leer

Ein WCVP-Konzept trägt keine `concept_relation`-Zeile. Panel 4 antwortet
darum mit **„keine Relation erfasst"** — als Aussage, in einem grauen
Kasten, nicht als Fehler. Das ist das korrekte, gemessene Verhalten: ein
Name ist über `sec.`-Räume hinweg konstruktionsbedingt mehrdeutig, und ohne
erfasste Relation wäre jede Antwort geraten. Übersetzt wird nur zwischen
CDM-Konzepten mit erfasster Relation.

Ein **unbekannter** Zielraum ist etwas anderes: der antwortet `404` und wird
rot dargestellt. Das Feld `target_space` ist ein Freitextfeld, weil kein
Endpunkt die verfügbaren `sec.`-Räume auflistet — siehe
[Bekannte Lücken](../explanation/known-gaps.md).

### Gleichnamige Konzepte sind nicht unterscheidbar

**171 Familiennamen kommen in mehr als einem `sec.`-Raum vor.** Weder
`/v1/suggest` noch `/v1/concept` geben ein `sec.`-Feld aus, deshalb zeigt
die Konsole für sie identische Zeilen: gleicher Name, gleicher Rang,
gleicher Score — unterscheidbar nur an der UUID. Ebenfalls in
[Bekannte Lücken](../explanation/known-gaps.md).

### Ein Index ohne SP4/SP5-Ingest zeigt drei Panels nur halb

Wer gegen einen älteren Index arbeitet (etwa `/tmp/full-real.sqlite`), sieht
Folgendes — und keines davon ist ein Fehler der Konsole:

| Beobachtung | Ursache |
| ----------- | ------- |
| Xrefs zeigen nur `powo`, immer genau eine ID | nur ein Backbone ingestiert; die Wikidata-Brücke (SP4) fehlt, die mehrere IDs je Autorität liefert |
| Panel 4 antwortet auf **jeden** Zielraum mit 404 | `sec_reference` ist leer, es gibt keinen Zielraum |
| Panel 2 zeigt „Traits" als Fehler (HTTP 500) | dem Index fehlt `trait_value.resolution`, und es gibt dafür keine Migration — siehe [Bekannte Lücken](../explanation/known-gaps.md) |

### Ein Seitenaufruf kostet API-Budget

Die Konsole hängt in derselben Middleware-Kette wie die API und teilt sich
deren **globalen Token-Bucket von 20 rps**. Das ist Absicht: eine Konsole
außerhalb des Limiters würde eine gesunde Hülle über eine überlastete API
zeichnen. Deshalb ist die Seite *ein* Dokument mit eingebettetem CSS und JS
statt einem Dutzend Einzeldateien. Wer trotzdem sehr schnell hintereinander
lädt und dabei die API bedient, kann `429` sehen — das ist der Limiter, der
arbeitet.

## Verwandt

- [Konfiguration](../reference/configuration.md) — der Schlüssel `ui.enabled`
  und die Auslieferungsregeln im Detail
- [HTTP-API](../reference/http-api.md) — die Endpunkte, die die Panels rufen
- [Bekannte Lücken](../explanation/known-gaps.md)

# Spec: CORS-Preflight reparieren (POST-Endpunkte browser-tauglich machen)

Datum: 2026-09-14
Status: verabschiedet (User: „untersuche, ob wir CORS in Hostus haben. Das
benötigen wir." → Befund vorgelegt → „Ja, bitte. Spec, Plan, Umsetzung,
Review und PR-Merge und ein neues Bugfix-Release")

## Befund (gemessen, v3.4.0-alpha.0)

```
OPTIONS /v1/match
  Origin: https://habitatus.example
  Access-Control-Request-Method: POST
→ HTTP 405, KEIN einziger CORS-Header
```

`/v1/match` und `/v1/translate` sind damit aus einem Browser nicht
aufrufbar: ein JSON-Body erzwingt einen Preflight, und der scheitert.
`GET /v1/suggest` funktioniert cross-origin, weil es ohne Preflight
auskommt — deshalb blieb die Lücke unsichtbar.

Zwei unabhängige Ursachen:

1. **Der Preflight erreicht die Middleware nie.** `gorilla/mux` führt per
   `Use()` registrierte Middleware NUR für gematchte Routen aus. `OPTIONS`
   gegen eine als `.Methods(POST)` registrierte Route matcht nichts und
   landet im `MethodNotAllowedHandler` — außerhalb der Kette. Der Router
   umwickelt den `NotFoundHandler` bereits von Hand (`applyChain`), den
   `MethodNotAllowedHandler` aber nicht.
2. **`Access-Control-Allow-Methods` ist hart `"GET, OPTIONS"`**
   (`internal/middleware/cors.go:22`). Selbst mit behobenem Punkt 1 würde
   der Browser den POST ablehnen.

Nebenbefunde: `Vary: Origin` wird nur im Allowlist-Zweig gesetzt; Origins
werden nur exakt verglichen (keine Subdomain-Wildcards); es existiert **kein
einziger Test** für CORS (nur das Parsen der Config ist getestet).

## Entscheidungen

1. **CORS wird ein Wrapper UM den Router, nicht ein Glied der `Use`-Kette.**
   Das ist die einzige Position, an der ein Preflight ankommt, der keine
   Route matcht. `internal/middleware/cors.go` entfällt ersatzlos; der neue
   Code lebt in `internal/adapters/http/cors.go` — dort, wo der Router
   liegt (die Methoden-Ableitung braucht ihn) und wo das Mutation-Gate
   greift (`internal/adapters/http` ist in der CI-Matrix, `internal/
   middleware` nicht).
2. **`NewRouter` liefert künftig `http.Handler` statt `*mux.Router`.**
   `app.App.Router` ist bereits als `http.Handler` deklariert, und keiner
   der 89 Testaufrufe nutzt mux-spezifische API auf dem Ergebnis — die
   Signaturänderung ist damit quellkompatibel.
3. **Preflights laufen durch dieselbe Middleware-Kette.** Der Wrapper
   beantwortet einen echten Preflight nicht direkt, sondern über
   `applyChain(chain, <204-Handler>)`. Begründung: die Projekt-Invariante
   „alles ist observierbar" (Request-ID, Logging, Metriken, Spans) soll
   auch für Preflights gelten, und ein direkt beantworteter Preflight
   umginge zusätzlich Rate-Limiter und Load-Shedder — eine OPTIONS-Flut
   wäre sonst ein Bypass der DoS-Schutzmaßnahmen aus Spec B4.
4. **`Access-Control-Allow-Methods` wird aus der Routen-Tabelle
   abgeleitet**, nicht handgeschrieben: Der Wrapper fragt den Router per
   `router.Match` mit dem angefragten Verfahren; `mux` meldet einen Pfad,
   der nur unter einem anderen Verfahren existiert, über
   `RouteMatch.MatchErr == ErrMethodMismatch`. Geantwortet wird
   `<Methode>, OPTIONS`. Eine handgeschriebene Liste ist exakt so lange
   korrekt, bis jemand eine Route hinzufügt — nichts schlägt dabei fehl,
   der Endpunkt ist nur stumm unbrauchbar.
5. **Ein echter Preflight ist OPTIONS + `Origin` + `Access-Control-Request-
   Method`.** Ein blankes `OPTIONS` (ohne beides) fällt unverändert an den
   Router durch, damit das Einschalten von CORS nicht jedes OPTIONS still
   in ein 204 verwandelt und ein 405 ein 405 bleibt.
6. **Der Default bleibt `*`.** Ist `cors.allowed_origins` leer, antwortet
   der Dienst wie bisher mit `Access-Control-Allow-Origin: *`. Das ist die
   dokumentierte Bestandsschnittstelle (`example.env`), und hostus ist
   read-only und ohne Authentifizierung. **Bedingung:**
   `Access-Control-Allow-Credentials` wird nie gesetzt — ohne dieses Feld
   sendet ein Browser weder Cookies noch Auth-Header, und der Wildcard
   bleibt ungefährlich. Ein Wechsel auf „aus, solange nicht konfiguriert"
   wäre ein Breaking Change ohne Anlass.
7. **Origin-Allowlist mit scheme/port-genauen Wildcards.** Zusätzlich zum
   exakten Vergleich wird `https://*.example.com` unterstützt: nur das
   Host-Label ist Platzhalter, Schema und Port müssen exakt passen. Ein
   Muster darf also NICHT `http://sub.example.com` (Klartext) oder
   `https://sub.example.com:8443` (anderer Dienst) zulassen; die bare
   Domain (`example.com`) ist nicht abgedeckt, `evil-example.com`
   ebensowenig.
8. **`Vary: Origin` bei jeder Anfrage mit `Origin`-Header** — auch bei
   abgelehnter Herkunft. Sonst kann ein Shared Cache die Antwort eines
   Origins an ein anderes ausliefern. Bei Wildcard-Betrieb entfällt es,
   weil die Antwort dann nicht von der Herkunft abhängt.
9. **Preflight-Status ist 204, unabhängig davon, ob die Herkunft erlaubt
   ist.** Ohne den `Allow-Origin`-Header verwirft der Browser die Antwort
   ohnehin; eine einheitliche Antwort verrät nicht, welche Origins
   konfiguriert sind.
10. **`Access-Control-Allow-Headers` bleibt `Accept, Content-Type,
    X-Request-ID`** (Bestand, `X-Request-ID` ist hostus-spezifisch),
    `Max-Age` bleibt `86400`.

## Nicht in diesem Scope

Authentifizierung, `Allow-Credentials`, konfigurierbare Header-/Methodenlisten
(beides ergibt sich aus Routen bzw. Bestand), CSP und andere
Sicherheits-Header.

## Projektweite Anforderungen

- `internal/adapters/http` ist mutation-gated (CI-Matrix) — neue Zweige
  brauchen Tests, die die Mutanten töten; `Not covered: 0`.
- Ein **Fitness-Test** pinnt den Regressionsfall: `OPTIONS` gegen die
  POST-Route `/v1/match` mit `Origin` + `Access-Control-Request-Method`
  MUSS `204` plus `Access-Control-Allow-Origin` und ein `Allow-Methods`
  liefern, das `POST` enthält. Dieser Test muss gegen den ALTEN Code
  fehlschlagen (Kontroll-Assertion wie bei den EXPLAIN-Plan-Tests).
- Antworten ohne `Origin`-Header bleiben byte-identisch zu heute, abgesehen
  von den bereits heute gesetzten CORS-Headern.
- OpenAPI ist codegeneriert; CHANGELOG unter `## [Unreleased]`;
  Conventional Commits; englische WHY-Kommentare.
- Abschluss: PR mit grüner CI mergen, danach das Bugfix-Release
  (release-please) durchziehen.

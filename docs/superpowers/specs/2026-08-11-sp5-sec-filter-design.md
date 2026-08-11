# SP5 — `sec.`-Auflösungsfilter und `sec`-Ausgabe: Design

**Datum:** 2026-08-11
**Status:** Entwurf, vom Owner freigegeben (Scope: beide Komponenten in einem Spec/Plan)

## Problem

Seit CDM als zweiter Backbone ingestiert ist, liegen im Index viele Konzepte
**desselben Namens** — eines je `sec.`-Referenzraum (bis zu ~119 CDM-Räume
plus WCVP). `Repository.MatchExact(canon)` sucht über **alle** Backbones/Räume
ohne Filter, sodass ein Allerweltsname wie *Abies alba* auf 1 WCVP- + 8
CDM-Konzepte fällt — neun gleich starke Kandidaten. Die Auflösung rät korrekt
nicht und meldet **mehrdeutig/`unresolvable`**. Zwei Folgen, beide gemessen:

- **`/v1/match` (inkl. `target_space`, UC4):** gängige Namen lösen nicht
  eindeutig auf, also erscheint kein `target_space_name` — die
  Namensraum-Integration (floraveg/eurosl) ist über die API kaum bedienbar.
- **`/v1/translate`-`verbatim` (UC6):** von 300 Namen mit nachweislicher
  CDM-Gegenseite kamen **265 `UNRESOLVABLE`**, 0 übersetzt (siehe
  `docs/explanation/known-gaps.md`).

Zusätzlich sind gleichnamige Treffer in `/v1/suggest` und `/v1/concept`
**nicht unterscheidbar**: beide Endpunkte geben kein `sec.`-Feld aus (nur eine
undurchsichtige UUID trennt zwei identisch angezeigte *Asteraceae*).

## Ziel

Gleichnamige Konzepte **eindeutig auflösbar** (Komponente A) und in der
**Ausgabe unterscheidbar** (Komponente B) machen. Beides in einem Spec/Plan.

## Nicht-Ziele

- Kein `sec.`-Filter für `/v1/suggest`-Ranking (nur `sec`-Ausgabe dort).
- Keine automatische Dedup/Vorzugsraum-Heuristik (verliert Information, die
  UC6 braucht — bewusst verworfen).
- Keine Änderung der Klassifikationslogik (`classify`, exact/exact_author/
  aggregate/fuzzy) — der Filter wirkt **vor** ihr.

## Komponente A — Auflösungsfilter

### Request

Zwei optionale, komponierbare Felder:

- `entry_backbone` — eine ingestierte Backbone-id (`wcvp` | `cdm` | `colxr`).
- `entry_sec` — eine `sec_reference`-id (impliziert ein sec-tragendes
  Backbone, d. h. CDM).

Auf:

- **`POST /v1/match`** — top-level im Request-Body, gilt für **alle** `names`
  des Batches. Ersetzt das aktuell tote Feld `sec_hint` (wurde entgegen-
  genommen, nie ausgewertet; wird entfernt — unbekannte JSON-Felder werden vom
  Decoder ohnehin ignoriert, also kein Bruch für Altclients).
- **`POST /v1/translate`** — im Request-Body, wirkt nur auf den
  `verbatim`-Einstieg; bei `concept_id` ignoriert (mit `concept_id` ist die
  Auflösung schon eindeutig).

### Semantik

Nach `MatchExact(canon)` werden die Kandidaten im **Application-Layer**
gefiltert:

```
behalte Kandidat c  ⇔  (entry_backbone == "" ∨ c.Concept.BackboneID == entry_backbone)
                     ∧  (entry_sec      == "" ∨ c.Concept.SecReference == entry_sec)
```

`MatchCandidate.Concept` trägt `BackboneID` und `SecReference` bereits, daher
bleibt der Port `Repository.MatchExact` **unverändert** (Alternative — ein
Filter-Argument am Port — wäre minimal effizienter, aber mehr Fläche und
Risiko; verworfen). Anschließend klassifiziert `classify` wie heute über die
verbleibenden Kandidaten.

- **Ohne beide Felder: byteweise das heutige Verhalten** (per Test gepinnt —
  `/v1/match` teilen sich UC3/UC4/UC6).
- `entry_sec` gesetzt ⇒ Kandidaten mit leerem `SecReference` (z. B. WCVP)
  fallen raus. `entry_backbone` und `entry_sec` verknüpfen mit **UND**.
- Komposition mit UC4: `entry_backbone=wcvp` + `target_space=floraveg` löst
  eindeutig auf **ein** WCVP-Konzept auf **und** hängt den ESy-Namen an — der
  produktive Kernfall.

### Validierung / Fehler

- Unbekannter `entry_backbone` (keine ingestierte Backbone-id) → `400
  INVALID_QUERY`, benennt den Wert (konsistent mit `target_space` /
  `application.ErrUnknownTargetSpace`).
- Unbekannter `entry_sec` (nicht in `sec_reference`) → `400 INVALID_QUERY`,
  benennt den Wert.
- Die Prüfung läuft vorab gegen `repo.BackboneVersions` bzw. `sec_reference`,
  bevor irgendein Name aufgelöst wird (kein Teil-Arbeiten bei ungültigem
  Filter).

## Komponente B — `sec` in der Ausgabe

- **`GET /v1/concept/{id}`** und **`GET /v1/suggest`** bekommen je Konzept/
  Treffer ein optionales `sec` `{ "id": ..., "title": ... }`, gefüllt **nur**
  wenn `Concept.SecReference` gesetzt ist (also für CDM-Konzepte), sonst
  weggelassen (`omitempty`).
- Form: dieselbe `secReferenceDTO` `{id, title}`, die `/v1/translate` bereits
  ausliefert (kein neuer Typ).
- Lese-Pfad: kleiner Join `sec_reference` (id → title). Für `/v1/concept` liegt
  die `SecReference`-id am Konzept vor; `title` per Join. Für `/v1/suggest`
  liefert die Suggest-Query je Treffer zusätzlich `sec_reference` id+title.
- **Ohne CDM-`sec.`** (reiner WCVP-Index) ändert sich die Ausgabe nicht (Feld
  fehlt) — byteweise wie heute; per Test gepinnt.

## Kritische Auflage: erst messen, dann glauben

Die known-gap warnt ausdrücklich: **ein Filter, der Mehrdeutigkeit nur
verschiebt, ist keine Verbesserung.** Daher ist ein Mess-Schritt Teil des
Plans und ein Erfolgskriterium, kein Zusatz:

- Auf der 300-Namen-CDM-Stichprobe (die mit nachweislicher Gegenseite, aus der
  known-gap) messen: wie viele der bisher **265 mehrdeutigen** werden mit
  gesetztem `entry_sec` (Quellraum) **eindeutig**, und wie viele davon
  `translate` dann tatsächlich übersetzt (0 → N).
- Analog für `entry_backbone=wcvp`: Anteil gängiger Namen, die dadurch
  eindeutig auf ihr WCVP-Konzept auflösen (und damit `target_space` bedienbar
  machen).
- Ergebnis nach `docs/research/` schreiben; ist die Verbesserung nicht
  messbar, gilt das Design als nicht gehalten.

## Betroffene Dateien (Orientierung, nicht bindend)

- `internal/application/match.go` — Filter-Struct + Anwendung nach
  `MatchExact`, vor `classify`; Validierung (neben `ErrUnknownTargetSpace`).
- `internal/adapters/http/taxa.go` — `matchRequestDTO`: `sec_hint` → `entry_backbone`/`entry_sec`; Weiterreichen + Fehlermapping.
- `internal/adapters/http/translate.go` — `translateRequestDTO` um
  `entry_backbone`/`entry_sec`; nur verbatim-Pfad.
- `internal/adapters/http/suggest.go`, `taxa.go` (concept), zugehörige
  Repository-Leseabfragen — `sec` `{id,title}` in der Ausgabe.
- `api/openapi/openapi.yaml`, `docs/reference/http-api.md` — beide Felder + das
  `sec`-Ausgabefeld dokumentieren.
- `docs/research/` — Messung; `docs/explanation/known-gaps.md` — die zwei
  SP5-Einträge entfernen (behoben), Verlauf ins CHANGELOG.

## Teststrategie

- **Byte-identisch ohne Filter** (`/v1/match`, `/v1/suggest`, `/v1/concept`) —
  gepinnt, weil mehrere UCs den Pfad teilen.
- `entry_backbone=wcvp` macht einen über CDM vervielfachten Namen eindeutig →
  ein WCVP-Konzept, `exact`.
- `entry_sec=<uuid>` löst in genau **einem** Referenzraum auf.
- Unbekannter `entry_backbone` / `entry_sec` → `400 INVALID_QUERY`, benennt ihn.
- Komposition `entry_backbone=wcvp` + `target_space=floraveg` → auflösen **und**
  ESy-Name anhängen.
- `/v1/translate`-`verbatim` + `entry_sec` → übersetzt (die 0→N-Verbesserung),
  über echte CDM-Relationen.
- `sec`-Feld in `/v1/suggest` und `/v1/concept`: present für ein CDM-Konzept,
  absent für ein WCVP-Konzept.
- **e2e** über den realen Multi-Backbone-Index (WCVP + CDM), mit spezifischen
  Namen/Räumen — nicht nur 200.
- Mutation-grün auf berührten Paketen; Lint inkl. `_test.go`.

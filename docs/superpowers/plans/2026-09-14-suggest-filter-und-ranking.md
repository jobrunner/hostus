# Suggest-Filter und -Ranking Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Wer „Inula hirta" tippt und eurosl als Namensraum wählt, bekommt
die passende Art als **erste** Zeile — und sieht an der Autorschaft, warum
die zweite Zeile ebenfalls da ist.

**Architecture:** Die Reihenfolge entsteht weiterhin in
`domain.RankSuggestions`; der SQLite-Adapter liefert die dafür nötigen
Signale (`ExactHit`, `TargetSpaceHit`, echter `PrefixHit`, `MatchedName`).
Der neue Namensraum-Filter greift IM Hauptquery vor dem Limit, nicht im
Nachlade-Pass.

**Tech Stack:** Go 1.26, modernc.org/sqlite (FTS5), gorilla/mux, Vanilla-JS-Konsole.

**Spec:** `docs/superpowers/specs/2026-09-14-suggest-filter-und-ranking.md`

## Global Constraints

- Nur erlaubte Bibliotheken; keine neue Dependency.
- Mutation-Gates: `internal/domain`, `internal/adapters/http`, `cmd/hostus` (CI) und **lokal** `internal/adapters/sqlite` (in CI nur report-only) — überall `Not covered: 0`.
- Jede Ranking-Änderung braucht eine **Kontroll-Assertion**: der Test muss gegen den alten Code fehlschlagen; den beobachteten Fehlschlag wörtlich in den Report.
- Antworten ohne die neuen Parameter bleiben byte-identisch, abgesehen vom additiven `matched_name`.
- SQLite-Planner-Falle: Bei jedem neuen `spalte = ?` in einer Query mit FTS-Treffern oder ID-Liste das unäre `+`-Idiom prüfen (`+space`, `+tc.backbone_id`) und per EXPLAIN-Regressionstest mit Kontroll-Assertion pinnen. Gilt nur für TEXT-Spalten (das Idiom hebt die Spalten-Affinität auf).
- Latenz gegen die echte DB messen (vorher/nachher) und im Report nennen; Messungen nur im Leerlauf, nicht während ein Mutationslauf die Maschine belegt.
- **NIEMALS `git add -A` oder `git add .`** — im Arbeitsbaum liegen unversionierte Dateien des Nutzers (`out/` mit mehreren GB, `console-*.png`, `.playwright-mcp/`, `build-full.yaml`). Nur die in den Tasks genannten Dateien namentlich adden.
- Testdatenbank für manuelle Messungen: `out/hostus-deploy-v3.4.0-alpha.0.sqlite` (nur lesen, niemals überschreiben).

## File Structure

- `internal/domain/suggest.go` — `SuggestItem`-Felder + `RankSuggestions`.
- `internal/adapters/sqlite/suggest.go` — Hauptquery (exact/prefix/Namensraum-Filter), `attachMatchedNames`, `TargetSpaceHit`.
- `internal/ports/output/repository.go` — `SuggestOpts.RequireTargetSpace`.
- `internal/application/suggest.go` — Durchreichen + Gebietsvalidierung.
- `internal/adapters/http/suggest.go` + beide `openapi.yaml` — Parameter und DTO.
- `internal/adapters/http/assets/{index.html,app.js}` — Konsole.
- `docs/reference/http-api.md`, `CHANGELOG.md`.

---

### Task 1: Ranking-Signale im Domain-Modell

**Files:**
- Modify: `internal/domain/suggest.go`
- Test: `internal/domain/suggest_test.go` (bestehende Datei erweitern)

**Interfaces:**
- Produces: `SuggestItem.ExactHit bool`, `SuggestItem.TargetSpaceHit bool`, `SuggestItem.MatchedName MatchedName`; `RankSuggestions` mit zwei neuen führenden Kriterien.
- `type MatchedName struct { Canonical, Authorship, Role string }` — `Role` ist `"accepted"` oder `"synonym"`, leere `Canonical` heißt „nicht ermittelt".

- [ ] **Step 1: Failing test für die neue Reihenfolge**

Der Test bildet den gemessenen Realfall ab (Homonym „Inula hirta"):

```go
// TestRankSuggestions_TargetSpaceHitBeatsBetterScore pins the measured
// Inula-hirta case: both concepts carry the queried name (Inula hirta L.
// for Pentanema hirtum, Inula hirta Pollich for P. britannica), so the
// exact-hit key ties and the target-space key decides. Before this key
// existed, bm25 alone ordered them and put britannica — whose eurosl name
// is "Inula britannica", i.e. NOT what was typed — first.
func TestRankSuggestions_TargetSpaceHitBeatsBetterScore(t *testing.T) {
	britannica := SuggestItem{
		ConceptID: "wcvp:concept:3217682", Canonical: "Pentanema britannica",
		Rank: RankSpecies, Status: StatusAccepted,
		ExactHit: true, TargetSpaceHit: false, PrefixHit: true, Score: 1.0,
	}
	hirtum := SuggestItem{
		ConceptID: "wcvp:concept:3217689", Canonical: "Pentanema hirtum",
		Rank: RankSpecies, Status: StatusAccepted,
		ExactHit: true, TargetSpaceHit: true, PrefixHit: true, Score: 2.0,
	}

	got := RankSuggestions([]SuggestItem{britannica, hirtum})

	if got[0].ConceptID != hirtum.ConceptID {
		t.Fatalf("first = %q, want the target-space hit %q", got[0].ConceptID, hirtum.ConceptID)
	}
}
```

Dazu ein zweiter Test, der das Exakt-Kriterium isoliert (`ExactHit` true vs.
false bei sonst gleichen Feldern, schlechterer Score beim exakten Treffer),
und ein dritter, der belegt, dass die BESTEHENDE Ordnung unverändert gilt,
wenn beide neuen Felder gleich sind (`InArea`, dann `accepted`, dann Rang,
dann Score) — sonst merkt niemand, wenn die alten Kriterien verrutschen.

- [ ] **Step 2: Fehlschlag bestätigen**

Run: `go test ./internal/domain/ -run TestRankSuggestions -v`
Expected: FAIL (`first = "wcvp:concept:3217682"`). Wörtlich in den Report.

- [ ] **Step 3: Felder und Kriterien ergänzen**

In `SuggestItem` die drei Felder mit WHY-Kommentaren ergänzen; in
`RankSuggestions` die zwei Vergleiche VOR `PrefixHit` einhängen und den
Doc-Kommentar (die nummerierte Liste!) auf sieben Kriterien fortschreiben.
`TargetSpaceHit` ist ohne angefragten `target_space` für jede Zeile false
und damit wirkungslos — das gehört als Satz in den Doc-Kommentar, sonst
liest es sich wie ein stiller Sonderfall.

- [ ] **Step 4: Tests grün + Mutation**

Run: `go test ./internal/domain/ && make mutation PKG=./internal/domain`
Expected: grün, `Not covered: 0`.

- [ ] **Step 5: Commit**

```bash
git add internal/domain/suggest.go internal/domain/suggest_test.go
git commit -m "feat(domain): Exakt- und Zielraum-Treffer als führende Suggest-Ranking-Kriterien"
```

---

### Task 2: Signale und Namensraum-Filter im SQLite-Adapter

**Files:**
- Modify: `internal/adapters/sqlite/suggest.go`
- Modify: `internal/ports/output/repository.go`
- Test: `internal/adapters/sqlite/suggest_internal_test.go`, `internal/adapters/sqlite/suggest_plan_internal_test.go`

**Interfaces:**
- Consumes: Task 1s `SuggestItem`-Felder.
- Produces: `output.SuggestOpts.RequireTargetSpace bool`; der Adapter füllt `ExactHit`, `PrefixHit`, `TargetSpaceHit`, `MatchedName`.

- [ ] **Step 1: `RequireTargetSpace` im Port**

Feld in `SuggestOpts` mit Doc-Kommentar: wirkt nur zusammen mit
`TargetSpace`; filtert IM Query vor dem Limit — mit derselben Begründung,
die schon bei `Backbone` steht (nachträgliches Filtern würde die Seite
leerräumen).

- [ ] **Step 2: Failing tests**

Drei Tests gegen eine Testdatenbank im Stil der vorhandenen
`suggest_internal_test.go`-Fixtures:
- `TestSuggest_RequireTargetSpaceDropsConceptsWithoutEntry` — Konzept ohne
  Eintrag im Raum verschwindet, eines mit Eintrag bleibt.
- `TestSuggest_ExactHitIsSetOnlyForFullNameEquality` — Anfrage „Inula hirta"
  setzt `ExactHit` beim Konzept mit genau diesem Namen, nicht beim Konzept,
  das nur „Inula hirtella" trägt.
- `TestSuggest_MatchedNameCarriesAuthorshipOfTheHit` — der getroffene
  Synonym-Name kommt mit Autorschaft und `Role: "synonym"` zurück, nicht
  der akzeptierte Name des Konzepts.

- [ ] **Step 3: Hauptquery erweitern**

`exact_hit` als korrelierte EXISTS-Spalte dort ergänzen, wo bereits
`nameStartFilter` sitzt (gleiche Form, gleiche Kosten), und als FÜHRENDEN
`ORDER BY`-Schlüssel setzen — sonst kann das Fetch-Budget den exakten
Treffer abschneiden, bevor das Ranking ihn je sieht:

```sql
ORDER BY exact_hit DESC, in_area DESC, score ASC
```

`prefix_hit` analog als echte Spalte (`canonical_fold LIKE ? || '%'`) statt
der Konstante in `scanSuggestItem`.

Den Namensraum-Filter als EXISTS auf `name_space_entry` in denselben
`WHERE`-Block, **mit dem `+`-Idiom** am Raum-Vergleich:

```sql
AND EXISTS (
  SELECT 1 FROM name_space_entry nse
  WHERE nse.concept_id = tc.id AND +nse.space = ?
)
```

- [ ] **Step 4: EXPLAIN-Regressionstest mit Kontroll-Assertion**

Im Stil von `TestAttachTargetSpaceNamesQueryPlanDoesNotScanSpace`: Der Plan
der Query MIT `+` darf den Space-Gleichheitsindex nicht als Treiber wählen;
die Variante OHNE `+` MUSS ihn im Plan zeigen (`t.Fatal`, wenn nicht) —
sonst pinnt der Test nichts.

- [ ] **Step 5: `attachMatchedNames`**

Zweiter Pass analog `attachTargetSpaceNames` (ID-Liste als JSON, ein Query,
Query als Paket-Konstante für den EXPLAIN-Test). Auswahlregel bei mehreren
Namen desselben Konzepts, in dieser Reihenfolge: exakte Gleichheit vor
Präfix, dann `accepted` vor `synonym`, dann `canonical` alphabetisch —
deterministisch, damit die Anzeige nicht zwischen zwei Aufrufen springt.
Die Regel als Kommentar begründen.

- [ ] **Step 6: `TargetSpaceHit` setzen**

Direkt nach `attachTargetSpaceNames`: true, wenn der ermittelte
`TargetSpaceName` kanonisiert der kanonisierten Anfrage gleicht oder mit ihr
beginnt. `domain.Canonicalize` benutzen, nicht selbst normalisieren.

- [ ] **Step 7: Tests, Gates, Latenzmessung**

```bash
go test ./internal/adapters/sqlite/
make mutation PKG=./internal/adapters/sqlite    # lokales Gate, Not covered: 0
```

Latenz vorher/nachher gegen `out/hostus-deploy-v3.4.0-alpha.0.sqlite`, je
dreimal im Leerlauf, für `q=Inula hir` und für ein kurzes `q=ca` mit
`limit=30`, mit und ohne `require_target_space`. Zahlen in den Report.

- [ ] **Step 8: Commit**

```bash
git add internal/adapters/sqlite/suggest.go internal/ports/output/repository.go \
        internal/adapters/sqlite/suggest_internal_test.go \
        internal/adapters/sqlite/suggest_plan_internal_test.go
git commit -m "feat(sqlite): Exakt-/Zielraum-Signale, Treffer-Name und Namensraum-Filter für Suggest"
```

---

### Task 3: HTTP-Parameter, Gebietsvalidierung, DTO

**Files:**
- Modify: `internal/adapters/http/suggest.go`
- Modify: `internal/application/suggest.go`
- Modify: `api/openapi/openapi.yaml` UND `internal/adapters/http/openapi.yaml` (identisch halten!)
- Modify: `docs/reference/http-api.md`
- Test: `internal/adapters/http/suggest_test.go`

**Interfaces:**
- Consumes: `output.SuggestOpts.RequireTargetSpace`, `domain.MatchedName`.
- Produces: Query-Parameter `require_target_space`; DTO-Feld `matched_name` (`{canonical, authorship, role}`, `omitempty`).

- [ ] **Step 1: Failing tests**

- `TestSuggest_RequireTargetSpaceWithoutTargetSpaceIs400` — `require_target_space=true` allein → `400 INVALID_QUERY`, Meldung nennt beide Parameter.
- `TestSuggest_UnknownAreaIs400` — `area=QUATSCH` → `400 INVALID_QUERY` mit dem Wert im Text (heute: 200 mit ungefilterten Treffern).
- `TestSuggest_KnownAreaAliasStillWorks` — ein dokumentierter Alias (z. B. `DE`) bleibt gültig; sonst bricht Schritt 2 die Konsole.
- `TestSuggest_MatchedNameIsRendered` — `matched_name.authorship` steht in der Antwort.
- `TestSuggest_ResponseUnchangedWithoutNewParams` — ohne die neuen Parameter ist der JSON-Körper identisch zum Bestand (bis auf `matched_name`).

- [ ] **Step 2: `require_target_space` parsen**

`strconv.ParseBool` auf den Rohwert; ein unparsbarer Wert ist `400
INVALID_QUERY` (nicht stillschweigend false — genau diese Klasse stiller
Nicht-Wirkung behebt der Branch). Fehlt der Parameter, bleibt es bei false.

- [ ] **Step 3: Gebietsvalidierung in der Application-Schicht**

Neben `validateBackbone`/`validateTargetSpace` ein `validateArea`, das
`repo.Areas(ctx)` nutzt und zusätzlich die dokumentierten Aliase zulässt.
**Wichtig:** Die Alias-Tabelle liegt heute im SQLite-Adapter
(`wgsrpdAlias`). Die Application darf nicht in den Adapter greifen
(depguard!). Auflösung: Der Adapter meldet seine Aliase über den
bestehenden `Areas`-Port mit (oder die Alias-Auflösung wandert nach
`domain`) — entscheide beim Implementieren, welcher der beiden Wege
weniger Fläche anfasst, und begründe die Wahl im Report. `make arch` ist
die harte Grenze.

- [ ] **Step 4: DTO + OpenAPI**

`matched_name` als verschachteltes Objekt; beide `openapi.yaml`-Kopien
identisch halten (`TestOpenAPISchemasMatchDTOs` prüft das). In
`docs/reference/http-api.md` den Suggest-Abschnitt um beide neuen
Parameter und das Feld ergänzen — inklusive des Satzes, dass `area` ein
Ranking-Signal und **kein** Filter ist (das ist der Kern des gemeldeten
Missverständnisses).

- [ ] **Step 5: Gates**

```bash
go test ./... && make lint && make arch
make mutation PKG=./internal/adapters/http    # Not covered: 0
```

- [ ] **Step 6: Commit**

```bash
git add internal/adapters/http/suggest.go internal/application/suggest.go \
        api/openapi/openapi.yaml internal/adapters/http/openapi.yaml \
        docs/reference/http-api.md internal/adapters/http/suggest_test.go
git commit -m "feat(http): require_target_space, Gebietsvalidierung und matched_name im Suggest"
```

---

### Task 4: Konsole

**Files:**
- Modify: `internal/adapters/http/assets/index.html`
- Modify: `internal/adapters/http/assets/app.js`
- Test: `internal/adapters/http/ui_requests_internal_test.go`, `internal/adapters/http/ui_internal_test.go`

**Interfaces:**
- Consumes: `require_target_space`, `rank`, `matched_name` aus Task 3.

- [ ] **Step 1: Bedienelemente**

Im Suggest-Panel ergänzen:
- `<select id="suggest-rank">` mit `(alle)` (Wert `""`), SPECIES,
  SUBSPECIES, VARIETY, GENUS, FAMILY → Parameter `rank`.
- `<input type="checkbox" id="suggest-require-space">` „nur mit Eintrag im
  Namensraum" → `require_target_space=true`, nur senden, wenn zusätzlich
  ein Namensraum gewählt ist (sonst antwortet der Server per Task 3 mit
  400 — die Konsole darf keinen Request bauen, der sicher scheitert).
- Neue Tabellenspalte **„Treffer-Name"** zwischen „Name" und „sec.", die
  `matched_name.canonical` + `matched_name.authorship` zeigt; fehlt das
  Feld, bleibt die Zelle leer.

Beide neuen Elemente lösen `scheduleSuggest` aus (`change`-Listener), damit
eine Änderung sofort wirkt.

- [ ] **Step 2: Konzeptraum auf `wcvp` vorbelegen**

In `loadCatalog` nach dem Füllen: Enthält die Liste `wcvp`, wird es
ausgewählt. Das ist die Vorbelegung, nicht mehr — der Nutzer kann weiterhin
alles wählen. Im Code begründen: entfernt die CDM-Dubletten (gemessen 9 → 2
für `q=Inula hirta`) und ist der Raum, den die Kette PlantNet→Habitatus
nutzt.

- [ ] **Step 3: Abort-Guard-Zähler pflegen**

`ui_requests_internal_test.go` hat einen zählenden Marker-Test
(`wantAbortedGuardCount`). Wenn neue `api()`-Aufrufstellen dazukommen, den
Zähler MITZÄHLEN; kommen keine dazu, bleibt er unverändert. Nicht raten:
Zahl aus dem Code ableiten.

- [ ] **Step 4: Tests**

```bash
go test ./internal/adapters/http/
make mutation PKG=./internal/adapters/http    # Not covered: 0
```

- [ ] **Step 5: Ende-zu-Ende gegen echte Daten**

Server mit `out/hostus-deploy-v3.4.0-alpha.0.sqlite` starten (freier Port,
NICHT 8080) und die Zielbild-Messung der Spec nachfahren:

```
q=Inula hirta, entry_backbone=wcvp, target_space=eurosl,
require_target_space=true, rank=SPECIES
```

Erwartet: 2 Zeilen, **erste** ist `wcvp:concept:3217689` (*Pentanema
hirtum*) mit `target_space_name: "Inula hirta"`, und die `matched_name`
der beiden Zeilen unterscheiden sich in der Autorschaft (`L.` gegenüber
`Pollich`). Ergebnis wörtlich in den Report.

- [ ] **Step 6: CHANGELOG + Commit**

CHANGELOG unter `## [Unreleased]`: `### Added` für die drei neuen
Bedienelemente und `matched_name`, `### Fixed` für die stille
Nicht-Wirkung des Gebiets-Tippfehlers und die Ranking-Reihenfolge.

```bash
git add internal/adapters/http/assets/index.html internal/adapters/http/assets/app.js \
        internal/adapters/http/ui_requests_internal_test.go CHANGELOG.md
git commit -m "feat(ui): Rang-Filter, Namensraum-Pflicht und Treffer-Name in der Suggest-Konsole"
```

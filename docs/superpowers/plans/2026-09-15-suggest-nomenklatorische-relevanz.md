# Nomenklatorische Relevanz im Suggest — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Wer „Inula hirta" über wcvp sucht, sieht den gültigen Namen zuerst
— und erkennt an der zweiten Zeile, dass sie auf einem nomenklatorisch
ungültigen Namen beruht, statt sie für einen gleichrangigen Treffer zu halten.

**Architecture:** Der SQLite-Adapter liest `name.nom_status` im vorhandenen
zweiten Pass mit und lässt `domain.ClassifyNomStatus` (existiert seit SP6)
urteilen; `domain.RankSuggestions` bekommt daraus ein neues Kriterium an
Position 2; HTTP und Konsole zeigen den Status an.

**Tech Stack:** Go 1.26, modernc.org/sqlite, Vanilla-JS-Konsole. Keine neue Abhängigkeit.

**Spec:** `docs/superpowers/specs/2026-09-15-suggest-nomenklatorische-relevanz.md`

## Global Constraints

- Nur erlaubte Bibliotheken; keine neue Dependency.
- Mutation-Gates: `internal/domain`, `internal/adapters/http` (CI) und lokal `internal/adapters/sqlite` — je `Not covered: 0`. `make mutation` läuft seit PR #122 über `scripts/mutation-sandbox.sh`; einfach `make mutation PKG=…` aufrufen, **synchron** und blockierend (nicht auf eine Benachrichtigung warten).
- Jede Ranking-Änderung braucht eine Kontroll-Assertion (Test gegen den alten Code rot) UND einen Test, der die **Position** des neuen Kriteriums pinnt — beide beteiligten Felder müssen zwischen den Vergleichsobjekten variieren. Im Vorgänger-Branch war genau das einmal versäumt worden und eine Vertauschung blieb unbemerkt.
- `domain.ClassifyNomStatus` ist die einzige Urteilsquelle. Keine eigene Token-Liste, keine `strings.Contains("illeg")`-Abkürzung — die vorhandene Tabelle ist botanisch durchgesehen und trägt Messwerte pro Regel.
- Nur `JudgementDisqualifying` wertet ab; `absent`, `acceptable`, `unclassified` nicht.
- Eine Zeile wird **nie** ausgeblendet.
- **NIEMALS `git add -A` oder `git add .`** — im Arbeitsbaum liegen unversionierte Dateien des Nutzers (`out/` mit mehreren GB, `console-*.png`, `build-full.yaml`, `.playwright-mcp/`). Nur die in den Tasks genannten Dateien namentlich adden.
- `out/hostus-deploy-v3.4.0-alpha.0.sqlite` ist die Produktionsdatenbank des Nutzers: **nur lesend** benutzen (`mode=ro` oder eine Kopie), niemals schreiben oder löschen.

## File Structure

- `internal/domain/suggest.go` — Feld `MatchedName.NomStatus`/`NomStatusJudgement`, Kriterium in `RankSuggestions`.
- `internal/adapters/sqlite/suggest.go` — `matchedNameQuery` + Auswahlregel + Klassifikation.
- `internal/adapters/http/suggest.go` + beide `openapi.yaml` — DTO-Felder.
- `internal/adapters/http/assets/{index.html,app.js}` — Kennzeichnung.
- `docs/reference/http-api.md`, `CHANGELOG.md`.

---

### Task 1: Urteil im Domain-Modell und im Ranking

**Files:**
- Modify: `internal/domain/suggest.go`
- Test: `internal/domain/suggest_test.go`

**Interfaces:**
- Produces: `MatchedName.NomStatus string` (normalisiert) und `MatchedName.NomStatusJudgement NomStatusJudgement`; `SuggestItem.MatchedNameDisqualified bool`; `RankSuggestions` mit dem neuen Kriterium an Position 2.

- [ ] **Step 1: Failing tests**

```go
// TestRankSuggestions_DisqualifiedMatchedNameLosesToLegitimate pins the
// reported Inula-hirta case as the user hits it: entry_backbone=wcvp and NO
// target_space, so the target-space key cannot decide. "Inula hirta Pollich"
// is a later illegitimate homonym (WCVP nom_status ", nom. illeg. homonym.
// post.") of Pentanema britannica; "Inula hirta L." is the legitimate name
// of Pentanema hirtum. Before this key existed, bm25 alone ordered them.
func TestRankSuggestions_DisqualifiedMatchedNameLosesToLegitimate(t *testing.T) {
	britannica := SuggestItem{
		ConceptID: "wcvp:concept:3217682", Rank: RankSpecies, Status: StatusAccepted,
		ExactHit: true, MatchedNameDisqualified: true, PrefixHit: true, Score: 1.0,
	}
	hirtum := SuggestItem{
		ConceptID: "wcvp:concept:3217689", Rank: RankSpecies, Status: StatusAccepted,
		ExactHit: true, MatchedNameDisqualified: false, PrefixHit: true, Score: 2.0,
	}

	got := RankSuggestions([]SuggestItem{britannica, hirtum})

	if got[0].ConceptID != hirtum.ConceptID {
		t.Fatalf("first = %q, want the legitimate name %q", got[0].ConceptID, hirtum.ConceptID)
	}
}

// TestRankSuggestions_LegitimacyOutranksTargetSpaceHit pins the POSITION of
// the new key: it sits ahead of TargetSpaceHit, so a disqualified name does
// not win just because it happens to be the spelling used in the requested
// space. Both keys differ between the items — otherwise swapping them would
// leave this test green and pin nothing.
func TestRankSuggestions_LegitimacyOutranksTargetSpaceHit(t *testing.T) {
	disqualifiedInSpace := SuggestItem{
		ConceptID: "a", Rank: RankSpecies, Status: StatusAccepted, ExactHit: true,
		MatchedNameDisqualified: true, TargetSpaceHit: true, Score: 0.1,
	}
	legitimateOutsideSpace := SuggestItem{
		ConceptID: "b", Rank: RankSpecies, Status: StatusAccepted, ExactHit: true,
		MatchedNameDisqualified: false, TargetSpaceHit: false, Score: 0.9,
	}

	got := RankSuggestions([]SuggestItem{disqualifiedInSpace, legitimateOutsideSpace})

	if got[0].ConceptID != "b" {
		t.Fatalf("first = %q, want the legitimate name %q", got[0].ConceptID, "b")
	}
}
```

Dazu einen dritten Test, der belegt, dass `absent`, `acceptable` und
`unclassified` **nicht** abwerten (drei Items, die sich nur im Urteil
unterscheiden, behalten ihre Eingabereihenfolge) — das ist Entscheidung 2 der
Spec und die Stelle, an der eine zu strenge Implementierung unbemerkt bliebe.

- [ ] **Step 2: Fehlschlag bestätigen**

Run: `go test ./internal/domain/ -run TestRankSuggestions -v`
Expected: FAIL. Die beobachtete Ausgabe wörtlich in den Report.

- [ ] **Step 3: Felder und Kriterium**

`MatchedName` um `NomStatus string` und `NomStatusJudgement NomStatusJudgement`
erweitern (Doc-Kommentar: normalisiert, leer heißt „nichts erfasst"; das
Urteil wird IMMER gesetzt, `JudgementAbsent` ist eine Aussage, kein
Fehlwert). `SuggestItem.MatchedNameDisqualified` ergänzen mit dem Hinweis,
dass es aus `MatchedName.NomStatusJudgement == JudgementDisqualifying`
abgeleitet ist und der Adapter es setzt.

In `RankSuggestions` den Vergleich **zwischen `ExactHit` und
`TargetSpaceHit`** einhängen (false vor true) und die nummerierte Liste im
Doc-Kommentar auf acht Kriterien fortschreiben, samt der Begründung aus
Spec-Entscheidung 1 (grundsätzlicher als die Raumfrage; der gemeldete Fall
tritt ohne `target_space` auf).

- [ ] **Step 4: Gates**

Run: `go test ./... && make lint && make arch && make mutation PKG=./internal/domain`
Expected: grün, `Not covered: 0`.

- [ ] **Step 5: Commit**

```bash
git add internal/domain/suggest.go internal/domain/suggest_test.go
git commit -m "feat(domain): disqualifizierte Treffer-Namen im Suggest-Ranking abwerten"
```

---

### Task 2: Status lesen und klassifizieren im SQLite-Adapter

**Files:**
- Modify: `internal/adapters/sqlite/suggest.go`
- Test: `internal/adapters/sqlite/suggest_internal_test.go`

**Interfaces:**
- Consumes: Task 1s Felder.
- Produces: gefüllte `MatchedName.NomStatus`/`.NomStatusJudgement` und `SuggestItem.MatchedNameDisqualified`.

- [ ] **Step 1: Failing tests**

- `TestSuggest_MatchedNameCarriesNomStatusJudgement` — eine Fixture mit
  `nom_status = ", nom. illeg. homonym. post."` liefert
  `NomStatusJudgement == domain.JudgementDisqualifying`, normalisierten
  `NomStatus` und `MatchedNameDisqualified == true`.
- `TestSuggest_MatchedNameWithoutNomStatusIsAbsentNotDisqualified` — ohne
  Status: `JudgementAbsent`, `MatchedNameDisqualified == false`, `NomStatus`
  leer.
- `TestSuggest_LegitimateNameWinsTheMatchedNameSelection` — ein Konzept trägt
  zwei Namen, die die Anfrage erfüllen, einer davon disqualifiziert: gewählt
  wird der gültige (Spec-Entscheidung 5).

- [ ] **Step 2: Query und Auswahlregel**

`matchedNameQuery` um `COALESCE(nm.nom_status, '')` in der Select-Liste
erweitern und die Vorrangstufe ergänzen. Die Klassifikation gehört NICHT in
SQL (die Regeltabelle lebt in `domain`), also im SQL nur ein grober
Sortierschlüssel — und genau diese Doppelung muss der Kommentar benennen:

```sql
CASE WHEN COALESCE(nm.nom_status, '') = '' THEN 0 ELSE 1 END AS has_nom_status
```

**Achtung, bewusste Ungenauigkeit:** `has_nom_status` ist NICHT dasselbe wie
`disqualifying` — `nom. cons.` trägt einen Status und ist trotzdem sauber.
Der SQL-Schlüssel dient nur als grobe Vorsortierung; die maßgebliche Auswahl
trifft Go, nachdem `domain.ClassifyNomStatus` über alle Kandidatenzeilen
eines Konzepts geurteilt hat. Wer das anders löst (z. B. die Auswahl ganz in
Go), muss den Determinismus der Reihenfolge erhalten — die vorhandenen
Tie-Breaks (canonical, authorship, id) bleiben verbindlich.

Reihenfolge der Auswahl insgesamt: exakt vor Präfix → nicht disqualifiziert
vor disqualifiziert → `accepted` vor `synonym` → canonical → authorship → id.

- [ ] **Step 3: Klassifizieren und setzen**

Im Scan-Pfad `domain.ClassifyNomStatus(raw)` aufrufen, `NomStatus` auf
`v.Normalized` setzen, `NomStatusJudgement` auf `v.Judgement`, und
`item.MatchedNameDisqualified = v.Judgement == domain.JudgementDisqualifying`.

- [ ] **Step 4: Gates + Latenz**

```bash
go test ./internal/adapters/sqlite/
make mutation PKG=./internal/adapters/sqlite      # Not covered: 0
```

Latenz gegen eine **Kopie** der Produktions-DB, interleaved vorher/nachher,
je drei warme Läufe, für `q=Inula hirta` und `q=ca&limit=30`. Erwartung: kein
messbarer Unterschied (eine Spalte mehr im vorhandenen Query, keine neue
Abfrage). Zahlen in den Report — auch wenn sie unauffällig sind.

- [ ] **Step 5: Commit**

```bash
git add internal/adapters/sqlite/suggest.go internal/adapters/sqlite/suggest_internal_test.go
git commit -m "feat(sqlite): nom_status des Treffer-Namens lesen, klassifizieren und bevorzugen"
```

---

### Task 3: Antwort, Doku und Konsole

**Files:**
- Modify: `internal/adapters/http/suggest.go`
- Modify: `api/openapi/openapi.yaml` UND `internal/adapters/http/openapi.yaml` (identisch halten!)
- Modify: `internal/adapters/http/assets/index.html`, `internal/adapters/http/assets/app.js`
- Modify: `docs/reference/http-api.md`, `CHANGELOG.md`
- Test: `internal/adapters/http/suggest_test.go`, `internal/adapters/http/ui_internal_test.go`

**Interfaces:**
- Consumes: Task 1/2.
- Produces: `matched_name.nom_status` (omitempty) und `matched_name.nom_status_judgement` (immer).

- [ ] **Step 1: Failing tests**

- `TestSuggest_MatchedNameRendersNomStatusJudgement` — `nom_status_judgement`
  ist auch dann im JSON, wenn kein Status erfasst ist (`"absent"`), und
  `nom_status` fehlt dann.
- `TestSuggest_DisqualifiedMatchedNameRendersBothFields` — bei einem
  illegitimen Namen stehen beide Felder drin.

- [ ] **Step 2: DTO**

`matchedNameDTO` um beide Felder erweitern, benannt und begründet **wie in
`internal/adapters/http/synonyms.go`** (dort steht die Begründung, warum das
Urteil immer gerendert wird: „nichts erfasst" ist nicht „geprüft und
sauber"). Den dortigen Kommentar nicht kopieren, sondern darauf verweisen.

- [ ] **Step 3: OpenAPI und Referenz**

Beide `openapi.yaml` (identisch!) um die Felder erweitern; die
Ranking-Liste in `docs/reference/http-api.md` UND in der
`suggest`-`description` beider OpenAPI-Dateien um das neue Kriterium an
Position 2 ergänzen. In der Referenz den *Inula-hirta*-Absatz fortschreiben:
er erklärt dort bereits das Homonym — jetzt gehört dazu, dass Pollichs Name
illegitim ist und deshalb abgewertet wird.

- [ ] **Step 4: Konsole**

In der Spalte „Treffer-Name" ein Badge für `nom_status_judgement ===
"disqualifying"`, Tooltip = `nom_status`. Am vorhandenen Badge-Muster
orientieren (die Konsole badget bereits Aggregat-Treffer und „kein Name").
Kein neuer `api()`-Aufruf → der Zähler `wantAbortedGuardCount` in
`ui_requests_internal_test.go` bleibt unverändert; das vor dem Commit per
`grep` verifizieren, nicht annehmen.

- [ ] **Step 5: Gates und Abnahme**

```bash
go test ./... && make lint && make arch
make mutation PKG=./internal/adapters/http     # Not covered: 0
make docs                                      # mkdocs --strict
```

Abnahme gegen eine **Kopie** der Produktions-DB, auf einem freien Port (nicht
8080), wörtlich in den Report:

```
q=Inula hirta&entry_backbone=wcvp          (KEIN target_space!)
```
Erwartet: erste Zeile *Pentanema hirtum* (`nom_status_judgement: "absent"`),
zweite Zeile *Pentanema britannica* mit
`nom_status: "nom. illeg. homonym. post."` und
`nom_status_judgement: "disqualifying"`.

- [ ] **Step 6: CHANGELOG + Commit**

`### Added` für die zwei Felder und die Kennzeichnung, `### Fixed` für die
Reihenfolge — mit dem Messwert aus der Spec (28.233 Schreibweisen, unter
denen ein disqualifizierter neben einem statusfreien Namen steht).

```bash
git add internal/adapters/http/suggest.go internal/adapters/http/suggest_test.go \
        api/openapi/openapi.yaml internal/adapters/http/openapi.yaml \
        internal/adapters/http/assets/index.html internal/adapters/http/assets/app.js \
        docs/reference/http-api.md CHANGELOG.md
git commit -m "feat(http,ui): nomenklatorischen Status des Treffer-Namens ausliefern und kennzeichnen"
```

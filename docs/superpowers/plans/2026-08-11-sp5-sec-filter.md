# SP5 `sec.`-Filter + `sec`-Ausgabe — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax.

**Goal:** Gleichnamige Multi-Backbone-Konzepte eindeutig auflösbar machen (`entry_backbone`/`entry_sec` auf `/v1/match` + `/v1/translate`) und in der Ausgabe unterscheidbar (`sec` `{id,title}` in `/v1/suggest` + `/v1/concept`).

**Architecture:** Filter im **Application-Layer** nach `repo.MatchExact` (Kandidaten tragen `Concept.BackboneID` + `Concept.SecReference` bereits; Port unverändert), vor `classify`. Validierung gegen `repo.BackboneVersions`/`repo.SecReferenceByID` mit typisierten Fehlern → HTTP `400 INVALID_QUERY`. `sec`-Ausgabe wiederverwendet die vorhandene `secReferenceDTO`.

**Tech Stack:** Go 1.26, hexagonal, `modernc.org/sqlite`, gorilla/mux. Kein neuer Dependency.

## Global Constraints

- Branch `feature/sp5-sec-filter`, von `master` geforkt.
- Hexagonal + depguard: `internal/application` importiert nur `internal/domain` + `internal/ports`. Kein neuer Runtime-Dependency (CLAUDE.md-Liste ist abschließend).
- Per-Task-DoD, FOREGROUND: `go test -timeout 180s ./...` → `make mutation PKG=<berührtes Paket>` (`Not covered: 0`) → `make lint` clean inkl. `_test.go`, beide Build-Tag-Pässe → `make verify` → `make test-integration` → `make security-check` → `make licenses` → `make doc` (mkdocs --strict) → `make doc-drift`.
- LSP-Diagnostics sind hier oft STALE — nur `nix develop -c go build ./...`/`go vet` trauen. Bei `make mutation`-Permission-Fehler: `chmod -R u+w ./.go && rm -rf ./.go`, retry.
- Docs deutsch; Code-Kommentare sparsam, englisch. `VERSION` nicht anfassen; CHANGELOG unter `[Unreleased]`. Nie `.envrc.local` lesen/ändern.
- **Ohne Filter/`sec`-Feld muss jede Antwort byteweise die heutige sein** — das ist die tragende Invariante (mehrere UCs teilen die Pfade).
- **Eine Zahl, die du nicht gemessen hast, darfst du nicht berichten.**

---

## Task 1: `entry_backbone`/`entry_sec`-Filter auf `POST /v1/match`

**Files:**
- Modify: `internal/application/match.go` (Filter-Typ, Anwendung in `matchOne`/`matchAggregate`, `MatchInSpace`-Signatur + Validierung, neue Sentinel-Fehler)
- Modify: `internal/adapters/http/taxa.go` (`matchRequestDTO`: `sec_hint`→`entry_backbone`/`entry_sec`; `handleMatch` reicht Filter durch + Fehlermapping)
- Test: `internal/application/match_filter_test.go`, `internal/adapters/http/match_filter_test.go`

**Interfaces:**
- Produces:
  - `application.MatchFilter{ Backbone string; Sec string }`
  - `application.ErrUnknownBackbone`, `application.ErrUnknownSec` (sentinel `error`)
  - `application.MatchInSpace(ctx, repo, reqs, space string, filter MatchFilter) ([]MatchResult, error)` (bestehende Signatur um `filter` erweitert)
- Consumes: `repo.MatchExact`, `repo.BackboneVersions`, `repo.SecReferenceByID` (liefert `domain.ErrNotFound` bei unbekannt), `domain.Concept.BackboneID`/`.SecReference`.

- [ ] **Step 1: Failing test (application)** — `internal/application/match_filter_test.go`. Seed einen Namen, der auf ZWEI Konzepte in zwei Backbones fällt (z. B. das bestehende `seedHomonymPair` erweitern oder ein WCVP-Konzept + ein zweites Backbone mit gleichem Canonical). Fälle:

```go
// ohne Filter: heutiges (mehrdeutiges) Ergebnis, unverändert
// entry_backbone auf Backbone A -> genau ein Konzept, exact
res, err := application.MatchInSpace(ctx, repo, []application.MatchRequest{{ID:"1",Verbatim:"Homonymus testicus L."}}, "", application.MatchFilter{Backbone: backboneA})
// r.MatchType == exact_author, r.ConceptID == conceptA
// unbekannter Backbone -> ErrUnknownBackbone
_, err = application.MatchInSpace(ctx, repo, reqs, "", application.MatchFilter{Backbone: "nope"})
if !errors.Is(err, application.ErrUnknownBackbone) { t.Fatal(...) }
// unbekannter sec -> ErrUnknownSec
_, err = application.MatchInSpace(ctx, repo, reqs, "", application.MatchFilter{Sec: "nope"})
if !errors.Is(err, application.ErrUnknownSec) { t.Fatal(...) }
// leerer Filter == MatchNames (reflect.DeepEqual)
```

- [ ] **Step 2: Run, verify FAIL** — `nix develop -c go test ./internal/application/ -run TestMatchFilter` → compile-Fail (undefinierte Symbole).

- [ ] **Step 3: Implement (application)** — in `match.go`:

```go
type MatchFilter struct{ Backbone, Sec string }

var ErrUnknownBackbone = errors.New("unknown entry_backbone")
var ErrUnknownSec = errors.New("unknown entry_sec")

// keep(c) reports whether candidate c survives the filter.
func (f MatchFilter) keep(c output.MatchCandidate) bool {
	if f.Backbone != "" && c.Concept.BackboneID != f.Backbone { return false }
	if f.Sec != "" && c.Concept.SecReference != f.Sec { return false }
	return true
}
func (f MatchFilter) apply(cands []output.MatchCandidate) []output.MatchCandidate {
	if f.Backbone == "" && f.Sec == "" { return cands }
	out := cands[:0:0]
	for _, c := range cands { if f.keep(c) { out = append(out, c) } }
	return out
}
```

`matchOne`/`matchAggregate` bekommen den Filter durchgereicht und wenden `filter.apply(candidates)` direkt nach `repo.MatchExact` an. `MatchInSpace` erhält `filter MatchFilter`, validiert ihn ZUERST (vor jeder Auflösung):

```go
if filter.Backbone != "" {
	bvs, err := repo.BackboneVersions(ctx); if err != nil { return nil, err }
	if !slices.ContainsFunc(bvs, func(b domain.BackboneVersion) bool { return b.ID == filter.Backbone }) {
		return nil, ErrUnknownBackbone
	}
}
if filter.Sec != "" {
	if _, err := repo.SecReferenceByID(ctx, filter.Sec); errors.Is(err, domain.ErrNotFound) {
		return nil, ErrUnknownSec
	} else if err != nil { return nil, err }
}
```

Die filterlose `MatchNames(ctx, repo, reqs)` bleibt bestehen und delegiert an den gemeinsamen Kern mit `MatchFilter{}` (Nullwert) — damit ist `MatchInSpace(...,"",MatchFilter{})` byteweise `MatchNames`.

- [ ] **Step 4: Run, verify PASS** — `nix develop -c go test ./internal/application/ -run TestMatchFilter -v`.

- [ ] **Step 5: Failing test (http)** — `internal/adapters/http/match_filter_test.go`: ohne Filter byte-identisch (Feld-Abwesenheit); `entry_backbone`/`entry_sec` im Body macht einen mehrdeutigen Namen eindeutig (Seed via bestehendem Test-Setup + ein zweites Backbone/sec-Konzept); unbekannter `entry_backbone`/`entry_sec` → `400 INVALID_QUERY`, Meldung nennt den Wert.

- [ ] **Step 6: Implement (http)** — `matchRequestDTO`: `SecHint` entfernen, `EntryBackbone string \`json:"entry_backbone,omitempty"\`` + `EntrySec string \`json:"entry_sec,omitempty"\`` ergänzen. `handleMatch`:

```go
results, err := application.MatchInSpace(r.Context(), repo, reqs, body.TargetSpace,
	application.MatchFilter{Backbone: body.EntryBackbone, Sec: body.EntrySec})
switch {
case errors.Is(err, application.ErrUnknownTargetSpace):
	httperr.InvalidQueryError(w, "unknown target_space "+strconv.Quote(body.TargetSpace)); return
case errors.Is(err, application.ErrUnknownBackbone):
	httperr.InvalidQueryError(w, "unknown entry_backbone "+strconv.Quote(body.EntryBackbone)); return
case errors.Is(err, application.ErrUnknownSec):
	httperr.InvalidQueryError(w, "unknown entry_sec "+strconv.Quote(body.EntrySec)); return
case err != nil:
	httperr.InternalError(w); return
}
```

- [ ] **Step 7: Run, verify PASS** — `nix develop -c go test ./internal/adapters/http/ -run TestHandleMatch`.

- [ ] **Step 8: DoD** — `go test ./...`, `make mutation PKG=./internal/application`, `make mutation PKG=./internal/adapters/http`, `make lint`, `make verify`.

- [ ] **Step 9: Commit** `feat(match): entry_backbone/entry_sec resolution filter`.

---

## Task 2: `entry_backbone`/`entry_sec` auf `POST /v1/translate` (verbatim-Pfad)

**Files:**
- Modify: `internal/application/translate.go` (verbatim-Auflösung nutzt `MatchFilter`; Validierung wiederverwenden)
- Modify: `internal/adapters/http/translate.go` (`translateRequestDTO` + Fehlermapping)
- Test: `internal/application/translate_filter_test.go`, `internal/adapters/http/translate_filter_test.go`

**Interfaces:**
- Consumes: `application.MatchFilter`, `application.ErrUnknownBackbone/ErrUnknownSec` (aus Task 1).
- Produces: `application.Translate(...)`-Einstieg um `filter MatchFilter` erweitert (genaue Signatur beim Implementieren an `translate.go` anpassen; der `concept_id`-Pfad ignoriert den Filter).

- [ ] **Step 1: Failing test (application)** — `translate_filter_test.go`: ein Verbatim, das ohne Filter mehrdeutig ist (WCVP + mehrere CDM-`sec.`), mit `entry_sec=<Quellraum>` auf GENAU ein Konzept auflöst und dann über eine echte Relation übersetzt wird (result `translated`). `concept_id`-Pfad + gesetztem Filter → Filter ignoriert (unverändert).

```go
// erwartet: ohne Filter -> unresolvable/mehrdeutig (0 übersetzt)
// mit entry_sec=srcSec -> genau ein Quellkonzept -> result "translated", candidates>=1
```

- [ ] **Step 2: Run, verify FAIL.**

- [ ] **Step 3: Implement** — die verbatim-Auflösung in `translate.go` ruft denselben gefilterten Match-Kern wie Task 1 (bzw. `MatchInSpace`/den geteilten Helper) mit `filter`. Validierung des Filters über die Task-1-Helper (vor der Auflösung). `concept_id`-Pfad bleibt unberührt.

- [ ] **Step 4: Run, verify PASS.**

- [ ] **Step 5: Failing test (http)** — `translate_filter_test.go`: `verbatim` + `entry_sec` übersetzt (200, `result:"translated"`); unbekannter `entry_backbone`/`entry_sec` → `400 INVALID_QUERY`; `concept_id` + Filter → Filter ignoriert.

- [ ] **Step 6: Implement (http)** — `translateRequestDTO` um `EntryBackbone`/`EntrySec` (`json:"entry_backbone,omitempty"`/`"entry_sec,omitempty"`); Weiterreichen an `application.Translate`; Fehlermapping wie Task 1 Step 6.

- [ ] **Step 7: Run, verify PASS.**

- [ ] **Step 8: DoD** — `make mutation PKG=./internal/application`, `PKG=./internal/adapters/http`, `make lint`, `make verify`.

- [ ] **Step 9: Commit** `feat(translate): entry_backbone/entry_sec for the verbatim entry`.

---

## Task 3: `sec` `{id,title}` in `GET /v1/concept/{id}`

**Files:**
- Modify: `internal/adapters/http/taxa.go` (`conceptDTO` bekommt `Sec *secReferenceDTO`; `conceptToDTO` füllt es; `writeConcept` beschafft den Titel)
- Modify ggf.: `internal/adapters/http/translate.go` (`secReferenceDTO` ist dort definiert — sicherstellen, dass es paketweit nutzbar ist; sonst nach `taxa.go` verschieben)
- Test: `internal/adapters/http/taxa_test.go` (bzw. neues `concept_sec_test.go`)

**Interfaces:**
- Consumes: `domain.Concept.SecReference`, `repo.SecReferenceByID(ctx, id) (domain.SecReference, error)`.

- [ ] **Step 1: Failing test** — für ein CDM-Konzept (mit `SecReference`) enthält die Antwort `sec` `{id,title}`; für ein WCVP-Konzept (kein `SecReference`) fehlt das Feld (Abwesenheit prüfen via `map[string]json.RawMessage`). Ohne CDM byteweise unverändert.

- [ ] **Step 2: Run, verify FAIL.**

- [ ] **Step 3: Implement** — `conceptDTO`:

```go
Sec *secReferenceDTO `json:"sec,omitempty"`
```

In `writeConcept`: wenn `c.SecReference != ""`, `sr, err := repo.SecReferenceByID(ctx, c.SecReference)` (bei `ErrNotFound` das Feld weglassen, kein 500), und `dto.Sec = &secReferenceDTO{ID: sr.ID, Title: sr.Title}`.

- [ ] **Step 4: Run, verify PASS.**

- [ ] **Step 5: DoD** — `make mutation PKG=./internal/adapters/http`, `make lint`, `make verify`.

- [ ] **Step 6: Commit** `feat(concept): emit sec {id,title} for sec-bearing concepts`.

---

## Task 4: `sec` `{id,title}` in `GET /v1/suggest`

**Files:**
- Modify: `internal/domain/suggest.go` (`SuggestItem` bekommt `SecReference string` — nur die id, kein I/O im domain)
- Modify: `internal/adapters/sqlite/suggest.go`/`read.go` (Suggest-Query liefert `taxon_concept.sec_reference` je Treffer)
- Modify: `internal/adapters/http/suggest.go` (`suggestItemDTO.Sec *secReferenceDTO`; Titel je distinktem sec via `repo.SecReferenceByID`, gecacht über die Ergebnisliste)
- Test: `internal/adapters/sqlite/suggest_test.go`, `internal/adapters/http/suggest_test.go`

**Interfaces:**
- Consumes: `repo.SecReferenceByID`.
- Produces: `domain.SuggestItem.SecReference string`.

- [ ] **Step 1: Failing test (sqlite)** — ein CDM-Suggest-Treffer trägt `SecReference` = die sec-id; ein WCVP-Treffer trägt `""`.

- [ ] **Step 2: Run, verify FAIL.**

- [ ] **Step 3: Implement (sqlite)** — `SuggestItem.SecReference` ergänzen; die Suggest-`SELECT` um `tc.sec_reference` erweitern und in den Scan aufnehmen (NULL → `""`).

- [ ] **Step 4: Run, verify PASS.**

- [ ] **Step 5: Failing test (http)** — CDM-Treffer hat `sec` `{id,title}`; WCVP-Treffer nicht; ohne CDM byteweise unverändert.

- [ ] **Step 6: Implement (http)** — `suggestItemDTO.Sec *secReferenceDTO`; beim Rendern je Treffer mit gesetztem `SecReference` den Titel auflösen (kleine `map[string]secReferenceDTO`-Cache über die Ergebnisliste, damit nicht pro Treffer neu gelesen wird).

- [ ] **Step 7: Run, verify PASS.**

- [ ] **Step 8: DoD** — `make mutation PKG=./internal/domain`, `PKG=./internal/adapters/sqlite`, `PKG=./internal/adapters/http`, `make lint`, `make verify`.

- [ ] **Step 9: Commit** `feat(suggest): emit sec {id,title} per candidate`.

---

## Task 5: e2e, Messung, Docs, known-gaps, CHANGELOG

**Files:**
- Modify/Create: `internal/app/integration_secfilter_test.go` (`integration`-Tag)
- Create: `docs/research/sp5-sec-filter.md` (Messung)
- Modify: `api/openapi/openapi.yaml`, `docs/reference/http-api.md`
- Modify: `docs/explanation/known-gaps.md` (beide SP5-Einträge entfernen), `CHANGELOG.md`

- [ ] **Step 1: e2e** — `integration`-Test über WCVP + CDM: ein mehrdeutiger Name wird mit `entry_backbone=wcvp` eindeutig (`exact`) und mit `target_space=eurosl` zusätzlich mit `target_space_name` versehen; `/v1/translate`-`verbatim` + `entry_sec` übersetzt (`result:"translated"`); `/v1/suggest` + `/v1/concept` liefern `sec` für ein CDM-, keins für ein WCVP-Konzept. **Konkrete Namen/Räume asserten, nicht nur 200.**

- [ ] **Step 2: Messung** — gegen den realen Multi-Backbone-Index (WCVP+CDM) messen: von den bisher 265 mehrdeutigen der 300-Namen-Stichprobe, wie viele mit `entry_sec=<Quellraum>` eindeutig werden und wie viele `translate` dann übersetzt (0→N); Anteil gängiger Namen, die mit `entry_backbone=wcvp` eindeutig werden. Ergebnis + Kommandos nach `docs/research/sp5-sec-filter.md`. **Verbessert der Filter die Zahlen nicht messbar, gilt das Design als nicht gehalten — dann STOPP und berichten.**

- [ ] **Step 3: OpenAPI + `docs/reference/http-api.md`** — `entry_backbone`/`entry_sec` auf `/v1/match` + `/v1/translate`; `sec` `{id,title}` in `/v1/suggest` + `/v1/concept`; die 400-Fälle. Byte-identisch-ohne-Filter/`sec` klarstellen.

- [ ] **Step 4: known-gaps + CHANGELOG** — die zwei SP5-Einträge (`verbatim`-tot; kein `sec`-Feld) aus `docs/explanation/known-gaps.md` entfernen (Verlauf → CHANGELOG). CHANGELOG unter `[Unreleased]`.

- [ ] **Step 5: Voller Gate** — `make verify`, `make test-integration`, `make security-check`, `make licenses`, `make doc`, `make doc-drift`.

- [ ] **Step 6: Commit** `docs(sp5): sec-filter e2e, measurement, api docs, known-gaps`.

---

## Self-Review-Notizen

- **Byte-Identität ist die tragende Invariante.** Jeder Endpunkt-Task pinnt „ohne Filter/`sec` unverändert" zuerst — mehrere UCs teilen die Pfade.
- **Erst messen (Task 5 Step 2), dann glauben.** Ein Filter, der Mehrdeutigkeit nur verschiebt, ist keine Verbesserung — explizites Erfolgskriterium.
- **Filter im Application-Layer, Port `MatchExact` unverändert** — geringste Fläche; `MatchCandidate` trägt `BackboneID`/`SecReference` bereits.
- **`sec_hint` (totes Feld) wird ersetzt, nicht danebengelegt** — unbekannte JSON-Felder ignoriert der Decoder, also kein Bruch.
- **`entry_sec` alleine genügt** (impliziert sec-tragendes Backbone); `entry_backbone`+`entry_sec` verknüpfen mit UND.

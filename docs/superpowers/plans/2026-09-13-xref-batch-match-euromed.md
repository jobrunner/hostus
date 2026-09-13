# Xref-Batch-Match + E+M-UUIDs + Synonymie-Schluss Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** `/v1/match` nimmt POWO-/Fremd-IDs batchweise entgegen (PlantNet-Workflow in EINEM Call), Zielraum-Antworten tragen E+M-UUID + Status, und der Namespace-Ingest schließt die 3.719 fehlenden accepted-Einträge über die Quell-Synonymie.

**Architecture:** Xref-Zweig in `matchOne` vor der Namens-Leiter (Port `ConceptByXref` existiert); `domain.ResolveTargetSpace` liefert den gewählten Eintrag statt nur den Namen; Schluss-Pass als dritter Schritt in `IngestNameSpace` (nach Resolve, vor Write), rein quell-intern über `accepted_taxon`-Gruppen.

**Tech Stack:** Go 1.26, bestehende Ports/Adapter — keine neuen Dependencies, kein Schema-Change (`resolution`-Spalte trägt den neuen Marker).

**Spec:** `docs/superpowers/specs/2026-09-13-xref-batch-match-euromed.md`

## Global Constraints

- Pro Match-Zeile GENAU EINES von `verbatim`/`xref` (beides/keines → 400 `INVALID_QUERY` mit Zeilen-ID); unbekannte Xref-ID → UNRESOLVABLE-Result, kein HTTP-Fehler.
- Xref-Zweig umgeht Namens-Leiter/Fuzzy komplett; `entry_backbone`/`entry_sec` gelten (Konzept fällt aus dem Filter → unresolvable mit Note); Batch-/Body-Limits unverändert.
- Bestehende Antworten byte-identisch ohne neue Eingaben (neue DTO-Felder `omitempty`; `ResolveTargetSpace`-Umbau darf die Namenswahl NICHT ändern — bestehende Tests sind der Beweis).
- Schluss-Pass ändert NIE ein Resolve-Ergebnis, füllt nur offene Zeilen; attached nur bei GENAU EINEM Gruppen-Zielkonzept; Marker `source_synonymy_closure`, Zähler `SynonymyClosed` + Sample (Invariante Matched+Unmatched+Ambiguous == Rows bleibt: geschlossene Zeilen wandern von Ambiguous/Unmatched nach Matched und werden separat gezählt).
- Mutation-Gates: application, adapters/http, domain, adapters/namelist, cmd/hostus.
- CHANGELOG unter `## [Unreleased]`; Conventional Commits; KEIN `git add -A`.

---

### Task 1: Xref-Einträge in `/v1/match` (Ende-zu-Ende)

**Files:**
- Modify: `internal/domain/` (MatchType-Konstanten — Datei mit `MatchExact`/`MatchFuzzy` suchen: `git grep -n "MatchFuzzy MatchType\|MatchExact MatchType" internal/domain/`), `internal/application/match.go` (MatchRequest, matchOne, matchNamesFiltered-Validierung), `internal/adapters/http/taxa.go` (DTO + Validierung)
- Test: `internal/application/match_xref_test.go` (neu), `internal/adapters/http/taxa_test.go` (erweitern)

**Interfaces:**
- Consumes: `output.Repository.ConceptByXref(ctx, authority, extID)` (Port existiert, repository.go:57).
- Produces: `application.MatchRequest.Xref *application.XrefRef` mit `XrefRef{Authority, ID string}`; `domain.MatchXref MatchType = "xref"`; DTO-Feld `"xref": {"authority": "...", "id": "..."}`.

- [ ] **Step 1: Fehlschlagende Tests schreiben**

`match_xref_test.go` (Fixtures via `openMemoryRepo` + `application.Ingest` + Xref-Anlage — Muster für Xref-Seeding: `git grep -n "AddXref\|IngestXrefs" internal/application/*_test.go`):

```go
// TestMatchNames_XrefEntryResolvesByForeignID pins the PlantNet workflow
// (spec 2026-09-13): a row carrying {"xref":{"authority":"powo","id":...}}
// resolves through Repository.ConceptByXref — no name ladder, no fuzzy —
// with match_type "xref" and confidence 1.0, and target_space enrichment
// works exactly as for verbatim rows.
func TestMatchNames_XrefEntryResolvesByForeignID(t *testing.T) {
	// Fixture: WCVP-Konzept + powo-Xref + eurosl-Eintrag.
	// MatchInSpace(reqs=[{ID:"1", Xref:&XrefRef{"powo","77178414-1"}}],
	//   space="eurosl", filter leer)
	// Assertions: MatchType==domain.MatchXref, Confidence==1.0,
	//   ConceptID==erwartet, TargetSpaceName gefüllt.
}

// TestMatchNames_XrefUnknownIDIsUnresolvable: unbekannte ID -> Result mit
// leerer ConceptID + RequiresReview + Note (Batch läuft weiter, die
// Nachbarzeile mit gültigem verbatim resolved normal).

// TestMatchNames_XrefRespectsBackboneFilter: Konzept ist wcvp, Filter
// entry_backbone="cdm" -> unresolvable mit Note (Filter gilt auch für
// Xref-Treffer).

// TestMatchNames_RowWithBothOrNeitherIsRejected: Xref UND Verbatim gesetzt
// (bzw. beides leer) -> Fehler ErrInvalidMatchRequest mit Zeilen-ID (die
// HTTP-Schicht mappt auf 400 INVALID_QUERY) — Tabellentest beide Fälle.
```

`taxa_test.go`: httptest — Batch mit gemischten Zeilen (verbatim + xref) → 200 mit beiden Ergebnissen; Zeile mit beidem → 400 `INVALID_QUERY`, Message nennt die Zeilen-ID; `match_type:"xref"` erscheint im JSON.

- [ ] **Step 2: Fehlschlag verifizieren** — `go test ./internal/application/ -run TestMatchNames_Xref -v` → FAIL.

- [ ] **Step 3: Implementierung**
  - `domain`: `MatchXref MatchType = "xref"` neben den bestehenden Konstanten, Doc-Kommentar (Fremd-ID schlägt jeden Namens-Match; PlantNet/POWO-Use-Case).
  - `application`: 
    ```go
    // XrefRef identifies one row by a foreign authority's id instead of a
    // verbatim name — the PlantNet path: the identification carries a POWO
    // id, and an id lookup is exact where every name match is a heuristic.
    type XrefRef struct{ Authority, ID string }
    ```
    `MatchRequest.Xref *XrefRef`; Validierung in `matchNamesFiltered` VOR der Schleife (jede Zeile: genau eines gesetzt, sonst `fmt.Errorf`-Sentinel `ErrInvalidMatchRequest` mit Zeilen-ID); in `matchOne` als erster Zweig:
    ```go
    if req.Xref != nil {
        return matchByXref(ctx, repo, req, filter)
    }
    ```
    `matchByXref`: `ConceptByXref` → nil/NotFound → unresolvable-Result mit Note `noteXrefUnknown`; Filter prüfen (`filter.apply`-Äquivalent auf dem Einzelkonzept: BackboneID/SecReference gegen Filterfelder, sonst unresolvable mit `noteXrefFiltered`); sonst Result `{MatchType: domain.MatchXref, Confidence: 1.0, ConceptID: ...}`. Kein preferGenuineClaimants (Einzelkonzept per ID — es gibt keine Claimant-Menge).
  - `http`: DTO `matchNameDTO` um `Xref *struct{Authority, ID string}`; `ErrInvalidMatchRequest` → 400 `INVALID_QUERY`.
  - MatchInSpace-Anreicherung (TargetSpaceName etc.) läuft für Xref-Resultate über denselben Pfad wie exact (prüfen: sie hängt an ConceptID, nicht am MatchType — verifizieren und im Test pinnen).

- [ ] **Step 4: Tests** — `go test ./internal/application/ ./internal/adapters/http/ ./internal/domain/`, dann `go test ./...`, `make lint` → PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/domain/ internal/application/ internal/adapters/http/
git commit -m "feat(match): Batch-Auflösung über Fremd-IDs (powo u. a.) per xref-Eintrag"
```

---

### Task 2: E+M-UUID + Status in Zielraum-Antworten

**Files:**
- Modify: `internal/domain/namespace.go` (ResolveTargetSpace), alle Aufrufer (`git grep -n "ResolveTargetSpace(" internal/ | grep -v _test`), `internal/application/match.go`/`translate.go` (Felder durchreichen), `internal/adapters/http/taxa.go`/`translate.go` (DTOs)
- Test: bestehende ResolveTargetSpace-/MatchInSpace-/translate-Tests erweitern

**Interfaces:**
- Produces: `domain.ResolveTargetSpace(...) (TargetSpaceChoice, AggregatePolicy)` mit `TargetSpaceChoice{Name, ExtID, Status string}` (Name=="" heißt wie bisher „kein Eintrag"); `MatchResult.TargetSpaceExtID/-Status`; translate-DTO `name_space_translation.ext_id`/`.status`; match-DTO `target_space_ext_id`/`target_space_status` (omitempty).

- [ ] **Step 1: Fehlschlagende Tests** — bestehende ResolveTargetSpace-Tests auf den neuen Rückgabetyp heben (Namenswahl-Erwartungen UNVERÄNDERT — das ist die Byte-Identitäts-Garantie der Präzedenz), plus je ein neuer Fall: gewählter Eintrag liefert ExtID+Status korrekt (accepted-Fall und Synonym-Fallback-Fall — letzterer pinnt, dass Habitatus am Status erkennt, dass es ein E+M-Synonym bekommt); MatchInSpace-/Translate-Test: Response enthält `ext_id`/`status` (z. B. die reale UUID-Form aus dem Fixture).
- [ ] **Step 2: RED verifizieren.**
- [ ] **Step 3: Implementierung** — Rückgabetyp umstellen (alle Aufrufer mechanisch), Felder durch match/translate bis in die DTOs (omitempty; Feld-Doku: ext_id ist die Quell-ID des Namensraums — für eurosl die Euro+Med-TaxonUsage-UUID, stabiler Schlüssel für externe E+M-Ressourcen wie EuroVeg; name_space ist die Herkunft, eurosl == Euro+Med-PlantBase-Snapshot).
- [ ] **Step 4: Tests + lint.**
- [ ] **Step 5: Commit** — `feat(translate): Zielraum-Antworten tragen Quell-UUID und Status des gewählten Namens`

---

### Task 3: Synonymie-Schluss im Namespace-Ingest + CLI + CHANGELOG

**Files:**
- Modify: `internal/application/namespace_ingest.go` (NameRow.AcceptedTaxon, Schluss-Pass, Report), `internal/app/ingest.go` (nameSpaceRowSource reicht AcceptedTaxon durch — namelist.Row.AcceptedTaxon existiert), `cmd/hostus/ingest.go` (Zähler+Sample-Ausgabe), `CHANGELOG.md`
- Test: `internal/application/namespace_ingest_test.go`, `cmd/hostus/ingest_test.go`

**Interfaces:**
- Produces: `NameRow.AcceptedTaxon string`; `NameSpaceIngestReport.SynonymyClosed int` + `SynonymyClosedSample []string`; resolution-Marker `source_synonymy_closure`.

- [ ] **Step 1: Fehlschlagende Tests**

```go
// TestIngestNameSpace_SourceSynonymyClosesUnattachedAccepted pins spec
// 2026-09-13 decision 3 (the Inula-hirta class, 3719 of 47989 concepts on
// the real index): a source row the name crosswalk cannot place (its
// spelling has no accepted bearer in WCVP) is attached to the ONE concept
// its own source-synonymy group already resolved to.
//
// Fixture (WCVP): accepted "Pentanema hirtum" (ph1); its synonym rows make
// "Inula hirta" ambiguous-or-unmatched by name. Name space rows:
//   {Taxon:"Pentanema hirtum", AcceptedTaxon:"Inula hirta", Status:"synonymobjective", SourceID:"e-syn"}
//   {Taxon:"Inula hirta",      AcceptedTaxon:"Inula hirta", Status:"accepted",         SourceID:"e-acc"}
// (plus WCVP-Fixture so, dass "Inula hirta" per Name NICHT auflöst — zwei
// Synonym-Träger wie real, siehe TestIngestNameSpace_SynonymOnlyHomonymStaysAmbiguous-Muster.)
// Assertions: beide Zeilen attached an ph1; report.SynonymyClosed==1;
// Sample enthält "Inula hirta"; der geschlossene Eintrag trägt
// resolution=="source_synonymy_closure" und Status "accepted" — womit
// ResolveTargetSpace fortan den akzeptierten E+M-Namen wählt (genau der
// Habitatus-Gewinn; als Folge-Assertion via repo.NameSpaceEntries +
// domain.ResolveTargetSpace prüfen).

// TestIngestNameSpace_SynonymyClosureRefusesAmbiguousGroups: Gruppe, deren
// attachte Mitglieder auf ZWEI Konzepte zeigen -> Zeile bleibt offen,
// SynonymyClosed==0 (kein Raten).

// TestIngestNameSpace_SynonymyClosureNeverOverridesResolved: eine bereits
// per Name aufgelöste Zeile behält Konzept+resolution, auch wenn ihre
// Gruppe woandershin zeigt (Schluss-Pass füllt nur Lücken).
```

- [ ] **Step 2: RED verifizieren.**
- [ ] **Step 3: Implementierung** — `NameRow.AcceptedTaxon` (app-Wiring aus namelist.Row.AcceptedTaxon); in `IngestNameSpace` nach Phase 1 (Resolve) ein Schluss-Pass VOR der Write-Phase: Gruppen über `AcceptedTaxon`-Name bilden (leerer AcceptedTaxon → eigene Gruppe = eigener Taxon-Name, damit accepted-Zeilen und ihre Synonyme zusammenfallen — Quell-CSV-Konvention prüfen: bei accepted-Zeilen ist accepted_taxon leer oder Selbstverweis? `awk`-Stichprobe im Report dokumentieren); je Gruppe die Ziel-Konzepte der AUFGELÖSTEN Mitglieder einsammeln; genau eines → offene Mitglieder als resolved markieren (`traitResolution{conceptID, matched:true}` + Marker-Kennzeichnung, eigener Mechanismus neben rule/tieBroken — z. B. Feld `synonymyClosed bool` in traitResolution, `resolutionWithTieBreak`-Analog erweitern); Zähler/Sample/CLI wie beim Tie-Break-Muster.
- [ ] **Step 4: Tests + `go test ./...` + lint.**
- [ ] **Step 5: CHANGELOG** (### Added: Xref-Batch-Match mit PlantNet-Kontext; ext_id/status in Zielraum-Antworten; ### Fixed/Added: Synonymie-Schluss mit Messwert-Platzhalter, den der Controller nach dem Full-Ingest einsetzt — NEIN: Messwert erst in der Verifikation → Eintrag qualitativ formulieren, Zahl liefert der PR-Text).
- [ ] **Step 6: Commit** — `feat(ingest): Quell-Synonymie schließt offene Namespace-Zeilen (Inula-hirta-Klasse)`

---

## Verifikation nach Abschluss (Controller, kein Task)

1. `make verify`; Mutation-Gates: application, adapters/http, domain, adapters/namelist, cmd/hostus (alle im normalen CI-Gate — lokale Läufe für application + http zur Sicherheit).
2. Full-Ingest: `SynonymyClosed`-Zahlen je Space; die 3.719er-Lücke neu messen (Ziel: deutlich reduziert); Stichprobe 20 geschlossene Zeilen (`resolution='source_synonymy_closure'`) dem Maintainer vorlegen.
3. E2E PlantNet-Form: `POST /v1/match {"names":[{"id":"1","xref":{"authority":"powo","id":"77178414-1"}}],"target_space":"eurosl"}` → `match_type:"xref"`, `target_space_name:"Inula hirta"` (nach Schluss-Pass!), `target_space_status:"accepted"`, `target_space_ext_id:"a08253f0-…"`.

# Ambiguitäts-Konsistenz-Fixes Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ingest-Crosswalk und Serving-Pfad benutzen dieselbe Zwei-Stufen-Claimant-Präferenz; CDM-Konzepte ohne sec werden synthetisch attribuiert; POST /v1/match und /v1/translate bekommen Body-/Batch-Limits.

**Architecture:** Eine gemeinsame Funktion `preferGenuineClaimants` in `internal/application/crosswalk.go` (Stufe 1: non-sec vor sec; Stufe 2: Taxonomie-Backbone vor Name-Space-nativ), gespeist aus dem bestehenden Port `Repository.NameSpaces`. Der Serving-Pfad wendet sie nur bei leerem `MatchFilter` an. Kein Schema-, kein Port-Umbau.

**Tech Stack:** Go 1.26, modernc.org/sqlite (bestehend), keine neuen Dependencies.

**Spec:** `docs/superpowers/specs/2026-09-01-ambiguity-consistency-fixes.md`

## Global Constraints

- `internal/application` und `internal/adapters/http` sind mutation-gated: jede neue Verzweigung braucht einen Test, der den Mutanten tötet (`make mutation PKG=<pkg>`; kein `Not covered`-Mutant).
- Hexagon-Grenzen (depguard): `internal/application` importiert KEINE `internal/adapters/*`-Pakete.
- Fehlercodes ausschließlich aus CLAUDE.md-Liste; hier nur `INVALID_QUERY`. Format `{"error":{"code":"...","message":"..."}}` via `internal/httperr`.
- Der ungefilterte Serving-Pfad mit gesetztem `entry_backbone`/`entry_sec` bleibt byte-identisch (Präferenz läuft NUR bei `filter.empty()`).
- Fallback-Invariante beider Präferenz-Stufen: Bleibt nach dem Filtern NICHTS übrig, wird die ungefilterte Kandidatenmenge zurückgegeben (ein Name, den nur ein sec-/natives Konzept trägt, löst weiter auf).
- Conventional Commits (`fix:`/`feat:`/`test:`); CHANGELOG unter `## [Unreleased]`.
- Doc-Kommentare im Stil des Pakets: WARUM, mit Messwerten aus der Spec.

---

### Task 1: `preferGenuineClaimants` (Stufe 2) im Ingest-Crosswalk

**Files:**
- Modify: `internal/application/crosswalk.go` (neue Funktion + `resolveTraitName`-Signatur)
- Modify: `internal/application/namespace_ingest.go` (`resolveNameSpaceNames` lädt Set einmal)
- Test: `internal/application/namespace_ingest_test.go`, `internal/application/crosswalk_internal_test.go` (neu, falls nicht vorhanden)

**Interfaces:**
- Consumes: `output.Repository.NameSpaces(ctx) ([]domain.NameSpaceMeta, error)` (bestehender Port, `internal/ports/output/repository.go:155`).
- Produces (für Task 2):
  - `func preferGenuineClaimants(candidates []output.MatchCandidate, nativeSpaces map[string]bool) []output.MatchCandidate`
  - `func nativeSpaceSet(ctx context.Context, repo output.Repository) (map[string]bool, error)`
  - Geänderte Signatur: `func resolveTraitName(ctx context.Context, repo output.Repository, canon string, policy crosswalkPolicy, nativeSpaces map[string]bool) (traitResolution, error)`

- [ ] **Step 1: Fehlschlagende Tests schreiben**

In `internal/application/namespace_ingest_test.go` (Muster: bestehende Tests dort, `openMemoryRepo`, `fakeRowSource` aus `ingest_test.go`; für native Konzepte `application.IngestNativeSpace` mit einem Fake, der `application.NativeRowSource` erfüllt — existiert einer in `nativespace_ingest_internal_test.go`/den externen Tests, diesen wiederverwenden, sonst analog `fakeRowSource` anlegen):

```go
// TestIngestNameSpace_NativeConceptDoesNotShadowBackboneGenus pinnt den
// Fall-B-Befund des Audits (2026-09-01, Spec B2): eurosl legt native
// GENUS-Konzepte auch für Gattungen an, die WCVP führt ("Abies", "Acer",
// 2866 gemessene Folds). Ein danach gecrosswalkter Name-Space (germansl)
// muss die Gattung trotzdem auf das WCVP-Konzept auflösen — gemessen
// verlor germansl ~544 Gattungs-Einträge (417 vs. 961 auf identischer
// Liste), rein reihenfolgeabhängig.
func TestIngestNameSpace_NativeConceptDoesNotShadowBackboneGenus(t *testing.T) {
	repo := openMemoryRepo(t)
	ctx := context.Background()

	// 1. WCVP-artiges Backbone mit der Gattung.
	ds := &application.Dataset{Backbones: []application.Backbone{{ID: "wcvp", Version: "v1"}}, ManifestSHA: "x"}
	taxa := []application.TaxonRow{
		{TaxonID: "g1", AcceptedTaxonID: "g1", Accepted: true, Canonical: "Abies", Rank: "GENUS", Status: "Accepted"},
	}
	readerFor := func(application.Backbone) (application.RowSource, error) {
		return fakeRowSource{taxa: taxa}, nil
	}
	if _, err := application.Ingest(ctx, ds, readerFor, repo); err != nil {
		t.Fatalf("Ingest: %v", err)
	}

	// 2. eurosl Fall B: natives GENUS-Konzept gleichen Namens.
	bv := domain.BackboneVersion{ID: "eurosl", Version: "v1"}
	native := fakeNativeRowSource{rows: []application.NativeRow{
		{Taxon: "Abies", SourceID: "e1", Rank: "Genus", Status: "accepted"},
	}}
	if _, err := application.IngestNativeSpace(ctx, repo, native, bv, domain.RankRoot, nil); err != nil {
		t.Fatalf("IngestNativeSpace: %v", err)
	}
	// eurosl muss auch als name_space registriert sein, damit nativeSpaceSet
	// es kennt — im echten Ingest passiert das durch eurosls eigenen
	// Fall-A-Lauf (IngestNameSpace -> UpsertNameSpace).
	if _, err := application.IngestNameSpace(ctx, repo, fakeNameRowSource{}, domain.NameSpaceMeta{ID: "eurosl", Version: "v1"}); err != nil {
		t.Fatalf("IngestNameSpace(eurosl): %v", err)
	}

	// 3. germansl Fall A: "Abies" muss aufs WCVP-Konzept auflösen, nicht
	//    ambiguous sein.
	report, err := application.IngestNameSpace(ctx, repo,
		fakeNameRowSource{rows: []application.NameRow{{Taxon: "Abies", SourceID: "g-1", Status: "accepted"}}},
		domain.NameSpaceMeta{ID: "germansl", Version: "v1"})
	if err != nil {
		t.Fatalf("IngestNameSpace(germansl): %v", err)
	}
	if report.Ambiguous != 0 || report.Matched != 1 {
		t.Fatalf("report = matched %d / ambiguous %d, want 1/0", report.Matched, report.Ambiguous)
	}
	entries, err := repo.NameSpaceEntries(ctx, "wcvp:concept:g1", []string{"germansl"})
	if err != nil || len(entries) != 1 {
		t.Fatalf("NameSpaceEntries(wcvp:concept:g1) = %v, %v — der Eintrag muss am WCVP-Konzept hängen", entries, err)
	}
}

// TestIngestNameSpace_NativeOnlyNameStillResolves pinnt die
// Fallback-Invariante: eine Gattung, die NUR als natives Konzept existiert
// (Moos-Gattung "Abietinella" — WCVP führt keine Moose), muss weiterhin auf
// dieses native Konzept auflösen; Stufe 2 darf sie nicht verwerfen.
func TestIngestNameSpace_NativeOnlyNameStillResolves(t *testing.T) {
	repo := openMemoryRepo(t)
	ctx := context.Background()
	bv := domain.BackboneVersion{ID: "eurosl", Version: "v1"}
	native := fakeNativeRowSource{rows: []application.NativeRow{
		{Taxon: "Abietinella", SourceID: "e2", Rank: "Genus", Status: "accepted"},
	}}
	if _, err := application.IngestNativeSpace(ctx, repo, native, bv, domain.RankRoot, nil); err != nil {
		t.Fatalf("IngestNativeSpace: %v", err)
	}
	if _, err := application.IngestNameSpace(ctx, repo, fakeNameRowSource{}, domain.NameSpaceMeta{ID: "eurosl", Version: "v1"}); err != nil {
		t.Fatalf("IngestNameSpace(eurosl): %v", err)
	}
	report, err := application.IngestNameSpace(ctx, repo,
		fakeNameRowSource{rows: []application.NameRow{{Taxon: "Abietinella", SourceID: "g-2", Status: "accepted"}}},
		domain.NameSpaceMeta{ID: "germansl", Version: "v1"})
	if err != nil {
		t.Fatalf("IngestNameSpace(germansl): %v", err)
	}
	if report.Matched != 1 {
		t.Fatalf("report.Matched = %d, want 1 — native-only Namen müssen weiter auflösen", report.Matched)
	}
}
```

`fakeNameRowSource`/`fakeNativeRowSource` (nur falls nicht vorhanden — vorher suchen):

```go
type fakeNameRowSource struct{ rows []application.NameRow }

func (f fakeNameRowSource) Rows() []application.NameRow { return f.rows }

type fakeNativeRowSource struct{ rows []application.NativeRow }

func (f fakeNativeRowSource) Rows() []application.NativeRow { return f.rows }
```

Zusätzlich Unit-Tests für die Funktion selbst (interner Test, da unexportiert), `internal/application/crosswalk_internal_test.go`, Tabellen-Test mit den vier Fällen: (a) backbone+nativ → nur backbone, (b) nur nativ → unverändert, (c) sec+nativ → nur nativ (Stufe 1 zuerst), (d) leeres `nativeSpaces`-Set → identisch zu `preferBackboneConcepts`.

- [ ] **Step 2: Tests laufen lassen, Fehlschlag verifizieren**

Run: `go test ./internal/application/ -run 'TestIngestNameSpace_Native|TestPreferGenuineClaimants' -v`
Expected: FAIL (Funktion existiert nicht / report.Ambiguous == 1).

- [ ] **Step 3: Implementierung**

In `crosswalk.go` (nach `preferBackboneConcepts`):

```go
// preferGenuineClaimants is the shared two-tier claimant preference both the
// ingest crosswalk (resolveTraitName) and the serving path (match.go) apply
// before counting distinct concepts:
//
//	tier 1: preferBackboneConcepts — sec.-space candidates are dropped when a
//	        non-sec candidate holds the name (PR #94, see that function).
//	tier 2: name-space-NATIVE candidates (Fall B concepts eurosl/germansl
//	        write for ranks above SPECIES, nativespace_ingest.go) are dropped
//	        when a genuine taxonomic-backbone concept remains. A native
//	        concept carries SecReference == "" and is therefore invisible to
//	        tier 1 — measured on a full real ingest (2026-09-01, spec B2):
//	        2866 GENUS + 319 FAMILY folds held both a WCVP and a native
//	        concept, costing germansl ~544 genus entries purely by ingest
//	        order. nativeSpaces is the set of name_space ids (Repository.
//	        NameSpaces) — floraveg is in it and harmless (it writes no
//	        native concepts, so no candidate ever carries its id).
//
// Both tiers share the same fallback: a name that ONLY the filtered class
// carries keeps its candidates (single native -> matched, several -> the
// base ambiguity rule), so bryophyte genera existing solely as native
// concepts (Abietinella) keep resolving.
func preferGenuineClaimants(candidates []output.MatchCandidate, nativeSpaces map[string]bool) []output.MatchCandidate {
	candidates = preferBackboneConcepts(candidates)
	if len(nativeSpaces) == 0 {
		return candidates
	}
	genuine := make([]output.MatchCandidate, 0, len(candidates))
	for _, c := range candidates {
		if !nativeSpaces[c.Concept.BackboneID] {
			genuine = append(genuine, c)
		}
	}
	if len(genuine) == 0 {
		return candidates
	}
	return genuine
}

// nativeSpaceSet loads the name-space id set preferGenuineClaimants consults,
// exactly once per crosswalk run / match batch — never per name.
func nativeSpaceSet(ctx context.Context, repo output.Repository) (map[string]bool, error) {
	spaces, err := repo.NameSpaces(ctx)
	if err != nil {
		return nil, err
	}
	set := make(map[string]bool, len(spaces))
	for _, s := range spaces {
		set[s.ID] = true
	}
	return set, nil
}
```

`resolveTraitName`: Parameter `nativeSpaces map[string]bool` ergänzen; die Zeile `candidates = preferBackboneConcepts(candidates)` wird zu `candidates = preferGenuineClaimants(candidates, nativeSpaces)` (weiterhin unter beiden Policies). Doc-Kommentar entsprechend erweitern (Stufe 2 benennen, Spec referenzieren).

`resolveNameSpaceNames`: vor der Schleife einmal laden und durchreichen:

```go
	nativeSpaces, err := nativeSpaceSet(ctx, repo)
	if err != nil {
		return nil, fmt.Errorf("application: loading name-space set: %w", err)
	}
```
und `resolveTraitName(ctx, repo, canon, policyPreferBackbone, nativeSpaces)`.

WICHTIG (Selbst-Shadowing): Beim Re-Ingest desselben Space steht der eigene
Space bereits im Set — die eigenen Fall-B-Konzepte aus Lauf 1 werden dadurch
korrekt demotet (Re-Ingest-Idempotenz, Spec B2). Kein Sonderfall nötig.

- [ ] **Step 4: Tests laufen lassen**

Run: `go test ./internal/application/ -v -run 'TestIngestNameSpace|TestPreferGenuineClaimants'` und danach `go test ./internal/application/`
Expected: PASS, keine Regressionen (insb. `TestIngestNameSpace_HomonymStaysAmbiguousHere` und `TestIngestNameSpace_SecReferenceCandidateDoesNotCauseAmbiguous` unverändert grün).

- [ ] **Step 5: Commit**

```bash
git add internal/application/crosswalk.go internal/application/namespace_ingest.go internal/application/namespace_ingest_test.go internal/application/crosswalk_internal_test.go
git commit -m "fix(crosswalk): Fall-B-native Konzepte verschatten Backbone-Konzepte nicht mehr"
```

---

### Task 2: Zwei-Stufen-Präferenz im Serving-Pfad (/v1/match, /v1/translate)

**Files:**
- Modify: `internal/application/match.go` (`matchNamesFiltered`, `matchOne`, `matchAggregate`, `matchAggregateNominate`, `matchFuzzy`)
- Test: `internal/application/match_test.go` (bestehende Muster), `internal/application/translate_test.go`

**Interfaces:**
- Consumes: `preferGenuineClaimants`, `nativeSpaceSet` (Task 1).
- Produces: keine neuen Exporte; interne Signaturen erhalten den Parameter `nativeSpaces map[string]bool`.

- [ ] **Step 1: Fehlschlagende Tests schreiben**

In `match_test.go` (Fixtures über `openMemoryRepo` + `application.Ingest`/`application.IngestCDM`/`application.IngestNativeSpace` aufbauen, wie in Task 1; öffentliche API `application.MatchNames` testen):

```go
// TestMatchNames_SecSpaceConceptDoesNotCauseAmbiguity pinnt Audit-Befund B1
// (e2e gemessen: POST /v1/match lieferte "unresolvable" für Pinus
// sylvestris, sobald CDM geladen ist): ein sec.-Space-Konzept ist ein
// Attributions-Detail, kein zweiter Claimant — dieselbe Regel, die der
// Ingest-Crosswalk seit PR #94 anwendet.
func TestMatchNames_SecSpaceConceptDoesNotCauseAmbiguity(t *testing.T) {
	repo := openMemoryRepo(t)
	ctx := context.Background()
	// WCVP: akzeptierte Art.
	// CDM: dieselbe Schreibung als accepted-Konzept in einem sec-Space.
	// (Aufbau wie in den bestehenden CDM-Tests dieses Pakets.)
	// ... Fixture-Aufbau ...
	results, err := application.MatchNames(ctx, repo, []application.MatchRequest{{ID: "1", Verbatim: "Pinus sylvestris"}})
	if err != nil {
		t.Fatalf("MatchNames: %v", err)
	}
	if results[0].ConceptID != "wcvp:concept:ps1" {
		t.Fatalf("ConceptID = %q, want wcvp:concept:ps1 (sec-Konzept darf nicht als Claimant zählen); Note=%q", results[0].ConceptID, results[0].Note)
	}
}

// TestMatchNames_EntrySecFilterStillReachesSecConcept pinnt Entscheidung 2
// der Spec: mit explizitem entry_sec bleibt das sec-Konzept erreichbar —
// die Präferenz läuft NUR bei leerem Filter.
func TestMatchNames_EntrySecFilterStillReachesSecConcept(t *testing.T) {
	// gleiche Fixture; MatchInSpace bzw. matchNamesFiltered mit
	// MatchFilter{Sec: "<sec-id>"} -> ConceptID == cdm-Konzept.
}

// TestMatchNames_NativeGenusConceptDoesNotCauseAmbiguity pinnt B2 für den
// Serving-Pfad: "Acer"/"Abies" (WCVP-Gattung + eurosl-natives Konzept)
// muss aufs WCVP-Konzept auflösen. Fixture wie Task 1.
func TestMatchNames_NativeGenusConceptDoesNotCauseAmbiguity(t *testing.T) { /* analog */ }

// TestTranslate_VerbatimResolvesDespiteSecSpaces (translate_test.go): der
// Verbatim-Einstieg von /v1/translate läuft durch matchNamesFiltered und
// erbt den Fix — vorher 422 UNRESOLVABLE (Audit B1). Fixture wie oben,
// Assertion: kein ErrUnresolvableName.
```

Fixture-Aufbau und exakte Concept-IDs an die bestehenden Tests des Pakets anpassen (`git grep -n "IngestCDM" internal/application/*_test.go` zeigt das Muster); die vier Assertions oben sind verbindlich.

- [ ] **Step 2: Fehlschlag verifizieren**

Run: `go test ./internal/application/ -run 'TestMatchNames_Sec|TestMatchNames_Native|TestMatchNames_Entry|TestTranslate_Verbatim' -v`
Expected: FAIL (heute: leere ConceptID + Ambiguitäts-Note bzw. ErrUnresolvableName).

- [ ] **Step 3: Implementierung**

`matchNamesFiltered` (match.go:288): nach `validateFilter` einmal laden:

```go
	nativeSpaces, err := nativeSpaceSet(ctx, repo)
	if err != nil {
		return nil, err
	}
```
und an `matchOne` durchreichen. `matchOne`, `matchAggregate`, `matchAggregateNominate`, `matchFuzzy` erhalten den Parameter `nativeSpaces map[string]bool`. An den vier Stellen, an denen heute `candidates = filter.apply(candidates)` steht (match.go:549, :698, :850, :935), direkt danach:

```go
	if filter.empty() {
		// Dieselbe Zwei-Stufen-Präferenz wie der Ingest-Crosswalk
		// (preferGenuineClaimants, Spec Entscheidung 1+2): ein sec.-Space-
		// oder Fall-B-natives Konzept ist kein zweiter Claimant. Mit
		// gesetztem entry_backbone/entry_sec hat der Caller den Raum
		// gepinnt — dann läuft KEINE Präferenz, das Ergebnis bleibt
		// byte-identisch zum bisherigen gefilterten Pfad.
		candidates = preferGenuineClaimants(candidates, nativeSpaces)
	}
```

(Den erklärenden Kommentar nur an der ersten Stelle in voller Länge; an den drei anderen ein Einzeiler mit Verweis.) In `matchFuzzy` läuft die Präferenz VOR dem Scoring (auf den Prefilter-Pool), sodass Near-Miss-Listen und Winner-Ties dieselbe Kandidatenbasis sehen. Den veralteten Kommentar bei `genuineBearerWinner`/classify (match.go:1100-1103, „the tie legitimately stands" für sec-Spaces) an die neue Realität anpassen.

- [ ] **Step 4: Tests laufen lassen**

Run: `go test ./internal/application/` — Expected: PASS inkl. aller Bestandstests (besonders die MatchFilter-/entry_sec-Tests: gefilterte Pfade byte-identisch).

- [ ] **Step 5: Commit**

```bash
git add internal/application/match.go internal/application/match_test.go internal/application/translate_test.go
git commit -m "fix(match): Serving-Pfad wendet dieselbe Claimant-Präferenz an wie der Ingest-Crosswalk"
```

---

### Task 3: Synthetische sec-Referenz für CDM-Konzepte ohne sec_uuid

**Files:**
- Modify: `internal/application/cdm_ingest.go` (`planCDMConcepts`)
- Test: `internal/application/cdm_ingest_test.go` (bestehende Muster dort)

**Interfaces:**
- Consumes: nichts Neues.
- Produces: Konstante `cdmUnattributedSecID = "cdm:unattributed"` (paketintern).

- [ ] **Step 1: Fehlschlagenden Test schreiben**

```go
// TestIngestCDM_ConceptWithoutSecGetsSyntheticSec pinnt Audit-Befund B3:
// 124 CDM-Zeilen ohne sec_uuid wurden mit SecReference=="" geschrieben und
// zählten damit als Backbone-Claimants — "Leucanthemum maximum" fehlte
// deshalb in ALLEN drei Name-Spaces. Ohne sec-Angabe bekommt ein Konzept
// jetzt die synthetische Referenz cdm:unattributed: weiterhin erreichbar
// (entry_sec, Relationen), aber nie mehr ein Ambiguitätskandidat gegen ein
// echtes Backbone-Konzept.
func TestIngestCDM_ConceptWithoutSecGetsSyntheticSec(t *testing.T) {
	repo := openMemoryRepo(t)
	ctx := context.Background()
	concepts := []application.CDMConceptRow{
		{ConceptUUID: "u1", ScientificName: "Leucanthemum maximum", Rank: "Species", Status: "accepted"}, // SecUUID leer
	}
	report, err := application.IngestCDM(ctx, repo, concepts, nil, domain.ConceptSourceMeta{ID: "cdm", Version: "v1"})
	if err != nil {
		t.Fatalf("IngestCDM: %v", err)
	}
	if report.ConceptsWithoutSec != 1 {
		t.Errorf("ConceptsWithoutSec = %d, want 1 (der Zähler bleibt)", report.ConceptsWithoutSec)
	}
	sec, err := repo.SecReferenceByID(ctx, "cdm:unattributed")
	if err != nil {
		t.Fatalf("SecReferenceByID(cdm:unattributed): %v", err)
	}
	if sec.Title == "" {
		t.Errorf("synthetische sec-Referenz ohne Titel")
	}
	// Kandidat trägt jetzt die synthetische sec — MatchExact-Kandidaten
	// dieses Konzepts fallen unter preferBackboneConcepts' Stufe 1.
	cands, err := repo.MatchExact(ctx, "leucanthemum maximum")
	if err != nil || len(cands) != 1 {
		t.Fatalf("MatchExact: %v, %v", cands, err)
	}
	if cands[0].Concept.SecReference != "cdm:unattributed" {
		t.Errorf("SecReference = %q, want cdm:unattributed", cands[0].Concept.SecReference)
	}
}
```

(`ConceptSourceMeta`/Meta-Typ und Feldnamen an die tatsächliche `IngestCDM`-Signatur anpassen — `git grep -n "func IngestCDM" internal/application/`.)

- [ ] **Step 2: Fehlschlag verifizieren**

Run: `go test ./internal/application/ -run TestIngestCDM_ConceptWithoutSec -v` — Expected: FAIL (SecReference == "").

- [ ] **Step 3: Implementierung**

In `planCDMConcepts` (cdm_ingest.go:302-314), Konstante oben im File:

```go
// cdmUnattributedSecID is the SYNTHETIC sec reference assigned to CDM rows
// whose sec_uuid is empty (124 rows on the real export, spec B3). Writing
// them with SecReference=="" made them indistinguishable from backbone
// concepts for preferBackboneConcepts — the exact ambiguity class PR #94
// fixed, in residual form: "Leucanthemum maximum" was lost in every name
// space. The synthetic space keeps them reachable (entry_sec, relations)
// without letting them claim a spelling against a genuine backbone concept.
const cdmUnattributedSecID = "cdm:unattributed"
```

Im Schleifenkörper den `row.SecUUID == ""`-Zweig ersetzen:

```go
		if row.SecUUID == "" {
			report.ConceptsWithoutSec++
			row.SecUUID = cdmUnattributedSecID
			if !seenSec[cdmUnattributedSecID] {
				seenSec[cdmUnattributedSecID] = true
				plan.secs = append(plan.secs, domain.SecReference{ID: cdmUnattributedSecID, Title: "CDM ohne sec-Referenz (synthetisch)"})
			}
		} else if !seenSec[row.SecUUID] {
			seenSec[row.SecUUID] = true
			plan.secs = append(plan.secs, domain.SecReference{ID: row.SecUUID, Title: row.SecTitle})
		}
```

(`row` ist die Schleifen-Kopie — die Zuweisung ist lokal; `SecReference: row.SecUUID` weiter unten übernimmt den synthetischen Wert.)

- [ ] **Step 4: Tests laufen lassen**

Run: `go test ./internal/application/` — Expected: PASS; bestehende CDM-Tests (SecReferences-Zählung!) prüfen und, wo der Zähler sich um die synthetische Referenz erhöht, die Erwartung MIT Begründung im Test anpassen.

- [ ] **Step 5: Commit**

```bash
git add internal/application/cdm_ingest.go internal/application/cdm_ingest_test.go
git commit -m "fix(cdm): Konzepte ohne sec_uuid bekommen synthetische sec-Referenz statt Backbone-Status"
```

---

### Task 4: Body- und Batch-Limits für POST /v1/match und /v1/translate

**Files:**
- Modify: `internal/adapters/http/taxa.go` (`handleMatch`), `internal/adapters/http/translate.go` (Handler, Decode-Stelle Zeile ~186-190)
- Test: `internal/adapters/http/taxa_test.go`, `internal/adapters/http/translate_test.go` (httptest-Muster der bestehenden Handler-Tests)

**Interfaces:**
- Consumes: `httperr.InvalidQueryError(w, msg)` (bestehend).
- Produces: Konstanten `maxMatchBodyBytes = 1 << 20`, `maxMatchNames = 1000`, `maxTranslateBodyBytes = 64 << 10` (paketintern).

- [ ] **Step 1: Fehlschlagende Tests schreiben**

```go
// TestHandleMatch_RejectsOversizedBatch pinnt Audit-Befund B4 (DoS): ein
// Batch über maxMatchNames wird mit 400 INVALID_QUERY abgelehnt statt
// sequenziell abgearbeitet (pro Name bis zu 20k-Kandidaten-Levenshtein).
func TestHandleMatch_RejectsOversizedBatch(t *testing.T) {
	names := make([]map[string]string, 1001)
	for i := range names {
		names[i] = map[string]string{"id": strconv.Itoa(i), "verbatim": "x"}
	}
	body, _ := json.Marshal(map[string]any{"names": names})
	// httptest-Request gegen den gemounteten Handler (Muster der
	// bestehenden handleMatch-Tests), Assertions:
	//   Status 400, error.code == "INVALID_QUERY",
	//   Message nennt das Limit (1000).
}

// TestHandleMatch_ExactlyAtLimitStillServed: 1000 Namen -> 200 (Boundary,
// tötet den CONDITIONALS_BOUNDARY-Mutanten am > -Vergleich).

// TestHandleMatch_RejectsOversizedBody: Body > 1 MiB -> 400 INVALID_QUERY
// (MaxBytesReader schlägt im Decoder auf).

// TestHandleTranslate_RejectsOversizedBody: Body > 64 KiB -> 400.
```

- [ ] **Step 2: Fehlschlag verifizieren**

Run: `go test ./internal/adapters/http/ -run 'TestHandleMatch_Rejects|TestHandleMatch_Exactly|TestHandleTranslate_Rejects' -v`
Expected: FAIL (heute 200 bzw. voller Durchlauf).

- [ ] **Step 3: Implementierung**

Konstanten mit Doc-Kommentar (warum 1000/1 MiB: Suggest/Synonyms haben Caps, Match hatte keinen; Spec B4). In `handleMatch` als ERSTE Zeilen des Handlers:

```go
		r.Body = http.MaxBytesReader(w, r.Body, maxMatchBodyBytes)
		var body matchRequestDTO
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			httperr.InvalidQueryError(w, "malformed request body")
			return
		}
		if len(body.Names) > maxMatchNames {
			httperr.InvalidQueryError(w, fmt.Sprintf("too many names: %d (limit %d)", len(body.Names), maxMatchNames))
			return
		}
```

(Der MaxBytesReader-Fehler landet im Decode-Fehler → bestehende 400-Antwort; kein neuer Fehlerpfad nötig. `fmt` ggf. importieren.) Analog in `translate.go` vor dem Decode: `r.Body = http.MaxBytesReader(w, r.Body, maxTranslateBodyBytes)`.

- [ ] **Step 4: Tests laufen lassen**

Run: `go test ./internal/adapters/http/` — Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/adapters/http/taxa.go internal/adapters/http/translate.go internal/adapters/http/taxa_test.go internal/adapters/http/translate_test.go
git commit -m "fix(http): Body- und Batch-Limits für /v1/match und /v1/translate (DoS-Härtung)"
```

---

### Task 5: CHANGELOG

**Files:**
- Modify: `CHANGELOG.md` (unter `## [Unreleased]`, Abschnitte `### Fixed` und `### Security`)

- [ ] **Step 1: Einträge schreiben**

Unter `## [Unreleased]` (Abschnitt anlegen, falls nach dem letzten Release keiner existiert):

```markdown
### Fixed
- Serving-Pfad (`POST /v1/match`, `POST /v1/translate`) wendet jetzt dieselbe
  Zwei-Stufen-Claimant-Präferenz an wie der Ingest-Crosswalk: sec.-Space-
  Konzepte (PR #94-Regel) und Fall-B-native Name-Space-Konzepte zählen nicht
  mehr als gleichrangige Ambiguitätskandidaten. Vorher lieferte /v1/match auf
  einem Index mit CDM für gemeine Arten („Pinus sylvestris", „Abies alba",
  „Acer") `unresolvable`; /v1/translate antwortete 422. Mit gesetztem
  `entry_backbone`/`entry_sec` bleibt das Verhalten unverändert.
- Namespace-Crosswalk: Fall-B-native Konzepte (eurosl/germansl, Ränge oberhalb
  SPECIES) verschatten Backbone-Konzepte gleichen Namens nicht mehr. Gemessen
  verlor germansl dadurch ~544 Gattungs-Einträge (417 statt 961 aufgelöst),
  rein abhängig von der Ingest-Reihenfolge; Re-Ingest ist jetzt idempotent.
- CDM-Konzepte ohne sec_uuid (124 auf dem realen Export) erhalten die
  synthetische sec-Referenz `cdm:unattributed` statt als Backbone-Konzepte
  zu zählen — „Leucanthemum maximum", „Papaver lecoqii", „Rubus affinis"
  lösen in den Name-Spaces wieder auf.

### Security
- `POST /v1/match` begrenzt Request-Body (1 MiB) und Batch-Größe (1000
  Namen), `POST /v1/translate` den Body (64 KiB); Überschreitung → 400
  `INVALID_QUERY`. Vorher wurden beliebig große Batches voll dekodiert und
  sequenziell verarbeitet (DoS-Vektor).
```

- [ ] **Step 2: Commit**

```bash
git add CHANGELOG.md
git commit -m "docs(changelog): Ambiguitäts-Konsistenz-Fixes und Match-Limits dokumentieren"
```

---

## Verifikation nach Abschluss (Controller, kein Task)

1. `make verify` (fmt, vet, lint, test, arch, debt-guard, compile).
2. `make mutation PKG=./internal/application` und `make mutation PKG=./internal/adapters/http` — kein `Not covered`.
3. Realer Full-Ingest (`dataset.full-deploy.yaml`) + Vergleich gegen die Audit-Messwerte: germansl-Gattungs-Einträge ≥ 900 (vorher 417); „Leucanthemum maximum" in allen drei Spaces; `POST /v1/match` löst „Pinus sylvestris", „Abies alba", „Acer" auf.

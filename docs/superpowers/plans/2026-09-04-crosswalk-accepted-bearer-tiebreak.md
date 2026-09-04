# Tier-1-Tie-Break im Namespace-Crosswalk Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Der Namespace-Crosswalk löst die „1 accepted-Träger + Synonym-Träger"-Homonymklasse (Abies-alba-Klasse, 4716 eurosl-Folds) per Tier-1-Tie-Break auf — voll auditierbar über Report-Zähler, Sample und `resolution`-Marker.

**Architecture:** Neue Policy `policyResolveAcceptedBearer` in `crosswalk.go`; Tier 1 wird als `acceptedBearerWinner` neben `genuineBearerWinner` aus denselben Bausteinen (`soleConcept`, `roleAccepted`) gebaut — kein Regel-Klon. Serving-Pfad und Alt-Policies bleiben byte-identisch.

**Tech Stack:** Go 1.26, bestehende Test-Infrastruktur (`openMemoryRepo`, `fakeRowSource`, `fakeNameRowSource`).

**Spec:** `docs/superpowers/specs/2026-09-04-crosswalk-accepted-bearer-tiebreak.md`

## Global Constraints

- `internal/application` und `cmd/hostus` sind mutation-gated: jede neue Verzweigung braucht einen mutanten-tötenden Test (insb. beide Zweige des resolution-Marker-Helpers und die „mehrere accepted → ambiguous"-Grenze).
- `policyPreferBackbone` und `policyResolveGenuineBearer` bleiben in Verhalten UND Tests unverändert; der Serving-Pfad (match.go classify) wird nicht angefasst (nur die additive Funktion `acceptedBearerWinner` kommt in match.go dazu).
- Report-Invariante `Matched + Unmatched + Ambiguous == Rows` bleibt bestehen; `TieBroken` ist Teilmenge von `Matched`.
- Marker-Strings exakt: `"accepted_bearer_tiebreak"` bzw. `"<rule>+accepted_bearer_tiebreak"`.
- Ein AMBIGUOUS-Ausgang stoppt die NameCandidates-Leiter weiterhin.
- CHANGELOG unter `## [Unreleased]`; Conventional Commits; englische WHY-Doc-Kommentare mit den Spec-Messwerten.

---

### Task 1: Policy, Tie-Break, Report und Marker in `internal/application`

**Files:**
- Modify: `internal/application/crosswalk.go` (neue Policy-Konstante, resolveTraitName-Zweig, Marker-Helper)
- Modify: `internal/application/match.go` (nur additiv: `acceptedBearerWinner` neben `genuineBearerWinner`, match.go:1156ff)
- Modify: `internal/application/namespace_ingest.go` (Policy-Wechsel am Call-Site, Report-Felder, Tally, writeNameSpaceRow)
- Test: `internal/application/namespace_ingest_test.go`, `internal/application/crosswalk_internal_test.go`

**Interfaces:**
- Consumes: `soleConcept`, `roleAccepted`, `classifiedHit`, `traitBearers`, `traitResolution.tieBroken` (alle bestehend).
- Produces (für Task 2): `NameSpaceIngestReport.TieBroken int`, `NameSpaceIngestReport.TieBrokenSample []string`.

**Vorprüfung (Teil von Step 1):** `git grep -n "\.Resolution" internal/ cmd/ | grep -v _test` — jeder Konsument von `name_space_entry.resolution` muss den Wert als opaken String behandeln (Export, Bundle). Behandelt einer ihn semantisch (parst Rule-Namen), STOPP → BLOCKED melden.

- [ ] **Step 1: Fehlschlagende Tests schreiben**

In `namespace_ingest_test.go` (Fixtures via `openMemoryRepo` + `application.Ingest` mit `fakeRowSource` + `application.IngestNameSpace` mit `fakeNameRowSource` — alle vorhanden):

```go
// TestIngestNameSpace_AcceptedBearerTieBreakResolvesHomonym pins the spec's
// core decision (2026-09-04): a spelling held as ACCEPTED by exactly one
// concept and as a mere synonym by others resolves to the accepted bearer —
// the Abies-alba class (4716 eurosl folds measured 2026-09-01). The outcome
// must be fully auditable: report counter, sample, and resolution marker.
func TestIngestNameSpace_AcceptedBearerTieBreakResolvesHomonym(t *testing.T) {
	repo := openMemoryRepo(t)
	ctx := context.Background()
	ds := &application.Dataset{Backbones: []application.Backbone{{ID: "wcvp", Version: "v1"}}, ManifestSHA: "x"}
	taxa := []application.TaxonRow{
		// Concept A: bears "Abies alba" as its ACCEPTED name.
		{TaxonID: "a1", AcceptedTaxonID: "a1", Accepted: true, Canonical: "Abies alba", Rank: "SPECIES", Status: "Accepted"},
		// Concept B: a different accepted taxon...
		{TaxonID: "b1", AcceptedTaxonID: "b1", Accepted: true, Canonical: "Picea otherica", Rank: "SPECIES", Status: "Accepted"},
		// ...that holds "Abies alba" (the later homonym) only as a SYNONYM.
		{TaxonID: "s1", AcceptedTaxonID: "b1", Accepted: false, Canonical: "Abies alba", Rank: "SPECIES", Status: "Illegitimate"},
	}
	readerFor := func(application.Backbone) (application.RowSource, error) { return fakeRowSource{taxa: taxa}, nil }
	if _, err := application.Ingest(ctx, ds, readerFor, repo); err != nil {
		t.Fatalf("Ingest: %v", err)
	}

	report, err := application.IngestNameSpace(ctx, repo,
		fakeNameRowSource{rows: []application.NameRow{{Taxon: "Abies alba", SourceID: "e1", Status: "accepted"}}},
		domain.NameSpaceMeta{ID: "eurosl", Version: "v1"})
	if err != nil {
		t.Fatalf("IngestNameSpace: %v", err)
	}
	if report.Matched != 1 || report.Ambiguous != 0 {
		t.Fatalf("matched/ambiguous = %d/%d, want 1/0", report.Matched, report.Ambiguous)
	}
	if report.TieBroken != 1 {
		t.Errorf("TieBroken = %d, want 1", report.TieBroken)
	}
	if len(report.TieBrokenSample) != 1 || report.TieBrokenSample[0] != "Abies alba" {
		t.Errorf("TieBrokenSample = %v, want [Abies alba]", report.TieBrokenSample)
	}
	entries, err := repo.NameSpaceEntries(ctx, "wcvp:concept:a1", []string{"eurosl"})
	if err != nil || len(entries) != 1 {
		t.Fatalf("NameSpaceEntries(a1) = %v, %v — der Eintrag muss am accepted-Träger hängen", entries, err)
	}
	if entries[0].Resolution != "accepted_bearer_tiebreak" {
		t.Errorf("Resolution = %q, want accepted_bearer_tiebreak", entries[0].Resolution)
	}
}

// TestIngestNameSpace_SynonymOnlyHomonymStaysAmbiguous pins the tie-break's
// lower boundary: a spelling NO candidate bears as accepted stays ambiguous
// (4580 measured eurosl folds are this genuinely undecidable class).
func TestIngestNameSpace_SynonymOnlyHomonymStaysAmbiguous(t *testing.T) {
	// Fixture: zwei accepted Konzepte mit ANDEREN Namen, beide tragen
	// "Shared synonymum" nur als Synonym. Erwartung: Ambiguous=1, TieBroken=0,
	// keine name_space_entry.
}

// TestIngestNameSpace_TwoAcceptedBearersStayAmbiguous pins the upper
// boundary: several accepted bearers must NOT be rescued (no tier 2 in the
// crosswalk — spec decision 1) and stay ambiguous.
func TestIngestNameSpace_TwoAcceptedBearersStayAmbiguous(t *testing.T) {
	// Fixture: zwei accepted Konzepte, BEIDE mit Canonical "Duplex nomen".
	// Erwartung: Ambiguous=1, TieBroken=0.
}
```

In `crosswalk_internal_test.go` Tabellen-Test für den Marker-Helper:

```go
func TestResolutionWithTieBreak(t *testing.T) {
	cases := []struct {
		name      string
		rule      domain.NormalizationRule
		tieBroken bool
		want      string
	}{
		{"exact ohne tie-break", domain.RuleExact, false, ""},
		{"exact mit tie-break", domain.RuleExact, true, "accepted_bearer_tiebreak"},
		{"rule ohne tie-break", domain.RuleHybridSpacing, false, string(domain.RuleHybridSpacing)},
		{"rule mit tie-break", domain.RuleHybridSpacing, true, string(domain.RuleHybridSpacing) + "+accepted_bearer_tiebreak"},
	}
	for _, c := range cases {
		if got := resolutionWithTieBreak(c.rule, c.tieBroken); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}
```
(`domain.RuleHybridSpacing` ggf. durch einen real existierenden Rule-Bezeichner ersetzen — `git grep "Rule.* NormalizationRule\|Rule[A-Z].* =" internal/domain/`.)

**Bestandstest-Prüfung:** `TestIngestNameSpace_HomonymStaysAmbiguousHere` lesen. Ist seine Fixture die accepted+synonym-Klasse, ändert dieser Task sein Verhalten SPEC-GEWOLLT: Test umbenennen/ersetzen durch die neuen Erwartungen, mit Kommentar auf die Spec (2026-09-04) — nicht stillschweigend. Ist die Fixture synonym-only, bleibt er unverändert.

- [ ] **Step 2: Fehlschlag verifizieren**

Run: `go test ./internal/application/ -run 'TestIngestNameSpace_AcceptedBearer|TestIngestNameSpace_SynonymOnly|TestIngestNameSpace_TwoAccepted|TestResolutionWithTieBreak' -v`
Expected: FAIL (Policy/Helper/Felder existieren nicht).

- [ ] **Step 3: Implementierung**

`match.go` (direkt vor `genuineBearerWinner`, rein additiv):

```go
// acceptedBearerWinner is genuineBearerWinner's tier 1 alone: exactly one
// candidate holding the name as its ACCEPTED name wins; no accepted bearer,
// or several, resolves nothing — and no weaker tier runs. The name-space
// crosswalk uses this (policyResolveAcceptedBearer) instead of the full
// two-tier rule: tier 1 is the nomenclaturally grounded case (a later
// homonym is illegitimate BECAUSE it duplicates a legitimate name's
// spelling — measured 2026-09-01: Illegitimate rows collide with a foreign
// accepted canonical at 17.3% vs 0.16% for ordinary synonyms), while
// tier 2 (homotypic synonym bearer) is not yet measured for name spaces
// and stays serving-path-only (spec 2026-09-04, decision 1).
func acceptedBearerWinner(winners []classifiedHit) (string, bool) {
	id, present := soleConcept(winners, func(w classifiedHit) bool { return w.role == roleAccepted })
	return id, present && id != ""
}
```

`crosswalk.go`:
- Konstante `policyResolveAcceptedBearer` ans Ende des Policy-Blocks, mit Doc-Kommentar (Spec-Referenz, Messwerte 4716 Folds, Abgrenzung zu policyResolveGenuineBearer: KEIN Tier 2).
- In `resolveTraitName`, den `len(distinct) > 1`-Zweig erweitern (bestehende genuineBearer-Logik unangetastet lassen):

```go
		if len(distinct) > 1 {
			if id, ok := genuineBearerWinner(traitBearers(candidates)); ok && policy == policyResolveGenuineBearer {
				return traitResolution{conceptID: id, matched: true, tieBroken: true, rule: cand.Rule}, nil
			}
			if policy == policyResolveAcceptedBearer {
				if id, ok := acceptedBearerWinner(traitBearers(candidates)); ok {
					return traitResolution{conceptID: id, matched: true, tieBroken: true, rule: cand.Rule}, nil
				}
			}
			return traitResolution{ambiguous: true, rule: cand.Rule}, nil
		}
```
- `preferGenuineClaimants` muss auch unter der neuen Policy laufen: die Policy-Bedingung um `|| policy == policyResolveAcceptedBearer` erweitern (crosswalk.go, die Zeile mit `policyResolveGenuineBearer || policy == policyPreferBackbone`).
- Marker-Helper neben `resolutionFor`:

```go
// resolutionWithTieBreak renders the stored resolution for one entry: the
// normalisation rule as before (empty for the exact key), suffixed with the
// tie-break marker when acceptedBearerWinner decided the concept — so every
// tie-broken row stays identifiable in SQL
// (resolution LIKE '%accepted_bearer_tiebreak%'), which is the audit trail
// the spec makes mandatory.
func resolutionWithTieBreak(rule domain.NormalizationRule, tieBroken bool) string {
	base := resolutionFor(rule)
	if !tieBroken {
		return base
	}
	if base == "" {
		return "accepted_bearer_tiebreak"
	}
	return base + "+accepted_bearer_tiebreak"
}
```

`namespace_ingest.go`:
- Report-Felder (mit Doc-Kommentar: Teilmenge von Matched, warum auditierbar):

```go
	// TieBroken counts the matched rows whose concept was decided by the
	// tier-1 accepted-bearer tie-break (policyResolveAcceptedBearer) rather
	// than by a single-candidate key; TieBrokenSample is its bounded,
	// deterministic name sample. Subset of Matched — the report invariant
	// Matched+Unmatched+Ambiguous == Rows is untouched. See spec 2026-09-04.
	TieBroken       int
	TieBrokenSample []string
```
- Tally: `tieBroken map[string]bool` + `countTieBroken(name string)` + im `report()` `r.TieBrokenSample = sortedSample(t.tieBroken)`.
- `writeNameSpaceRow`: direkt nach `report.Matched++`:

```go
	if res.tieBroken {
		report.TieBroken++
		tally.countTieBroken(row.Taxon)
	}
```
  und `Resolution: resolutionFor(res.rule)` ersetzen durch `Resolution: resolutionWithTieBreak(res.rule, res.tieBroken)`.
- `resolveNameSpaceNames`: Aufruf auf `policyResolveAcceptedBearer` umstellen und den großen Begründungs-Kommentar aktualisieren: der Absatz „the homonym TIE-BREAK … is measured for trait vocabularies, not for name spaces … evidence nobody gathered" wird ersetzt durch die neue Faktenlage (Tier 1 aktiviert, Messwerte, Report-Zähler existiert jetzt; Tier 2 bleibt aus, Spec-Referenz).

- [ ] **Step 4: Tests laufen lassen**

Run: `go test ./internal/application/` — Expected: PASS; danach `go test ./...` und `make lint`.

- [ ] **Step 5: Commit**

```bash
git add internal/application/
git commit -m "feat(crosswalk): Tier-1-Tie-Break (accepted bearer) für Name-Space-Homonyme"
```

---

### Task 2: Report-Ausgabe in `hostus ingest` + CHANGELOG

**Files:**
- Modify: `cmd/hostus/ingest.go` (~Zeile 97-106, neben den bestehenden Sample-Zeilen)
- Modify: `CHANGELOG.md` (`## [Unreleased]`)
- Test: bestehende Tests zu `cmd/hostus/ingest.go`-Ausgabe (Muster nachschlagen: `git grep -n "flagged sample" cmd/hostus/`)

**Interfaces:**
- Consumes: `NameSpaceIngestReport.TieBroken`, `.TieBrokenSample` (Task 1).

- [ ] **Step 1: Fehlschlagenden Test schreiben** — bestehenden Ausgabe-Test um die Erwartung erweitern: bei `TieBroken > 0` erscheint eine Zeile `tie-broken (accepted bearer)=N` und `printSampleLine(w, "tie-broken sample", r.TieBrokenSample)`-Ausgabe; bei 0 erscheint sie NICHT (dem Muster der bestehenden bedingten Zeilen folgen — nachschlagen, wie `ambiguous sample` bei leerem Sample behandelt wird, und identisch verfahren; deckt den Mutations-Zweig).
- [ ] **Step 2: Fehlschlag verifizieren** — `go test ./cmd/hostus/ -run <Testname> -v` → FAIL.
- [ ] **Step 3: Implementierung** — Zähler in die Summary-Zeile oder als eigene Zeile im Stil der Datei; Sample via bestehendem `printSampleLine`.
- [ ] **Step 4: Tests** — `go test ./cmd/hostus/` + `make lint` → PASS.
- [ ] **Step 5: CHANGELOG** unter `## [Unreleased]` → `### Added`:

```markdown
### Added

* **Namespace-Crosswalk:** Tier-1-Tie-Break — trägt genau EIN Konzept eine
  Schreibweise als accepted-Namen (und andere nur als Synonym, typisch
  illegitime spätere Homonyme wie „Abies alba"), löst der
  eurosl/germansl/floraveg-Crosswalk jetzt auf den accepted-Träger auf,
  statt die Zeile als mehrdeutig zu verwerfen (gemessen: 4716 betroffene
  eurosl-Namen). Voll auditierbar: `hostus ingest` meldet
  `tie-broken (accepted bearer)` samt Namens-Sample, und jede so
  entstandene Zeile trägt den `resolution`-Marker
  `accepted_bearer_tiebreak`. Tier 2 (homotypischer Träger) bleibt dem
  Serving-Pfad vorbehalten. Wirkt erst nach Re-Ingest.
```

- [ ] **Step 6: Commit**

```bash
git add cmd/hostus/ingest.go CHANGELOG.md <testfiles>
git commit -m "feat(cli): tie-broken-Zähler und Sample im Ingest-Report ausgeben"
```

---

## Verifikation nach Abschluss (Controller, kein Task)

1. `make verify`; `make mutation PKG=./internal/application` und `PKG=./cmd/hostus` — kein `Not covered`.
2. Full-Ingest (`dataset.full-deploy.yaml`) vorher/nachher: eurosl matched ~+4.700, TieBroken-Zähler plausibel; „Abies alba"/„Aconitum napellus" in den Spaces.
3. 20–30 tie-broken Namen per SQL ziehen (`resolution LIKE '%accepted_bearer_tiebreak%'`) und dem Maintainer VOR dem Merge als Stichprobe vorlegen.

# Spec: Ambiguitäts-Konsistenz-Fixes (Serving-Pfad, Fall B, CDM-no-sec, Match-Limits)

Datum: 2026-09-01
Status: verabschiedet (Audit-Befunde, empirisch am realen Full-Ingest-Index verifiziert)

## Kontext

Nach den Fixes PR #94 (`policyPreferBackbone`: sec.-Space-Konzepte zählen im
Namespace-Crosswalk nicht mehr als Ambiguitätskandidaten) und PR #98
(WCVP-`Misapplied`-Zeilen werden nicht mehr als Synonyme verlinkt) hat ein
systematisches Audit (2026-09-01, realer Index aus `dataset.full-deploy.yaml`)
vier weitere, strukturgleiche Ambiguitätslöcher sowie eine DoS-Lücke
identifiziert. Dieses Dokument ist die verbindliche Autorität für deren Fixes.

## Befunde (verifiziert)

### B1 — Serving-Pfad wendet den sec.-Filter nicht an (KRITISCH)

`classify`/`matchFuzzy`/`matchAggregate`/`matchAggregateNominate`
(`internal/application/match.go`) reichen `repo.MatchExact`-Kandidaten
ungefiltert weiter. CDM-sec.-Konzepte zählen dort als gleichrangige Claimants.
End-to-end gemessen: `POST /v1/match` liefert für „Pinus sylvestris",
„Abies alba", „Leucanthemum maximum" und „Acer" `unresolvable`, obwohl der
Ingest-Crosswalk dieselben Namen auflöst. `/v1/translate` (Verbatim-Einstieg,
läuft durch `matchNamesFiltered`) erbt das als 422 UNRESOLVABLE.

### B2 — Fall-B-native Konzepte gelten als Backbone (HOCH)

`IngestNativeSpace` (`nativespace_ingest.go`) schreibt eurosl/germansl-eigene
Konzepte mit `SecReference=""` — für `preferBackboneConcepts` ununterscheidbar
von WCVP. Gemessen: 2866 GENUS- + 319 FAMILY-Folds mehrfach accepted;
germansl verliert ~544 Gattungs-Einträge, weil eurosls Fall B vor germansls
Fall A läuft (417 vs. 961 aufgelöst auf identischer Namensliste) —
reihenfolgeabhängig und bei Re-Ingest nicht idempotent. Unterfall: eurosl-
Fall-B-Aggregate mit nacktem Binomialnamen („Hieracium sabaudum",
SPECIES_AGGREGATE ohne „agg.") kollidieren mit der WCVP-Art.

### B3 — 124 CDM-Konzepte ohne sec_uuid umgehen den Filter (MITTEL)

`planCDMConcepts` (`cdm_ingest.go`) zählt `ConceptsWithoutSec`, schreibt die
Konzepte aber mit `SecReference=""`. Folge: „Leucanthemum maximum",
„Papaver lecoqii", „Rubus affinis" fehlen in ALLEN drei Name-Spaces.

### B4 — POST /v1/match ohne Body-/Batch-Limit (SECURITY/DoS)

Kein `http.MaxBytesReader` im Repo; `len(body.Names)` unbegrenzt. Ein Request
mit 500k Namen wird voll dekodiert und sequenziell verarbeitet (pro Name bis
zu 20k-Kandidaten-Levenshtein). Suggest/Synonyms haben Caps, Match nicht.
`/v1/translate` (ein Name pro Request) braucht nur das Body-Limit.

## Entscheidungen

1. **Zwei-Stufen-Claimant-Präferenz** als EINE gemeinsame Funktion
   (`preferGenuineClaimants` in `crosswalk.go`), benutzt von Ingest-Crosswalk
   UND Serving-Pfad (Projektprinzip: beide Pfade, dieselbe Regel):
   - Stufe 1 (bestehend): sec.-Space-Kandidaten fallen weg, wenn ein
     non-sec-Kandidat den Namen trägt (`preferBackboneConcepts`, PR #94).
   - Stufe 2 (neu): Name-Space-native Kandidaten (BackboneID ∈ Menge der
     `name_space`-IDs, via bestehendem Port `Repository.NameSpaces`) fallen
     weg, wenn ein echtes Taxonomie-Backbone-Konzept übrig bleibt. Fallback
     wie Stufe 1: Trägt NUR ein natives Konzept den Namen, bleibt es
     Kandidat (Moos-Gattungen wie Abietinella lösen weiter auf).
2. **Serving-Pfad wendet die Präferenz NUR bei leerem `MatchFilter` an**
   (`filter.empty()`): Setzt der Caller `entry_backbone`/`entry_sec`, hat er
   den Raum explizit gepinnt — die Präferenz darf sein Ergebnis nicht
   verändern. Der ungefilterte Pfad bleibt byte-identisch zum
   Ingest-Crosswalk-Verhalten.
3. **CDM-Konzepte ohne sec_uuid** bekommen eine SYNTHETISCHE sec-Referenz
   (`cdm:unattributed`, Titel „CDM ohne sec-Referenz (synthetisch)") statt
   verworfen zu werden: sie bleiben Relations-Ziele und via `entry_sec`
   erreichbar, zählen aber nicht mehr als Backbone-Claimants.
   `ConceptsWithoutSec` zählt weiter.
4. **Match-Limits**: `http.MaxBytesReader` (1 MiB) + `maxMatchNames = 1000`
   für POST /v1/match; `MaxBytesReader` (64 KiB) für POST /v1/translate.
   Überschreitung → 400 `INVALID_QUERY` (kein neuer Error-Code — die
   Fehlercode-Liste in CLAUDE.md ist abschließend).
5. **Explizit AUSSERHALB dieses Scopes** (brauchen eigene Entscheidungen,
   siehe Audit-Memo): genuineBearerWinner-Tie-Break im Namespace-Crosswalk
   (4716 Folds, Abies-alba-Klasse); „Provisionally Accepted"-Behandlung;
   Suggest-Ranking; Performance-Overfetch (`repo.Concept` im Batch).

## Projektweite Anforderungen

- `internal/application` und `internal/adapters/http` sind mutation-gated:
  `make mutation PKG=<pkg>` muss ohne `Not covered`-Mutanten durchlaufen.
- Hexagon-Grenzen (depguard): `application` importiert keine Adapter.
- Fehlercodes nur aus der Liste in CLAUDE.md; Fehlerformat
  `{"error":{"code":...,"message":...}}`.
- CHANGELOG.md unter `## [Unreleased]`; Conventional Commits.
- Tests folgen den bestehenden Mustern (`openMemoryRepo`, Tabellen-Tests in
  `match_test.go`, `namespace_ingest_test.go`, `httptest` in Adapter-Tests).

# Spec: Tier-1-Tie-Break (accepted bearer) im Namespace-Crosswalk

Datum: 2026-09-04
Status: verabschiedet (User-Entscheidung: „Erstmal für a/b nur Tier 1 in den Crosswalk")

## Kontext

Nach den Ambiguitäts-Fixes v3.0.4-alpha.0 ist die größte verbleibende
Ambiguitätsklasse im eurosl/germansl/floraveg-Crosswalk die
WCVP-interne Homonym-Klasse „1 accepted-Träger + nur-Synonym-Träger"
(gemessen 2026-09-01: **4716 eurosl-Folds**, darunter Abies alba,
Aconitum napellus). Ursache sind vor allem illegitime spätere Homonyme:
`Illegitimate`-Zeilen kollidieren zu 17,3 % (8431/48722) mit dem
accepted-Namen eines fremden Konzepts, `Invalid` 1928× — gegenüber 0,16 %
bei gewöhnlichen Synonymen. Nomenklatorisch (ICN) ist der Sieger bekannt:
das illegitime Homonym darf nicht verwendet werden; eine Quelle ohne
Autorangabe meint den legitimen (accepted) Träger.

Der Serving-Pfad (`classify` → `genuineBearerWinner`, match.go) löst diese
Klasse seit Issue #67 per Tie-Break auf; der Namespace-Crosswalk
(`resolveNameSpaceNames` mit `policyPreferBackbone`) verweigerte bisher —
mit der dokumentierten Begründung „evidence nobody gathered" und „der
Report hat keinen tiebroken-Zähler". Beides ist jetzt adressierbar: die
Evidenz ist gemessen, und dieser Change führt die Auditierbarkeit ein.

## Entscheidungen

1. **Nur Tier 1** („genau EIN Kandidat trägt den Namen als accepted →
   dieser gewinnt") wird im Namespace-Crosswalk aktiviert. Tier 2
   (homotypischer Synonym-Träger) bleibt im Crosswalk AUS — bewusste,
   dokumentierte Abweichung vom Serving-Pfad (der weiterhin Tier 1+2
   fährt), bis Tier 2 separat vermessen und entschieden ist.
2. Neue Policy `policyResolveAcceptedBearer` in `crosswalk.go` (Kette:
   `preferGenuineClaimants` wie bisher, dann Tier-1-Tie-Break).
   `resolveNameSpaceNames` wechselt von `policyPreferBackbone` darauf.
   Die Tier-1-Logik wird aus `genuineBearerWinner` als gemeinsame
   Funktion extrahiert (kein zweiter Regel-Klon; `soleConcept` +
   accepted-Qualifier sind die bestehenden Bausteine, `roleAccepted`
   die bestehende Konstante).
3. **Auditierbarkeit (Pflicht, nicht optional):**
   - `NameSpaceIngestReport` erhält `TieBroken int` und
     `TieBrokenSample []string` (via `sortedSample`, Cap wie alle
     Samples); Invariante `Matched + Unmatched + Ambiguous == Rows`
     bleibt — TieBroken ist eine Teilmenge von Matched.
   - `name_space_entry.resolution` markiert tie-broken Zeilen:
     `"accepted_bearer_tiebreak"` wenn der Exact-Key auflöste, sonst
     `"<rule>+accepted_bearer_tiebreak"`. Konsumenten der Spalte
     behandeln sie als opaken String (vor Implementierung verifizieren:
     Bundle-/Crosswalk-Export und Tests, die `resolution` lesen).
   - `hostus ingest` druckt den Zähler und das Sample
     (cmd/hostus/ingest.go, neben den bestehenden Sample-Zeilen).
4. **Kein Verhaltens-Change am Serving-Pfad** und keiner an
   `policyPreferBackbone`/`policyResolveGenuineBearer` — bestehende
   Semantik der beiden Alt-Policies bleibt byte-identisch.
5. **Grenzen des Tie-Breaks** (müssen durch Tests gepinnt sein):
   - Kein accepted-Träger unter den Kandidaten → ambiguous (unverändert).
   - MEHRERE accepted-Träger → ambiguous; Tier 2 darf NICHT retten
     (existiert im Crosswalk nicht).
   - Der Tie-Break läuft NACH `preferGenuineClaimants` — sec.-Space- und
     Fall-B-native Kandidaten sind zu dem Zeitpunkt bereits demotet.
   - Ein AMBIGUOUS-Ausgang stoppt die NameCandidates-Leiter weiterhin
     (keine Rettung durch losere Normalisierungs-Keys).

## Erwartete Wirkung (zu verifizieren per Full-Ingest vorher/nachher)

- eurosl: Größenordnung +4.700 matched (ambiguous sinkt entsprechend);
  germansl/floraveg anteilig. „Abies alba", „Aconitum napellus"
  erscheinen in den Spaces.
- Vor dem Merge: 20–30 tie-broken Namen als Stichprobe dem Maintainer
  vorlegen (aus TieBrokenSample bzw. per SQL über resolution-Marker).

## Risiko & Rückholbarkeit

Fehlerszenario: die Quelle meinte das Taxon hinter dem Synonym-Eintrag →
stille Fehlzuordnung statt sichtbarem Verlust. Gegenmittel: der
resolution-Marker macht jede tie-broken Zeile per SQL identifizierbar
(`WHERE resolution LIKE '%accepted_bearer_tiebreak%'`), der Report macht
Umfang und Beispiele bei jedem Ingest sichtbar, und ein Policy-Rollback
ist eine Ein-Zeilen-Änderung am Call-Site.

## Projektweite Anforderungen

- `internal/application` und `cmd/hostus` sind mutation-gated
  (`make mutation PKG=<pkg>`, kein `Not covered`-Mutant).
- depguard-Hexagon-Grenzen; CHANGELOG unter `## [Unreleased]`;
  Conventional Commits; Doc-Kommentare englisch, WHY-Stil mit den
  Messwerten dieser Spec.

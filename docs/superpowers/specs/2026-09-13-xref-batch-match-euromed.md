# Spec: Xref-Batch-Match, E+M-UUIDs im Zielraum-Response, Synonymie-Schluss

Datum: 2026-09-13
Status: verabschiedet (User: „Den Xref-Batch-Match machen wir auf jeden Fall … klug, die E+M-UUIDs zu haben"; situs außen vor, GBIF-Weg vermeiden)

## Use Case (verbindlich)

PlantNet-Bestimmung liefert `preferedReferential: "k-world-flora"` + **POWO-ID**
(ggf. mehrere Pflanzen pro Anfrage). Für jede POWO-ID soll **Habitatus** einen
**Euro+Med-PlantBase-Namen** samt Namensherkunft erhalten; die **E+M-UUID**
(TaxonUsageID) soll mitgeliefert werden (spätere Anbindung externer
Ressourcen, z. B. EuroVeg.eu). Der GBIF-ID-Weg wird bewusst gemieden
(GBIF-Umstrukturierung auf Cybertaxonomy — instabil).

Ist-Kette (verifiziert): POWO-ID → `/v1/xref?authority=powo` (440.534
POWO-Xrefs, eine je WCVP-Konzept) → `/v1/translate target_space=eurosl` →
E+M-Name. Lücken: (1) kein Batch/keine ID-Eingabe in `/v1/match`; (2) bei
3.719 von 47.989 Konzepten existiert kein accepted-Status-eurosl-Eintrag —
geliefert wird nur ein E+M-Synonym (Inula-hirta-Klasse: E+M akzeptiert
invers zu WCVP); (3) die E+M-UUID (`name_space_entry.ext_id`) wird in keiner
Antwort exponiert.

## Entscheidungen

1. **Xref-Einträge in `/v1/match`:** `MatchRequest`/DTO erhalten optional
   `xref: {authority, id}` — pro Zeile ist GENAU EINES von `verbatim` oder
   `xref` gesetzt (beides/keines → 400 `INVALID_QUERY` mit Zeilen-ID).
   Auflösung via bestehendem Port `Repository.ConceptByXref`; Treffer
   bekommt `match_type: "xref"` (neuer `domain.MatchType`, Confidence 1.0 —
   eine Fremd-ID ist exakter als jeder Namens-Match); unbekannte ID →
   normales UNRESOLVABLE-Result (kein HTTP-Fehler, Batch läuft weiter).
   Xref-Zeilen durchlaufen NICHT die Namens-Leiter/Fuzzy; `entry_backbone`/
   `entry_sec`-Filter gelten (Konzept außerhalb des Filters → unresolvable
   mit Note). Batch-Limit (1000) und Body-Limit gelten unverändert.
2. **Zielraum-Antworten tragen die Quell-Identität:** `domain.
   ResolveTargetSpace` liefert statt nur des Namens den GEWÄHLTEN Eintrag
   (Name, ExtID, Status). Response-Erweiterungen (rein additiv):
   - `/v1/match` (mit `target_space`): neben `target_space_name` neu
     `target_space_ext_id` und `target_space_status`
     (`accepted`/`synonym`/… verbatim aus der Quelle — Habitatus sieht, ob
     es den akzeptierten E+M-Namen bekommt).
   - `/v1/translate`: `name_space_translation` um `ext_id` und `status`.
   Die HERKUNFT ist das vorhandene `name_space`-Feld (`"eurosl"`); das
   Mapping auf Habitatus' Label „euro+med" macht der Aufrufer — im
   OpenAPI-/Feld-Doc wird festgehalten, dass der eurosl-Raum der
   Euro+Med-PlantBase-Snapshot ist (EuroSL.sqlite, AccordingTo
   api.cybertaxonomy.org/euromed, 139.039/139.039 Zeilen).
3. **Synonymie-Schluss im Namespace-Ingest (Phase A):** Eine Zeile, die der
   Namens-Crosswalk NICHT auflöst (ambiguous oder unmatched), wird in einem
   NACHGELAGERTEN Schluss-Pass über die QUELL-Synonymie attached: Gruppe =
   alle Zeilen mit demselben `accepted_taxon`-Namen (namelist.Row trägt ihn
   bereits; `NameRow` reicht ihn künftig durch). Haben die bereits
   attachten Mitglieder der Gruppe GENAU EIN Ziel-Konzept, wird die offene
   Zeile dorthin attached; bei null oder mehreren Ziel-Konzepten bleibt sie
   offen (kein Raten). Auditierbarkeit wie beim Tie-Break:
   `resolution`-Marker `source_synonymy_closure` (bzw.
   `<rule>+source_synonymy_closure` sinngemäß — die Zeile kam NICHT über
   eine Normalisierungsregel, also schlicht der Marker), Report-Zähler
   `SynonymyClosed int` + Sample, CLI-Ausgabe. Erwartete Wirkung: schließt
   die Inula-hirta-Klasse (E+M-accepted-Namen, deren WCVP-Seite nur
   Synonym-Träger hat) — Messung per Full-Ingest, Stichprobe vor Merge.
4. **Grenzen:** Der Schluss-Pass läuft NACH dem normalen Resolve (er darf
   dessen Ergebnisse nie ändern, nur offene Zeilen füllen); Ambiguous
   bleibt für die Leiter weiterhin terminal (der Schluss ist KEIN weiterer
   Leiter-Schritt, sondern nutzt ausschließlich Quell-interne Evidenz);
   Duplicate-ExtID-Regeln unverändert. `ResolveTargetSpace`s
   Präzedenz (accepted-in-space zuerst) bleibt identisch — durch Phase A
   wird der accepted-Eintrag schlicht wieder VORHANDEN sein.
5. **Außerhalb dieses Scopes:** Euro+Med-Backbone (Phasen 1–3 der Skizze
   2026-09-12 — zurückgestellt, bis E+M-UUIDs als Konzept-IDs oder die
   E+M-Sicht als Taxonomie gebraucht werden), situs-Export, GBIF-Xrefs als
   Eingang (bewusst gemieden), Habitatus-seitige 48 Länder-Synonymlisten.

## Projektweite Anforderungen

- `internal/application`, `internal/adapters/http`, `internal/domain`,
  `internal/adapters/namelist`, `cmd/hostus` sind mutation-gated (CI);
  lokale Läufe für heavy-Pakete falls berührt (hier: keins).
- Bestehende match-/translate-Antworten bleiben byte-identisch, solange
  weder `xref`-Einträge noch die neuen Felder ins Spiel kommen (additive
  Felder mit `omitempty`).
- OpenAPI ist codegeneriert — DTO-Änderungen genügen.
- CHANGELOG unter `## [Unreleased]`; Conventional Commits; englische
  WHY-Doc-Kommentare mit den Messwerten dieser Spec (3.719/47.989 etc.).

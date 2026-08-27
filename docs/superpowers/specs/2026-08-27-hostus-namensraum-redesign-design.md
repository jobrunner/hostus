# hostus — Namensraum-, Klassifikations- und Aggregat-Redesign (Teilprojekt 1)

**Stand:** 2026-08-27
**Status:** Design, noch nicht implementiert
**Vorgänger:** `docs/superpowers/specs/2026-07-31-hostus-2.0-architecture.md`,
`hostus-loesungsarchitektur.md`, SP9 (`2026-08-04-sp9-uc4-aggregate-policy.md`)

## Zweck

Dieses Dokument spezifiziert eine Korrektur an hostus' Datenmodell und API, die
aus der praktischen Nutzung für zwei Anwendungsfälle entstand, die beim
ursprünglichen Entwurf nicht zu Ende gedacht waren:

1. **UC Situs** — ein Feldbeobachter kennt einen Namen (ggf. ein Aggregat wie
   *Achillea millefolium* agg.) und braucht die Sippen darunter, um situs nach
   EUNIS-Habitaten/FFH-LRT fragen zu können.
2. **UC Historische Namen** — entomologische Literatur (18./frühes 19. Jh.)
   verwendet Wirtspflanzennamen, die sich nur noch mit Aufwand auf moderne
   Konzepte zurückführen lassen; das soll in einen Wissensgraphen einfließen.

Beide Anwendungsfälle scheiterten am selben strukturellen Loch: hostus'
Backbone (WCVP) kennt **keine Aggregate**, **keine Klassifikation oberhalb der
Familie** und **keine Sektionen** — Dinge, die für die Orientierung im Feld
(*"ist das ein Chenopodiaceae?"*) und für kritische Gattungen (*Rubus*,
*Hieracium*) unverzichtbar sind.

## Nicht-Ziele (ausdrücklich aus diesem Spec ausgeklammert)

Diese Themen wurden im Brainstorming ebenfalls aufgeworfen, aber bewusst in
eigene, spätere Teilprojekte ausgelagert — sie hängen von der hier
spezifizierten API-Form ab, sind aber eigenständige Entwürfe:

- **Teilprojekt 2** — situs' neues, einheitliches Trait-Modul (EIVE/Tichý/
  Midolo **und** Ellenberg-Zeigerwerte, kreuzvalidierbar an Konzepte gebunden).
  Quelle der Ellenberg-Werte ist noch offen.
- **Teilprojekt 3** — hostus-Frontend/Testkonsole: a11y, mobile-first, Tabs als
  Gruppen, als **konkretes Testwerkzeug** für die Zielgruppen nutzbar (nicht
  nur abstrakte API-Exploration). Muss insbesondere den `/v1/translate`-
  Workflow (Raum wählen → Name auflösen → übersetzen) führen, siehe Abschnitt 9.
- **Teilprojekt 4** — Dokumentation umfassend aktualisieren und straffen.

## Ausgangslage — was gemessen wurde

Alle folgenden Zahlen sind gegen die real gecachten Rohdaten gemessen
(`pipelines/{eurosl,germansl,floraveg}/.cache/`), nicht angenommen:

- **EuroSL** (`EuroSL.sqlite`, Tabelle `EuroPlusMed.Plantae`, 139.039 Zeilen,
  einzige Datentabelle) trägt eine vollständige `IsChildTaxonOfID`-Kette von
  Art bis Phylum, **287 Species-Aggregate** als eigenen Rang, und ist eine
  **kohärente Klassifikation** (`AccordingTo` = `api.cybertaxonomy.org/euromed`
  über die gesamte Tabelle — kein Multi-`sec.`-Problem wie bei CDM).
  37.234 von 37.234 akzeptierten Species (100%) haben eine vollständige
  Klassifikationskette. Beispiel: *Salsola kali* → *Salsola kali* aggr. →
  *Salsola* → **Chenopodiaceae** → Caryophyllales → Magnoliopsida →
  Tracheophyta.
- **GermanSL** (`GermanSL1.5.5.xlsx`, Sheet `TCS`, 26.129 Zeilen) trägt
  `AccordingTo = "Buttler 2018"` auf praktisch jeder Zeile — es ist **die**
  maschinenlesbare Form der deutschen Standardliste (Florenliste Deutschland,
  aktuell Version 16, Juni 2026 unter florenliste-deutschland.de). GermanSL
  selbst hat ein Update vom **29.09.2025**, neuer als die gecachte 1.5.5 —
  **muss vor Produktiv-Ingest neu gepinnt werden.**
- GermanSL trägt **deutsche Vernakularnamen** direkt in der Rohtabelle (Feld
  `VernacularName`, z.B. *Achillea millefolium* → "Gewöhnliche
  Wiesen-Schafgarbe"). Weder EuroSL noch WCVP haben das.
- **EuroSL und GermanSL können bei Aggregaten unterschiedliche
  Mitgliederlisten führen** — gemessenes Beispiel *Salsola kali*: EuroSL führt
  die Subspecies *tragus*/*ruthenica* als objektive Synonyme unter *S. kali*,
  GermanSL/Buttler hängt sie stattdessen unter eine **andere** akzeptierte Art
  (*Salsola tragus* subsp. *tragus*) — eine echte Circumscription-Differenz,
  kein Tippfehler.
- **EuroSL behandelt kritische Sammelgattungen flach** (*Rubus*: 1.325 Species,
  keine Sektionen), **GermanSL gliedert sie** (Untergattung → Sektion →
  Subsektion → Serie, Beispiel *Rubus* sect. *Rubus*, sect. *Corylifolii*, …).
- **WCVP führt keine Ränge oberhalb der Familie** und **keine
  Aggregat-Konzepte** — bestätigt am realen Index (aus SP9-Verdikt).
- `/v1/match`'s bisheriges `aggregate_alias`-Verhalten ist **absichtlich eine
  Sackgasse**: der Code-Kommentar sagt wörtlich *"No microspecies resolution
  is attempted"* — das war für den ursprünglichen Zweck (Deckungsprozente in
  Vegetationsaufnahmen der ESy-Regel-Engine zuordnen, UC4/SP9) korrekt, scheitert
  aber an der Kernzielgruppe selbst, sobald diese *"Rubus sect. Rubus"* oder
  *"Achillea millefolium agg."* schreibt (siehe Abschnitt 6).
- `/v1/translate` funktioniert mechanisch bereits (99,67% eindeutige
  Auflösung über `entry_sec`, `GET /v1/sec` existiert zur Raum-Discovery) —
  das Problem war nie API oder Lizenz, sondern (a) CDM muss explizit
  mit-ingestiert werden (nicht Standard) und (b) es gibt keinen geführten
  Workflow dafür (→ Teilprojekt 3).

## 1. Datenquellen & Rollen

| Namensraum | Rolle | Deckung | Besonderheit |
|---|---|---|---|
| **WCVP** | Haupt-Backbone (unverändert) | Global | Ranks bis Familie, stabile IDs, Regionsfilter (WGSRPD) |
| **EuroSL** | **Default**-Namensraum für Suggest/Konzept-Anzeige; Quelle für Klassifikation + Aggregate | Europa/Mediterran | Eine kohärente Klassifikation |
| **GermanSL** | Zusätzlicher Namensraum (`name_space=germansl`, Alias `buttler`) | Deutschland | = Buttler-Florenliste; deutsche Vernakularnamen; eigene Aggregat-/Sektions-Gliederung |
| **CDM** (Wisskirchen u.a.) | `sec.`-Referenzräume für `/v1/translate` | ~119 Literaturwerke | Unverändert aus SP5, hier nur in die vereinheitlichte `target_space`-Form integriert |

Alle Namensräume bleiben **eigenständige, ehrliche Perspektiven**. Wenn
EuroSL und GermanSL bei einem Aggregat unterschiedliche Sippen zählen, zeigt
die API das **transparent pro Namensraum** (Abschnitt 5), statt es zu
verschmelzen.

`name_space=buttler` ist ein **Alias**, der intern auf `germansl` auflöst —
der kanonische Wert in Antworten ist immer `germansl`. Begründung: "Buttler"
ist der in der Praxis bekannte Name, "GermanSL" der Pipeline-/Datenbank-Name.

## 2. Kanonisches Rang-Set

EuroSL (29 Rohwerte) und GermanSL (27 Kürzel) werden auf ein gemeinsames,
erweitertes Rang-Enum gemappt (`domain.Rank`, Erweiterung des heutigen
6-Werte-Sets nach der ParseRank-Lektion aus SP1: strikt mappen, unbekannte
Werte NIE raten, sondern zählen + mit Beispiel melden, Ingest läuft weiter):

```
ROOT, PHYLUM, SUBDIVISION, INFORMAL_CLADE (mit Tier-Attribut),
CLASS, SUBCLASS, SUPERORDER, ORDER, FAMILY, SUBFAMILY, TRIBE,
GENUS, SUBGENUS, SECTION, SUBSECTION, SERIES,
SPECIES_AGGREGATE, GENUS_AGGREGATE,
SPECIES, COLL_SPECIES, SUBSPECIES, SUBSPECIES_GROUP,
VARIETY, SUBVARIETY, FORM, SUBFORM, PROLES, RACE, CONVAR, GREX,
UNRANKED_INFRAGENERIC, UNRANKED_INFRASPECIFIC
```

**`INFORMAL_CLADE`** deckt die APG-Klade-Hierarchie ab (GermanSL `CL1`–`CL5`:
*Pteridopsida*, *Coniferopsida*, *Angiosperms*, *Eudicots*, *Rosids*,
*Campanulids* etc., 28 Zeilen gemessen) — Tier-Nummer als Zusatzattribut, da
die genaue Abstufung zwischen `CL1`–`CL5` noch nicht abschließend verifiziert
ist (offener Punkt, siehe Abschnitt 10).

**`GENUS_AGGREGATE`** (GermanSL `AG2`, "sensu lato" auf Gattungsebene, z.B.
*Barbula* s. l.) wird ins Rang-Set aufgenommen, aber **nicht** vom initialen
situs-Ingest verwendet — verfügbar für Clients, die es selbst nutzen wollen.

**Bewusst nicht gemappt (zählen, nicht raten):** GermanSL `AG3` (kein echter
Rang, Domänen-Bookkeeping-Knoten: *Embryophyta*/*Flechten*/*Algen*) und `UAB`
(6 Zeilen, ausschließlich Moose/Algen — außerhalb des hostus-Scopes).
Kosten-Nutzen gemessen: 37 von 26.129 GermanSL-Zeilen (0,14%) betroffen,
überwiegend ohnehin außerhalb des Scopes (Gefäßpflanzen + Flechten).

## 3. Suchmodus (`/v1/suggest`)

**Problem, gemessen (SP7):** Die heutige FTS5-Phrase-Prefix-Query
(`"token"*`) matcht **jedes Token** eines mehrteiligen Namens, nicht nur den
Anfang — `q=ca` liefert *Kunzea capitata* (Treffer auf dem Epithet-Token), weil
FTS5 Phrasenanker keinen Positions-Bezug zum ganzen Namen kennen.

**Neuer Parameter `match_mode`:**

| Wert | Mechanik | `q=ca` liefert |
|---|---|---|
| `name_start` (**Default**) | Nur der Anfang des vollständigen Namens zählt | *Carex…*, *Calamagrostis…* |
| `anywhere` | Heutiges Verhalten, jedes Token darf treffen | *Kunzea capitata*, *Carex*, *Corynephorus canescens* |

`name_start` braucht eine andere Query-Strategie als reines FTS5-Phrase-
Matching (z.B. `LIKE 'ca%'`-Anker auf dem kanonisierten Vollnamen, kombiniert
mit FTS5-Ranking für die übrige Relevanz) — technischer Implementierungs-
Detail, hier nicht vorgeschrieben.

Rang-übergreifende Suche (Abschnitt 2) bedeutet: `q=Aster` trifft sowohl die
Gattung *Aster* als auch (weil "Aster" ein echtes Präfix ist) die Familie
*Asteraceae* — das ist eine natürliche Konsequenz der Rang-Erweiterung, kein
Sonderfall von `match_mode`.

## 4. API-Form: `/v1/concept/{id}`

```json
{
  "id": "wcvp:concept:xyz",
  "name": "Inula hirta",
  "rank": "SPECIES",
  "name_space": "eurosl",
  "classification": {
    "family": "Asteraceae",
    "order": "Asterales",
    "class": "Magnoliopsida"
  },
  "synonyms": [
    {"name": "Pentanema hirtum", "name_space": "wcvp", "role": "accepted"}
  ],
  "vernacular_names": [
    {"language": "de", "name": "Rauhaariges Alant", "source": "germansl"}
  ],
  "aggregate_memberships": [
    {
      "name_space": "eurosl",
      "aggregate_concept_id": "eurosl:concept:258049b6...",
      "aggregate_name": "Salsola kali aggr."
    }
  ]
}
```

Für ein Aggregat-/Sektions-Konzept (Fall B, Abschnitt 5) trägt die Antwort
stattdessen `members[]` (siehe Abschnitt 5).

## 5. Aggregat-Modell — zwei grundverschiedene Fälle

**Fall A — Anreicherung bestehender WCVP-Konzepte** (Art-/Unterart-Ebene):
EuroSL-/GermanSL-Zeile wird per Name gegen WCVP gejoint (`genuineBearerWinner`-
Tie-Break, siehe Abschnitt 7). Ergebnis hängt sich als Zusatzinfo an ein
bereits existierendes `wcvp:concept:...` — Klassifikation, Vernakularname,
`aggregate_memberships`-Rückverweis.

**Fall B — genuin neue Konzepte** (Ordnung, Klasse, Sektion, Aggregat,
Untergattung): Diese Ränge existieren in WCVP nicht. Es entstehen **eigene**
Konzepte mit eigener ID (`eurosl:concept:...`, `germansl:concept:...`), ohne
WCVP-Pendant:

```json
{
  "id": "eurosl:concept:258049b6...",
  "name": "Salsola kali aggr.",
  "rank": "SPECIES_AGGREGATE",
  "name_space": "eurosl",
  "members": [
    {"concept_id": "wcvp:concept:123", "name": "Salsola kali"}
  ]
}
```

**`concept_agreement` — Vorberechnung, kein Raten zur Laufzeit.** Für jedes
Sammel-Konzept wird beim Ingest das namensgleiche Gegenstück im jeweils
anderen Namensraum gesucht (Namen normalisiert, Marker-Varianten wie
"aggr." / "s. l." ignoriert) und die Mitglieder-Mengen (beide bereits auf
WCVP-Konzept-IDs aufgelöst) verglichen:

| Befund | Bedeutung |
|---|---|
| `identical` | exakt dieselbe Mitglieder-Menge |
| `subset` / `superset` | eine Liste vollständig in der anderen enthalten |
| `overlap` | teilweise gemeinsam, teilweise verschieden |
| `disjoint` | keine gemeinsamen Mitglieder |
| `one_sided` | nur ein Namensraum kennt dieses Konzept |

```json
"agreement": "overlap",
"agreement_detail": {
  "text": "germansl zieht 'Salsola tragus subsp. tragus' als eigene Art heraus, die eurosl unter diesem Aggregat führt",
  "only_in_eurosl": [],
  "only_in_germansl": ["wcvp:concept:..."]
}
```

**Bewusst NICHT gebaut:** eine automatische "Versöhnung" zwischen EuroSL- und
GermanSL-Mitgliederlisten. Der Konflikt wird sichtbar gelassen, nicht
algorithmisch aufgelöst — das wäre eine stille taxonomische Entscheidung, die
hostus nicht treffen darf.

## 6. `/v1/match` — Kernänderungen

**Ausgangsbefund:** `/v1/match` wurde für den **Batch-Import von
Vegetationsaufnahmen** gebaut (Doku-Zitat: *"gedacht für den Import von
Vegetationsaufnahmen mit uneinheitlicher Namensschreibweise"*) — nicht für
interaktive Sippen-Exploration. Genau diese Zielgruppe (Vegetationskundler)
schreibt aber routinemäßig *"Rubus sect. Rubus"* oder *"Achillea millefolium
agg."* — der bisherige `aggregate_alias`-Sackgassen-Mechanismus (*"keine
Kleinartauflösung"*) verfehlt damit seine eigene Kernzielgruppe.

**Änderung 1 — Multi-Namensraum-Suche.** `MatchExact` sucht künftig über
WCVP + EuroSL + GermanSL (nicht mehr WCVP-exklusiv), mit demselben
Rang-Vokabular wie Suggest. Das behebt "Rubus sect. Rubus" strukturell, ohne
einen taxonomiespezifischen Sonderfall zu brauchen — GermanSL trägt das
Konzept bereits.

**Änderung 2 — `classification` immer im Ergebnis.** Bricht bewusst die
bisherige Garantie ("ohne `target_space` byte-identisch"); UC3/UC6 müssen mit
der erweiterten Form leben. Begründung: Klassifikation ist risikoarme
Zusatzinfo (anders als Aggregat-Mitgliedschaft), kein Grund für einen
Parameter-Schalter.

**Änderung 3 — `aggregate_resolution` ist Pflichtfeld bei Sammel-Rängen**
(`SPECIES_AGGREGATE`, `GENUS_AGGREGATE`, `SECTION`, `SUBSECTION`,
`SUBGENUS`). Statt einer stillen Sackgasse liefert der Match-Treffer die
Namensraum-Optionen **direkt im selben Aufruf**:

```json
{
  "id": "2",
  "verbatim": "Achillea millefolium agg.",
  "match_type": "aggregate_alias",
  "concept_id": "eurosl:concept:...",
  "rank": "SPECIES_AGGREGATE",
  "classification": { "family": "Asteraceae", "order": "Asterales", "class": "Magnoliopsida" },
  "aggregate_resolution": {
    "requested_name_space": "eurosl",
    "status": "known",
    "member_count": 16,
    "options": [
      {"name_space": "eurosl", "status": "known", "aggregate_concept_id": "...", "member_count": 16},
      {"name_space": "germansl", "status": "known", "aggregate_concept_id": "...", "member_count": 16},
      {"name_space": "wcvp", "status": "absent"}
    ],
    "agreement": "identical"
  }
}
```

Kostenpunkt: Bei jedem Treffer mit Sammel-Rang werden **alle** ingestierten
Namensräume geprüft, nicht nur der angefragte — akzeptiert als Pflichtkosten
(Nutzerentscheidung geht vor Antwortzeit-Optimierung), kein Opt-in-Flag.

## 7. Crosswalk-Mechanik (Fall A)

Homonym-Handling reicht `genuineBearerWinner` (aus PR #76, `match.go:901`)
unverändert weiter: **Tier 1** (Name ist akzeptierter Name des Kandidaten)
entscheidet, sobald genau ein Kandidat trifft; erst wenn Tier 1 leer bleibt,
entscheidet **Tier 2** (homotypisches Synonym). Eine schwächere Stufe rettet
nie eine mehrdeutige stärkere Stufe (Lehre aus issue #67 / *Beckmannia
eruciformis*). Mehrdeutige Treffer werden gezählt und mit Beispiel gemeldet,
nie geraten — exakt wie im Trait-Crosswalk (PR #76).

Für Fall B gibt es kein Homonym-Problem in diesem Sinne (nichts wird gegen
WCVP gejoint) — die relevante Frage ist stattdessen `concept_agreement`
(Abschnitt 5).

## 8. Traits-Entfernung aus hostus

- `GET /v1/concept/{id}/traits` entfernen (OpenAPI, HTTP-Handler, Doku)
- `trait_value`/`trait_vocabulary`-Tabellen aus dem Schema entfernen
- `internal/application/traits_ingest.go` + Ports (`BeginTraitIngest` etc.)
  entfernen
- `pipelines/{eive,tichy,midolo}/` aus hostus entfernen — **Transfer, kein
  Datenverlust**: Startpunkt für situs' neues Trait-Modul (Teilprojekt 2),
  dort aber neu gedacht (situs-Entscheidung), nicht 1:1 kopiert.
- **Wichtig für Teilprojekt 2:** Die Homonym-Tie-Break-Logik aus
  `traitBearers`/`genuineBearerWinner`, die den 11–18%-Mehrdeutigkeits-Bug
  behoben hat (siehe [[trait-crosswalk-homonyms]]), muss mitwandern — sonst
  reproduziert situs denselben Bug neu.
- CHANGELOG: als Breaking Change dokumentieren.

## 9. `/v1/translate` — Vereinheitlichung

`target_space` akzeptiert künftig sowohl die neuen Namensraum-Ids
(`eurosl`/`germansl`/`wcvp`) als auch weiterhin CDM-`sec.`-UUIDs — ein
Mechanismus für "wie hängen zwei Konzept-Perspektiven zusammen", egal ob die
Perspektiven Namensräume oder Literatur-`sec.`-Räume sind. Antwortform
(`is_equality`, `relation_from_source`, `stored_relation`/`direction`,
Richtungs-Regeln) bleibt unverändert (SP5, bewährt).

**Bestätigt funktionsfähig, kein Redesign nötig:** Der `concept_id` +
`entry_sec`-Einstieg funktioniert bereits (99,67% eindeutig gemessen),
`GET /v1/sec` existiert zur Raum-Discovery. Die tatsächliche Hürde ist ein
fehlender **geführter Workflow** (Raum wählen → Name auflösen → übersetzen)
— explizit an **Teilprojekt 3** (Frontend) delegiert, nicht Teil dieses Specs.

**Voraussetzung, die oft übersehen wird:** CDM muss explizit im Ingest-
Manifest eingebunden werden (~14h-Crawl, nicht Standard) — ohne CDM-Daten
liefert `/v1/translate` für CDM-`sec.`-Räume grundsätzlich nichts, unabhängig
vom API-Design.

## 10. Lizenz-Haltung (PoC-Status, keine Änderung am bestehenden Muster)

hostus ist aktuell eine Machbarkeitsstudie (Owner-Entscheidung, 2026-08-27):

- Ingest-Pipelines laufen **lokal**, keine Lizenz-Klärung wird zum jetzigen
  Zeitpunkt aktiv verfolgt (zu aufwändig, bevor das Konzept trägt).
- **Code bleibt öffentlich** auf GitHub (enthält keine Daten, unproblematisch).
- Die **Rohdaten** (EuroSL, GermanSL, CDM) werden **nicht redistribuiert** —
  jeder Nutzer von hostus führt die Ingests selbst aus.
- Bei einem künftigen Live-Betrieb würde **nicht** die Bulk-Datenbank
  weitergegeben, sondern nur kuratierte Einzelantworten (pro Konzept) — eine
  andere rechtliche Kategorie als "wesentlichen Teil der Datenbank
  weiterverteilen" (relevant für das EU-Datenbankherstellerrecht, sui
  generis, unabhängig vom Urheberrecht an einzelnen Fakten).
- Technisch ändert sich nichts an der bestehenden Redistribution-Gate-Logik
  (Bundle-Export verweigert `redistribution: unknown`-Quellen ohne
  `--force-include-restricted`) — sie wird konsequent auch auf EuroSL/
  GermanSL angewendet, wie es der Code für CDM/FloraVeg bereits tut.

## 11. Fachliche Korrektheits-Tests

Gezielte Absicherung der Stellen, an denen hostus jetzt **interpretiert**
statt nur durchreicht:

1. **Rang-Mapping-Vollständigkeit.** Golden-Liste aller gemessenen Rohwerte
   (Abschnitt 2). Ein neuer, unbekannter Rohwert muss den Ingest sichtbar
   stoppen (ParseRank-Lehre: nie raten, nie still droppen).
2. **Crosswalk-Regression auf echten, gemessenen Fällen** (nicht
   synthetischen Fixtures — eigene Projekt-Lehre: "die 20-Zeilen-Fixture-
   Blindstelle biss viermal"): *Inula hirta* → *Pentanema hirtum* (Tier-2-
   Homonym), *Salsola kali* agg. → `agreement: overlap`, *Rubus* sect.
   *Rubus* → `agreement: one_sided`.
3. **Kein-Fabrikat-Invariante.** Jede abgeleitete Aussage (Klassifikation,
   Aggregat-Mitgliedschaft, `agreement`) muss auf eine konkrete Quellzeile
   (`source_id`) rückführbar sein.
4. **Stille Artefakt-Drift verhindern.** CI-Check pinnt gemessene Kennzahlen
   (287 EuroSL-Aggregate, 26.129 GermanSL-Zeilen etc.) gegen den aktuell
   gepinnten Datenstand — eine Änderung nach einem Quellen-Update muss
   auffallen, nicht kommentarlos durchlaufen.

## 12. Ingest-/Migrationsreihenfolge

```
0. GermanSL neu pinnen (29.09.2025-Stand statt gecachter 1.5.5)
1. WCVP-Ingest (unverändert, bestehend)
2. EuroSL-Ingest:
   a) Fall A — Namens-Crosswalk gegen WCVP (genuineBearerWinner)
   b) Fall B — native Konzepte für Ordnung/Klasse/Sektion/Aggregat
3. GermanSL-Ingest (a+b wie EuroSL) — kann parallel zu 2 laufen, beide
   hängen nur von 1 ab
4. concept_agreement-Vorberechnung — braucht 2+3 vollständig
5. FTS5-Suggest-Index-Rebuild — braucht 2+3 (alle Ränge, alle Namensräume)
6. Traits-Subsystem entfernen — unabhängig, kann jederzeit/zuerst passieren
```

Schritt 4 ist der einzige echte Synchronisationspunkt.

## 13. Offene Punkte

- **GermanSL `AG3`/`CL1`–`CL5`/`UAB`** — Bedeutung/genaue Tier-Abstufung nicht
  abschließend verifiziert (Abschnitt 2). Betrifft 0,14% der Zeilen,
  überwiegend außerhalb des Scopes — niedrige Priorität, aber vor
  Produktiv-Ingest zu klären oder bewusst als "unbekannt, gezählt" zu
  akzeptieren.
- **Teilprojekt 2 (situs-Trait-Modul):** Ellenberg-Quelle noch offen, eigenes
  Brainstorming nötig.
- **Teilprojekt 3 (Frontend):** a11y-Standard, Zielgeräte, Interaktionsmuster,
  `/v1/translate`-Workflow-Führung — eigenes Brainstorming nötig.
- **Teilprojekt 4 (Dokumentation):** Umfang und Straffung noch nicht
  spezifiziert.
- Die byte-identische Rückwärtskompatibilität von `/v1/match` (Abschnitt 6,
  Änderung 2) wird **bewusst** aufgegeben — falls es externe Konsumenten
  gibt, die darauf bauen, ist das vor der Umsetzung zu prüfen.

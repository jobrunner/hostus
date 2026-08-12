# Marker-insensitives Aggregat-Suggest — Design

**Ziel:** Wer im Suggest-Feld eine Aggregat-Schreibweise tippt (`Achillea
millefolium agg.` / `aggr.` / `s.l.`), findet dasselbe Taxon — unabhängig davon,
welchen Marker die Quelle verwendet — und sieht am Treffer, dass er ein Aggregat
getroffen hat.

**Architektur:** Zwei koordinierte Änderungen, keine neue Abhängigkeit. (1)
Query-seitig den Aggregat-Marker vor der FTS-Präfixsuche abstreifen, damit alle
Marker-Schreibweisen auf dieselbe Basis konvergieren. (2) Ingest-seitig die
aufgelösten aggregatmarkierten Namensraum-Namen (FloraVeg) in den FTS-Index
aufnehmen, mit einem `is_aggregate`-Flag, das Suggest bis in die Antwort
durchreicht.

## Ausgangslage (gemessen im Code)

- `internal/adapters/sqlite/suggest.go::ftsPrefixToken` kanonisiert den Query
  (`domain.Canonicalize`: lower + Whitespace + Diakritika) und macht daraus eine
  FTS5-Phrasen-Präfixsuche `"<canon>"*`. **Keine** Marker-Normalisierung.
- WCVP führt **keine** aggregatmarkierten Namen. „Achillea millefolium agg."
  sucht ein drittes Token mit Präfix „agg" — die 2-Token-Nominatart erfüllt das
  nicht → **leeres Ergebnis** (der gemeldete Bug).
- `fts_name` wird nur aus `concept_name` befüllt (akzeptiert + Synonyme).
  `name_space_entry` (FloraVeg/EuroSL) ist **nicht** im FTS-Index.
- Suggest **dedupliziert bereits** pro Konzept (`GROUP BY tc.id`, `MIN(score)`)
  und zeigt den **akzeptierten** Namen (`tc.accepted_name`).
- `domain` kennt die Aggregat-Marker bereits: `aggregateMarkers`
  (`agg./aggr./s.l./s.lat./sl.`) + `sensuQualifiers` in `normalize.go`, plus
  `AggregateBases`, das sie Schicht für Schicht abstreift. FloraVeg-Aggregate
  crosswalken per `aggregate_to_nominate` auf die **Nominatart**.

## Komponenten

### 1. Domain: Marker-Stripping als wiederverwendbarer Helfer
`internal/domain/normalize.go`: neue Funktion
`StripAggregateMarkers(canon string) string`, die die **voll abgestreifte**
Basis zurückgibt — das **letzte** Element von `AggregateBases(canon)` (alle
Marker-Schichten entfernt) — bzw. den unveränderten Namen, wenn `AggregateBases`
`nil` liefert (kein Marker). Sie nutzt exakt die vorhandene `aggregateMarkers`/
`sensuQualifiers`-Logik — **eine** Marker-Definition, kein zweiter Ort.

### 2. sqlite: Query-Normalisierung
`ftsPrefixToken` ruft nach `Canonicalize` zusätzlich
`domain.StripAggregateMarkers` auf, bevor es die Präfix-Phrase baut. Damit
suchen `X agg.`, `X aggr.`, `X s.l.` alle `"x"*` und finden die Nominatart **und**
jeden indexierten Alias, der mit `x` beginnt.

### 3. Schema + Ingest: Aggregat-Aliase in den FTS-Index
- Schema (`schema.sql`): `fts_name_map` erhält
  `is_aggregate INTEGER NOT NULL DEFAULT 0`. Die Schema-Spaltenprüfung in
  `db.go` (`expectedSchemaColumns`) wird entsprechend erweitert.
- Ingest: nach dem Namensraum-Crosswalk werden die `name_space_entry`-Zeilen mit
  `aggregate = 1 AND concept_id <> ''` zusätzlich als `fts_name`-Zeilen
  eingefügt (`canonical = Canonicalize(name)`, `fts_name_map.concept_id =
  entry.concept_id`, `is_aggregate = 1`). Nur aufgelöste Einträge — ein
  Suggest-Treffer ist immer ein Konzept.

### 4. Suggest-Query trägt das Flag
Die Suggest-SQL ergänzt `MAX(fnm.is_aggregate) AS aggregate` in der bestehenden
`GROUP BY tc.id`-Aggregation. `domain.SuggestItem` erhält `Aggregate bool`;
`scanSuggestItem` liest die Spalte.

### 5. API + Konsole
- `suggestItemDTO` erhält `Aggregate bool \`json:"aggregate,omitempty"\``.
- OpenAPI `SuggestItem`-Schema erhält die Property (der Schema-Contract-Test
  `TestOpenAPISchemasMatchDTOs` erzwingt das) + `http-api.md`.
- Konsole (`app.js`/`index.html`): ist `aggregate` true, zeigt die Suggest-Zeile
  ein Badge „agg." neben dem akzeptierten Namen.

## Datenfluss

`"Achillea millefolium agg."` → `Canonicalize` → `"achillea millefolium agg."`
→ `StripAggregateMarkers` → `"achillea millefolium"` → FTS `"achillea
millefolium"*` → trifft die Nominatart-Namenszeile **und** die (per Ingest
eingefügte) Aggregat-Alias-Zeile → beide zeigen auf das Nominatart-Konzept →
`GROUP BY tc.id` kollabiert zu einer Zeile, `MAX(is_aggregate)=1` →
`SuggestItem.Aggregate=true` → Konsole zeigt „Achillea millefolium" + Badge.

## Semantik der Badge (bewusste Entscheidung)

Das Aggregat hat **kein eigenes Konzept** (FloraVeg-Aggregate zeigen auf die
Nominatart), daher ist ein Aggregat kein separater Treffer, sondern das
Nominatart-Konzept **mit Badge**. Das Badge erscheint, sobald eine
Aggregat-Schreibweise auf das Konzept auflöst — auch bei der schlichten Anfrage
„Achillea millefolium", weil das Konzept dann eben (auch) ein Aggregat trägt.
Das ist ehrlich: es sagt „dieses Taxon hat eine Aggregat-Lesart", nicht „du hast
exakt das Aggregat getippt".

## Fehlerbehandlung

Keine neuen Fehlerpfade. Ein Namensraum-Eintrag ohne `concept_id` wird
übersprungen (nicht indexierbar). `StripAggregateMarkers` ohne Marker gibt den
Namen unverändert zurück (die heutige Form bleibt für nicht-aggregate Anfragen
byteweise erhalten).

## Tests (TDD)

- **domain:** `StripAggregateMarkers` — jeder Marker (`agg./aggr./s.l./s. l./
  s.lat./sl.`), geschichtet (`… aggr. s. l.`), und der Nicht-Marker-Fall
  (unverändert), inkl. `s. str.` bleibt unangetastet.
- **sqlite (ingest):** ein aggregatmarkierter `name_space_entry` mit
  `concept_id` landet als `fts_name`-Zeile mit `is_aggregate=1`; ein
  unaufgelöster wird übersprungen.
- **sqlite (suggest):** `"X agg."` und `"X s.l."` liefern das Nominat-Konzept;
  `Aggregate=true`, wenn ein Aggregat-Alias auf das Konzept zeigt; eine reine
  Nicht-Aggregat-Art hat `Aggregate=false`.
- **http:** `suggestItemDTO.aggregate` erscheint (nur wenn true); Schema-Contract
  grün.
- **Konsole:** Serve-Smoke gegen die reale konsolidierte DB — „Achillea
  millefolium agg." liefert das Taxon mit Badge.

## Bewusst außerhalb des Scopes

- Nur **aggregatmarkierte** Namensraum-Namen werden indexiert, nicht jeder
  FloraVeg/EuroSL-Name (das wäre eine breitere Produktentscheidung).
- Kein eigenes Aggregat-**Konzept** — solange keine aggregat-tragende Quelle
  ingestiert ist, bleibt es die Nominatart mit Badge (siehe SP9/UC4-Verdikt).

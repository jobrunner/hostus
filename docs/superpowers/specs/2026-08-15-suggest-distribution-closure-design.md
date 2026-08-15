# Suggest: Verbreitungs-Closure (`distribution_effective`) — Design

**Status:** Entwurf zur Review
**Datum:** 2026-08-15
**Kontext-Branch:** `feature/suggest-short-prefix-speed`

## Problem

`GET /v1/suggest?q=<präfix>&area=<code>` ist für **breite Kurz-Präfixe** langsam
(2 Zeichen wie „ca" → ~104k FTS-Treffer; volle Query ~1,8 s ohne, ~1 s mit
`area`). Hinter einem Reverse-Proxy äußert sich das als 502/504.

Messungen (voller Index, `full.sqlite`) haben die Ursache eingegrenzt:

- Der FTS-Präfix-Scan selbst ist billig (~12 ms). Die Kosten stecken im
  **Downstream** pro Treffer-Zeile: Join → `GROUP BY` → `in_area`.
- `bm25` diskriminiert bei Kurz-Präfixen kaum (für „ca" teilen ~46k Namen
  denselben Boundary-Score) — ein bm25-Pool kappt also weitgehend willkürlich.
- Ein reiner bm25-Pool (Top-5000) ist schnell (~0,16 s), **verliert aber
  in-area-Recall**: `in_area` ist der *primäre* Sortier-Schlüssel, also fallen
  in **sparse Gebieten** (weniger in-area-Konzepte als eine Ergebnisseite)
  Seite-1-Treffer weg. Verifiziert: `ca`+PHX 57 → 20 in-area-Konzepte.
- Der teure Anteil von `in_area` ist die **Namens-Ableitung** für CDM-Konzepte
  (0 eigene Verbreitung; in-area nur, wenn der gleichnamige WCVP-Zwilling im
  Gebiet ist). Diese über die Treffermenge auszuwerten kostet ~1 s; per Union
  nachzurüsten sogar 4–8 s.

**Kern-Einsicht:** Der Namens-Fallback ist **deterministisch aus den Daten**.
Er gehört nicht in jede Laufzeit-Query, sondern **einmal in eine abgeleitete
Tabelle**.

## Ziel

`suggest?q=<kurzpräfix>&area=<code>` **schnell** (~0,3–0,4 s warm) **und voll
korrekt für alle Gebiete** (jedes in-area-Konzept — eigene Verbreitung *und*
CDM-Namens-Ableitung — erscheint auf Seite 1). Kurz-Präfixe wie „ca" bleiben
erlaubt. Match-Semantik (FTS any-token) unverändert.

## Design

### Abgeleitete Tabelle `distribution_effective`

Pro Konzept die **effektive** Verbreitung — exakt die Semantik des heutigen
`in_area`-Ausdrucks, aber vorberechnet:

- Konzept **mit** eigener Verbreitung → seine eigenen Gebiete (`origin='own'`).
- Konzept **ohne** eigene Verbreitung, mit nicht-leerem `canonical_fold` → die
  Gebiete jedes gleichnamigen (`canonical_fold`-gleichen) **WCVP**-Konzepts
  (`origin='name'`).

```sql
CREATE TABLE IF NOT EXISTS distribution_effective (
  concept_id  TEXT NOT NULL REFERENCES taxon_concept(id),
  area_scheme TEXT NOT NULL,
  area_code   TEXT NOT NULL,
  origin      TEXT NOT NULL,          -- 'own' | 'name' (Provenienz, für Debug/Tests)
  PRIMARY KEY (concept_id, area_scheme, area_code)
);
CREATE INDEX IF NOT EXISTS idx_distribution_effective_area
  ON distribution_effective(area_scheme, area_code);
```

Damit ist `in_area` eine simple, indizierte Eigenschaft — die ~30-s-Subquery,
beide `|| ''`-Planner-Hacks und der `canonical_fold <> ''`-Guard aus
`suggest.go` **entfallen**.

### Aufbau der Closure

Zwei Schritte, idempotent (`INSERT OR IGNORE`):

1. `INSERT ... SELECT concept_id, area_scheme, area_code, 'own' FROM distribution`.
2. Für jedes Konzept **ohne** Zeile aus Schritt 1 und mit `canonical_fold <> ''`:
   `INSERT ... SELECT c.id, wd.area_scheme, wd.area_code, 'name'` über den
   Fold-Join `accepted_name.canonical_fold = wn.canonical_fold` →
   `concept_name` → `taxon_concept (backbone_id='wcvp')` → `distribution wd`.

Umfang (gemessen): 51.466 CDM-Konzepte ohne eigene Verbreitung; die Closure-
Zeilen sind durch (Konzept × Gebiete des Zwillings) beschränkt.

**Wann:** als **Finalize-Schritt des Ingest** (nach dem Laden aller Backbones —
die WCVP-Zwillinge müssen vorhanden sein). Bei Re-Ingest wird die Tabelle
geleert und neu aufgebaut.

**Bestehende DBs (ohne Re-Ingest):** ein **idempotenter Build beim `Open`**
(wie die vorhandenen `migrateXrefSourceColumn`/`migrateConceptRelationPK`):
ist `distribution_effective` leer, aber `distribution` befüllt, wird die Closure
einmalig aufgebaut. So heilen bestehende DBs sich selbst; ein Re-Ingest ist
nicht erforderlich, nur ein einmaliger Build (Sekunden bis wenige Minuten auf
`full.sqlite`, einmalig).

### Laufzeit-Query (`suggest.go`)

- `in_area` →
  `EXISTS (SELECT 1 FROM distribution_effective de WHERE de.concept_id = tc.id
   AND de.area_scheme = 'wgsrpd_l3' AND de.area_code IN (?))` — PK-Punktlookup.
- Recall-Union (`in_area_rows`) →
  `SELECT DISTINCT fnm.rowid FROM distribution_effective de
   JOIN fts_name_map fnm ON fnm.concept_id = de.concept_id
   WHERE de.area_scheme='wgsrpd_l3' AND de.area_code IN (?)
     AND fnm.rowid IN (SELECT rowid FROM match_rows)` — über
  `idx_distribution_effective_area`; deckt eigene **und** CDM-in-area ab.
- `matches = pool (Top-`suggestMatchPool` bm25) ∪ in_area_rows`.

Erwartung (aus Prototyp-Messungen der own-Union, jetzt vollständig): ~0,3–0,4 s
warm, voller Recall für alle Gebiete. Der cache-kalte Erstlauf eines breiten
Präfixes bleibt ~1–1,5 s (zwei FTS-Scans kalt).

## Datenfluss

```
Ingest (alle Backbones) ──finalize──▶ distribution_effective (own ∪ name-closure)
                                            │
serve: Suggest(area) ── pool(bm25) ∪ (distribution_effective∩matches) ──▶ Kandidaten
                                            │
                              domain.RankSuggestions ──▶ Top-limit
```

## Fehlerbehandlung / Invarianten

- Closure-Build in einer Transaktion; schlägt er fehl, schlägt Ingest/Open fehl
  (kein halb gefüllter Zustand). `INSERT OR IGNORE` macht Wiederholung sicher.
- `distribution_effective` ist rein abgeleitet — nie von außen beschrieben,
  immer aus `distribution` + `name`/`concept_name` reproduzierbar.
- `readiness` unverändert (hängt an eingelesenem Backbone, nicht an der Closure).

## Tests (TDD)

1. **Closure-Korrektheit:** WCVP mit eigener Verbreitung → nur `own`-Zeilen;
   CDM ohne eigene Verbreitung mit WCVP-Zwilling in GER → `('…','wgsrpd_l3',
   'GER','name')`; CDM ohne Zwilling → keine Zeile; leerer Fold → keine Zeile.
2. **Recall (der eigentliche Bug):** sparse Gebiet + breiter Präfix, in dem ein
   CDM-Konzept (kein eigenes Vorkommen) via Zwilling in-area ist und schlechtes
   bm25 hat → erscheint trotzdem (pool klein gesetzt). Rot ohne Closure-Union.
3. **`in_area`-Äquivalenz:** der neue `in_area`-Wert stimmt für alle bestehenden
   `TestSuggest_InArea…`-Fälle mit dem alten (fallback-basierten) überein.
4. **Migration/Build beim Open:** DB mit befülltem `distribution` aber leerer
   `distribution_effective` → nach `Open` ist die Closure da.
5. **Perf-Guard/Mutation:** `make verify` grün, `Not covered = 0`; Realdaten-
   Messung (`ca`+GER, `ca`+PHX) dokumentiert.

## Verworfene Alternativen

- **C1 — Kanonisch-Präfix-Matching** (`canonical_fold`-Range statt FTS): „ca"
  hat immer noch ~57k Präfix-Namen / 26k Konzepte (nur ~halbiert) → kein
  drastischer Gewinn, und verliert Epitheton-/Mid-Name-Treffer (Semantik-Bruch).
- **Laufzeit-Namens-Fallback** (heutiger Stand): entweder ~1 s (voll korrekt)
  oder Recall-Lücke (bm25-Pool) — der Trade-off, den dieses Design auflöst.
- **Voller Recall via Laufzeit-Union** (own + fallback): 4–8 s — unbrauchbar.

## Offene Punkte (für Review)

1. Closure **beim Ingest-Finalize**, **beim Open** oder **beidem** (Ingest baut,
   Open heilt bestehende DBs)? Vorschlag: beidem.
2. Spalte `origin` behalten (nützlich für Debug/`/v1/concept` später) oder
   weglassen (schlanker)? Vorschlag: behalten.
3. `suggestMatchPool` = 5000 beibehalten? Da Recall jetzt garantiert per Union
   kommt, steuert der Pool nur noch die Nicht-in-area-Relevanz-Füllung und
   könnte kleiner sein — separater, späterer Tuning-Schritt.

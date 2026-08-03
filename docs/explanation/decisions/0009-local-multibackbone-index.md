# ADR-0009: Lokaler Multi-Backbone-Index

**Status:** Accepted

**Ersetzt:** ADR-001 (GBIF als alleiniger Provider), ADR-003 und ADR-008 (kein
Persistenz-Layer) aus `architecture/adrs.md` im Repository-Root.

## Kontext

hostus 1.x war ein zustandsloser Proxy vor der GBIF-REST-API: keine eigene
Persistenz, GBIF als einzige Taxonomie-Quelle, In-Memory-Cache mit TTL. Das
Master-Architektur-Spec (`docs/superpowers/specs/2026-07-31-hostus-2.0-architecture.md`)
verlangt jedoch einen **Multi-Backbone-Namens- und Merkmalsservice**: COL XR,
WCVP/POWO, Euro+Med, FloraVeg.EU/ESy sowie mehrere Trait-Vokabulare (EIVE,
Tichý, Midolo) müssen gleichzeitig, versioniert und mit reproduzierbarer
`sec.`-Semantik bedienbar sein. Das ist mit einem reinen GBIF-Proxy ohne
eigenen Zustand nicht abbildbar: GBIF kennt weder Euro+Med- noch
FloraVeg-Konzepte, und ein In-Memory-Cache verliert bei jedem Neustart die
Fähigkeit, mehrere Backbone-Versionen nebeneinander zu bedienen.

## Entscheidung

hostus 2.0 betreibt einen **lokalen, aus versionierten Artefakten gespeisten
Index** (SQLite/FTS5, siehe [ADR-0010](0010-sqlite-fts5-persistence.md)).
Backbone-Daten werden per Ingest-Pipeline aus gepinnten Artefakt-Versionen
(niemals „latest") importiert; GBIF wird — wo überhaupt noch verwendet — nur
noch als ein Anreicherungs-/Ingest-Datenlieferant unter mehreren behandelt,
nicht mehr als alleiniger Provider zur Laufzeit. Die drei Ebenen Nomenklatur
(`name`), Taxonomie/Konzept (`taxon_concept`) und Merkmal (`trait_value`)
bleiben strikt getrennt (Architektur-Invariante A.1 des Spec-Dokuments).

## Konsequenzen

- hostus ist kein zustandsloser Proxy mehr, sondern führt ein eigenes,
  wachsendes Storage-Schema (siehe ADR-0010) und eine Ingest-Pipeline pro
  Backbone.
- Persistenz erfordert Migrationen/Schema-Disziplin, die ADR-003/-008
  bewusst ausschließen wollten — dieser Trade-off wird hier explizit
  akzeptiert, weil die fachliche Anforderung (Multi-Backbone, `sec.`-Semantik,
  Offline-Bundle) ohne eigenen Zustand nicht erfüllbar ist.
- Unabhängigkeit von GBIF-Verfügbarkeit und Rate-Limits zur Laufzeit; höhere
  Betriebskomplexität (Ingest, Versionierung, Re-Ingest bei Backbone-Wechsel).
- `internal/gbif` und der GBIF-gestützte `suggest`-Handler aus 1.x entfallen;
  neue Use Cases (Suggest, Match, ResolveConcept, ReverseXref, Traits,
  Synonyms, Translate, Ingest, Bundle) treten an ihre Stelle.

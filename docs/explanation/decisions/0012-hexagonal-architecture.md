# ADR-0012: Hexagonale Architektur (Ports & Adapters)

**Status:** Accepted

## Kontext

Der Wechsel von einem einfachen GBIF-Proxy zu einem Multi-Backbone-Index mit
mehreren Adaptern (SQLite/FTS5, ColDP-Import, HTTP, Debug-MCP, Telemetrie,
Offline-Bundle) und mehreren Use Cases (Suggest, Match, ResolveConcept,
ReverseXref, Traits, Synonyms, Translate, Ingest, Bundle) verlangt eine
Struktur, die Domänenlogik von I/O-Details trennt — sonst verwachsen
Storage-, HTTP- und Ingest-Belange schnell zu einem monolithischen Paket.

## Entscheidung

hostus übernimmt die hexagonale Struktur (Ports & Adapters) aus ortus:

```
internal/
  domain/        # Name, Concept, Trait, Xref, Distribution, Relation — keine I/O-Deps
  application/   # Use Cases
  ports/
    input/       # von der Application angebotene Interfaces
    output/      # von der Application benötigte Interfaces (Repository, TraitStore, …)
  adapters/
    sqlite/  coldp/  http/  mcp/  telemetry/  bundle/
  app/           # Composition-Root (Wiring)
  config/        # viper
```

Die Hexagon-Grenzen werden nicht nur durch Konvention, sondern durch
`depguard`/`gomodguard` im Linter erzwungen (`make arch`, Teil von `make
verify`), sodass unerlaubte Importe zwischen Schichten am CI-Gate scheitern.

## Konsequenzen

- Domäne bleibt frei von SQLite-, HTTP- und MCP-Details testbar.
- Neue Adapter (z. B. weitere Backbones oder ein künftiges REST-Xref) lassen
  sich hinzufügen, ohne die Application- oder Domain-Schicht anzufassen.
- Der Umbau ist eine Inversion, kein Ausbau: `internal/gbif` und der
  GBIF-spezifische Taxonomie-Mapper aus 1.x entfallen ersatzlos.
- Architektur-Fitness-Funktionen (`make arch`) sind ab SP0 Teil des
  kanonischen Grün-Checks, nicht nachträglich ergänzt.

# ADR-0010: SQLite/FTS5-Persistenz via `modernc.org/sqlite`

**Status:** Accepted

**Voraussetzung für:** [ADR-0009](0009-local-multibackbone-index.md) (lokaler Multi-Backbone-Index)

## Kontext

Der lokale Index braucht einen Persistenz- und Such-Layer für Präfix-Suche
(Autosuggest), Volltext auf kanonischen und volkssprachlichen Namen sowie
relationale Abfragen über `name`/`taxon_concept`/`trait_value`. Das
Offline-Bundle (gefilterte Kopie für einen Bezugsraum, ~10–20 MB) muss exakt
dasselbe Schema verwenden wie der Server. gcc-basierte SQLite-Bindings
(`mattn/go-sqlite3`) würden CGO erzwingen und damit den bisherigen
CGO-freien, distroless-tauglichen Build (ADR-006) gefährden.

## Entscheidung

Persistenz erfolgt über SQLite mit `modernc.org/sqlite` — einer reinen
Go-Implementierung ohne CGO-Abhängigkeit — inklusive der virtuellen
FTS5-Tabelle für Präfix-/Volltextsuche (`tokenize='unicode61
remove_diacritics 2'`). Die FTS5-Verfügbarkeit inkl. `bm25()`-Ranking wird
vor SP1 durch PoC P1 gegen Echtdaten verifiziert (Hard Gate). Das Schema
trennt die drei Ebenen strikt in eigene Tabellen (`name`, `taxon_concept`,
`concept_name`, `trait_value`, `xref`, `vernacular`, `distribution`,
`concept_relation`, `backbone_version`) statt einer flachen Struktur.

## Konsequenzen

- Der Build bleibt CGO-frei; distroless-Multi-Arch-Images (amd64/arm64) sind
  weiterhin ohne libc-Abhängigkeiten für SQLite baubar.
- FTS5-Funktionsumfang und Performance von `modernc.org/sqlite` sind eine
  tragende Annahme — scheitert PoC P1, muss die Architektur vor SP1
  angepasst werden (Hard Gate, siehe Spec Abschnitt 3).
- Server-Index und Offline-Bundle nutzen identisches Schema; das Bundle ist
  eine gefilterte Kopie mit eigenem `snapshot_version`-Feld.
- Migrationen und Schema-Versionierung werden Teil der Betriebsverantwortung
  von hostus (siehe auch ADR-0009).

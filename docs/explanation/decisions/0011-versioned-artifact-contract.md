# ADR-0011: Versionierter Artefaktvertrag (`dataset.yaml` + eingebettetes JSON Schema)

**Status:** Accepted

## Kontext

Backbone- und Trait-Daten kommen aus heterogenen externen Quellen (ColDP,
TSV, REST/SPARQL) mit unterschiedlichen Lizenzen, Versionierungsschemata und
Aktualisierungsrhythmen. Ohne einen erzwungenen Vertrag zwischen
Ingest-Pipeline und hostus-Import würden fehlerhafte oder unversionierte
Artefakte unbemerkt in den Index gelangen — insbesondere problematisch, weil
IDs zwischen Backbone-Versionen gelöscht/neu vergeben werden können
(Architektur-Invariante: „Versionen immer pinnen, nie `latest`").

## Entscheidung

Jedes Backbone-/Trait-Bundle wird durch ein **Fixed-Name-Manifest
`dataset.yaml`** beschrieben (Felder `backbones:` und
`trait_vocabularies:` mit gepinnten Versionen/Lizenzen), das gegen ein **im
hostus-Binary eingebettetes JSON Schema** (JSON Schema 2020-12) validiert
wird — doppelt gate-t: die Ingest-Pipeline (`pipelines/<backbone>/build.sh`)
validiert beim Bauen, hostus validiert erneut beim Ingest
(`hostus ingest --dataset dataset.yaml` bzw. `hostus validate --dataset
dataset.yaml`) mit `KnownFields(true)` — unbekannte Felder sind ein Fehler,
kein stillschweigend ignoriertes Extra. Jede Artefakt-Identität (`id`)
kodiert Version/Periode; „latest" ist kein gültiger Wert. Eine Checksumme
(`manifest_sha`) bindet den serverseitigen Ingest an das exakt validierte
Manifest.

## Konsequenzen

- Pipeline und hostus können unabhängig voneinander weiterentwickelt werden,
  solange sie sich am selben, versionierten Schema orientieren.
- Ein Backbone-Wechsel ist immer ein expliziter Re-Ingest, nie eine
  stillschweigende Migration — reproduzierbar und auditierbar
  (`backbone_version`-Tabelle inkl. `manifest_sha`, `ingested_at`).
- Schema-Änderungen erfordern eine bewusste Versionsanhebung des
  eingebetteten JSON Schemas; Alt-Manifeste ohne Anpassung schlagen beim
  Ingest fehl, statt inkonsistente Daten zu erzeugen.
- Der Namenskonflikt mit dem ortus-Muster (`ortus-<x>.yaml`) wird zugunsten
  der Doc-treuen Bezeichnung `dataset.yaml` aufgelöst; das Schema-Muster
  selbst folgt weiterhin ortus.

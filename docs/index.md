# hostus-Dokumentation

hostus ist ein hochperformanter Go-Backend-Service: ein lokaler,
schreibgeschützter Namens- und Merkmalsdienst für Gefäßpflanzen, der ein
Autosuggest-Feld im Frontend bedient. Der Service betreibt einen eigenen,
versionierten Multi-Backbone-Index (COL XR, WCVP/POWO, Euro+Med,
FloraVeg.EU) in SQLite/FTS5, gespeist aus fixierten Ingest-Artefakten, und
gruppiert Synonyme unter ihrem akzeptierten Taxon. GBIF ist dabei höchstens
eine von mehreren Ingest-/Enrichment-Quellen, nicht der Laufzeit-Provider.

Diese Dokumentation folgt dem [Diátaxis](https://diataxis.fr/)-Framework —
vier Dokumentationsarten für vier unterschiedliche Bedürfnisse:

| | |
|---|---|
| **[Tutorials](tutorials/index.md)** | Lernorientiert — hier starten für den ersten Lauf. |
| **[How-to-Anleitungen](how-to/index.md)** | Aufgabenorientiert — Rezepte für ein konkretes Ziel (Entwicklungsumgebung, Betrieb). |
| **[Referenz](reference/index.md)** | Informationsorientiert — HTTP-API, Konfiguration, Observability. |
| **[Erklärung](explanation/index.md)** | Verständnisorientiert — Architektur, Entscheidungen (ADRs). |

Neu hier? Beginne mit **[Erste Schritte](tutorials/getting-started.md)**.

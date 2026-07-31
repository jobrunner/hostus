# hostus-Dokumentation

hostus ist ein hochperformanter Go-Backend-Service, der als schreibgeschützter
Taxonomie-Gateway für ein Autosuggest-Feld (Gefäßpflanzen) im Frontend dient.
Der Service proxied Anfragen an die GBIF-REST-API, cached Antworten im
Speicher und gruppiert Synonyme unter ihrem akzeptierten Taxon.

Diese Dokumentation folgt dem [Diátaxis](https://diataxis.fr/)-Framework —
vier Dokumentationsarten für vier unterschiedliche Bedürfnisse:

| | |
|---|---|
| **[Tutorials](tutorials/index.md)** | Lernorientiert — hier starten für den ersten Lauf. |
| **[How-to-Anleitungen](how-to/index.md)** | Aufgabenorientiert — Rezepte für ein konkretes Ziel (Entwicklungsumgebung, Betrieb). |
| **[Referenz](reference/index.md)** | Informationsorientiert — HTTP-API, Konfiguration, Observability. |
| **[Erklärung](explanation/index.md)** | Verständnisorientiert — Architektur, Entscheidungen (ADRs). |

Neu hier? Beginne mit **[Erste Schritte](tutorials/getting-started.md)**.

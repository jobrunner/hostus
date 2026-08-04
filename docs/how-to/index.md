# How-to-Anleitungen

Aufgabenorientierte Rezepte — jedes löst ein konkretes Problem. Sie setzen
[Erste Schritte](../tutorials/getting-started.md) voraus.

- **[Entwicklungsumgebung einrichten](development.md)**
- **[Offline-Bundle exportieren](offline-bundle.md)** — `hostus bundle` für
  gebietsgescoptes, feldeinsatztaugliches Offline-Serving
- **[Merkmalswerte (Traits) pipeln und ingestieren](trait-ingest.md)** —
  EIVE/Tichý/Midolo von der Zenodo-Quelle bis `GET /v1/concept/{id}/traits`,
  inkl. Attributionspflicht und dokumentiertem Lizenz-Scope-Schnitt
- **[Von hostus zu iNaturalist (UC2)](inat-uc2.md)**
- **[Konzepte zwischen `sec.`-Räumen übersetzen (UC6)](sec-translate-uc6.md)**
  — `POST /v1/translate`, typisierte Konzeptrelationen, Ein-Hop-Grenze und
  die Lizenzlage der CDM-Daten
- **[Publikationsfähige Synonyme filtern (UC5)](synonyms-uc5.md)** —
  `GET /v1/concept/{id}/synonyms?relevance=publication`, das durchgerechnete
  *Corynephorus-canescens*-Beispiel und ausdrücklich: **was dieser Filter
  nicht kann** (keine regionale, keine Standardwerk-Filterung)
- **[Die eingebettete Testkonsole benutzen](test-console.md)** — die SPA
  unter `/`, der Schalter in allen drei Stufen, wozu die vier Panels da
  sind, ausdrücklich **was die Konsole nicht ist**, und was man an bekannten
  Mängeln zu sehen bekommt

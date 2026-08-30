# How-to-Anleitungen

Aufgabenorientierte Rezepte — jedes löst ein konkretes Problem. Sie setzen
[Erste Schritte](../tutorials/getting-started.md) voraus.

- **[Entwicklungsumgebung einrichten](development.md)**
- **[Offline-Bundle exportieren](offline-bundle.md)** — `hostus bundle` für
  gebietsgescoptes, feldeinsatztaugliches Offline-Serving
- **[Von hostus zu iNaturalist (UC2)](inat-uc2.md)**
- **[Konzepte zwischen `sec.`-Räumen übersetzen (UC6)](sec-translate-uc6.md)**
  — `POST /v1/translate`, typisierte Konzeptrelationen, Ein-Hop-Grenze und
  die Lizenzlage der CDM-Daten
- **[Publikationsfähige Synonyme filtern (UC5)](synonyms-uc5.md)** —
  `GET /v1/concept/{id}/synonyms?relevance=publication`, das durchgerechnete
  *Corynephorus-canescens*-Beispiel und ausdrücklich: **was dieser Filter
  nicht kann** (keine regionale, keine Standardwerk-Filterung)
- **[Aufnahmen in den ESy-Namensraum auflösen (UC4)](aggregate-uc4.md)** —
  `POST /v1/match` mit `target_space`, die dreiwertige `aggregate_policy`, die
  durchgerechnete Beispielaufnahme und ausdrücklich **was fehlt**: die nicht
  bestimmbare `esy_diagnostic_relevance` ohne ESy-Regelwerk
- **[Die eingebettete Testkonsole benutzen](test-console.md)** — die SPA
  unter `/`, der Schalter in allen drei Stufen, wozu die vier Panels da
  sind, ausdrücklich **was die Konsole nicht ist**, und was man an bekannten
  Mängeln zu sehen bekommt

# SP9 / UC4 — Verdikt: **hält mit Auflagen**

Stand: 2026-08-05. Grundlage: der ingestierte FloraVeg-Namensraum
([Messung](floraveg-namespace.md)), die implementierten Felder `target_space`,
`aggregate_policy`, `target_space_name`, `esy_diagnostic_relevance` auf
`POST /v1/match`, und die e2e-Auflösung einer Beispielaufnahme
(`internal/app/integration_targetspace_test.go`).

## Die Frage

UC4 verlangt, pro aufgelöster Aufnahme-Zeile: einen ESy-kompatiblen Namen,
eine `aggregate_policy` und eine `esy_diagnostic_relevance`. Auf `master` waren
**alle drei abwesend**. SP9 sollte die *baubare* Hälfte liefern und die
*unbaubare* Hälfte **sichtbar abwesend** machen statt still fehlend.

## Was hält

- **`aggregate_policy` ist dreiwertig und gemessen.** `known`,
  `unresolvable` und der abwesende dritte Zustand (gewöhnliche Art) sind
  jeweils durch Unit-, HTTP- und e2e-Tests gepinnt. Der Wert stammt aus
  **derselben** Aggregat-Prädikatsfunktion (`domain.IsAggregateName`) und
  demselben SP3-Crosswalk, den der Ingest nutzt — kein zweiter
  Auflösungspfad.
- **`target_space_name` liefert die ESy-Schreibweise** aus dem gemessenen
  Crosswalk (14.050 von 16.402 FloraVeg-Namen auf ein WCVP-Konzept, 85,7 %;
  246 von 309 Aggregaten aufgelöst).
- **`esy_diagnostic_relevance` ist konspikuierend abwesend, nicht still
  fehlend.** Bei gesetztem `target_space` immer present, immer
  `not_determinable` — ein selbsterklärender String, niemals `null`, damit er
  in keiner Sprache als falsy-„nicht relevant" gelesen werden kann.
- **Opt-in-Vertrag gehalten.** Ohne `target_space` ist die Antwort byteweise
  die SP1-Form; UC3/UC6 am selben Endpunkt sehen keine Änderung. Ein
  unbekannter `target_space` ist `400 INVALID_QUERY` und nennt den Raum.

## Die Auflagen

1. **`known` ist über einem WCVP-only-Backbone praktisch unerreichbar** — und
   das ist ein gemessener Befund, kein Defekt. Eine Aggregat-Anfrage
   („Festuca ovina agg.") liefert `known` nur, wenn sie auf ein Konzept
   auflöst, das eine FloraVeg-Aggregatschreibweise trägt. WCVP führt **null**
   aggregatmarkierte Konzepte; FloraVegs „Festuca ovina aggr." landet per
   `aggregate_to_nominate` auf der **Nominatart** *Festuca ovina* (415853),
   erreichbar nur über die *einfache* Anfrage → Zustand „fehlt". Die
   Aggregat-Anfrage selbst löst mangels Aggregat-Konzept **gar nicht** auf
   (`unresolvable` als Match, nicht als Policy). Die e2e demonstriert `known`
   deshalb über ein eigens gesetztes Aggregat-Konzept — den Zustand, den ein
   Backbone *mit* Aggregat-Taxa (etwa ein künftiges Aggregat-Vokabular)
   natürlich erzeugen würde.
   *Konsequenz:* `aggregate_policy` ist heute vor allem als **`unresolvable`-
   Signal** wertvoll (die Aufnahme nennt ein Aggregat, hostus kann es nicht
   auf ein zuweisbares Taxon abbilden). Der volle Nutzen von `known` setzt
   eine Aggregat-tragende Quelle voraus.

2. **Die Mehrdeutigkeit ist die eigentliche Deckenhöhe des Crosswalks.** Mit
   12,2 % ambiguous (fünfmal so viel wie unmatched) ist der nächste Hebel für
   Abdeckung **Disambiguierung**, nicht mehr Schreibweisenregeln — bewusst
   außerhalb dieser Aufgabe, weil der Crosswalk mit dem SP3-Trait-Ingest
   geteilt ist ([Messung](floraveg-namespace.md)).

3. **`esy_diagnostic_relevance` bleibt eine Datenlücke.** Das ESy-Regelwerk
   ist nicht ingestiert — siehe [Bekannte Lücken](../explanation/known-gaps.md).
   Ohne es kann hostus „nicht entscheidbar" nicht von „Habitat nicht erfüllt"
   trennen; die Feldform (immer present, sprechender Sentinel) sorgt lediglich
   dafür, dass niemand diese Lücke als negative Antwort missversteht.

## Fazit

Die baubare Hälfte von UC4 ist gebaut, gemessen und getestet; die unbaubare
Hälfte ist sichtbar und dokumentiert abwesend statt still fehlend. Das Verdikt
ist **hält mit Auflagen**: produktiv nutzbar als ESy-Namens- und
`unresolvable`-Signal, mit zwei bekannten Decken (Aggregat-Konzepte im
Backbone, ESy-Regelwerk als Datenbeschaffung) und einer bekannten
Verbesserungsrichtung (Disambiguierung).

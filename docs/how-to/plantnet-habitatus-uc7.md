# Von einer PlantNet-Bestimmung zu Euro+Med-Namen und Zeigerwerten (UC7)

Diese Anleitung beschreibt die Kette von einer Bild-Bestimmung durch
[Pl@ntNet](https://my.plantnet.org/) bis zu zwei Zielen, die hostus bedient:
einem **Euro+Med-PlantBase-Namen** (den z. B. Habitatus verlangt) und den
**pflanzenökologischen Zeigerwerten** (die seit v3.0 nicht mehr in hostus,
sondern in situs liegen).

Der Angelpunkt beider Wege ist derselbe: die **hostus-Konzept-ID**. Wer sie
einmal hat, bedient beide Ziele ohne weiteren Namensabgleich.

## Der Fluss

```
PlantNet /v2/identify  →  powo.id (IPNI)
  →  hostus POST /v1/match   (xref-Zeile)      →  concept_id + Euro+Med-Name + E+M-UUID
  →  situs GET /v1/species/{concept_id}/traits →  EIVE / Tichý / Midolo
```

## 1. Die POWO-ID aus der PlantNet-Antwort

PlantNet liefert pro Kandidat neben dem Namen zwei Fremd-IDs. Relevant ist
`powo.id`:

```json
{
  "preferedReferential": "k-world-flora",
  "results": [
    {
      "species": { "scientificNameWithoutAuthor": "Inula hirta" },
      "gbif": { "id": "10629881" },
      "powo": { "id": "77178414-1" }
    }
  ]
}
```

**Das Format passt ohne Umbau.** PlantNet gibt die bloße IPNI-ID
(`"77178414-1"`), und genau so liegt sie in hostus' `xref`-Tabelle — keine
LSID (`urn:lsid:ipni.org:names:…`), kein Präfix. `powo.id` kann unverändert
durchgereicht werden.

Der GBIF-Weg (`gbif.id`) ist bewusst **nicht** empfohlen, solange GBIF auf
Cybertaxonomy umstellt; die POWO-Achse ist die stabile.

## 2. hostus: IDs zu Konzepten und Euro+Med-Namen auflösen

Ein einziger Aufruf erledigt Auflösung **und** Übersetzung in den Zielraum —
auch für mehrere Pflanzen gleichzeitig (Batch-Limit 1000):

```bash
curl -X POST http://localhost:8080/v1/match \
  -H 'Content-Type: application/json' \
  -d '{
        "names": [
          {"id": "1", "xref": {"authority": "powo", "id": "77178414-1"}}
        ],
        "target_space": "eurosl"
      }'
```

```json
{
  "backbone_versions": {
    "cdm": "2026-08-02", "eurosl": "2024-11-03",
    "germansl": "1.5.6", "wcvp": "2026-06-15"
  },
  "results": [
    {
      "id": "1",
      "match_type": "xref",
      "confidence": 1,
      "concept_id": "wcvp:concept:3217689",
      "target_space_name": "Inula hirta",
      "target_space_ext_id": "a08253f0-411a-435e-9517-3a9b936efa3f",
      "target_space_status": "accepted",
      "esy_diagnostic_relevance": "not_determinable",
      "classification": {
        "family": "Compositae", "order": "Asterales", "class": "Magnoliopsida"
      }
    }
  ]
}
```

Was die Felder für einen Zielkonsumenten bedeuten:

- `match_type: "xref"` — über die Fremd-ID aufgelöst, nicht über den Namen.
  Confidence ist immer `1.0`: eine ID ist exakter als jeder Namensvergleich.
  Xref-Zeilen durchlaufen weder die Namens-Leiter noch den Fuzzy-Pfad.
- `target_space_name` — der Name im Zielraum. `eurosl` **ist** der
  Euro+Med-PlantBase-Snapshot (`AccordingTo api.cybertaxonomy.org/euromed`);
  das Herkunfts-Label `"euro+med"`, das ein Zieldienst erwartet, mappt der
  Aufrufer aus dem Namensraum `eurosl`. Einen Namensraum `euromed` gibt es
  nicht.
- `target_space_status` — `accepted` oder `synonym`, verbatim aus der Quelle.
  Wichtig, weil WCVP und Euro+Med sich widersprechen können: *Inula hirta*
  ist in WCVP ein Synonym (akzeptiert: *Pentanema hirtum*), in Euro+Med der
  akzeptierte Name. Wer den akzeptierten E+M-Namen braucht, prüft dieses Feld.
- `target_space_ext_id` — die **Euro+Med-TaxonUsage-UUID**. Der stabile
  Schlüssel für externe Ressourcen (EuroVeg.eu, CDM-Dienste), der über den
  Namen hinaus trägt.

Ohne `target_space` liefert derselbe Aufruf nur die `concept_id` — das
genügt für Schritt 3.

### Grenzfälle, mit denen ein Client rechnen muss

**Unbekannte ID ist ein Zeilen-Ergebnis, kein HTTP-Fehler.** Der Batch läuft
weiter; ein Client darf also nie vom Gesamtstatus auf die Einzelzeile
schließen:

```json
{
  "id": "weg",
  "match_type": "unresolvable",
  "confidence": 0,
  "requires_review": true,
  "note": "Fremd-ID unbekannt: kein Konzept mit dieser Authority/ID im Index"
}
```

**Nur akzeptierte IPNI-IDs sind abgedeckt.** Der Index trägt genau eine
POWO-ID pro Konzept — die des akzeptierten Namens (440.534 Xrefs auf 440.534
Konzepte). Liefert PlantNet ausnahmsweise die IPNI-ID eines *Synonym*-Namens,
läuft der Xref-Weg ins `unresolvable`. Fallback ist dann der Namensweg: im
**selben** Request eine zweite Zeile mit `verbatim` aus
`scientificNameWithoutAuthor` schicken und nehmen, was auflöst.

```json
{"names": [
  {"id": "1", "xref": {"authority": "powo", "id": "77178414-1"}},
  {"id": "1-fallback", "verbatim": "Inula hirta"}
]}
```

**Pro Zeile genau eines von `verbatim` oder `xref`.** Beides oder keines —
ebenso ein leeres `authority`/`id` — ist ein `400 INVALID_QUERY`, und die
Meldung nennt die betroffene Zeilen-ID:

```json
{"error": {"code": "INVALID_QUERY",
           "message": "xref requires both authority and id: row \"2\""}}
```

**Nicht jeder WCVP-Treffer hat eine Euro+Med-Entsprechung.** Euro+Med deckt
Europa und den Mittelmeerraum ab. Für ein außereuropäisches Taxon löst die
ID sauber auf, aber die `target_space_*`-Felder **fehlen ganz** (kein leerer
String, kein `null` — die Felder sind `omitempty`), z. B. für das
australische *Acacia arbiana* (`1007204-1`):

```json
{"id": "aus", "match_type": "xref", "confidence": 1,
 "concept_id": "wcvp:concept:2611278"}
```

Das ist kein Fehler, sondern der Scope der Quelle.

Für eine Einzelabfrage statt eines Batches tut es auch
`GET /v1/xref?authority=powo&id=77178414-1` — die Antwort trägt dieselbe
`concept_id`, aber keine Zielraum-Übersetzung.

## 3. situs: Zeigerwerte zur Konzept-ID

**Zeigerwerte gibt es in hostus nicht mehr.** Das Traits-Subsystem
(EIVE/Tichý/Midolo samt `GET /v1/concept/{id}/traits`) wurde entfernt und
nach [situs](https://github.com/jobrunner/situs) übertragen; in der
hostus-Datenbank existiert keine Trait-Tabelle mehr.

situs schlüsselt die Werte nach **derselben WCVP-Konzept-ID**, die
Schritt 2 geliefert hat — ein weiterer Namensabgleich entfällt:

```
GET /v1/species/wcvp:concept:3217689/traits
```

```
eive 1.0        L 6.56 · T 4.90 · M 3.08 · R 7.94 · N 3.21   (+ niche_width, n_systems)
midolo2023 3    disturbance_severity 0.536
```

**Abdeckung einplanen:** EIVE deckt 13.099, Tichý 8.612 und Midolo 6.278
Konzepte ab. Ein leeres Array ist damit ein normaler Ausgang, kein Fehler —
situs antwortet in diesem Fall bewusst mit `200` und leerem Array statt `404`.

## Warum FloraVeg hier nicht vorkommt

Naheliegende Fehlannahme: Zeigerwerte hingen am `floraveg`-Namensraum, weil
EIVE aus dem FloraVeg-Umfeld stammt. Das ist nicht so — die EIVE-Namen sind
bereits beim Ingest auf WCVP-Konzept-IDs aufgelöst worden, und die
Konzept-ID ist die gemeinsame Währung zwischen hostus und situs.

Der `floraveg`-Namensraum ist nomenklatorisch ohnehin weitgehend
deckungsgleich mit `eurosl`: FloraVeg.EU folgt für Gefäßpflanzen Euro+Med,
und 14.747 der 16.402 FloraVeg-Namen stehen wörtlich auch in `eurosl`
(der Rest sind außereuropäische Kultur- und Ziergehölze außerhalb des
Euro+Med-Scopes). Moose und Flechten, die das FloraVeg-Portal aus eigenen
Referenzlisten führt, sind im hostus-Namensraum **nicht** enthalten.

Gebraucht wird `floraveg` deshalb erst, wenn man in Richtung der Rohdaten
geht — ESy/EUNIS-Zuordnungen und EVA-Aufnahmen schlüsseln nach Namen in
FloraVeg-Schreibweise, nicht nach Konzept-ID.

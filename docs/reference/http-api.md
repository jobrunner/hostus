# HTTP-API

!!! note "Stand"
    Dies beschreibt den SP5-Stand: die lokale SQLite/FTS5-Rückgrat-Basis ist
    implementiert (`hostus ingest` befüllt die Datenbank aus WCVP/POWO-
    DwC-A-Manifesten, per Wikidata-Brücke angereicherten Cross-References
    sowie, seit SP5, der CDM-Konzeptquelle; `hostus serve` bedient
    `/v1/concept/{id}`, `/v1/xref`, `/v1/match`, `/v1/suggest`,
    `/v1/concept/{id}/traits` und `/v1/translate` daraus). Weitere
    `/v1/*`-Endpunkte (`concept/{id}/synonyms`) sowie `/openapi` folgen in
    späteren SPs. Die maßgebliche OpenAPI-Spezifikation liegt unter
    `api/openapi/openapi.yaml`.

    Der Offline-Export (`hostus bundle`) ist kein HTTP-Endpunkt und daher
    nicht Teil dieser Seite oder der OpenAPI-Spezifikation — siehe
    [Offline-Bundle exportieren](../how-to/offline-bundle.md).

## Health-Endpunkte

### `GET /health/live`

Liveness-Probe. Antwortet `200 OK`, solange der Prozess HTTP bedienen kann —
unabhängig vom Zustand nachgelagerter Abhängigkeiten.

### `GET /health/ready`

Readiness-Probe, an die lokale SQLite-Datenbank gekoppelt: Antwortet
`200 OK`, sobald mindestens ein Backbone erfolgreich per `hostus ingest`
eingelesen wurde. Antwortet `503 Service Unavailable`, solange keine
Datenbank konfiguriert ist, sie sich nicht öffnen lässt, oder die
Datenbank noch leer ist (kein Backbone eingelesen).

## Metrics-Endpunkt

### `GET /metrics`

Prometheus-Metriken im Text-Exposition-Format (`text/plain`). Siehe
[Observability](observability.md) für die Details der Middleware-Chain.

## Taxa-Endpunkte

Alle drei Endpunkte lesen ausschließlich aus dem lokalen SQLite/FTS5-Index,
den `hostus ingest` befüllt — kein Laufzeitzugriff auf GBIF oder andere
externe Dienste.

### `GET /v1/concept/{id}`

Löst eine `taxon_concept`-ID (Format `<backbone-id>:concept:<taxon-id>`,
z. B. `wcvp:concept:405825`) zum vollständigen Concept auf.

**Beispiel-Request**

```
GET /v1/concept/wcvp:concept:405825
```

**Beispiel-Response (`200 OK`)**

```json
{
  "concept_id": "wcvp:concept:405825",
  "display": "Corynephorus canescens (L.) P.Beauv.",
  "canonical": "Corynephorus canescens",
  "rank": "SPECIES",
  "status": "ACCEPTED",
  "backbone": { "id": "wcvp", "version": "2026-06-15" },
  "xrefs": { "powo": ["396681-1"], "inat": ["160927"] },
  "classification": [
    { "concept_id": "wcvp:concept:451295", "canonical": "Corynephorus", "rank": "GENUS" }
  ],
  "synonyms": [
    { "canonical": "Weingaertneria canescens var. pallida", "authorship": "Beckh." },
    { "canonical": "Corynephorus canescens f. pallidus", "authorship": "(Beckh.) Soó" },
    { "canonical": "Corynephorus canescens var. montana", "authorship": "Cout." },
    { "canonical": "Corynephorus canescens subsp. maritimus", "authorship": "(Godr.) Rivas Mart." }
  ],
  "distribution": [
    { "area_scheme": "wgsrpd_l3", "area_code": "AUT" },
    { "area_scheme": "wgsrpd_l3", "area_code": "BGM" },
    { "area_scheme": "wgsrpd_l3", "area_code": "BLR" },
    { "area_scheme": "wgsrpd_l3", "area_code": "BLT" },
    { "area_scheme": "wgsrpd_l3", "area_code": "BRC" },
    { "area_scheme": "wgsrpd_l3", "area_code": "CNT" },
    { "area_scheme": "wgsrpd_l3", "area_code": "CZE" },
    { "area_scheme": "wgsrpd_l3", "area_code": "DEN" },
    { "area_scheme": "wgsrpd_l3", "area_code": "RUC" }
  ]
}
```

`vernacular_de` (deutscher Trivialname) ist Teil der DTO, aber immer
leer/omitted — die Vernakular-Tabelle wird noch nicht ingestiert.

`classification` (Klassifikationskette) wird durch Verfolgen von
`taxon_concept.parent_id` nach oben ermittelt und ROOT-FIRST geliefert:
Index 0 ist die oberste erreichte Vorfahren-Ebene, das letzte Element das
direkte Elternteil des angefragten Concepts; das Concept selbst ist nie
Teil der Kette. Die Tiefe ist auf 10 Hops begrenzt, damit eine
zyklische/korrupte `parent_id`-Kette niemals hängen bleibt. `parent_id`
wird nur gesetzt, wenn das Eltern-Taxon selbst als akzeptiertes Concept
ingestiert wurde — andernfalls (und wenn die Kette nach der
Tiefenbegrenzung endet) ist `classification` leer/omitted.

`synonyms[].homotypic` ist `true`, wenn die Basionym-Verknüpfung ein
gemeinsames Basionym mit dem akzeptierten Namen beweist (Rekombination
davon, oder das Basionym selbst). Fehlt das Feld, ist die Typisierung
unbekannt — das Feld wird niemals als `false` ausgeliefert, da das eine
unbelegbare "heterotypisch"-Behauptung wäre.

`rank_verbatim` erscheint nur, wenn `rank` = `OTHER` ist — der Backbone
verwendet einen taxonomischen Rang, der keinem der kanonischen Ränge
entspricht (z. B. WCVPs `proles`, `lusus`, `nothosubsp.`; siehe
`docs/research/reality-check.md` für die volle gemessene Rang-Inventur).
In diesem Fall trägt `rank_verbatim` die ursprüngliche Quell-Schreibweise,
damit sie nicht verloren geht. Für jedes kanonisch eingestufte Concept
fehlt das Feld ganz (nie ein leerer String) — Abwesenheit bedeutet "nicht
zutreffend", nicht "unbekannt", dasselbe Muster wie bei `homotypic` und
`niche_width`.

`distribution` (Referenzgebiets-Zuordnungen, z. B. WGSRPD L3) wird von
`hostus ingest` befüllt und ausgeliefert; das Feld ist leer, wenn der
Backbone für dieses Concept keine Distribution liefert.

`xrefs` bildet jede Autorität auf ein **Array** ihrer externen IDs ab, nie
auf eine einzelne ID: SP4s Wikidata-Brücken-Ingest maß, dass ein Concept
legitim mehrere IDs derselben Autorität tragen kann (z. B. zwei
verschiedene Wikidata-Items, die über POWO/IPNI auf dasselbe Concept
auflösen, aber je eine eigene `inat`-Taxon-ID mitbringen — am vollen Index
gemessen: 954 Concepts mit >1 Wikidata-ID, 635 GBIF, 299 WFO, 63 iNat, 39
ColXR, 3 FloraVeg). Jedes Array ist deterministisch nach externer ID
sortiert — nie von der Ingest- oder Query-Reihenfolge abhängig. Die
bekannten Autoritäten sind `powo`, `wikidata`, `gbif`, `wfo`, `colxr`,
`inat`, `floraveg` und `euromed`; ein Concept ohne Cross-Reference zu einer
Autorität hat schlicht keinen Schlüssel dafür (nie ein leeres Array). Zum
`inat`-Schlüssel und seiner Reichweite siehe
[Von hostus zu iNaturalist (UC2)](../how-to/inat-uc2.md) — insbesondere die
dort gemessene 41,50-%-Obergrenze: nur gut zwei Fünftel aller Concepts
tragen überhaupt eine iNat-Verknüpfung.

Unbekannte IDs liefern `404 NOT_FOUND` im [Fehlerformat](#fehlerformat).

### `GET /v1/xref?authority={authority}&id={id}`

Löst eine externe Cross-Reference (z. B. eine POWO-ID oder eine
iNaturalist-Taxon-ID) auf und liefert dieselbe Concept-Repräsentation wie
`GET /v1/concept/{id}`. Alle in `xrefs` aufgeführten Autoritäten
(`powo`, `wikidata`, `gbif`, `wfo`, `colxr`, `inat`, `floraveg`, `euromed`)
werden reverse aufgelöst — nicht nur die ursprüngliche POWO-Verknüpfung.

**Beispiel-Request**

```
GET /v1/xref?authority=powo&id=396681-1
```

Die Response entspricht exakt der von `GET /v1/concept/wcvp:concept:405825`
oben.

Fehlende `authority`- oder `id`-Parameter liefern `400 INVALID_QUERY`,
eine unbekannte Kombination `404 NOT_FOUND`.

### `POST /v1/match`

Löst eine Liste verbatimer Namen batch-weise gegen den lokalen Index auf —
gedacht für den Import von Vegetationsaufnahmen mit uneinheitlicher
Namensschreibweise. Jeder Eintrag wird unabhängig klassifiziert; ein
`unresolvable`-Ergebnis ist ein normales Element der `200`-Antwort, kein
HTTP-Fehler. Nur ein nicht parsbarer Request-Body liefert `400
INVALID_QUERY`.

**Beispiel-Request**

```json
POST /v1/match
{
  "names": [
    { "id": "1", "verbatim": "Senecio jacobaea L." },
    { "id": "2", "verbatim": "Festuca ovina agg." },
    { "id": "3", "verbatim": "Silene otitis" }
  ]
}
```

**Beispiel-Response (`200 OK`)**

```json
{
  "backbone_versions": { "wcvp": "2026-06-15" },
  "results": [
    {
      "id": "1",
      "match_type": "exact_author",
      "confidence": 0.99,
      "concept_id": "wcvp:concept:3082777"
    },
    {
      "id": "2",
      "match_type": "aggregate_alias",
      "confidence": 0.95,
      "concept_id": "<aggregat-concept-id>",
      "note": "Aggregat, keine Kleinartauflösung"
    },
    {
      "id": "3",
      "match_type": "unresolvable",
      "confidence": 0,
      "requires_review": true,
      "note": "kein Treffer im Index"
    }
  ]
}
```

`match_type` ist eines von `exact`, `exact_author`, `aggregate_alias` oder
`unresolvable`. `candidates` (Liste von Kanonicalnamen) wird nur bei
Autor-Mehrdeutigkeit gefüllt. `target_space`/`sec_hint` im Request werden
entgegengenommen, aber nicht ausgewertet — die Sekundärraum-Übersetzung
liegt in [`POST /v1/translate`](#post-v1translate).

### `GET /v1/suggest?q={q}&area={area}&rank={rank}&limit={limit}`

Autosuggest-Endpunkt für ein Frontend-Eingabefeld: ein FTS5-Präfix-Treffer
über den lokalen Index, optional nach Referenzgebiet und Rang gefiltert,
priorisiert und auf `limit` gekürzt. `q` ist erforderlich; fehlt oder ist es
leer (auch nur Leerzeichen), liefert der Endpunkt `400 INVALID_QUERY`.

- `area` (optional): WGSRPD-L3-Referenzgebietscode (z. B. `AUT`) oder eine
  dokumentierte Kurzform (z. B. `DE`). Leer bedeutet kein Gebietsfilter —
  `in_area` ist dann bei jedem Ergebnis `false`.
- `rank` (optional): kommagetrennte Liste von Rängen, z. B.
  `species,subspecies`. Ein unbekannter Rang-Token liefert `400
  INVALID_QUERY`.
- `limit` (optional): maximale Ergebnisanzahl. Nicht-numerische Werte
  liefern `400 INVALID_QUERY`; ein leerer oder `<= 0` Wert verwendet den
  serverseitigen Standardwert.

Die Priorisierung folgt §B.1: Präfix-Treffer vor Nicht-Treffer, im
angefragten Gebiet vor nicht im Gebiet, akzeptiert vor Synonym, breitere vor
feineren Rängen (FAMILY/GENUS vor SPECIES vor SUBSPECIES/VARIETY/FORM),
zuletzt bm25-Score aufsteigend (niedriger ist relevanter).

**Beispiel-Request**

```
GET /v1/suggest?q=coryn&area=AUT
```

**Beispiel-Response (`200 OK`)**

```json
{
  "backbone_versions": { "wcvp": "2026-06-15" },
  "results": [
    {
      "concept_id": "wcvp:concept:405825",
      "display": "Corynephorus canescens (L.) P.Beauv.",
      "canonical": "Corynephorus canescens",
      "rank": "SPECIES",
      "status": "ACCEPTED",
      "in_area": true,
      "score": -2.31
    }
  ]
}
```

`vernacular_de` ist Teil der DTO, wird aber nur ausgeliefert, wenn ein
deutscher Trivialname für das Concept ingestiert wurde (`omitempty`).

## Trait-Endpunkt

### `GET /v1/concept/{id}/traits?vocab={vocab}`

Liefert alle für ein Concept ingestierten ökologischen Merkmalswerte
(EIVE, Tichý et al. 2023, Midolo et al. 2023), gruppiert **pro Vokabular**
— nie über Vokabulare hinweg zusammengeführt, da deren Taxonomie-
Namensräume (`taxonomy`) nachweislich divergieren (z. B. `euromed-aligned`
für EIVE vs. `floraveg-eunis-aligned` für Tichý).

- `vocab` (optional): kommagetrennte Liste von Vokabular-Token (`eive`,
  `tichy2023`, `midolo2023`). Leer bedeutet alle Vokabulare, für die
  dieses Concept Werte hat. Ein unbekanntes Token liefert `400
  INVALID_QUERY`.

**Beispiel-Request**

```
GET /v1/concept/wcvp:concept:405825/traits?vocab=eive,tichy2023
```

**Beispiel-Response (`200 OK`)**

```json
{
  "concept_id": "wcvp:concept:405825",
  "traits": [
    {
      "vocab": "eive",
      "vocab_version": "1.0",
      "taxonomy": "euromed-aligned",
      "values": [
        {
          "dim": "M",
          "value": 2.49,
          "niche_width": 3.43,
          "n_systems": 20,
          "scale": { "min": 0, "max": 10, "normalized": true }
        }
      ]
    },
    {
      "vocab": "tichy2023",
      "vocab_version": "2.0",
      "taxonomy": "floraveg-eunis-aligned",
      "values": [
        {
          "dim": "L",
          "value": 8.4,
          "scale": { "min": 1, "max": 9, "normalized": false }
        },
        {
          "dim": "T",
          "value": 6.3,
          "resolution": "aggregate_to_nominate",
          "scale": { "min": 1, "max": 12, "normalized": false }
        }
      ]
    }
  ]
}
```

Wichtige Punkte für Clients:

- **`scale` wird pro Wert gerendert, nicht pro Set.** Selbst innerhalb
  eines Vokabulars unterscheiden sich die Skalen zwischen Dimensionen —
  Tichý misst `T` auf 1–12, `L` aber auf 1–9. Ein einzelnes Set-weites
  `scale`-Feld wäre für mindestens eine Dimension falsch, deshalb trägt
  jeder Eintrag in `values` sein eigenes `scale`. `normalized: true`
  gibt es ausschließlich bei EIVE (uniform 0–10); alle anderen
  Kombinationen sind `normalized: false`. Ein `{"min": 0, "max": 0,
  "normalized": false}`-Ergebnis ist ein Sentinel für "keine feste Skala
  definiert" (z. B. Midolo-Störungsindikatoren), nicht "der Wert ist
  exakt 0". **Ein EIVE-Wert von 4.2 ist daher niemals direkt mit einem
  Tichý-Wert von 4.2 vergleichbar** — genau deshalb liefert jeder Wert
  seine Skala mit.
- **`niche_width`/`n_systems` fehlen ganz (nicht `0`), wenn das
  Vokabular sie nicht liefert.** EIVE liefert beide für jeden Wert;
  Tichý und Midolo liefern keines von beiden. Ein fehlendes Feld
  bedeutet "dieses Vokabular kennt dieses Datum nicht", ein `0`-Wert
  hätte fälschlich eine tatsächliche Nullbreite/-quellenzahl behauptet.
- **`resolution` fehlt ganz, wenn der Taxonname des Vokabulars exakt auf
  das Concept passte** — der Normalfall. Ist das Feld gesetzt, wurde der
  Name über eine deterministische Normalisierungsregel aufgelöst
  (`hybrid_spacing`, `hybrid_marker_dropped`, `hybrid_marker_added`,
  `aggregate`, `aggregate_to_nominate`, `autonym`,
  `orthography_genitive`). Zwei dieser Werte sind für Clients
  entscheidend, weil sie zwei **nicht identische** Umgrenzungen
  gleichsetzen:

    - `aggregate_to_nominate` — eine Sammelart (`Acer opalus aggr.`) ist
      WEITER als ihre Nominatart und umfasst weitere Kleinarten. Der
      Wert ist also ein Kollektivmittel, das das Vokabular nie über
      diese eine Art ausgesagt hat.
    - `autonym` — ein Autonym (`Acer obtusatum subsp. obtusatum`) ist
      ENGER als seine Art; es landet hier nur, weil das Rückgrat die
      infraspezifische Gliederung überhaupt nicht führt.

    Wer diese Näherung nicht akzeptieren kann, filtert auf genau diese
    beiden Werte. Die übrigen Regeln korrigieren ausschließlich die
    Schreibweise (Hybridmarker, `-ii`/`-i`-Genitiv) und lassen die
    Umgrenzung unberührt. Hintergrund und gemessene Wirkung:
    `docs/research/reality-check.md`, Abschnitt „Nach Hardening
    (Task 5)".
- **`taxonomy` fehlt ganz, wenn keine Vokabular-Metadatenzeile
  zugeordnet werden konnte** — wird nicht als leerer String
  vorgetäuscht.
- Ein unbekanntes Concept liefert `404 NOT_FOUND`. Ein **bekanntes**
  Concept ohne ingestierte Merkmalswerte liefert `200 OK` mit leerem
  `traits`-Array — das ist kein Fehlerfall.

## Übersetzungs-Endpunkt

### `POST /v1/translate`

Übersetzt ein Konzept aus seinem `sec.`-Referenzraum in einen anderen (UC6):
„Dieses Konzept *sec.* Rothmaler — was ist es *sec.* Wisskirchen & Haeupler
1998, und **wie genau** hängen die beiden zusammen?"

Zwei Konzepte mit gleichem Namen und verschiedenem `sec.` sind in hostus
absichtlich **getrennte Zeilen**. Dieser Endpunkt liefert die Verbindung
zwischen ihnen — und zwar nur dann, wenn eine Quelle sie tatsächlich
behauptet.

!!! warning "Lizenz — nicht öffentlich ausliefern"
    Die Konzeptrelationen stammen aus der CDM-Ernte
    (`rl_standardliste`, BGBM/EDIT). Für diese Daten ist **keine Lizenz
    auffindbar** (Portal, API und Payloads schweigen), sie sind aus
    urheberrechtlich geschützter Floren-Literatur abgeleitet, und das
    Manifest führt sie als `redistribution: unknown`. `/v1/translate`
    darf auf dieser Datenbasis **nicht öffentlich** betrieben werden,
    solange keine schriftliche Freigabe von BGBM/EDIT vorliegt — nur
    lokale Auswertung. Der Bundle-Export verweigert CDM-Daten
    entsprechend ohne `--force-include-restricted`.

**Request**

Genau eines von `concept_id` und `verbatim` muss gesetzt sein, dazu
`target_space` (die Id eines `sec.`-Referenzraums).

```json
POST /v1/translate
{
  "concept_id": "cdm:concept:b7a352aa-1f73-41f3-a4e3-b24fc1c2cd5f",
  "target_space": "060afae5-76ef-44a7-921f-1202685ef351"
}
```

| Feld | Pflicht | Bedeutung |
|---|---|---|
| `concept_id` | alternativ | hostus-Konzept-Id des Ausgangskonzepts |
| `verbatim` | alternativ | Name, der zuerst über die Auflösung von `/v1/match` aufgelöst wird |
| `target_space` | ja | Id des Ziel-`sec.`-Referenzraums |
| `max_hops` | nein | muss `1` sein (Default). Alles andere → `400 INVALID_QUERY` |
| `include_name_candidates` | nein | schaltet den ausdrücklich **nicht**-relationalen Namensblock frei (Default `false`) |

**Beispiel-Response (`200 OK`, congruent)**

```json
{
  "source": {
    "concept_id": "cdm:concept:b7a352aa-1f73-41f3-a4e3-b24fc1c2cd5f",
    "canonical": "Abies alba",
    "authorship": "Mill.",
    "rank": "SPECIES",
    "sec": { "id": "87ed3300-…", "title": "Schubert & Vent (eds.) 1990: Exkursionsflora … Rothmaler, 8. Aufl." }
  },
  "target_space": { "id": "060afae5-…", "title": "Wisskirchen & Haeupler 1998: Standardliste …" },
  "entry": { "mode": "concept_id" },
  "max_hops": 1,
  "result": "translated",
  "candidates": [
    {
      "concept_id": "cdm:concept:872088a4-95f4-472c-ae79-a29028bb3fbf",
      "canonical": "Abies alba",
      "authorship": "Mill.",
      "rank": "SPECIES",
      "status": "ACCEPTED",
      "sec": { "id": "060afae5-…", "title": "Wisskirchen & Haeupler 1998: Standardliste …" },
      "relation": "congruent",
      "relation_from_source": "congruent",
      "direction": "source_to_target",
      "statement": {
        "from": "cdm:concept:b7a352aa-…",
        "relation": "congruent",
        "to": "cdm:concept:872088a4-…"
      },
      "is_equality": true,
      "hops": 1,
      "source": "cdm"
    }
  ],
  "requires_review": false,
  "backbone_versions": { "wcvp": "2026-06-15", "cdm": "2026-08-02" }
}
```

#### Nur `congruent` ist eine Gleichsetzung

`is_equality` ist das **einzige** Feld, das als „dasselbe Taxon" gelesen
werden darf. Es steht auf jedem Kandidaten und ist bei genau einer Relation
`true`:

| `relation` | Symbol | `is_equality` | Bedeutung |
|---|---|---|---|
| `congruent` | ≜ | **`true`** | gleiche Umgrenzung |
| `not_congruent` | — | `false` | ausdrücklich **nicht** gleich |
| `includes` | ⊃ | `false` | Quelle ist die **weitere** Umgrenzung |
| `included_in` | ⊂ | `false` | Quelle ist die **engere** Umgrenzung |
| `overlaps` | ⊕ | `false` | teilweise Überschneidung, keine Gleichsetzung |
| `includes_or_included_in_or_overlaps` | ⊂⊃⊕ | `false` | Quelle **legt sich nicht fest**, welche der drei gilt |
| `pro_parte` | p.p. | `false` | gerichtete Teilaussage über den Namen der From-Seite |

`includes_or_included_in_or_overlaps` wird **nie** auf `overlaps`
eingeebnet: die Quelle sagt ausdrücklich, dass sie sich nicht festlegt, und
eine Einebnung würde eine unsichere Aussage still zu einer definiten
aufwerten. Jeder Kandidat mit `is_equality: false` trägt zusätzlich ein
deutschsprachiges `note`-Feld, das das in Prosa sagt.

`misapplied` kommt hier nie vor: CDM flaggt diese Zeilen
`conceptRelationship: false`, weil sie eine Aussage über **Namensverwendung**
und nicht über Umgrenzungen sind — der Ingest verwirft sie (gezählt und
bemustert), statt sie unter derselben Spalte mitzuführen.

#### Richtung: `A includes B` ist nicht `B included_in A`

hostus speichert eine Relation **in der Richtung, in der die Quelle sie
nennt**, und legt keine gespiegelte Zeile an. Die Antwort macht das sichtbar:

- `statement` ist die gespeicherte Aussage, wortwörtlich — geordnetes Paar
  plus Relation.
- `direction` ist `source_to_target`, wenn das Ausgangskonzept die
  From-Seite ist, sonst `target_to_source`.
- `relation_from_source` ist dieselbe Kante quellenseitig gelesen (bei einer
  eingehenden `includes`-Kante also `included_in`).
- Bei `pro_parte` **fehlt `relation_from_source` ganz**, wenn die Kante
  eingehend ist: eine gerichtete Aussage über den Namen der Gegenseite hat
  keine sinnvolle Umkehrung, und hostus erfindet keine. Das `note`-Feld sagt
  das.

Ein Konzeptpaar kann zwei **verschiedene** Relationstypen tragen (der
Primärschlüssel von `concept_relation` ist
`(from_concept, to_concept, relation, source)`). Beide erscheinen als eigene
Kandidaten; sie werden nicht zusammengefasst.

#### Genau ein Hop

`/v1/translate` folgt **genau einer** Relationskante. Eine transitive Kette
ist über das gemessene Vokabular nicht allgemein gültig: `congruent ∘
includes` wäre vertretbar, `overlaps ∘ overlaps` sagt gar nichts (zwei
Umgrenzungen, die beide eine dritte überlappen, können disjunkt sein), und
`⊂⊃⊕ ∘ irgendwas` ist per Konstruktion undefiniert. Deshalb komponiert
hostus nicht. `max_hops` steht auf jeder Antwort; ein Request mit
`max_hops != 1` wird mit `400 INVALID_QUERY` **benannt abgelehnt**, statt
still eine Ein-Hop-Antwort zu liefern.

#### Keine Relation ist eine Antwort, kein Fehler

```json
{
  "result": "no_relation_recorded",
  "candidates": [],
  "note": "Keine erfasste Relation in den Zielreferenzraum. Das bedeutet NICHT, dass keine Beziehung besteht — nur, dass keine Quelle eine erfasst hat.",
  "…": "…"
}
```

`candidates` wird in diesem Fall als **leeres Array** ausgeliefert, nicht
weggelassen, und `result` sagt den Ausgang ausdrücklich. Ein
Namenstreffer wird **nie** ersatzweise als Übersetzung präsentiert — genau
diese Verwechslung soll UC6 verhindern.

Wer trotzdem einen Anhaltspunkt braucht, setzt
`include_name_candidates: true`. Namensgleiche Konzepte des Zielraums
erscheinen dann unter dem eigenen Schlüssel `unrelated_name_candidates`,
jeder Eintrag mit `requires_review: true`, ohne jedes Relationsfeld, und die
Antwort als Ganzes trägt `requires_review: true`. Sobald eine echte Relation
existiert, entfällt der Block.

#### Einstieg über einen Namen

Mit `verbatim` statt `concept_id` läuft die Auflösung durch dieselbe Logik
wie `POST /v1/match`. `entry` protokolliert das Ergebnis
(`mode`, `verbatim`, `match_type`, `confidence`, `note`). Ein
**Fuzzy-Treffer setzt `requires_review: true`** auf der gesamten Antwort —
ausnahmslos. Ein Name, der sich nicht auf **genau ein** Konzept auflösen
lässt (kein Treffer oder mehrdeutig über mehrere `sec.`-Räume), liefert
`422 UNRESOLVABLE`.

#### Fehlerfälle

| Fall | Status | Code |
|---|---|---|
| Body nicht parsbar; keins oder beides von `concept_id`/`verbatim`; `target_space` fehlt; `max_hops != 1` | `400` | `INVALID_QUERY` |
| Unbekannte `concept_id` **oder** unbekannter `target_space` | `404` | `NOT_FOUND` |
| `verbatim` nicht auf genau ein Konzept auflösbar | `422` | `UNRESOLVABLE` |

Ein unbekannter `target_space` ist ausdrücklich ein `404` und keine leere
Antwort: ein Tippfehler im Zielraum darf nicht wie „keine Relation erfasst"
aussehen.

## Fehlerformat

Alle Fach-Endpunkte liefern Fehler einheitlich als JSON:

```json
{
  "error": {
    "code": "ERROR_CODE",
    "message": "Human readable message"
  }
}
```

| Code                  | HTTP | Beschreibung                                          |
|-----------------------|------|--------------------------------------------------------|
| `INVALID_QUERY`       | 400  | Ungültiger Query-Parameter oder Request-Body            |
| `RATE_LIMIT_EXCEEDED` | 429  | Rate-Limit überschritten                                |
| `UPSTREAM_OVERLOADED` | 503  | Load-Shedding aktiv                                     |
| `NOT_FOUND`           | 404  | Unbekannte Concept-/Xref-ID                             |
| `UNRESOLVABLE`        | 422  | `POST /v1/translate`: `verbatim` lässt sich nicht auf genau ein Konzept auflösen. Bei `POST /v1/match` **kein** HTTP-Fehler — dort ist eine nicht auflösbare Anfrage ein normales `200`-Ergebnis mit `match_type: "unresolvable"`, siehe oben |
| `GBIF_TIMEOUT`        | 504  | GBIF-Anfrage Timeout (nur Ingest-/Enrichment-Pfad)      |
| `GBIF_UNAVAILABLE`    | 502  | GBIF nicht erreichbar (nur Ingest-/Enrichment-Pfad)     |
| `INTERNAL_ERROR`      | 500  | Interner Serverfehler                                   |

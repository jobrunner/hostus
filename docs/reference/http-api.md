# HTTP-API

!!! note "Stand"
    Dies beschreibt den SP5-Stand: die lokale SQLite/FTS5-Rückgrat-Basis ist
    implementiert (`hostus ingest` befüllt die Datenbank aus WCVP/POWO-
    DwC-A-Manifesten, per Wikidata-Brücke angereicherten Cross-References
    sowie, seit SP5, der CDM-Konzeptquelle; `hostus serve` bedient
    `/v1/concept/{id}`, `/v1/xref`, `/v1/match`, `/v1/suggest`,
    `/v1/concept/{id}/traits`, `/v1/concept/{id}/synonyms` und
    `/v1/translate` daraus). `/openapi` folgt in einem späteren SP. Die
    maßgebliche OpenAPI-Spezifikation liegt unter
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

`sec` `{id, title}` (SP5) nennt den `sec.`-Referenzraum des Concepts — nur
für ein sec-tragendes (CDM-)Concept present, weggelassen für ein
WCVP-Concept. So sind zwei gleichnamige Konzepte (eines je Referenzwerk)
unterscheidbar.

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
HTTP-Fehler. Ein nicht parsbarer Request-Body **oder** ein unbekannter
`target_space` liefert `400 INVALID_QUERY` (die Meldung nennt den
unbekannten Raum).

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
Autor-Mehrdeutigkeit gefüllt.

#### `entry_backbone` / `entry_sec` (SP5): Auflösungs-Filter

Im Multi-Backbone-Index (WCVP + CDMs ~119 `sec.`-Räumen) liegt derselbe Name
oft mehrfach — `MatchExact` sucht über alle Backbones, also bleibt ein
Allerweltsname mehrdeutig (`unresolvable`). Zwei optionale, komponierbare
Request-Felder (top-level, für den ganzen Batch) beschränken die Auflösung:

- `entry_backbone` — eine Backbone-id (`wcvp`|`cdm`|`colxr`). `entry_backbone=wcvp`
  löst gemessen 12.979 bisher mehrdeutige Namen eindeutig auf ihr WCVP-Konzept
  auf — der Kernfall für `target_space` (kombinierbar).
- `entry_sec` — eine `sec_reference`-id (impliziert CDM). Löst gemessen
  **99,67 %** der (Name, Raum)-Kombis eindeutig auf.

Beide verknüpfen mit UND. Ohne Filter ist die Antwort **byteweise** die
gewohnte Form. Ein unbekannter Wert ist `400 INVALID_QUERY` und nennt ihn.
Messung: [`docs/research/sp5-sec-filter.md`](../research/sp5-sec-filter.md).

#### `target_space` (SP9/UC4): ESy-kompatibler Name und `aggregate_policy`

Mit dem optionalen `target_space` (aktuell nur `floraveg`) wird jeder Treffer
zusätzlich in den genannten Namensraum aufgelöst. **Ohne `target_space` ist
die Antwort byteweise die oben gezeigte Form** — die drei folgenden Felder
fehlen dann vollständig, damit UC3/UC6, die denselben Endpunkt nutzen, keine
Formänderung sehen.

```json
POST /v1/match
{ "target_space": "floraveg",
  "names": [ { "id": "1", "verbatim": "Festuca ovina agg." } ] }
```

```json
{
  "backbone_versions": { "wcvp": "2026-06-15" },
  "results": [
    {
      "id": "1",
      "match_type": "aggregate_alias",
      "confidence": 0.95,
      "concept_id": "<aggregat-concept-id>",
      "note": "Aggregat, keine Kleinartauflösung",
      "target_space_name": "Festuca ovina aggr.",
      "aggregate_policy": "known",
      "esy_diagnostic_relevance": "not_determinable"
    }
  ]
}
```

- `target_space_name` — die ESy-kompatible Schreibweise, die der Zielraum für
  das aufgelöste Concept führt. Fehlt, wenn der Zielraum keine passende
  Schreibweise hat, **insbesondere bei `aggregate_policy: unresolvable`**: dort
  wird bewusst kein Name geliefert, weil die Kleinart als Aggregatnamen
  anzubieten genau die falsche „nicht erfüllt"-Antwort wäre.
- `aggregate_policy` — dreiwertig:
  - `known` — der Zielraum führt das Aggregat als eigenes Taxon (Beispiel oben:
    `Festuca ovina aggr.`).
  - `unresolvable` — die Anfrage **ist** ein Aggregat, der Zielraum kennt
    darunter aber nur Kleinarten. Das heißt **„nicht entscheidbar", nicht
    „nicht erfüllt"**, und die Deckung darf **nicht** auf die Kleinarten
    verteilt werden.
  - **fehlt** (dritter Zustand) — gar kein Aggregat im Spiel (gewöhnliche Art).
    Ein `known` für jede Art würde das Feld bedeutungslos machen.
- `esy_diagnostic_relevance` — bei gesetztem `target_space` **immer present**
  und **immer** `not_determinable`. hostus kann die ESy-diagnostische Relevanz
  derzeit nicht bestimmen, weil das ESy-Regelwerk nicht ingestiert ist (siehe
  [known-gaps](../explanation/known-gaps.md)). Der Wert ist absichtlich ein
  selbsterklärender String und niemals `null` oder fehlend: seine Abwesenheit
  oder ein falsy-Wert dürfte **nie** als „nicht relevant" gelesen werden —
  genau dieser Fehlschluss ist der von UC4 gefürchtete False Negative.

### `GET /v1/suggest?q={q}&area={area}&rank={rank}&limit={limit}`

Autosuggest-Endpunkt für ein Frontend-Eingabefeld: ein FTS5-Präfix-Treffer
über den lokalen Index, optional nach Referenzgebiet und Rang gefiltert,
priorisiert und auf `limit` gekürzt. `q` ist erforderlich; fehlt oder ist es
leer (auch nur Leerzeichen), liefert der Endpunkt `400 INVALID_QUERY`.

- `area` (optional): WGSRPD-L3-Referenzgebietscode (z. B. `AUT`) oder eine
  dokumentierte Kurzform (z. B. `DE`). Leer bedeutet kein Gebietsfilter —
  `in_area` ist dann bei jedem Ergebnis `false`.

`in_area` ist ein **positiver** Verbreitungsbeleg, kein Ja/Nein: `true`, wenn
das Concept selbst im Gebiet verbreitet ist ODER — bei Concepts ohne eigene
Distribution (die CDM-`sec.`-Concepts) — derselbe akzeptierte Name bei WCVP
(akzeptiert oder als Synonym) im Gebiet vorkommt. `false` bedeutet **nicht**
„kommt dort nicht vor", sondern nur „kein positiver Beleg" — Distribution ist
Präsenz-Daten, ein fehlender Eintrag ist keine belegte Abwesenheit. Die
Testkonsole zeigt `false` deshalb als „keine Angabe", nie als „nein".
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

Jeder Treffer trägt `sec` `{id, title}` (SP5), sofern er zu einem
sec-tragenden (CDM-)Concept gehört — das unterscheidet gleichnamige
CDM-Treffer, die sonst bis zum Score identisch sind. Für WCVP-Treffer fehlt
das Feld.

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

`aggregate` ist `true`, wenn das Concept über eine Aggregat-Namensraum-
Schreibweise (z. B. „Achillea millefolium aggr.") getroffen wurde — der
FTS-Query streift den Aggregat-Marker ab (`agg./aggr./s.l.` sind gleichwertig),
und aufgelöste Aggregat-Schreibweisen sind als Aliase indexiert. Bei einem
gewöhnlichen Treffer fehlt das Feld (`omitempty`). Da ein FloraVeg-Aggregat auf
die Nominatart zeigt, ist der Treffer die Nominatart mit gesetztem `aggregate`,
kein separater Aggregat-Eintrag.

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

## Synonym-Endpunkt

### `GET /v1/concept/{id}/synonyms?relevance={relevance}&rank={rank}&max={max}`

Liefert die Synonyme eines Concepts — auf Wunsch reduziert auf die, die in
eine Publikation gehören. UC5 formuliert das Problem so: **„Das Problem ist
Filterung, nicht Beschaffung."** POWO führt für *Corynephorus canescens* 26
Synonyme; eine Publikation braucht ein bis drei. Im gemessenen Index trägt
ein Concept im Mittel **4,09** Synonyme, im Maximum **1.127**.

#### Der Standard ist die ungefilterte Liste

`relevance` ist ohne Parameter `all`. Das ist eine bewusste Entscheidung,
keine Bequemlichkeit: der Publikationsfilter hält bei *Corynephorus
canescens* 20 von 26 Synonymen zurück, und ein Filter dieser Stärke muss
angefordert werden.

1. Dieser Endpunkt darf nicht die einzige Tür werden, die Daten
   stillschweigend verbirgt. Die ungefilterte Liste muss erreichbar sein —
   und „erreichbar" ist als Standard mehr wert als als Opt-out, das man
   kennen muss.
2. `GET /v1/concept/{id}` liefert für dasselbe Concept dieselben Synonyme
   ungefiltert. Zwei Endpunkte, die auf dieselbe Frage unterschiedlich viele
   Zeilen liefern, und einer davon schweigend, lesen sich als Fehler in
   demjenigen, den der Client zufällig als zweiten probiert hat.
3. Die Publikationsregeln fußen auf einem `nom_status`-Vokabular, das auf
   6,85 % der Namen gefüllt ist und einen Schwanz von 1.225 Werten mit
   weniger als je 10 Treffern hat. Dieses — hostus' unsicherstes — Urteil
   gehört nicht per Default auf den meistbenutzten Pfad.

Beide Modi liefern **dieselbe Begründung pro Synonym** und **dieselbe
Ausschluss-Bilanz**. `relevance` entscheidet nur, ob die zurückgehaltenen
Einträge in der Liste auftauchen — nicht, wie gründlich geurteilt wurde.

#### Parameter

- `relevance` (optional): `all` (Standard) oder `publication`. Jeder andere
  Wert liefert `400 INVALID_QUERY` und **nennt den Wert**.
- `rank` (optional): die Rangstufe, auf der publiziert wird. `species`
  schließt **genau die vier von UC5 genannten** untergeordneten Ränge aus:
  `VARIETY`, `SUBVARIETY`, `FORM`, `SUBFORM`. Die Nothotaxon-Ränge
  (`NOTHOSUBSPECIES`, `NOTHOVARIETY`, `NOTHOFORM`) sind **nicht** darunter
  und passieren den Filter, obwohl sie unterhalb der Art stehen — im
  gemessenen Index betrifft das 190 Synonymzeilen (130/51/9). UC5 nennt sie
  nicht, und hostus erfindet keine Regel, die der Use Case nicht verlangt
  hat. Ebenso wenig ausgeschlossen wird `OTHER` (6.409 Zeilen), dessen
  ursprüngliche Schreibweise deshalb als `rank_verbatim` mitgeliefert wird.
  Fehlt der Parameter, wird **kein** Rang ausgeschlossen — der Fall
  einer vollständigen infraspezifischen Behandlung. Ein syntaktisch gültiger, aber nicht unterstützter Rang
  (`genus`) wird **abgelehnt**, nicht still ignoriert: eine
  unbeabsichtigt ungefilterte Antwort an einen Aufrufer, der einen Filter
  angefordert hat, wäre die gefährlichere Variante.
- `max` (optional): Obergrenze der zurückgegebenen Liste. `0` und ein
  fehlender Parameter bedeuten beide **keine Kappung** (nicht „null
  Zeilen"). Gekappt wird **immer nach dem Ranking** — `max=3` liefert die
  drei besten Synonyme, nie drei beliebige. Werte außerhalb `[0, 2000]`
  liefern `400`, bevor irgendetwas alloziert wird. Die Obergrenze 2000
  liegt über dem gemessenen Maximum von 1.127 Synonymen pro Concept, so
  dass „alle" durch `max` ausdrückbar bleibt.

#### Rangfolge

Die Antwort nennt die Sortierregel im Feld `ordering` selbst, damit „die
besten drei" nachprüfbar ist. Sie lautet, in dieser Reihenfolge:

1. publikationsfähige zuerst;
2. `homotypic` vor `unknown` vor `heterotypic`;
3. das **Basionym** führt seinen Typisierungsblock an (UC5-Regel 4);
4. `name_id` als letzter Tiebreaker (deterministisch).

Jedes darin genannte Feld wird pro Eintrag mitgeliefert.

#### Beispiel-Request

```
GET /v1/concept/wcvp:concept:405825/synonyms?relevance=publication&rank=species&max=3
```

**Beispiel-Response** (gegen den realen WCVP-Index; Eintrag 3,
*Weingaertneria canescens*, ist der Kürze halber weggelassen — er ist bis
auf Name und Position identisch mit Eintrag 2):

```json
{
  "concept_id": "wcvp:concept:405825",
  "relevance": "publication",
  "publication_rank": "species",
  "ordering": "publishable first, then homotypic before unknown before heterotypic, the basionym first within its typification block, then name_id",
  "synonyms": [
    {
      "position": 1,
      "name_id": "wcvp:name:476481",
      "canonical": "Aira canescens",
      "authorship": "L.",
      "rank": "SPECIES",
      "typification": "homotypic",
      "is_basionym": true,
      "nom_status_judgement": "absent",
      "publishable": true,
      "reason": "homotypic, no nom_status recorded (not the same as verified clean)"
    },
    {
      "position": 2,
      "name_id": "wcvp:name:397417",
      "canonical": "Avena canescens",
      "authorship": "(L.) Weber",
      "rank": "SPECIES",
      "typification": "homotypic",
      "is_basionym": false,
      "nom_status_judgement": "absent",
      "publishable": true,
      "reason": "homotypic, no nom_status recorded (not the same as verified clean)"
    }
  ],
  "summary": {
    "total": 26,
    "publishable": 6,
    "returned": 3,
    "truncated": 3,
    "absent": 6,
    "excluded": { "nom_status": 4, "rank": 16 },
    "unclassified_statuses": []
  }
}
```

*Corynephorus incanescens* Bubani (`wcvp:name:405842`, `", nom. illeg.
superfl."`) ist einer der vier `nom_status`-Ausschlüsse; *Aira canescens*
L. führt als Basionym.

#### Jeder Ausschluss ist sichtbar

`summary` beschreibt **immer das Concept**, nie die ausgelieferte Seite:
`total`, `publishable`, `absent`, `excluded` und `unclassified_statuses`
zählen alle 26 Synonyme, auch wenn die Antwort drei enthält. Ein Filter,
der 20 von 26 Synonymen entfernt, ohne das zu sagen, ist von einer kaputten
Abfrage nicht zu unterscheiden — dieselbe Disziplin, mit der `hostus
ingest` `matched/unmatched/ambiguous` ausgibt und `trait_value.resolution`
Näherungen protokolliert.

- `excluded` zählt je Regel: `nom_status` (ein erfasster nomenklatorischer
  Mangel), `unclassified_nom_status` (die Quelle hat etwas erfasst, keine
  Regel deckt es — **zurückgehalten**, nicht publiziert) und `rank`.
- `unclassified_statuses` listet die betroffenen Rohwerte wörtlich. Aus
  dieser Liste wächst die Regeltabelle; ein zurückgehaltener Wert bleibt
  sichtbar statt zu verschwinden.
- `absent` zählt, wie viele der publikationsfähigen Synonyme auf einem
  **leeren** `nom_status` beruhen. „Nichts erfasst" ist nicht „als sauber
  erfasst" — im Beispiel oben ruhen alle sechs auf einer Abwesenheit.
- `truncated` steht bewusst **außerhalb** von `excluded`: ein gekapptes
  Synonym wurde nicht als irrelevant beurteilt, es hat nur nicht mehr
  hineingepasst.

#### Doppelt ausgeschlossene Synonyme zählen nur einmal

Ein Synonym wird mit **genau einem** Grund gezählt, dem zuerst greifenden:
`nom_status` vor `unclassified_nom_status` vor `rank`. Wer beide Kriterien
erfüllt — eine Varietät mit `", nom. nud."` bei `rank=species` — erscheint
unter `nom_status`, nicht unter `rank`. `excluded.rank` ist deshalb **nicht**
die Zahl der rangbedingt unpublizierbaren Synonyme, sondern die Zahl derer,
die *nur* am Rang gescheitert sind. Im Beispiel oben steht `rank: 16`,
obwohl das Concept 17 infraspezifische Synonyme hat: eines davon
(*Corynephorus canescens* var. *andinus*, `", nom. nud."`) ist bereits unter
`nom_status` verbucht. Korpusweit betrifft das 14.202 Zeilen. `total` minus
`publishable` bleibt in jedem Fall die Summe über `excluded`.

#### Die vollständige `nom_status`-Regeltabelle

Gematcht wird per **Token-Containment über die normalisierte Zelle**
(kleingeschrieben, Whitespace zusammengezogen, führendes `", "` entfernt),
niemals per Gleichheit: Das Vokabular hat 1.304 distinkte Werte, von denen
1.225 weniger als zehn Treffer haben. Die Spalte „Namen" ist die gemessene
Containment-Trefferzahl im Index (1.448.984 Namen).

Reihenfolge der Auswertung: **Unsicherheitsmarker** → **Guards** (ihr
Treffer wird aus der Zelle maskiert, damit ein breiteres Token nicht auf
Text feuert, den ein engeres schon beansprucht hat) → **Regeln**. Die
Präzedenz entscheidet dann in dieser Reihenfolge: Unsicherheit gewinnt über
alles, danach ein beliebiger disqualifizierender Treffer, danach ein Guard,
danach ein akzeptierender Treffer, sonst `unclassified`.

⚠️ markiert die Werte, deren Behandlung eine **botanische** und keine
technische Entscheidung ist; sie werden zurückgehalten statt geraten.

| Token | Urteil | Namen | Bedeutung |
| --- | --- | ---: | --- |
| `?` | unclassified ⚠️ | 13 | Fragezeichen: die Quelle selbst ist unsicher; deckt `, not validly publ.?` (8), `, an nom. valid.?` (4), `, nom. superfl. ?` (1) |
| `sensu auct.` | unclassified ⚠️ | 1.117 | Fehlanwendung, kein nomenklatorischer Mangel |
| `tentatively listed as a synonym` | unclassified ⚠️ | 290 | taxonomische Unsicherheit, keine Publikationsfrage |
| `fossil name` | unclassified ⚠️ | 274 | sagt nichts über die nomenklatorische Gültigkeit |
| `isonym` | unclassified ⚠️ | 13 | Doppelveröffentlichung desselben Namens |
| `nom. cons. prop.` | unclassified | 33 | Konservierung **beantragt**, nicht entschieden |
| `nom. utique rej. prop.` | unclassified | 14 | vollständige Verwerfung beantragt, nicht entschieden |
| `nom. rej. prop.` | unclassified | 48 | Verwerfung beantragt, nicht entschieden |
| `illeg` | disqualifying | 49.705 | illegitim; deckt `nom. illeg. homonym. post.` (36.424), `nom. illeg. superfl.` (10.768), `nom. illeg.` (2.405) |
| `not validly publ` | disqualifying | 18.623 | nicht gültig veröffentlicht (inkl. Basionym-/Gattungs-/Artvarianten) |
| `superfl` | disqualifying | 12.502 | überflüssig veröffentlicht — das Token des UC5-Beispiels |
| `nom. nud.` | disqualifying | 9.222 | nomen nudum — ohne Beschreibung veröffentlicht |
| `pro syn` | disqualifying | 6.224 | als Synonym veröffentlicht, also nicht gültig |
| `orth. var.` | disqualifying | 2.196 | orthografische Variante — ein Schreibfehler |
| `opus utique` | disqualifying | 1.640 | in einem unterdrückten Werk erschienen (`oppr.` 1.528 / `rej.` 111) |
| `basionym` | disqualifying | 1.438 | fehlerhafter oder fehlender Basionym-Bezug (alle 1.438 Zellen sind Mangelaussagen) |
| `latin descr` | disqualifying | 1.344 | keine lateinische Beschreibung; führt die Schreibvarianten zusammen |
| `type` | disqualifying | 1.099 | Typus-Zitatmangel; 1.098 der 1.099 Zellen sind Mangelaussagen (siehe Hinweis unten) |
| `nom. rej` | disqualifying | 894 | verworfen; `nom. rej.` (831) + `nom. rejic.` (10), Anträge per Guard maskiert |
| `contrary to art` | disqualifying | 432 | entgegen einem benannten ICN/ICBN-Artikel veröffentlicht |
| `nom. provis` | disqualifying | 363 | provisorischer Name — nicht gültig veröffentlicht |
| `nom. subnud` | disqualifying | 238 | unzureichend beschrieben |
| `comb. not` | disqualifying | 201 | Kombination nicht (gültig) vorgenommen |
| `sphalm` | disqualifying | 199 | sphalmate — ein Druckfehler, kein Name |
| `nom. utique rej` | disqualifying | 151 | vollständig verworfen; Anträge per Guard maskiert |
| `not effectively publ` | disqualifying | 66 | nicht wirksam veröffentlicht; führt `publ.`/`published` zusammen |
| `describing the collection` | disqualifying | 61 | beschreibt die Aufsammlung, nicht das Taxon |
| `later homonym` | disqualifying | 60 | späteres Homonym; illegitim nach Art. 53 |
| `combination not` | disqualifying | 37 | ausgeschriebene Variante von `comb. not made.` |
| `without diagnostic descr` | disqualifying | 17 | keine diagnostische Beschreibung |
| `sine descr. lat.` | disqualifying | 15 | lateinische Schreibung von „ohne lateinische Beschreibung" |
| `nom. cons.` | acceptable | 1.237 | konservierter Name — ausdrücklich legitim (Anträge per Guard maskiert) |
| `nom. altern.` | acceptable | 103 | Alternativname, gültig veröffentlicht |
| `nom. alt.` | acceptable | 36 | Kurzschreibung von `nom. altern.` |
| `legitimate homonym` | acceptable | 12 | ausdrücklich legitim — der Grund, warum blankes `homonym` keine Regel ist |
| `orth. cons.` | acceptable | 11 | konservierte Schreibweise; deckt auch `nom. & orth. cons.` (7) |

Die Tabelle ist gegen `domain.NomStatusRules()` festgenagelt
(`internal/domain/nomstatus_doc_test.go`): Token, Urteil und gemessene
Trefferzahl müssen zeilenweise übereinstimmen, sonst schlägt `make test`
fehl. Eine neue Regel ohne Doku-Zeile ist damit kein stillschweigendes
Auseinanderlaufen mehr.

!!! note "Bekannter Einzelfall in `type`"
    Von den 1.099 Zellen mit `type` ist genau **eine** keine Mangelaussage:
    `", type variety."` (1 Name) ist eine taxonomische Anmerkung. Dieser eine
    Name wird derzeit mit ausgeschlossen. Zurückhalten ist die sichere
    Richtung; die saubere Auflösung wäre ein Guard, sobald entschieden ist,
    wie `type variety` in einer Publikationsliste zu behandeln ist.

#### `typification`: dreiwertig, aber `heterotypic` kommt nicht vor

`typification` ist `homotypic`, `unknown` oder `heterotypic`. Auf dem
aktuellen Index **kann `heterotypic` nicht auftreten**:
`concept_name.homotypic` ist `1` (271.821 Zeilen) oder `NULL` (1.133.475
Zeilen) und in **keiner einzigen** Zeile `0`. Ein Synonym ist damit heute
entweder nachweislich homotypisch oder `unknown`.

Das ist kein Versehen, sondern die Fortsetzung einer früheren Entscheidung:
SP3 hat sich geweigert, Heterotypie zu raten, wo die Quelle keine
Basionym-Verknüpfung liefert, und `/v1/concept` lässt das Feld `homotypic`
lieber ganz weg, als `false` zu behaupten. `unknown` hier auf `heterotypic`
zusammenzuziehen würde den größten Teil des Korpus aufgrund einer Tatsache
herabstufen, die niemand festgestellt hat. Der Wert steht im Modell, weil
die Spalte dreiwertig ist — nicht, weil eine Antwort ihn heute zeigen wird.

#### Weitere Zusicherungen

- **`rank_verbatim`** trägt die ursprüngliche Schreibweise, wenn `rank`
  `OTHER` ist (`proles`, `lusus`, `microgène`, `Convariety`, `grex`) —
  dieselbe Begründung wie bei `GET /v1/concept/{id}`: `OTHER` ist der eine
  Rangwert, der die Quellschreibweise durch das Sammelbecken verloren hat.
  Bei jedem kanonisch benannten Rang fehlt das Feld, weil `rank` die
  Schreibweise dort bereits exakt benennt.
- **Jedes Urteilsfeld ist immer vorhanden**, auch wenn es `false` ist:
  `is_basionym: false` ist eine Antwort, und ein weggelassenes Feld wäre
  von „nicht geprüft" nicht zu unterscheiden. Ausgelassen werden nur
  `nom_status` (die Quelle hat nichts erfasst — `nom_status_judgement`
  sagt dann ausdrücklich `absent`), `authorship`, `exclusion` (nicht
  ausgeschlossen) und `publication_rank` (kein Rang ausgeschlossen).
- Ein unbekanntes Concept liefert `404 NOT_FOUND`. Ein **bekanntes**
  Concept ohne Synonyme liefert `200 OK` mit leerem `synonyms`-Array und
  genullter `summary` — das ist kein Fehlerfall.
- `GET /v1/concept/{id}` bleibt unverändert: dessen `synonyms`-Array ist
  weiterhin ungefiltert und trägt weder `nom_status` noch `is_basionym`
  noch ein Publikationsurteil.

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

Der `verbatim`-Einstieg war am vollen Index praktisch tot (gleichnamige
Konzepte über die `sec.`-Räume → mehrdeutig). Mit dem SP5-Filter
**`entry_sec`** (id eines `sec.`-Raums; oder `entry_backbone`) löst `verbatim`
in **einem** Raum auf und übersetzt dann — gemessen eindeutig in 99,67 % der
Fälle. Bei `concept_id` wird der Filter ignoriert. Unbekannter Wert → `400`.

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
      "stored_relation": "congruent",
      "relation_from_source": "congruent",
      "has_inverse": true,
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
`true`.

Die folgende Tabelle beschreibt **`relation_from_source`** — die
quellenseitige, richtungssichere Lesart. Sie gilt **nicht** unbesehen für
`stored_relation`: das ist die gespeicherte Zeile, deren Richtung von
`direction` abhängt (siehe [nächster Abschnitt](#richtung-a-includes-b-ist-nicht-b-included_in-a)).

| `relation_from_source` | Symbol | `is_equality` | Bedeutung (Quelle → Kandidat) |
|---|---|---|---|
| `congruent` | ≜ | **`true`** | gleiche Umgrenzung |
| `not_congruent` | — | `false` | ausdrücklich **nicht** gleich |
| `includes` | ⊃ | `false` | die **Quelle** ist die weitere Umgrenzung |
| `included_in` | ⊂ | `false` | die **Quelle** ist die engere Umgrenzung |
| `overlaps` | ⊕ | `false` | teilweise Überschneidung, keine Gleichsetzung |
| `includes_or_included_in_or_overlaps` | ⊂⊃⊕ | `false` | Quelle **legt sich nicht fest**, welche der drei gilt |
| `pro_parte` | p.p. | `false` | gerichtete Teilaussage über den Namen der From-Seite |
| `null` | — | `false` | **keine sinnvolle Umkehrung** (eingehende `pro_parte`-Kante); `has_inverse` ist dann `false`, und nur `statement` gilt |

`included_in` kommt als **gespeicherter** Wert nie vor — CDM emittiert
ausschließlich die `Includes`-Richtung. Als `relation_from_source` tritt es
bei jeder eingehenden `includes`-Kante auf; genau dafür ist der Wert im
Vokabular behalten worden.

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
- `stored_relation` ist die Relation dieser Zeile. Ihre Richtung hängt von
  `direction` ab; sie ist **nicht** quellenseitig zu lesen.
- `direction` ist `source_to_target`, wenn das Ausgangskonzept die
  From-Seite ist, sonst `target_to_source`.
- `relation_from_source` ist dieselbe Kante quellenseitig gelesen (bei einer
  eingehenden `includes`-Kante also `included_in`), plus `has_inverse: true`.

!!! warning "Es gibt bewusst kein Feld `relation`"
    Der kurze, verlockende Name wird nicht vergeben. CDM emittiert
    ausschließlich die `Includes`-Richtung, eingehende Kanten sind also
    häufig — ein Client mit `if c.relation == "includes"` läse eine
    eingehende Kante **genau verkehrt herum**. Wer eine
    richtungsunabhängige Aussage braucht, nimmt `relation_from_source`;
    wer die Rohzeile braucht, `stored_relation`/`statement`.

Bei einer **eingehenden `pro_parte`-Kante** ist `relation_from_source`
ausdrücklich `null` und `has_inverse` `false` — nicht etwa weggelassen. Eine
gerichtete Aussage über den Namen der Gegenseite hat keine sinnvolle
Umkehrung, hostus erfindet keine, und ein fehlender Schlüssel läse sich wie
„unbekannt". Das `note`-Feld nennt den Grund.

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

## `sec.`-Endpunkt

### `GET /v1/sec`

Listet jeden ingestierten `sec.`-Referenzraum als `{id, title}`, id-sortiert.
Der Endpunkt existiert, damit `target_space` (`POST /v1/translate`) und
`entry_sec` (`POST /v1/match`) nicht geraten werden müssen: ohne diese Liste
ist ein falsch getippter Raum von einem leeren Ergebnis nicht zu
unterscheiden. Die Testkonsole füllt daraus die Auswahlliste des
`target_space`-Felds.

```
GET /v1/sec
```

```json
{
  "sec_references": [
    { "id": "060afae5-76ef-44a7-921f-1202685ef351", "title": "Wisskirchen & Haeupler 1998: Standardliste der Farn- und Blütenpflanzen Deutschlands" },
    { "id": "0d1e…", "title": "auct." }
  ]
}
```

`sec_references` ist immer ein Array — ein Index ohne `sec.`-Räume liefert
`[]`, nie `null`. `title` wird ausgelassen, wenn die `sec_reference`-Zeile
keine Zitation trägt. Keine Parameter, keine Fehlerantwort außer
`500 INTERNAL_ERROR`: die Frage „gibt es diesen Raum?" wird durch seine
**Abwesenheit** aus der Liste beantwortet.

## Gebiets-Endpunkt

### `GET /v1/areas`

Listet jedes Verbreitungsgebiet, das im Index Daten trägt (ein DISTINCT
`area_scheme`/`area_code` aus der Distribution), je mit seinem ausgeschriebenen
Namen. Der Name wird beim Ingest aus der `Locality`-Spalte des
WCVP-Distributionsdumps selbst beschafft. Der Endpunkt existiert, damit ein
Client (die Testkonsole füllt daraus die Gebiets-Auswahlliste) „Germany (GER)"
anbieten kann statt des bloßen WGSRPD-Codes — die Codes sind **WGSRPD, nicht
ISO** (`GER`, nicht `DE`) und werden selten erinnert.

```
GET /v1/areas
```

```json
{
  "areas": [
    { "code": "FRA", "name": "France", "scheme": "wgsrpd_l3" },
    { "code": "GER", "name": "Germany", "scheme": "wgsrpd_l3" }
  ]
}
```

`areas` ist immer ein Array (`[]`, nie `null`). Es werden **nur Gebiete mit
Daten** gelistet, sortiert nach (`scheme`, `code`); `name` wird ausgelassen,
wenn die Quelle keinen lieferte. Keine Parameter, keine Fehlerantwort außer
`500 INTERNAL_ERROR`. Der `?area=`-Parameter von `GET /v1/suggest` bleibt
code-basiert (plus die Aliase `DE/AT/CH`); die Auflösung „Germany"→`GER` ist
eine Konsolen-Bequemlichkeit auf Basis dieser Liste.

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

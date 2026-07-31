# HTTP-API

!!! note "Stand"
    Dies beschreibt den SP2-Stand: die lokale SQLite/FTS5-Rückgrat-Basis ist
    implementiert (`hostus ingest` befüllt die Datenbank aus WCVP/POWO-
    DwC-A-Manifesten; `hostus serve` bedient `/v1/concept/{id}`, `/v1/xref`,
    `/v1/match` und `/v1/suggest` daraus). Weitere `/v1/*`-Endpunkte
    (`concept/{id}/traits`, `concept/{id}/synonyms`, `translate`) sowie
    `/openapi` folgen in späteren SPs. Die maßgebliche OpenAPI-Spezifikation
    liegt unter `api/openapi/openapi.yaml`.

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
  "xrefs": { "powo": "396681-1" },
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

`vernacular_de` (deutscher Trivialname) und `classification`
(Klassifikationskette) sind Teil der DTO, aber in SP1 immer leer/omitted —
die Vernakular-Tabelle bzw. Eltern-Verknüpfungen werden noch nicht
ingestiert. `distribution` (Referenzgebiets-Zuordnungen, z. B. WGSRPD L3)
wird dagegen bereits von `hostus ingest` befüllt und ausgeliefert; das
Feld ist leer, wenn der Backbone für dieses Concept keine Distribution
liefert.

Unbekannte IDs liefern `404 NOT_FOUND` im [Fehlerformat](#fehlerformat).

### `GET /v1/xref?authority={authority}&id={id}`

Löst eine externe Cross-Reference (z. B. eine POWO-ID) auf und liefert
dieselbe Concept-Repräsentation wie `GET /v1/concept/{id}`.

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
entgegengenommen, aber in SP1 nicht ausgewertet (Sekundärraum-Übersetzung
folgt erst in SP5 als `POST /v1/translate`).

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
| `UNRESOLVABLE`        | —    | Kein HTTP-Fehler: `POST /v1/match` rendert eine nicht auflösbare Anfrage als normales `200`-Ergebnis mit `match_type: "unresolvable"`, siehe oben |
| `GBIF_TIMEOUT`        | 504  | GBIF-Anfrage Timeout (nur Ingest-/Enrichment-Pfad)      |
| `GBIF_UNAVAILABLE`    | 502  | GBIF nicht erreichbar (nur Ingest-/Enrichment-Pfad)     |
| `INTERNAL_ERROR`      | 500  | Interner Serverfehler                                   |

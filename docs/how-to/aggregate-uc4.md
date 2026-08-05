# Aufnahmen in den ESy-Namensraum auflösen (UC4)

Ziel dieser Anleitung: eine Vegetationsaufnahme, die Sammelarten
(„Aggregate") enthält, gegen den FloraVeg-Namensraum auflösen und dabei **pro
Treffer erfahren, ob die Deckung eines Aggregats überhaupt zuweisbar ist** —
und die eine Auskunft, die hostus derzeit *nicht* geben kann, klar als fehlend
erkennen.

`POST /v1/match` liefert dazu mit gesetztem `target_space` drei zusätzliche
Felder je Treffer: `target_space_name` (die ESy-kompatible Schreibweise),
`aggregate_policy` (dreiwertig) und `esy_diagnostic_relevance`.

## Vorab: die Lizenzlage

!!! danger "FloraVeg-Daten: unklare Lizenz — nur lokale Auswertung"
    Der FloraVeg-Namensraum stammt aus einem floraveg.eu-Download
    (`Life_form.xlsx`). Für die Weitergabe ist **keine Lizenz auffindbar**;
    das Manifest führt die Quelle als `redistribution: unknown`. Zulässig
    ist die lokale Auswertung. Der Bundle-Export verweigert FloraVeg-Daten
    ohne `--force-include-restricted`.

## 1. Ingestieren

Ein Manifest, das WCVP als Backbone **und** FloraVeg als Namensraum pinnt
(siehe `dataset.example.yaml`, Abschnitt `name_spaces:`):

```bash
hostus ingest --dataset dataset.yaml --db hostus.sqlite
```

Der Report weist den Crosswalk offen aus — `matched/unmatched/ambiguous`, die
aufgelösten Aggregate und jede angewandte Normalisierungsregel. Die
gemessenen Zahlen gegen den vollen WCVP-Index stehen in
[FloraVeg-Namensraum](../research/floraveg-namespace.md).

## 2. Eine Aufnahme auflösen

Die Beispielaufnahme aus dem Quelldokument — *Corynephorus canescens* (40 %),
*Festuca ovina* agg. (15 %), *Jacobaea vulgaris* (2 %), *Rumex acetosella*
(5 %):

```bash
curl -sS -X POST http://localhost:8080/v1/match \
  -H 'Content-Type: application/json' \
  -d '{
        "target_space": "floraveg",
        "names": [
          {"id": "1", "verbatim": "Corynephorus canescens"},
          {"id": "2", "verbatim": "Festuca ovina agg."},
          {"id": "3", "verbatim": "Jacobaea vulgaris"},
          {"id": "4", "verbatim": "Rumex acetosella"}
        ]
      }'
```

Ohne `target_space` ist die Antwort byteweise die gewohnte SP1-Form — die drei
Felder fehlen dann vollständig. Ein **unbekannter** `target_space` ist ein
`400 INVALID_QUERY` und nennt den Raum.

## 3. `aggregate_policy` lesen — die drei Zustände

```json
{
  "id": "2",
  "match_type": "aggregate_alias",
  "concept_id": "…",
  "target_space_name": "Festuca ovina aggr.",
  "aggregate_policy": "known",
  "esy_diagnostic_relevance": "not_determinable"
}
```

- **`known`** — der Zielraum führt das Aggregat als eigenes Taxon; die
  ESy-Schreibweise steht in `target_space_name`. Die Deckung ist einem Namen
  zuweisbar, mit dem eine ESy-Regel rechnen kann.
- **`unresolvable`** — die Anfrage **ist** ein Aggregat, der Zielraum kennt
  darunter aber nur Kleinarten. Das heißt **„nicht entscheidbar", nicht
  „nicht erfüllt"**. Die Deckung darf **nicht** auf die Kleinarten verteilt
  werden, und es wird bewusst **kein** `target_space_name` geliefert — eine
  Kleinart als Aggregatnamen anzubieten wäre genau die falsche Zuweisung.
- **fehlt** — gar kein Aggregat im Spiel (gewöhnliche Art wie *Corynephorus
  canescens*). Ein `known` für jede Art würde das Feld bedeutungslos machen.

!!! note "`known` setzt ein Aggregat-Konzept im Backbone voraus"
    Damit eine Aggregat-Anfrage `known` liefert, muss sie auf ein Konzept
    auflösen, das eine FloraVeg-Aggregatschreibweise trägt. WCVP führt
    **keine** aggregatmarkierten Konzepte: dort landet FloraVegs „Festuca
    ovina aggr." per Normalisierung auf der **Nominatart** *Festuca ovina* —
    erreichbar nur über die *einfache* Anfrage (→ Zustand „fehlt"), nicht über
    „Festuca ovina agg.". Über einem WCVP-only-Backbone ist `known` daher
    praktisch nur erreichbar, wenn eine Aggregat-tragende Quelle (etwa ein
    künftiges Aggregat-Vokabular) dazukommt. Details und Messung:
    [SP9-Verdikt](../research/sp9-uc4-verdict.md).

## Was fehlt: `esy_diagnostic_relevance`

`esy_diagnostic_relevance` ist bei gesetztem `target_space` **immer present**
und **immer** `not_determinable`. hostus kann die ESy-diagnostische Relevanz
derzeit nicht bestimmen, weil **das ESy-Regelwerk nicht ingestiert** ist (die
FloraVeg-Pipeline hat nur eine Namensliste geerntet, nicht das Expertensystem
— siehe [Bekannte Lücken](../explanation/known-gaps.md)).

Das ist die wichtigste Einschränkung von UC4, und die am leichtesten zu
übersehende. Das Quelldokument nennt genau diesen Fall den entscheidenden:
Ruht eine ESy-Regel auf einer Kleinart, die im Feld nicht bestimmt werden
konnte, ist die richtige Antwort **„nicht entscheidbar"**, nicht „Habitat
nicht erfüllt". **Solange das Regelwerk fehlt, kann hostus diese beiden Fälle
nicht auseinanderhalten.**

Deshalb ist der Wert absichtlich ein selbsterklärender String und niemals
`null` oder fehlend: Seine Abwesenheit oder ein falsy-Wert dürfte **nie** als
„nicht relevant" gelesen werden — genau dieser Fehlschluss ist der von UC4
gefürchtete False Negative. Ein Konsument, der auf diesem Feld operiert, muss
`not_determinable` als „hier kann ich nicht entscheiden" behandeln, nicht als
Freibrief.

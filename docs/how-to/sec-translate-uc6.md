# Konzepte zwischen `sec.`-Referenzräumen übersetzen (UC6)

Ziel dieser Anleitung: „Dieses Konzept *sec.* Rothmaler — was ist es *sec.*
Wisskirchen & Haeupler 1998, und **wie genau** hängen die beiden zusammen?"

Ein Artname allein beantwortet das nicht. `Abies alba` Mill. steht in der
CDM-Ernte in zehn verschiedenen Referenzräumen — als **zehn getrennte
Konzeptzeilen**, weil jede Flora ihre eigene Umgrenzung (Circumscription)
meint. `POST /v1/translate` liefert die Verbindung zwischen zweien davon,
und zwar nur dann, wenn eine Quelle sie tatsächlich behauptet.

## Vorab: die Lizenzlage

!!! danger "CDM-Daten: keine auffindbare Lizenz — nur lokale Auswertung"
    Die Konzeptrelationen, von denen dieser Endpunkt lebt, stammen aus der
    CDM-Ernte der `rl_standardliste` (BGBM/EDIT).

    - Für diese Daten ist **keine Lizenz auffindbar** — weder auf dem
      Portal noch in der API noch in den Payloads selbst.
    - Sie sind aus **urheberrechtlich geschützter Floren-Literatur**
      abgeleitet (Rothmaler, HEGI, Oberdorfer, Flora Europaea, …); die
      Zitationstitel sind geernteter Inhalt, keine bloßen Metadaten.
    - Das Manifest führt die Quelle deshalb als
      `redistribution: unknown`.

    Konsequenz: **`/v1/translate` darf auf dieser Datenbasis nicht
    öffentlich betrieben werden**, solange keine **schriftliche Freigabe
    von BGBM/EDIT** vorliegt. Zulässig ist lokale Auswertung. Der
    Bundle-Export verweigert CDM-Daten ohne
    `--force-include-restricted` und trägt sie andernfalls in
    `bundle_meta.restricted_sources` ein.

## 1. Referenzräume auflisten

Die Ziel-Id ist die Id eines `sec.`-Referenzraums, nicht sein Titel. Nach
einem CDM-Ingest stehen sie in `sec_reference`:

```bash
sqlite3 hostus.sqlite "SELECT id, title FROM sec_reference ORDER BY title;"
```

```
060afae5-76ef-44a7-921f-1202685ef351|Wisskirchen & Haeupler 1998: Standardliste der Farn- und Blütenpflanzen Deutschlands
0f11a41f-8827-49dc-82df-4b02ad96d33e|HEGI: Illustrierte Flora von Mitteleuropa, Aufl. 2 u. 3
87ed3300-5846-4ff1-809c-eb289aec54d8|Schubert & Vent (eds.) 1990: Exkursionsflora … Rothmaler, 8. Aufl.
…
```

## 2. Übersetzen

```bash
curl -sS -X POST http://localhost:8080/v1/translate \
  -H 'Content-Type: application/json' \
  -d '{
        "concept_id": "cdm:concept:b7a352aa-1f73-41f3-a4e3-b24fc1c2cd5f",
        "target_space": "060afae5-76ef-44a7-921f-1202685ef351"
      }'
```

```json
{
  "result": "translated",
  "max_hops": 1,
  "candidates": [
    {
      "concept_id": "cdm:concept:872088a4-95f4-472c-ae79-a29028bb3fbf",
      "canonical": "Abies alba",
      "stored_relation": "congruent",
      "relation_from_source": "congruent",
      "has_inverse": true,
      "direction": "source_to_target",
      "statement": { "from": "cdm:concept:b7a352aa-…", "relation": "congruent", "to": "cdm:concept:872088a4-…" },
      "is_equality": true,
      "hops": 1,
      "source": "cdm"
    }
  ],
  "requires_review": false
}
```

Statt `concept_id` geht auch `verbatim` — dann läuft die Auflösung durch
dieselbe Logik wie `POST /v1/match`, und ein **Fuzzy-Treffer setzt
`requires_review: true`** auf der gesamten Antwort.

!!! tip "`verbatim` braucht `entry_sec` — sonst ist der Name mehrdeutig"

    Ein `sec.`-Referenzraum trennt Konzepte gleichen Namens: `Abies alba`
    Mill. ist mehrere CDM-Konzepte (eines je Referenzwerk) plus das
    WCVP-Konzept. `MatchExact` sucht über **alle** Backbones, also ist ein
    bloßes `verbatim` mehrdeutig — ohne Filter kam es am vollen Index in
    **265 von 300** Fällen als `UNRESOLVABLE` zurück, 0 übersetzt.

    **Lösung (SP5):** den `verbatim`-Einstieg mit **`entry_sec`** (der Id des
    **Quell**-`sec.`-Raums) auf genau einen Raum einschränken. Gemessen wird
    ein Name damit in **99,67 %** der Fälle eindeutig
    ([`docs/research/sp5-sec-filter.md`](../research/sp5-sec-filter.md)) und
    übersetzt dann über seine Relation. Alternativ `entry_backbone` (z. B.
    `wcvp`). Ein unbekannter `entry_sec`/`entry_backbone` ist `400
    INVALID_QUERY`.

    ```bash
    curl -sS -X POST http://localhost:8080/v1/translate \
      -H 'Content-Type: application/json' \
      -d '{"verbatim":"Abies alba Mill.","entry_sec":"<quell-sec-id>",
           "target_space":"<ziel-sec-id>"}'
    ```

    `concept_id` bleibt der direkteste Weg, wenn man die Id schon hat (dann
    wird der Filter ignoriert); mit `entry_sec` ist aber auch der
    Namens-Einstieg produktiv nutzbar.

## 3. Die Antwort richtig lesen

### Nur `congruent` heißt „dasselbe Taxon"

`is_equality` ist das **einzige** Feld, das als Gleichsetzung gelesen werden
darf. Es steht auf jedem Kandidaten — auch wenn es `false` ist, denn ein
fehlendes Feld läse sich wie „unbekannt".

Die Tabelle gilt für **`relation_from_source`** — die quellenseitige,
richtungssichere Lesart. Für `stored_relation` gilt sie **nicht** unbesehen;
dessen Richtung hängt von `direction` ab (siehe nächster Abschnitt).

| `relation_from_source` | Symbol | `is_equality` | Was die Quelle sagt (Quelle → Kandidat) |
|---|---|---|---|
| `congruent` | ≜ | **`true`** | gleiche Umgrenzung |
| `not_congruent` | — | `false` | ausdrücklich **nicht** gleich |
| `includes` | ⊃ | `false` | die **Quelle** ist weiter |
| `included_in` | ⊂ | `false` | die **Quelle** ist enger |
| `overlaps` | ⊕ | `false` | Überschneidung, aber jede Seite hat Eigenes |
| `includes_or_included_in_or_overlaps` | ⊂⊃⊕ | `false` | die Quelle **legt sich nicht fest** |
| `pro_parte` | p.p. | `false` | gerichtete Teilaussage über den Namen |
| `null` | — | `false` | keine sinnvolle Umkehrung; `has_inverse: false` |

`⊂⊃⊕` wird bewusst **nicht** auf `overlaps` eingeebnet. Die Quelle sagt
ausdrücklich, dass sie offenlässt, welche der drei Beziehungen gilt; eine
Einebnung würde eine unsichere Aussage still zu einer definiten aufwerten.

Ein `overlaps`- oder `⊂⊃⊕`-Ergebnis ist **kein** Ersatz für das Zielkonzept.
Wer es als solches weiterverarbeitet, produziert genau die Verwechslung, die
UC6 verhindern soll.

### Richtung: `A includes B` ist nicht `B included_in A`

hostus speichert jede Relation **in der Richtung, in der die Quelle sie
nennt**, und legt keine gespiegelte Zeile an. Deshalb trägt jeder Kandidat
vier Angaben:

- `statement` — die gespeicherte Aussage, wortwörtlich.
- `stored_relation` — die Relation dieser Zeile. Richtung laut `direction`,
  **nicht** quellenseitig zu lesen.
- `direction` — `source_to_target`, wenn das Ausgangskonzept die From-Seite
  ist, sonst `target_to_source`.
- `relation_from_source` — dieselbe Kante quellenseitig gelesen. Bei einer
  eingehenden `includes`-Kante steht dort `included_in`.

!!! warning "Es gibt bewusst kein Feld `relation`"
    Der kurze Name wird nicht vergeben. CDM emittiert ausschließlich die
    `Includes`-Richtung, eingehende Kanten sind also häufig — ein Client
    mit `if c.relation == "includes"` läse eine eingehende Kante **genau
    verkehrt herum**. Richtungssicher ist `relation_from_source`; die
    Rohzeile steht in `stored_relation`/`statement`.

Bei einer **eingehenden `pro_parte`-Kante** ist `relation_from_source`
ausdrücklich `null` (nicht weggelassen) und `has_inverse` `false`: eine
gerichtete Aussage über den Namen der Gegenseite hat keine sinnvolle
Umkehrung, hostus erfindet keine, und ein fehlender Schlüssel läse sich wie
„unbekannt". Das `note`-Feld sagt das.

`misapplied` erscheint hier nie. CDM flaggt diese Zeilen
`conceptRelationship: false`, weil sie über **Namensverwendung** sprechen und
nicht über Umgrenzungen; der Ingest verwirft sie sichtbar (gezählt und
bemustert), statt zwei Arten von Aussage unter derselben Spalte zu mischen.

## 4. Die Ein-Hop-Grenze

`/v1/translate` folgt **genau einer** Relationskante — immer, ohne
Konfigurationsmöglichkeit.

Der Grund ist inhaltlich, nicht technisch: eine transitive Kette ist über
dieses Vokabular nicht allgemein gültig. `congruent ∘ includes` wäre
vertretbar; `overlaps ∘ overlaps` sagt **gar nichts** (zwei Umgrenzungen, die
beide eine dritte überlappen, können disjunkt sein); und `⊂⊃⊕ ∘ irgendwas`
ist per Konstruktion undefiniert. Eine korrekte Kompositionsregel wäre
paarweise verschieden, und keine Quelle in hostus nennt eine. Also
komponiert hostus nicht.

Praktisch heißt das: Gibt es `A ≜ B` und `B ⊃ C`, dann liefert eine Anfrage
von A in den Raum von C **keine** Kandidaten. Wer die Kette braucht, fragt
sie Schritt für Schritt ab und entscheidet die Verkettung selbst — mit dem
Wissen, dass sie eine eigene Behauptung ist.

`max_hops` steht auf jeder Antwort. Ein Request mit `max_hops != 1` wird mit
`400 INVALID_QUERY` **benannt abgelehnt**, statt still eine Ein-Hop-Antwort
zu liefern, die man für tiefer halten könnte.

## 5. Wenn nichts gefunden wird

```json
{
  "result": "no_relation_recorded",
  "candidates": [],
  "note": "Keine erfasste Relation in den Zielreferenzraum. Das bedeutet NICHT, dass keine Beziehung besteht — nur, dass keine Quelle eine erfasst hat."
}
```

**Das ist die wichtigste Zeile dieser Anleitung:** eine leere Antwort
bedeutet „**keine Relation erfasst**", nicht „**keine Relation
vorhanden**". Die Ernte deckt ab, was die Standardliste verzeichnet — nicht
die Gesamtheit taxonomischer Beziehungen. Ein fehlender Eintrag ist eine
Lücke im Datenbestand, kein negativer Befund.

`candidates` kommt in diesem Fall als **leeres Array** (nicht weggelassen),
und `result` benennt den Ausgang. Ein Namenstreffer wird **nie**
ersatzweise als Übersetzung ausgeliefert.

Wer trotzdem einen Anhaltspunkt für die manuelle Prüfung braucht:

```bash
curl -sS -X POST http://localhost:8080/v1/translate \
  -H 'Content-Type: application/json' \
  -d '{"concept_id":"…","target_space":"…","include_name_candidates":true}'
```

Namensgleiche Konzepte des Zielraums erscheinen dann unter dem **eigenen**
Schlüssel `unrelated_name_candidates`, jeder Eintrag mit
`requires_review: true` und **ohne jedes Relationsfeld**; die Antwort als
Ganzes trägt ebenfalls `requires_review: true`. Das ist ausdrücklich **keine
Übersetzung**, sondern eine Namensgleichheit, die ein Mensch prüfen muss.
Sobald eine echte Relation existiert, entfällt der Block.

## 6. Fehlerfälle

| Fall | Status | Code |
|---|---|---|
| Body nicht parsbar; keins oder beides von `concept_id`/`verbatim`; `target_space` fehlt; `max_hops != 1` | `400` | `INVALID_QUERY` |
| Unbekannter `entry_backbone` oder `entry_sec` (Filter, SP5) | `400` | `INVALID_QUERY` |
| Unbekannte `concept_id` **oder** unbekannter `target_space` | `404` | `NOT_FOUND` |
| `verbatim` nicht auf genau ein Konzept auflösbar (Filter erwägen: `entry_sec`) | `422` | `UNRESOLVABLE` |

Ein unbekannter `target_space` ist ausdrücklich ein `404` und **keine leere
Antwort**: Ein Tippfehler im Zielraum darf nicht wie „keine Relation
erfasst" aussehen.

## Siehe auch

- [HTTP-API: `POST /v1/translate`](../reference/http-api.md#post-v1translate)
  — vollständige Feldreferenz
- [Offline-Bundle exportieren](offline-bundle.md) — warum ein Bundle mit
  CDM-Daten ohne `--force-include-restricted` verweigert wird

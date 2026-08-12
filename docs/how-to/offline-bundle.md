# Offline-Bundle exportieren (`hostus bundle`)

`hostus bundle` exportiert einen eigenständigen, gebietsgescopten Auszug der
lokalen SQLite/FTS5-Datenbank in eine zweite, unabhängige SQLite-Datei — das
**Bundle**. Ein Bundle ist keine Kopie im laufenden Betrieb, sondern eine
vollständig eigenständige Datenbank: einmal exportiert, lässt sie sich per
`hostus serve` bedienen, ohne dass die Quell-Datenbank, ein Netzwerk oder ein
Upstream-Dienst erreichbar sein muss. Das ist der Fall für den
Feldeinsatz (z. B. Kartierungs-Apps ohne Konnektivität): das Bundle wird
einmal online erzeugt und danach komplett offline abgefragt.

## Voraussetzung

Eine bereits per `hostus ingest` befüllte Quell-Datenbank (siehe
[Konfiguration](../reference/configuration.md) für `--db`/`HOSTUS_SQLITE_PATH`).

## Verwendung

```bash
hostus bundle --db hostus.sqlite --area DE,AT,CH --out bundle.sqlite [--snapshot v1]
```

| Flag                          | Pflicht | Beschreibung                                                                 |
|-------------------------------|---------|-------------------------------------------------------------------------------|
| `--db`                        | ja      | Pfad zur Quell-SQLite-Datenbank (bereits ingestiert).                          |
| `--out`                       | ja      | Zielpfad für die neu erzeugte Bundle-Datei.                                    |
| `--area`                      | nein    | WGSRPD-L3-Referenzgebietscode (z. B. `AUT`), Kurzform (z. B. `DE`) oder eine **kommagetrennte Liste** davon (z. B. `DE,AT,CH` für Mitteleuropa) — das Bundle enthält dann die Vereinigung aller aufgelösten Gebiete. Leer = gesamte Datenbank, ungescopt. |
| `--snapshot`                  | nein    | Freitext-Versionskennung, wird unverändert in `bundle_meta.snapshot_version` geschrieben. |
| `--force-include-restricted`  | nein    | Übersteuert das Redistribution-Gate (siehe unten) — nur explizit setzen, wenn die Weitergabe der genannten Quelle(n) bewusst in Kauf genommen wird. |

Ein ungescopter Export (`--area` leer) funktioniert unabhängig von der
Datenbankgröße: der Export bindet die Konzept-ID-Liste als ein einziges
`json_each`-JSON-Parameter (dasselbe Muster, das `MatchFuzzyCandidates`
bereits verwendet), statt einen SQL-Platzhalter je Konzept-ID zu binden.
Vor Task 4 scheiterte ein Voll-Export an SQLites Parameterlimit
(`SQLITE_MAX_VARIABLE_NUMBER`) — siehe
[`docs/research/reality-check.md`](../research/reality-check.md) M5.1.

**Beispiel-Ausgabe**

```
Bundle complete: bundle.sqlite (concepts=3 names=13 areas=16)
```

## Redistribution-Gate: ein Bundle kann keine ungeklärte Quelle mitführen

Jeder Backbone-, Trait-Vokabular-, Xref-Quellen- und Namensraum-Eintrag im
Manifest trägt ein Pflichtfeld `redistribution: allowed|restricted|unknown` (siehe
[Merkmalswerte pipeln und ingestieren](trait-ingest.md) für die volle
Erklärung). `hostus bundle` prüft vor jedem Export, welche Quellen
tatsächlich Daten zum gewählten Scope (`--area` oder die ganze Datenbank)
beitragen:

- **Trägt eine nicht-`allowed`-Quelle bei** (ein Backbone, ein
  Trait-Vokabular, eine Xref-Quelle unter `xref_sources:` — die
  Herkunft jeder Xref-Zeile steht dafür in `xref.source` und der
  `xref_source`-Tabelle — oder ein Namensraum unter `name_spaces:`, dessen
  Einträge in `name_space_entry` am jeweiligen Konzept hängen),
  **schlägt der Export standardmäßig fehl** —
  die Fehlermeldung nennt die Quelle und ihren Redistribution-Wert:

  ```
  Error: sqlite: bundle: refusing to export: source(s) not cleared for
  redistribution: eive (redistribution=unknown) (use --force-include-restricted
  to override)
  ```

  Lokales Ingestieren dieser Quelle ist davon **nicht** betroffen — nur der
  Export ist gesperrt.

- **Mit `--force-include-restricted`** gelingt derselbe Export — die
  betroffene(n) Quell-ID(s) werden zusätzlich, kommagetrennt und sortiert,
  in `bundle_meta.restricted_sources` festgehalten:

  ```bash
  hostus bundle --db hostus.sqlite --out bundle.sqlite --force-include-restricted
  ```

  Ein leeres `bundle_meta.restricted_sources` bedeutet also entweder "der
  Export wurde ohne `--force-include-restricted` erzeugt und war deshalb
  garantiert frei von nicht-`allowed`-Quellen", oder "jede beitragende
  Quelle war ohnehin `allowed`" — ein Bundle kann so nie stillschweigend
  ungeklärte Daten mitführen.

## Was ein Bundle enthält

Ein Bundle ist dasselbe Schema wie die Haupt-Datenbank (`hostus.sqlite`),
gefüllt mit nur den Zeilen, die `--area` selektiert (oder allen, wenn
`--area` leer ist):

- die betroffenen `taxon_concept`-Zeilen samt `name`/`concept_name`/`xref`/
  `distribution`/`vernacular`
- die referenzierten `backbone_version`-Zeilen
- die `name_space_entry`-Zeilen der kopierten Konzepte samt der zugehörigen
  `name_space`-Provenienz — beides **konzept-gescopt**, nicht vollständig
  kopiert: `name_space_entry.name` ist geernteter Inhalt (die Schreibweise
  des Fremdraums), kein Quellen-Metadatum. Dadurch kann ein Bundle nur
  Namen tragen, die das Redistribution-Gate zuvor gesehen und entweder
  abgelehnt oder in `restricted_sources` protokolliert hat. Für FloraVeg
  (`redistribution: unknown`) heißt das: standardmäßig verweigert.
- ein neu aufgebauter `fts_name`/`fts_name_map`-FTS5-Index (nicht kopiert,
  sondern aus den kopierten Zeilen neu erzeugt — ein Bundle ist also selbst
  über `GET /v1/suggest` abfragbar)
- eine `bundle_meta`-Provenienz-Zeile (`snapshot_version`, `area`,
  `created_at`, `source_manifest_sha`, `restricted_sources` — siehe unten,
  "Redistribution-Gate")

### Was ein gebietsgescoptes Bundle NICHT mehr enthält (Task 4)

Der Reality-Check (M5.2) maß das GER-Bundle mit 108,9 MB gegen ein
Spec-Ziel von 10–20 MB — Faktor 5,4 zu groß — und fand den größten
Einzelposten in `distribution`: ein gescoptes Bundle kopierte bislang die
**vollständige globale Verbreitung** jedes ausgewählten Konzepts (bis zu
369 WGSRPD-L3-Gebiete pro Art), nicht nur das angefragte Gebiet. Gemessen
per `SELECT name, SUM(pgsize) FROM dbstat GROUP BY name` trug
`distribution` plus seinen eindeutigen Index rund 39 % zur Dateigröße bei.

**Seit Task 4:** Ein gebietsgescoptes Bundle (`--area` nicht leer) kopiert
in `distribution` **nur noch die Zeilen der angefragten Gebietscodes**
(`wgsrpd_l3`-Schema, exakt die per `--area` aufgelöste Codeliste) — nicht
mehr die globale Verbreitung außerhalb des Scopes. Ein Bundle beantwortet
damit "kommt dieses Konzept im angefragten Gebiet vor?", nicht mehr "wo auf
der Welt kommt dieses Konzept überall vor?". Ein ungescopter Export
(`--area` leer) ist davon **nicht** betroffen: er kopiert weiterhin jede
`distribution`-Zeile unverändert, weil "alles" dort auch wirklich alles
bedeutet.

Namen/Synonyme sind von dieser Kürzung **nicht** betroffen: ein Bundle
enthält weiterhin jedes Synonym der ausgewählten (im Scope befindlichen)
Konzepte — Feldnutzer:innen, die einen Synonym-Namen eintippen, werden
weiterhin auf das akzeptierte Konzept verwiesen. Synonyme von Konzepten
**außerhalb** des Scopes waren schon vor Task 4 nie Teil eines Bundles
(die Namen-Kopie ist über `concept_name` an die ausgewählten Konzepte
gebunden). Trait-Werte und die FTS5-Volltextsuche sind ebenfalls
unverändert — beides sind Daten, die ein Feldeinsatz direkt braucht, keine
verwerfbaren Nebenprodukte.

**Aktuell gemessen (2026-08-12, frischer WCVP-+-Trait-Ingest, `--area GER`):**
das GER-Bundle ist **89,2 MiB (93,5 MB)** — 11.583 Konzepte, 169.670 Namen,
`distribution` auf 11.583 GER-Zeilen gekürzt. Das ist **kleiner** als die
108,9-MB-M5.2-Baseline (die Distribution-Kürzung überwiegt deutlich) und liegt
nur rund **8 MiB** über der 81-MiB-Messung direkt nach Task 4: die seither in
SP6 befüllten Spalten `nom_status`/`rank_verbatim` (auf 20.688 bzw. 2.243 der
169.670 Namen) kosten zusammen einstellige MiB — **nicht** die früher grob
geschätzten ~50 MB (deshalb: messen, nicht schätzen). Größter Einzelposten ist
jetzt das `name`-Table samt Indizes (~28 MiB), nicht mehr `distribution`. Gegen
das 10–20-MB-Spec-Ziel bleibt das Bundle damit **Faktor ~4,7–9,4 zu groß** —
die Kürzung half, reicht aber nicht an die Obergrenze heran.

Die vollständige dbstat-Aufschlüsselung und die Mitteleuropa-Zahlen (`--area
DE,AT,CH`) stehen im "Nach Hardening"-Abschnitt von M5 in
`docs/research/reality-check.md` (repo-lokal, nicht Teil der veröffentlichten
Doku — siehe deren eigener Hinweis dazu).

## Bundle offline bedienen

Ein Bundle ist eine ganz normale, von `hostus serve` lesbare SQLite-Datei —
es braucht keinen speziellen Serve-Modus:

```bash
HOSTUS_SQLITE_PATH=bundle.sqlite hostus serve
```

```bash
curl "http://localhost:8080/v1/suggest?q=coryn&area=AUT"
```

liefert Ergebnisse ausschließlich aus dem Bundle — ohne dass die
ursprüngliche `hostus.sqlite` oder ein Netzwerkzugriff dafür nötig ist.
`GET /v1/concept/{id}` und `GET /v1/xref` funktionieren ebenso, solange die
angefragte ID im Bundle-Scope liegt.

## Nicht-HTTP

`hostus bundle` ist ein reiner CLI-Exportbefehl, kein HTTP-Endpunkt — er
erscheint daher nicht in der [OpenAPI-Spezifikation](../reference/http-api.md).

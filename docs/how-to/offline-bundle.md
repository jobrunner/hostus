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
hostus bundle --db hostus.sqlite --area AUT --out bundle.sqlite [--snapshot v1]
```

| Flag                          | Pflicht | Beschreibung                                                                 |
|-------------------------------|---------|-------------------------------------------------------------------------------|
| `--db`                        | ja      | Pfad zur Quell-SQLite-Datenbank (bereits ingestiert).                          |
| `--out`                       | ja      | Zielpfad für die neu erzeugte Bundle-Datei.                                    |
| `--area`                      | nein    | WGSRPD-L3-Referenzgebietscode (z. B. `AUT`) oder Kurzform (z. B. `DE`). Leer = gesamte Datenbank, ungescopt. |
| `--snapshot`                  | nein    | Freitext-Versionskennung, wird unverändert in `bundle_meta.snapshot_version` geschrieben. |
| `--force-include-restricted`  | nein    | Übersteuert das Redistribution-Gate (siehe unten) — nur explizit setzen, wenn die Weitergabe der genannten Quelle(n) bewusst in Kauf genommen wird. |

**Beispiel-Ausgabe**

```
Bundle complete: bundle.sqlite (concepts=3 names=13 areas=16)
```

## Redistribution-Gate: ein Bundle kann keine ungeklärte Quelle mitführen

Jeder Backbone- und Trait-Vokabular-Eintrag im Manifest trägt ein
Pflichtfeld `redistribution: allowed|restricted|unknown` (siehe
[Merkmalswerte pipeln und ingestieren](trait-ingest.md) für die volle
Erklärung). `hostus bundle` prüft vor jedem Export, welche Quellen
tatsächlich Daten zum gewählten Scope (`--area` oder die ganze Datenbank)
beitragen:

- **Trägt eine nicht-`allowed`-Quelle bei** (z. B. ein Trait-Vokabular mit
  `redistribution: unknown`), **schlägt der Export standardmäßig fehl** —
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
- ein neu aufgebauter `fts_name`/`fts_name_map`-FTS5-Index (nicht kopiert,
  sondern aus den kopierten Zeilen neu erzeugt — ein Bundle ist also selbst
  über `GET /v1/suggest` abfragbar)
- eine `bundle_meta`-Provenienz-Zeile (`snapshot_version`, `area`,
  `created_at`, `source_manifest_sha`, `restricted_sources` — siehe unten,
  "Redistribution-Gate")

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

# Merkmalswerte (Traits) pipeln und ingestieren

hostus liefert für jedes Concept ökologische Zeigerwerte (Merkmale/Traits)
aus drei unabhängigen Vokabularen: **EIVE**, **Tichý et al. 2023** und
**Midolo et al. 2023**. Diese Anleitung beschreibt den kompletten Weg von
den rohen Zenodo-Downloads bis zum servierten `GET /v1/concept/{id}/traits`
— und, ebenso wichtig, welche Quellen dabei **bewusst nicht** ingestiert
werden und warum.

## Überblick: Pipeline → kanonisches CSV → Ingest

```
xlsx/csv (Zenodo)  →  pipelines/<vocab>/build.sh  →  kanonisches CSV  →  hostus ingest
```

Die drei Pipelines (`pipelines/eive/build.sh`, `pipelines/tichy/build.sh`,
`pipelines/midolo/build.sh`) laufen **offline, vorab, außerhalb von
hostus** — der Dienst selbst liest zur Laufzeit nie xlsx-Dateien oder das
Netzwerk, sondern ausschließlich das von den Pipelines erzeugte
pipe-delimited kanonische CSV-Format (`taxon|vocab|vocab_version|dim|value
|niche_width|n_systems`). Details zum Format, den Pipelines selbst und den
gemessenen Wertebereichen je Dimension stehen in `pipelines/README.md`.

### 1. Pipeline laufen lassen

```bash
nix develop -c bash pipelines/eive/build.sh
nix develop -c bash pipelines/tichy/build.sh
nix develop -c bash pipelines/midolo/build.sh
```

Jede Pipeline lädt (oder nutzt einen bereits vorhandenen Cache), konvertiert
mit `python3`+`openpyxl` und schreibt das Ergebnis nach
`pipelines/<vocab>/output/<vocab>-canonical.csv` (generiertes Artefakt,
nicht eingecheckt) sowie eine Zusammenfassung (Zeilen, Taxa, Dimensionen,
beobachtete Min/Max je Dimension) auf stdout und in
`pipelines/<vocab>/<vocab>.summary.txt`.

### 2. Vokabular im Manifest pinnen

Jedes Vokabular wird — wie ein Backbone — im `dataset.yaml`-Manifest
gepinnt (`trait_vocabularies:`):

```yaml
trait_vocabularies:
  - id: eive
    version: "1.0"
    taxonomy: euromed-via-eurosl
    license: CC-BY-4.0
    source: https://doi.org/10.5281/zenodo.7534792
    path: pipelines/eive/output/eive-canonical.csv
  - id: tichy2023
    version: "2.0"
    taxonomy: floraveg-eunis-aligned
    license: CC-BY-4.0
    source: https://doi.org/10.5281/zenodo.7427088
    path: pipelines/tichy/output/tichy2023-canonical.csv
```

`id` muss einer der bekannten Vokabular-Bezeichner sein (`eive`,
`tichy2023`, `midolo2023` — siehe `internal/domain.ParseTraitVocab`);
`taxonomy` dokumentiert den Namensraum, gegen den das Vokabular
harmonisiert wurde (siehe unten, "Namensraum-Kreuzung").

### 3. Ingestieren

```bash
hostus ingest --dataset dataset.yaml --db hostus.sqlite
```

`hostus ingest` liest zuerst alle `backbones:`-Einträge, dann alle
`trait_vocabularies:`-Einträge. Für jedes Vokabular wird am Ende eine
Zeile ausgegeben:

```
Trait vocabularies:
  eive: rows=25 matched=15 unmatched=10 ambiguous=0
    unmatched sample: Abies alba, Quercus robur
  tichy2023: rows=25 matched=16 unmatched=9 ambiguous=0
    unmatched sample: Abies alba, Quercus robur
```

(Werte aus dem checked-in Fixture-Datensatz, `internal/adapters/traits/testdata/`.)

### 4. Servieren und abfragen

```bash
hostus serve
curl "http://localhost:8080/v1/concept/wcvp:concept:405825/traits"
```

Details zur Response-Struktur (pro-Vokabular gruppiert, pro-Wert-Skala)
stehen in [HTTP-API](../reference/http-api.md#trait-endpunkt).

## Namensraum-Kreuzung ist verlustbehaftet — und das wird sichtbar gemacht

Die Trait-Tabellen (EIVE/Tichý/Midolo) verwenden **keine** WCVP-/POWO-IDs,
sondern reine Taxon-Namensstrings. Der Ingest gleicht jeden Taxon-String
gegen den bereits ingestierten Backbone-Index ab (exakter kanonischer
Namensvergleich). Das ist verlustbehaftet by construction:

- **matched**: Der Name wurde exakt einem Concept zugeordnet — der
  Merkmalswert wird geschrieben.
- **unmatched**: Der Name kommt im ingestierten Backbone (z. B. dem
  WCVP-Ausschnitt) nicht vor — der Merkmalswert wird **verworfen**, nicht
  geraten.
- **ambiguous**: Der Name passt auf mehr als ein Concept — ebenfalls
  verworfen statt geraten.

Diese Verluste werden von `hostus ingest` **immer** ausgegeben (Zeilen-,
Treffer- und eine Stichprobe der nicht zugeordneten Namen), nie
stillschweigend verschluckt — siehe oben, Schritt 3.

## Attribution (Pflicht — alle drei Vokabulare sind CC-BY-4.0)

EIVE, Tichý et al. 2023 und Midolo et al. 2023 sind auf Zenodo unter
**CC-BY-4.0** veröffentlicht. Jedes Produkt, das aus diesen Pipelines
abgeleitete Daten ausliefert (also auch `hostus serve` selbst), muss die
folgenden Quellen nennen:

- **EIVE 1.0**: Dengler, J. et al. (2023). *Ecological Indicator Values
  for Europe (EIVE) 1.0.* Vegetation Classification and Survey 4: 7–29.
  [doi.org/10.3897/VCS.98324](https://doi.org/10.3897/VCS.98324) — Daten:
  Zenodo [10.5281/zenodo.7534792](https://doi.org/10.5281/zenodo.7534792)
- **Tichý et al. 2023**: Tichý, L. et al. (2023). *Ellenberg-type
  indicator values for European vascular plant species.* Journal of
  Vegetation Science 34: e13168.
  [doi.org/10.1111/jvs.13168](https://doi.org/10.1111/jvs.13168) — Daten:
  Zenodo [10.5281/zenodo.7427088](https://doi.org/10.5281/zenodo.7427088)
  (v2.0)
- **Midolo et al. 2023**: Midolo, G. et al. (2023). *Disturbance
  indicator values for European plants.* Global Ecology and Biogeography
  32(1): 24–34. [doi.org/10.1111/geb.13603](https://doi.org/10.1111/geb.13603)
  — Daten: Zenodo
  [10.5281/zenodo.7116957](https://doi.org/10.5281/zenodo.7116957) (v3)

Wer ein Bundle exportiert (`hostus bundle`, siehe
[Offline-Bundle exportieren](offline-bundle.md)) oder die API weiterverwendet,
muss diese Attribution mitführen — sie ist Lizenzbedingung, keine
Empfehlung.

## Bewusster Scope-Schnitt: was NICHT ingestiert wird

Vier Quellen, die in der ursprünglichen Architektur-Recherche für SP3
vorgesehen waren, werden **nicht** ingestiert, weil sich für keine davon
eine belastbare Lizenz zur Weiterverbreitung feststellen ließ (siehe
`docs/research/quellenregister.md`, Abschnitt "Blocker", für die
vollständige Recherche):

| Quelle | Warum ausgeschlossen |
|---|---|
| **Euro+Med PlantBase** | Kein Bulk-/API-Export mit erkennbarer Lizenz auffindbar; weder auf `europlusmed.org` noch über GBIF/ChecklistBank. |
| **GermanSL** | Keine Lizenz-/Copyright-Angabe auf Downloadseite, Startseite, README oder Newsletter — nur eine Zitierempfehlung, keine Nutzungsbedingung. |
| **EuroSL** | Ebenfalls keine Lizenzangabe; bündelt zusätzlich Euro+Med-Daten mit ungeklärter Lizenz weiter (verstärktes Risiko). |
| **FloraVeg.EU-Downloads** (Vegetationseinheiten, Zeigerwert-Tabellen o. ä. — **nicht** zu verwechseln mit EUNIS-ESy, das separat CC-BY-4.0 lizenziert ist) | Keine Lizenzangabe je Downloaddatei; ein Hinweis auf CC BY-NC aus der zugehörigen Publikation (Chytrý et al. 2024) konnte nicht direkt verifiziert werden. |

Das ist ein **Proof-of-Concept-Scope-Schnitt (R1)**, keine technische
Einschränkung: der taxonomische Anschluss für EIVE/Tichý/Midolo läuft
deshalb direkt gegen den ingestierten WCVP/POWO-Backbone (Namensvergleich,
siehe oben), nicht über eine der vier genannten Quellen als
Vermittlungsschicht. Wer eine dieser vier Quellen produktiv einbinden
möchte, muss zuerst eine schriftliche Lizenzklärung mit dem jeweiligen
Betreiber einholen (Kontaktpunkte im Quellenregister dokumentiert) — bis
dahin bleibt der reduzierte Scope bestehen, und das ist hier absichtlich
sichtbar dokumentiert statt stillschweigend übergangen.

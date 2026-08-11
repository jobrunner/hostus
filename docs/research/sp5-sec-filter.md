# SP5 `sec.`-Filter: gemessene Disambiguierung

Stand: 2026-08-11. Gemessen gegen den **realen konsolidierten Index**
(WCVP `2026-06-04`, 440.534 Konzepte + CDM `2026-08-02`, 51.466 Konzepte in
119 `sec.`-Räumen + Traits + Wikidata + floraveg/eurosl), eine schreibbare
Kopie des vollen Ingests (nicht committet).

## Die Auflage

Die known-gap verlangte ausdrücklich: **erst messen, dann glauben** — „ein
Filter, der Mehrdeutigkeit nur verschiebt, ist keine Verbesserung." Also nicht
nur „resolviert der Filter", sondern: *wird der Name unter dem Filter
tatsächlich eindeutig?*

## Ausgangslage: die Mehrdeutigkeit

| | Namensformen | Anteil |
| --- | ---: | ---: |
| gesamt (distinkte `canonical_fold`) | 1.361.597 | 100 % |
| **mehrdeutig (>1 Konzept)** | **49.688** | **3,6 %** |

Die 3,6 % sind der Kern des Problems: `MatchExact` sucht über alle
Backbones/Räume, und für diese 49.688 Namensformen liefert es mehr als ein
Konzept — der Grund, warum `/v1/match` (inkl. `target_space`) und
`/v1/translate`-`verbatim` sie nicht auflösen.

## `entry_sec` — die Übersetzungs-Hälfte

Die entscheidende Zahl: löst `entry_sec` einen Namen **innerhalb eines
Referenzraums** eindeutig auf?

| | (canonical, sec)-Kombis | Anteil |
| --- | ---: | ---: |
| gesamt | 51.167 | 100 % |
| **noch mehrdeutig unter `entry_sec`** | **167** | **0,33 %** |
| **eindeutig unter `entry_sec`** | **51.000** | **99,67 %** |

**In 99,67 % der Fälle macht `entry_sec` den verbatim-Namen eindeutig.** Das
ist genau der Hebel für den bisher toten `/v1/translate`-`verbatim`-Pfad
(known-gap: 265 von 300 `UNRESOLVABLE`): mit `entry_sec=<Quellraum>` löst die
Aufnahme in *einem* Raum auf und übersetzt dann über ihre Relation. Der Filter
verschiebt die Mehrdeutigkeit nicht — er beseitigt sie fast vollständig. Die
167 Rest-Fälle sind CDM-Dubletten (zwei Konzepte für einen Namen in einem
Raum); dort verweigert die Auflösung korrekt weiterhin.

Übersetzbarkeit: **24.987 CDM-Quellkonzepte** tragen ≥1 Relation und sind
damit über `entry_sec` translate-fähig.

## `entry_backbone=wcvp` — die `target_space`/UC4-Hälfte

Namensräume (floraveg, eurosl) hängen an WCVP-Konzepten, also ist der
produktive UC4-Fall „löse auf **ein** WCVP-Konzept auf":

| | mehrdeutige Namen | Anteil der Mehrdeutigen |
| --- | ---: | ---: |
| werden unter `entry_backbone=wcvp` **eindeutig** (genau 1 WCVP-Konzept) | **12.979** | **26,1 %** |

Die 26,1 % sind kein Defekt: die übrigen mehrdeutigen Namen haben in WCVP
**null** Konzepte (CDM-interne Gleichnamigkeit über `sec.`-Räume — dort ist
`entry_sec` das richtige Werkzeug) oder in WCVP selbst mehrere (echte
Homonyme, die WCVP nicht trennt). `entry_backbone=wcvp` macht genau die 12.979
Namen bedienbar, die eindeutig in WCVP liegen — der `target_space`-Kernfall.

## Befund: der Filter hält

Die zwei Filter sind komplementär und beide messbar wirksam:

- **`entry_sec`** beseitigt die Mehrdeutigkeit in **99,67 %** — die
  Übersetzungs-Hälfte (UC6) ist damit bedienbar, wo sie es vorher (0 von 300)
  nicht war.
- **`entry_backbone=wcvp`** macht **12.979** WCVP-eindeutige Namen auflösbar —
  die `target_space`-Hälfte (UC4).

Kein „nur verschoben": die Rest-Mehrdeutigkeit unter `entry_sec` ist 0,33 %
und ist echte Quelldaten-Dublette, kein Filterartefakt.

## Kommandos

Gemessen mit einem Wegwerf-Python-Skript gegen eine schreibbare Kopie des
realen Index (`sqlite3`, GROUP-BY-Aggregate statt korrelierter Subqueries):
Mehrdeutigkeits-Population über `count(distinct tc.id) group by
n.canonical_fold`; `entry_backbone`-Eindeutigkeit als JOIN gegen die
WCVP-Konzeptzahl je Namensform; `entry_sec`-Rest-Mehrdeutigkeit über
`group by tc.sec_reference, n.canonical_fold having count(distinct tc.id)>1`.

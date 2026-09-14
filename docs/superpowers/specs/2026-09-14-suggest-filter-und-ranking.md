# Spec: Suggest-Filter, die filtern — und ein Ranking, das den gesuchten Treffer nach oben bringt

Datum: 2026-09-14
Status: verabschiedet (User: „Die Filter im frontend im suggest Panel
funktionieren nicht. … Ziel ist immer eine möglichst eingegrenzte Art auf
eurosl (eine!)" → Befund vorgelegt → „Zusätzlich das ranking Thema")

## Befund (gemessen gegen v3.4.0-alpha.0, Produktions-DB)

Von den drei Bedienelementen im Suggest-Panel filtert **eines**:

| Element | Verhalten heute | Messung |
|---|---|---|
| `Konzeptraum (entry_backbone)` | echter Filter (`WHERE`) | `q=Inula hirta`: 9 → 2 Treffer |
| `Namensraum (target_space)` | **nur Anreicherung** | 9 Treffer, davon 2 mit eurosl-Namen; die 7 CDM-Konzepte ohne jeden eurosl-Eintrag bleiben stehen |
| `Gebiet (area)` | **nur Ranking-Signal** | `area=NZL` liefert *Pentanema hirtum* weiter, obwohl dafür null NZL-Verbreitungszeilen existieren |

Weitere Defekte:

1. **Ungültiges Gebiet wird stillschweigend verschluckt.** `areaCodes`
   reicht jeden unbekannten Wert großgeschrieben durch; `area=QUATSCH`
   liefert alle Treffer ohne Fehler. Der Nutzer glaubt zu filtern.
2. **`rank` und `match_mode` kennt der Endpunkt, die Konsole bietet sie
   nicht an.**
3. **Der exakte Treffer steht nicht oben.** Bei vollständig getipptem
   „Inula hirta" führt *Pentanema britannica* die Liste an, *Pentanema
   hirtum* steht auf Platz 2.
4. **`PrefixHit` ist eine Konstante.** `scanSuggestItem` setzt
   `item.PrefixHit = true` hart. Das erste und wichtigste Kriterium von
   `domain.RankSuggestions` kann damit **nie** unterscheiden, und die
   Konsolenspalte „Präfix" zeigt in jeder Zeile dasselbe.
5. **Die Antwort nennt den Treffer-Namen nicht.** Zu sehen ist nur der
   akzeptierte Name des Konzepts.

### Warum Punkt 3 und 5 zusammengehören: es ist ein Homonym

Ursache ist kein Ranking-Zufall, sondern Nomenklatur:

```
Inula hirta L.       → Synonym von Pentanema hirtum      (wcvp:concept:3217689)
Inula hirta Pollich  → Synonym von Pentanema britannica  (wcvp:concept:3217682)
```

**Beide Treffer sind fachlich richtig.** Die Konsole zeigt dem Nutzer aber
zwei Zeilen „Pentanema britannica" / „Pentanema hirtum" ohne jeden Hinweis,
welcher getippte Name zu welcher Zeile geführt hat — die Autorschaft, die
das Homonym allein auflöst, steht nirgends. Ein Ranking kann hier nicht
„die eine richtige" Art erraten; es kann aber die für den erklärten
Zweck relevante nach oben bringen, und die Oberfläche muss die
Mehrdeutigkeit sichtbar machen, statt sie zu verstecken.

## Entscheidungen

1. **`target_space` filtert auf Wunsch wirklich.** Neuer Query-Parameter
   `require_target_space=true` (Default `false`). Ist er gesetzt UND ein
   `target_space` angegeben, fallen Konzepte ohne Eintrag in diesem Raum
   heraus. Ohne den Parameter bleibt die Antwort byte-identisch zu heute —
   ein bestehender Client merkt nichts. `require_target_space=true` ohne
   `target_space` ist ein `400 INVALID_QUERY` (sonst filtert der Parameter
   stumm nichts).
2. **Ein unbekanntes Gebiet ist ein Fehler, keine stille Nicht-Wirkung.**
   `area` wird gegen die bekannten WGSRPD-L3-Codes und die dokumentierten
   Aliase geprüft; ein unbekannter Wert liefert `400 INVALID_QUERY` mit dem
   Wert im Text. Das ist eine bewusste Verhaltensänderung: bisher lieferte
   ein Tippfehler ungefiltert Treffer.
3. **`PrefixHit` wird berechnet statt behauptet.** Die Query liefert eine
   echte Spalte (`canonical_fold LIKE prefix || '%'` über die Namen des
   Konzepts). Im Default-Modus `name_start` ist sie zwangsläufig für jede
   Zeile wahr — dort filtert bereits der `WHERE`. Bedeutung bekommt sie in
   `match_mode=anywhere`, wo Treffer über einen Tokentreffer irgendwo im
   Namen hereinkommen. Damit wird Kriterium 1 von `RankSuggestions` zum
   ersten Mal wirksam.
4. **Zwei neue Ranking-Kriterien, ganz oben eingehängt.** Neue Reihenfolge
   in `domain.RankSuggestions`:

   1. **`ExactHit`** — das Konzept trägt einen Namen, dessen kanonisierte
      Form der kanonisierten Anfrage **gleicht** (nicht nur mit ihr
      beginnt). Wer den vollen Namen tippt, will keinen längeren.
   2. **`TargetSpaceHit`** — nur wenn ein `target_space` angefragt ist: der
      Name des Konzepts **in diesem Raum** trifft die Anfrage (gleich, sonst
      Präfix). Das ist der entscheidende Hebel für den erklärten Zweck: bei
      „Inula hirta" trägt *P. hirtum* in eurosl exakt diesen Namen, *P.
      britannica* dort „Inula britannica" — die Anfrage traf bei ihm nur ein
      WCVP-Synonym.
   3. `PrefixHit` (ab jetzt echt, siehe 3)
   4. `InArea` — unverändert
   5. Status `accepted` — unverändert
   6. Rangordnung — unverändert
   7. Score aufsteigend — unverändert

   Für „Inula hirta" ergibt das: Kriterium 1 unentschieden (beide tragen
   den Namen exakt), Kriterium 2 entscheidet für *Pentanema hirtum*.
5. **Die Antwort nennt den Namen, der getroffen hat.** Neues Feld
   `matched_name` mit `canonical`, `authorship` und `role`
   (`accepted`/`synonym`), gefüllt aus dem Namen, der den Treffer
   ausgelöst hat; bei mehreren gewinnt derselbe Vorrang wie im Ranking
   (exakt vor Präfix, accepted vor synonym). Das Feld ist `omitempty` —
   additive Erweiterung. Erst damit ist das Homonym auf dem Schirm:
   „Inula hirta **L.**" gegenüber „Inula hirta **Pollich**".
6. **Konsole.** Das Suggest-Panel bekommt: ein `Rang`-Auswahlfeld
   (`(alle)`, SPECIES, SUBSPECIES, VARIETY, GENUS, FAMILY → `rank`), eine
   Checkbox „nur mit Eintrag im Namensraum" (→ `require_target_space`), den
   Konzeptraum **auf `wcvp` vorbelegt** (entfernt die CDM-Dubletten sofort
   und ist der Raum, den die Kette PlantNet→Habitatus ohnehin nutzt) und
   eine Spalte **„Treffer-Name"**, die `matched_name` samt Autorschaft
   zeigt. Die Spalte „Präfix" bleibt, weil sie jetzt etwas aussagt.
7. **Nicht geändert:** `target_space` bleibt ohne den neuen Parameter reine
   Anreicherung; `area` bleibt auch künftig ein Ranking-Signal und kein
   Filter (ein Gebiets-Filter würde Neophyten und Kulturpflanzen
   unterschlagen, die in der Aufnahme trotzdem vorkommen); der fehlende
   `sec`-Filter des Serving-Pfads bleibt offen (Audit-Befund, eigener
   Zuschnitt) — die wcvp-Vorbelegung entschärft ihn praktisch.

## Zielbild, messbar

Für `q=Inula hirta`, Konzeptraum `wcvp`, Namensraum `eurosl`,
„nur mit Eintrag" an, Rang `SPECIES`:

- genau **2** Zeilen (beide fachlich korrekt, Homonym),
- **erste Zeile** ist *Pentanema hirtum* mit `target_space_name: "Inula hirta"`,
- die Spalte „Treffer-Name" zeigt `Inula hirta L.` gegenüber
  `Inula hirta Pollich`, sodass die Wahl begründbar ist.

Ohne `require_target_space` und ohne Konzeptraum-Vorbelegung waren es 9
Zeilen mit der falschen zuerst.

## Projektweite Anforderungen

- Mutation-gated: `internal/domain`, `internal/adapters/http`, `cmd/hostus`
  (CI-Matrix) — `Not covered: 0`. `internal/adapters/sqlite` ist in CI nur
  report-only; das **lokale** `make mutation PKG=./internal/adapters/sqlite`
  ist dort das echte Gate und muss vor dem Merge laufen.
- Jede Ranking-Änderung braucht einen Test, der die ALTE Reihenfolge
  fehlschlagen lässt (Kontroll-Assertion), sonst pinnt er nichts.
- Antworten ohne die neuen Parameter bleiben byte-identisch, abgesehen vom
  additiven `matched_name`.
- Neue SQL-Ausdrücke dürfen die Suggest-Latenz nicht wieder in die
  Planner-Falle führen (`+spalte`-Idiom, EXPLAIN-Regressionstests —
  siehe `suggest_plan_internal_test.go`); Laufzeit gegen die
  Produktions-DB vorher/nachher messen und im Report nennen.
- OpenAPI codegeneriert; CHANGELOG unter `## [Unreleased]`; Conventional
  Commits; englische WHY-Kommentare mit den Messwerten dieser Spec.

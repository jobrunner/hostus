# SP9 — Spike: Ist das ESy-Regelwerk beschaffbar und für `esy_diagnostic_relevance` nutzbar?

Stand: 2026-08-12. Sondierung (kein Implementierungs-Task). Grundlage: der
Zenodo-Record des EUNIS-ESy-Expertensystems, die formale ESy-Sprache in der
Regeldatei selbst, und eine gemessene Namensüberlappung gegen den bereits
ingestierten FloraVeg-Namensraum. Anlass:
[known-gaps „Das ESy-Regelwerk ist nicht geerntet"](../explanation/known-gaps.md)
und [SP9/UC4-Verdikt](sp9-uc4-verdict.md), Auflage 3.

## Die Frage

`esy_diagnostic_relevance` (UC4) ist auf dem `target_space`-Pfad heute immer
`not_determinable`, weil das ESy-Regelwerk fehlt. Der known-gap nannte drei zu
klärende Punkte: (a) ist das Regelwerk maschinell beschaffbar und
lizenzrechtlich unbedenklich? (b) ist es maschinell in einen Regelvertrag
überführbar? (c) gegen welchen Namensraum sind die Regeln geschlüsselt, und
fällt dieser mit dem ingestierten FloraVeg-Namensraum zusammen?

## Befund (a): beschaffbar und CC BY 4.0 — bestätigt

Das Expertensystem liegt auf **Zenodo, DOI
[10.5281/zenodo.3841729](https://zenodo.org/records/3841729)**, Lizenz
**Creative Commons Attribution 4.0 International (CC BY 4.0)** — anders als die
floraveg.eu-Downloads (`redistribution: unknown`) also **redistribuierbar**.
Der Record trägt u. a.:

| Datei | Format | Größe | Rolle |
|---|---|---|---|
| `EUNIS-ESy-2020-06-08.txt` | TXT | 1,6 MB | **das Regelwerk** (die formale Sprache) |
| `EUNIS-ESy-User-Guide.pdf` | PDF | 1,0 MB | Syntax- und Betriebsanleitung |
| `Characteristic-species-combinations.xlsx` | XLSX | 0,4 MB | Habitat-Charakterarten |
| `Nomenclature-translation-from-Turboveg-2-databases.zip` | ZIP | 5,5 MB | Namens-Übersetzungstabellen |

Ein einzelner, direkt per HTTP ladbarer Textfile — maschinelle Beschaffung ist
trivial (kein Crawl, kein Login).

## Befund (b): maschinell parsbar — ja, formale Grammatik mit R-Referenz

`EUNIS-ESy-2020-06-08.txt` ist keine Prosa, sondern eine strukturierte
Formalsprache in vier Abschnitten:

1. **SECTION 1: Species aggregation** — jede Art mit ihren Unterarten/Synonymen,
   die auf sie zusammengezogen werden (Aggregat → Varianten).
2. **SECTION 2: Species groups** — **169** benannte funktionale Artengruppen
   (`### Alpine-acidophilous-herbs` gefolgt von einer Artenliste).
3. **SECTION 3: Group definitions** — **304** Habitat-Membership-Regeln als
   logische Formeln. Beispiel (gekürzt):
   ```
   ((<#TC +01 N11-...-sand-beach GR #T$> AND <##Q +01 N11-...-sand-beach>)
     NOT <#TC Trees|#TC Shrubs GR 15>)
   AND ((<$$C Coast_EEA EQ ATL_COAST> OR <$$C Coast_EEA EQ BAL_COAST>) NOT ...)
   ```
   Operatoren `AND`/`OR`/`NOT`, Gruppenreferenzen (`#TC`, `##Q`, `#02`),
   Standortprädikate (`$$C`, `$$N`), Deckungsschwellen (`GR 15`).
4. **SECTION 4: Similarity**.

Die Grammatik ist bereits in R implementiert (Bruelheide et al. 2021, *Applied
Vegetation Science*, „Implementing the formal language of the vegetation
classification expert systems (ESy) in R") — ein maschineller Parser existiert
also als Referenz. Ein Überführen in einen hostus-Regelvertrag ist damit ein
Parser-Task, kein Reverse-Engineering.

## Befund (c): Namensraum — gemessen, fällt weitgehend mit FloraVeg zusammen

Die Artspezifizierer sind **schlichte Binomiale ohne Autorschaft** (z. B.
`Achillea millefolium`, `Festuca eskia`) — genau die Form, die hostus' SP3-
Crosswalk (kanonischer Name → Konzept) schon verarbeitet. Gemessene
Überlappung der aus SECTION 1+2 extrahierten Namen gegen den ingestierten
FloraVeg-Namensraum (`pipelines/floraveg/output/floraveg-canonical.csv`):

| Menge | Zahl |
|---|---|
| distinkte ESy-Namen (Section 1+2) | 20.024 |
| davon Art-Ebene (2-Token-Binomiale) | 11.850 |
| FloraVeg-Namen (Art-Ebene) | 15.677 |
| **verbatim in FloraVeg vorhanden (Art-Ebene)** | **7.865 = 66,4 %** |

Zwei Einschränkungen dieser Zahl, damit sie nicht überlesen wird:

- **Es ist ein exakter Verbatim-Vergleich und damit ein *Untergrenze*.** Groß-/
  Kleinschreibung, Hybrid-Marker (`×`), `aggr.`/`s.l.`-Zusätze und einseitig auf
  die Art zusammengezogene Unterarten zählen hier als Nicht-Treffer, obwohl sie
  taxonomisch dieselbe Art meinen. Die *echte* nomenklatorische Überlappung
  liegt höher; um wie viel, ist nicht gemessen.
- **FloraVeg-Name ≠ hostus-Konzept.** Die 66,4 % sind Überlappung mit dem
  FloraVeg-*Namensraum*, nicht mit auflösbaren Backbone-Konzepten. FloraVeg
  selbst bildet nur 85,7 % seiner Namen auf ein WCVP-Konzept ab
  ([Messung](floraveg-namespace.md)); die End-to-End-Kette ESy-Name → verbatim
  FloraVeg → WCVP-Konzept liegt also bei ≈ 0,664 × 0,857 ≈ **57 %** (ebenfalls
  ein Boden). Die tatsächliche Zahl misst erst der Ingest-SP über den vollen
  SP3-Crosswalk.

Die ~34 % Verbatim-Nicht-Treffer sind **nicht klassifiziert** — der exakte
Vergleich sagt nichts über ihre Zusammensetzung. Qualitativ (Stichprobe, keine
Auszählung) enthalten sie Moose/Flechten (`Abietinella abietina`,
`Absconditella sphagnorum` — FloraVegs `Life_form.xlsx` ist Gefäßpflanzen),
Platzhalter-Aggregate (`Abies species`), außereuropäische Ziergehölze (`Abelia
coreana`, `Acacia karoo`) und Schreibvarianten (`venestum`/`venustum`). Wie
viele davon der SP3-Crosswalk auffinge, ist **ungetestet** und gehört in den
Ingest-SP gemessen, nicht hier geschätzt. Belegt ist nur: ESy und der
FloraVeg-Namensraum stammen beide aus dem floraveg.eu-Ökosystem und teilen die
Binomial-Nomenklatur (schlichte Namen ohne Autorschaft), weshalb der
Crosswalk-Pfad derselbe ist wie bei FloraVeg — nicht, mit welcher Trefferquote.

## Die Scope-Grenze (der eigentliche Ertrag der Sondierung)

**Eine volle ESy-Klassifikation (Aufnahme → EUNIS-Habitat) ist für einen
Namensdienst systembedingt unmöglich, nicht bloß unimplementiert.** Von den 304
Regeln nutzen **alle 304** Deckungsschwellen (`GR <n>`, z. B. „>15 % Deckung")
und **168** Standortprädikate (`$$` Küste/Koordinaten/Land). Beides sind
Aufnahme-Metadaten — Deckungswerte je Art, Plotkoordinaten —, die hostus
konstruktionsbedingt nicht hat: hostus kennt einen *Namen*, keine *Aufnahme*.

Die von UC4 tatsächlich gestellte Frage ist aber enger und **beantwortbar**:
„ist dieser Name im ESy-Regelwerk eine **diagnostische Art**?" — d. h. taucht
er (direkt oder über eine in SECTION 2 definierte Artengruppe) in mindestens
einer Habitat-Regel auf? Das entscheidet allein das Regelwerk, ohne
Aufnahmedaten. Genau das ist der Sinn des Sentinels `not_determinable` heute:
er hält die Antwort offen, statt Abwesenheit als „nicht relevant" zu lesen.

## Verdikt und Umsetzungspfad

Alle drei known-gap-Fragen sind beantwortet: **beschaffbar (CC BY 4.0),
maschinell parsbar (formale Grammatik + R-Referenz), namensraum-kompatibel
(66,4 % verbatim, Rest größtenteils Scope/Schreibvariante)**. Der Blocker war
Datenbeschaffung + Machbarkeit — beides ist ausgeräumt.

Baubarer Pfad für ein echtes `esy_diagnostic_relevance` (eigener SP, nicht
dieser Spike):

1. `EUNIS-ESy-2020-06-08.txt` als **eigene, CC-BY-4.0-`redistribution: allowed`
   Quelle** ins Manifest pinnen (Zenodo-DOI als `source`).
2. Parser für SECTION 1–3 (Aggregation, Gruppen, Regeln) — Referenz: Bruelheide
   et al. 2021.
3. Je ESy-Art über den bestehenden SP3-Crosswalk auf hostus-Konzepte abbilden;
   die 66,4 % verbatim + Crosswalk-Rest messen (wie beim FloraVeg-Ingest).
4. `esy_diagnostic_relevance` als **dreiwertig** definieren, spiegelbildlich zu
   `aggregate_policy`:
   - `diagnostic` — der Name ist in ≥1 Habitat-Regel (SECTION 3) ein
     Spezifizierer, **direkt** genannt ODER über eine von einer Regel benutzte
     Artengruppe (SECTION 2). Beide Pfade zählen — Regeln referenzieren teils
     Gruppen (`#TC …`), teils Arten direkt (`<Ulex europaeus GR 25>`).
   - `not_diagnostic` — der Name ist auf eine ESy-Art abbildbar, taucht aber in
     **keiner** Habitat-Regel als Spezifizierer auf (weder direkt noch via
     Gruppe).
   - `not_determinable` — der Name lässt sich nicht auf eine ESy-Art abbilden
     (nicht im Crosswalk).

   Die volle Plot-Klassifikation bleibt bewusst **außerhalb** des
   Namensdienstes.

Bis dieser SP läuft, bleibt das Feld korrekt `not_determinable` (siehe
[SP9/UC4-Verdikt](sp9-uc4-verdict.md)).

## Reproduktion

```bash
curl -sSL -A "<honest-ua>" \
  https://zenodo.org/records/3841729/files/EUNIS-ESy-2020-06-08.txt -o esy.txt
# Abschnitte: grep -n '^SECTION' esy.txt  (Section 1+2 = Zeilen 1..39980)

# ESy-Namen aus Section 1+2: trailing "- N"/Zahl entfernen, trimmen,
# nur Binomial-artige Zeilen, SECTION/### ausschließen.
awk 'NR<=39980' esy.txt \
  | sed -E 's/[[:space:]]*-?[[:space:]]*[0-9]+[[:space:]]*$//' \
  | sed -E 's/^[[:space:]]+//; s/[[:space:]]+$//' \
  | grep -E '^[A-Z][a-z]+ [a-z]' | grep -vE '^SECTION|^###' | sort -u > esy-names.txt

# Auf Art-Ebene (genau 2 Token) beschränken, beide Seiten gleich:
awk 'NF==2 && $2 ~ /^[a-z][a-z-]+$/' esy-names.txt | sort -u > esy-species.txt
tail -n +2 pipelines/floraveg/output/floraveg-canonical.csv \
  | cut -d'|' -f1 | awk 'NF==2 && $2 ~ /^[a-z][a-z-]+$/' | sort -u > fv-species.txt

comm -12 esy-species.txt fv-species.txt | wc -l   # -> 7865
# 7865 / 11850 (wc -l esy-species.txt) = 66,4 %  (Stand 2026-08-12)
```
(Die Regeldatei wird **nicht** ins Repo eingecheckt — sie ist eine externe,
per DOI gepinnte Quelle, kein Repo-Artefakt.)

# Publikationsfähige Synonyme filtern (UC5)

Ziel dieser Anleitung: „Dieses Konzept hat 26 Synonyme — welche zwei bis
drei davon gehören in meine Publikation?"

UC5 ist ausdrücklich **kein Beschaffungs-, sondern ein Filterproblem**. Die
Synonyme liegen längst im Index; `GET /v1/concept/{id}/synonyms` sortiert
und filtert sie und sagt dabei **pro Eintrag**, warum er drin oder draußen
ist.

!!! warning "Zuerst lesen: zwei der fünf UC5-Kriterien liefert hostus nicht"
    UC5 nennt fünf Relevanzkriterien. `relevance=publication` setzt zwei
    davon vollständig um, eines teilweise und **zwei überhaupt nicht** —
    insbesondere findet **keinerlei regionale Filterung** statt. Wer
    `relevance=publication` liest und annimmt, die Liste sei auf den
    eigenen Bezugsraum eingeschränkt, publiziert die falsche Synonymliste.
    Die Details stehen unten in
    [„Was dieser Filter nicht kann"](#was-dieser-filter-nicht-kann) — dieser
    Abschnitt ist Pflichtlektüre, bevor eine Antwort dieses Endpunkts in
    eine Veröffentlichung übernommen wird.

## Die fünf Kriterien und ihr tatsächlicher Stand

| UC5-Kriterium | Stand in SP6 |
| --- | --- |
| Nomenklatorischer Status | ✅ umgesetzt — aber `nom_status` ist nur auf **99.252 von 1.448.984 Namen (6,85 %)** belegt |
| Rang | ✅ umgesetzt — mit zwei benannten Lücken (SUBSPECIES, Nothotaxa) |
| Homotypisch vor heterotypisch | ⚠️ teilweise — faktisch ein Zwei-Wege-Split, `heterotypic` kommt im Index **nicht vor** |
| Im Bezugsraum verwendet | ❌ **nicht ausdrückbar** — kein Feld dafür im Schema |
| In Standardwerken verwendet (Rothmaler/Oberdorfer) | ❌ nicht verfügbar — Datenquelle vorhanden, aber lizenzoffen und unfertig |

## 1. Die ungefilterte Liste holen

Ohne `relevance` liefert der Endpunkt **alle** Synonyme — der Filter ist
Opt-in, nie Voreinstellung:

```bash
curl -sS 'http://localhost:8080/v1/concept/wcvp:concept:405825/synonyms' | jq '.summary'
```

```json
{
  "total": 26,
  "publishable": 6,
  "returned": 26,
  "truncated": 0,
  "absent": 6,
  "excluded": { "nom_status": 4, "rank": 16 },
  "unclassified_statuses": []
}
```

Auch hier trägt schon jeder Eintrag sein Urteil (`publishable`,
`exclusion`, `reason`) — die ungefilterte Liste ist also bereits die
vollständige Begründung dafür, was der Filter tun würde.

## 2. Filtern: das durchgerechnete Beispiel

*Corynephorus canescens* (`wcvp:concept:405825`), Publikation auf
Artniveau, höchstens drei Synonyme:

```bash
curl -sS 'http://localhost:8080/v1/concept/wcvp:concept:405825/synonyms?relevance=publication&rank=species&max=3'
```

Gegen den realen WCVP-Index (1.448.984 Namen / 440.534 Konzepte) sind das
diese drei, in dieser Reihenfolge:

| # | Name | Autor | Typisierung | Basionym |
| ---: | --- | --- | --- | --- |
| 1 | *Aira canescens* | L. | homotypic | ✅ |
| 2 | *Avena canescens* | (L.) Weber | homotypic | – |
| 3 | *Weingaertneria canescens* | (L.) Bernh. | homotypic | – |

*Aira canescens* L. (`wcvp:name:476481`) führt, weil es das **Basionym**
ist (UC5-Regel 4). `summary.truncated` ist `3`: publikationsfähig wären
sechs, `max=3` hat drei davon abgeschnitten — abgeschnitten ist **nicht**
dasselbe wie ausgeschlossen und wird deshalb getrennt gezählt.

Die 20 zurückgehaltenen Synonyme verteilen sich so:

- **4× `nom_status`** — darunter *Corynephorus incanescens* Bubani
  (`wcvp:name:405842`) mit dem Zellwert `", nom. illeg. superfl."`,
  außerdem *Aira triflora* (`", pro syn."`), *Aira variegata*
  (`", nom. illeg. superfl."`) und *Corynephorus canescens* var. *andinus*
  (`", nom. nud."`).
- **16× `rank`** — die VARIETY- und FORM-Synonyme, die auf Artniveau nicht
  publiziert werden.

Warum `rank: 16` und nicht 17, obwohl das Concept 17 infraspezifische
Synonyme hat: Ein Synonym wird mit **genau einem** Grund gezählt, dem zuerst
greifenden, und `nom_status` geht vor `rank`. Die Varietät *Corynephorus
canescens* var. *andinus* (`", nom. nud."`) steht deshalb oben unter
`nom_status`. `excluded.rank` ist also die Zahl der Synonyme, die *nur* am
Rang gescheitert sind.

Der Zellwert lautet `", nom. illeg. superfl."` und **nicht** das in der
UC5-Quelle angenommene `", nom. superfl."`. Genau deshalb matcht hostus per
Token-Containment und nie per Gleichheit; die vollständigen 36 Regeln stehen
mit Urteil und gemessener Trefferzahl in der
[HTTP-Referenz](../reference/http-api.md).

## 3. Ohne `rank` filtern

`rank` ist das Publikationsniveau des **Aufrufers**, nicht eine Eigenschaft
der Daten. Wer eine vollständige infraspezifische Behandlung schreibt,
lässt `rank` weg — dann greift **keine** Rangausschlussregel und nur der
nomenklatorische Status filtert:

```bash
curl -sS 'http://localhost:8080/v1/concept/wcvp:concept:405825/synonyms?relevance=publication' | jq '.summary.publishable'
# 22
```

Ein syntaktisch gültiger, aber nicht unterstützter Wert (`rank=genus`) wird
mit `400 INVALID_QUERY` **abgelehnt** statt stillschweigend ignoriert: eine
Antwort ohne Rangfilterung an einen Aufrufer, der eine Rangfilterung
angefordert hat, wäre eine falsche Aussage.

## Was dieser Filter nicht kann

### (a) Keine regionale Filterung — „im Bezugsraum verwendet"

**Der Endpunkt kennt keinen Bezugsraum. Es gibt keinen `area`-Parameter,
und es gäbe auch nichts, worauf er wirken könnte.**

Der Grund ist strukturell, kein Versäumnis: `distribution` hängt im Schema
am **Konzept**, ein Synonym ist aber ein **Name**. Die Frage „wurde
*Weingaertneria canescens* im deutschsprachigen Raum verwendet?" ist eine
Aussage über einen Namen in einem Gebiet — und diese Tabelle existiert
nicht. Die Verbreitungsangaben, die hostus hat, sagen, wo das *Taxon*
vorkommt, nicht, welcher *Name* dort in Gebrauch war. Beides zu
verwechseln, wäre ein grober Fehler: ein weltweit verbreitetes Taxon hat
eine WGSRPD-Zeile für Deutschland, ganz gleich, ob irgendein deutsches Werk
je eines seiner Synonyme benutzt hat.

Was es bräuchte:

1. eine **Name-×-Gebiet-Relation** (`name_area_usage` o. ä.) mit einer
   Quelle je Zeile — nicht ableitbar aus WCVP, das führt keine Namensnutzung
   pro Gebiet;
2. eine Quelle, die so etwas überhaupt behauptet: praktisch die
   regionalen Florenwerke bzw. Checklisten selbst (Euro+Med,
   Standardliste, CDM-Klassifikationen), also dieselbe Datenlage wie (b);
3. einen `area`-Parameter am Endpunkt plus eine ausdrückliche Regel, was
   „keine Aussage für dieses Gebiet" bedeuten soll — vermutlich
   „zurückhalten mit eigenem Ausschlussgrund", niemals „stillschweigend
   weglassen".

Bis dahin gilt: **`relevance=publication` filtert global.**

### (b) Keine Filterung nach Standardwerken — „in Rothmaler/Oberdorfer verwendet"

Ebenfalls nicht umgesetzt, aber aus einem anderen Grund: die Daten
existieren, dürfen aber nicht verwendet werden und sind nicht fertig
geerntet.

ROTHMALER, OBERDORFER, HEGI und SCHMEIL-FITSCHEN sind buchstäblich vier der
18 CDM-Klassifikationen, die SP5 für `POST /v1/translate` erschlossen hat.
Die Nutzung genau dieser Zeilen wäre also technisch nah. Sie scheitert an
zweierlei:

- **Lizenz.** Die CDM-Ernte ist im Manifest als `redistribution: unknown`
  geführt; für die Daten ist keine Lizenz auffindbar und sie sind aus
  urheberrechtlich geschützter Florenliteratur abgeleitet. Siehe
  [UC6-Anleitung, „Vorab: die Lizenzlage"](sec-translate-uc6.md). Ein
  Relevanzfilter, der auf diesen Zeilen beruht, würde die
  Nutzungsbeschränkung in jede Antwort dieses Endpunkts hineintragen.
- **Vollständigkeit.** Die CDM-Ernte läuft und ist nicht abgeschlossen. Ein
  Filter auf unvollständigen Daten liefert „nicht in Rothmaler" für Namen,
  die schlicht noch nicht geerntet sind — die gefährlichste Sorte
  Falschaussage, weil sie wie ein Befund aussieht.

Solange beides offen ist, existiert dieses Kriterium in hostus nicht — auch
nicht in abgeschwächter Form.

### (c) Die Typisierung ist dreiwertig, und `heterotypic` kommt nicht vor

`typification` hat drei Werte, aber der Index zeigt heute nur zwei:

| Wert | Zeilen (Synonym-Rollen) |
| --- | ---: |
| `homotypic` (`homotypic = 1`) | 271.821 |
| `unknown` (`homotypic IS NULL`) | 692.941 |
| `heterotypic` (`homotypic = 0`) | **0** |

`unknown` heißt **unbekannt**, nicht „heterotypisch". SP3 hat sich bewusst
geweigert, das zu raten, und hostus rät es hier auch nicht: `unknown`
sortiert zwischen den beiden bekannten Zuständen und wird als eigener Wert
ausgeliefert. Praktisch bedeutet das:

- UC5-Regel 3 („homotypisch vor heterotypisch") wirkt real als
  „**basionym-belegt vor unbelegt**";
- rund 72 % der Synonymzeilen tragen `unknown` — eine Liste, die nach
  Typisierung sortiert ist, ordnet also überwiegend nach *Beleglage*, nicht
  nach Typisierung;
- wer `typification: "heterotypic"` in einer Antwort erwartet, wartet
  vergeblich. Der Wert existiert im Modell, weil die Spalte dreiwertig ist,
  nicht weil eine Antwort ihn heute zeigen könnte.

### (d) Ein fehlender `nom_status` ist kein Unbedenklichkeitsnachweis

`nom_status` ist auf **99.252 von 1.448.984 Namen (6,85 %)** belegt. Für
die übrigen 93 % hat die Quelle **nichts eingetragen** — was etwas anderes
ist als „geprüft und für sauber befunden".

hostus unterscheidet das ausdrücklich: das Urteil heißt `absent`, nicht
`acceptable`, und `summary.absent` zählt, wie viele der ausgelieferten
Synonyme auf dieser Abwesenheit ruhen. Im Beispiel oben sind das **alle
sechs**. Ein `absent`-Name ist trotzdem publikationsfähig — 93 % des
Korpus zurückzuhalten würde den Endpunkt sinnlos machen —, aber die Zahl
steht in jeder Antwort, damit niemand sie mit einer Prüfung verwechselt.

Wie klein der geprüfte Anteil wirklich ist, zeigt die Verteilung über alle
964.762 Synonymzeilen des Index:

| Urteil | Zeilen |
| --- | ---: |
| `absent` (nichts eingetragen) | 872.270 |
| `disqualifying` | 89.836 |
| `unclassified` (zurückgehalten) | 2.309 |
| **`acceptable`** (ausdrücklich als sauber bezeichnet) | **347** |

**347.** Im gesamten Korpus behauptet die Quelle für 347 Synonymzeilen, dass
der Name nomenklatorisch in Ordnung ist. Jede publikationsfähige Liste
besteht damit zu über 99,9 % aus Namen, über die niemand etwas gesagt hat.

Praktisch: **ein publikationsfähiges Synonym ohne `nom_status` ist ein
ungeprüftes, kein geprüftes.** Für eine Veröffentlichung bleibt die
nomenklatorische Prüfung dieser Namen beim Autor.

### (e) Zwei benannte Lücken im Rangkriterium

`rank=species` schließt genau die vier von UC5 benannten Ränge aus:
VARIETY (201.957 Synonymnamen), FORM (42.681), SUBVARIETY (3.328) und
SUBFORM (641).

Zwei Dinge, die es damit **nicht** tut:

- **SUBSPECIES wird nicht ausgeschlossen.** UC5 nennt Unterarten nicht, und
  hostus erfindet keine Regel, die der Use Case nicht verlangt hat. Im
  Index betrifft das **45.526 Synonymnamen**; im Beispielkonzept oben ist
  *Corynephorus canescens* subsp. *maritimus* deshalb publikationsfähig.
  Wer auf Artniveau publiziert und auch Unterarten weglassen will, muss sie
  selbst herausnehmen.
- **Nothotaxa passieren den Filter.** 190 Zeilen tragen einen
  Hybrid-Rang — NOTHOSUBSPECIES (130), NOTHOVARIETY (51), NOTHOFORM (9) —,
  und `rank=species` schließt keinen davon aus, obwohl NOTHOVARIETY und
  NOTHOFORM inhaltlich unterhalb der Art liegen. Kleine Menge, aber eine
  echte Inkonsistenz gegenüber der Absicht der Regel.

### (f) Offener fachlicher Punkt: fünf Werte werden zurückgehalten

Fünf `nom_status`-Werte klassifiziert hostus als `unclassified` und hält
die betroffenen Namen damit **zurück**, weil ihre Behandlung eine
**botanische und keine technische** Entscheidung ist. Gemessen betrifft das
**1.697 Namen**:

| Wert | Namen | warum offen |
| --- | ---: | --- |
| `, sensu auct.` | 1.117 | eine **Fehlanwendung**, kein nomenklatorischer Mangel |
| `, tentatively listed as a synonym.` | 290 | taxonomische Unsicherheit, keine Publikationsfrage |
| `, fossil name.` | 264 | sagt nichts über die nomenklatorische Gültigkeit |
| `, isonym` | 13 | Doppelveröffentlichung desselben Namens |
| Wert enthält `?` | 13 | die Quelle selbst ist unsicher |

Der wichtigste davon ist der erste. Fehlanwendungen werden in Florenwerken
üblicherweise **nicht weggelassen, sondern als *auct. non* geführt** — ein
Leser muss sehen, dass der Name in der Literatur vorkommt, aber für etwas
anderes verwendet wurde. Sollte UC5 das so wollen, ist das eine Änderung an
der **Guard-Tabelle** (`nomStatusGuards` in `internal/domain/synonym.go`),
keine Code-Änderung: der Wert bekommt ein eigenes Urteil und einen eigenen
Ausschluss- bzw. Kennzeichnungsgrund. Bis eine Botanikerin oder ein
Botaniker das entscheidet, sind diese 1.117 Namen zurückgehalten und in
`summary.unclassified_statuses` sichtbar — zurückhalten ist die sichere
Richtung, aber es ist eine Entscheidung, keine Wahrheit.

## Checkliste vor der Veröffentlichung

- [ ] `summary.excluded` gelesen — weiß ich, welche Regel wie viele
      Synonyme entfernt hat?
- [ ] `summary.absent` gelesen — wie viele meiner Namen ruhen auf einem
      **fehlenden** Status statt auf einem geprüften?
- [ ] `summary.unclassified_statuses` gelesen — hält hostus etwas zurück,
      das in meine Liste gehört (`sensu auct.`!)?
- [ ] Ist mir bewusst, dass **keine regionale** und **keine
      Standardwerk-Filterung** stattgefunden hat?
- [ ] Habe ich Unterarten und Nothotaxa selbst geprüft?

## Weiterführend

- [HTTP-Referenz zum Endpunkt](../reference/http-api.md) — vollständige
  Parameter, Fehlercodes und Regeltabelle
- [UC6: Konzepte zwischen `sec.`-Räumen übersetzen](sec-translate-uc6.md) —
  die CDM-Daten aus (b) und ihre Lizenzlage

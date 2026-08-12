# hostus 2.0 — Leitfaden zur Lösungsarchitektur

**Stand:** 28.07.2026
**Zweck:** Konzeptueller Leitfaden für den Umbau von `hostus` von einem GBIF-Autosuggest-PoC zu einem Multi-Backbone-Namens- und Merkmalsservice für sechs konkrete Anwendungsfälle.

**Zu den Beispielen:** Alle mit ✅ markierten Identifikatoren wurden am 28.07.2026 gegen die Primärquelle verifiziert. Wo ein Wert nicht verifiziert wurde, steht `‹auflösen›` — diese Werte sind beim ersten Ingest zu ermitteln, nicht zu raten.

---

## Teil A — Grundprinzipien

### A.1 Die drei Ebenen, die nie vermischt werden dürfen

| Ebene | Frage | Stabilität | Anker |
|---|---|---|---|
| **Nomenklatur** | Wie heißt der Name, wer hat ihn publiziert? | hoch | IPNI-ID |
| **Taxonomie (Konzept)** | Welche Individuen umfasst dieses Taxon? | mittel | Backbone-ID + Version + `sec.` |
| **Merkmal** | Welche Zeigerwerte/Traits hängen daran? | an Konzept gebunden | Trait-Vokabular + Version |

Der häufigste Architekturfehler ist, diese drei in eine Tabelle zu legen. Dann kann man nicht mehr ausdrücken, dass *Festuca ovina* sec. Euro+Med etwas anderes umfasst als *Festuca ovina* sec. WCVP — und Zeigerwerte, die für das eine Konzept gelten, werden stillschweigend dem anderen zugeschrieben.

### A.2 Die drei Rollen eines Backbones

| Rolle | Anforderung | Kandidaten |
|---|---|---|
| **Operativ** — Autosuggest, Erfassung, Occurrence-Anschluss | stabile IDs, Versionierung, Lizenz, Updates | COL XR, WCVP/POWO, Euro+Med |
| **Trait-Vokabular** — Join für Zeigerwerte | feste, gepinnte Version | FloraVeg.EU, EuroSL (für EIVE), GermanSL |
| **Konzeptreferenz (`sec.`)** — Interpretation von Literatur | bibliografische Identität, Auflage, Jahr | Hegi, Rothmaler *(Auflage!)*, Oberdorfer 8. Aufl. 2001, Ehrendorfer 1973, Schubert et al., Wisskirchen & Haeupler 1998 |

### A.3 Die eiserne Regel: Late Binding

**Speichere immer den Verbatim-Namen. Die Auflösung ist derivativ und jederzeit wiederholbar.**

Das ist die wichtigste operative Regel des ganzen Systems. Jede Erfassung — ob per PlantNet, Autosuggest oder Literaturzitat — speichert:

```json
{
  "verbatim_name": "Corynephorus canescens",
  "verbatim_authorship": null,
  "determination_source": "plantnet",
  "determination_evidence": { "score": 0.907, "organ": "flower", "images": ["…"] },
  "resolution": {
    "concept_id": "b3f1…",
    "match_type": "exact_author",
    "confidence": 0.99,
    "resolved_at": "2026-07-28T10:14:00Z",
    "backbone_versions": { "colxr": "2026-06-15", "euromed": "2024-12-01", "floraveg": "2021-06-01" }
  }
}
```

Wenn COL XR im nächsten Release umsortiert oder du einen Matching-Bug findest, wirfst du `resolution` weg und lässt neu auflösen. Der Beobachtungsdatensatz selbst bleibt unangetastet. Ohne diese Trennung wird jeder Backbone-Wechsel zur Datenmigration mit Verlustrisiko.

### A.4 Referenz-Beispieltaxa für dieses Dokument

**Corynephorus canescens (L.) P.Beauv.** — Silbergras, Charakterart der Sandtrockenrasen

| Autorität | ID | |
|---|---|---|
| IPNI / POWO | `396681-1` | ✅ `urn:lsid:ipni.org:names:396681-1` |
| Genus *Corynephorus* P.Beauv. | `331174-2` | ✅ |
| Wikidata | `Q159953` | ✅ |
| World Flora Online | `wfo-0000860632` | ✅ |
| GBIF COL-XR taxonKey | `‹auflösen›` | via `/v2/species/match` |
| Euro+Med CDM-UUID | `‹auflösen›` | via Euro+Med ColDP |

Verifizierte Synonyme (POWO): *Weingaertneria canescens* (L.) Bernh. (1800) · *Avena canescens* (L.) Weber (1780) · *Corynephorus incanescens* Bubani (1901, nom. superfl.) · Basionym: *Aira canescens* L.

**Jacobaea vulgaris Gaertn.** — Jakobs-Greiskraut, Beispiel für Gattungswechsel

| Autorität | ID | |
|---|---|---|
| IPNI / POWO | `226649-1` | ✅ Fruct. Sem. Pl. 2: 445 (1791) |
| Synonym | *Senecio jacobaea* L., Sp. Pl. 2: 870 (1753) | ✅ |
| GBIF COL-XR taxonKey | `‹auflösen›` | |

Weiteres verifiziertes Beispiel für UC3: *Ceutorhynchus coerulescens* (Gyllenhal) lebt monophag an *Lepidium campestre* (L.) R.Br. (Bayer & Winkelmann 2005, Rote Liste Rüsselkäfer Berlin).

---

## Teil B — Die API-Oberfläche von hostus 2.0

Alle sechs Use Cases werden von sieben Endpunkten getragen. Das ist der eigentliche Umbau: der PoC hatte einen (`suggest` gegen GBIF), das Zielsystem hat sieben gegen einen lokalen Multi-Backbone-Index.

| Endpunkt | Zweck | UC |
|---|---|---|
| `GET /v1/suggest` | Autosuggest, bezugsraum-gerankt | 1, 2, 4 |
| `POST /v1/match` | Batch-Namensauflösung (Verbatim → Konzeptkandidaten) | 3, 4, 6 |
| `GET /v1/concept/{id}` | Konzept mit allen Xrefs und Klassifikation | alle |
| `GET /v1/xref` | Reverse-Lookup: Fremd-ID → Konzept | 1, 2 |
| `GET /v1/concept/{id}/traits` | Zeigerwerte/Traits pro Vokabular | 1, 4 |
| `GET /v1/concept/{id}/synonyms` | Synonymliste, filterbar | 5 |
| `POST /v1/translate` | Konzeptübersetzung zwischen `sec.`-Räumen | 3, 6 |

Dazu ein Nicht-HTTP-Artefakt:

| Artefakt | Zweck | UC |
|---|---|---|
| **Offline-Bundle** (SQLite/FTS5) | Autosuggest ohne Netz im Feld | 1, 4 |

### B.1 `GET /v1/suggest`

```
GET /v1/suggest?q=coryn&space=euromed&area=DE-BY&rank=species,subspecies&limit=10
```

```json
{
  "backbone_versions": { "euromed": "2024-12-01" },
  "results": [
    {
      "concept_id": "b3f1…",
      "display": "Corynephorus canescens (L.) P.Beauv.",
      "canonical": "Corynephorus canescens",
      "vernacular_de": "Silbergras",
      "rank": "species",
      "status": "accepted",
      "in_area": true,
      "score": 0.98
    }
  ]
}
```

**Das entscheidende Designdetail:** `in_area` und das darauf aufbauende Ranking. Ein Autosuggest, das bei „coryn" für einen Erfasser in Unterfranken zuerst die europäischen Arten zeigt und nicht *Corynephorus divaricatus* aus Marokko, ist im Feld ein anderes Werkzeug. Datenbasis: WGSRPD-Level-3-Codes aus WCVP, Euro+Med-Gebietscodes, für Bayern zusätzlich der „Bayernstatus" aus TaxRef.

Rankingformel, in dieser Priorität:
1. Präfixtreffer auf Gattung oder Art
2. `in_area == true`
3. `status == accepted` vor Synonym
4. Rang: Art vor Unterart vor Varietät
5. String-Score (nur als Tiebreaker)

### B.2 `POST /v1/match`

```json
{
  "names": [
    { "id": "1", "verbatim": "Senecio jacobaea L." },
    { "id": "2", "verbatim": "Festuca ovina agg." },
    { "id": "3", "verbatim": "Silene otitis" }
  ],
  "target_space": "floraveg",
  "sec_hint": "schubert-2001"
}
```

```json
{
  "results": [
    { "id": "1", "match_type": "exact_author", "confidence": 0.99, "concept_id": "…" },
    { "id": "2", "match_type": "aggregate_alias", "confidence": 0.95, "concept_id": "…",
      "note": "Aggregat, keine Kleinartauflösung" },
    { "id": "3", "match_type": "fuzzy", "confidence": 0.91, "concept_id": "…",
      "candidates": ["Silene otites (L.) Wibel"], "requires_review": true }
  ]
}
```

`requires_review: true` bei Fuzzy-Treffern ist nicht optional. Ein Fuzzy-Fehltreffer zwischen zwei kongenerischen Sandtrockenrasen-Arten verschiebt N und Disturbance-Severity um mehrere Skalenpunkte — und genau diese Arten haben die engsten Nischen im ganzen Datensatz.

---

## Teil C — Use Cases

## UC1 — Feldbestimmung per PlantNet oder Autosuggest, mit Zeigerwerten

### Ziel
Pflanze im Feld bestimmen (Foto oder manuell), einer Exkursion an einem Ort zuordnen, Ellenberg-/EIVE-Werte mitführen.

### Konzeptueller Ablauf

**Pfad A — PlantNet**

1. App sendet Foto an `POST https://my-api.plantnet.org/v2/identify/k-central-europe`
2. PlantNet liefert pro Kandidat: `scientificNameWithoutAuthor`, `scientificNameAuthorship`, `score` — **und, entscheidend, `gbif.id` sowie `powo.id`** ✅ (belegt in der PlantNet-API-Doku)
3. App ruft `GET /v1/xref?authority=powo&id=396681-1` → hostus-Konzept-ID
4. App speichert Verbatim + Evidence + Resolution (Schema aus A.3)
5. Traits per `GET /v1/concept/{id}/traits?vocab=eive,tichy,midolo`

**Warum über `powo.id` und nicht über den Namen matchen:** Der Namensstring von PlantNet muss geparst, kanonisiert und gematcht werden — die POWO-ID ist ein direkter Schlüssel in dein WCVP-Backbone. Das spart nicht nur Rechenzeit, es eliminiert eine ganze Fehlerklasse. Fallback auf Namensmatching nur, wenn `powo.id` fehlt (kommt bei Nicht-Kew-Projekten vor).

**Pfad B — manueller Autosuggest**

1. `GET /v1/suggest?q=…&area=DE-BY` gegen das **Offline-Bundle**, nicht gegen das Netz
2. Nutzer wählt → Konzept-ID direkt bekannt, kein Matching nötig
3. Weiter wie A.4/A.5

**Das Offline-Bundle** ist kein Nice-to-have. Sandtrockenrasen liegen selten im LTE-Vollausbau. Konkret: eine SQLite-Datei mit FTS5-Index, gefiltert auf `in_area` für den relevanten Bezugsraum, ~10–20 MB für Mitteleuropa, delta-synchronisiert über eine Versionsnummer. Enthält: Konzept-ID, canonical, Autorschaft, deutscher Name, Rang, Status, plus ein Trait-Snapshot.

### Zeigerwerte: welches System?

Zwei Systeme, unterschiedliche Eigenschaften — verifizierte Unterschiede:

| | **EIVE 1.0** (Dengler et al. 2023) | **Tichý et al. 2023** |
|---|---|---|
| Dimensionen | M, N, R, L, T | M, N, R, L, T **+ Salinität** |
| Skala | einheitlich 0–10, kontinuierlich | Ellenberg-kompatibel, variierend (1–9, 0–9, 1–12) |
| Nischenbreite | ✅ ja | ✗ nein |
| Quellsysteme | 31 | 12 |
| Taxa | 14.835 akzeptierte (11.148 Arten) | 8.908 akzeptierte (8.679 Arten) |
| Unterarten | ✅ ja | eingeschränkt |
| Taxonomie | Euro+Med (via EuroSL) | Euro+Med + EUNIS-ESy-Anpassungen (FloraVeg.EU) |

**Empfehlung:** beide führen, EIVE als Default. Begründung: die Nischenbreite ist für gewichtete Mittelwerte pro Aufnahme wertvoll (Gewichtung mit inverser Nischenbreite), und die Abdeckung der Unterarten ist bei Sandtrockenrasen-Sippen relevant. Tichý zusätzlich, weil nur dort Salinität steht und weil er im FloraVeg.EU-Namensraum liegt — demselben, den dein `situs`/ESy-Pfad ohnehin braucht.

**Speicherung der Werte:** Zeiger auf Konzept + Vokabularversion, nicht die Zahlen selbst. Ausnahme: der Trait-Snapshot im Offline-Bundle, klar als `snapshot_version` markiert. Sonst hast du in drei Jahren Aufnahmen mit EIVE-1.0-Werten und Aufnahmen mit EIVE-1.1-Werten in derselben Spalte, ohne es zu wissen.

### Beispiel

```
Foto → PlantNet k-central-europe
  → "Corynephorus canescens", authorship "(L.) P.Beauv.", score 0.907
  → powo.id = "396681-1"                                          ✅
GET /v1/xref?authority=powo&id=396681-1
  → concept_id (Backbone wcvp, Version 2026-06-15)
GET /v1/concept/{id}/traits?vocab=eive
  → { "M": ‹Wert›, "N": ‹Wert›, "R": ‹Wert›, "L": ‹Wert›, "T": ‹Wert›,
      "nw": {…}, "n_systems": …, "vocab_version": "eive-1.0" }
```

Die konkreten EIVE-Zahlen stehen bewusst als `‹Wert›`: sie kommen beim Ingest aus der EIVE-1.0-Tabelle und sind nicht aus dem Gedächtnis zu zitieren.

### Fallstricke

- PlantNet-Projekte sind geografisch geschnitten (`k-*` = Kew-TDWG). Ein falsch gewähltes Projekt liefert plausible, aber gebietsfremde Treffer. Projekt aus der Koordinate ableiten: `GET /v2/projects?lat=&lon=`.
- PlantNet bestimmt auf Artniveau. Für Unterarten und Kleinarten ist der manuelle Pfad zwingend.
- Der PlantNet-Score ist eine Bildähnlichkeit, keine Bestimmungssicherheit. Er gehört in `determination_evidence`, nicht in `resolution.confidence` — das sind zwei verschiedene Unsicherheiten (Bestimmung vs. Namensauflösung), und sie dürfen nicht zu einer Zahl verrechnet werden.

---

## UC2 — Zielart über iNaturalist finden, dann Exkursion

### Ziel
Seltenere Art recherchieren, Koordinate bekommen, hinfahren, dann Aufnahme wie UC1. Bisher Shell-Skript, soll Autosuggest + kleine App werden.

### Konzeptueller Ablauf

1. **Autosuggest gegen hostus**, nicht gegen iNaturalist. `GET /v1/suggest?q=…&space=euromed&area=DE-BY`
2. Aus dem Konzept die **iNaturalist-taxon_id** ziehen: `GET /v1/concept/{id}` → `xrefs.inat`
3. `GET https://api.inaturalist.org/v1/observations?taxon_id=…&nelat=&nelng=&swlat=&swlng=&quality_grade=research&geoprivacy=open`
4. Parallel GBIF-Occurrences über den COL-XR-taxonKey, und für Bayern ASK/FIN-Web
5. Kandidatenkoordinaten → Exkursionsplanung → Aufnahme nach UC1

**Warum der Autosuggest gegen hostus laufen muss und nicht gegen `/v1/taxa/autocomplete` von iNaturalist:** Sonst arbeitest du im iNat-Namensraum und musst hinterher zurückübersetzen. Der Umweg über hostus gibt dir außerdem das Bezugsraum-Ranking und dieselbe Konzept-ID, die später in der Aufnahme landet — eine ID über den ganzen Workflow.

### Der Fallstrick, der diesen Use Case dominiert

**Koordinaten seltener Arten sind bei iNaturalist systematisch verschleiert.** Für Taxa mit Schutzstatus setzt iNaturalist `geoprivacy`/`taxon_geoprivacy` auf `obscured`; die veröffentlichte Koordinate wird dann auf eine Zelle von etwa 0,2° × 0,2° gerundet — also grob 20 km. Das ist genau die Artengruppe, die dich interessiert.

Konsequenzen für den Entwurf:
- `geoprivacy=open` als Filter setzen und die Trefferzahl gegen die ungefilterte prüfen. Wenn 90 % obscured sind, ist iNaturalist für dieses Taxon keine Fundpunktquelle.
- `positional_accuracy` immer mitführen und in der App anzeigen. Eine Koordinate ohne Genauigkeitsangabe ist für eine Anfahrt wertlos.
- Für FFH-Arten in Bayern sind ASK und Biotopkartierung die bessere Quelle — mit Genehmigungserfordernis, aber ohne Verschleierung.
- iNaturalist-Bestimmungen sind community-verifiziert, nicht taxonomisch geprüft. `quality_grade=research` heißt „zwei Zustimmungen", nicht „von einem Spezialisten bestätigt". Bei kritischen Sippen praktisch wertlos.

### Beispiel

```
suggest "jakobs" → Jacobaea vulgaris Gaertn. (IPNI 226649-1) ✅
GET /v1/concept/{id} → xrefs: { powo: "226649-1", inat: ‹auflösen›, colxr: ‹auflösen› }
GET api.inaturalist.org/v1/observations?taxon_id=‹inat›&swlat=49.6&swlng=9.7&nelat=50.1&nelng=10.4
     &quality_grade=research&geoprivacy=open&per_page=200
→ Koordinatenliste + positional_accuracy
```

Ein zusätzlicher Nutzen dieses Pfads: iNaturalist führt **Synonyme als eigene Namen** mit Weiterleitung. Wenn du nach *Senecio jacobaea* suchst, findest du die Beobachtungen unter *Jacobaea vulgaris*. Dein `xrefs.inat` muss deshalb auf die iNat-**taxon_id des akzeptierten Taxons** zeigen, nicht auf die des Synonyms.

---

## UC3 — Wirtspflanzenbindung aus Literatur in den Knowledge Graph

### Ziel
Autor nennt eine Wirtspflanze. Benötigt: Name, IPNI-ID, Konzept. Das Konzept ist meist **nicht explizit genannt**.

### Konzeptueller Ablauf

1. Verbatim-Zitat erfassen — genau wie im Text, inklusive Autorschaft und Schreibweise
2. `POST /v1/match` mit `sec_hint` = die Publikation
3. hostus liefert Konzeptkandidaten **plus** eine `sec.`-Inferenz mit Konfidenzstufe
4. Der Kurator bestätigt oder korrigiert → Assertion mit Provenienz in den Graph

### Der Kern: `sec.`-Inferenz als eigenes, prüfbares Artefakt

Wenn der Autor kein Bezugssystem nennt, ist die Aufgabe nicht, es zu erraten, sondern die Inferenz **explizit und widerlegbar** zu machen:

```json
{
  "sec_inference": {
    "assumed": "hegi-1906-1931",
    "confidence": "inferred",
    "basis": "zeitgenössischer Standard für deutschsprachige Entomologen; keine explizite Nennung in der Quelle geprüft",
    "alternatives": ["rothmaler-1954", "regional-flora"],
    "checked_source_pages": null,
    "asserted_by": "jo",
    "asserted_at": "2026-07-28"
  }
}
```

Der Punkt: `confidence: "inferred"` ist eine andere Aussage als `confidence: "explicit"`. Wenn später jemand das Literaturverzeichnis der Quelle durcharbeitet, kommt eine neue Zeile mit besserer Evidenz hinzu — die alte bleibt als historiografische Schicht erhalten. Eine flache Tabelle kann das nicht.

Heuristik für die Vorbelegung von `assumed` nach Publikationsjahr (als Vorschlag, nie als Automatik):

| Zeitraum | Vermutete botanische Referenz |
|---|---|
| bis ~1965 | Hegi, ältere Regionalfloren |
| ~1965–1990 | Ehrendorfer 1973, Rothmaler *(Auflage prüfen)*, Oberdorfer |
| ~1990–2005 | Wisskirchen & Haeupler 1998, Oberdorfer 8. Aufl. 2001 |
| ab ~2005 | Buttler et al. 2018 / Rothmaler neuere Auflagen |

### Warum die IPNI-ID hier der richtige Anker ist

Die IPNI-ID identifiziert den **Namen**, nicht das Konzept. Für eine Literaturassertion ist das exakt die passende Granularität: der Autor hat einen Namensstring geschrieben, und der ist eindeutig auflösbar. Das Konzept dahinter ist die separate, unsichere Aussage — und deshalb gehört sie in ein separates Feld mit eigener Konfidenz.

### Beispiel

Bayer & Winkelmann (2005) nennen für *Ceutorhynchus coerulescens* (Gyllenhal) monophage Bindung an *Lepidium campestre* (L.) R.Br. ✅

```
POST /v1/match
  { "verbatim": "Lepidium campestre (L.) R. BR.", "sec_hint": "bayer-winkelmann-2005" }
→ { "match_type": "exact_author", "confidence": 0.99,
    "ipni_id": "‹auflösen›",
    "concept_id": "…",
    "sec_inference": { "assumed": "wisskirchen-1998", "confidence": "inferred",
                       "basis": "Publikationsjahr 2005, deutsche Rote-Liste-Praxis" } }
```

Die resultierende Assertion im Graph — reifiziert, nicht als direkte Kante:

```turtle
:assertion_0042
  a :HostPlantAssertion ;
  :beetle        :ceutorhynchus_coerulescens ;
  :plantConcept  :concept_lepidium_campestre_wisskirchen1998 ;
  :plantName     <https://ipni.org/n/‹id›> ;
  :evidenceType  :LiteratureCompilation ;
  :monophagy     :Monophagous ;
  :source        :bayer_winkelmann_2005 ;
  :secConfidence "inferred" .
```

`evidenceType: LiteratureCompilation` ist wichtig: Bayer & Winkelmann kompilieren hier, sie haben nicht selbst gezüchtet. Ohne diese Unterscheidung reproduziert dein Graph exakt die Zitationskaskade, die du eigentlich auflösen willst.

---

## UC4 — Vegetationsaufnahme mit Deckung → EUNIS-Habitat

### Ziel
Nahezu vollständige Aufnahme mit Deckungsgraden, im Feld oft nur auf Aggregatniveau. Auswertung soll EUNIS-Habitate liefern (via EUNIS-ESy).

### Das strukturelle Problem

ESy-Ausdrücke referenzieren **konkrete Namen im FloraVeg.EU-Namensraum**. Eine Feldangabe `Festuca ovina agg.` ist dort möglicherweise kein Eintrag, oder sie ist einer — mit anderem Umfang als das, was du im Feld gemeint hast. Drei Fälle sind zu unterscheiden, und der Service muss sie unterscheiden können:

| Fall | Behandlung |
|---|---|
| ESy kennt das Aggregat als Taxon | direkt verwenden, `aggregate_policy: "known"` |
| ESy kennt nur Kleinarten | Aggregat markieren, Deckung **nicht** auf Kleinarten verteilen, `aggregate_policy: "unresolvable"` |
| ESy-Regel nutzt eine Kleinart als Differentialart | Aufnahme kann diese Regel nicht bedienen → in der Diagnose als Datenlücke ausweisen |

Der dritte Fall ist der wichtige und der, den man am leichtesten übersieht. Wenn eine ESy-Regel für R1P auf einer Kleinart beruht, die du im Feld nicht ansprechen konntest, dann ist das Ergebnis nicht „Habitat nicht erfüllt", sondern „nicht entscheidbar". Der Unterschied muss durch die ganze Kette bis in die Ausgabe durchgereicht werden.

### Konzeptueller Ablauf

1. Feldaufnahme: Verbatim-Namen + Deckung (%, Braun-Blanquet, oder beides)
2. `POST /v1/match` mit `target_space=floraveg`, batchweise für die ganze Aufnahme
3. hostus liefert pro Eintrag: ESy-kompatibler Name, `aggregate_policy`, `esy_diagnostic_relevance`
4. Aufnahme + Koordinate an `situs`
5. `situs` → ESy-Sidecar (R/plumber) → EUNIS-Level-3, **plus** Liste der nicht entscheidbaren Regeln
6. Ergebnis mit Vertrauensangabe, nicht als einzelner Habitatcode

### Beispiel

```
Feldaufnahme (Sandtrockenrasen, Unterfranken):
  Corynephorus canescens        40 %   → exact_author, IPNI 396681-1 ✅
  Festuca ovina agg.            15 %   → aggregate_alias, policy: ‹prüfen›
  Jacobaea vulgaris              2 %   → exact_author, IPNI 226649-1 ✅
  Rumex acetosella               5 %   → ‹auflösen›

POST /v1/relevé/prepare?target=floraveg
→ { "usable": 3, "aggregates": 1, "esy_blocked_rules": [ … ] }

→ situs → EUNIS R1P (o. ä.) + FFH-LRT 2330 (Zuordnung aus der ESy-Diagnose,
  nicht aus dem ML-Prior — der ist für azonale Sandrasen unzuverlässig)
```

Der Hinweis zum ML-Prior ist hier kein Beiwerk: die ML-basierten EUNIS-Wahrscheinlichkeitsraster sind für rare azonale Habitate wie R1Q/R1P systematisch schwach. Für diesen Use Case ist die ESy-Diagnose die Primärquelle und der Prior nur Plausibilisierung.

### Fallstrick: Deckungssummen

Wenn ein Aggregat und eine seiner Kleinarten beide in der Aufnahme stehen (im Feld passiert das), summieren sich Deckungen doppelt. `POST /v1/relevé/prepare` muss diese Überlappung erkennen und melden — über die Elternschaftsbeziehung im Konzeptbaum, nicht über String-Vergleich.

---

## UC5 — Publikation: akzeptierter Name plus gängige Synonyme

### Ziel
Aus Funddaten eine Publikation machen, Pflanze korrekt nennen, gängige Synonyme angeben.

### Das Problem ist Filterung, nicht Beschaffung

POWO liefert für *Corynephorus canescens* über zwanzig Synonyme, inklusive `nom. nud.`, `pro syn.` und einem Dutzend Varietäten des 19. Jahrhunderts. In eine Publikation gehören ein bis drei. Der Service muss also **relevanzfiltern**, und die Relevanzkriterien sind publikationsspezifisch:

| Kriterium | Wirkung |
|---|---|
| homotypisch vor heterotypisch | Basionym und Rekombinationen zuerst |
| im Bezugsraum verwendet | regionale Synonyme, nicht globale |
| in Standardwerken des Fachgebiets verwendet | was Leser aus Rothmaler/Oberdorfer kennen |
| Nomenklaturstatus | `nom. nud.`, `nom. superfl.`, `pro syn.` ausschließen |
| Rang | Varietäten und Formen ausschließen, wenn auf Artniveau publiziert |

### Beispiel — verifiziert

```
GET /v1/concept/{id}/synonyms?relevance=publication&area=DE&max=3
```

Akzeptierter Name: **Corynephorus canescens (L.) P.Beauv.**, Ess. Agrostogr. 90 (1812) ✅

| Synonym | Typ | Publikationsrelevanz |
|---|---|---|
| *Aira canescens* L. | Basionym, homotypisch | ✅ hoch |
| *Weingaertneria canescens* (L.) Bernh. (1800) | homotypisch | ✅ hoch — in älterer deutscher Literatur verbreitet |
| *Avena canescens* (L.) Weber (1780) | homotypisch | mittel |
| *Corynephorus incanescens* Bubani (1901) | `nom. superfl.` | ✗ ausschließen |
| *Corynephorus canescens* var. *auratus* Opiz (1852) | Varietät | ✗ ausschließen |

Publikationsfertige Zeile:

> *Corynephorus canescens* (L.) P.Beauv. [*Aira canescens* L.; *Weingaertneria canescens* (L.) Bernh.]

Und — nicht vergessen — die Nomenklaturreferenz im Methodenteil:

> Nomenklatur der Gefäßpflanzen nach WCVP/POWO (Stand 2026-06-15); deutsche Namen nach Buttler et al. (2018).

Das ist genau die Angabe, die deine eigenen Daten in zwanzig Jahren interpretierbar hält — und die in der Wirtspflanzenliteratur, die du auswertest, meistens fehlt.

---

## UC6 — Name aus pflanzensoziologischer Literatur → aktueller Name für die Feldarbeit

### Ziel
Ein Name aus z. B. Schubert et al. ist unbekannt. Gesucht: der heute gültige Name, um die Art mit einem Bestimmungsschlüssel im Feld anzusprechen.

### Konzeptueller Ablauf

1. `POST /v1/translate` mit Quellname, `from_sec`, `to_space`
2. hostus löst über die Synonymie **und** über `concept_relation` auf
3. Ausgabe: aktueller Name + Rangänderungen + Warnung bei Konzeptdivergenz + deutscher Name
4. Zusätzlich: welcher Schlüssel die Art unter welchem Namen führt

### Der Unterschied, der diesen Use Case gefährlich macht

Es gibt zwei Arten von Namensänderung, und für die Feldarbeit haben sie völlig unterschiedliche Konsequenzen:

**Reine Nomenklaturänderung** — dasselbe Konzept, neuer Name. Unproblematisch: du suchst dieselbe Pflanze, nur unter anderem Namen im Schlüssel.

**Konzeptänderung** — Aufspaltung, Zusammenlegung, Rangwechsel. Hier ist die Antwort „der aktuelle Name ist X" potenziell falsch, weil der alte Name mehr oder weniger umfasste als der neue. Bei einer Aufspaltung musst du im Feld **zusätzliche Merkmale prüfen**, die der alte Schlüssel nicht abfragt.

Die Ausgabe muss diese beiden Fälle sichtbar trennen. Ein Service, der nur „heute: X" sagt, ist an dieser Stelle irreführend.

### Beispiel — verifiziert

```
POST /v1/translate
  { "name": "Senecio jacobaea L.", "from_sec": "schubert-2001", "to_space": "colxr" }
```

```json
{
  "input": { "name": "Senecio jacobaea", "authorship": "L.",
             "published_in": "Sp. Pl. 2: 870 (1753)" },
  "current": { "name": "Jacobaea vulgaris", "authorship": "Gaertn.",
               "published_in": "Fruct. Sem. Pl. 2: 445 (1791)",
               "ipni_id": "226649-1",
               "vernacular_de": "Jakobs-Greiskraut" },
  "change_type": "nomenclatural_genus_transfer",
  "concept_relation": "congruent",
  "field_note": "Gattungswechsel Senecio → Jacobaea auf Basis molekularer Phylogenie. Artkonzept unverändert. Ältere Schlüssel führen die Art unter Senecio.",
  "subspecies_warning": "Vier Unterarten werden heute unterschieden (subsp. vulgaris, dunensis, gotlandica, pannonica) — im Feld ggf. zusätzlich zu prüfen."
}
```

Alle Angaben in diesem Beispiel sind verifiziert: IPNI `226649-1`, das Basionym-Verhältnis zu *Senecio jacobaea* L. (1753), und die vier Unterarten nach POWO. ✅

Der `field_note`-Text ist der eigentliche Wert dieses Endpunkts: er sagt dir, dass du mit dem alten Schlüssel arbeiten kannst, weil das Konzept kongruent ist. Bei `concept_relation: "includes"` müsste dort das Gegenteil stehen.

### Datenquelle für `concept_relation`

Hier zahlt sich die Wisskirchen-Aufarbeitung aus. Die Standardliste 1998 vermerkt für jedes Taxon, ob es in identischer oder abweichender Abgrenzung in den einschlägigen Florenwerken enthalten ist; diese Konzeptbeziehungen wurden im Rahmen des Rote-Listen-2020-Projekts nach dem Berendsohn-Modell (1995) digital aufgearbeitet und liegen im CDM-Portal `portal.cybertaxonomy.org/rotelisten_flora_deutschland`. Du importierst also, statt zu erfinden — und du musst die gedruckten Floren dafür nicht digitalisieren.

---

## Teil D — Querschnittsthemen

### D.1 Was aus jedem Use Case in die Datenbank geht

Einheitliches Erfassungs-Schema über alle sechs Fälle:

```sql
CREATE TABLE plant_record (
  id                uuid PRIMARY KEY,
  -- Kontext
  excursion_id      uuid,
  locality_id       uuid,
  observed_at       timestamptz,
  -- Verbatim (unveränderlich)
  verbatim_name     text NOT NULL,
  verbatim_auth     text,
  cover_percent     numeric,
  cover_bb          text,
  -- Herkunft der Bestimmung
  det_source        text NOT NULL,   -- plantnet|manual|inat|key|literature
  det_evidence      jsonb,
  det_by            text,
  -- Auflösung (derivativ, jederzeit neu berechenbar)
  concept_id        uuid REFERENCES taxon_concept(id),
  match_type        text,
  match_confidence  real,
  resolved_at       timestamptz,
  backbone_versions jsonb,
  needs_review      boolean NOT NULL DEFAULT false
);
```

Die Trennlinie zwischen den beiden unteren Blöcken ist die Architektur. Alles unterhalb `concept_id` ist löschbar und neu berechenbar. Alles oberhalb ist Primärdatum.

### D.2 Versionierung und Artefaktvertrag

Nach dem `ortus`-Muster: jeder Backbone-Ingest erzeugt ein versioniertes, reproduzierbares Artefakt.

```yaml
# dataset.yaml
backbones:
  - id: colxr
    source: https://download.checklistbank.org/col/...
    version: "2026-06-15"
    license: CC-BY-4.0
  - id: wcvp
    version: "2026-06-15"
    license: CC-BY-4.0
  - id: euromed
    version: "2024-12-01"
  - id: floraveg
    version: "2021-06-01"
    note: "ESy-Namensraum, gepinnt — nicht mit EIVE/EuroSL verwechseln"
trait_vocabularies:
  - id: eive
    version: "1.0"
    taxonomy: euromed-via-eurosl
  - id: tichy2023
    version: "1.0"
    taxonomy: floraveg
  - id: midolo2023
    version: "1.0"
    taxonomy: floraveg
```

**Versionen pinnen, nie „latest" ziehen.** Der GermanSL-Changelog dokumentiert explizit, dass IDs zwischen Versionen gelöscht und neu zugeordnet werden — wer dort automatisch nachzieht, verliert Zuordnungen still.

### D.3 Umsetzungsreihenfolge

Nach Abhängigkeit, nicht nach Aufwand:

1. **Schema + ColDP-Importer** — ein Parser für alle Backbones. WCVP zuerst, weil es der größte Kew-Anschluss ist und die IPNI-IDs mitbringt.
2. **`/v1/match` + `/v1/concept`** — ohne die geht kein UC.
3. **Euro+Med + FloraVeg.EU** — schaltet UC1 (Traits) und UC4 (ESy) frei.
4. **`/v1/suggest` + Offline-Bundle** — schaltet die Felderfassung frei (UC1, UC4).
5. **Xref-Anreicherung** (Wikidata P14607/P846/P10585/P12380/P12100, iNat) — UC2.
6. **`sec_reference` + `concept_relation`** inkl. Wisskirchen-Import — UC3, UC6.
7. **`/v1/synonyms` mit Relevanzfilter** — UC5, kann warten.

Punkt 6 ist der intellektuell anspruchsvollste und der, der am meisten Wert für die Wirtspflanzendatenbank schafft — aber er setzt 1–3 voraus. Nicht vorziehen.

### D.4 Die verbleibende Unsicherheit, die man nicht wegarchitektieren kann

Bei Sandtrockenrasen-Sippen (*Festuca*-Gruppe, *Thymus*, *Potentilla*, *Armeria*, *Dianthus*) divergieren globale (WCVP/COL) und regionale (Euro+Med, GermanSL) Konzepte am stärksten, und genau dort sind auch die Zeigerwerte am aussagekräftigsten, weil die Nischen eng sind. Für diese Gruppen wird kein automatisches Matching ausreichen. Der Review-Queue-Mechanismus ist deshalb kein Fallback für Randfälle, sondern der Regelpfad für den ökologisch wichtigsten Teil deiner Daten.

---

## Anhang — Quellenlage der verwendeten Identifikatoren

| Angabe | Quelle | verifiziert |
|---|---|---|
| IPNI `396681-1` (*Corynephorus canescens*) | ipni.org, powo.science.kew.org | ✅ |
| IPNI `331174-2` (*Corynephorus* P.Beauv.) | worldfloraonline.org, POWO | ✅ |
| Wikidata `Q159953` | wikidata.org | ✅ |
| WFO `wfo-0000860632` | worldfloraonline.org | ✅ |
| Synonyme *C. canescens* | POWO taxon page | ✅ |
| IPNI `226649-1` (*Jacobaea vulgaris*) | ipni.org/n/226649-1 | ✅ |
| *Senecio jacobaea* L., Sp. Pl. 2: 870 (1753) | IPNI, POWO, Wikispecies | ✅ |
| Vier Unterarten *J. vulgaris* | POWO | ✅ |
| PlantNet liefert `gbif.id` + `powo.id` | my.plantnet.org/doc/api | ✅ |
| PlantNet `k-*`-Projekte = Kew-TDWG/WGSRPD | my.plantnet.org/doc/api/taxonomy | ✅ |
| EIVE 1.0: 14.835 Taxa, 31 Systeme, Skala 0–10, Nischenbreite | Dengler et al. 2023, VCS 6: 98324 | ✅ |
| Tichý et al. 2023: 8.908 Taxa, 12 Systeme, + Salinität | vegsciblog.org, Tichý et al. 2023 | ✅ |
| EIVE-Taxonomie = Euro+Med | Dengler et al. 2023, Abstract | ✅ |
| Tichý/Midolo-Taxonomie = Euro+Med + ESy-Anpassungen | FloraVeg.EU | ✅ |
| COL XR checklistKey `7ddf754f-d193-4cc9-b351-99906754a03b` | GBIF Data Blog, techdocs.gbif.org | ✅ |
| GBIF-Backbone eingefroren (2023), IDs erhalten | techdocs.gbif.org | ✅ |
| `/v1/species/suggest` ohne `checklistKey`-Support | GBIF Data Blog | ✅ |
| Wikidata P14607 (GBIF neu), P846 (alt), P10585 (COL), P12380 (Euro+Med), P12100 (FloraVeg.EU) | wikidata.org | ✅ |
| Wisskirchen-Konzeptbeziehungen im CDM-Portal | portal.cybertaxonomy.org | ✅ |
| *Ceutorhynchus coerulescens* monophag an *Lepidium campestre* | Bayer & Winkelmann 2005 | ✅ |
| iNaturalist obscured coordinates ~0,2° für geschützte Taxa | iNaturalist-Dokumentation | ⚠️ Größenordnung, vor Implementierung prüfen |

# Phase 0 — Gate-Zusammenfassung (Task 0.11)

Roll-up der Annahmen-Verifikation P1–P10 gegen Echtdaten (Findings-Details in `PXX-findings.md`, Reports im SDD-Workspace). Phase R (`docs/research/quellenregister.md`) lieferte die validierten Quellen; Phase 0 prüfte die technischen Annahmen dagegen.

| PoC | Annahme | Verdikt | Gate für | Aktion bei ⚠️/🔴 |
|---|---|---|---|---|
| **P1** | `modernc.org/sqlite` kann FTS5 (Prefix + bm25) | 🟢 | SP1/SP2 | — (v1.55.0, kein Build-Tag, kein CGO) |
| **P2** | WCVP-Struktur trägt IPNI-ID, Synonyme, WGSRPD-L3 | 🟢 | SP1/SP2 | Spec-Korrektur: WCVP ist **DwC-A** (nicht ColDP), Delimiter `\|`, IPNI via `dynamicproperties.powoid`, WGSRPD-L3 als `locationid=TDWG:XXX` (kein Kontinent-Feld). Fixture geschnitten. |
| **P3** | GBIF `/v2/species/match` ehrt COL-XR-`checklistKey` | 🟢 | SP1 | Korrektur: v2-Param `scientificName` (nicht `name`), IDs unter `usage.*`. v1/suggest ignoriert checklistKey. |
| **P4** | PlantNet liefert `gbif.id` **und** `powo.id` | 🟢 | SP2/UC1 | `powo.id` 10/10 non-null, `gbif` teils null → über `powo.id` matchen. Korrektur: Projekt = **`k-middle-europe`**. Score = Bildähnlichkeit → `determination_evidence`. |
| **P5** | Euro+Med als ColDP beziehbar | 🔴 | SP3 | **Blockiert** (kein Export, keine Lizenz). SP3 ohne Euro+Med starten (WCVP+COL-XR); BGBM-Klärung; Trait-Anschluss via Namens-Crosswalk. |
| **P6** | EIVE/Tichý/Midolo: Format + Join-Schlüssel | ⚠️ | SP3 | Tabellen beziehbar (CC-BY, Zenodo), Formate bestätigt (EIVE 0–10 + Nischenbreite; Tichý +Salinität; Midolo Disturbance+EUNIS). **Aber: Join nur über bare name strings, keine externe ID** → SP3 baut Namens-Crosswalks gegen WCVP/COL-XR. GermanSL/EuroSL lizenz-blockiert. |
| **P7** | Wikidata P14607/P846/P10585/P12380/P12100 liefern Xrefs | ⚠️ | SP4 | P846(GBIF-legacy)+P10585(COL) lösen auf & kreuzvalidieren gegen P3; P14607+P12380 an Refs **noch nicht befüllt**. SP4 muss P14607 **und** P846 abfragen. Korrekturen: Jacobaea=Q15630491, WFO=P7715. |
| **P8** | Wisskirchen-Konzeptbeziehungen maschinell beziehbar | ⚠️ | SP5 | **Technisch machbar**: CDM-REST (`api.cybertaxonomy.org/rl_standardliste`, `/portal/taxon/{uuid}/taxonRelationships` + `secSource`), live verifiziert. Caveat: Two-Hop-UUID-Join; Lizenz ⚠️ (BGBM/EDIT-Klärung). SP5-Fallback: Offline-Batch-ETL. |
| **P9** | iNat obscured coords ~0,2° | 🟢 | SP4/UC2 | Bestätigt, **präzisiert ~26–28 km** (Spec „20 km" nach oben korrigiert), ~32–38 % obscured für geschütztes Taxon (62,6 % nutzbar). Felder: `taxon_geoprivacy`/`geoprivacy`/`obscured`/`public_positional_accuracy` (Spec-`coordinates_obscured` existiert nicht). |
| **P10** | FloraVeg/ESy-Namensraum ≠ EIVE/EuroSL | 🟢 | SP3/UC4 | Divergenz belegt (`Festuca lemanii` eigenes Konzept in EIVE, von ESy in `F. ovina` gefaltet). Zwei-Namensraum-Design gerechtfertigt: Traits pro `(namespace, name)`. |

## Gating-Entscheidung je Sub-Projekt

- **SP1 (Foundation):** P1 🟢 · P2 🟢 · P3 🟢 → **ENTBLOCKT.** Umsetzung kann starten. Übernehmen: DwC-A-Parsing-Pfad, IPNI via `powoid`, GBIF v2 `scientificName`+`usage.*`.
- **SP2 (Suggest + Bundle):** P1 🟢 · P4 🟢 → **grün.** Einschränkung: ASK/FIN-Web ❌ (genehmigungspflichtig) mindert nur die `in_area`-Qualität für Bayern; Ersatz über GBIF/iNat-Occurrences.
- **SP3 (Traits + Backbones):** P5 🔴 · P6 ⚠️ · P10 🟢 → **rescoping nötig.** Ohne Euro+Med starten; Namens-Crosswalks statt ID-Join; Lizenzklärung GermanSL/EuroSL/Euro+Med vor Redistribution.
- **SP4 (Xref):** P7 ⚠️ · P9 🟢 → **grün** mit Dual-Property-Abfrage (P14607+P846) und iNat-Feldkorrekturen.
- **SP5 (sec. + concept_relation):** P8 ⚠️ → **machbar** (CDM-REST, Two-Hop-Join) vorbehaltlich Lizenzklärung; sonst manuelle Kuratierung.

## Querschnitts-Blocker (Lizenz)

Vier Quellen sind **lizenzrechtlich ungeklärt/blockiert** und dürfen vor schriftlicher Freigabe **nicht in einen servierten/redistribuierten Index**: Euro+Med, GermanSL, EuroSL (alle keine Lizenz auffindbar) sowie FloraVeg.EU-Downloaddateien (ungeklärt; EUNIS-ESy selbst ist separat CC-BY). Wisskirchen/CDM ebenfalls ohne explizite Lizenz. **Empfehlung:** Lizenzklärungen als eigenständigen, früh gestarteten Arbeitsstrang parallel zu SP1/SP2 führen (lange Vorlaufzeiten), damit SP3/SP5 nicht darauf warten.

## Fazit

Kein 🔴 auf dem SP1-Pfad — **SP1 ist voll entblockt und kann als nächstes umgesetzt werden.** Die ⚠️/🔴-Funde betreffen die späteren SPs (3/4/5) und sind sämtlich mit dokumentierten Fallbacks versehen; der größte Rest-Risiko-Faktor ist **Lizenz**, nicht Technik.

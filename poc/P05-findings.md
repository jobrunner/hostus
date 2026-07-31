# PoC P5 — Euro+Med als ColDP beziehbar? (Task 0.5)

**Gate für:** SP3 (Traits + weitere Backbones)
**Verdikt:** 🔴 **blockiert** — die Annahme „Euro+Med als ColDP beziehbar (CDM-UUID-Anschluss)" hält **nicht**.

## Warum kein ausführbarer PoC

Anders als P1–P4/P6–P10 gibt es hier **nichts herunterzuladen**: Die Recherche-Phase (R1, `docs/research/quellenregister.md`) hat gegen die Primärquellen validiert, dass **Euro+Med PlantBase weder einen bestätigten ColDP-/CDM-Bulk-Export noch eine auffindbare Lizenz** besitzt:

- `europlusmed.org` und das Legacy-Portal `ww2.bgbm.org/EuroPlusMed/` nennen **keine** Nutzungs-/Lizenzbedingungen.
- Über GBIF und ChecklistBank ist **kein** ColDP-/CDM-Vollexport von Euro+Med auffindbar (nur interaktive Web-Abfrage).
- Die im Ausgangsdokument vermutete Checklist-Key-Spur `7ddf754f-…` ist **der COL-XR-Key, nicht Euro+Med** (Fehlspur, in R1 korrigiert).

Ein PoC, der einen nicht existierenden Bulk-Export „verifiziert", wäre wertlos. Das Gate ist damit über die Quellenvalidierung (R1) entschieden, nicht über einen Download-Probe.

## Auswirkung auf SP3

Der Architektur-Entwurf listet Euro+Med als **operativen Backbone** (A.2) und als Taxonomie-Anschluss für EIVE (via EuroSL). Beides ist ohne beziehbare, lizenzierte Euro+Med-Daten nicht wie geplant umsetzbar. Verschärfend (aus P6): die Trait-Tabellen (EIVE/Tichý/Midolo) joinen ohnehin nur über **bare name strings**, nicht über Euro+Med-IDs — ein ID-basierter Euro+Med-Anschluss war also nie verfügbar.

## Fallback (aus R1, für SP3-Planung)

1. **Lizenz-/Zugang klären:** BGBM (Euro+Med-Betreiber) direkt kontaktieren — schriftliche Freigabe + Export anfragen. Erst danach als Backbone aufnehmen.
2. **Bis dahin ohne Euro+Med:** operativen Index auf **WCVP + COL-XR** stützen (beide ✅ CC-BY, P2/P3 grün). COL-XR inkorporiert Euro+Med-Konzepte indirekt über die Catalogue-of-Life-Aggregation.
3. **Trait-Anschluss:** Namens-Crosswalks der drei Trait-Tabellen **direkt gegen WCVP/COL-XR-Namen** bauen (siehe P6), statt über die lizenz-unklaren Zwischenlisten (Euro+Med/EuroSL/GermanSL).

## Konsequenz für den Spec

`dataset.yaml`/A.2 muss Euro+Med als **„pending license clarification"** markieren; SP3 startet ohne Euro+Med als harte Abhängigkeit. Das `sec.`-Beispiel (*Festuca ovina* sec. Euro+Med vs sec. WCVP) bleibt konzeptuell gültig, aber die Euro+Med-Seite ist bis zur Klärung nicht bestückbar.

**Verweis:** `docs/research/quellenregister.md` (Zeile Euro+Med PlantBase, Status ❌, Fußnote [^5]) und `task-R1-report.md`.

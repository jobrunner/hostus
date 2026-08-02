# Von hostus zu iNaturalist-Beobachtungen (UC2)

Diese Anleitung beschreibt den in der Architektur-Spezifikation vorgesehenen
UC2-Fluss: von einer Nutzereingabe über den lokalen hostus-Index bis zu den
tatsächlichen iNaturalist-Beobachtungsdaten eines Taxons — und die
Einschränkungen, die dabei **nicht** ignoriert werden dürfen.

## Der Fluss

```
GET /v1/suggest?q=...  →  Concept auswählen  →  GET /v1/concept/{id}
  →  xrefs.inat[]  →  GET https://api.inaturalist.org/v1/observations?taxon_id=...
```

1. **Autosuggest.** `GET /v1/suggest?q=Corynephorus` liefert Kandidaten-
   Concepts, arealsensitiv gerankt (SP2).
2. **Concept auflösen.** `GET /v1/concept/{concept_id}` liefert das volle
   Concept inklusive `xrefs`.
3. **iNat-ID extrahieren.** `xrefs.inat` ist ein Array, kein einzelner
   String — ein Concept kann mehrere iNat-Taxon-IDs tragen (siehe unten).
   In aller Regel reicht die erste:

   ```json
   { "xrefs": { "inat": ["486076"], "powo": ["396681-1"] } }
   ```

4. **iNaturalist abfragen.** Mit der iNat-`taxon_id` gegen die öffentliche
   iNaturalist-API:

   ```
   GET https://api.inaturalist.org/v1/observations?taxon_id=486076&geoprivacy=open
   ```

## Die 41,50-%-Obergrenze — vor Implementierung lesen

**Nicht jedes hostus-Concept hat eine iNaturalist-Verknüpfung.** Die SP4-
Messung gegen den vollständigen Index (440.534 Concepts) ergab: **182.821
Concepts (41,50 %) tragen eine `inat`-Cross-Reference.** Das ist keine
Ingest-Lücke, sondern die gemessene Obergrenze der ID-basierten Wikidata-
Brücke (POWO/IPNI → Wikidata-Item → `P3151`) — ein Client MUSS also
damit rechnen, dass für **rund 58 % aller Concepts** `xrefs.inat` fehlt
(kein Schlüssel, kein leeres Array), und darf UC2 nicht als garantiert
verfügbar annehmen. Siehe `docs/research/reality-check.md` für die volle
Messung inklusive Konfliktrate.

## Mehrere iNat-IDs pro Concept

Ein Concept kann **mehr als eine** `inat`-ID tragen — z. B. weil zwei
unterschiedliche Wikidata-Items (nach unabhängiger IPNI/POWO-Zuordnung) auf
dasselbe hostus-Concept auflösen, aber jeweils eine eigene iNat-Taxon-ID
tragen. Der Full-Index maß 63 solcher Concepts. `xrefs.inat` ist deshalb
grundsätzlich ein (deterministisch nach ID sortiertes) Array, nie ein
einzelner Wert — eine Implementierung, die nur `xrefs.inat` als String
liest, verliert stillschweigend jede weitere ID.

## P9-Caveats: iNaturalist-Beobachtungsdaten sind kein Rohsignal

Die im Zuge von Poc-Task P9 gemessenen Eigenheiten der iNaturalist-API
gelten für **jede** Auswertung von Beobachtungsdaten, die über `xrefs.inat`
erreicht werden — hostus selbst ingestiert keine Beobachtungsdaten (siehe
[Lizenz-Hinweis unten](#lizenz-hinweis)), aber ein Client, der diesem Fluss folgt,
trifft direkt auf sie:

- **Verdunkelte Koordinaten:** Für geschützte Taxa verdunkelt iNaturalist
  die Koordinaten auf eine Zellgröße von **~26–28 km** (gemessen; nicht die
  in älteren Dokumenten genannten ~20 km). Die reale iNat-API meldet dies
  über die Felder `taxon_geoprivacy`, `geoprivacy` und `obscured` —
  **`coordinates_obscured` existiert nicht** als Feld und darf nicht
  abgefragt werden.
- **Anteil verdunkelter Datensätze:** Bei einem geschützten Taxon sind
  **~32–38 %** der Beobachtungen verdunkelt; **62,6 %** sind ungefiltert
  nutzbar. Ein Client, der Positionsdaten auswertet, muss diesen Anteil
  einkalkulieren, statt ihn stillschweigend zu ignorieren.
- **`public_positional_accuracy` immer anzeigen.** Dieses Feld gibt die
  reale Positionsgenauigkeit an (in Metern) — es MUSS mit angezeigt werden,
  wenn ein Client Koordinaten aus der iNat-API darstellt, damit Nutzer die
  Präzision einschätzen können.
- **`quality_grade=research` ≠ expertengeprüft.** `research`-Grade bedeutet
  **zwei übereinstimmende Community-Einschätzungen**, nicht eine
  taxonomische Expertenverifikation. Ein Client darf `research`-Grade nicht
  als "von einem Experten bestätigt" kommunizieren.

## Lizenz-Hinweis

hostus ingestiert ausschließlich die **Taxon-ID** (`P3151` über die
Wikidata-Brücke) — niemals iNaturalist-**Beobachtungsdaten** selbst.
Einzelne Beobachtungsdatensätze tragen individuelle Beobachter-Lizenzen
(von CC0 bis "alle Rechte vorbehalten"); ein Client, der über diesen Fluss
Beobachtungsdaten abruft und weiterverwendet, muss die Lizenz jedes
einzelnen Datensatzes selbst prüfen (siehe `docs/research/quellenregister.md`,
Fußnote 10).

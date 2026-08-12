# Region-Kombobox mit selbst-beschafften WGSRPD-Namen — Design

**Ziel:** Im Gebiets-Feld der Konsole kann man sowohl `Germany` als auch `GER`
eingeben, und die Auswahl zeigt immer den **ausgeschriebenen Namen neben dem
Code** — weil die WGSRPD-Codes (nicht ISO!) selten erinnert werden.

**Architektur:** Die Gebietsnamen werden **aus der gepinnten Quelle** bezogen
(die `Locality`-Spalte des WCVP-Distributionsdumps), beim Ingest in eine kleine
Lookup-Tabelle geschrieben und über einen neuen `GET /v1/areas`-Endpunkt
ausgeliefert, den die Konsole als Datalist-Kombobox nutzt — dasselbe Muster wie
`GET /v1/sec`.

## Ausgangslage (gemessen im Code)

- `distribution` (`schema.sql`) speichert **nur** `(concept_id, area_scheme,
  area_code)` — **keinen Namen**.
- `internal/adapters/wcvp/reader.go::DistributionRow` liest `Locality` (der
  Klarname, z. B. „Germany") und `LocationID` (`TDWG:<L3-Code>`), aber der
  Ingest verwirft `Locality` und behält nur den Code.
- Es gibt eine kleine Alias-Tabelle `areaCodes` (`suggest.go`: `DE`→`GER`, `AT`,
  `CH`); `?area=` nimmt genau einen Wert.
- `GET /v1/sec` (jüngst gebaut) ist die Vorlage: Endpoint + DTO + Route +
  OpenAPI + Registry + Konsolen-Datalist.

## Komponenten

### 1. Schema + Ingest: Gebietsnamen selbst beschaffen
- Schema (`schema.sql`): neue Tabelle
  `area (scheme TEXT, code TEXT, name TEXT, PRIMARY KEY (scheme, code))`. Die
  Schema-Spaltenprüfung in `db.go` wird erweitert.
- `application.DistributionRow` + der WCVP-Adapter reichen `Locality` zusätzlich
  zum `AreaCode` durch; der Ingest schreibt je (scheme, code) genau einen
  `area`-Eintrag (`INSERT OR IGNORE`, erster nicht-leerer Name gewinnt —
  deterministisch, weil die Zeilen in Dateireihenfolge kommen; ein leeres
  `Locality` schreibt keinen Namen).

### 2. Port + sqlite: Gebiete mit Daten auflisten
- `output.Repository` erhält `Areas(ctx) ([]domain.Area, error)` mit
  `domain.Area{Scheme, Code, Name}`.
- sqlite-Impl: `SELECT DISTINCT d.area_scheme, d.area_code, COALESCE(a.name, '')
  FROM distribution d LEFT JOIN area a ON a.scheme = d.area_scheme AND a.code =
  d.area_code ORDER BY d.area_scheme, d.area_code` — **nur Codes mit
  Distribution-Daten**, je mit Namen (leer, wenn die Quelle keinen lieferte).

### 3. API: `GET /v1/areas`
- Handler `handleAreas(repo)` → `{"areas":[{code, name, scheme}]}`, `[]` (nie
  `null`) bei leerem Index, `500 INTERNAL_ERROR` bei Repo-Fehler. Muster exakt
  wie `handleSec`.
- Route in `router.go` (im `deps.Repo != nil`-Block), `GET`.
- `areaListResponseDTO` + `areaDTO`; OpenAPI-Pfad + `AreaListResponse`/`Area`-
  Component-Schemas + Registry-Einträge (Schema-Contract-Test erzwingt sie) +
  `http-api.md`-Abschnitt.

### 4. Konsole: Kombobox Name + Code
- `index.html`: das `suggest-area`-Freitextfeld erhält `list="areas"` +
  `<datalist id="areas">`.
- `app.js`: beim Laden `GET /v1/areas` holen und je Gebiet eine
  `<option value="GER" label="Germany (GER)">` einfügen (Datalist zeigt Label,
  schreibt beim Auswählen den `value` = Code ins Feld). Fällt bei Fehler auf
  Freitext zurück (wie die `/v1/sec`-Datalist).
- **Namenseingabe „Germany"** löst client-seitig auf den Code auf, bevor
  `?area=` gerufen wird: die Konsole hält die `/v1/areas`-Liste und mappt eine
  eingegebene (Teil-)Bezeichnung case-insensitiv auf den Code; ist die Eingabe
  bereits ein bekannter Code, bleibt sie unverändert. So funktioniert sowohl
  „Germany" als auch „GER" im selben Feld.

### 5. Server bleibt code-basiert
`?area=` nimmt weiterhin einen WGSRPD-L3-Code (plus die bestehenden `areaCodes`-
Aliase `DE/AT/CH`). Kein server-seitiges Namens-Parsing — die Auflösung
„Germany"→`GER` ist eine reine Konsolen-Bequemlichkeit (bewusste Entscheidung,
minimaler Server-Eingriff).

## Datenfluss

Ingest: `wcvp_distribution.csv` Zeile `Locality="Germany", LocationID="TDWG:GER"`
→ `area(scheme='wgsrpd_l3', code='GER', name='Germany')`.
Runtime: Konsole lädt `GET /v1/areas` → `[{code:"GER", name:"Germany", …}]` →
Datalist „Germany (GER)". Nutzer tippt „Germany" → Konsole mappt auf `GER` →
`GET /v1/suggest?...&area=GER`.

## Fehlerbehandlung

- Fehlender/leerer Name: `area.name` bleibt leer; die Datalist zeigt dann nur
  den Code (kein Absturz). `GET /v1/areas` liefert den Code trotzdem.
- `GET /v1/areas`-Fehler: `500 INTERNAL_ERROR`; Konsole bleibt Freitextfeld.

## Tests (TDD)

- **sqlite (ingest):** eine Distribution-Zeile mit `Locality` schreibt genau
  einen `area`-Eintrag; ein leeres `Locality` schreibt keinen Namen; derselbe
  Code aus zwei Zeilen bleibt ein Eintrag.
- **sqlite (Areas):** listet nur Codes mit Distribution-Daten, je mit Namen;
  ein Code ohne `area`-Zeile kommt mit leerem Namen.
- **http:** `handleAreas` — Liste, `[]`-nicht-`null`, 500-Pfad (mit
  INTERNAL_ERROR-Envelope); Schema-Contract grün; Routen-Contract grün.
- **Konsole:** Serve-Smoke gegen die reale DB — `/v1/areas` liefert Gebiete mit
  Namen; „Germany" im Feld resolvt zu `GER` und liefert Suggest-Ergebnisse.

## Bewusst außerhalb des Scopes

- **Nur Gebiete mit Daten** (nicht die volle ~370er-WGSRPD-Liste).
- Kein Multi-Gebiet im Suggest (`?area=` bleibt ein Code; Mehrgebiet gibt es nur
  in `hostus bundle`).
- Keine server-seitige Namensauflösung.

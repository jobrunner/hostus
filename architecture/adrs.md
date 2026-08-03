# Architecture Decision Records (ADR)

## ADR‑001: Nutzung von GBIF als externer Taxonomie‑Provider

**Status:** Superseded — siehe [ADR-0009: Lokaler Multi-Backbone-Index](../docs/explanation/decisions/0009-local-multibackbone-index.md) (hostus 2.0)

**Kontext:**
Für die Ermittlung eindeutiger Pflanzennamen inkl. historischer Synonyme wird eine belastbare, öffentlich verfügbare Quelle benötigt.

**Entscheidung:**
GBIF wird als alleiniger externer Taxonomie‑Provider genutzt.

**Begründung:**

* Breite taxonomische Abdeckung
* Synonym‑Beziehungen maschinenlesbar
* REST‑API verfügbar

**Konsequenzen:**

* Abhängigkeit von externer Verfügbarkeit
* Notwendigkeit von Caching und Timeouts

---

## ADR‑002: Synonyme werden unter akzeptierten Taxa gruppiert

**Status:** Accepted

**Kontext:**
Autosuggest‑UX darf Nutzer nicht mit widersprüchlichen Namen überfordern.

**Entscheidung:**
Synonyme werden stets unter dem akzeptierten Taxon dargestellt und explizit markiert.

**Konsequenzen:**

* Fachlich klare Ergebnisse
* Historische Namen bleiben auffindbar

---

## ADR‑003: Kein Persistenz‑Layer

**Status:** Superseded — siehe [ADR-0009: Lokaler Multi-Backbone-Index](../docs/explanation/decisions/0009-local-multibackbone-index.md) und [ADR-0010: SQLite/FTS5-Persistenz](../docs/explanation/decisions/0010-sqlite-fts5-persistence.md) (hostus 2.0)

**Kontext:**
Der Service soll leichtgewichtig bleiben und kein System‑of‑Record sein.

**Entscheidung:**
Keine Datenbank, nur In‑Memory‑Cache.

**Konsequenzen:**

* Sehr einfache Betriebsführung
* Keine Migrationen

---

## ADR‑004: Go + Minimal‑Dependencies

**Status:** Accepted (Wortlaut für hostus 2.0 reconciled)

**Entscheidung:**
Go mit möglichst wenig Abhängigkeiten. Für hostus 2.0 zählt dazu explizit der
im Master-Architektur-Spec freigegebene, feste Stack: `gorilla/mux`,
`spf13/viper`, `spf13/cobra`, `modernc.org/sqlite` (CGO-frei), das
OpenTelemetry-Go-SDK (+ `otelmux`), `modelcontextprotocol/go-sdk` und der
offizielle Prometheus-Client — siehe `CLAUDE.md` „Allowed Libraries Only" für
die maßgebliche, aktuelle Liste. Kein Wildwuchs darüber hinaus; keine
schweren Frameworks, ORMs oder reflection-lastigen Abhängigkeiten.

**Begründung:**

* Performance
* Wartbarkeit
* Vorhersagbares Laufzeitverhalten

---

## ADR‑005: Code‑First OpenAPI

**Status:** Accepted

**Entscheidung:**
OpenAPI wird aus dem Code generiert.

**Konsequenzen:**

* API und Spec bleiben synchron
* CI erzwingt Konsistenz

---

## ADR‑006: Distroless Container

**Status:** Accepted

**Entscheidung:**
Auslieferung ausschließlich als distroless Container.

**Begründung:**

* Minimale Angriffsfläche
* Kleine Images

---

## ADR‑007: Releases nur via Feature‑Merge

**Status:** Accepted

**Entscheidung:**
Releases werden nur bei Feature‑Branch‑Merges erzeugt.

**Konsequenzen:**

* Saubere Release‑Historie
* Zwang zu expliziten Versionen

---

## ADR‑008: Explizite Nicht‑Ziele

**Status:** Superseded (teilweise) — „keine Persistenz" ersetzt durch [ADR-0009](../docs/explanation/decisions/0009-local-multibackbone-index.md)/[ADR-0010](../docs/explanation/decisions/0010-sqlite-fts5-persistence.md); „keine Auth, keine User" gilt für hostus 2.0 unverändert fort (siehe Spec Abschnitt 8, Nicht-Ziele)

**Entscheidung:**
Keine Auth, keine User, keine Persistenz.

**Begründung:**

* Klarer Scope
* Geringe Komplexität

---

## Ende der ADRs


# Architecture Decision Records (ADR)

## ADR‑001: Nutzung von GBIF als externer Taxonomie‑Provider

**Status:** Accepted

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

**Status:** Accepted

**Kontext:**
Der Service soll leichtgewichtig bleiben und kein System‑of‑Record sein.

**Entscheidung:**
Keine Datenbank, nur In‑Memory‑Cache.

**Konsequenzen:**

* Sehr einfache Betriebsführung
* Keine Migrationen

---

## ADR‑004: Go + Minimal‑Dependencies

**Status:** Accepted

**Entscheidung:**
Go mit möglichst wenig Abhängigkeiten.

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

**Status:** Accepted

**Entscheidung:**
Keine Auth, keine User, keine Persistenz.

**Begründung:**

* Klarer Scope
* Geringe Komplexität

---

## Ende der ADRs


# Spec: Serve-Lese-Pool und funktionierender Load-Shedder (Paket B)

Datum: 2026-09-05
Status: verabschiedet (Folge von Paket A, `2026-09-05-console-abort-stale-guards.md`)

## Kontext

1. **Ein-Verbindungs-Pool auf dem Serve-Pfad:** `sqlite.Open` pinnt den Pool
   auf `SetMaxOpenConns(1)` (db.go:59). Die dokumentierten Gründe — FK-Pragma
   ist per-Connection und betrifft nur SCHREIBENDE Statements; `:memory:` hat
   pro Connection eine eigene Datenbank; Single-Writer-Ingest — gelten für
   Ingest und Tests, nicht für den read-only Serve-Pfad. Folge im Betrieb:
   JEDER Request serialisiert hinter dem jeweils laufenden (ein teurer
   Fuzzy-Match blockiert alle Suggests); unter Tipp-Last staut sich die
   `database/sql`-Warteschlange, der Reverse-Proxy antwortet mit 502. Die DB
   ist WAL-journaled (db.go:43) — mehrere gleichzeitige LESER sind genau der
   Fall, den WAL unterstützt.
2. **Toter Load-Shedder:** `middleware.LoadShedder.RecordError/RecordSuccess`
   haben KEINEN Aufrufer im gesamten Repo — der Shedder kann nie aktivieren
   und die LoadShed-Middleware ist ein No-Op-Relikt aus v1.x, obwohl
   CLAUDE.md „load shedding" als Kernverantwortung listet und die
   Middleware-Kette ihn führt.

## Entscheidungen

1. **Lese-Pool für den Serve-Pfad:** Neue Funktion
   `sqlite.OpenPool(path string, maxConns int) (*DB, error)`;
   `Open(path)` bleibt exakt `OpenPool(path, 1)` (Ingest, Tests,
   `:memory:` — unverändert). Guards in OpenPool:
   - `path == ":memory:"` erzwingt 1 (per-Connection-Datenbank);
   - `maxConns < 1` fällt auf 1 zurück.
   Die per-Connection-Pragmas (`_journal_mode=WAL`, `_busy_timeout=5000`)
   kommen bereits aus dem DSN und gelten damit für jede Pool-Verbindung.
   FK-Enforcement ist für den read-only Serve-Pfad irrelevant (FKs werden
   nur bei Schreib-Statements geprüft); das Schema wird weiterhin einmal
   beim Open angewandt.
2. **Wiring:** NUR der Serve-Pfad (Komposition in `internal/app`, dort wo
   `hostus serve` das Repo öffnet) benutzt `OpenPool(path, n)`. Ingest,
   Bundle, Validate und alle Tests bleiben auf `Open` (1 Verbindung).
3. **Konfiguration:** Viper-Schlüssel `sqlite.max_read_conns`
   (Default **4**; env `HOSTUS_SQLITE_MAX_READ_CONNS` via bestehendem
   Prefix-Mechanismus). Default 4 statt NumCPU: konservativ, deckt die
   Tipp-Parallelität einer Konsole ab, ohne Checkpoint-Konkurrenz zu
   provozieren; im `config.yaml.example` mit Kommentar dokumentieren.
4. **Load-Shedder verdrahten statt entfernen** (CLAUDE.md nennt Load
   Shedding als Kernverantwortung): Die `LoadShed`-Middleware beobachtet
   den Response-Status des inneren Handlers über einen
   Status-erfassenden ResponseWriter-Wrapper und ruft
   `RecordError()` bei Status ≥ 500, sonst `RecordSuccess()`.
   Sheds selbst (der 503-Kurzschluss VOR next) zählen nicht als Fehler —
   sonst hielte sich der Shedder selbst aktiv. Semantik unverändert:
   Threshold aufeinanderfolgende 5xx → shed; nach Backoff Probe-Request;
   Erfolg resettet. Defaults bleiben (Threshold 1000, Backoff 5 s) — ein
   Sicherheitsventil gegen Dauerfehler (z. B. kaputte DB-Datei), kein
   Latenz-Regler.
5. **Metriken/Sichtbarkeit:** bestehende Gauge `LoadSheddingActive` bleibt;
   keine neuen Metriken in diesem Paket.

## Risiken & Grenzen

- Mehrere Lese-Verbindungen erhöhen die Speichernutzung (Page-Cache pro
  Verbindung) — bei 4 Verbindungen unkritisch.
- Der Shedder bleibt bewusst grob (consecutive errors), er ist KEIN Ersatz
  für Request-Priorisierung; die 502-Ursache adressiert primär der Pool
  plus Paket A.
- `hostus mcp`/Debug-Pfade: bleiben auf `Open` — nur explizites
  Serve-Wiring wechselt.

## Projektweite Anforderungen

- `internal/adapters/sqlite` ist gremlins-HEAVY (CI nur wöchentlich):
  lokaler `make mutation PKG=./internal/adapters/sqlite`-Lauf ist Teil der
  Abschluss-Verifikation. `internal/config` und `internal/app` sind im
  normalen Gate; `internal/middleware` hat kein CI-Gate, Tests trotzdem
  mutation-tauglich schreiben.
- Hexagon-Grenzen; CHANGELOG unter `## [Unreleased]`; Conventional Commits;
  englische WHY-Doc-Kommentare.

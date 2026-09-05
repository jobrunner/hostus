# Serve-Lese-Pool + Load-Shedder Implementation Plan (Paket B)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Der Serve-Pfad liest mit einem konfigurierbaren Verbindungs-Pool (Default 4) statt einer Einzelverbindung, und der bislang tote Load-Shedder beobachtet tatsächlich Response-Status.

**Architecture:** `sqlite.OpenPool(path, maxConns)` mit `Open(path) == OpenPool(path, 1)`; nur `internal/app`'s Serve-Wiring (`openRepo`, app.go:143) wechselt auf den Pool, gespeist aus neuem Config-Schlüssel `sqlite.max_read_conns`. Die `LoadShed`-Middleware erfasst den Status des inneren Handlers und ruft `RecordError`/`RecordSuccess`.

**Tech Stack:** Go 1.26, modernc.org/sqlite (WAL bereits aktiv), viper.

**Spec:** `docs/superpowers/specs/2026-09-05-serve-read-pool-loadshed.md`

## Global Constraints

- `Open(path)` bleibt byte-identisch in Verhalten (Ingest/Bundle/Validate/Export/Tests unverändert auf 1 Verbindung); NUR `internal/app/app.go:openRepo` (Serve) wechselt auf `OpenPool`.
- OpenPool-Guards: `":memory:"` → 1; `maxConns < 1` → 1. Beide Zweige mutanten-tötend getestet.
- Load-Shedder: der 503-Kurzschluss (shed) ruft WEDER RecordError NOCH RecordSuccess; Status ≥ 500 vom inneren Handler → RecordError, sonst RecordSuccess. Alle drei Pfade getestet.
- `internal/config` und `internal/app` sind mutation-gated; `internal/adapters/sqlite` ist gremlins-heavy → lokaler Lauf in der Abschluss-Verifikation.
- CHANGELOG unter `## [Unreleased]`; Conventional Commits; englische WHY-Doc-Kommentare mit Spec-Bezug.
- KEIN `git add -A`/`git add .` — nur benannte Dateien stagen.

---

### Task 1: `sqlite.OpenPool` + Config + Serve-Wiring

**Files:**
- Modify: `internal/adapters/sqlite/db.go` (OpenPool, Open delegiert)
- Modify: `internal/config/config.go` (SQLiteConfig.MaxReadConns, Default 4), `config.yaml.example` (Kommentar), `example.env` (Beispielzeile mit englischer Beschreibung)
- Modify: `internal/app/app.go` (openRepo → OpenPool)
- Test: `internal/adapters/sqlite/db_pool_internal_test.go` (neu), bestehende config-/app-Tests erweitern (Muster nachschlagen)

**Interfaces:**
- Produces: `func OpenPool(path string, maxConns int) (*DB, error)`; `config.SQLiteConfig.MaxReadConns int` (`mapstructure:"max_read_conns"`, Default 4).
- Consumes: bestehendes `Open`-Gerüst (DSN mit `_journal_mode=WAL&_busy_timeout=5000`, Schema-Apply, Migrationen, verifySchemaColumns).

- [ ] **Step 1: Fehlschlagende Tests schreiben**

`internal/adapters/sqlite/db_pool_internal_test.go` (internes Testpackage, wie db_internal_test.go):

```go
// TestOpenPoolGuards pins spec 2026-09-05 decision 1: ":memory:" must stay
// on ONE connection (each pooled connection would otherwise see its own
// empty database), and a nonsensical maxConns falls back to 1 instead of
// an unlimited pool.
func TestOpenPoolGuards(t *testing.T) {
	cases := []struct {
		name     string
		path     string
		maxConns int
		want     int
	}{
		{"memory forces one", ":memory:", 4, 1},
		{"zero falls back to one", filepath.Join(t.TempDir(), "a.db"), 0, 1},
		{"negative falls back to one", filepath.Join(t.TempDir(), "b.db"), -3, 1},
		{"file db honors pool size", filepath.Join(t.TempDir(), "c.db"), 4, 4},
	}
	for _, c := range cases {
		db, err := OpenPool(c.path, c.maxConns)
		if err != nil {
			t.Fatalf("%s: OpenPool: %v", c.name, err)
		}
		if got := db.sql.Stats().MaxOpenConnections; got != c.want {
			t.Errorf("%s: MaxOpenConnections = %d, want %d", c.name, got, c.want)
		}
		_ = db.Close()
	}
}

// TestOpenPoolServesConcurrentReaders proves the point of the pool: two
// overlapping readers on a file-backed WAL database proceed concurrently
// (with one connection the second would queue behind the first). Uses a
// slow-ish query via a large generate-like scan? Keep it simple and
// deterministic instead: two goroutines each holding an open rows cursor
// simultaneously — with MaxOpenConns(1) the second Query blocks until the
// first cursor closes; with the pool both cursors are open at once.
func TestOpenPoolServesConcurrentReaders(t *testing.T) {
	// Implementierung: file-DB mit OpenPool(path, 2); Tabelle mit ein paar
	// Zeilen anlegen (direkt via db.sql.Exec, internes Testpackage);
	// Query 1 öffnen, ERSTE Zeile lesen, Cursor OFFEN halten; dann mit
	// Timeout-Context (2s) Query 2 ausführen und vollständig lesen.
	// Assertion: Query 2 gelingt, solange Cursor 1 offen ist. Gegenprobe im
	// selben Test mit Open(path2) (1 Verbindung): Query 2 läuft in den
	// Context-Timeout. Beide Zweige assertieren, sonst ist der Test blind.
}
```

`internal/config`-Test (bestehende Default-Tests erweitern): `sqlite.max_read_conns` Default == 4; env-Override `HOSTUS_SQLITE_MAX_READ_CONNS=8` greift (Muster der bestehenden env-Tests in config nachschlagen und exakt folgen).

`internal/app`: bestehender openRepo-/Serve-Test — prüfe, wie openRepo getestet ist; mindestens ein Test, der pinnt, dass cfg.SQLite.MaxReadConns an OpenPool durchgereicht wird (ggf. indirekt über Stats des geöffneten Repos, wenn openRepo das *DB exponiert — sonst als ⚠ im Report benennen und die Verifikation dem Controller-E2E überlassen; NICHT dafür die Architektur verbiegen).

- [ ] **Step 2: Fehlschlag verifizieren**

Run: `go test ./internal/adapters/sqlite/ -run TestOpenPool -v` → FAIL (OpenPool existiert nicht); config-Test → FAIL.

- [ ] **Step 3: Implementierung**

`db.go`: bestehenden `Open`-Rumpf nach `OpenPool` verschieben; Kopf:

```go
// OpenPool opens path with a pool of up to maxConns connections. Open's
// single-connection contract (see its doc comment) exists for WRITERS and
// for ":memory:" — the serve path is read-only on a WAL database, which is
// exactly the concurrent-reader case WAL supports, and one connection
// serialized every request behind whichever query happened to run
// (measured 2026-09-05: keystroke bursts on /v1/suggest queued on the
// single connection until the reverse proxy answered 502; spec
// 2026-09-05-serve-read-pool-loadshed.md). The per-connection pragmas
// (_journal_mode, _busy_timeout) come from the DSN and therefore apply to
// every pooled connection; FK enforcement is a write-time concern and
// stays pinned to Open's single connection for every writing caller.
//
// ":memory:" always uses ONE connection regardless of maxConns — each
// physical connection gets its own private empty database. maxConns < 1
// falls back to 1.
func OpenPool(path string, maxConns int) (*DB, error) {
	if path == ":memory:" || maxConns < 1 {
		maxConns = 1
	}
	...bisheriger Open-Rumpf, SetMaxOpenConns(maxConns)...
}

// Open opens path pinned to exactly one physical connection — the contract
// every WRITING caller (ingest, bundle, export) and every ":memory:" test
// relies on. See OpenPool for the read-pool variant the serve path uses.
func Open(path string) (*DB, error) { return OpenPool(path, 1) }
```
Den bestehenden Pool-Doc-Kommentar (db.go:48-58) zu OpenPool mitnehmen und um den Lese-Pool-Absatz ergänzen — nicht duplizieren.

`config.go`: `MaxReadConns int \`mapstructure:"max_read_conns"\`` in SQLiteConfig; `viper.SetDefault("sqlite.max_read_conns", 4)` neben dem bestehenden sqlite.path-Default. `config.yaml.example`: unter `sqlite:` dokumentieren (Kommentar: nur Serve-Lesepool; Ingest bleibt 1). `example.env`: `# Maximum read connections for the serve path (default 4)` + `HOSTUS_SQLITE_MAX_READ_CONNS=4`.

`app.go:147`: `sqlite.Open(cfg.SQLite.Path)` → `sqlite.OpenPool(cfg.SQLite.Path, cfg.SQLite.MaxReadConns)`; Doc-Kommentar von openRepo um einen Satz ergänzen (read-only Pool, Spec-Verweis).

- [ ] **Step 4: Tests laufen lassen**

Run: `go test ./internal/adapters/sqlite/ ./internal/config/ ./internal/app/` → PASS; dann `go test ./...`, `make lint`.

- [ ] **Step 5: Commit**

```bash
git add internal/adapters/sqlite/db.go internal/adapters/sqlite/db_pool_internal_test.go internal/config/config.go config.yaml.example example.env internal/app/app.go <config/app testfiles>
git commit -m "feat(serve): konfigurierbarer SQLite-Lese-Pool statt Einzelverbindung"
```

---

### Task 2: Load-Shedder verdrahten + CHANGELOG

**Files:**
- Modify: `internal/middleware/loadshed.go` (LoadShed beobachtet Status)
- Test: `internal/middleware/loadshed_test.go` (erweitern)
- Modify: `CHANGELOG.md`

**Interfaces:**
- Consumes: bestehende `LoadShedder.RecordError/RecordSuccess/ShouldShed`.
- Produces: keine neuen Exporte (der Status-Writer bleibt paketintern; falls logging.go/metrics.go bereits einen Status-erfassenden Wrapper haben, DIESEN wiederverwenden statt einen dritten zu bauen — `git grep -n "WriteHeader" internal/middleware/`).

- [ ] **Step 1: Fehlschlagende Tests schreiben** (Tabellen-Test in loadshed_test.go, Muster der Datei):
  - Handler antwortet 500 × threshold → nächster Request wird geshedded (503, `UPSTREAM_OVERLOADED`).
  - Handler antwortet 200 nach Fehlern unterhalb des Thresholds → Zähler resettet (kein Shed nach weiteren threshold-1 Fehlern… als eigener Fall: Fehler, Erfolg, Fehler×(threshold-1) → KEIN Shed).
  - Shed-Antworten selbst erhöhen den Fehlerzähler nicht: nach Ablauf des Backoffs lässt der Shedder den Probe-Request durch (bestehendes Verhalten bleibt beobachtbar).
  - 4xx zählt als Erfolg (RecordSuccess) — Client-Fehler sind keine Upstream-Fehler.

- [ ] **Step 2: Fehlschlag verifizieren** — `go test ./internal/middleware/ -run TestLoadShed -v` → FAIL.

- [ ] **Step 3: Implementierung** in `LoadShed`:

```go
func LoadShed(shedder *LoadShedder) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if shedder.ShouldShed() {
				// The shed short-circuit itself records NOTHING: counting our
				// own 503s as errors would keep the breaker latched forever.
				httperr.UpstreamOverloadedError(w)
				return
			}
			sw := ...statusCapturingWriter (wiederverwenden, s. Interfaces)...
			next.ServeHTTP(sw, r)
			if sw.status >= http.StatusInternalServerError {
				shedder.RecordError()
			} else {
				shedder.RecordSuccess()
			}
		})
	}
}
```
Doc-Kommentar am File-Kopf: der Shedder war seit dem v2-Umbau ein No-Op (RecordError hatte keinen Aufrufer — Audit 2026-09-05); Semantik = Sicherheitsventil gegen konsekutive 5xx, kein Latenz-Regler.

- [ ] **Step 4: Tests** — `go test ./internal/middleware/` + `go test ./...` + `make lint` → PASS.

- [ ] **Step 5: CHANGELOG** unter `## [Unreleased]`:

```markdown
### Added
* **Serve-Lese-Pool:** `hostus serve` liest jetzt mit bis zu
  `sqlite.max_read_conns` (Default 4, env `HOSTUS_SQLITE_MAX_READ_CONNS`)
  parallelen SQLite-Verbindungen statt einer einzigen — unter Tipp-Last
  (Suggest) serialisierte bisher jeder Request hinter dem laufenden Query,
  was ein vorgeschalteter Reverse-Proxy als 502 quittierte. Ingest,
  Bundle-Export und `:memory:`-Betrieb bleiben unverändert bei einer
  Verbindung.

### Fixed
* **Load-Shedding funktioniert wieder:** Der Load-Shedder war seit dem
  v2-Umbau ein No-Op (kein Aufrufer von RecordError) — die Middleware
  beobachtet jetzt den Response-Status und öffnet den Breaker nach
  aufeinanderfolgenden 5xx-Antworten (Threshold unverändert 1000,
  Backoff 5 s; Sicherheitsventil, kein Latenz-Regler).
```

- [ ] **Step 6: Commit**

```bash
git add internal/middleware/loadshed.go internal/middleware/loadshed_test.go CHANGELOG.md
git commit -m "fix(middleware): Load-Shedder beobachtet Response-Status statt nie zu feuern"
```

---

## Verifikation nach Abschluss (Controller, kein Task)

1. `make verify`; `make mutation PKG=./internal/adapters/sqlite` (heavy, lokal), `PKG=./internal/config`, `PKG=./internal/app`.
2. E2E gegen Produktions-DB: `hostus serve` mit Default-Pool; paralleles Feuern von 8 Suggest-Requests (`curl` parallel) → alle antworten, Latenzverteilung deutlich flacher als mit `HOSTUS_SQLITE_MAX_READ_CONNS=1`; Ingest-Regression: `hostus ingest` in frische DB läuft unverändert (Ingest bleibt Einzelverbindung).

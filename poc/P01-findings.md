# P01 — Findings: FTS5 support in modernc.org/sqlite

**Question**: Does the pure-Go SQLite driver `modernc.org/sqlite` support FTS5 with
prefix queries and bm25 ranking out of the box? (SP1/SP2 depend on this for the
taxa autosuggest search index.)

**Probe**: [`p01_fts5/main.go`](./p01_fts5/main.go)

**Environment**: `go version go1.26.5 darwin/arm64` (via `nix develop`),
`modernc.org/sqlite v1.55.0` (latest at time of testing, pulled via `go get`).

## Command

```bash
cd poc/p01_fts5
nix develop -c bash -lc 'go get modernc.org/sqlite && go mod tidy && go run .'
```

## Output (actual)

```
FTS5 virtual table created successfully.

-- prefix match 'coryn*' ordered by implicit rank --
SQL: SELECT canonical, rank FROM t WHERE t MATCH 'coryn*' ORDER BY rank
columns: [canonical rank]
Corynephorus canescens -0.5877866649021191
Corynephorus divaricatus -0.5877866649021191

-- prefix match 'coryn*' ordered by explicit bm25(t) --
SQL: SELECT canonical, bm25(t) AS score FROM t WHERE t MATCH 'coryn*' ORDER BY score
columns: [canonical score]
Corynephorus canescens -0.5877866649021191
Corynephorus divaricatus -0.5877866649021191

-- exact token match 'ovina' --
SQL: SELECT canonical, rank FROM t WHERE t MATCH 'ovina' ORDER BY rank
columns: [canonical rank]
Festuca ovina -1.2992829841302609

-- remove_diacritics: search 'demersum' (no accent) against accented row --
SQL: SELECT canonical, rank FROM t WHERE t MATCH 'demersum' ORDER BY rank
columns: [canonical rank]
Ceratophyllum démersum -1.2992829841302609

-- remove_diacritics + prefix: search 'demer*' --
SQL: SELECT canonical, rank FROM t WHERE t MATCH 'demer*' ORDER BY rank
columns: [canonical rank]
Ceratophyllum démersum -1.2992829841302609

Done.
```

## Findings

1. **FTS5 works out of the box, no build tag required.** `CREATE VIRTUAL TABLE t
   USING fts5(...)` succeeded immediately against `modernc.org/sqlite v1.55.0`
   with a plain `go get modernc.org/sqlite`. No CGO, no special import path, no
   `-tags` flag. (modernc.org/sqlite is a pure-Go transpile of the SQLite C
   amalgamation, and FTS5 is compiled in as part of that amalgamation by
   default — unlike `mattn/go-sqlite3`, which needs `-tags sqlite_fts5`.)

2. **Prefix queries (`term*`) work correctly.** `coryn*` matched both
   `Corynephorus canescens` and `Corynephorus divaricatus` and excluded all
   other rows (including the decoy `Cortaderia selloana`, which shares the
   `Cor` prefix but not the `Coryn` token prefix — confirms FTS5 does
   token-prefix matching, not substring matching).

3. **`rank` and `bm25(t)` are both available and agree.** The implicit `rank`
   column (FTS5's built-in ordering column, bm25-based by default when no
   explicit ranking function is registered) and the explicit `bm25(t)` scalar
   function returned identical scores for the same query. Scores are negative
   floats where **more negative = more relevant** (this is the standard SQLite
   FTS5 bm25 convention — ascending `ORDER BY rank`/`ORDER BY bm25(t)` yields
   best-match-first). Scores are per-document and comparable/orderable within
   a result set, confirming SP1/SP2 can rank suggestions by relevance without
   writing a custom scoring function.

4. **`tokenize = 'unicode61 remove_diacritics 2'` folds diacritics as
   expected.** A query for `demersum` (plain ASCII, no accent) matched the
   seeded row `Ceratophyllum démersum` (with an accented "é"), both as an
   exact-token match and as a prefix match (`demer*`). This confirms accent-
   insensitive search works for scientific-name lookups where users may not
   type diacritics (relevant for German-locale users typing Latin binomials).

5. No errors, no panics, no "no such module: fts5" — the failure mode this
   PoC was checking for did not occur at all.

## Verdict

**Verdict: 🟢 (assumption holds — SP1/SP2 can use modernc FTS5)**

`modernc.org/sqlite` v1.55.0 supports FTS5 fully out of the box: virtual
table creation, token-prefix queries (`term*`), bm25-based ranking via both
the implicit `rank` column and the explicit `bm25(t)` function, and
diacritic-insensitive matching via `unicode61 remove_diacritics 2`. No build
tags, no CGO, no driver workarounds needed. SP1/SP2 can proceed on the
assumption that a single pure-Go SQLite dependency provides the full-text
search + ranking substrate for the taxa autosuggest index.

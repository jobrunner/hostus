package sqlite

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

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

// seedPoolTestTable creates a two-row scratch table on db, opens the first
// Query cursor and reads one row, then leaves that cursor DELIBERATELY OPEN
// (not closed, not drained, only released via t.Cleanup at the very end of
// the subtest) — the "second reader arrives while the first is still using
// its connection" setup both branches of TestOpenPoolServesConcurrentReaders
// need.
func seedPoolTestTable(t *testing.T, db *DB) {
	t.Helper()
	if _, err := db.sql.Exec(`CREATE TABLE t (id INTEGER PRIMARY KEY, v TEXT)`); err != nil {
		t.Fatalf("creating table: %v", err)
	}
	if _, err := db.sql.Exec(`INSERT INTO t (v) VALUES ('a'), ('b')`); err != nil {
		t.Fatalf("seeding table: %v", err)
	}
	rows, err := db.sql.Query(`SELECT id, v FROM t ORDER BY id`)
	if err != nil {
		t.Fatalf("query 1: %v", err)
	}
	t.Cleanup(func() { _ = rows.Close() })
	if !rows.Next() {
		t.Fatalf("query 1: expected at least one row")
	}
}

// TestOpenPoolServesConcurrentReaders proves the point of the pool: two
// overlapping readers on a file-backed WAL database proceed concurrently
// (with one connection the second would queue behind the first). Two
// goroutines each hold an open rows cursor simultaneously — with
// MaxOpenConns(1) the second Query blocks until the first cursor closes;
// with the pool both cursors are open at once.
func TestOpenPoolServesConcurrentReaders(t *testing.T) {
	dir := t.TempDir()

	t.Run("pooled connections serve a concurrent reader", func(t *testing.T) {
		pooled, err := OpenPool(filepath.Join(dir, "pooled.db"), 2)
		if err != nil {
			t.Fatalf("OpenPool: %v", err)
		}
		t.Cleanup(func() { _ = pooled.Close() })
		seedPoolTestTable(t, pooled) // cursor 1 stays open

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		rows2, err := pooled.sql.QueryContext(ctx, `SELECT id, v FROM t ORDER BY id`)
		if err != nil {
			t.Fatalf("query 2 failed while cursor 1 was open (pool should allow concurrent readers): %v", err)
		}
		defer func() { _ = rows2.Close() }()
		count := 0
		for rows2.Next() {
			count++
		}
		if err := rows2.Err(); err != nil {
			t.Fatalf("iterating query 2: %v", err)
		}
		if count != 2 {
			t.Fatalf("query 2 read %d rows, want 2", count)
		}
	})

	t.Run("single connection serializes into a context timeout", func(t *testing.T) {
		single, err := Open(filepath.Join(dir, "single.db"))
		if err != nil {
			t.Fatalf("Open: %v", err)
		}
		t.Cleanup(func() { _ = single.Close() })
		seedPoolTestTable(t, single) // cursor 1 stays open

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_, err = single.sql.QueryContext(ctx, `SELECT id, v FROM t ORDER BY id`)
		if err == nil {
			t.Fatal("query 2 succeeded while cursor 1 was open and connections are capped at 1 — expected it to block into the context timeout")
		}
		if !isContextDeadlineErr(err) {
			t.Fatalf("query 2 failed with %v, want a context-deadline error", err)
		}
	})
}

// isContextDeadlineErr accepts either the direct context error the driver
// may surface, or SQLITE_BUSY wrapped in it — both are the "second reader
// never got a connection before the timeout" outcome this test checks for.
func isContextDeadlineErr(err error) bool {
	return errors.Is(err, context.DeadlineExceeded) ||
		strings.Contains(err.Error(), "context deadline exceeded") ||
		strings.Contains(err.Error(), "database is locked")
}

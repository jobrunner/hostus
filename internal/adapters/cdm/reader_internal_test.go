package cdm

import (
	"errors"
	"io"
	"strings"
	"testing"
)

// stickyErrorReader serves prefix and then fails with the SAME error on every
// subsequent Read — the shape of a real I/O failure (a bad block, a
// disconnected volume) as encoding/csv sees it. Crucially it never returns
// io.EOF, so a reader loop that trusts EOF alone to terminate will spin
// forever.
type stickyErrorReader struct {
	prefix string
	off    int
	err    error
}

func (r *stickyErrorReader) Read(p []byte) (int, error) {
	if r.off < len(r.prefix) {
		n := copy(p, r.prefix[r.off:])
		r.off += n
		return n, nil
	}
	return 0, r.err
}

// TestDecodeCSVAbortsOnAStickyReadError is why decodeCSV exists as a seam: a
// FILE can only ever contain finitely many bad records, so no fixture can
// reproduce a Read that fails WITHOUT CONSUMING INPUT — the one error mode
// that never reaches EOF and would otherwise loop forever.
//
// It must abort with a HARD error, not a collected one. A caller that got
// (dataset, nil) back could not tell a truncated read from a complete one,
// and would happily ingest a partial backbone.
func TestDecodeCSVAbortsOnAStickyReadError(t *testing.T) {
	src := &stickyErrorReader{
		prefix: strings.Join(conceptColumns, "|") + "\n",
		err:    errors.New("input/output error"),
	}

	var collected []error
	rows := 0
	err := decodeCSV(src, "sticky.csv", "concepts", conceptColumns,
		func(int, func(string) string) { rows++ },
		func(e error) { collected = append(collected, e) })
	if err == nil {
		t.Fatal("a read that never consumes input must abort, not return success")
	}
	if !strings.Contains(err.Error(), "consumed no input") || !strings.Contains(err.Error(), "input/output error") {
		t.Errorf("error = %q, want it to name the stall and the underlying cause", err)
	}
	if rows != 0 {
		t.Fatalf("emitted %d rows, want 0", rows)
	}
	// Exactly maxStalledReads reports before giving up — pinned as an exact
	// number so the bound's boundary is observable, not merely finite.
	if len(collected) != maxStalledReads {
		t.Fatalf("collected %d errors, want %d", len(collected), maxStalledReads)
	}
	if !strings.Contains(collected[0].Error(), "input/output error") {
		t.Errorf("first error = %q, want the underlying I/O error", collected[0])
	}
}

// TestDecodeCSVFallsBackToTheRecordOrdinal pins errorLine's non-parse-error
// branch: an I/O error carries no line of its own, so the reported number is
// the record ordinal, and it must ADVANCE with each failed record rather than
// standing still or counting backwards.
func TestDecodeCSVFallsBackToTheRecordOrdinal(t *testing.T) {
	src := &stickyErrorReader{
		prefix: strings.Join(conceptColumns, "|") + "\n",
		err:    io.ErrUnexpectedEOF,
	}

	var collected []error
	if err := decodeCSV(src, "sticky.csv", "concepts", conceptColumns,
		func(int, func(string) string) {},
		func(e error) { collected = append(collected, e) }); err == nil {
		t.Fatal("want the stall abort")
	}

	// The header is record 1, so the first failing record is 2 and they count
	// upward from there.
	for i, want := range []string{"sticky.csv:2:", "sticky.csv:3:", "sticky.csv:4:"} {
		if !strings.Contains(collected[i].Error(), want) {
			t.Errorf("error %d = %q, want it to name %q", i, collected[i], want)
		}
	}
}

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

// TestDecodeCSVBoundsAStickyReadError is why decodeCSV exists as a seam: a
// FILE can only ever contain finitely many bad records, so no fixture can
// reproduce a Read that fails without consuming input.
func TestDecodeCSVBoundsAStickyReadError(t *testing.T) {
	src := &stickyErrorReader{
		prefix: strings.Join(conceptColumns, "|") + "\n",
		err:    errors.New("input/output error"),
	}

	var collected []error
	rows := 0
	err := decodeCSV(src, "sticky.csv", "concepts", conceptColumns,
		func(int, func(string) string) { rows++ },
		func(e error) { collected = append(collected, e) })
	if err != nil {
		t.Fatalf("decodeCSV: %v", err)
	}
	if rows != 0 {
		t.Fatalf("emitted %d rows, want 0", rows)
	}
	// Exactly maxConsecutiveRowErrors read failures plus the one "giving up"
	// line. Pinning the exact number is what makes the bound's boundary
	// observable rather than merely "not infinite".
	if len(collected) != maxConsecutiveRowErrors+1 {
		t.Fatalf("collected %d errors, want %d", len(collected), maxConsecutiveRowErrors+1)
	}
	last := collected[len(collected)-1].Error()
	if !strings.Contains(last, "giving up after 20 consecutive unreadable records") {
		t.Errorf("last error = %q", last)
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
	_ = decodeCSV(src, "sticky.csv", "concepts", conceptColumns,
		func(int, func(string) string) {},
		func(e error) { collected = append(collected, e) })

	// The header is record 1, so the first failing record is 2 and they count
	// upward from there.
	for i, want := range []string{"sticky.csv:2:", "sticky.csv:3:", "sticky.csv:4:"} {
		if !strings.Contains(collected[i].Error(), want) {
			t.Errorf("error %d = %q, want it to name %q", i, collected[i], want)
		}
	}
}

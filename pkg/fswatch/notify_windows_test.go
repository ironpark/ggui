package fswatch

import (
	"encoding/binary"
	"path/filepath"
	"slices"
	"testing"
)

func TestParseNotifyBounds(t *testing.T) {
	// Valid single record "a.go".
	rec := make([]byte, 12+8)
	binary.LittleEndian.PutUint32(rec[0:], 0)
	binary.LittleEndian.PutUint32(rec[4:], 1)
	binary.LittleEndian.PutUint32(rec[8:], 8)
	for i, r := range []uint16{'a', '.', 'g', 'o'} {
		binary.LittleEndian.PutUint16(rec[12+2*i:], r)
	}
	got := parseNotify(`C:\dir`, rec)
	want := []string{filepath.Join(`C:\dir`, "a.go")}
	if !slices.Equal(got, want) {
		t.Fatalf("valid = %v, want %v", got, want)
	}

	// FileNameLength exceeding buffer must stop without panic.
	badLen := slices.Clone(rec)
	binary.LittleEndian.PutUint32(badLen[8:], 100)
	if got := parseNotify(`C:\dir`, badLen); len(got) != 0 {
		t.Fatalf("oversized name = %v, want empty", got)
	}

	// Non-progressing NextEntryOffset (smaller than header+name) must stop without looping.
	badNext := slices.Clone(rec)
	binary.LittleEndian.PutUint32(badNext[0:], 4)
	if got := parseNotify(`C:\dir`, badNext); !slices.Equal(got, want) {
		t.Fatalf("bad next = %v, want %v", got, want)
	}
}

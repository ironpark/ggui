package text

import (
	"strings"
	"testing"
)

// lendingBytes is a reader that lends its bytes, as a mapped file does.
type lendingBytes []byte

func (b lendingBytes) ReadAt(p []byte, off int64) (int, error) { return copy(p, b[off:]), nil }
func (b lendingBytes) Slice(off, n int64) ([]byte, bool)       { return b[off : off+n], true }

// A font's reader lends a table the file holds as it is, and copies one a
// patch changes, or one of a reader that cannot lend.
func TestPatchedReaderLendsOnlyWhatNoPatchChanges(t *testing.T) {
	p := &patchedReaderAt{r: lendingBytes("0123456789abcdef")}
	p.patch(4, []byte("XY"))
	if b, ok := p.slice(8, 4); !ok || string(b) != "89ab" {
		t.Fatalf("slice(8, 4) = %q, %v", b, ok)
	}
	for _, off := range []int64{3, 4, 5} {
		if _, ok := p.slice(off, 2); ok {
			t.Errorf("slice(%d, 2) lent bytes a patch changes", off)
		}
	}
	if _, ok := (&patchedReaderAt{r: strings.NewReader("0123")}).slice(0, 2); ok {
		t.Error("a reader that cannot lend lent bytes")
	}
}

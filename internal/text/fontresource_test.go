package text

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/ironpark/ggui/internal/typesetting/font"
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

// permanentBytes lends its bytes, and has them stay, as a file mapped for
// the program's life does.
type permanentBytes struct{ lendingBytes }

func (permanentBytes) Permanent() bool { return true }

// The faces of a collection a permanent reader holds share their common
// 'morx' table; those of a reader that only lends read their own.
func TestCollectionFacesShareMorxOnlyOfPermanentBytes(t *testing.T) {
	data, err := os.ReadFile("/System/Library/Fonts/AppleSDGothicNeo.ttc")
	if err != nil {
		t.Skip(err)
	}
	morx := func(r io.ReaderAt, index int) *font.MorxChain {
		t.Helper()
		src, err := NewGoTextFaceSourceAt(r, int64(len(data)), SourceOptions{Index: index})
		if err != nil {
			t.Fatal(err)
		}
		if len(src.f.Morx) == 0 {
			t.Fatal("no morx table")
		}
		return &src.f.Morx[0]
	}
	perm := permanentBytes{lendingBytes(data)}
	if morx(perm, 0) != morx(perm, 1) {
		t.Error("faces of permanent bytes read their common table each")
	}
	lent := lendingBytes(bytes.Clone(data))
	if morx(lent, 0) == morx(lent, 1) {
		t.Error("faces of bytes that may go share a table")
	}
}

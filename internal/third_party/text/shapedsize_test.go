package text

import (
	"bytes"
	"fmt"
	"runtime"
	"testing"

	"golang.org/x/image/font/gofont/goregular"
)

// shapedBytes is the heap a source's caches hold per distinct string
// measured and laid out, as a label does.
func shapedBytes(t *testing.T) float64 {
	src, err := NewGoTextFaceSource(bytes.NewReader(goregular.TTF))
	if err != nil {
		t.Fatal(err)
	}
	face := &GoTextFace{Source: src, Size: 14}
	const n = 2000
	labels := make([]string, n)
	for i := range labels {
		labels[i] = fmt.Sprintf("Label number %d", i)
	}
	Measure("warm", face, 0)
	var glyphs []LazyGlyph
	glyphs = AppendLazyGlyphs(glyphs, "warm", face, nil)
	var before, after runtime.MemStats
	runtime.GC()
	runtime.GC()
	runtime.ReadMemStats(&before)
	for _, s := range labels {
		Measure(s, face, 0)
		glyphs = AppendLazyGlyphs(glyphs[:0], s, face, nil)
	}
	runtime.GC()
	runtime.GC()
	runtime.ReadMemStats(&after)
	runtime.KeepAlive(src)
	return float64(int64(after.HeapAlloc)-int64(before.HeapAlloc)) / n
}

// A shaped string is kept as the glyph records drawing reads, not as
// HarfBuzz's output: a cache of thousands of labels holds a few MB at
// most. The full output held some 4.1 KB for each of these labels.
func TestShapedStringBytes(t *testing.T) {
	b := shapedBytes(t)
	t.Logf("%.0f bytes per shaped string", b)
	if b <= 0 {
		t.Skip("the heap shrank under the measurement: garbage of earlier tests went at once")
	}
	if b > 2600 {
		t.Errorf("a shaped string holds %.0f bytes, want at most 2600", b)
	}
}

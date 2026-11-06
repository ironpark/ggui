package fontsys

import (
	"testing"

	"github.com/ironpark/ggui/internal/third_party/text"

	"golang.org/x/image/font/gofont/goregular"
)

// sample is text of a few scripts, as a label or a chat line has.
const sample = "Hello, world 안녕하세요 مرحبا 👍 0123456789 — fin."

// What a chain costs to look up a rune in, rune by rune, on one goroutine.
func BenchmarkHasGlyph(b *testing.B) {
	src := mustLoad(b, goregular.TTF).Meta()
	b.ReportAllocs()
	for b.Loop() {
		for _, r := range sample {
			HasGlyph(src, r)
		}
	}
}

// The same from every core at once, as text laid out off the UI thread is:
// what the lookups cost one another.
func BenchmarkHasGlyphParallel(b *testing.B) {
	src := mustLoad(b, goregular.TTF).Meta()
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			for _, r := range sample {
				HasGlyph(src, r)
			}
		}
	})
}

// A Picker over a font and its fallback, picking each rune of a line: the
// first picks of a rune no font of the chain has go to the system fonts.
func BenchmarkPickerPick(b *testing.B) {
	f := emojiChain(b)
	b.ReportAllocs()
	for b.Loop() {
		p := f.Picker(400)
		for _, r := range sample {
			p.Pick(r)
		}
	}
}

func BenchmarkPickerPickParallel(b *testing.B) {
	f := emojiChain(b)
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			p := f.Picker(400)
			for _, r := range sample {
				p.Pick(r)
			}
		}
	})
}

// A face of a chain at a size and weight, the way a TextStyle asks for one
// each time it draws: after the first, the cached one.
func BenchmarkFace(b *testing.B) {
	f := emojiChain(b)
	f.Face(14, 400, nil)
	b.ReportAllocs()
	for b.Loop() {
		f.Face(14, 400, nil)
	}
}

// A frame's faces at a few sizes, frame after frame, as a screen of text
// asks for them: the cache turns over generations, and lookups allocate
// nothing.
func BenchmarkFaceSteadyFrames(b *testing.B) {
	f := emojiChain(b)
	sizes := []float64{11, 12, 13, 14, 16, 18, 24, 32}
	for _, s := range sizes {
		f.Face(s, 400, nil)
	}
	b.ReportAllocs()
	for b.Loop() {
		text.AdvanceTick()
		for _, s := range sizes {
			f.Face(s, 400, nil)
		}
	}
}

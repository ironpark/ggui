package text

import (
	"sync"

	"github.com/ironpark/ggfx/vector"
)

var _ Face = (*FallbackFace)(nil)

// FallbackFace is a Face that finds faces for runes as it meets them, for
// the last place of a MultiFace: the faces before it draw what they have,
// and Resolve finds a face for each rune they lack. Runes no face is found
// for are drawn with Last, as a MultiFace draws them with its last face.
//
// A rune is resolved once, and the faces Resolve returns are kept; Resolve
// returns the same Face for the same font, so that runes of one font draw
// as one run.
type FallbackFace struct {
	resolve func(r rune) Face
	last    Face

	mu    sync.Mutex
	known map[rune]bool
	faces []Face
	multi *MultiFace
}

// NewFallbackFace returns a FallbackFace that resolves runes with resolve
// and draws what it finds nothing for with last, whose direction it has.
func NewFallbackFace(resolve func(r rune) Face, last Face) *FallbackFace {
	f := &FallbackFace{resolve: resolve, last: last, known: map[rune]bool{}}
	f.multi, _ = NewMultiFace(last)
	return f
}

// Metrics implements Face. They are last's: a font found for a rune draws
// its glyphs, and does not size the lines around them.
func (f *FallbackFace) Metrics() Metrics {
	return f.last.Metrics()
}

// hasGlyph implements Face. It resolves r if it was not.
func (f *FallbackFace) hasGlyph(r rune) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.resolveLocked(r)
}

func (f *FallbackFace) resolveLocked(r rune) bool {
	if has, ok := f.known[r]; ok {
		return has
	}
	face := f.resolve(r)
	if face != nil && face.direction() != f.last.direction() {
		face = nil
	}
	f.known[r] = face != nil
	if face == nil {
		return false
	}
	for _, have := range f.faces {
		if have == face {
			return true
		}
	}
	f.faces = append(f.faces, face)
	f.multi, _ = NewMultiFace(append(f.faces[:len(f.faces):len(f.faces)], f.last)...)
	return true
}

// faceFor returns the MultiFace that draws text, once every rune of it is
// resolved.
func (f *FallbackFace) faceFor(text string) *MultiFace {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, r := range text {
		f.resolveLocked(r)
	}
	return f.multi
}

// advanceAt implements Face.
func (f *FallbackFace) advanceAt(text string, indexInBytes int) float64 {
	return f.faceFor(text).advanceAt(text, indexInBytes)
}

// appendLazyGlyphsForLine implements Face.
func (f *FallbackFace) appendLazyGlyphsForLine(glyphs []LazyGlyph, line string, indexOffset int, originX, originY float64, keepGlyph func(originX, originY float64) bool) []LazyGlyph {
	return f.faceFor(line).appendLazyGlyphsForLine(glyphs, line, indexOffset, originX, originY, keepGlyph)
}

// appendVectorPathForLine implements Face.
func (f *FallbackFace) appendVectorPathForLine(path *vector.Path, line string, originX, originY float64) {
	f.faceFor(line).appendVectorPathForLine(path, line, originX, originY)
}

// direction implements Face.
func (f *FallbackFace) direction() Direction {
	return f.last.direction()
}

// private implements Face.
func (f *FallbackFace) private() {
}

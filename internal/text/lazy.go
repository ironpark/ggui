// Copyright 2026 The Ebitengine Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package text

import (
	"sync"
	"sync/atomic"

	"github.com/ironpark/ggfx/vector"
)

var _ Face = (*LazyFace)(nil)

// LazyFace is a Face that stands in for one not loaded yet. Its metrics,
// direction and the runes it has come from a light face, one whose source
// was read with SourceOptions.MetricsOnly; the face it stands in for is
// loaded the first time text is measured, shaped or drawn with it. In a
// MultiFace of fallbacks, a font is then loaded only once text needs a
// glyph only it has.
//
// The light face and the loaded one must be of the same font at the same
// size and variations, so that their metrics and glyph coverage agree.
type LazyFace struct {
	light Face
	load  func() Face

	once   sync.Once
	face   Face
	loaded atomic.Bool
}

// NewLazyFace returns a LazyFace answering from light until load, called
// once, gives the face it stands in for. A nil face from load leaves light
// in its place.
func NewLazyFace(light Face, load func() Face) *LazyFace {
	return &LazyFace{light: light, load: load}
}

// Loaded reports whether the face l stands in for has been loaded.
func (l *LazyFace) Loaded() bool { return l.loaded.Load() }

func (l *LazyFace) get() Face {
	l.once.Do(func() {
		l.face = l.light
		if f := l.load(); f != nil {
			l.face = f
		}
		l.loaded.Store(true)
	})
	return l.face
}

// Metrics implements Face.
func (l *LazyFace) Metrics() Metrics { return l.light.Metrics() }

// advanceAt implements Face.
func (l *LazyFace) advanceAt(text string, indexInBytes int) float64 {
	return l.get().advanceAt(text, indexInBytes)
}

// hasGlyph implements Face.
func (l *LazyFace) hasGlyph(r rune) bool { return l.light.hasGlyph(r) }

// appendLazyGlyphsForLine implements Face.
func (l *LazyFace) appendLazyGlyphsForLine(glyphs []LazyGlyph, line string, indexOffset int, originX, originY float64, keepGlyph func(originX, originY float64) bool) []LazyGlyph {
	return l.get().appendLazyGlyphsForLine(glyphs, line, indexOffset, originX, originY, keepGlyph)
}

// appendVectorPathForLine implements Face.
func (l *LazyFace) appendVectorPathForLine(path *vector.Path, line string, originX, originY float64) {
	l.get().appendVectorPathForLine(path, line, originX, originY)
}

// direction implements Face.
func (l *LazyFace) direction() Direction { return l.light.direction() }

// private implements Face.
func (l *LazyFace) private() {}

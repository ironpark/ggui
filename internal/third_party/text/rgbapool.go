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
	"image"

	"github.com/ironpark/ggfx"
	"github.com/ironpark/ggui/pkg/gem"
)

var theRGBAPool = gem.Pool[*image.RGBA]{
	New: func() *image.RGBA {
		return &image.RGBA{}
	},
}

// newPooledRGBA returns an *image.RGBA whose Pix buffer capacity is reused from a pool.
// The returned image must be passed to releasePooledRGBA when it is no longer used.
// The Pix buffer is guaranteed to be zero-filled.
func newPooledRGBA(w, h int) *image.RGBA {
	size := 4 * w * h
	rgba := theRGBAPool.Get()
	if cap(rgba.Pix) < size {
		rgba.Pix = make([]byte, size)
	} else {
		rgba.Pix = rgba.Pix[:size]
	}
	rgba.Stride = 4 * w
	rgba.Rect = image.Rect(0, 0, w, h)
	return rgba
}

// newRGBA returns a new *image.RGBA of its own, for a glyph image that stays
// on the CPU.
func newRGBA(w, h int) *image.RGBA {
	return image.NewRGBA(image.Rect(0, 0, w, h))
}

// uploadPooledRGBA returns an image of img, made by newPooledRGBA, and
// returns img to the pool; nil for nil.
func uploadPooledRGBA(img *image.RGBA) *ggfx.Image {
	if img == nil {
		return nil
	}
	defer releasePooledRGBA(img)
	return ggfx.NewImageFromImage(img)
}

// releasePooledRGBA returns an image obtained from newPooledRGBA to the pool.
//
// A buffer past mostPooledRGBA is let go instead: the pool would keep the
// largest glyph it ever drew, a big emoji, for every small one after.
func releasePooledRGBA(rgba *image.RGBA) {
	if cap(rgba.Pix) > mostPooledRGBA {
		return
	}
	clear(rgba.Pix[:cap(rgba.Pix)])
	rgba.Pix = rgba.Pix[:0]
	theRGBAPool.Put(rgba)
}

// mostPooledRGBA is the largest buffer, in bytes, the pool keeps: a glyph
// 128 pixels square.
const mostPooledRGBA = 64 << 10

var theAlphaPool = gem.Pool[*image.Alpha]{
	New: func() *image.Alpha {
		return &image.Alpha{}
	},
}

// newPooledAlpha is newPooledRGBA for the coverage of a monochrome glyph,
// one byte a pixel. The returned image must be passed to
// releasePooledAlpha, or uploadPooledAlpha, when it is no longer used.
func newPooledAlpha(w, h int) *image.Alpha {
	size := w * h
	a := theAlphaPool.Get()
	if cap(a.Pix) < size {
		a.Pix = make([]byte, size)
	} else {
		a.Pix = a.Pix[:size]
	}
	a.Stride = w
	a.Rect = image.Rect(0, 0, w, h)
	return a
}

// glyphMaskOptions makes a monochrome glyph's image alpha-only: it is
// white coverage, which an alpha image draws as.
var glyphMaskOptions = &ggfx.NewImageFromImageOptions{Format: ggfx.PixelFormatAlpha}

// uploadPooledAlpha returns an alpha-only image of img, made by
// newPooledAlpha, and returns img to the pool; nil for nil.
func uploadPooledAlpha(img *image.Alpha) *ggfx.Image {
	if img == nil {
		return nil
	}
	defer releasePooledAlpha(img)
	return ggfx.NewImageFromImageWithOptions(img, glyphMaskOptions)
}

// releasePooledAlpha returns an image obtained from newPooledAlpha to the
// pool, unless it is larger than a glyph 128 pixels square, as
// releasePooledRGBA does.
func releasePooledAlpha(a *image.Alpha) {
	if cap(a.Pix) > mostPooledRGBA/4 {
		return
	}
	clear(a.Pix[:cap(a.Pix)])
	a.Pix = a.Pix[:0]
	theAlphaPool.Put(a)
}

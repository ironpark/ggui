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
	"bytes"
	"image"

	"github.com/ironpark/ggui/internal/third_party/typesetting/font"
	"golang.org/x/image/math/fixed"

	"github.com/ironpark/ggfx"
	"github.com/ironpark/ggfx/svg"
)

// svgGlyphData is an OpenType SVG glyph description: the SVG document and its
// resolved viewBox.
type svgGlyphData struct {
	source  []byte
	viewBox font.SVGViewBox
}

// svgToImage rasterizes an OpenType SVG glyph document into a color image.
// sizeInPixels is the em size in pixels. subpixelOffset and glyphBounds
// follow the same conventions as segmentsToImage, and the returned image
// has the same dimensions as segmentsToImage would produce, so the two
// are interchangeable at the draw position derived from glyphBounds.
//
// svgToImage returns nil if the glyph produces no image or the SVG
// document cannot be parsed.
//
// The whole document is rasterized: a document shared by multiple
// glyphs is reduced to a single glyph's description beforehand (see
// [GoTextFaceSource.svgGlyphData]).
func svgToImage(glyph *svgGlyphData, sizeInPixels float64, subpixelOffset fixed.Point26_6, glyphBounds fixed.Rectangle26_6) *ggfx.Image {
	return uploadPooledRGBA(svgToRGBA(glyph, sizeInPixels, subpixelOffset, glyphBounds, newPooledRGBA))
}

// svgToRGBA is svgToImage on the CPU, into an image alloc makes.
func svgToRGBA(glyph *svgGlyphData, sizeInPixels float64, subpixelOffset fixed.Point26_6, glyphBounds fixed.Rectangle26_6, alloc func(w, h int) *image.RGBA) *image.RGBA {
	if glyph.viewBox.Width <= 0 || glyph.viewBox.Height <= 0 {
		return nil
	}

	w, h := (glyphBounds.Max.X - glyphBounds.Min.X).Ceil(), (glyphBounds.Max.Y - glyphBounds.Min.Y).Ceil()
	if w == 0 || h == 0 {
		return nil
	}

	// Add always 1 to the size, following segmentsToImage.
	w++
	h++

	doc, err := svg.Parse(bytes.NewReader(glyph.source))
	if err != nil {
		return nil
	}

	// The OpenType SVG coordinate system has its origin at the glyph origin with
	// the Y axis pointing down, and the viewBox maps onto the em square.
	// This matches the display space of the scaled outline segments, so the
	// glyph pixel position of an SVG user-space point is
	// (point - viewBoxMin) * (emSizeInPixels / viewBoxSize).
	sx := sizeInPixels / float64(glyph.viewBox.Width)
	sy := sizeInPixels / float64(glyph.viewBox.Height)
	var g ggfx.GeoM
	g.Translate(-float64(glyph.viewBox.MinX), -float64(glyph.viewBox.MinY))
	g.Scale(sx, sy)
	g.Translate(fixed26_6ToFloat64(-glyphBounds.Min.X+subpixelOffset.X), fixed26_6ToFloat64(-glyphBounds.Min.Y+subpixelOffset.Y))

	dst := alloc(w, h)
	doc.Draw(dst, g)
	return dst
}

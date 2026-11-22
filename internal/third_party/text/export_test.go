// Copyright 2023 The Ebitengine Authors
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

// SVGGlyphDocument extracts the SVG document describing only the glyph
// gid from an OpenType SVG document.
func SVGGlyphDocument(source []byte, gid uint32) []byte {
	return newSVGDocIndex(source).glyphDocument(gid)
}

// GlyphImageCacheCount returns the number of the glyph image caches of the source, one per face size.
func GlyphImageCacheCount(source *GoTextFaceSource) int {
	c := &source.glyphImageCache
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.caches)
}

// HasGlyphImageCache reports whether the source has a glyph image cache for the given size.
func HasGlyphImageCache(source *GoTextFaceSource, size float64) bool {
	c := &source.glyphImageCache
	c.mu.Lock()
	defer c.mu.Unlock()
	_, ok := c.caches[size]
	return ok
}

// TouchGlyphImageCache creates or refreshes the glyph image cache for the size of the given face,
// as if the face were used at the given tick.
func TouchGlyphImageCache(face *GoTextFace, tick int64) {
	face.Source.glyphImageCache.cacheForFace(face, tick)
}

func HasGlyph(f Face, r rune) bool {
	return f.hasGlyph(r)
}

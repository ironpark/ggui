# typesetting (fork)

This directory is a fork of [github.com/go-text/typesetting](https://github.com/go-text/typesetting)
at version `v0.3.5`, with [github.com/go-text/render](https://github.com/go-text/render)
at version `v0.2.1` in `render/`. Both are provided under the Unlicense or the
BSD-3-Clause license; the full text is in LICENSE beside this README, and
HarfBuzz's in `harfbuzz/LICENSE`.

Import paths are rewritten from `github.com/go-text/typesetting/...` and
`github.com/go-text/render` to `github.com/ironpark/ggui/internal/typesetting/...`.
The tests, testdata, and `testutils` directory of the originals are not
included, nor is anything that only they use.

## Local modifications

- Unused private cluster-mapping and set helpers from the upstream test
  surface are removed. Reserved Indic position values keep their original
  numeric slots.

- Glyph variations are decoded as each glyph is first drawn, not all at once
  when a font loads. `tables.Gvar` keeps the table and the offsets of each
  glyph's data, checked as it is parsed, and `Gvar.GlyphVariationData`
  parses one glyph's; `font`'s `gvar` decodes a glyph's tuples and deltas
  on first use and keeps them in a per-glyph `atomic.Pointer`, so a `*Font`
  stays safe for concurrent use. Upstream decodes every glyph up front,
  which kept about 37 MB live for SF Pro (`SFNS.ttf`) of which ggui draws a
  few hundred glyphs; it now keeps about 9 MB. A glyph whose data is
  malformed now has no variations, where upstream dropped the variations
  of it and every glyph after it.
- A table read with no buffer to reuse is lent rather than copied when the
  font's source can lend its bytes: `opentype.Slicer`, which ggui's mapped
  font files implement. A font's tables then stay in the file's pages,
  which macOS does not count as the program's memory, and not on the heap:
  SF Pro kept 9.3 MB on the heap, and keeps 1.9 MB.
- A face's glyph extents are cached in pages of 256 glyphs, each made when
  one of its glyphs is first measured, rather than a slot for every glyph
  of the font as the face is made.

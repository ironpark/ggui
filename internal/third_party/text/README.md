# text (fork)

This directory is a fork of [github.com/hajimehoshi/ebiten/v2/text/v2](https://github.com/hajimehoshi/ebiten/tree/main/text/v2),
provided under the Apache License, Version 2.0; the full text is in `LICENSE`
beside this README.

Import paths are rewritten to use `github.com/ironpark/ggfx` for rendering and
`github.com/ironpark/ggui/internal/third_party/typesetting/...` for shaping.
Upstream's large test font fixtures (`internal/testfonts`, Roboto/MPLUS fonts
in `testdata/`) and upstream tests relying on them or on `bitmapfont/v4` are
omitted. Minimal ChromaCheck color-glyph fixtures in `testdata/` (see
`testdata/LICENSE.md`) and ggui-specific tests are kept.

## Local modifications

- Renders glyph images and vector paths against `github.com/ironpark/ggfx`
  instead of Ebiten, storing monochrome glyph masks as alpha-only images and
  drawing SVG glyphs through `svgx`.
- Supports on-demand font file reading (`NewGoTextFaceSourceAt`), metrics-only
  sources, and lazy fallback faces (`LazyFace`) backed by `fontfile.go`,
  `lazy.go`, and `fallback.go`.
- Compacts shaped strings into glyph records, caches per-size glyph images
  with explicit frame tick aging (`AdvanceTick`), and supports letter spacing.

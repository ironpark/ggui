package ggui

import (
	"image"
	"image/color"
	"math"
	"slices"

	"github.com/ironpark/ggfx"
	"github.com/ironpark/ggui/internal/fontsys"
	"github.com/ironpark/ggui/internal/paragraph"
	"github.com/ironpark/ggui/internal/pool"
	"github.com/ironpark/ggui/internal/text"
	"github.com/ironpark/ggui/internal/textedit"
)

// engine is st as the text engine sets text in it: its font, size, weight
// and line height, beside the emoji font the active world chose. st must
// be resolved.
func (st TextStyle) engine() paragraph.Style {
	return paragraph.Style{Font: st.Font.engine(), Size: st.Size, Weight: fontsys.Weight(st.Weight), LineHeight: st.LineHeight, Emoji: activeEmoji()}
}

// RenderText draws s in st on the CPU, with no GPU, window or App, into an
// image just large enough to hold it: for icons, thumbnails, files and
// tests. Lines break at hard breaks and, when maxWidth is positive, wrap to
// maxWidth as Text wraps them. Fields st leaves unset are Text's defaults:
// the built-in font, DefaultTextSize, black, and 1.2 times the size between
// baselines. A pixel of the image is a pixel of st.Size; scale st.Size for a
// denser image.
//
// Glyphs come from st.Font and its fallbacks, as Text draws them, and emoji
// from the emoji font, in color. Text is shaped with go-text and drawn with
// go-text/render; emoji are drawn as Text draws them, from sbix, CBDT,
// COLRv0 or SVG glyphs, rasterized on the CPU.
func RenderText(s string, st TextStyle, maxWidth float64) *image.RGBA {
	st = st.resolved()
	return paragraph.Render(s, st.engine(), st.Color, maxWidth)
}

// textBidi is a Text's lines in visual order, for the lines that need
// reordering: every line in a right-to-left paragraph, and a line holding
// right-to-left text in a left-to-right one. Other lines draw as they are.
type textBidi struct {
	dir    TextDirection
	lang   string
	key    wrapKey
	laid   bool
	lines  [][]paragraph.Piece     // nil for a line drawn as it is
	styles [][]paragraph.LineStyle // each reordered line's styles
}

// layoutBidi orders the lines Layout just wrapped for the Env's direction.
func (t *TextWidget) layoutBidi(env Env) {
	b := &t.bidi
	dir, lang := env.Direction(), env.Language()
	if b.laid && b.dir == dir && b.lang == lang && b.key == t.wrapped {
		return
	}
	b.laid, b.dir, b.lang, b.key = true, dir, lang, t.wrapped
	b.lines, b.styles = b.lines[:0], b.styles[:0]
	rtl := dir == DirRTL
	for _, line := range t.lines {
		if !paragraph.NeedsBidi(line, rtl) {
			b.lines, b.styles = append(b.lines, nil), append(b.styles, nil)
			continue
		}
		styles := t.lineStyles(line)
		b.lines, b.styles = append(b.lines, paragraph.LayoutLine(line, styles, rtl, lang)), append(b.styles, styles)
	}
}

// lineStyles is a line of a Text in its one style.
func (t *TextWidget) lineStyles(line string) []paragraph.LineStyle {
	return []paragraph.LineStyle{{Start: 0, End: len(line), Style: t.current().engine()}}
}

// paintBidi draws line i in visual order at y, aligned at align in r, and
// reports whether it did; a line that needs no reordering is left to the
// caller.
func (t *TextWidget) paintBidi(dst *Canvas, i int, r Rect, align, y float64, col color.Color, place func(op *text.DrawOptions, x, y float64)) bool {
	if i >= len(t.bidi.lines) || t.bidi.lines[i] == nil {
		return false
	}
	ps, line := t.bidi.lines[i], t.lines[i]
	x := r.Origin.X + (r.Size.W-paragraph.Width(ps))*align
	baseline := t.faceAt(dst.Scale()).Metrics().HAscent
	paragraph.Draw(dst.Image, &t.pieceOp, line, ps, t.bidi.styles[i], dst.Scale(), baseline,
		func(int) color.Color { return col },
		func(op *text.DrawOptions) { place(op, x, y) })
	return true
}

// PaintRotated paints already-laid-out text clockwise around the center of r.
// It uses the regular glyph cache without allocating an intermediate image.
// Layout is unchanged; the caller reserves space for the rotated bounds.
func (t *TextWidget) PaintRotated(dst *Canvas, r Rect, radians float64) {
	if radians == 0 {
		t.Paint(dst, r)
		return
	}
	if math.IsNaN(radians) || math.IsInf(radians, 0) {
		return
	}
	center := r.Center()
	t.paintLines(dst, r, false, func(op *text.DrawOptions, x, y float64) {
		op.GeoM.Translate(dst.px(x-center.X), dst.px(y-center.Y))
		op.GeoM.Rotate(radians)
		op.GeoM.Translate(dst.px(center.X), dst.px(center.Y))
	})
}

// DrawTextCells draws s as DrawTextStyle does, but with each grapheme at
// the start of a cell of its own, cellW wide, from at: the layout of a
// terminal or any other grid of monospaced text. A grid's cells are best a
// whole number of device pixels, so that fills meet without seams, while a
// font's advance rarely is (SF Mono is 8.03 pixels wide at 13pt), and text
// drawn at its own advance would drift off the grid along a row. Each
// grapheme is placed on a whole device pixel, so the error stays under
// half a pixel instead of growing with the column. The run is shaped
// once, so it costs what DrawTextStyle does rather than a draw per cell.
func (c *Canvas) DrawTextCells(s string, st TextStyle, at Point, cellW float64) {
	if c == nil || c.Image == nil || c.Image.Bounds().Empty() || s == "" {
		return
	}
	st = st.resolved()
	// A terminal draws a run per row and colour every frame, each a string
	// of its own: the run's graphemes, glyphs, shifts and images are kept
	// in buffers it reuses.
	buf := theCellsBufPool.Get()
	defer func() {
		clear(buf.glyphs)
		clear(buf.entries)
		theCellsBufPool.Put(buf)
	}()
	buf.bounds = textedit.AppendBounds(buf.bounds[:0], s)
	face := st.Font.face(c.px(st.Size), st.Weight)
	face = fontsys.PlainFace(face, s)
	if face == nil {
		// An emoji is drawn from a font of its own, with its own
		// metrics: one grapheme at a time keeps it in its cell.
		x := at.X
		for k := 0; k+1 < len(buf.bounds); k++ {
			c.DrawTextStyle(s[buf.bounds[k]:buf.bounds[k+1]], st, Pt(x, at.Y))
			x += cellW
		}
		return
	}

	var tint, colored ggfx.ColorScale
	tint.ScaleWithColor(st.Color)
	colored.ScaleAlpha(tint.A())
	x0, y0, cell := c.px(at.X), c.px(at.Y), c.px(cellW)

	buf.glyphs = text.AppendLazyGlyphs(buf.glyphs[:0], s, face, nil)
	buf.shifts = cellShifts(buf.shifts, buf.bounds, buf.glyphs, x0, cell)
	// Realize every image before drawing any, as text.Draw does, so the
	// atlas is written in one batch, then draw those on one atlas page as
	// one.
	buf.entries = buf.entries[:0]
	for i, g := range buf.glyphs {
		if g.ImageBounds.Empty() {
			continue
		}
		if img := g.Image(); img != nil {
			e := ggfx.ImageDraw{Image: img, X: x0 + (float64(g.ImageBounds.Min.X) + buf.shifts[i]), Y: y0 + float64(g.ImageBounds.Min.Y), ColorScale: tint}
			if g.Colored() {
				e.ColorScale = colored
			}
			buf.entries = append(buf.entries, e)
		}
	}
	c.Image.DrawImages(buf.entries, nil)
}

// cellsBuf holds what DrawTextCells works out for a run between runs.
type cellsBuf struct {
	bounds  []int
	glyphs  []text.LazyGlyph
	shifts  []float64
	entries []ggfx.ImageDraw
}

var theCellsBufPool = pool.Pool[*cellsBuf]{
	New: func() *cellsBuf { return &cellsBuf{} },
}

// cellShifts is how far each glyph moves from where its run shaped it, in
// device pixels, to start its grapheme's cell: the kth grapheme, from
// bounds[k] to bounds[k+1] of the run as textedit.Bounds gives them, has
// its first glyph at the whole pixel nearest x0+k·cell, and the rest of a
// grapheme, its marks or the parts of a ligature, move with it. It is kept
// in dst's storage when that has room.
func cellShifts(dst []float64, bounds []int, glyphs []text.LazyGlyph, x0, cell float64) []float64 {
	shifts := slices.Grow(dst[:0], len(glyphs))[:len(glyphs)]
	k, shift := -1, 0.0
	for i, g := range glyphs {
		if g.StartIndexInBytes >= bounds[k+1] {
			for k+1 < len(bounds)-1 && g.StartIndexInBytes >= bounds[k+1] {
				k++
			}
			shift = math.Round(x0+float64(k)*cell) - x0 - g.OriginX
		}
		shifts[i] = shift
	}
	return shifts
}

package ggui

import (
	"image"
	"image/color"

	"github.com/ironpark/ggui/internal/fontsys"
	"github.com/ironpark/ggui/internal/paragraph"
	"github.com/ironpark/ggui/internal/text"
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

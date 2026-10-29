package ggui

import (
	"image/color"

	"github.com/ironpark/ggui/internal/text"
)

// textBidi is a Text's lines in visual order, for the lines that need
// reordering: every line in a right-to-left paragraph, and a line holding
// right-to-left text in a left-to-right one. Other lines draw as they are.
type textBidi struct {
	dir    TextDirection
	lang   string
	key    wrapKey
	laid   bool
	lines  [][]piece     // nil for a line drawn as it is
	styles [][]lineStyle // each reordered line's styles
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
	for _, line := range t.lines {
		if !needsBidi(line, dir) {
			b.lines, b.styles = append(b.lines, nil), append(b.styles, nil)
			continue
		}
		styles := t.lineStyles(line)
		b.lines, b.styles = append(b.lines, layoutLine(line, styles, dir, lang)), append(b.styles, styles)
	}
}

// lineStyles is a line of a Text in its one style.
func (t *TextWidget) lineStyles(line string) []lineStyle {
	st := t.current()
	return []lineStyle{{start: 0, end: len(line), font: st.Font, size: st.Size, weight: st.Weight}}
}

// paintBidi draws line i in visual order at y, aligned at align in r, and
// reports whether it did; a line that needs no reordering is left to the
// caller.
func (t *TextWidget) paintBidi(dst *Canvas, i int, r Rect, align, y float64, col color.Color, place func(op *text.DrawOptions, x, y float64)) bool {
	if i >= len(t.bidi.lines) || t.bidi.lines[i] == nil {
		return false
	}
	ps, line := t.bidi.lines[i], t.lines[i]
	x := r.Origin.X + (r.Size.W-piecesWidth(ps))*align
	baseline := t.faceAt(dst.Scale()).Metrics().HAscent
	drawPieces(dst.Image, &t.pieceOp, line, ps, t.bidi.styles[i], dst.Scale(), baseline,
		func(int) color.Color { return col },
		func(op *text.DrawOptions) { place(op, x, y) })
	return true
}

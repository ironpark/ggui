package ggui

import (
	"math"

	"github.com/ironpark/ggfx"
	"github.com/ironpark/ggfx/text/v2"

	"github.com/ironpark/ggui/internal/emojidata"
	"github.com/ironpark/ggui/internal/textedit"
)

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
	face := st.Font.face(c.px(st.Size), st.Weight)
	if ef, ok := face.(*emojiFace); ok {
		if emojidata.MayHold(s) {
			// An emoji is drawn from a font of its own, with its own
			// metrics: one grapheme at a time keeps it in its cell.
			x := at.X
			for i := 0; i < len(s); {
				j := textedit.NextGrapheme(s, i)
				c.DrawTextStyle(s[i:j], st, Pt(x, at.Y))
				x += cellW
				i = j
			}
			return
		}
		face = ef.Face
	}

	var op ggfx.DrawImageOptions
	op.ColorScale.ScaleWithColor(st.Color)
	colored := op
	colored.ColorScale.Reset()
	colored.ColorScale.ScaleAlpha(op.ColorScale.A())
	x0, y0, cell := c.px(at.X), c.px(at.Y), c.px(cellW)

	glyphs := text.AppendLazyGlyphs(nil, s, face, nil)
	shifts := cellShifts(s, glyphs, x0, cell)
	type entry struct {
		img     *ggfx.Image
		x, y    float64
		colored bool
	}
	// Realize every image before drawing any, as text.Draw does, so the
	// atlas is written in one batch.
	entries := make([]entry, 0, len(glyphs))
	for i, g := range glyphs {
		if g.ImageBounds.Empty() {
			continue
		}
		if img := g.Image(); img != nil {
			entries = append(entries, entry{img, float64(g.ImageBounds.Min.X) + shifts[i], float64(g.ImageBounds.Min.Y), g.Colored()})
		}
	}
	for _, e := range entries {
		o := &op
		if e.colored {
			o = &colored
		}
		o.GeoM.Reset()
		o.GeoM.Translate(x0+e.x, y0+e.y)
		c.Image.DrawImage(e.img, o)
	}
}

// cellShifts is how far each glyph moves from where s shaped it, in device
// pixels, to start its grapheme's cell: the kth grapheme's first glyph goes
// to the whole pixel nearest x0+k·cell, and the rest of a grapheme, its
// marks or the parts of a ligature, move with it.
func cellShifts(s string, glyphs []text.LazyGlyph, x0, cell float64) []float64 {
	shifts := make([]float64, len(glyphs))
	k, next, shift := -1, 0, 0.0
	for i, g := range glyphs {
		if g.StartIndexInBytes >= next {
			for g.StartIndexInBytes >= next && next < len(s) {
				next = textedit.NextGrapheme(s, next)
				k++
			}
			shift = math.Round(x0+float64(k)*cell) - x0 - g.OriginX
		}
		shifts[i] = shift
	}
	return shifts
}

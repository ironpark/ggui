package ui

import (
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ironpark/ggui"
)

// toggle is the shared body of Checkbox, Radio and Switch: a glyph, an
// optional label, hover and tap.
type toggle struct {
	interactive
	label *ggui.TextWidget
	onTap func()

	labelSize ggui.Size
	glyph     ggui.Size
	theme     ggui.Theme
}

func (g *toggle) layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	g.theme = env.Theme()
	size := g.glyph
	if g.label != nil {
		g.label.Color(pick[color.Color](g.disabled, g.theme.Muted, nil))
		g.labelSize = g.label.Layout(ggui.Loose(ggui.Sz(max(c.MaxW-size.W-controlGap, 0), c.MaxH)), env)
		size.W += controlGap + g.labelSize.W
		size.H = max(size.H, g.labelSize.H)
	}
	return c.Constrain(size)
}

// paint registers the region and paints the label, and returns the Rect the
// glyph should be drawn in.
func (g *toggle) paint(dst *ggui.Canvas, r ggui.Rect, handler control) ggui.Rect {
	g.hit(dst, r, handler, ebiten.CursorShapePointer)
	if g.label != nil {
		at := ggui.Pt(r.Origin.X+g.glyph.W+controlGap, r.Origin.Y+(r.Size.H-g.labelSize.H)/2)
		dst.Paint(g.label, ggui.Rct(at, g.labelSize))
	}
	return ggui.Rct(ggui.Pt(r.Origin.X, r.Origin.Y+(r.Size.H-g.glyph.H)/2), g.glyph)
}

// HandleKey implements KeyHandler: Space or Enter toggles.
func (g *toggle) HandleKey(ev ggui.KeyEvent) { g.key(ev, g.onTap) }

// HandlePointer implements PointerHandler.
func (g *toggle) HandlePointer(ev ggui.PointerEvent) bool { return g.pointer(ev, g.onTap) }

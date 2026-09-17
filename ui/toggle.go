package ui

import (
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ironpark/ggui"
)

// toggle is the shared body of Checkbox, Radio and Switch: a glyph, an
// optional label, hover and tap.
type toggle struct {
	label    *ggui.TextWidget
	disabled bool
	hovered  bool
	focus    focusState
	onTap    func()

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

// control is what a toggle's concrete widget implements.
type control interface {
	ggui.PointerHandler
	ggui.KeyHandler
}

// paint registers the region and paints the label, and returns the Rect the
// glyph should be drawn in.
func (g *toggle) paint(dst *ggui.Canvas, r ggui.Rect, handler control) ggui.Rect {
	if !g.disabled {
		dst.HitPointer(r, handler)
		dst.HitKey(r, handler)
		dst.HitCursor(r, ebiten.CursorShapePointer)
	}
	if g.label != nil {
		at := ggui.Pt(r.Origin.X+g.glyph.W+controlGap, r.Origin.Y+(r.Size.H-g.labelSize.H)/2)
		dst.Paint(g.label, ggui.Rct(at, g.labelSize))
	}
	return ggui.Rct(ggui.Pt(r.Origin.X, r.Origin.Y+(r.Size.H-g.glyph.H)/2), g.glyph)
}

func (g *toggle) state() *toggle { return g }

// Adopt implements ggui.Adopter: hover and focus carry across a rebuild.
func (g *toggle) Adopt(prev any) {
	if p, ok := prev.(interface{ state() *toggle }); ok {
		g.hovered, g.focus = p.state().hovered, p.state().focus
	}
}

// HandleKey implements KeyHandler: Space or Enter toggles.
func (g *toggle) HandleKey(ev ggui.KeyEvent) {
	g.focus.handle(ev)
	if activates(ev) && g.onTap != nil {
		g.onTap()
	}
}

func (g *toggle) handle(ev ggui.PointerEvent) bool {
	switch ev.Kind {
	case ggui.PointerEnter, ggui.PointerMove:
		g.hovered = true
	case ggui.PointerExit:
		g.hovered = false
	case ggui.PointerTap:
		if ev.Button == ebiten.MouseButtonLeft && g.onTap != nil {
			g.onTap()
		}
	case ggui.PointerScroll:
		return false
	}
	return true
}

// CheckboxWidget is a box that is ticked while its signal is true. Build one

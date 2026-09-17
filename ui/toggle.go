package ui

import (
	"image/color"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ironpark/ggui"
)

// toggle is the shared body of Checkbox, Radio and Switch: a glyph, an
// optional label, hover and tap.
type toggle struct {
	ggui.Interactive
	label *ggui.TextWidget
	onTap func()

	labelSize ggui.Size
	glyph     ggui.Size
	theme     ggui.Theme
	motion    time.Duration // MotionFast, or zero under reduced motion
}

// layout takes the glyph size because each embedder sizes its own: a
// checkbox and a radio are square control glyphs, a switch is a track.
func (g *toggle) layout(c ggui.Constraints, env ggui.Env, glyph ggui.Size) ggui.Size {
	g.Sync()
	g.theme = env.Theme()
	g.motion = env.Motion(g.theme.MotionFast)
	g.glyph = glyph
	size := g.glyph
	if g.label != nil {
		g.label.Color(pick[color.Color](g.Inert, g.theme.MutedFg, nil))
		g.labelSize = g.label.Layout(ggui.Loose(ggui.Sz(max(c.MaxW-size.W-g.theme.ControlGap, 0), c.MaxH)), env)
		size.W += g.theme.ControlGap + g.labelSize.W
		size.H = max(size.H, g.labelSize.H)
	}
	return c.Constrain(size)
}

// paint registers the region and paints the label, and returns the Rect the
// glyph should be drawn in.
func (g *toggle) paint(dst *ggui.Canvas, r ggui.Rect, handler ggui.Control) ggui.Rect {
	g.Hit(dst, r, handler, ebiten.CursorShapePointer)
	if g.label != nil {
		at := ggui.Pt(r.Origin.X+g.glyph.W+g.theme.ControlGap, r.Origin.Y+(r.Size.H-g.labelSize.H)/2)
		dst.Paint(g.label, ggui.Rct(at, g.labelSize))
	}
	return ggui.Rct(ggui.Pt(r.Origin.X, r.Origin.Y+(r.Size.H-g.glyph.H)/2), g.glyph)
}

// HandleKey implements KeyHandler: Space or Enter toggles.
func (g *toggle) HandleKey(ev ggui.KeyEvent) { g.Keyboard(ev, g.onTap) }

// HandlePointer implements PointerHandler.
func (g *toggle) HandlePointer(ev ggui.PointerEvent) bool { return g.Pointer(ev, g.onTap) }

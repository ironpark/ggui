package ui

import (
	"image/color"
	"time"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/internal/property"
	uitheme "github.com/ironpark/ggui/ui/theme"
)

// toggle is the shared body of Checkbox, Radio and Switch: a glyph, an
// optional label, hover and tap.
type toggle struct {
	ggui.Interactive
	label *ggui.TextWidget
	onTap func()

	labelSize ggui.Size
	glyph     ggui.Size
	theme     *uitheme.Theme
	env       ggui.Env
	motion    time.Duration // MotionFast, or zero under reduced motion

	invalidProp  property.Value[bool] // set by Invalid or BindInvalid
	fieldInvalid bool                 // the Field around the control holds an error
}

// toggleInvalidSlot eases the destructive halo of an invalid control.
var toggleInvalidSlot = ggui.NewSlot[*ggui.Motion]("toggle invalid")

// invalid reports whether the control failed validation: by its own
// Invalid, or by the error of the Field it sits in.
func (g *toggle) invalid() bool { return g.invalidProp.Get() || g.fieldInvalid }

// invalidAmount is how far the control is into the invalid state, from 0
// to 1, eased over the control's motion: what its border and halo follow.
func (g *toggle) invalidAmount(dst *ggui.Canvas, box ggui.Rect) float64 {
	return ease(dst, g.theme, g.Anchor(box), toggleInvalidSlot, pick(g.invalid(), 1.0, 0.0), g.motion)
}

// stateRing draws what surrounds the glyph in box, over it: the destructive
// halo of an invalid control, at the invalidAmount given, and the keyboard
// focus ring, which is destructive too while invalid.
func (g *toggle) stateRing(dst *ggui.Canvas, box ggui.Rect, radius, invalid float64) {
	t := g.theme
	if invalid > 0 && !g.IsInert() {
		haloStroke(dst, box, radius, fade(t.Halo(destructiveRing(t)), invalid))
	}
	focusRing(dst, &g.Interactive, t, box, radius, fieldRing(t, g.invalid()), g.motion)
}

// layout takes the glyph size because each embedder sizes its own: a
// checkbox and a radio are square control glyphs, a switch is a track.
func (g *toggle) layout(c ggui.Constraints, env ggui.Env, glyph ggui.Size) ggui.Size {
	g.Sync()
	g.theme = uitheme.Of(env)
	g.env = env
	g.motion = env.Motion(g.theme.MotionFast)
	g.glyph = glyph
	g.fieldInvalid, _ = env.Get(fieldInvalid)
	size := g.glyph
	if g.label != nil {
		// A label inherits the destructive text of an invalid Field, as
		// everything in shadcn's does.
		var col color.Color
		switch {
		case g.IsInert():
			col = g.theme.Disabled(g.theme.Fg)
		case g.fieldInvalid:
			col = g.theme.Destructive
		}
		g.label.Color(col)
		g.labelSize = g.label.Layout(ggui.Loose(ggui.Sz(max(c.MaxW-size.W-g.theme.ControlGap, 0), c.MaxH)), env)
		size.W += g.theme.ControlGap + g.labelSize.W
		size.H = max(size.H, g.labelSize.H)
	}
	return c.Constrain(size)
}

// paint registers the region and paints the label, and returns the Rect the
// glyph should be drawn in.
func (g *toggle) paint(dst *ggui.Canvas, r ggui.Rect, handler ggui.Control) ggui.Rect {
	g.Hit(dst, r, handler, ggui.CursorShapeDefault)
	if g.label != nil {
		at := ggui.Pt(r.Origin.X+g.glyph.W+g.theme.ControlGap, r.Origin.Y+(r.Size.H-g.labelSize.H)/2)
		dst.Paint(g.label, mirrorHorizontal(ggui.Rct(at, g.labelSize), r, g.env.Direction() == ggui.DirRTL))
	}
	return mirrorHorizontal(ggui.Rct(ggui.Pt(r.Origin.X, r.Origin.Y+(r.Size.H-g.glyph.H)/2), g.glyph), r, g.env.Direction() == ggui.DirRTL)
}

// HandleKey implements KeyHandler: Space or Enter toggles.
func (g *toggle) HandleKey(ev ggui.KeyEvent) { g.Keyboard(ev, g.onTap) }

// HandlePointer implements PointerHandler.
func (g *toggle) HandlePointer(ev ggui.PointerEvent) bool { return g.Pointer(ev, g.onTap) }

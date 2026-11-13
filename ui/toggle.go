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
	inCard       bool                 // a FieldCard around the control shows its focus
}

// hovered reports whether the pointer is on the control, for the FieldCard
// around it.
func (g *toggle) hovered() bool { return g.Hovered && !g.IsInert() }

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
	// In a FieldCard the ring is the card's, and the glyph keeps its
	// border: shadcn's group-has-[:focus-visible]/field-label:ring-0.
	if !g.inCard {
		focusRing(dst, &g.Interactive, t, box, radius, fieldRing(t, g.invalid()), g.motion)
	}
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
	g.inCard, _ = env.Get(inFieldCard)
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
// glyph should be drawn in. The pointer lands on the control a little way
// outside the glyph as well, as shadcn's after:-inset-x-3 after:-inset-y-2
// has it: 12px to either side and 8px above and below.
func (g *toggle) paint(dst *ggui.Canvas, r ggui.Rect, handler ggui.Control) ggui.Rect {
	glyph := mirrorHorizontal(ggui.Rct(ggui.Pt(r.Origin.X, r.Origin.Y+(r.Size.H-g.glyph.H)/2), g.glyph), r, g.env.Direction() == ggui.DirRTL)
	g.HitReach(dst, r, outset(glyph, 1.5*g.theme.Space, g.theme.Space), handler, ggui.CursorShapeDefault)
	if g.label != nil {
		at := ggui.Pt(r.Origin.X+g.glyph.W+g.theme.ControlGap, r.Origin.Y+(r.Size.H-g.labelSize.H)/2)
		dst.Paint(g.label, mirrorHorizontal(ggui.Rct(at, g.labelSize), r, g.env.Direction() == ggui.DirRTL))
	}
	return glyph
}

// outset is r grown by x to either side and by y above and below.
func outset(r ggui.Rect, x, y float64) ggui.Rect {
	return ggui.Rct(ggui.Pt(r.Origin.X-x, r.Origin.Y-y), ggui.Sz(r.Size.W+2*x, r.Size.H+2*y))
}

// HandleKey implements KeyHandler: Space or Enter toggles.
func (g *toggle) HandleKey(ev ggui.KeyEvent) { g.Keyboard(ev, g.onTap) }

// HandlePointer implements PointerHandler.
func (g *toggle) HandlePointer(ev ggui.PointerEvent) bool { return g.Pointer(ev, g.onTap) }

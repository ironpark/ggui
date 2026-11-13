package ui

import (
	"image/color"
	"time"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui/icons"
	uitheme "github.com/ironpark/ggui/ui/theme"
)

// panelBox applies the chrome every floating surface shares: menus, context
// menus, the menubar, select lists and the date picker panel.
func panelBox(b *ggui.BoxWidget, t *uitheme.Theme) *ggui.BoxWidget {
	return b.Shadow(t.PanelShadow).Fill(t.Popover).Border(t.BorderWidth, fade(t.Fg, .1)).Radius(t.Radius).Padding(t.PanelPad)
}

// fieldBox applies the chrome every input shares: the Input surface, the
// theme's padding and radius, and a border that becomes the focus ring
// while the editor inside has the keyboard. A disabled field takes the
// muted surface shadcn gives one. It sets the colors at once; a field that
// eases between its states sets only fieldBoxShape in Layout and calls
// fieldChrome in Paint.
func fieldBox(b *ggui.BoxWidget, t *uitheme.Theme, focused, inert bool) *ggui.BoxWidget {
	return fieldBoxPadded(b, t, focused, inert, t.FieldPad)
}

// fieldBoxPadded is fieldBox with the caller's padding, set once: writing
// the theme's and then another would change the box twice a layout, and a
// box that changed after it was measured is never cached.
func fieldBoxPadded(b *ggui.BoxWidget, t *uitheme.Theme, focused, inert bool, pad ggui.EdgeInsets) *ggui.BoxWidget {
	border := pick(focused, t.Ring, t.InputBorder)
	if inert {
		border = t.Disabled(border)
	}
	return fieldBoxShape(b, t, pad).Fill(fieldFill(t, inert)).Border(t.BorderWidth, border)
}

// fieldBoxShape gives a field's box what its size depends on, the padding
// and the radius, and leaves its colors to fieldChrome.
func fieldBoxShape(b *ggui.BoxWidget, t *uitheme.Theme, pad ggui.EdgeInsets) *ggui.BoxWidget {
	return b.Padding(pad).Radius(t.Radius)
}

// fieldFill is the surface of a field: the theme's Input, which may be
// translucent and is drawn as it is, or the muted one of a disabled field.
func fieldFill(t *uitheme.Theme, inert bool) color.Color {
	if inert {
		return fade(t.InputBorder, pick(isDark(t.Bg), .8, .5))
	}
	return t.Input
}

// invalidBorder is the border of a control that failed validation: the
// Destructive color, at half strength on a dark page as shadcn draws it.
func invalidBorder(t *uitheme.Theme) color.Color {
	if isDark(t.Bg) {
		return fade(t.Destructive, .5)
	}
	return t.Destructive
}

// The eased state of a field's chrome and of a control's focus ring.
var (
	fieldFocusSlot   = ggui.NewSlot[*ggui.Motion]("field focus")
	fieldInvalidSlot = ggui.NewSlot[*ggui.Motion]("field invalid")
	focusRingSlot    = ggui.NewSlot[*ggui.Motion]("focus ring")
)

// fieldState is what a field's chrome shows.
type fieldState struct {
	focused, invalid, inert bool
}

// fieldChrome colors a field's box for its state and draws the halo around
// it; see fieldTones. Call it in Paint, before painting b in r.
func fieldChrome(dst *ggui.Canvas, at ggui.Anchor, b *ggui.BoxWidget, t *uitheme.Theme, r ggui.Rect, radius float64, s fieldState, motion time.Duration) {
	fill, border := fieldTones(dst, at, t, r, radius, s, motion)
	b.Fill(fill).Border(t.BorderWidth, border)
}

// fieldTones draws the halo around a field in r and returns the fill and
// the border its state calls for, easing between states over motion: the
// border moves from InputBorder to the Ring while the editor has the
// keyboard, and to the destructive border while the field is invalid,
// focused or not, when it also keeps a destructive halo. The motions are
// retained under at and rest once they arrive, so an idle field asks for
// no frame.
func fieldTones(dst *ggui.Canvas, at ggui.Anchor, t *uitheme.Theme, r ggui.Rect, radius float64, s fieldState, motion time.Duration) (fill, border color.Color) {
	focus := dst.Ease(at, fieldFocusSlot, pick(s.focused && !s.inert, 1.0, 0.0), motion)
	invalid := dst.Ease(at, fieldInvalidSlot, pick(s.invalid, 1.0, 0.0), motion)
	border = mix(mix(t.InputBorder, t.Ring, focus), invalidBorder(t), invalid)
	if s.inert {
		border = t.Disabled(border)
	} else if focus > 0 || invalid > 0 {
		// An invalid field shows the destructive halo whether or not it
		// is focused, as aria-invalid outranks focus-visible in shadcn.
		haloStroke(dst, r, radius, mix(fade(t.Halo(t.Ring), focus), t.Halo(destructiveRing(t)), invalid))
	}
	return fieldFill(t, s.inert), border
}

// haloStroke draws the three-pixel halo just outside r in col, as it is.
func haloStroke(dst *ggui.Canvas, r ggui.Rect, radius float64, col color.Color) {
	dst.StrokeRoundRect(ggui.Rct(r.Origin.Add(ggui.Pt(-2, -2)), ggui.Sz(r.Size.W+4, r.Size.H+4)), radius+2, 3, col)
}

// chevron resolves the directional role shared by selects and accordions.
// dy points the tip down when positive and up when negative.
func chevron(dst *ggui.Canvas, env ggui.Env, t *uitheme.Theme, center ggui.Point, dy float64, col color.Color) {
	paintIconAt(dst, env, pick(dy < 0, icons.ChevronUp, icons.ChevronDown), center, t.IconSize, col, 0)
}

// fieldHalo draws the focus ring just outside a text field, at the theme's
// halo opacity.
func fieldHalo(dst *ggui.Canvas, t *uitheme.Theme, r ggui.Rect, radius float64, ring color.Color) {
	haloStroke(dst, r, radius, t.Halo(ring))
}

// fieldRing is the focus ring for a field: the theme's, or the destructive
// one while the field is invalid.
func fieldRing(t *uitheme.Theme, invalid bool) color.Color {
	if invalid {
		return destructiveRing(t)
	}
	return t.Ring
}

// destructiveRing is the focus ring of anything that warns: destructive
// buttons and invalid fields.
func destructiveRing(t *uitheme.Theme) color.Color {
	return fade(t.Destructive, pick(isDark(t.Bg), .8, .4))
}

// isDark reports whether c is closer to black than to white.
func isDark(c color.Color) bool {
	r, g, b, _ := c.RGBA()
	return r+g+b < 3*32768
}

// iconButtonSide is the side of a square button holding one icon, as
// ButtonIcon pads it.
func iconButtonSide(t *uitheme.Theme) float64 { return max(t.IconSize, 20+2*t.ButtonPad.Top) }

// paintDisclosure paints content folding open below y: the body is clipped
// to progress of its height and inert while closing. The progress comes
// from Motion.LayoutToggle, which lays the disclosure out again for each
// step while it moves.
func paintDisclosure(dst *ggui.Canvas, content ggui.Widget, at ggui.Point, width float64, size ggui.Size, progress float64, open bool) {
	if progress <= 0 {
		return
	}
	body := dst.Clip(ggui.Rct(at, ggui.Sz(width, size.H*progress)))
	if !open {
		body = body.Inert()
	}
	body.Paint(content, ggui.Rct(at, size))
}

func colorOr(value, fallback color.Color) color.Color {
	if value != nil {
		return value
	}
	return fallback
}

// Shared hover timing for disclosure headers, independent of their opening motion.
var disclosureHoverSlot = ggui.NewSlot[*ggui.Motion]("disclosure hover")

// controlFocusRing draws the keyboard focus of a control: the ring color on
// its border and the three-pixel soft halo around it. The ring eases in and
// out over the theme's MotionFast, retained under the control's identity,
// and rests once it arrives, so an idle control asks for no frame. A
// caller that knows its Env calls focusRing with env.Motion(t.MotionFast),
// which lands at once under reduced motion.
func controlFocusRing(dst *ggui.Canvas, state *ggui.Interactive, t *uitheme.Theme, r ggui.Rect, radius float64, ring color.Color) {
	focusRing(dst, state, t, r, radius, ring, t.MotionFast)
}

// focusRing is controlFocusRing easing over motion; zero switches at once.
func focusRing(dst *ggui.Canvas, state *ggui.Interactive, t *uitheme.Theme, r ggui.Rect, radius float64, ring color.Color, motion time.Duration) {
	on := state.Focused && state.FocusVisible && !state.IsInert()
	k := dst.Ease(state.Anchor(r), focusRingSlot, pick(on, 1.0, 0.0), motion)
	if k <= 0 {
		return
	}
	dst.StrokeRoundRect(r, radius, t.BorderWidth, fade(ring, k))
	haloStroke(dst, r, radius, fade(t.Halo(ring), k))
}

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
// menuSurface lays content out under the theme its theme draws menus in,
// for the popup of a menu, a select or a combobox; see Theme.Menu.
func menuSurface(content ggui.Widget) ggui.Widget { return ggui.WithEnv(uitheme.MenuEnv, content) }

func panelBox(b *ggui.BoxWidget, t *uitheme.Theme) *ggui.BoxWidget {
	return b.Shadow(t.PanelShadow).Fill(t.Popover).Border(t.BorderWidth, fade(t.Fg, .1)).Radius(t.Radius).Padding(t.PanelPad)
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

// fieldChromeJoined is fieldChrome for a field that may sit in a
// ButtonGroup: on the sides joined to a neighbour its corners are square,
// its border gives way to the neighbour's, and its halo, which the group
// draws above its children, runs past the seam. With no side joined it is
// fieldChrome.
func fieldChromeJoined(dst *ggui.Canvas, at ggui.Anchor, b *ggui.BoxWidget, t *uitheme.Theme, r ggui.Rect, radius float64, s fieldState, motion time.Duration, sides JoinedSides) {
	if !sides.Any() {
		fieldChrome(dst, at, b, t, r, radius, s, motion)
		return
	}
	b.Fill(nil).Border(0, nil)
	fieldSurface(dst, at, t, r, radius, s, motion, sides)
}

// fieldSurface paints a field's fill, border and halo in r itself, for a
// field that draws its own surface rather than through a box, following
// the sides it is joined on.
func fieldSurface(dst *ggui.Canvas, at ggui.Anchor, t *uitheme.Theme, r ggui.Rect, radius float64, s fieldState, motion time.Duration, sides JoinedSides) {
	fill, border, halo, ringed := fieldToneParts(dst, at, t, s, motion)
	paintJoinedBox(dst, r, radius, sides, fill, t.BorderWidth, border)
	if !ringed {
		return
	}
	f := joinedFocus{on: true, r: r, radius: radius, sides: sides, width: t.BorderWidth, line: border, halo: halo}
	if g, ok := dst.Enclosing().(*ButtonGroupWidget); ok && sides.Any() {
		g.focus = f
		return
	}
	f.paint(dst)
}

// fieldTones draws the halo around a field in r and returns the fill and
// the border its state calls for, easing between states over motion: the
// border moves from InputBorder to the Ring while the editor has the
// keyboard, and to the destructive border while the field is invalid,
// focused or not, when it also keeps a destructive halo. The motions are
// retained under at and rest once they arrive, so an idle field asks for
// no frame.
func fieldTones(dst *ggui.Canvas, at ggui.Anchor, t *uitheme.Theme, r ggui.Rect, radius float64, s fieldState, motion time.Duration) (fill, border color.Color) {
	fill, border, halo, ringed := fieldToneParts(dst, at, t, s, motion)
	if ringed {
		haloStroke(dst, r, radius, halo)
	}
	return fill, border
}

// ease is Canvas.Ease along the theme's Ease, the curve of a Tailwind
// transition: what a control's hover, focus, checked and invalid states
// move on. A nil t is the default theme's.
func ease(dst *ggui.Canvas, t *uitheme.Theme, at ggui.Anchor, s ggui.Slot[*ggui.Motion], target float64, d time.Duration) float64 {
	var c uitheme.Curve
	if t != nil {
		c = t.Ease
	}
	return dst.EaseWith(at, s, target, d, c.Easing())
}

// fieldToneParts is fieldTones without the drawing: the fill, the border,
// and the halo with whether there is one to draw.
func fieldToneParts(dst *ggui.Canvas, at ggui.Anchor, t *uitheme.Theme, s fieldState, motion time.Duration) (fill, border, halo color.Color, ringed bool) {
	focus := ease(dst, t, at, fieldFocusSlot, pick(s.focused && !s.inert, 1.0, 0.0), motion)
	invalid := ease(dst, t, at, fieldInvalidSlot, pick(s.invalid, 1.0, 0.0), motion)
	border = mix(mix(t.InputBorder, t.Ring, focus), invalidBorder(t), invalid)
	if s.inert {
		border = t.Disabled(border)
	} else if focus > 0 || invalid > 0 {
		// An invalid field shows the destructive halo whether or not it
		// is focused, as aria-invalid outranks focus-visible in shadcn.
		halo, ringed = mix(fade(t.Halo(t.Ring), focus), t.Halo(destructiveRing(t)), invalid), true
	}
	return fieldFill(t, s.inert), border, halo, ringed
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
	k := ease(dst, t, state.Anchor(r), focusRingSlot, pick(on, 1.0, 0.0), motion)
	if k <= 0 {
		return
	}
	dst.StrokeRoundRect(r, radius, t.BorderWidth, fade(ring, k))
	haloStroke(dst, r, radius, fade(t.Halo(ring), k))
}

package ui

import (
	"image/color"
	"math"
	"time"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui/internal/controlkit"
	uitheme "github.com/ironpark/ggui/ui/theme"
	"github.com/ironpark/ggui/widget"
)

// panelBox applies the chrome every floating surface shares: menus, context
// menus, the menubar, select lists and the date picker panel.
// menuSurface lays content out under the theme its theme draws menus in,
// for the popup of a menu, a select or a combobox; see Theme.Menu.
func menuSurface(content ggui.Widget) ggui.Widget { return ggui.WithEnv(uitheme.MenuEnv, content) }

func panelBox(b *widget.BoxWidget, t *uitheme.Theme) *widget.BoxWidget {
	return panelShadow(b, t).Fill(t.Popover).Border(t.BorderWidth, fade(t.Fg, .1)).Radius(t.Radius).Padding(t.PanelPad)
}

// panelShadow gives b the shadow of a popup, shadcn's shadow-md: a select
// list, a combobox list, a menu, a popover, a hover card. It is the theme's
// PanelShadow over its PanelContact, the two layers Tailwind draws.
func panelShadow(b *widget.BoxWidget, t *uitheme.Theme) *widget.BoxWidget {
	layers := [2]ggui.ShadowStyle{t.PanelShadow, t.PanelContact}
	return b.Shadow(layers[:]...)
}

// paintedBaseline is where the glyphs of a line of text stand, for a line
// whose box starts at top with the ascent layout reports, at the canvas's
// scale. It is not quite top+ascent: text is drawn on whole pixels, the
// line's top rounded to one and the baseline's place under it cut down to
// one. A rule drawn a set distance from the letters, as an underline is,
// starts from here; at top+ascent it would stand up to a pixel and a bit
// further off, by a different amount from one position to the next.
func paintedBaseline(top, ascent, scale float64) float64 {
	return (math.Round(top*scale) + math.Floor(ascent*scale)) / scale
}

// fieldBoxShape gives a field's box what its size depends on, the padding
// and the radius, and leaves its colors to fieldChromeJoined.
func fieldBoxShape(b *widget.BoxWidget, t *uitheme.Theme, pad ggui.EdgeInsets) {
	b.Padding(pad).Radius(t.Radius)
}

// invalidBorder is the border of a control that failed validation: the
// Destructive color, at half strength on a dark page as shadcn draws it.
func invalidBorder(t *uitheme.Theme) color.Color {
	if controlkit.IsDark(t.Bg) {
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

// fieldLook is what a field's chrome is drawn from: the theme, what the
// field shows, the motion it eases between states over, and the sides it
// is joined on. Its corners are the theme's Radius.
type fieldLook struct {
	theme                   *uitheme.Theme
	focused, invalid, inert bool
	motion                  time.Duration
	sides                   JoinedSides
}

// fieldChromeJoined colors a field's box for its look and draws the halo
// around it; see fieldTones. Call it in Paint, before painting b in r. The
// field may sit in a ButtonGroup: on the sides joined to a neighbour its
// corners are square, its border gives way to the neighbour's, and its
// halo, which the group draws above its children, runs past the seam.
func fieldChromeJoined(dst *ggui.Canvas, at ggui.Anchor, b *widget.BoxWidget, r ggui.Rect, look fieldLook) {
	if !look.sides.Any() {
		fill, border := fieldTones(dst, at, r, look)
		b.Fill(fill).Border(look.theme.BorderWidth, border)
		return
	}
	b.Fill(nil).Border(0, nil)
	fieldSurface(dst, at, r, look)
}

// fieldSurface paints a field's fill, border and halo in r itself, for a
// field that draws its own surface rather than through a box, following
// the sides it is joined on.
func fieldSurface(dst *ggui.Canvas, at ggui.Anchor, r ggui.Rect, look fieldLook) {
	t := look.theme
	fill, border, halo, ringed := fieldToneParts(dst, at, look)
	shape := joinedBox{rect: r, radius: t.Radius, sides: look.sides}
	paintJoinedBox(dst, shape, fill, t.BorderWidth, border)
	if !ringed {
		return
	}
	f := joinedFocus{joinedBox: shape, on: true, width: t.BorderWidth, line: border, halo: halo}
	if g, ok := dst.Enclosing().(*ButtonGroupWidget); ok && look.sides.Any() {
		g.ring(f)
		return
	}
	f.paint(dst)
}

// fieldTones draws the halo around a field in r and returns the fill and
// the border its look calls for, easing between states over its motion:
// the border moves from InputBorder to the Ring while the editor has the
// keyboard, and to the destructive border while the field is invalid,
// focused or not, when it also keeps a destructive halo. The motions are
// retained under at and rest once they arrive, so an idle field asks for
// no frame.
func fieldTones(dst *ggui.Canvas, at ggui.Anchor, r ggui.Rect, look fieldLook) (fill, border color.Color) {
	fill, border, halo, ringed := fieldToneParts(dst, at, look)
	if ringed {
		controlkit.HaloStroke(dst, r, look.theme.Radius, halo)
	}
	return fill, border
}

// fieldToneParts is fieldTones without the drawing: the fill, the border,
// and the halo with whether there is one to draw. The fill is the theme's
// Input, which may be translucent and is drawn as it is, or the muted
// surface of a disabled field.
func fieldToneParts(dst *ggui.Canvas, at ggui.Anchor, look fieldLook) (fill, border, halo color.Color, ringed bool) {
	t := look.theme
	focus := controlkit.Ease(dst, t, at, fieldFocusSlot, pick(look.focused && !look.inert, 1.0, 0.0), look.motion)
	invalid := controlkit.Ease(dst, t, at, fieldInvalidSlot, pick(look.invalid, 1.0, 0.0), look.motion)
	fill = t.Input
	border = mix(mix(t.InputBorder, t.Ring, focus), invalidBorder(t), invalid)
	if look.inert {
		fill = fade(t.InputBorder, pick(controlkit.IsDark(t.Bg), .8, .5))
		border = t.Disabled(border)
	} else if focus > 0 || invalid > 0 {
		// An invalid field shows the destructive halo whether or not it
		// is focused, as aria-invalid outranks focus-visible in shadcn.
		halo, ringed = mix(fade(t.Halo(t.Ring), focus), t.Halo(destructiveRing(t)), invalid), true
	}
	return fill, border, halo, ringed
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
	return fade(t.Destructive, pick(controlkit.IsDark(t.Bg), .8, .4))
}

// Shared hover timing for disclosure headers, independent of their opening motion.
var disclosureHoverSlot = ggui.NewSlot[*ggui.Motion]("disclosure hover")

// Ring describes a keyboard focus indicator: Color strokes the border of
// Rect, rounded by Radius, and a soft three-pixel halo surrounds it. The
// ring eases in and out over Motion (typically env.Motion(Theme.MotionFast),
// which lands at once under reduced motion); zero switches at once. Theme
// is required: the ring takes its border width and halo from it.
type Ring struct {
	Theme  *uitheme.Theme
	Rect   ggui.Rect
	Radius float64
	Color  color.Color
	Motion time.Duration
}

// FocusRing draws the keyboard focus indicator for a control: a border stroke
// in ring.Color and a soft three-pixel halo outside ring.Rect, easing in and
// out over ring.Motion, retained under the control's identity, and resting
// once it arrives, so an idle control asks for no frame.
func FocusRing(dst *ggui.Canvas, state *ggui.Interactive, ring Ring) {
	on := state.Focused && state.FocusVisible && !state.IsInert()
	k := controlkit.Ease(dst, ring.Theme, state.Anchor(ring.Rect), focusRingSlot, pick(on, 1.0, 0.0), ring.Motion)
	if k <= 0 {
		return
	}
	dst.StrokeRoundRect(ring.Rect, ring.Radius, ring.Theme.BorderWidth, fade(ring.Color, k))
	controlkit.HaloStroke(dst, ring.Rect, ring.Radius, fade(ring.Theme.Halo(ring.Color), k))
}

// DashedBorder strokes a rounded dashed outline inside r with the given corner
// radius, stroke width, and color.
func DashedBorder(dst *ggui.Canvas, r ggui.Rect, radius, width float64, col color.Color) {
	inset := width / 2
	x, y := r.Origin.X+inset, r.Origin.Y+inset
	w, h := max(0, r.Size.W-width), max(0, r.Size.H-width)
	radius = max(0, min(radius-inset, min(w, h)/2))
	straightX, straightY := w-2*radius, h-2*radius
	arc := math.Pi * radius / 2
	lengths := []float64{straightX, arc, straightY, arc, straightX, arc, straightY, arc}
	point := func(d float64) ggui.Point {
		segment := 0
		for segment < 7 && d > lengths[segment] {
			d -= lengths[segment]
			segment++
		}
		switch segment {
		case 0:
			return ggui.Pt(x+radius+d, y)
		case 2:
			return ggui.Pt(x+w, y+radius+d)
		case 4:
			return ggui.Pt(x+w-radius-d, y+h)
		case 6:
			return ggui.Pt(x, y+h-radius-d)
		}
		theta := float64(segment-1)*math.Pi/4 - math.Pi/2
		if radius > 0 {
			theta += d / radius
		}
		cx, cy := x+w-radius, y+radius
		if segment >= 3 {
			cy = y + h - radius
		}
		if segment >= 5 {
			cx = x + radius
		}
		if segment == 7 {
			cy = y + radius
		}
		return ggui.Pt(cx+radius*math.Cos(theta), cy+radius*math.Sin(theta))
	}
	total := 2*(straightX+straightY) + 4*arc
	for d := 0.0; d < total; d += 7 {
		if end := min(d+4, total); end > d {
			dst.StrokeLine(point(d), point(end), width, col)
		}
	}
}

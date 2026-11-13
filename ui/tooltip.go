package ui

import (
	"math"
	"time"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/internal/property"
	uitheme "github.com/ironpark/ggui/ui/theme"
)

// TooltipWidget shows a short text near its child after the cursor has
// rested on it. Build one with Tooltip.
type TooltipWidget struct {
	props        property.Owner
	child        ggui.Widget
	tip          *ggui.TextWidget
	keys         *ggui.TextWidget // the shortcut hint, if any
	hint         string           // what Shortcut set, before the child's chord
	text         func() string    // the text, asked as the tip opens; nil keeps tip's
	delay        time.Duration
	pad          ggui.EdgeInsets
	arrow        bool
	below        bool
	arrowX       float64
	disabled     bool
	disabledProp property.Value[bool]
	escaped      bool // Escape dismissed the tip, as of the last paint
	// focusOpens reports whether keyboard focus opens the tip; nil means
	// focus on a widget the child painted. Tabs, whose strip takes focus
	// as a whole, opens the shown tab's.
	focusOpens func(dst *ggui.Canvas, r ggui.Rect) bool

	box    *ggui.BoxWidget
	effect *ggui.TransitionWidget
	env    ggui.Env
	theme  *uitheme.Theme
}

// tooltipHover is the hover timer, retained on the Canvas by Rect so a
// tooltip rebuilt every frame still opens.
type tooltipHover struct {
	since     time.Time
	dismissed bool // pressed since the pointer came over it
	reveal    ggui.Motion
	escape    *bool
}

// tooltipEscape closes an open tip.
var tooltipEscape = ggui.MustChord("escape")

var tooltipSlot = ggui.NewSlot[tooltipHover]("tooltip hover")

// Tooltip shows text above child once the cursor rests on it, or while keyboard focus is within child. A press closes it, and
// it stays closed until the cursor leaves and comes back; focus a click
// gives the child does not open it, as focus moved with the keyboard does. It registers no hit
// region, so the child gets every event, and paints through Canvas.Overlay.
func Tooltip(child ggui.Widget, text string) *TooltipWidget {
	t := &TooltipWidget{child: child, tip: ggui.Text(text), delay: 0, arrow: true}
	t.box = ggui.Box(t.tip)
	t.effect = ggui.PopIn(tooltipSurface{t})
	return t
}

// TooltipText is Tooltip with text asked for while the tip is shown, for
// text that goes stale, such as how long until something happens.
func TooltipText(child ggui.Widget, text func() string) *TooltipWidget {
	t := Tooltip(child, "")
	t.text = text
	return t
}

// Shortcut shows a key hint after the text, written the way
// MenuItemWidget.Shortcut writes it: "Undo ⌘Z". Around a Button with a
// Shortcut, the tip shows the button's chord without this.
func (t *TooltipWidget) Shortcut(s string) *TooltipWidget {
	defer property.Watch(&t.props, &t.hint)()
	t.hint = s
	return t
}

// Disabled suppresses the tooltip while leaving its child interactive.
func (t *TooltipWidget) Disabled(v bool) *TooltipWidget {
	property.Set(&t.props, &t.disabledProp, v)
	t.disabled = v
	return t
}

// BindDisabled follows whether the tooltip is suppressed.
func (t *TooltipWidget) BindDisabled(r ggui.Readable[bool]) *TooltipWidget {
	property.Bind(&t.props, &t.disabledProp, r, "BindDisabled")
	return t
}

// Arrow shows or hides the directional arrow (shown by default).
func (t *TooltipWidget) Arrow(v bool) *TooltipWidget {
	defer property.Watch(&t.props, &t.arrow)()
	t.arrow = v
	return t
}

// Delay sets how long the cursor must rest before the tip appears.
func (t *TooltipWidget) Delay(d time.Duration) *TooltipWidget {
	defer property.Watch(&t.props, &t.delay)()
	t.delay = d
	return t
}

// Layout implements ggui.Widget.
func (t *TooltipWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	defer t.props.Layout().EndLayout()
	t.disabled = t.disabledProp.Get()
	t.theme = uitheme.Of(env)
	t.env = env
	t.pad = ggui.Insets(t.theme.Space*.75, t.theme.Space*1.5)
	t.tip.Style(t.theme.Caption).Color(t.theme.Bg)
	t.showKeys()
	t.box.Padding(t.pad).Fill(t.theme.Fg).Radius(t.theme.RadiusSm)
	return t.child.Layout(c, env)
}

// showKeys puts the shortcut hint, Shortcut's or else the child's chord,
// after the text, building the row the first time there is one.
func (t *TooltipWidget) showKeys() {
	label := shortcutLabel(t.hint)
	if c, ok := t.child.(chorded); ok && label == "" {
		if chord, ok := c.ShortcutChord(); ok {
			label = chord.Label()
		}
	}
	if label == "" && t.keys == nil {
		return
	}
	if t.keys == nil {
		t.keys = ggui.Text(label).NoWrap()
		t.box = ggui.Box(ggui.Row(t.tip, t.keys).Gap(t.theme.Space).Align(ggui.AlignBaseline))
		t.effect = ggui.PopIn(tooltipSurface{t})
	}
	t.keys.Content(label).Style(t.theme.Caption).Color(fade(t.theme.Bg, .65))
}

// Baseline implements ggui.Baseliner: the child's.
func (t *TooltipWidget) Baseline() (float64, bool) {
	if b, ok := t.child.(ggui.Baseliner); ok {
		return b.Baseline()
	}
	return 0, false
}

// Paint implements ggui.Widget.
func (t *TooltipWidget) Paint(dst *ggui.Canvas, r ggui.Rect) {
	focused := dst.PaintFocus(t.child, r) && dst.FocusVisible()
	if t.focusOpens != nil {
		focused = t.focusOpens(dst, r)
	}
	p, ok := dst.Pointer()
	now := ggui.FrameTime()
	at := ggui.Anchor{Rect: r}
	state, kept := dst.Retained(at, tooltipSlot)
	if !kept {
		// An idle tooltip keeps nothing: it comes back closed, so that
		// opening still eases in.
		state.reveal.MoveTo(0, now, 0)
	}
	hovered := ok && r.Contains(p)
	if hovered && state.since.IsZero() {
		state.since = now
	}
	if !hovered {
		state.since, state.dismissed = time.Time{}, false
	}
	if hovered && dst.PointerPressed() {
		state.dismissed = true
	}
	// escape is shared with the shortcut below, which outlives this paint;
	// only a tip that opened has one.
	escaped := state.escape != nil && *state.escape
	if escaped && !hovered && !focused {
		*state.escape, escaped = false, false
	}
	t.escaped = escaped
	open := !t.disabled && !escaped && (focused || (hovered && !state.dismissed && now.Sub(state.since) >= t.delay))
	if open {
		if state.escape == nil {
			state.escape = new(bool)
		}
		escape := state.escape
		dst.Shortcut(tooltipEscape, func() { *escape = true; ggui.Invalidate(t.env) })
	}
	if hovered && !open {
		ggui.WakeAt(state.since.Add(t.delay))
	}
	progress := state.reveal.Toggle(open, now, t.env.Motion(t.theme.MotionFast))
	// Kept only while it has something to keep: every tooltip on screen
	// would otherwise store its idle state each frame.
	if hovered || open || progress > 0 || state.dismissed || escaped {
		dst.Retain(at, tooltipSlot, state)
	}
	if !open && progress <= 0 {
		return
	}
	if t.text != nil {
		t.tip.Content(t.text())
	}
	t.effect.Progress(progress, !open) // text-only content never accepts input
	dst.Overlay(func(dst *ggui.Canvas) { t.paintTip(dst, r) })
}

func (t *TooltipWidget) paintTip(dst *ggui.Canvas, anchor ggui.Rect) {
	screen := dst.Size()
	maxW := 320.0
	if screen.W > 0 {
		maxW = min(maxW, screen.W)
	}
	maxH := ggui.Unbounded
	if screen.H > 0 {
		maxH = screen.H
	}
	size := t.effect.Layout(ggui.Loose(ggui.Sz(maxW, maxH)), t.env)
	gap := pick(t.arrow, 0.0, 4.0)
	at := ggui.Pt(anchor.Origin.X+(anchor.Size.W-size.W)/2, anchor.Origin.Y-size.H-gap)
	t.below = false
	if screen != (ggui.Size{}) {
		at.X = clamp(at.X, 0, max(screen.W-size.W, 0))
		if at.Y < 0 {
			t.below = true
			at.Y = min(anchor.Origin.Y+anchor.Size.H+gap, max(screen.H-size.H, 0))
		}
	}
	// Keep the arrow pointed at the anchor when collision shifts the body,
	// without letting it emerge from one of the rounded corners.
	margin := min(t.theme.RadiusSm+5, size.W/2)
	t.arrowX = clamp(anchor.Center().X-at.X, margin, size.W-margin)
	dst.Node(ggui.Rct(at, size), ggui.Node{Role: ggui.RoleGroup}, func(dst *ggui.Canvas) { dst.Paint(t.effect, ggui.Rct(at, size)) })
}

// tooltipSurface includes the arrow in the same transition layer as the body.
// A 10px square with 2px rounded corners, rotated 45 degrees, forms its tip.
// Its four-pixel protrusion occupies the existing body-to-anchor gap.
type tooltipSurface struct{ t *TooltipWidget }

func (s tooltipSurface) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	arrow := pick(s.t.arrow, 4.0, 0.0)
	inner := c
	inner.MinH = max(0, inner.MinH-arrow)
	inner.MaxH = max(0, inner.MaxH-arrow)
	size := s.t.box.Layout(inner, env)
	size.H += arrow
	return c.Constrain(size)
}
func (s tooltipSurface) Paint(dst *ggui.Canvas, r ggui.Rect) {
	t := s.t
	body := r
	if t.arrow {
		body.Size.H = max(0, body.Size.H-4)
		if t.below {
			body.Origin.Y += 4
		}
		y := body.Origin.Y + body.Size.H - 3
		if t.below {
			y = body.Origin.Y + 3
		}
		center := ggui.Pt(r.Origin.X+t.arrowX, y)
		dst.Transform(ggui.Affine{}.Rotate(math.Pi/4).About(center), func(dst *ggui.Canvas) {
			dst.FillRoundRect(ggui.Rct(center.Sub(ggui.Pt(5, 5)), ggui.Sz(10, 10)), 2, t.theme.Fg)
		})
	}
	dst.Paint(t.box, body)
}

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
	side         ggui.PopupSide
	align        ggui.PopupAlignment
	alignOffset  float64
	placed       ggui.PopupSide // the side the tip was last painted on
	arrowAt      float64        // how far along the surface's edge the arrow points
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

const (
	// tooltipMaxWidth is the most a tip's text runs to before it wraps:
	// shadcn's max-w-xs.
	tooltipMaxWidth = 320
	// tooltipGap is the room between a tip's body and its anchor, which
	// the arrow, when there is one, stands in.
	tooltipGap = 4
)

// tooltipEscape closes an open tip.
var tooltipEscape = ggui.MustChord("escape")

var tooltipSlot = ggui.NewSlot[tooltipHover]("tooltip hover")

// Tooltip shows text above child once the cursor rests on it, or while keyboard focus is within child. A press closes it, and
// it stays closed until the cursor leaves and comes back; focus a click
// gives the child does not open it, as focus moved with the keyboard does. It registers no hit
// region, so the child gets every event, and paints through Canvas.Overlay.
func Tooltip(child ggui.Widget, text string) *TooltipWidget {
	t := &TooltipWidget{child: child, tip: ggui.Text(text), delay: 0, arrow: true, side: ggui.PopupTop, placed: ggui.PopupTop, align: ggui.PopupCenter}
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

// Side sets the side of the child the tip opens on, ggui.PopupTop by
// default. It opens on the opposite side when only that one has the room,
// and the arrow points at the child from wherever the tip ends up.
func (t *TooltipWidget) Side(v ggui.PopupSide) *TooltipWidget {
	defer property.Watch(&t.props, &t.side)()
	t.side = v
	return t
}

// Align sets where the tip sits along the child's side: centered by
// default, or lined up with its logical start or end.
func (t *TooltipWidget) Align(v ggui.PopupAlignment) *TooltipWidget {
	defer property.Watch(&t.props, &t.align)()
	t.align = v
	return t
}

// AlignOffset moves the tip along the child's side, away from the edge
// Align lines it up with; see ggui.PopupWidget.AlignOffset.
func (t *TooltipWidget) AlignOffset(v float64) *TooltipWidget {
	defer property.Watch(&t.props, &t.alignOffset)()
	t.alignOffset = v
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
	if t.keys != nil {
		// A key hint is a block of its own, and sits closer to the edge
		// than text does.
		if env.Direction() == ggui.DirRTL {
			t.pad.Left = t.theme.Space * .75
		} else {
			t.pad.Right = t.theme.Space * .75
		}
	}
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
		t.box = ggui.Box(ggui.Row(t.tip, t.keys).Gap(t.theme.Space * .75).Align(ggui.AlignBaseline))
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
	maxW := float64(tooltipMaxWidth)
	if screen.W > 0 {
		maxW = min(maxW, screen.W)
	}
	maxH := ggui.Unbounded
	if screen.H > 0 {
		maxH = screen.H
	}
	// The surface is the body and, on the side facing the anchor, the
	// arrow. Which side that is takes the body's size to settle, and the
	// opposite side is along the same axis, so the size holds either way.
	rtl := t.env.Direction() == ggui.DirRTL
	_, t.placed, _ = floatBeside(ggui.Size{}, anchor, ggui.Size{}, t.side, t.align, 0, 0, rtl)
	size := t.effect.Layout(ggui.Loose(ggui.Sz(maxW, maxH)), t.env)
	body := size
	if t.arrow {
		if t.horizontal() {
			body.W -= tooltipGap
		} else {
			body.H -= tooltipGap
		}
	}
	at, placed, _ := floatBeside(screen, anchor, body, t.side, t.align, t.alignOffset, tooltipGap, rtl)
	t.placed = placed
	// Keep the arrow pointed at the anchor when collision shifts the body,
	// without letting it emerge from one of the rounded corners.
	extent, center := body.W, anchor.Center().X-at.Origin.X
	if t.horizontal() {
		extent, center = body.H, anchor.Center().Y-at.Origin.Y
	}
	margin := min(t.theme.RadiusSm+5, extent/2)
	t.arrowAt = clamp(center, margin, extent-margin)
	if t.arrow {
		switch placed {
		case ggui.PopupBottom:
			at.Origin.Y -= tooltipGap
		case ggui.PopupRight:
			at.Origin.X -= tooltipGap
		}
		at.Size = size
	}
	t.effect.PopFrom(placed, t.arrowAt/max(extent, 1))
	dst.Node(at, ggui.Node{Role: ggui.RoleGroup}, func(dst *ggui.Canvas) { dst.Paint(t.effect, at) })
}

// horizontal reports whether the tip was last placed to the left or right
// of its child.
func (t *TooltipWidget) horizontal() bool {
	return t.placed == ggui.PopupLeft || t.placed == ggui.PopupRight
}

// tooltipSurface includes the arrow in the same transition layer as the body.
// A 10px square with 2px rounded corners, rotated 45 degrees, forms its tip.
// Its four-pixel protrusion occupies the body-to-anchor gap, on whichever
// side of the body faces the anchor.
type tooltipSurface struct{ t *TooltipWidget }

func (s tooltipSurface) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	arrow := pick(s.t.arrow, float64(tooltipGap), 0.0)
	inner := c
	if s.t.horizontal() {
		inner.MinW = max(0, inner.MinW-arrow)
		inner.MaxW = max(0, inner.MaxW-arrow)
	} else {
		inner.MinH = max(0, inner.MinH-arrow)
		inner.MaxH = max(0, inner.MaxH-arrow)
	}
	size := s.t.box.Layout(inner, env)
	if s.t.horizontal() {
		size.W += arrow
	} else {
		size.H += arrow
	}
	return c.Constrain(size)
}

func (s tooltipSurface) Paint(dst *ggui.Canvas, r ggui.Rect) {
	t := s.t
	body := r
	if t.arrow {
		// The square's center sits three pixels inside the body's edge, so
		// that its corner reaches the gap's far side.
		const half, inset = 5, 3
		var center ggui.Point
		switch t.placed {
		case ggui.PopupBottom:
			body.Origin.Y += tooltipGap
			body.Size.H = max(0, body.Size.H-tooltipGap)
			center = ggui.Pt(body.Origin.X+t.arrowAt, body.Origin.Y+inset)
		case ggui.PopupLeft:
			body.Size.W = max(0, body.Size.W-tooltipGap)
			center = ggui.Pt(body.Origin.X+body.Size.W-inset, body.Origin.Y+t.arrowAt)
		case ggui.PopupRight:
			body.Origin.X += tooltipGap
			body.Size.W = max(0, body.Size.W-tooltipGap)
			center = ggui.Pt(body.Origin.X+inset, body.Origin.Y+t.arrowAt)
		default:
			body.Size.H = max(0, body.Size.H-tooltipGap)
			center = ggui.Pt(body.Origin.X+t.arrowAt, body.Origin.Y+body.Size.H-inset)
		}
		dst.Transform(ggui.Affine{}.Rotate(math.Pi/4).About(center), func(dst *ggui.Canvas) {
			dst.FillRoundRect(ggui.Rct(center.Sub(ggui.Pt(half, half)), ggui.Sz(2*half, 2*half)), 2, t.theme.Fg)
		})
	}
	dst.Paint(t.box, body)
}

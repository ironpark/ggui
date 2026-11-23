package ui

import (
	"math"
	"time"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/property"
	uitheme "github.com/ironpark/ggui/ui/theme"
	"github.com/ironpark/ggui/widget"
)

// TooltipWidget shows a short text near its child after the cursor has
// rested on it. Build one with Tooltip.
type TooltipWidget struct {
	props        property.Owner
	child        ggui.Widget
	tip          *widget.TextWidget
	keys         *KbdWidget    // the shortcut hint, if any, as a key cap
	hint         string        // what Shortcut set, before the child's chord
	text         func() string // the text, asked as the tip opens; nil keeps tip's
	delay        time.Duration
	delaySet     bool // Delay was called; otherwise the theme's TooltipDelay
	pad          ggui.EdgeInsets
	arrow        bool
	side         widget.PopupSide
	align        widget.PopupAlignment
	alignOffset  float64
	placed       widget.PopupSide // the side the tip was last painted on
	arrowAt      float64          // how far along the surface's edge the arrow points
	disabled     bool
	disabledProp property.Value[bool]
	escaped      bool // Escape dismissed the tip, as of the last paint
	// focusOpens reports whether keyboard focus opens the tip; nil means
	// focus on a widget the child painted. Tabs, whose strip takes focus
	// as a whole, opens the shown tab's.
	focusOpens func(dst *ggui.Canvas, r ggui.Rect) bool

	box    *widget.BoxWidget
	effect *widget.TransitionWidget
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

// tooltipCue is what may open a tip in a frame: the pointer over its child,
// keyboard focus on it, the frame's time, and how long the pointer must
// rest before the tip opens.
type tooltipCue struct {
	hovered, focused bool
	now              time.Time
	delay            time.Duration
}

// timeRest times the pointer's rest on the child: from when it came over,
// or the delay earlier when another tip of the window was open within
// tooltipSkipDelay before, as of warm. A press while it is over the child
// dismisses the tip until it leaves.
func (h *tooltipHover) timeRest(dst *ggui.Canvas, c tooltipCue, warm time.Time) {
	if c.hovered && h.since.IsZero() {
		h.since = c.now
		if !warm.IsZero() && c.now.Sub(warm) < tooltipSkipDelay {
			h.since = c.now.Add(-c.delay) // another tip was just open
		}
	}
	if !c.hovered {
		h.since, h.dismissed = time.Time{}, false
	}
	if c.hovered && dst.PointerPressed() {
		h.dismissed = true
	}
}

// resting reports whether the pointer rests on the child and no press has
// dismissed the tip.
func (h *tooltipHover) resting(c tooltipCue) bool { return c.hovered && !h.dismissed }

// escaped reports whether Escape dismissed the tip, forgetting it once
// neither the pointer nor focus is on the child. escape is shared with the
// shortcut decideOpen adds, which outlives the paint; only a tip that
// opened has one.
func (h *tooltipHover) escaped(c tooltipCue) bool {
	escaped := h.escape != nil && *h.escape
	if escaped && !c.hovered && !c.focused {
		*h.escape, escaped = false, false
	}
	return escaped
}

const (
	// tooltipMaxWidth is the most a tip's text runs to before it wraps:
	// shadcn's max-w-xs.
	tooltipMaxWidth = 320
	// tooltipGap is the room between a tip's body and its anchor, which
	// the arrow, when there is one, stands in.
	tooltipGap = 4
	// tooltipSkipDelay is how soon after a tip was last open another
	// opens without its delay: running the cursor along a toolbar shows
	// each button's tip at once. shadcn/ui's skipDelayDuration.
	tooltipSkipDelay = 300 * time.Millisecond
)

// tooltipWarm is when a tip of the window was last open, retained under
// a key of its own and kept by every tooltip painted; see tooltipSkipDelay.
var (
	tooltipWarmKey  byte
	tooltipWarm     = ggui.Anchor{ID: &tooltipWarmKey}
	tooltipWarmSlot = ggui.NewSlot[time.Time]("tooltip last open")
)

// tooltipEscape closes an open tip.
var tooltipEscape = ggui.MustChord("escape")

var tooltipSlot = ggui.NewSlot[tooltipHover]("tooltip hover")

// Tooltip shows text above child once the cursor rests on it, or while keyboard focus is within child. A press closes it, and
// it stays closed until the cursor leaves and comes back; focus a click
// gives the child does not open it, as focus moved with the keyboard does. It registers no hit
// region, so the child gets every event, and paints through Canvas.Overlay.
func Tooltip(child ggui.Widget, text string) *TooltipWidget {
	t := &TooltipWidget{child: child, tip: widget.Text(text), delay: 0, arrow: true, side: widget.PopupTop, placed: widget.PopupTop, align: widget.PopupCenter}
	t.box = widget.Box(t.tip)
	t.effect = widget.PopIn(tooltipSurface{t})
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

// Side sets the side of the child the tip opens on, widget.PopupTop by
// default. It opens on the opposite side when only that one has the room,
// and the arrow points at the child from wherever the tip ends up.
func (t *TooltipWidget) Side(v widget.PopupSide) *TooltipWidget {
	defer property.Watch(&t.props, &t.side)()
	t.side = v
	return t
}

// Align sets where the tip sits along the child's side: centered by
// default, or lined up with its logical start or end.
func (t *TooltipWidget) Align(v widget.PopupAlignment) *TooltipWidget {
	defer property.Watch(&t.props, &t.align)()
	t.align = v
	return t
}

// AlignOffset moves the tip along the child's side, away from the edge
// Align lines it up with; see widget.PopupWidget.AlignOffset.
func (t *TooltipWidget) AlignOffset(v float64) *TooltipWidget {
	defer property.Watch(&t.props, &t.alignOffset)()
	t.alignOffset = v
	return t
}

// Delay sets how long the cursor must rest before the tip appears. Unset,
// it is the theme's TooltipDelay. Once a tip has opened, the next opens at
// once if the cursor comes to it soon after.
func (t *TooltipWidget) Delay(d time.Duration) *TooltipWidget {
	defer property.Watch(&t.props, &t.delay)()
	t.delay, t.delaySet = d, true
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
	if label == "" {
		if t.keys != nil {
			t.keys = nil
			t.box = widget.Box(t.tip)
			t.effect = widget.PopIn(tooltipSurface{t})
		}
		return
	}
	if t.keys == nil {
		t.keys = Kbd(label)
		t.box = widget.Box(widget.Row(t.tip, t.keys).Gap(t.theme.Space * .75).Align(widget.AlignCenter))
		t.effect = widget.PopIn(tooltipSurface{t})
	}
	t.keys.Content(label)
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
	cue := tooltipCue{hovered: ok && r.Contains(p), focused: focused, now: ggui.FrameTime(), delay: t.delay}
	if !t.delaySet {
		cue.delay = t.theme.TooltipDelay
	}
	at := ggui.Anchor{Rect: r}
	state, kept := dst.Retained(at, tooltipSlot)
	if !kept {
		// An idle tooltip keeps nothing: it comes back closed, so that
		// opening still eases in.
		state.reveal.MoveTo(0, cue.now, 0)
	}
	open := t.decideOpen(dst, &state, cue)
	progress := state.reveal.Toggle(open, cue.now, t.env.Motion(t.theme.MotionFast))
	// Kept only while it has something to keep: every tooltip on screen
	// would otherwise store its idle state each frame.
	if cue.hovered || open || progress > 0 || state.dismissed || t.escaped {
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

// decideOpen reports whether the tip is open this frame: on keyboard
// focus, or once the pointer has rested on the child for the delay, unless
// the tip is disabled or Escape dismissed it. It keeps the window's tips
// warm, lets Escape close the open tip, and wakes the window when a rest
// under way will have lasted the delay.
func (t *TooltipWidget) decideOpen(dst *ggui.Canvas, state *tooltipHover, cue tooltipCue) bool {
	warm, _ := dst.Retained(tooltipWarm, tooltipWarmSlot)
	state.timeRest(dst, cue, warm)
	escaped := state.escaped(cue)
	t.escaped = escaped
	open := !t.disabled && !escaped && (cue.focused || (state.resting(cue) && cue.now.Sub(state.since) >= cue.delay))
	if open {
		warm = cue.now
	}
	if !warm.IsZero() {
		dst.Retain(tooltipWarm, tooltipWarmSlot, warm)
	}
	if open {
		if state.escape == nil {
			state.escape = new(bool)
		}
		escape := state.escape
		dst.Shortcut(tooltipEscape, func() { *escape = true; ggui.Invalidate(t.env) })
	}
	if !open && !t.disabled && !escaped && state.resting(cue) {
		ggui.WakeAt(state.since.Add(cue.delay))
	}
	return open
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
	_, t.placed, _ = floatBeside(ggui.Size{}, floatSpec{anchor: anchor, side: t.side, align: t.align, rtl: rtl})
	size := t.effect.Layout(ggui.Loose(ggui.Sz(maxW, maxH)), t.env)
	body := size
	if t.arrow {
		if t.horizontal() {
			body.W -= tooltipGap
		} else {
			body.H -= tooltipGap
		}
	}
	at, placed, _ := floatBeside(screen, floatSpec{anchor: anchor, size: body, side: t.side, align: t.align, offset: t.alignOffset, gap: tooltipGap, rtl: rtl})
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
		case widget.PopupBottom:
			at.Origin.Y -= tooltipGap
		case widget.PopupRight:
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
	return t.placed == widget.PopupLeft || t.placed == widget.PopupRight
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
	// The tip is the inverse of the page, and so is a key cap on it.
	size := s.t.box.Layout(inner, env.With(KbdInverseKey, true))
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
		case widget.PopupBottom:
			body.Origin.Y += tooltipGap
			body.Size.H = max(0, body.Size.H-tooltipGap)
			center = ggui.Pt(body.Origin.X+t.arrowAt, body.Origin.Y+inset)
		case widget.PopupLeft:
			body.Size.W = max(0, body.Size.W-tooltipGap)
			center = ggui.Pt(body.Origin.X+body.Size.W-inset, body.Origin.Y+t.arrowAt)
		case widget.PopupRight:
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

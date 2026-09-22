package ui

import (
	"time"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/internal/property"
	uitheme "github.com/ironpark/ggui/ui/theme"
)

// TooltipWidget shows a short text near its child after the cursor has
// rested on it. Build one with Tooltip.
type TooltipWidget struct {
	props property.Owner
	child ggui.Widget
	tip   *ggui.TextWidget
	delay time.Duration
	pad   ggui.EdgeInsets

	box    *ggui.BoxWidget
	effect *ggui.TransitionWidget
	env    ggui.Env
	theme  uitheme.Theme
}

// tooltipHover is the hover timer, retained on the Canvas by Rect so a
// tooltip rebuilt every frame still opens.
type tooltipHover struct {
	since  time.Time
	reveal ggui.Motion
}

var tooltipSlot = ggui.NewSlot[tooltipHover]("tooltip hover")

// Tooltip shows text below child once the cursor has rested on it for half
// a second, or while keyboard focus is within child. It registers no hit
// region, so the child gets every event, and paints through Canvas.Overlay.
func Tooltip(child ggui.Widget, text string) *TooltipWidget {
	t := &TooltipWidget{child: child, tip: ggui.Text(text).Size(12), delay: 500 * time.Millisecond}
	t.box = ggui.Box(t.tip)
	t.effect = ggui.PopIn(t.box)
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
	defer t.props.Layout()()
	t.theme = uitheme.From(env)
	t.env = env
	t.pad = ggui.Insets(t.theme.Space*.75, t.theme.Space*1.5)
	t.tip.Color(t.theme.Bg)
	t.box.Padding(t.pad).Fill(t.theme.Fg).Radius(t.theme.Radius * .75)
	return t.child.Layout(c, env)
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
	dst.Paint(t.child, r)
	p, ok := dst.Pointer()
	now := ggui.Now()
	at := ggui.Anchor{Rect: r}
	state, _ := dst.Retained(at, tooltipSlot)
	focused := dst.FocusWithin(r)
	hovered := ok && r.Contains(p)
	if hovered && state.since.IsZero() {
		state.since = now
	}
	if !hovered {
		state.since = time.Time{}
	}
	open := focused || (hovered && now.Sub(state.since) >= t.delay)
	progress := state.reveal.Toggle(open, now, t.env.Motion(t.theme.MotionFast))
	dst.Retain(at, tooltipSlot, state)
	if !open && progress <= 0 {
		return
	}
	t.effect.Progress(progress, !open) // text-only content never accepts input
	dst.Overlay(func(dst *ggui.Canvas) { t.paintTip(dst, r) })
}

func (t *TooltipWidget) paintTip(dst *ggui.Canvas, anchor ggui.Rect) {
	screen := dst.Size()
	maxW := 280 + t.pad.Left + t.pad.Right
	if screen.W > 0 {
		maxW = min(maxW, screen.W)
	}
	size := t.effect.Layout(ggui.Loose(ggui.Sz(maxW, ggui.Unbounded)), t.env)
	at := ggui.Pt(anchor.Origin.X+(anchor.Size.W-size.W)/2, anchor.Origin.Y+anchor.Size.H+4)
	if screen != (ggui.Size{}) {
		at.X = clamp(at.X, 0, max(screen.W-size.W, 0))
		if at.Y+size.H > screen.H {
			at.Y = max(anchor.Origin.Y-size.H-4, 0)
		}
	}
	dst.Node(ggui.Rct(at, size), ggui.Node{Role: ggui.RoleGroup}, func(dst *ggui.Canvas) { dst.Paint(t.effect, ggui.Rct(at, size)) })
}

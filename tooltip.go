package ggui

import "time"

// TooltipWidget shows a short text near its child after the cursor has
// rested on it. Build one with Tooltip.
type TooltipWidget struct {
	child Widget
	tip   *TextWidget
	delay time.Duration
	pad   EdgeInsets

	box     *BoxWidget
	effect  *TransitionWidget
	env     Env
	tipSize Size
	theme   Theme
}

// tooltipHover is the hover timer, retained on the Canvas by Rect so a
// tooltip rebuilt every frame still opens.
type tooltipHover struct {
	since  time.Time
	reveal Motion
}

var tooltipSlot = NewSlot[tooltipHover]("tooltip hover")

// Tooltip wraps child and shows text below it once the cursor has hovered
// for half a second. It takes no space and registers no hit region, so it
// never steals events from the child, and the hover timer is retained on
// the Canvas, so it survives the widget being rebuilt.
func Tooltip(child Widget, text string) *TooltipWidget {
	t := &TooltipWidget{child: child, tip: Text(text).Size(12), delay: 500 * time.Millisecond}
	t.box = Box(t.tip)
	t.effect = Transition(t.box).Fade().Scale(.95).Easing(EaseLinear)
	return t
}

// Delay sets how long the cursor must rest before the tip appears.
func (t *TooltipWidget) Delay(d time.Duration) *TooltipWidget { t.delay = d; return t }

// Layout implements Widget.
func (t *TooltipWidget) Layout(c Constraints, env Env) Size {
	t.theme = env.Theme()
	t.env = env
	t.pad = Insets(t.theme.Space*.75, t.theme.Space*1.5)
	t.tip.Color(t.theme.Bg)
	t.box.Padding(t.pad).Fill(t.theme.Fg).Radius(t.theme.Radius * .75)
	return t.child.Layout(c, env)
}

// Paint implements Widget.
func (t *TooltipWidget) Paint(dst *Canvas, r Rect) {
	dst.Paint(t.child, r)
	p, ok := dst.Pointer()
	now := Now()
	at := Anchor{Rect: r}
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
	state.reveal.MoveTo(pick(open, 1.0, 0.0), now, t.env.Motion(t.theme.MotionFast))
	progress := state.reveal.Value(now)
	dst.Retain(at, tooltipSlot, state)
	if !open && progress <= 0 {
		return
	}
	t.effect.Progress(progress, !open) // text-only content never accepts input
	dst.Overlay(func(dst *Canvas) { t.paintTip(dst, r) })
}

func (t *TooltipWidget) paintTip(dst *Canvas, anchor Rect) {
	maxW := 280 + t.pad.Left + t.pad.Right
	if screen := dst.Size(); screen.W > 0 {
		maxW = min(maxW, screen.W)
	}
	size := t.effect.Layout(Loose(Sz(maxW, Unbounded)), t.env)
	at := Pt(anchor.Origin.X+(anchor.Size.W-size.W)/2, anchor.Origin.Y+anchor.Size.H+4)
	if screen := dst.Size(); screen != (Size{}) {
		at.X = clamp(at.X, 0, max(screen.W-size.W, 0))
		if at.Y+size.H > screen.H {
			at.Y = max(anchor.Origin.Y-size.H-4, 0)
		}
	}
	dst.Node(Rct(at, size), Node{Role: RoleGroup}, func(dst *Canvas) { dst.Paint(t.effect, Rct(at, size)) })
}

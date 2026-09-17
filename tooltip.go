package ggui

import "time"

// TooltipWidget shows a short text near its child after the cursor has
// rested on it. Build one with Tooltip.
type TooltipWidget struct {
	child Widget
	tip   *TextWidget
	delay time.Duration
	pad   EdgeInsets

	tipSize  Size
	theme    Theme
	hovering bool
	since    time.Time
}

// Tooltip wraps child and shows text below it once the cursor has hovered
// for half a second. It takes no space and registers no hit region, so it
// never steals events from the child.
func Tooltip(child Widget, text string) *TooltipWidget {
	return &TooltipWidget{child: child, tip: Text(text).Size(12), delay: 500 * time.Millisecond}
}

// Delay sets how long the cursor must rest before the tip appears.
func (t *TooltipWidget) Delay(d time.Duration) *TooltipWidget { t.delay = d; return t }

// Layout implements Widget.
func (t *TooltipWidget) Layout(c Constraints, env Env) Size {
	t.theme = env.Theme()
	t.pad = Insets(t.theme.Space*0.5, t.theme.Space)
	t.tip.Color(t.theme.Bg)
	t.tipSize = t.tip.Layout(Loose(Sz(280, Unbounded)), env)
	return t.child.Layout(c, env)
}

// Paint implements Widget.
func (t *TooltipWidget) Paint(dst *Canvas, r Rect) {
	dst.Paint(t.child, r)
	p, ok := dst.Pointer()
	if !ok || !r.Contains(p) {
		t.hovering = false
		return
	}
	now := time.Now()
	if !t.hovering {
		t.hovering, t.since = true, now
	}
	if now.Sub(t.since) < t.delay {
		return
	}
	dst.Overlay(func(dst *Canvas) { t.paintTip(dst, r) })
}

func (t *TooltipWidget) paintTip(dst *Canvas, anchor Rect) {
	size := t.pad.Inflate(t.tipSize)
	at := Pt(anchor.Origin.X+(anchor.Size.W-size.W)/2, anchor.Origin.Y+anchor.Size.H+4)
	if screen := dst.Size(); screen != (Size{}) {
		at.X = clamp(at.X, 0, max(screen.W-size.W, 0))
		if at.Y+size.H > screen.H {
			at.Y = max(anchor.Origin.Y-size.H-4, 0)
		}
	}
	dst.FillRoundRect(Rct(at, size), t.theme.Radius*0.75, t.theme.Fg)
	dst.Paint(t.tip, Rct(at.Add(Pt(t.pad.Left, t.pad.Top)), t.tipSize))
}

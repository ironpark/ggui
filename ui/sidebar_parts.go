package ui

import (
	"math"
	"sync"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui/icons"
	uitheme "github.com/ironpark/ggui/ui/theme"
)

// sidebarToggleName is what the trigger and the rail are called.
const sidebarToggleName = "Toggle Sidebar"

// sidebarPanelIcon is the role of the trigger's icon, which an icon set in
// the Env or the theme may supply; sidebarIcons draws Lucide's panel-left
// where none does.
const sidebarPanelIcon icons.Role = "panel-left"

var sidebarIcons = sync.OnceValue(func() icons.Set {
	svg, err := icons.Parse([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M5 3h14a2 2 0 0 1 2 2v14a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2z" /><path d="M9 3v18" /></svg>`))
	if err != nil {
		panic(err)
	}
	return icons.Map{sidebarPanelIcon: svg}
})

// sidebarTriggerIcon is the trigger's icon, turned to face the other way
// under right-to-left.
type sidebarTriggerIcon struct{ icon *icons.Widget }

func (w sidebarTriggerIcon) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	w.icon.Rotate(pick(env.Direction() == ggui.DirRTL, math.Pi, 0))
	return w.icon.Layout(c, env)
}

func (w sidebarTriggerIcon) Paint(dst *ggui.Canvas, r ggui.Rect) { w.icon.Paint(dst, r) }

// SidebarTrigger creates the button that collapses and expands a sidebar:
// a small ghost button with a panel icon that toggles collapsed, the
// binding the sidebar follows through BindCollapsed. It is a Button, so it
// takes a Shortcut, "cmd+b" in shadcn/ui, and a Tooltip around it shows
// that.
//
//	collapsed := ggui.State(false)
//	bar := ui.Sidebar(page, items...).BindCollapsed(collapsed).Rail(true)
//	top := ggui.Row(ui.SidebarTrigger(collapsed), title)
func SidebarTrigger(collapsed ggui.Binding[bool]) *ButtonWidget {
	icon := sidebarTriggerIcon{icons.Placeholder(sidebarPanelIcon).Fallback(sidebarIcons())}
	return ButtonOf(icon, func() { collapsed.Set(!ggui.Untrack(collapsed.Get)) }).
		Ghost().Pad(6).Name(sidebarToggleName)
}

// SidebarInsetWidget is the content beside an inset sidebar. Build one
// with SidebarInset.
type SidebarInsetWidget struct {
	child ggui.Widget
	theme *uitheme.Theme
	size  ggui.Size
}

// SidebarInset raises child as a rounded panel on the page's background, a
// unit in from every side, over the sidebar's surface: what stands beside
// a Sidebar of SidebarVariantInset, which leaves its inner padding to it.
// It takes all the room it is given.
//
//	ggui.Row(
//		ui.Sidebar(page, items...).Variant(ui.SidebarVariantInset),
//		ggui.Expanded(ui.SidebarInset(content)),
//	)
func SidebarInset(child ggui.Widget) *SidebarInsetWidget {
	return &SidebarInsetWidget{child: child}
}

// Layout implements ggui.Widget.
func (w *SidebarInsetWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	w.theme = uitheme.Of(env)
	pad := 2 * w.theme.Space
	inner := ggui.Constraints{
		MinW: max(c.MinW-pad, 0), MaxW: max(c.MaxW-pad, 0),
		MinH: max(c.MinH-pad, 0), MaxH: max(c.MaxH-pad, 0),
	}
	if c.MaxW < ggui.Unbounded {
		inner.MinW = inner.MaxW
	}
	if c.MaxH < ggui.Unbounded {
		inner.MinH = inner.MaxH
	}
	w.size = w.child.Layout(inner, env)
	return c.Constrain(ggui.Sz(w.size.W+pad, w.size.H+pad))
}

// Paint implements ggui.Widget.
func (w *SidebarInsetWidget) Paint(dst *ggui.Canvas, r ggui.Rect) {
	t := w.theme
	dst.FillRect(r, t.Sidebar)
	panel := ggui.Rct(r.Origin.Add(ggui.Pt(t.Space, t.Space)), ggui.Sz(max(r.Size.W-2*t.Space, 0), max(r.Size.H-2*t.Space, 0)))
	dst.Shadow(panel, t.RadiusLg, t.CardShadow)
	dst.FillRoundRect(panel, t.RadiusLg, t.Bg)
	dst.Clip(panel).Paint(w.child, panel)
}

// sidebarRail is the strip along a sidebar's inner edge: its pointer
// handler and its node.
type sidebarRail struct{ s *SidebarWidget }

// Describe implements ggui.Describer.
func (h sidebarRail) Describe() ggui.Node {
	return ggui.Node{Role: ggui.RoleButton, Name: sidebarToggleName, Disabled: h.s.IsInert(), Actions: ggui.ActionPress}
}

// Semantics implements ggui.Semantic.
func (h sidebarRail) Semantics() (ggui.Role, string) { return ggui.RoleButton, sidebarToggleName }

// Act implements ggui.Actor.
func (h sidebarRail) Act(a ggui.Action) bool {
	if a.Kind != ggui.ActionPress || h.s.IsInert() {
		return false
	}
	h.s.Toggle()
	return true
}

func (h sidebarRail) HandlePointer(ev ggui.PointerEvent) bool {
	switch ev.Kind {
	case ggui.PointerEnter, ggui.PointerMove:
		h.s.railHover = true
	case ggui.PointerExit:
		h.s.railHover = false
	case ggui.PointerTap:
		if ev.Button == ggui.MouseButtonLeft {
			h.s.Toggle()
		}
	case ggui.PointerScroll:
		return false
	}
	return true
}

// paintRail registers the strip a unit wide inside the sidebar's inner
// edge and, under the pointer, draws the line along that edge.
func (s *SidebarWidget) paintRail(dst *ggui.Canvas, r ggui.Rect, right bool) {
	t := s.theme
	w := min(t.Space, r.Size.W)
	strip := ggui.Rct(ggui.Pt(pick(right, r.Origin.X, r.Origin.X+r.Size.W-w), r.Origin.Y), ggui.Sz(w, r.Size.H))
	h := sidebarRail{s}
	dst.Describe(strip, h)
	if s.IsInert() {
		return
	}
	dst.HitPointer(strip, h)
	dst.HitCursor(strip, ggui.CursorShapeEWResize)
	if s.railHover {
		line := min(2*t.BorderWidth, w)
		dst.FillRect(ggui.Rct(ggui.Pt(pick(right, r.Origin.X, r.Origin.X+r.Size.W-line), r.Origin.Y), ggui.Sz(line, r.Size.H)), t.Border)
	}
}

// sidebarTip is the tip a rail shows beside the item under the pointer or
// the keyboard: one for the sidebar, since one row has either at a time.
type sidebarTip struct {
	text   *ggui.TextWidget
	box    *ggui.BoxWidget
	effect *ggui.TransitionWidget
	label  string
	anchor ggui.Rect
	reveal ggui.Motion
}

// paintTip shows the Tooltip of the row the pointer is over, or the
// keyboard is on, while the sidebar is a rail, at once as shadcn/ui's
// does, and eases it away after.
func (s *SidebarWidget) paintTip(dst *ggui.Canvas, right bool) {
	t, tip := s.theme, &s.tip
	want := -1
	if s.railed() && !s.IsInert() {
		tipped := func(i int) bool { return i >= 0 && i < len(s.rows) && s.rows[i].e.tooltip != "" }
		switch {
		case tipped(s.hover):
			want = s.hover
		case s.Focused && s.FocusVisible && tipped(s.active):
			want = s.active
		}
	}
	if want < 0 && tip.effect == nil {
		return
	}
	if want >= 0 {
		if tip.effect == nil {
			tip.text = ggui.Text("")
			tip.box = ggui.Box(tip.text)
			tip.effect = ggui.PopIn(tip.box)
		}
		tip.label, tip.anchor = s.rows[want].e.tooltip, s.rows[want].rect
	}
	progress := tip.reveal.Toggle(want >= 0, ggui.FrameTime(), s.env.Motion(t.MotionFast))
	if progress <= 0 {
		return
	}
	tip.text.Content(tip.label).Style(ggui.TextStyle{Size: t.Caption.Size}).Color(t.Bg)
	tip.box.Padding(ggui.Insets(t.Space*.75, t.Space*1.5)).Fill(t.Fg).Radius(t.Radius * .75)
	tip.effect.Progress(progress, want < 0)
	dst.Overlay(func(dst *ggui.Canvas) {
		gap, anchor := t.Space/2, tip.anchor
		size := tip.effect.Layout(ggui.Loose(ggui.Sz(280, ggui.Unbounded)), s.env)
		at := ggui.Pt(anchor.Origin.X+anchor.Size.W+gap, anchor.Origin.Y+(anchor.Size.H-size.H)/2)
		if right {
			at.X = anchor.Origin.X - gap - size.W
		}
		if screen := dst.Size(); screen != (ggui.Size{}) {
			at.X = clamp(at.X, 0, max(screen.W-size.W, 0))
			at.Y = clamp(at.Y, 0, max(screen.H-size.H, 0))
		}
		dst.Node(ggui.Rct(at, size), ggui.Node{Role: ggui.RoleGroup}, func(dst *ggui.Canvas) { dst.Paint(tip.effect, ggui.Rct(at, size)) })
	})
}

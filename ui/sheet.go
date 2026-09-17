package ui

import "github.com/ironpark/ggui"

// sheetSide says which edge a sheet slides in from.
type sheetSide int

const (
	sheetRight sheetSide = iota // the default
	sheetLeft
	sheetTop
	sheetBottom // what Drawer uses
)

// SheetWidget is a modal panel anchored to an edge of the window: a scrim
// dims everything else and takes the clicks, Tab stays inside, Escape or a
// click on the scrim closes it, and focus returns to where it was. Build one
// with Sheet, or with Drawer for the bottom edge. It takes no space where it
// sits in the tree; put it anywhere.
//
// The sheet slides in and out over the theme's MotionSlow. The motion lives
// in the widget, so a sheet kept across rebuilds -- in a Component, or
// simply constructed once beside the signal it binds -- animates, and one
// rebuilt from scratch every frame appears and goes at once, as Dialog does.
type SheetWidget struct {
	modal
	side   sheetSide
	extent float64
	handle bool
	slide  ggui.Motion
}

// Sheet creates a panel that shows content along the window's right edge
// while open is true.
//
//	filters := ggui.State(false)
//	ui.Sheet(filters, filterForm).Title("Filters").Left()
func Sheet(open ggui.Binding[bool], content ggui.Widget) *SheetWidget {
	s := &SheetWidget{extent: 320}
	s.open, s.content, s.dismissible = open, content, true
	return s
}

// Drawer is a Sheet that rises from the bottom edge, with the grab handle
// that says so. It is the shape a phone-style panel takes.
func Drawer(open ggui.Binding[bool], content ggui.Widget) *SheetWidget {
	s := Sheet(open, content).Bottom()
	s.extent, s.handle = 280, true
	return s
}

// Title puts a heading above the content, which also names the sheet for
// Probe.Find.
func (s *SheetWidget) Title(v string) *SheetWidget { s.setTitle(v); return s }

// Named sets an accessible name without adding a visible heading.
func (s *SheetWidget) Named(name string) *SheetWidget { s.name = name; return s }

// Compact removes the outer padding for content with its own spacing.
func (s *SheetWidget) Compact() *SheetWidget { s.compact = true; return s }

// Dismissible says whether a click on the scrim closes the sheet; it does
// by default. Escape closes either way.
func (s *SheetWidget) Dismissible(v bool) *SheetWidget { s.dismissible = v; return s }

// Left anchors the sheet to the left edge.
func (s *SheetWidget) Left() *SheetWidget { s.side = sheetLeft; return s }

// Right anchors the sheet to the right edge, which is where it starts.
func (s *SheetWidget) Right() *SheetWidget { s.side = sheetRight; return s }

// Top anchors the sheet to the top edge.
func (s *SheetWidget) Top() *SheetWidget { s.side = sheetTop; return s }

// Bottom anchors the sheet to the bottom edge.
func (s *SheetWidget) Bottom() *SheetWidget { s.side = sheetBottom; return s }

// Size sets how far the sheet reaches from its edge: its width on the left
// or right, its height on the top or bottom. It shrinks to fit a smaller
// window.
func (s *SheetWidget) Size(v float64) *SheetWidget { s.extent = max(0, v); return s }

// OnClose fires when the sheet closes, however it closed.
func (s *SheetWidget) OnClose(fn func()) *SheetWidget { s.onClose = fn; return s }

// horizontal reports whether the sheet slides along the x axis.
func (s *SheetWidget) horizontal() bool { return s.side == sheetLeft || s.side == sheetRight }

// Layout implements ggui.Widget: the sheet takes no room in the tree. The
// panel gets no border or radius: it is flush with the edge it came from.
func (s *SheetWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	s.build(env)
	return c.Constrain(ggui.Size{})
}

// Paint implements ggui.Widget: the panel paints through Canvas.Overlay
// while it is open, and while it is still on its way out.
func (s *SheetWidget) Paint(dst *ggui.Canvas, _ ggui.Rect) {
	open := s.open.Peek()
	now := ggui.Now()
	s.slide.MoveTo(pick(open, 1.0, 0.0), now, s.env.Motion(s.theme.MotionSlow))
	v := clamp(s.slide.Value(now), 0, 1)
	if !open && v <= 0.001 {
		s.rect = ggui.Rect{}
		return
	}
	dst.Overlay(func(dst *ggui.Canvas) { s.paintPanel(dst, v, open) })
}

func (s *SheetWidget) paintPanel(dst *ggui.Canvas, v float64, open bool) {
	t := s.theme
	screen := dst.Size()
	if screen == (ggui.Size{}) {
		screen = ggui.Sz(s.extent, s.extent)
	}
	var cs ggui.Constraints
	if s.horizontal() {
		w := min(s.extent, screen.W)
		cs = ggui.Constraints{MinW: w, MaxW: w, MinH: screen.H, MaxH: screen.H}
	} else {
		h := min(s.extent, screen.H)
		cs = ggui.Constraints{MinW: screen.W, MaxW: screen.W, MinH: h, MaxH: h}
	}
	size := s.panel.Layout(cs, s.env)
	var at ggui.Point
	switch s.side {
	case sheetLeft:
		at = ggui.Pt(-(1-v)*size.W, 0)
	case sheetTop:
		at = ggui.Pt(0, -(1-v)*size.H)
	case sheetBottom:
		at = ggui.Pt(0, screen.H-size.H+(1-v)*size.H)
	default:
		at = ggui.Pt(screen.W-size.W+(1-v)*size.W, 0)
	}

	// A sheet on its way out is a picture: it must not take the click that
	// lands where it used to be, nor hold focus it is about to give back.
	if !open {
		dst = dst.Inert()
	}
	rect := ggui.Rct(at, size)
	s.paint(dst, screen, rect, fade(t.Scrim, v), s)
	if s.handle {
		grip := ggui.Sz(36.0, 4.0)
		at := ggui.Pt(rect.Origin.X+(rect.Size.W-grip.W)/2, rect.Origin.Y+t.Space/2)
		dst.FillRoundRect(ggui.Rct(at, grip), grip.H/2, t.Border)
	}
}
